package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/alecthomas/kingpin"
	contracts "github.com/estafette/estafette-ci-contracts"
	foundation "github.com/estafette/estafette-foundation"
	"github.com/opentracing/opentracing-go"
	"github.com/rs/zerolog/log"
	"github.com/uber/jaeger-client-go"
	jaegercfg "github.com/uber/jaeger-client-go/config"
)

var (
	appgroup  string
	app       string
	version   string
	branch    string
	revision  string
	buildDate string
	goVersion = runtime.Version()

	// params for apiClient
	apiBaseURL   = kingpin.Flag("api-base-url", "The base url of the estafette-ci-api to communicate with").Envar("API_BASE_URL").Required().String()
	clientID     = kingpin.Flag("client-id", "The id of the client as configured in Estafette, to securely communicate with the api.").Envar("CLIENT_ID").Required().String()
	clientSecret = kingpin.Flag("client-secret", "The secret of the client as configured in Estafette, to securely communicate with the api.").Envar("CLIENT_SECRET").Required().String()

	// other params for gsuiteClient
	pipelinesToExtract = kingpin.Flag("pipelines-to-extract", "A comma separated list of pipelines to extract.").Envar("PIPELINES_TO_EXTRACT").Required().String()
	saveToDirectory    = kingpin.Flag("save-to-directory", "Directory to store responses.").Default("./mocks").OverrideDefaultFromEnvar("SAVE_TO_DIRECTORY").String()
	logObfuscateRegex  = kingpin.Flag("log-obfuscate-regex", "Regular expression to obfuscate parts of the logs").Envar("LOG_OBFUSCATE_REGEX").String()
)

func main() {

	// parse command line parameters
	kingpin.Parse()

	// init log format from envvar ESTAFETTE_LOG_FORMAT
	foundation.InitLoggingFromEnv(foundation.NewApplicationInfo(appgroup, app, version, branch, revision, buildDate))

	closer := initJaeger(app)
	defer closer.Close()

	ctx := context.Background()

	span, ctx := opentracing.StartSpanFromContext(ctx, "Main")
	defer span.Finish()

	apiClient := NewApiClient(*apiBaseURL)

	token, err := apiClient.GetToken(ctx, *clientID, *clientSecret)
	handleError(closer, err)

	pipelines := PipelinesListResponse{
		Items: []*contracts.Pipeline{},
	}

	for _, p := range strings.Split(*pipelinesToExtract, ",") {
		pipeline, err := apiClient.GetPipeline(ctx, token, p)
		handleError(closer, err)

		if pipeline != nil {

			obfuscatePipeline(pipeline)

			pipelines.Items = append(pipelines.Items, pipeline)

			err = saveObjectToFile(filepath.Join("/api/pipelines", p), pipeline)
			handleError(closer, err)

			// store builds json
			builds, err := apiClient.GetPipelineBuilds(ctx, token, p)
			handleError(closer, err)
			builds.Pagination.TotalPages = 1
			builds.Pagination.TotalItems = len(builds.Items)

			// http://jmoiron.net/blog/limiting-concurrency-in-go/
			concurrency := 10
			semaphore := make(chan bool, concurrency)

			for _, b := range builds.Items {
				obfuscateBuild(b)
			}

			err = saveObjectToFile(filepath.Join("/api/pipelines", p, "builds"), builds)
			handleError(closer, err)

			// loop builds
			for _, b := range builds.Items {
				// try to fill semaphore up to it's full size otherwise wait for a routine to finish
				semaphore <- true

				go func(b *contracts.Build) {
					// lower semaphore once the routine's finished, making room for another one to start
					defer func() { <-semaphore }()

					// store build json
					url := fmt.Sprintf("/api/pipelines/%v/builds/%v", p, b.ID)

					build, err := apiClient.GetPipelineBuild(ctx, token, url)
					handleError(closer, err)

					obfuscateBuild(build)

					err = saveObjectToFile(url, build)
					handleError(closer, err)

					// store build logs json
					url = fmt.Sprintf("/api/pipelines/%v/builds/%v/logs", p, b.ID)

					bytes, err := apiClient.GetBytesResponse(ctx, token, url)
					handleError(closer, err)

					bytes = obfuscateLog(bytes)

					err = saveBytesToFile(url, bytes)
					handleError(closer, err)
				}(b)
			}

			// store releases json
			releases, err := apiClient.GetPipelineReleases(ctx, token, p)
			handleError(closer, err)
			releases.Pagination.TotalPages = 1
			releases.Pagination.TotalItems = len(builds.Items)

			for _, r := range releases.Items {
				obfuscateRelease(r)
			}

			err = saveObjectToFile(filepath.Join("/api/pipelines", p, "releases"), releases)
			handleError(closer, err)

			// loop releases
			for _, r := range releases.Items {
				// try to fill semaphore up to it's full size otherwise wait for a routine to finish
				semaphore <- true

				go func(r *contracts.Release) {
					// lower semaphore once the routine's finished, making room for another one to start
					defer func() { <-semaphore }()

					// store release json
					url := fmt.Sprintf("/api/pipelines/%v/releases/%v", p, r.ID)

					release, err := apiClient.GetPipelineRelease(ctx, token, url)
					handleError(closer, err)

					obfuscateRelease(release)

					err = saveObjectToFile(url, release)
					handleError(closer, err)

					// store release logs json
					url = fmt.Sprintf("/api/pipelines/%v/releases/%v/logs", p, r.ID)

					bytes, err := apiClient.GetBytesResponse(ctx, token, url)
					handleError(closer, err)

					bytes = obfuscateLog(bytes)

					err = saveBytesToFile(url, bytes)
					handleError(closer, err)
				}(r)
			}

			pipelinesSubPaths := []string{"warnings", "stats/buildsdurations", "stats/buildscpu", "stats/buildsmemory", "stats/releasesdurations", "stats/releasescpu", "stats/releasesmemory"}
			for _, path := range pipelinesSubPaths {
				// try to fill semaphore up to it's full size otherwise wait for a routine to finish
				semaphore <- true

				go func(path string) {
					// lower semaphore once the routine's finished, making room for another one to start
					defer func() { <-semaphore }()

					url := fmt.Sprintf("/api/pipelines/%v/%v", p, path)

					bytes, err := apiClient.GetBytesResponse(ctx, token, url)
					handleError(closer, err)

					err = saveBytesToFile(url, bytes)
					handleError(closer, err)
				}(path)
			}

			// try to fill semaphore up to it's full size which only succeeds if all routines have finished
			for i := 0; i < cap(semaphore); i++ {
				semaphore <- true
			}
		}
	}

	if len(pipelines.Items) > 0 {
		pipelines.Pagination = contracts.Pagination{
			Page:       1,
			Size:       12,
			TotalItems: len(pipelines.Items),
			TotalPages: 1,
		}

		err = saveObjectToFile("/api/pipelines", pipelines)
		handleError(closer, err)
	}
}

func handleError(jaegerCloser io.Closer, err error) {
	if err != nil {
		jaegerCloser.Close()
		log.Fatal().Err(err).Msg("Failure")
	}
}

// initJaeger returns an instance of Jaeger Tracer that can be configured with environment variables
// https://github.com/jaegertracing/jaeger-client-go#environment-variables
func initJaeger(service string) io.Closer {

	cfg, err := jaegercfg.FromEnv()
	if err != nil {
		log.Fatal().Err(err).Msg("Generating Jaeger config from environment variables failed")
	}

	closer, err := cfg.InitGlobalTracer(service, jaegercfg.Logger(jaeger.StdLogger))
	if err != nil {
		log.Fatal().Err(err).Msg("Generating Jaeger tracer failed")
	}

	return closer
}

func saveObjectToFile(path string, object interface{}) (err error) {

	bytes, err := json.MarshalIndent(object, "", "  ")
	if err != nil {
		return
	}

	return saveBytesToFile(path, bytes)
}

func saveBytesToFile(path string, bytes []byte) (err error) {
	targetDir := filepath.Join(*saveToDirectory, path)
	err = os.MkdirAll(targetDir, os.ModePerm)
	if err != nil {
		return
	}

	targetPath := filepath.Join(targetDir, "/GET.json")
	err = ioutil.WriteFile(targetPath, bytes, 0644)
	if err != nil {
		return
	}

	log.Info().Msgf("Fetched and saved %v", path)

	return nil
}

func obfuscatePipeline(pipeline *contracts.Pipeline) {
	for i := 0; i < len(pipeline.Commits); i++ {
		pipeline.Commits[i].Author.Email = "me@estafette.io"
		pipeline.Commits[i].Author.Name = "Just Me"
		pipeline.Commits[i].Author.Username = "JustMe"
	}

	for i := 0; i < len(pipeline.ReleaseTargets); i++ {
		for j := 0; j < len(pipeline.ReleaseTargets[i].ActiveReleases); j++ {
			for k := 0; k < len(pipeline.ReleaseTargets[i].ActiveReleases[j].Events); k++ {
				if pipeline.ReleaseTargets[i].ActiveReleases[j].Events[k].Manual != nil {
					pipeline.ReleaseTargets[i].ActiveReleases[j].Events[k].Manual.UserID = "me@estafette.io"
				}
			}
		}
	}

	for i := 0; i < len(pipeline.Events); i++ {
		if pipeline.Events[i].Manual != nil {
			pipeline.Events[i].Manual.UserID = "me@estafette.io"
		}
	}
}

func obfuscateBuild(build *contracts.Build) {
	for i := 0; i < len(build.Commits); i++ {
		build.Commits[i].Author.Email = "me@estafette.io"
		build.Commits[i].Author.Name = "Just Me"
		build.Commits[i].Author.Username = "JustMe"
	}

	for i := 0; i < len(build.ReleaseTargets); i++ {
		for j := 0; j < len(build.ReleaseTargets[i].ActiveReleases); j++ {
			for k := 0; k < len(build.ReleaseTargets[i].ActiveReleases[j].Events); k++ {
				if build.ReleaseTargets[i].ActiveReleases[j].Events[k].Manual != nil {
					build.ReleaseTargets[i].ActiveReleases[j].Events[k].Manual.UserID = "me@estafette.io"
				}
			}
		}
	}

	for i := 0; i < len(build.Events); i++ {
		if build.Events[i].Manual != nil {
			build.Events[i].Manual.UserID = "me@estafette.io"
		}
	}
}

func obfuscateRelease(release *contracts.Release) {
	for i := 0; i < len(release.Events); i++ {
		if release.Events[i].Manual != nil {
			release.Events[i].Manual.UserID = "me@estafette.io"
		}
	}
}

func obfuscateLog(bytes []byte) []byte {
	re := regexp.MustCompile(`[a-z0-9-]+@[a-z0-9-]+\.iam\.gserviceaccount\.com`)
	bytes = re.ReplaceAll(bytes, []byte("***@***.iam.gserviceaccount.com"))

	if *logObfuscateRegex != "" {
		re2 := regexp.MustCompile(*logObfuscateRegex)
		bytes = re2.ReplaceAll(bytes, []byte("***"))
	}

	return bytes
}

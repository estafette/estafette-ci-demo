package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
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
	saveToDirectory    = kingpin.Flag("save-to-directory", "Directory to store responses.").Default("./mocks/api").OverrideDefaultFromEnvar("SAVE_TO_DIRECTORY").String()
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
	handleError(closer, err, "Failed retrieving JWT token")

	pipelines := PipelinesListResponse{
		Items: []*contracts.Pipeline{},
	}

	for _, p := range strings.Split(*pipelinesToExtract, ",") {
		pipeline, err := apiClient.GetPipeline(ctx, token, p)
		handleError(closer, err, "Failed fetching pipeline")

		log.Info().Msgf("Fetched /api/pipelines/%v", p)

		if pipeline != nil {
			pipelines.Items = append(pipelines.Items)

			// store pipeline json
			targetDir := filepath.Join(*saveToDirectory, "pipelines", p)
			err = os.MkdirAll(targetDir, os.ModePerm)
			handleError(closer, err, "Failed creating pipeline target dir")

			file, err := json.MarshalIndent(pipeline, "", "  ")
			handleError(closer, err, "Failed marshalling pipeline")

			targetPath := filepath.Join(targetDir, "/GET.json")
			err = ioutil.WriteFile(targetPath, file, 0644)
			handleError(closer, err, "Failed saving pipeline json")

			log.Info().Msgf("Saved %v", targetPath)

			// store builds json
			builds, err := apiClient.GetPipelineBuilds(ctx, token, p)
			handleError(closer, err, "Failed fetching pipeline builds")
			builds.Pagination.TotalPages = 1
			builds.Pagination.TotalItems = len(builds.Items)

			log.Info().Msgf("Fetched /api/pipelines/%v/builds", p)

			targetDir = filepath.Join(*saveToDirectory, "pipelines", p, "builds")
			err = os.MkdirAll(targetDir, os.ModePerm)
			handleError(closer, err, "Failed creating builds target dir")

			file, err = json.MarshalIndent(builds, "", "  ")
			handleError(closer, err, "Failed marshalling builds")

			targetPath = filepath.Join(targetDir, "/GET.json")
			err = ioutil.WriteFile(targetPath, file, 0644)
			handleError(closer, err, "Failed saving builds json")

			log.Info().Msgf("Saved %v", targetPath)

			// store releases json
			releases, err := apiClient.GetPipelineReleases(ctx, token, p)
			handleError(closer, err, "Failed fetching pipeline releases")
			releases.Pagination.TotalPages = 1
			releases.Pagination.TotalItems = len(builds.Items)

			log.Info().Msgf("Fetched /api/pipelines/%v/releases", p)

			targetDir = filepath.Join(*saveToDirectory, "pipelines", p, "releases")
			err = os.MkdirAll(targetDir, os.ModePerm)
			handleError(closer, err, "Failed creating releases target dir")

			file, err = json.MarshalIndent(releases, "", "  ")
			handleError(closer, err, "Failed marshalling releases")

			targetPath = filepath.Join(targetDir, "/GET.json")
			err = ioutil.WriteFile(targetPath, file, 0644)
			handleError(closer, err, "Failed saving releases json")

			log.Info().Msgf("Saved %v", targetPath)

			pipelinesSubPaths := []string{"warnings", "buildsdurations", "buildscpu", "buildsmemory", "releasesdurations", "releasescpu", "releasesmemory"}
			for _, path := range pipelinesSubPaths {

				bytes, err := apiClient.GetBytesResponse(ctx, token, fmt.Sprintf("/api/pipelines/%v/%v", p, path))
				handleError(closer, err, "Failed fetching bytes response")

				log.Info().Msgf("Fetched /api/pipelines/%v/%v", p, path)

				targetDir = filepath.Join(*saveToDirectory, "pipelines", p, path)
				err = os.MkdirAll(targetDir, os.ModePerm)
				handleError(closer, err, "Failed creating bytes target dir")

				targetPath := filepath.Join(targetDir, "/GET.json")
				err = ioutil.WriteFile(targetPath, bytes, 0644)
				handleError(closer, err, "Failed saving bytes json")
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

		targetDir := filepath.Join(*saveToDirectory, "pipelines")

		file, err := json.MarshalIndent(pipelines, "", "  ")
		handleError(closer, err, "Failed marshalling pipelines")

		targetPath := filepath.Join(targetDir, "/GET.json")
		err = ioutil.WriteFile(targetPath, file, 0644)
		handleError(closer, err, "Failed saving pipelines json")

		log.Info().Msgf("Saved %v", targetPath)
	}
}

func handleError(jaegerCloser io.Closer, err error, message string) {
	if err != nil {
		jaegerCloser.Close()
		log.Fatal().Err(err).Msg(message)
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

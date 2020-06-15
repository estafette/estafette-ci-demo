package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	contracts "github.com/estafette/estafette-ci-contracts"
	foundation "github.com/estafette/estafette-foundation"
	"github.com/opentracing-contrib/go-stdlib/nethttp"
	"github.com/opentracing/opentracing-go"
	"github.com/r3labs/sse"
	"github.com/rs/zerolog/log"
	"github.com/sethgrid/pester"
)

const gsuiteProviderName = "gsuite"
const googleProviderName = "google"

type ApiClient interface {
	GetToken(ctx context.Context, clientID, clientSecret string) (token string, err error)
	GetPipelines(ctx context.Context, token string, pageNumber, pageSize int, filters map[string][]string) (response PipelinesListResponse, err error)
	GetPipeline(ctx context.Context, token string, pipelinePath string) (pipeline *contracts.Pipeline, err error)
	GetPipelineBuilds(ctx context.Context, token string, pipelinePath string) (response PipelineBuildsListResponse, err error)
	GetPipelineBuild(ctx context.Context, token string, pipelineBuildPath string) (build *contracts.Build, err error)
	GetPipelineReleases(ctx context.Context, token string, pipelinePath string) (response PipelineReleasesListResponse, err error)
	GetPipelineRelease(ctx context.Context, token string, pipelineReleasePath string) (release *contracts.Release, err error)
	GetBytesResponse(ctx context.Context, token string, path string) (bytes []byte, err error)
	GetSSEResponse(ctx context.Context, token string, path string, maxNumberOfEvents int) (bytes []byte, err error)
}

// NewApiClient returns a new ApiClient
func NewApiClient(apiBaseURL string) ApiClient {
	return &apiClient{
		apiBaseURL: apiBaseURL,
	}
}

type apiClient struct {
	apiBaseURL string
}

func (c *apiClient) GetToken(ctx context.Context, clientID, clientSecret string) (token string, err error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "ApiClient::GetToken")
	defer span.Finish()

	clientObject := contracts.Client{
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}

	bytes, err := json.Marshal(clientObject)
	if err != nil {
		return
	}

	getTokenURL := fmt.Sprintf("%v/api/auth/client/login", c.apiBaseURL)
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	responseBody, err := c.postRequest(getTokenURL, span, strings.NewReader(string(bytes)), headers)
	if err != nil {
		return
	}

	tokenResponse := struct {
		Token string `json:"token"`
	}{}

	// unmarshal json body
	err = json.Unmarshal(responseBody, &tokenResponse)
	if err != nil {
		log.Error().Err(err).Str("body", string(responseBody)).Msgf("Failed unmarshalling get token response")
		return
	}

	return tokenResponse.Token, nil
}

func (c *apiClient) GetPipelines(ctx context.Context, token string, pageNumber, pageSize int, filters map[string][]string) (response PipelinesListResponse, err error) {

	span, ctx := opentracing.StartSpanFromContext(ctx, "ApiClient::GetPipelines")
	defer span.Finish()

	getPipelinesURL := fmt.Sprintf("%v/api/pipelines?page[number]=%v&page[size]=%v", c.apiBaseURL, pageNumber, pageSize)
	for k, v := range filters {
		for _, vv := range v {
			getPipelinesURL += "&" + k + "=" + vv
		}
	}

	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %v", token),
		"Content-Type":  "application/json",
	}

	responseBody, err := c.getRequest(getPipelinesURL, span, nil, headers)
	if err != nil {
		return
	}

	// unmarshal json body
	err = json.Unmarshal(responseBody, &response)
	if err != nil {
		log.Error().Err(err).Str("body", string(responseBody)).Msgf("Failed unmarshalling get pipelines response from %v", getPipelinesURL)
		return
	}

	return response, nil
}

func (c *apiClient) GetPipeline(ctx context.Context, token string, pipelinePath string) (pipeline *contracts.Pipeline, err error) {

	span, ctx := opentracing.StartSpanFromContext(ctx, "ApiClient::GetPipeline")
	defer span.Finish()

	getPipelineURL := fmt.Sprintf("%v/api/pipelines/%v", c.apiBaseURL, pipelinePath)

	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %v", token),
		"Content-Type":  "application/json",
	}

	responseBody, err := c.getRequest(getPipelineURL, span, nil, headers)
	if err != nil {
		return
	}

	// unmarshal json body
	err = json.Unmarshal(responseBody, &pipeline)
	if err != nil {
		log.Error().Err(err).Str("body", string(responseBody)).Msgf("Failed unmarshalling get pipeline response from %v", getPipelineURL)
		return
	}

	return pipeline, nil
}

func (c *apiClient) GetPipelineBuilds(ctx context.Context, token string, pipelinePath string) (response PipelineBuildsListResponse, err error) {

	span, ctx := opentracing.StartSpanFromContext(ctx, "ApiClient::GetPipelineBuilds")
	defer span.Finish()

	getPipelineBuildsURL := fmt.Sprintf("%v/api/pipelines/%v/builds?page[number]=%v&page[size]=%v", c.apiBaseURL, pipelinePath, 1, 10)

	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %v", token),
		"Content-Type":  "application/json",
	}

	responseBody, err := c.getRequest(getPipelineBuildsURL, span, nil, headers)
	if err != nil {
		return
	}

	// unmarshal json body
	err = json.Unmarshal(responseBody, &response)
	if err != nil {
		log.Error().Err(err).Str("body", string(responseBody)).Msgf("Failed unmarshalling get pipeline builds response from %v", getPipelineBuildsURL)
		return
	}

	return response, nil
}

func (c *apiClient) GetPipelineBuild(ctx context.Context, token string, pipelineBuildPath string) (build *contracts.Build, err error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "ApiClient::GetPipelineBuild")
	defer span.Finish()

	getPipelineBuildURL := fmt.Sprintf("%v%v", c.apiBaseURL, pipelineBuildPath)

	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %v", token),
		"Content-Type":  "application/json",
	}

	responseBody, err := c.getRequest(getPipelineBuildURL, span, nil, headers)
	if err != nil {
		return
	}

	// unmarshal json body
	err = json.Unmarshal(responseBody, &build)
	if err != nil {
		log.Error().Err(err).Str("body", string(responseBody)).Msgf("Failed unmarshalling get pipeline build response from %v", getPipelineBuildURL)
		return
	}

	return build, nil
}

func (c *apiClient) GetPipelineReleases(ctx context.Context, token string, pipelinePath string) (response PipelineReleasesListResponse, err error) {

	span, ctx := opentracing.StartSpanFromContext(ctx, "ApiClient::GetPipelineReleases")
	defer span.Finish()

	getPipelineReleasesURL := fmt.Sprintf("%v/api/pipelines/%v/releases?page[number]=%v&page[size]=%v", c.apiBaseURL, pipelinePath, 1, 10)

	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %v", token),
		"Content-Type":  "application/json",
	}

	responseBody, err := c.getRequest(getPipelineReleasesURL, span, nil, headers)
	if err != nil {
		return
	}

	// unmarshal json body
	err = json.Unmarshal(responseBody, &response)
	if err != nil {
		log.Error().Err(err).Str("body", string(responseBody)).Msgf("Failed unmarshalling get pipeline releases response from %v", getPipelineReleasesURL)
		return
	}

	return response, nil
}

func (c *apiClient) GetPipelineRelease(ctx context.Context, token string, pipelineReleasePath string) (release *contracts.Release, err error) {

	span, ctx := opentracing.StartSpanFromContext(ctx, "ApiClient::GetPipelineRelease")
	defer span.Finish()

	getPipelineReleaseURL := fmt.Sprintf("%v%v", c.apiBaseURL, pipelineReleasePath)

	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %v", token),
		"Content-Type":  "application/json",
	}

	responseBody, err := c.getRequest(getPipelineReleaseURL, span, nil, headers)
	if err != nil {
		return
	}

	// unmarshal json body
	err = json.Unmarshal(responseBody, &release)
	if err != nil {
		log.Error().Err(err).Str("body", string(responseBody)).Msgf("Failed unmarshalling get pipeline release response from %v", getPipelineReleaseURL)
		return
	}

	return release, nil
}

func (c *apiClient) GetBytesResponse(ctx context.Context, token string, path string) (bytes []byte, err error) {

	span, ctx := opentracing.StartSpanFromContext(ctx, "ApiClient::GetBytesResponse")
	defer span.Finish()

	url := fmt.Sprintf("%v%v", c.apiBaseURL, path)

	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %v", token),
		"Content-Type":  "application/json",
	}

	bytes, err = c.getRequest(url, span, nil, headers)
	if err != nil {
		return
	}

	return bytes, nil
}

func (c *apiClient) GetSSEResponse(ctx context.Context, token string, path string, maxNumberOfEvents int) (bytes []byte, err error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "ApiClient::GetSSEResponse")
	defer span.Finish()

	url := fmt.Sprintf("%v%v", c.apiBaseURL, path)

	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %v", token),
		"Content-Type":  "application/json",
	}

	client := sse.NewClient(url)
	client.Headers = headers

	events := make(chan *sse.Event)
	err = client.SubscribeChanRaw(events)
	if err != nil {
		return bytes, err
	}
	defer client.Unsubscribe(events)

	for i := 0; i < maxNumberOfEvents; i++ {
		select {
		case msg := <-events:

			// add line with event type (event:log)
			bytes = append(bytes, []byte(fmt.Sprintf("event:%v\n", msg.Event))...)

			// add line with data (data:true)
			bytes = append(bytes, []byte("data:")...)
			bytes = append(bytes, msg.Data...)
			bytes = append(bytes, []byte("\n\n")...)
		case <-time.After(time.Second * 5):
			return bytes, nil
		}
	}

	return bytes, nil
}

func (c *apiClient) getRequest(uri string, span opentracing.Span, requestBody io.Reader, headers map[string]string, allowedStatusCodes ...int) (responseBody []byte, err error) {
	return c.makeRequest("GET", uri, span, requestBody, headers, allowedStatusCodes...)
}

func (c *apiClient) postRequest(uri string, span opentracing.Span, requestBody io.Reader, headers map[string]string, allowedStatusCodes ...int) (responseBody []byte, err error) {
	return c.makeRequest("POST", uri, span, requestBody, headers, allowedStatusCodes...)
}

func (c *apiClient) putRequest(uri string, span opentracing.Span, requestBody io.Reader, headers map[string]string, allowedStatusCodes ...int) (responseBody []byte, err error) {
	return c.makeRequest("PUT", uri, span, requestBody, headers, allowedStatusCodes...)
}

func (c *apiClient) deleteRequest(uri string, span opentracing.Span, requestBody io.Reader, headers map[string]string, allowedStatusCodes ...int) (responseBody []byte, err error) {
	return c.makeRequest("DELETE", uri, span, requestBody, headers, allowedStatusCodes...)
}

func (c *apiClient) makeRequest(method, uri string, span opentracing.Span, requestBody io.Reader, headers map[string]string, allowedStatusCodes ...int) (responseBody []byte, err error) {

	// create client, in order to add headers
	client := pester.NewExtendedClient(&http.Client{Transport: &nethttp.Transport{}})
	client.MaxRetries = 3
	client.Backoff = pester.ExponentialJitterBackoff
	client.KeepLog = true
	client.Timeout = time.Second * 10

	request, err := http.NewRequest(method, uri, requestBody)
	if err != nil {
		return nil, err
	}

	// add tracing context
	request = request.WithContext(opentracing.ContextWithSpan(request.Context(), span))

	// collect additional information on setting up connections
	request, ht := nethttp.TraceRequest(span.Tracer(), request)

	// add headers
	for k, v := range headers {
		request.Header.Add(k, v)
	}

	// perform actual request
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	ht.Finish()

	if len(allowedStatusCodes) == 0 {
		allowedStatusCodes = []int{http.StatusOK}
	}

	if !foundation.IntArrayContains(allowedStatusCodes, response.StatusCode) {
		return nil, fmt.Errorf("%v responded with status code %v", uri, response.StatusCode)
	}

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return
	}

	return body, nil
}

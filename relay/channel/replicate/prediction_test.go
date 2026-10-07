package replicate

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/constant"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	relayconstant "github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/dqt081228OHVGFY/further-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func replicateTestContext(t *testing.T, base string) (*gin.Context, *httptest.ResponseRecorder, *relaycommon.RelayInfo) {
	t.Helper()
	service.InitHttpClient()
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: base, ApiKey: "fixture-key", ChannelType: constant.ChannelTypeReplicate}, Request: &dto.ImageRequest{ResponseFormat: "url"}}
	info.PriceData.UsePrice = true
	info.PriceData.AddOtherRatio("n", 5)
	return c, writer, info
}

func TestReplicateNativePredictionCreatePollsToSuccessAndUsesActualImageCount(t *testing.T) {
	var polls atomic.Int32
	var upstream *httptest.Server
	upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer fixture-key", r.Header.Get("Authorization"))
		require.Equal(t, "application/json", r.Header.Get("Accept"))
		if r.Method == http.MethodPost {
			require.Equal(t, "/v1/models/"+ModelFlux11Pro+"/predictions", r.URL.Path)
			var input map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&input))
			require.Equal(t, "a lighthouse", input["input"].(map[string]any)["prompt"])
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"id":"prediction_1","status":"starting","urls":{"get":"` + upstream.URL + `/v1/predictions/prediction_1"}}`))
			return
		}
		require.Equal(t, "/v1/predictions/prediction_1", r.URL.Path)
		if polls.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"id":"prediction_1","status":"processing"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"prediction_1","status":"succeeded","output":["https://images.example/one.png","https://images.example/two.png"]}`))
	}))
	defer upstream.Close()
	c, writer, info := replicateTestContext(t, upstream.URL)
	adaptor := &Adaptor{}
	payload, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{Prompt: "a lighthouse", Model: ModelFlux11Pro})
	require.NoError(t, err)
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	result, err := adaptor.DoRequest(c, info, bytes.NewReader(raw))
	require.NoError(t, err)
	usage, failure := adaptor.DoResponse(c, result.(*http.Response), info)
	require.Nil(t, failure)
	require.NotNil(t, usage)
	require.EqualValues(t, 2, polls.Load())
	var images dto.ImageResponse
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), &images))
	require.Len(t, images.Data, 2)
	require.Equal(t, float64(2), info.PriceData.OtherRatios()["n"])
}

func TestReplicatePredictionCancellationStopsPendingPolling(t *testing.T) {
	var polls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		polls.Add(1)
		_, _ = w.Write([]byte(`{"id":"cancel","status":"processing"}`))
	}))
	defer upstream.Close()
	c, writer, info := replicateTestContext(t, upstream.URL)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 25*time.Millisecond)
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	response := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"id":"cancel","status":"starting","urls":{"get":"` + upstream.URL + `/v1/predictions/cancel"}}`))}
	_, failure := finishReplicateImage(c, response, info)
	require.NotNil(t, failure)
	require.Empty(t, writer.Body.String())
	require.Zero(t, polls.Load())
}

func TestReplicateDoesNotSendAPICredentialToForeignPollOrRedirect(t *testing.T) {
	var foreignCalls atomic.Int32
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { foreignCalls.Add(1) }))
	defer foreign.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, foreign.URL, http.StatusTemporaryRedirect)
	}))
	defer api.Close()
	c, writer, info := replicateTestContext(t, api.URL)
	response := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"id":"foreign","status":"starting","urls":{"get":"` + foreign.URL + `/poll"}}`))}
	_, failure := finishReplicateImage(c, response, info)
	require.NotNil(t, failure)
	require.Empty(t, writer.Body.String())
	require.Zero(t, foreignCalls.Load())
	response, err := replicateAuthenticatedHTTP(context.Background(), info, http.MethodPost, api.URL+"/v1/files", "application/json", strings.NewReader("{}"))
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusTemporaryRedirect, response.StatusCode)
	require.Zero(t, foreignCalls.Load())
}

func TestReplicateMultipartUploadRebuildsOnlyImageField(t *testing.T) {
	var upstream *httptest.Server
	upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/files", r.URL.Path)
		require.Equal(t, "Bearer fixture-key", r.Header.Get("Authorization"))
		require.NoError(t, r.ParseMultipartForm(1<<20))
		defer r.MultipartForm.RemoveAll()
		require.Len(t, r.MultipartForm.File, 1)
		file, header, err := r.FormFile("content")
		require.NoError(t, err)
		defer file.Close()
		require.Equal(t, `quoted"image.png`, header.Filename)
		raw, err := io.ReadAll(file)
		require.NoError(t, err)
		require.Equal(t, "fixture-image", string(raw))
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"urls":{"get":"` + upstream.URL + `/v1/files/file_1"}}`))
	}))
	defer upstream.Close()
	c, _, info := replicateTestContext(t, upstream.URL)
	info.RelayMode = relayconstant.RelayModeImagesEdits
	var wire bytes.Buffer
	form := multipart.NewWriter(&wire)
	require.NoError(t, form.WriteField("prompt", "edited"))
	file, err := form.CreateFormFile("image", `quoted"image.png`)
	require.NoError(t, err)
	_, _ = file.Write([]byte("fixture-image"))
	require.NoError(t, form.Close())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &wire)
	c.Request.Header.Set("Content-Type", form.FormDataContentType())
	result, err := prepareReplicateImage(c, info, dto.ImageRequest{Prompt: "edited"})
	require.NoError(t, err)
	defer c.Request.MultipartForm.RemoveAll()
	require.Equal(t, upstream.URL+"/v1/files/file_1", result.(map[string]any)["input"].(map[string]any)["image_prompt"])
}

func TestReplicateBase64DownloadUsesProtectedTransportWithoutAPIKey(t *testing.T) {
	previous := *system_setting.GetFetchSetting()
	t.Cleanup(func() { *system_setting.GetFetchSetting() = previous })
	system_setting.GetFetchSetting().EnableSSRFProtection = true
	system_setting.GetFetchSetting().AllowPrivateIp = true
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Empty(t, r.Header.Get("Authorization"))
		_, _ = w.Write([]byte("fixture-image"))
	}))
	defer upstream.Close()
	downloadURL, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	system_setting.GetFetchSetting().AllowedPorts = []string{downloadURL.Port()}
	c, writer, info := replicateTestContext(t, upstream.URL)
	info.Request = &dto.ImageRequest{ResponseFormat: "b64_json"}
	response := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"status":"succeeded","output":"` + upstream.URL + `/image.png"}`))}
	_, failure := finishReplicateImage(c, response, info)
	require.Nil(t, failure)
	var images dto.ImageResponse
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), &images))
	require.Len(t, images.Data, 1)
	require.Equal(t, base64.StdEncoding.EncodeToString([]byte("fixture-image")), images.Data[0].B64Json)
}

type replicateFailWriter struct{ header http.Header }

func (w *replicateFailWriter) Header() http.Header { return w.header }
func (w *replicateFailWriter) WriteHeader(int)     {}
func (w *replicateFailWriter) Write([]byte) (int, error) {
	return 0, errors.New("fixture downstream failure")
}

func TestReplicateReturnsWriteFailureAndDecodesProviderStringError(t *testing.T) {
	c, _, info := replicateTestContext(t, "https://api.replicate.example")
	writer := &replicateFailWriter{header: http.Header{}}
	c, _ = gin.CreateTestContext(writer)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	response := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"status":"succeeded","output":"https://images.example/one.png"}`))}
	_, failure := finishReplicateImage(c, response, info)
	require.NotNil(t, failure)
	prediction, err := decodeReplicatePrediction([]byte(`{"status":"failed","error":"provider failed"}`))
	require.NoError(t, err)
	require.Equal(t, "provider failed", prediction.Error.Message)
}

package openai

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestImageUploadKeepsMixedArrayFieldsInNumericOrder(t *testing.T) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	require.NoError(t, writer.WriteField("model", "public-model"))
	for _, item := range []struct{ field, name, body string }{{"image[10]", "tenth.png", "ten"}, {"image[2]", "second.png", "two"}, {"image", "primary.png", "main"}, {"mask", "mask.png", "mask"}} {
		part, err := writer.CreateFormFile(item.field, item.name)
		require.NoError(t, err)
		_, err = part.Write([]byte(item.body))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	converted, err := ImagePayload(c, &relaycommon.RelayInfo{RelayMode: constant.RelayModeImagesEdits}, dto.ImageRequest{Model: "mapped-model"})
	require.NoError(t, err)
	replayed := httptest.NewRequest(http.MethodPost, "/v1/images/edits", converted.(*bytes.Buffer))
	replayed.Header.Set("Content-Type", c.Request.Header.Get("Content-Type"))
	require.NoError(t, replayed.ParseMultipartForm(1<<20))
	require.Equal(t, "mapped-model", replayed.PostForm.Get("model"))
	images := replayed.MultipartForm.File["image[]"]
	require.Len(t, images, 3)
	for i, want := range []string{"main", "two", "ten"} {
		file, err := images[i].Open()
		require.NoError(t, err)
		content, err := io.ReadAll(file)
		file.Close()
		require.NoError(t, err)
		require.Equal(t, want, string(content))
	}
	require.Len(t, replayed.MultipartForm.File["mask"], 1)
}

func TestFailedImageUploadKeepsItsOriginalBoundary(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
	c.Request.Header.Set("Content-Type", "multipart/form-data; boundary=original")
	c.Request.MultipartForm = &multipart.Form{Value: map[string][]string{"prompt": {"edit"}}, File: map[string][]*multipart.FileHeader{}}
	_, err := ImagePayload(c, &relaycommon.RelayInfo{RelayMode: constant.RelayModeImagesEdits}, dto.ImageRequest{Model: "gpt-image-1"})
	require.Error(t, err)
	require.Equal(t, "multipart/form-data; boundary=original", c.Request.Header.Get("Content-Type"))
}

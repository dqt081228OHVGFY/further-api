package openai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/common"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

type uploadFile struct {
	field string
	file  *multipart.FileHeader
	image bool
}

func multipartSource(c *gin.Context) (*multipart.Form, error) {
	if c == nil || c.Request == nil {
		return nil, errors.New("upload request context is missing")
	}
	if c.Request.MultipartForm != nil {
		return c.Request.MultipartForm, nil
	}
	form, err := common.ParseMultipartFormReusable(c)
	if err != nil {
		return nil, fmt.Errorf("parse upload form: %w", err)
	}
	if form == nil {
		return nil, errors.New("upload form is missing")
	}
	c.Request.MultipartForm = form
	return form, nil
}

func writeUpload(writer *multipart.Writer, item uploadFile) error {
	if item.file == nil {
		return errors.New("upload file is missing")
	}
	source, err := item.file.Open()
	if err != nil {
		return fmt.Errorf("open %s upload: %w", item.field, err)
	}
	defer source.Close()
	filename := filepath.Base(strings.ReplaceAll(item.file.Filename, "\\", "/"))
	headers := make(textproto.MIMEHeader)
	headers.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": item.field, "filename": filename}))
	contentType := item.file.Header.Get("Content-Type")
	if item.image {
		contentType = detectImageMimeType(filename)
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	headers.Set("Content-Type", contentType)
	part, err := writer.CreatePart(headers)
	if err != nil {
		return fmt.Errorf("create %s upload: %w", item.field, err)
	}
	if _, err = io.Copy(part, source); err != nil {
		return fmt.Errorf("copy %s upload: %w", item.field, err)
	}
	return nil
}

func rebuildUpload(c *gin.Context, form *multipart.Form, model string, files []uploadFile) (*bytes.Buffer, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("model", model); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(form.Value))
	for name := range form.Value {
		if name != "model" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		for _, value := range form.Value[name] {
			if err := writer.WriteField(name, value); err != nil {
				return nil, err
			}
		}
	}
	for _, item := range files {
		if err := writeUpload(writer, item); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("finish upload: %w", err)
	}
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	return body, nil
}

func AudioPayload(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	if info == nil {
		return nil, errors.New("audio operation is missing")
	}
	if info.RelayMode == constant.RelayModeAudioSpeech {
		body, err := json.Marshal(request)
		if err != nil {
			return nil, err
		}
		return bytes.NewReader(body), nil
	}
	if info.RelayMode != constant.RelayModeAudioTranslation && info.RelayMode != constant.RelayModeAudioTranscription {
		return nil, errors.New("OpenAI does not support this audio operation")
	}
	form, err := multipartSource(c)
	if err != nil {
		return nil, err
	}
	if len(form.File["file"]) == 0 {
		return nil, errors.New("audio file is required")
	}
	return rebuildUpload(c, form, request.Model, []uploadFile{{field: "file", file: form.File["file"][0]}})
}

func imageFieldIndex(name string) int {
	if name == "image" {
		return -2
	}
	if name == "image[]" {
		return -1
	}
	index, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(name, "image["), "]"))
	if err != nil {
		return int(^uint(0) >> 1)
	}
	return index
}

func ImagePayload(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	if info == nil {
		return nil, errors.New("image operation is missing")
	}
	if info.RelayMode != constant.RelayModeImagesEdits || isJSONRequest(c) {
		return request, nil
	}
	form, err := multipartSource(c)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(form.File))
	for name := range form.File {
		if name == "image" || strings.HasPrefix(name, "image[") {
			names = append(names, name)
		}
	}
	sort.Slice(names, func(i, j int) bool {
		first, second := imageFieldIndex(names[i]), imageFieldIndex(names[j])
		if first == second {
			return names[i] < names[j]
		}
		return first < second
	})
	images := make([]uploadFile, 0)
	for _, name := range names {
		for _, file := range form.File[name] {
			images = append(images, uploadFile{field: "image", file: file, image: true})
		}
	}
	if len(images) == 0 {
		return nil, errors.New("image is required")
	}
	if len(images) > 1 {
		for i := range images {
			images[i].field = "image[]"
		}
	}
	if mask := form.File["mask"]; len(mask) != 0 {
		images = append(images, uploadFile{field: "mask", file: mask[0], image: true})
	}
	return rebuildUpload(c, form, request.Model, images)
}

func isJSONRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	contentType, _, err := mime.ParseMediaType(c.Request.Header.Get("Content-Type"))
	return err == nil && strings.EqualFold(contentType, "application/json")
}
func detectImageMimeType(filename string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	default:
		return "image/png"
	}
}

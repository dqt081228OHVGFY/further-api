package replicate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path/filepath"
	"strings"

	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/gin-gonic/gin"
)

func uploadReplicateImage(c *gin.Context, info *relaycommon.RelayInfo) (string, error) {
	if c == nil || c.Request == nil {
		return "", errors.New("图片上传请求缺失")
	}
	ctx := c.Request.Context()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	form, err := c.MultipartForm()
	if err != nil {
		return "", fmt.Errorf("图片 multipart 内容无效: %w", err)
	}
	var fileHeader *multipart.FileHeader
	for _, key := range []string{"image", "image[]", "image_prompt"} {
		if len(form.File[key]) > 0 {
			fileHeader = form.File[key][0]
			break
		}
	}
	if fileHeader == nil {
		return "", nil
	}
	if fileHeader.Size > replicateUploadLimit {
		return "", errors.New("上传图片超过允许大小")
	}
	file, err := fileHeader.Open()
	if err != nil {
		return "", err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, replicateUploadLimit+1))
	if err != nil {
		return "", err
	}
	if int64(len(content)) > replicateUploadLimit {
		return "", errors.New("上传图片超过允许大小")
	}
	if len(content) == 0 {
		return "", errors.New("上传图片为空")
	}
	var wire bytes.Buffer
	writer := multipart.NewWriter(&wire)
	name := filepath.Base(fileHeader.Filename)
	if name == "" || name == "." {
		name = "image"
	}
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": "content", "filename": name}))
	contentType := fileHeader.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(content)
	}
	if strings.ContainsAny(contentType, "\r\n") {
		return "", errors.New("图片内容类型无效")
	}
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(content); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	target, err := replicateAPIURL(info, "/v1/files")
	if err != nil {
		return "", err
	}
	response, err := replicateAuthenticatedHTTP(ctx, info, http.MethodPost, target, writer.FormDataContentType(), &wire)
	if err != nil {
		return "", err
	}
	raw, err := readReplicateResponse(ctx, response, replicateJSONLimit)
	if err != nil {
		return "", err
	}
	var uploaded FileUploadResponse
	if err := json.Unmarshal(raw, &uploaded); err != nil {
		return "", fmt.Errorf("Replicate 文件响应格式无效: %w", err)
	}
	if uploaded.Urls.Get == "" {
		return "", errors.New("Replicate 文件响应缺少 URL")
	}
	return approvedReplicatePollURL(info, uploaded.Urls.Get)
}

package zhipu_4v

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"

	"github.com/gin-gonic/gin"
)

type zhipuImageRequest struct {
	Model            string `json:"model"`
	Prompt           string `json:"prompt"`
	Quality          string `json:"quality,omitempty"`
	Size             string `json:"size,omitempty"`
	WatermarkEnabled *bool  `json:"watermark_enabled,omitempty"`
	UserID           string `json:"user_id,omitempty"`
}

type zhipuImageResponse struct {
	Created       *int64            `json:"created,omitempty"`
	Data          []zhipuImageData  `json:"data,omitempty"`
	ContentFilter any               `json:"content_filter,omitempty"`
	Usage         *dto.Usage        `json:"usage,omitempty"`
	Error         *zhipuImageError  `json:"error,omitempty"`
	RequestID     string            `json:"request_id,omitempty"`
	ExtendParam   map[string]string `json:"extendParam,omitempty"`
}

type zhipuImageError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type zhipuImageData struct {
	Url      string `json:"url,omitempty"`
	ImageUrl string `json:"image_url,omitempty"`
	B64Json  string `json:"b64_json,omitempty"`
	B64Image string `json:"b64_image,omitempty"`
}

type openAIImagePayload struct {
	Created int64             `json:"created"`
	Data    []openAIImageData `json:"data"`
}

type openAIImageData struct {
	B64Json string `json:"b64_json"`
}

func zhipu4vImageHandler(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.GatewayError) {
	if c == nil || info == nil || response == nil || response.Body == nil {
		return nil, types.NewError(errors.New("Zhipu image response context is missing"), types.ErrorCodeBadResponseBody)
	}
	defer service.CloseResponseBodyGracefully(response)
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeReadResponseBodyFailed)
	}
	var wire *zhipuImageResponse
	if err = json.Unmarshal(body, &wire); err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	if wire == nil {
		return nil, types.NewError(errors.New("Zhipu image response must be an object"), types.ErrorCodeBadResponseBody)
	}
	if wire.Error != nil && wire.Error.Message != "" {
		return nil, types.WithOpenAIError(types.OpenAIError{Message: wire.Error.Message, Type: "zhipu_image_error", Code: wire.Error.Code}, http.StatusBadGateway)
	}
	output := dto.ImageResponse{Created: info.StartTime.Unix(), Data: make([]dto.ImageData, 0, len(wire.Data))}
	if wire.Created != nil {
		output.Created = *wire.Created
	}
	format := "b64_json"
	if request, ok := info.Request.(*dto.ImageRequest); ok && request.ResponseFormat != "" {
		format = request.ResponseFormat
	}
	for _, native := range wire.Data {
		address := native.Url
		if address == "" {
			address = native.ImageUrl
		}
		encoded := native.B64Json
		if encoded == "" {
			encoded = native.B64Image
		}
		image := dto.ImageData{}
		if format == "url" && address != "" {
			image.Url = address
		} else {
			if encoded == "" && address != "" {
				encoded, _, err = service.GetBase64Data(c, types.NewURLFileSource(address), "Zhipu generated image")
				if err != nil {
					return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
				}
			}
			image.B64Json = encoded
		}
		if image.Url != "" || image.B64Json != "" {
			output.Data = append(output.Data, image)
		}
	}
	if len(output.Data) == 0 {
		return nil, types.NewError(errors.New("Zhipu returned no generated images"), types.ErrorCodeEmptyResponse)
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	info.UpdateImageCount(int64(len(output.Data)))
	service.IOCopyBytesGracefully(c, response, encoded)
	if wire.Usage != nil {
		return wire.Usage, nil
	}
	return &dto.Usage{}, nil
}

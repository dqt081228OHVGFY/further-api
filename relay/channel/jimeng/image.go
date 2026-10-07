package jimeng

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"

	"github.com/gin-gonic/gin"
)

type ImageResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		BinaryDataBase64 []string `json:"binary_data_base64"`
		ImageUrls        []string `json:"image_urls"`
		RephraseResult   string   `json:"rephraser_result"`
		RequestID        string   `json:"request_id"`
		// Other fields are omitted for brevity
	} `json:"data"`
	RequestID   string `json:"request_id"`
	Status      int    `json:"status"`
	TimeElapsed string `json:"time_elapsed"`
}

func responseJimeng2OpenAIImage(_ *gin.Context, response *ImageResponse, info *relaycommon.RelayInfo) *dto.ImageResponse {
	output := &dto.ImageResponse{Created: info.StartTime.Unix(), Data: make([]dto.ImageData, 0, max(len(response.Data.BinaryDataBase64), len(response.Data.ImageUrls)))}
	for index := 0; index < max(len(response.Data.BinaryDataBase64), len(response.Data.ImageUrls)); index++ {
		image := dto.ImageData{}
		if index < len(response.Data.BinaryDataBase64) {
			image.B64Json = response.Data.BinaryDataBase64[index]
		}
		if index < len(response.Data.ImageUrls) {
			image.Url = response.Data.ImageUrls[index]
		}
		if image.B64Json != "" || image.Url != "" {
			output.Data = append(output.Data, image)
		}
	}
	return output
}
func jimengImageHandler(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.GatewayError) {
	if c == nil || info == nil || response == nil || response.Body == nil {
		return nil, types.NewError(errors.New("Jimeng image response context is missing"), types.ErrorCodeBadResponseBody)
	}
	defer service.CloseResponseBodyGracefully(response)
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeReadResponseBodyFailed)
	}
	var wire *ImageResponse
	if err = json.Unmarshal(data, &wire); err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	if wire == nil {
		return nil, types.NewError(errors.New("Jimeng image response must be an object"), types.ErrorCodeBadResponseBody)
	}
	if wire.Code != 10000 {
		return nil, types.WithOpenAIError(types.OpenAIError{Message: wire.Message, Type: "jimeng_error", Code: fmt.Sprint(wire.Code)}, http.StatusBadGateway)
	}
	output := responseJimeng2OpenAIImage(c, wire, info)
	if len(output.Data) == 0 {
		return nil, types.NewError(errors.New("Jimeng returned no generated images"), types.ErrorCodeEmptyResponse)
	}
	body, err := json.Marshal(output)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	info.UpdateImageCount(int64(len(output.Data)))
	service.IOCopyBytesGracefully(c, response, body)
	return &dto.Usage{}, nil
}

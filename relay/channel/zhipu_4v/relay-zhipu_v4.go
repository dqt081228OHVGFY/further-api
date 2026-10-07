package zhipu_4v

import (
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"strings"
)

func requestOpenAI2Zhipu(request dto.GeneralOpenAIRequest) *dto.GeneralOpenAIRequest {
	output := request
	output.Messages = make([]dto.Message, len(request.Messages))
	for index, source := range request.Messages {
		message := source
		if !source.IsStringContent() {
			parts := append([]dto.MediaContent(nil), source.ParseContent()...)
			for partIndex := range parts {
				if parts[partIndex].Type != dto.ContentTypeImageURL {
					continue
				}
				image := parts[partIndex].GetImageMedia()
				if image == nil {
					continue
				}
				copy := *image
				if strings.HasPrefix(copy.Url, "data:image/") {
					if _, data, found := strings.Cut(copy.Url, ","); found {
						copy.Url = data
					}
				}
				parts[partIndex].ImageUrl = &copy
			}
			message.SetMediaContent(parts)
		}
		output.Messages[index] = message
	}
	switch stop := request.Stop.(type) {
	case string:
		output.Stop = []string{stop}
	case []any:
		values := make([]string, 0, len(stop))
		for _, item := range stop {
			if value, ok := item.(string); ok {
				values = append(values, value)
			}
		}
		output.Stop = values
	}
	if request.MaxTokens != nil || request.MaxCompletionTokens != nil {
		limit := request.GetMaxTokens()
		output.MaxTokens = &limit
		output.MaxCompletionTokens = nil
	}
	return &output
}

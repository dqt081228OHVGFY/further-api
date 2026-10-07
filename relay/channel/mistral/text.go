package mistral

import (
	"crypto/sha256"
	"encoding/hex"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"regexp"
)

var mistralToolCallIdRegexp = regexp.MustCompile(`^[a-zA-Z0-9]{9}$`)

func requestOpenAI2Mistral(request *dto.GeneralOpenAIRequest) *dto.GeneralOpenAIRequest {
	if request == nil {
		return nil
	}
	output := *request
	output.Messages = make([]dto.Message, len(request.Messages))
	names := make(map[string]string)
	idFor := func(id string) string {
		if mistralToolCallIdRegexp.MatchString(id) {
			return id
		}
		if mapped, ok := names[id]; ok {
			return mapped
		}
		digest := sha256.Sum256([]byte(id))
		mapped := hex.EncodeToString(digest[:])[:9]
		names[id] = mapped
		return mapped
	}
	for index, source := range request.Messages {
		message := source
		calls := source.ParseToolCalls()
		for i := range calls {
			calls[i].ID = idFor(calls[i].ID)
		}
		if len(calls) > 0 {
			message.SetToolCalls(calls)
		}
		if message.ToolCallId != "" {
			message.ToolCallId = idFor(message.ToolCallId)
		}
		parts := append([]dto.MediaContent(nil), source.ParseContent()...)
		for i := range parts {
			if parts[i].Type == dto.ContentTypeImageURL {
				if image := parts[i].GetImageMedia(); image != nil {
					parts[i].ImageUrl = image.Url
				}
			}
		}
		if source.Role == "assistant" && len(source.ToolCalls) > 0 && source.StringContent() == "" {
			parts = []dto.MediaContent{}
		}
		message.SetMediaContent(parts)
		output.Messages[index] = message
	}
	if request.MaxTokens != nil || request.MaxCompletionTokens != nil {
		limit := request.GetMaxTokens()
		output.MaxTokens = &limit
		output.MaxCompletionTokens = nil
	}
	return &output
}

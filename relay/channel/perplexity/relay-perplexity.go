package perplexity

import "github.com/dqt081228OHVGFY/further-api/relaykit/dto"

func requestOpenAI2Perplexity(request dto.GeneralOpenAIRequest) *dto.GeneralOpenAIRequest {
	output := request
	output.Messages = make([]dto.Message, len(request.Messages))
	for index, message := range request.Messages {
		output.Messages[index] = dto.Message{Role: message.Role, Content: message.Content}
	}
	if request.MaxTokens != nil || request.MaxCompletionTokens != nil {
		limit := request.GetMaxTokens()
		output.MaxTokens = &limit
		output.MaxCompletionTokens = nil
	}
	return &output
}

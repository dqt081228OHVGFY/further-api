package ali

import "github.com/dqt081228OHVGFY/further-api/relaykit/dto"

const EnableSearchModelSuffix = "-internet"

func requestOpenAI2Ali(request dto.GeneralOpenAIRequest, mappedModel string) *dto.GeneralOpenAIRequest {
	if mappedModel == "" {
		mappedModel = request.Model
	}
	if !dto.IsQwenThinkingBudgetModel(mappedModel) {
		request.ThinkingBudget = nil
	}
	if request.TopP != nil {
		value := *request.TopP
		if value <= 0 {
			value = 0.01
		}
		if value >= 1 {
			value = 0.99
		}
		request.TopP = &value
	}
	return &request
}

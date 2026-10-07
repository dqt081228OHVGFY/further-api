package replicate

import "encoding/json"

type PredictionResponse struct {
	ID     string           `json:"id"`
	Status string           `json:"status"`
	Output any              `json:"output"`
	Error  *PredictionError `json:"error"`
	Urls   struct {
		Get    string `json:"get"`
		Cancel string `json:"cancel"`
	} `json:"urls"`
}

func (failure *PredictionError) UnmarshalJSON(raw []byte) error {
	var message string
	if json.Unmarshal(raw, &message) == nil {
		failure.Message = message
		return nil
	}
	type fields PredictionError
	var decoded fields
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	*failure = PredictionError(decoded)
	return nil
}

type PredictionError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"detail"`
}

type FileUploadResponse struct {
	Urls struct {
		Get string `json:"get"`
	} `json:"urls"`
}

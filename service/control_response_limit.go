package service

import (
	"errors"
	"io"
	"net/http"

	"github.com/dqt081228OHVGFY/further-api/common"
)

func decodeBoundedControlResponse(response *http.Response, destination any) error {
	const maximum = 1 << 20
	if response == nil || response.Body == nil {
		return errors.New("upstream control response missing")
	}
	if response.ContentLength > maximum {
		return errors.New("upstream control response exceeds size limit")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maximum+1))
	if err != nil {
		return errors.New("upstream control response read failed")
	}
	if len(data) > maximum {
		return errors.New("upstream control response exceeds size limit")
	}
	if common.Unmarshal(data, destination) != nil {
		return errors.New("upstream control response is invalid JSON")
	}
	return nil
}

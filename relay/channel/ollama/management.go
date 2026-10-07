package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/relay/channel/openai"
	"github.com/dqt081228OHVGFY/further-api/service"
)

func ollamaManagementRequest(ctx context.Context, base, key, method, route string, payload any, timeout time.Duration) (*http.Response, error) {
	if ctx == nil {
		ctx = common.BackgroundContext()
	}
	endpoint, err := openai.ProviderEndpoint(base, route)
	if err != nil {
		return nil, err
	}
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, err
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		request.Header.Set("Authorization", "Bearer "+key)
	}
	shared := service.GetHttpClient()
	client := *shared
	client.Timeout = timeout
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("Ollama %s failed (HTTP %d)", route, response.StatusCode)
	}
	return response, nil
}
func FetchOllamaModels(base, key string) ([]OllamaModel, error) {
	return FetchOllamaModelsContext(common.BackgroundContext(), base, key)
}
func FetchOllamaModelsContext(ctx context.Context, base, key string) ([]OllamaModel, error) {
	response, err := ollamaManagementRequest(ctx, base, key, http.MethodGet, "/api/tags", nil, 30*time.Second)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	var wire OllamaTagsResponse
	if err = json.NewDecoder(io.LimitReader(response.Body, 16<<20)).Decode(&wire); err != nil {
		return nil, err
	}
	return wire.Models, nil
}
func PullOllamaModel(base, key, model string) error {
	return PullOllamaModelContext(common.BackgroundContext(), base, key, model)
}
func PullOllamaModelContext(ctx context.Context, base, key, model string) error {
	response, err := ollamaManagementRequest(ctx, base, key, http.MethodPost, "/api/pull", OllamaPullRequest{Name: model, Stream: false}, 30*time.Minute)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	var wire struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	if err = json.NewDecoder(response.Body).Decode(&wire); err != nil {
		return err
	}
	if wire.Error != "" {
		return errors.New("Ollama model pull failed: " + wire.Error)
	}
	if !strings.EqualFold(wire.Status, "success") {
		return errors.New("Ollama model pull did not finish")
	}
	return nil
}
func PullOllamaModelStream(base, key, model string, progress func(OllamaPullResponse)) error {
	return PullOllamaModelStreamContext(common.BackgroundContext(), base, key, model, progress)
}
func PullOllamaModelStreamContext(ctx context.Context, base, key, model string, progress func(OllamaPullResponse)) error {
	if ctx == nil {
		ctx = common.BackgroundContext()
	}
	response, err := ollamaManagementRequest(ctx, base, key, http.MethodPost, "/api/pull", OllamaPullRequest{Name: model, Stream: true}, time.Hour)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	decoder := json.NewDecoder(response.Body)
	for {
		if err = ctx.Err(); err != nil {
			return err
		}
		var record json.RawMessage
		if err = decoder.Decode(&record); err == io.EOF {
			return errors.New("Ollama model pull ended without success")
		} else if err != nil {
			return err
		}
		var status struct {
			Error string `json:"error"`
		}
		if err = json.Unmarshal(record, &status); err != nil {
			return err
		}
		if status.Error != "" {
			return errors.New("Ollama model pull failed: " + status.Error)
		}
		var wire OllamaPullResponse
		if err = json.Unmarshal(record, &wire); err != nil {
			return err
		}
		if progress != nil {
			progress(wire)
		}
		if strings.EqualFold(wire.Status, "error") {
			return errors.New("Ollama model pull returned an error status")
		}
		if strings.EqualFold(wire.Status, "success") {
			return nil
		}
	}
}
func DeleteOllamaModel(base, key, model string) error {
	return DeleteOllamaModelContext(common.BackgroundContext(), base, key, model)
}
func DeleteOllamaModelContext(ctx context.Context, base, key, model string) error {
	response, err := ollamaManagementRequest(ctx, base, key, http.MethodDelete, "/api/delete", OllamaDeleteRequest{Name: model}, 30*time.Second)
	if err != nil {
		return err
	}
	response.Body.Close()
	return nil
}
func FetchOllamaVersion(base, key string) (string, error) {
	return FetchOllamaVersionContext(common.BackgroundContext(), base, key)
}
func FetchOllamaVersionContext(ctx context.Context, base, key string) (string, error) {
	response, err := ollamaManagementRequest(ctx, base, key, http.MethodGet, "/api/version", nil, 10*time.Second)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	var wire struct {
		Version string `json:"version"`
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&wire); err != nil {
		return "", err
	}
	if strings.TrimSpace(wire.Version) == "" {
		return "", errors.New("Ollama version response is empty")
	}
	return wire.Version, nil
}

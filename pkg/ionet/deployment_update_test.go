package ionet

import (
	"encoding/json"
	"testing"
)

type captureDeploymentUpdate struct {
	body map[string]json.RawMessage
}

func (capture *captureDeploymentUpdate) Do(request *HTTPRequest) (*HTTPResponse, error) {
	if err := json.Unmarshal(request.Body, &capture.body); err != nil {
		return nil, err
	}
	return &HTTPResponse{StatusCode: 200, Body: []byte(`{"status":"updated"}`)}, nil
}

func TestUpdateDeploymentDistinguishesClearedAndOmittedEnvironment(t *testing.T) {
	for _, test := range []struct {
		name            string
		request         UpdateDeploymentRequest
		wantEnvironment bool
		wantSecret      bool
	}{
		{name: "leave both unchanged", request: UpdateDeploymentRequest{ImageURL: "registry.example/app:v2"}},
		{name: "clear last variable", request: UpdateDeploymentRequest{EnvVariables: map[string]string{}}, wantEnvironment: true},
		{name: "clear last secret", request: UpdateDeploymentRequest{SecretEnvVariables: map[string]string{}}, wantSecret: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			capture := &captureDeploymentUpdate{}
			client := NewClientWithConfig("test-token", "https://deployment.example", capture)
			if _, err := client.UpdateDeployment("deployment-1", &test.request); err != nil {
				t.Fatal(err)
			}
			for key, want := range map[string]bool{"env_variables": test.wantEnvironment, "secret_env_variables": test.wantSecret} {
				value, present := capture.body[key]
				if present != want {
					t.Fatalf("%s presence = %v, want %v", key, present, want)
				}
				if present && string(value) != "{}" {
					t.Fatalf("%s = %s, want empty object", key, value)
				}
			}
		})
	}
}

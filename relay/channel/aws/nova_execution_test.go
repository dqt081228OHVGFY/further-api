package aws

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/stretchr/testify/require"
)

func TestNovaRequestSeparatesSystemMessagesAndPreservesZeroSampling(t *testing.T) {
	zero := 0.0
	request, err := novaRequest(nil, &dto.GeneralOpenAIRequest{Temperature: &zero, TopP: &zero, Messages: []dto.Message{{Role: "system", Content: "rules"}, {Role: "user", Content: "one"}, {Role: "user", Content: "two"}}})
	require.NoError(t, err)
	require.Len(t, request.System, 1)
	require.Equal(t, "rules", request.System[0].Text)
	require.Len(t, request.Messages, 1)
	require.Len(t, request.Messages[0].Content, 2)
	require.NotNil(t, request.InferenceConfig)
	require.Equal(t, &zero, request.InferenceConfig.Temperature)
	require.Equal(t, &zero, request.InferenceConfig.TopP)
}

func TestNovaExecutionJoinsAllBlocksAndRejectsEmptyContent(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(map[bool]string{false: "multiple blocks", true: "empty content"}[empty], func(t *testing.T) {
			content := `[{"text":"one"},{"text":"two"}]`
			if empty {
				content = `[]`
			}
			client := newAwsTestClient(awsHTTPClientFunc(func(request *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"output":{"message":{"content":` + content + `}},"usage":{"inputTokens":10,"outputTokens":2}}`)), Request: request}, nil
			}))
			writer := httptest.NewRecorder()
			c := newAwsTestContext(writer, context.Background())
			failure, usage := handleNovaRequest(c, newAwsTestRelayInfo(), &Adaptor{AwsClient: client, AwsReq: newAwsInvokeModelInput()})
			if empty {
				require.NotNil(t, failure)
				require.Nil(t, usage)
				require.Empty(t, writer.Body.String())
			} else {
				require.Nil(t, failure)
				require.Equal(t, 12, usage.TotalTokens)
				require.Contains(t, writer.Body.String(), `"content":"onetwo"`)
			}
		})
	}
}

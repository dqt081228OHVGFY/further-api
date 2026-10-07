package relay

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/relay/channel"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type fakeProtocolAdaptor struct {
	channel.Adaptor
	request  func() (any, error)
	response func(*http.Response) (any, *types.GatewayError)
}

func (fake *fakeProtocolAdaptor) DoRequest(*gin.Context, *relaycommon.RelayInfo, io.Reader) (any, error) {
	return fake.request()
}
func (fake *fakeProtocolAdaptor) DoResponse(_ *gin.Context, response *http.Response, _ *relaycommon.RelayInfo) (any, *types.GatewayError) {
	return fake.response(response)
}

type countedProtocolBody struct {
	io.Reader
	closes int
}

func (body *countedProtocolBody) Close() error { body.closes++; return nil }

func TestProtocolAttemptRejectsCanceledRequestWithoutProviderCall(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	called := false
	attempt := &protocolAttempt{ctx: c, info: &relaycommon.RelayInfo{}, adaptor: &fakeProtocolAdaptor{request: func() (any, error) { called = true; return nil, nil }}}
	_, err := attempt.perform(nil)
	require.ErrorIs(t, err, context.Canceled)
	require.False(t, called)
}

func TestProtocolAttemptClosesResponseOnceOnDecoderFailure(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	body := &countedProtocolBody{Reader: strings.NewReader(`{}`)}
	adaptor := &fakeProtocolAdaptor{
		request: func() (any, error) { return &http.Response{StatusCode: 200, Body: body}, nil },
		response: func(response *http.Response) (any, *types.GatewayError) {
			_ = response.Body.Close()
			return nil, types.NewError(errors.New("decoder failed"), types.ErrorCodeBadResponse)
		},
	}
	attempt := &protocolAttempt{ctx: c, info: &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}, adaptor: adaptor}
	_, err := attempt.perform(nil)
	require.Error(t, err)
	require.Equal(t, 1, body.closes)
}

func TestProtocolAttemptSupportsSDKResponseAndRejectsInvalidUsage(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		fake := &fakeProtocolAdaptor{
			request: func() (any, error) { return nil, nil },
			response: func(response *http.Response) (any, *types.GatewayError) {
				require.Nil(t, response)
				if invalid {
					return "invalid usage", nil
				}
				return &dto.Usage{PromptTokens: 3, CompletionTokens: 2}, nil
			},
		}
		attempt := &protocolAttempt{ctx: c, info: &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ApiType: constant.APITypeAws}}, adaptor: fake}
		usage, err := attempt.perform(nil)
		if invalid {
			require.Error(t, err)
			require.True(t, types.IsSkipRetryError(err))
		} else {
			require.Nil(t, err)
			require.Equal(t, 3, usage.PromptTokens)
		}
	}
}

func TestProtocolClonePreservesRuntimeFieldsAndDetachesMutableMetadata(t *testing.T) {
	original := &dto.AlphaSearchRequest{Model: "model", RawBody: json.RawMessage(`{"model":"model","nonce":9007199254740993}`)}
	cloned, err := cloneProtocolDTO(original)
	require.NoError(t, err)
	require.Equal(t, original.RawBody, cloned.RawBody)
	cloned.RawBody[0] = '['
	require.Equal(t, byte('{'), original.RawBody[0])
	type runtimeRequest struct {
		Fields   map[string]any
		Metadata []byte `json:"-"`
	}
	source := &runtimeRequest{Fields: map[string]any{"inner": []any{map[string]any{"value": "unchanged"}}}, Metadata: []byte("metadata")}
	copy, err := cloneProtocolDTO(source)
	require.NoError(t, err)
	copy.Fields["inner"].([]any)[0].(map[string]any)["value"] = "changed"
	copy.Metadata[0] = 'X'
	require.Equal(t, "unchanged", source.Fields["inner"].([]any)[0].(map[string]any)["value"])
	require.Equal(t, "metadata", string(source.Metadata))
}

func TestAlphaSearchModelMappingPreservesLargeIntegersAndUnknownFields(t *testing.T) {
	input := []byte(`{"model":"local","nonce":9007199254740993,"custom":{"keep":true}}`)
	body, err := buildAlphaSearchRequestBody(input, "local", "upstream")
	require.NoError(t, err)
	require.Contains(t, string(body), "9007199254740993")
	require.Contains(t, string(body), `"custom":{"keep":true}`)
	require.Contains(t, string(body), `"model":"upstream"`)
}

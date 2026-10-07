package aws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBedrockIdentitySupportsBearerAndSigV4WithoutMixingSecrets(t *testing.T) {
	bearerIdentity, err := bedrockIdentityFrom("provider-token|us-west-2")
	require.NoError(t, err)
	bearerOptions := bedrockOptions(bearerIdentity, &http.Client{})
	token, err := bearerOptions.BearerAuthTokenProvider.RetrieveBearerToken(context.Background())
	require.NoError(t, err)
	require.Equal(t, "provider-token", token.Value)
	require.Nil(t, bearerOptions.Credentials)
	identity, err := bedrockIdentityFrom("access|secret|eu-west-1")
	require.NoError(t, err)
	options := bedrockOptions(identity, &http.Client{})
	keys, err := options.Credentials.Retrieve(context.Background())
	require.NoError(t, err)
	require.Equal(t, "access", keys.AccessKeyID)
	require.Equal(t, "secret", keys.SecretAccessKey)
	require.Nil(t, options.BearerAuthTokenProvider)
	for _, key := range []string{"token", "|us-west-2", "token|", "access||us-west-2", "token|us-west-2/injected"} {
		_, err = bedrockIdentityFrom(key)
		require.Error(t, err)
	}
}

func TestBedrockEndpointUsesRegionInHostAndModelInPath(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ApiKey: "provider-token|us-west-2", UpstreamModelName: awsTestModel}, IsStream: true}
	endpoint, err := bedrockInvokeEndpoint(info)
	require.NoError(t, err)
	require.Equal(t, "https://bedrock-runtime.us-west-2.amazonaws.com/model/"+awsTestModel+"/invoke-with-response-stream", endpoint)
}

func TestBearerBedrockAttemptPreparesNativeSDKStateWithoutSendingTwice(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ApiKey: "provider-token|us-west-2", UpstreamModelName: awsTestModel}}
	a := &Adaptor{}
	a.Init(info)
	response, err := a.DoRequest(c, info, strings.NewReader(`{"max_tokens":8,"messages":[{"role":"user","content":"hello"}]}`))
	require.NoError(t, err)
	require.Nil(t, response)
	require.Equal(t, ClientModeApiKey, a.ClientMode)
	require.NotNil(t, a.AwsClient)
	prepared, ok := a.AwsReq.(*bedrockruntime.InvokeModelInput)
	require.True(t, ok)
	require.NotEmpty(t, prepared.Body)
	require.Equal(t, "us-west-2", a.AwsClient.Options().Region)
	token, err := a.AwsClient.Options().BearerAuthTokenProvider.RetrieveBearerToken(context.Background())
	require.NoError(t, err)
	require.Equal(t, "provider-token", token.Value)
	require.Equal(t, "application/json", *prepared.ContentType)
	require.NotContains(t, string(prepared.Body), "provider-token")
}

package aws

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	sdkaws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/smithy-go/auth/bearer"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/service"
)

type bedrockIdentity struct{ region, bearerToken, accessKey, secretKey string }

func bedrockIdentityFrom(key string) (bedrockIdentity, error) {
	parts := strings.Split(key, "|")
	var identity bedrockIdentity
	switch len(parts) {
	case 2:
		identity.bearerToken, identity.region = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	case 3:
		identity.accessKey, identity.secretKey, identity.region = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2])
	default:
		return identity, errors.New("Bedrock credentials require token|region or access-key|secret-key|region")
	}
	if identity.region == "" || (identity.bearerToken == "" && (identity.accessKey == "" || identity.secretKey == "")) {
		return bedrockIdentity{}, errors.New("Bedrock credentials and region cannot be empty")
	}
	for _, c := range identity.region {
		if c != '-' && (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return bedrockIdentity{}, errors.New("Bedrock region contains invalid characters")
		}
	}
	return identity, nil
}

func bedrockOptions(identity bedrockIdentity, client *http.Client) bedrockruntime.Options {
	options := bedrockruntime.Options{Region: identity.region, HTTPClient: client, Retryer: sdkaws.NopRetryer{}}
	if identity.bearerToken != "" {
		options.BearerAuthTokenProvider = bearer.StaticTokenProvider{Token: bearer.Token{Value: identity.bearerToken}}
	} else {
		options.Credentials = sdkaws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(identity.accessKey, identity.secretKey, ""))
	}
	return options
}

func bedrockClient(info *relaycommon.RelayInfo) (*bedrockruntime.Client, error) {
	if info == nil || !info.HasChannelMeta() {
		return nil, errors.New("Bedrock provider configuration is missing")
	}
	identity, err := bedrockIdentityFrom(info.ApiKey)
	if err != nil {
		return nil, err
	}
	client, err := service.GetHttpClientWithProxySettings(info.ChannelSetting.Proxy, info.ChannelSetting)
	if err != nil {
		return nil, err
	}
	options := bedrockOptions(identity, client)
	if info.ChannelBaseUrl != "" {
		endpoint, err := url.Parse(info.ChannelBaseUrl)
		if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
			return nil, errors.New("Bedrock custom endpoint must be an absolute HTTP(S) URL")
		}
		options.BaseEndpoint = sdkaws.String(strings.TrimRight(endpoint.String(), "/"))
	}
	return bedrockruntime.New(options), nil
}

func bedrockInvokeEndpoint(info *relaycommon.RelayInfo) (string, error) {
	if info == nil || !info.HasChannelMeta() {
		return "", errors.New("Bedrock provider configuration is missing")
	}
	identity, err := bedrockIdentityFrom(info.ApiKey)
	if err != nil {
		return "", err
	}
	model := getAwsModelID(info.UpstreamModelName)
	action := "invoke"
	if info.IsStream {
		action = "invoke-with-response-stream"
	}
	return "https://bedrock-runtime." + identity.region + ".amazonaws.com/model/" + url.PathEscape(model) + "/" + action, nil
}

package openai

import (
	"encoding/json"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/constant"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/stretchr/testify/require"
)

func TestChatPolicyUsesTheMappedModelForReasoningAndCapabilities(t *testing.T) {
	limit := uint(1024)
	temperature := 0.8
	info := &relaycommon.RelayInfo{OriginModelName: "mapped-public-name", ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, UpstreamModelName: "o3-high"}}
	request := &dto.GeneralOpenAIRequest{Model: "o3-high", MaxTokens: &limit, Temperature: &temperature, Messages: []dto.Message{{Role: "system", Content: "instructions"}}}
	converted, err := NativeChat(nil, info, request)
	require.NoError(t, err)
	require.Equal(t, "o3", converted.Model)
	require.Equal(t, "o3", info.UpstreamModelName)
	require.Equal(t, "high", converted.ReasoningEffort)
	require.Equal(t, "high", info.ReasoningEffort)
	require.Nil(t, converted.Temperature)
	require.Nil(t, converted.MaxTokens)
	require.Equal(t, &limit, converted.MaxCompletionTokens)
	require.Equal(t, "developer", converted.Messages[0].Role)
}

func TestRouterPolicyRetainsUnknownFieldsAndProjectsBudget(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenRouter, UpstreamModelName: "anthropic/claude-sonnet"}}
	request := &dto.GeneralOpenAIRequest{Model: "anthropic/claude-sonnet", Reasoning: json.RawMessage(`{"max_tokens":2048,"exclude":true,"vendor_flag":"keep"}`)}
	converted, err := NativeChat(nil, info, request)
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(converted.Reasoning, &fields))
	require.Equal(t, float64(2048), fields["max_tokens"])
	require.Equal(t, "keep", fields["vendor_flag"])
	require.Equal(t, true, fields["exclude"])
	require.Equal(t, true, fields["enabled"])
	require.Empty(t, converted.ReasoningEffort)
	require.JSONEq(t, `{"include":true}`, string(converted.Usage))
}

func TestNativePolicyKeepsResponsesSummaryAndAcceptsMissingHostContext(t *testing.T) {
	request, err := NativeResponses(nil, nil, dto.OpenAIResponsesRequest{Model: "o3-high", Reasoning: &dto.Reasoning{Summary: "detailed"}})
	require.NoError(t, err)
	require.Equal(t, "o3", request.Model)
	require.Equal(t, "high", request.Reasoning.Effort)
	require.Equal(t, "detailed", request.Reasoning.Summary)
	_, err = NativeChat(nil, nil, &dto.GeneralOpenAIRequest{Model: "gpt-4o"})
	require.Error(t, err)
}

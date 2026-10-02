package model

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayForMultiUserStatsUsesIDsExactNamesAndInclusiveDateBoundary(t *testing.T) {
	setupRelayForStatsTest(t)
	private := "private-access-token"
	require.NoError(t, DB.Create(&[]User{
		{Id: 1, Username: "alice", AffCode: "multi-alice", AccessToken: &private},
		{Id: 2, Username: "bob", AffCode: "multi-bob"},
		{Id: 3, Username: "quiet", AffCode: "multi-quiet"},
		{Id: 4, Username: "outside", AffCode: "multi-outside"},
	}).Error)
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.FixedZone("Asia/Shanghai", 28800)).Unix()
	end := start + 86400
	require.NoError(t, LOG_DB.Create(&[]Log{
		{UserId: 1, Username: "previous-name", TokenId: 7, ModelName: "model-a", CreatedAt: start, Type: LogTypeConsume, Quota: 10, PromptTokens: 40, CompletionTokens: 20, UseTime: 2},
		{UserId: 1, Username: "alice", TokenId: 8, ModelName: "model-b", CreatedAt: end - 1, Type: LogTypeConsume, Quota: 15, PromptTokens: 50, CompletionTokens: 30, UseTime: 3},
		{UserId: 1, Username: "alice", TokenId: 7, ModelName: "model-a", CreatedAt: start + 1, Type: LogTypeError, Quota: 900, PromptTokens: 900, UseTime: 1, Other: `{"private":"secret-request"}`},
		{UserId: 2, Username: "bob", TokenId: 9, ModelName: "model-a", CreatedAt: start, Type: LogTypeConsume, Quota: 50, PromptTokens: 200, CompletionTokens: 70, UseTime: 4},
		{UserId: 1, ModelName: "excluded", CreatedAt: start - 1, Type: LogTypeConsume, Quota: 10000},
		{UserId: 1, ModelName: "excluded", CreatedAt: end, Type: LogTypeConsume, Quota: 10000},
		{UserId: 4, ModelName: "excluded", CreatedAt: start, Type: LogTypeConsume, Quota: 10000},
		{UserId: 1, CreatedAt: start, Type: LogTypeTopup, Quota: 10000},
	}).Error)
	names := []string{"bob", "alice", "quiet", "missing", "alice' OR 1=1--"}
	data, err := GetRelayForMultiUserStats(names, start, end)
	require.NoError(t, err)
	require.Len(t, data.Items, 2)
	assert.Equal(t, "bob", data.Items[0].Username)
	assert.Equal(t, "alice", data.Items[1].Username)
	assert.EqualValues(t, 3, data.Items[1].RequestCount)
	assert.EqualValues(t, 1, data.Items[1].ErrorCount)
	assert.EqualValues(t, 25, data.Items[1].Quota)
	assert.EqualValues(t, 90, data.Items[1].PromptTokens)
	assert.EqualValues(t, 50, data.Items[1].CompletionTokens)
	assert.EqualValues(t, 6000, data.Items[1].RequestTime)
	assert.Equal(t, []string{"quiet"}, data.NoUsageUsernames)
	assert.Equal(t, []string{"missing", "alice' OR 1=1--"}, data.UnknownUsernames)
	require.Len(t, data.ModelUsage, 3)
	assert.EqualValues(t, 2, data.ModelUsage[0].RequestCount)
	assert.Equal(t, "alice", data.ModelUsage[0].Username)
	assert.Equal(t, "model-a", data.ModelUsage[0].ModelName)
	encoded, err := json.Marshal(data)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), private)
	assert.NotContains(t, string(encoded), "secret-request")
	assert.NotContains(t, string(encoded), "previous-name")
	assert.NotContains(t, string(encoded), "excluded")
	unknown, err := GetRelayForMultiUserStats([]string{"does-not-exist"}, start, end)
	require.NoError(t, err)
	assert.Empty(t, unknown.Items)
	assert.Empty(t, unknown.ModelUsage)
}

func TestRelayForMultiUserStatsKeepsASCIIAndChineseCommaNamesExact(t *testing.T) {
	setupRelayForStatsTest(t)
	require.NoError(t, DB.Create(&[]User{
		{Id: 1, Username: "alice,bob", AffCode: "ascii-comma"},
		{Id: 2, Username: "张，三", AffCode: "chinese-comma"},
		{Id: 3, Username: "alice", AffCode: "single-alice"},
		{Id: 4, Username: "bob", AffCode: "single-bob"},
	}).Error)
	require.NoError(t, LOG_DB.Create(&[]Log{
		{UserId: 1, Username: "alice,bob", ModelName: "m1", CreatedAt: 100, Type: LogTypeConsume, Quota: 11},
		{UserId: 2, Username: "张，三", ModelName: "m2", CreatedAt: 100, Type: LogTypeConsume, Quota: 22},
		{UserId: 3, Username: "alice", CreatedAt: 100, Type: LogTypeConsume, Quota: 999},
		{UserId: 4, Username: "bob", CreatedAt: 100, Type: LogTypeConsume, Quota: 999},
	}).Error)
	data, err := GetRelayForMultiUserStats([]string{"alice,bob", "张，三"}, 99, 101)
	require.NoError(t, err)
	require.Len(t, data.Items, 2)
	assert.Equal(t, "alice,bob", data.Items[0].Username)
	assert.EqualValues(t, 11, data.Items[0].Quota)
	assert.Equal(t, "张，三", data.Items[1].Username)
	assert.EqualValues(t, 22, data.Items[1].Quota)
	assert.Empty(t, data.UnknownUsernames)
	require.Len(t, data.ModelUsage, 2)
	for _, usage := range data.ModelUsage {
		assert.Less(t, usage.Quota, int64(999))
	}
}

package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayForTokenUsageAggregatesAllUsersAndPagesWithoutSecrets(t *testing.T) {
	setupRelayForStatsTest(t)
	logs := []Log{
		{UserId: 1, Username: "alice", TokenId: 7, TokenName: "production", CreatedAt: 1001, Type: LogTypeConsume, Quota: 10, PromptTokens: 40, CompletionTokens: 20},
		{UserId: 1, Username: "alice", TokenId: 7, TokenName: "production", CreatedAt: 1002, Type: LogTypeConsume, Quota: 15, PromptTokens: 50, CompletionTokens: 30},
		{UserId: 1, Username: "alice", TokenId: 7, TokenName: "production", CreatedAt: 1003, Type: LogTypeError, Quota: 900, PromptTokens: 900, Other: `{"status_code":500,"key":"private-key"}`},
		{UserId: 1, Username: "alice", TokenId: 8, TokenName: "staging", CreatedAt: 1004, Type: LogTypeConsume, Quota: 5, PromptTokens: 10},
		{UserId: 2, Username: "bob", TokenId: 9, TokenName: "production", CreatedAt: 1005, Type: LogTypeConsume, Quota: 50, PromptTokens: 200, CompletionTokens: 70},
		{UserId: 2, Username: "bob", TokenId: 9, TokenName: "production", CreatedAt: 999, Type: LogTypeConsume, Quota: 10000},
		{UserId: 3, Username: "excluded", CreatedAt: 1005, Type: LogTypeTopup, Quota: 10000},
	}
	require.NoError(t, DB.Create(&logs).Error)
	query := RelayForLogQuery{Page: 1, PageSize: 1, StartTimestamp: 1000, EndTimestamp: 1010}
	page, err := GetRelayForTokenUsage(query)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, 2, page.Items[0].UserID)
	assert.EqualValues(t, 3, page.Total)
	assert.EqualValues(t, 3, page.TotalPages)
	assert.EqualValues(t, 80, page.Summary.Quota)
	assert.EqualValues(t, 300, page.Summary.PromptTokens)
	assert.EqualValues(t, 120, page.Summary.CompletionTokens)
	assert.EqualValues(t, 2, page.Summary.ActiveUsers)
	assert.EqualValues(t, 1, page.Summary.Errors)
	query.Page = 2
	page, err = GetRelayForTokenUsage(query)
	require.NoError(t, err)
	assert.Equal(t, 7, page.Items[0].TokenID)
	assert.EqualValues(t, 3, page.Items[0].Requests)
	assert.EqualValues(t, 25, page.Items[0].Quota)
	assert.EqualValues(t, 90, page.Items[0].PromptTokens)
	assert.EqualValues(t, 1003, page.Items[0].LastUsedAt)
	encoded, err := json.Marshal(page)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "private-key")
	assert.NotContains(t, string(encoded), "other")
	query.Page, query.PageSize, query.Keyword = 1, 25, "alice"
	page, err = GetRelayForTokenUsage(query)
	require.NoError(t, err)
	assert.Len(t, page.Items, 2)
	assert.EqualValues(t, 30, page.Summary.Quota)
	query.Keyword = "%"
	page, err = GetRelayForTokenUsage(query)
	require.NoError(t, err)
	assert.Empty(t, page.Items)
}

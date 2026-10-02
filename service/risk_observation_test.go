package service

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeRiskObservationRejectsRawSensitiveMetadata(t *testing.T) {
	input, valid := normalizeRiskObservation(RiskObservationInput{
		UserID: 42, TokenID: -2, PromptTokens: -5,
		IPFingerprint: "192.0.2.15", InputSignature: "private prompt text",
		RequestID: "header with private text", Model: strings.Repeat("a", 190) + "模型",
	})
	require.True(t, valid)
	assert.Empty(t, input.IPFingerprint)
	assert.Empty(t, input.InputSignature)
	assert.Empty(t, input.RequestID)
	assert.Zero(t, input.TokenID)
	assert.Zero(t, input.PromptTokens)
	assert.Equal(t, strings.Repeat("a", 190), input.Model)
	assert.False(t, input.OccurredAt.IsZero())

	_, valid = normalizeRiskObservation(RiskObservationInput{UserID: 0})
	assert.False(t, valid)
}

func TestNormalizeRiskObservationPreservesKeyedDigests(t *testing.T) {
	input, valid := normalizeRiskObservation(RiskObservationInput{
		UserID: 42, IPFingerprint: strings.Repeat("a", 64), InputSignature: strings.Repeat("3", 64),
		RequestID: "req_123-abC", Model: "  gpt-4o  ",
	})
	require.True(t, valid)
	assert.Equal(t, strings.Repeat("a", 64), input.IPFingerprint)
	assert.Equal(t, strings.Repeat("3", 64), input.InputSignature)
	assert.Equal(t, "req_123-abC", input.RequestID)
	assert.Equal(t, "gpt-4o", input.Model)
}

func TestRiskRefreshSchedulerIncludesFinalBurstRequestsWithoutNewTraffic(t *testing.T) {
	var refreshed []int
	scheduler := riskRefreshScheduler{
		users:   make(map[int]riskRefreshState),
		refresh: func(id int) error { refreshed = append(refreshed, id); return nil },
	}
	now := time.Now()
	scheduler.observe(42, now)
	scheduler.observe(42, now.Add(time.Minute))
	scheduler.observe(42, now.Add(2*time.Minute))
	assert.Equal(t, []int{42}, refreshed)
	scheduler.flush(now.Add(riskScoreRefreshInterval))
	assert.Equal(t, []int{42, 42}, refreshed)
	scheduler.flush(now.Add(2 * riskScoreRefreshInterval))
	assert.Empty(t, scheduler.users)
	assert.Equal(t, []int{42, 42}, refreshed)
}

func TestRiskRefreshSchedulerRetriesFailuresAndKeepsHardCapacity(t *testing.T) {
	now := time.Now()
	attempts := 0
	scheduler := riskRefreshScheduler{
		users: make(map[int]riskRefreshState),
		refresh: func(int) error {
			attempts++
			if attempts == 1 {
				return errors.New("temporary database failure")
			}
			return nil
		},
	}
	scheduler.observe(42, now)
	require.True(t, scheduler.users[42].pending)
	scheduler.flush(now.Add(riskMaintenanceInterval))
	assert.Equal(t, 2, attempts)
	assert.False(t, scheduler.users[42].pending)
	for id := 1; id <= riskRefreshUserLimit; id++ {
		scheduler.users[id] = riskRefreshState{nextRefresh: now.Add(riskScoreRefreshInterval)}
	}
	scheduler.observe(riskRefreshUserLimit+1, now)
	assert.Len(t, scheduler.users, riskRefreshUserLimit)
	assert.Equal(t, 2, attempts)
	scheduler.flush(now.Add(riskScoreRefreshInterval))
	assert.Empty(t, scheduler.users)
}

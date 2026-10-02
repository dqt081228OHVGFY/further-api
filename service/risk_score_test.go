package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestRiskScoreCapturesRepeatedMeasurementSignals(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	events := make([]model.RiskEvent, 0, 90)
	for day := range 3 {
		start := time.Date(2026, 9, 21+day, 16, 0, 0, 0, time.UTC)
		for step := range 30 {
			status := 200
			if step <= 10 {
				status = 402
			}
			if step == 8 {
				status = 403
			}
			events = append(events, model.RiskEvent{
				ID: int64(len(events) + 1), UserID: 7, TokenID: 3,
				OccurredAt: start.Add(time.Duration(step) * 2 * time.Minute).Unix(),
				Model:      "model-" + string(rune('a'+step%4)), Stream: false,
				PromptTokens: 12, StatusCode: status,
				ChannelUnavailable: step == 1,
				LivenessKeywordHit: step == 8,
				InputSignature:     strings.Repeat("a", 64),
				IPFingerprint:      strings.Repeat(string(rune('a'+day)), 64),
			})
		}
	}
	snapshot := CalculateRiskSnapshot(7, 3, events, now)
	require.Equal(t, 90, snapshot.RequestCount)
	require.Equal(t, 3, snapshot.IPCount)
	require.Equal(t, 4, snapshot.DistinctModels)
	require.Positive(t, snapshot.RegularTiming)
	require.Positive(t, snapshot.RepeatedInput)
	require.Positive(t, snapshot.MultiModel)
	require.Positive(t, snapshot.Post402)
	require.Positive(t, snapshot.NonStream)
	require.Positive(t, snapshot.NightActivity)
	require.Positive(t, snapshot.PostChannelUnavailable)
	require.Positive(t, snapshot.BlockedLiveness)
	require.Positive(t, snapshot.MultiIP)
	require.GreaterOrEqual(t, snapshot.Score, 50)
	require.LessOrEqual(t, snapshot.Score, 100)
}

func TestRiskScoreFiltersOldEventsAndAvoidsSparseFalsePositives(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	events := []model.RiskEvent{
		{ID: 1, UserID: 8, OccurredAt: now.Add(-5 * 24 * time.Hour).Unix(), StatusCode: 402},
		{ID: 2, UserID: 8, OccurredAt: now.Add(-time.Hour).Unix(), StatusCode: 200, Model: "a", Stream: true},
		{ID: 3, UserID: 8, OccurredAt: now.Add(-30 * time.Minute).Unix(), StatusCode: 200, Model: "b", Stream: true},
	}
	snapshot := CalculateRiskSnapshot(8, 3, events, now)
	require.Equal(t, 2, snapshot.RequestCount)
	require.Zero(t, snapshot.RegularTiming)
	require.Zero(t, snapshot.Post402)
	require.Zero(t, snapshot.NonStream)
	require.Zero(t, snapshot.Score)
	require.Equal(t, "low", snapshot.Level)
}

func TestRiskRegularityLearnsIntervalsAndRequiresThreeDays(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	for _, period := range []time.Duration{5 * time.Minute, 15 * time.Minute, time.Hour} {
		events := make([]model.RiskEvent, 0, 30)
		for day := range 3 {
			for step := range 10 {
				events = append(events, model.RiskEvent{UserID: 9, StatusCode: 200, Stream: true,
					OccurredAt: now.Add(-time.Duration(3-day)*24*time.Hour + time.Duration(step)*period).Unix()})
			}
		}
		require.Positive(t, CalculateRiskSnapshot(9, 3, events, now).RegularTiming)
		require.Zero(t, CalculateRiskSnapshot(9, 3, events[10:], now).RegularTiming)
	}
}

func TestRiskFailedContinuationHonorsRecoveryModelAndSameSecond(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	events := []model.RiskEvent{
		{ID: 1, UserID: 7, OccurredAt: now.Unix() - 50, Model: "a", StatusCode: 402},
		{ID: 2, UserID: 7, OccurredAt: now.Unix() - 50, Model: "a", StatusCode: 402},
		{ID: 3, UserID: 7, OccurredAt: now.Unix() - 50, Model: "a", StatusCode: 402},
		{ID: 4, UserID: 7, OccurredAt: now.Unix() - 50, Model: "a", StatusCode: 402},
		{ID: 5, UserID: 7, OccurredAt: now.Unix() - 49, Model: "a", StatusCode: 200},
		{ID: 6, UserID: 7, OccurredAt: now.Unix() - 48, Model: "a", StatusCode: 500},
		{ID: 7, UserID: 7, OccurredAt: now.Unix() - 47, Model: "a", StatusCode: 503, ChannelUnavailable: true},
		{ID: 8, UserID: 7, OccurredAt: now.Unix() - 46, Model: "b", StatusCode: 503},
		{ID: 9, UserID: 7, OccurredAt: now.Unix() - 45, Model: "a", StatusCode: 503},
		{ID: 10, UserID: 7, OccurredAt: now.Unix() - 44, Model: "a", StatusCode: 200},
		{ID: 11, UserID: 7, OccurredAt: now.Unix() - 43, Model: "a", StatusCode: 503},
	}
	snapshot := CalculateRiskSnapshot(7, 3, events, now)
	require.Equal(t, 3, snapshot.Post402Requests)
	require.Equal(t, 1, snapshot.PostChannelUnavailableRequests)
	require.Zero(t, snapshot.PostChannelUnavailable)
}

func TestRiskNightActivityUsesUTC8AndRequiresHourlyFrequency(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	sparse := make([]model.RiskEvent, 0, 21)
	for day := range 7 {
		for hour := range 3 {
			sparse = append(sparse, model.RiskEvent{UserID: 7, Stream: true, StatusCode: 200,
				OccurredAt: time.Date(2026, 9, 17+day, 16+hour, 0, 0, 0, time.UTC).Unix()})
		}
	}
	require.Zero(t, CalculateRiskSnapshot(7, 7, sparse, now).NightActivity)
	burst := make([]model.RiskEvent, 0, 20)
	for step := range 20 {
		burst = append(burst, model.RiskEvent{UserID: 7, Stream: true, StatusCode: 200,
			OccurredAt: time.Date(2026, 9, 23, 16, 50, 0, 0, time.UTC).Add(time.Duration(step) * time.Minute).Unix()})
	}
	require.Positive(t, CalculateRiskSnapshot(7, 3, burst, now).NightActivity)
}

func TestRiskRejectsForeignFutureEventsAndUnknownTokenFallback(t *testing.T) {
	now := time.Now().UTC()
	events := []model.RiskEvent{
		{UserID: 8, OccurredAt: now.Unix(), StatusCode: 402},
		{UserID: 7, OccurredAt: now.Add(time.Second).Unix(), StatusCode: 402},
	}
	for step := range 25 {
		events = append(events, model.RiskEvent{UserID: 7, OccurredAt: now.Add(-time.Duration(step) * time.Second).Unix(), StatusCode: 401})
	}
	snapshot := CalculateRiskSnapshot(7, 3, events, now)
	require.Equal(t, 25, snapshot.RequestCount)
	require.Zero(t, snapshot.RepeatedInput)
	require.Zero(t, snapshot.NonStream)
	require.Zero(t, snapshot.Post402)
}

func TestRiskTokenFallbackIsWeakerThanExactSignatures(t *testing.T) {
	now := time.Now().UTC()
	events := make([]model.RiskEvent, 12)
	for index := range events {
		events[index] = model.RiskEvent{UserID: 7, OccurredAt: now.Add(-time.Duration(index) * time.Minute).Unix(),
			StatusCode: 200, Stream: true, PromptTokens: 42}
	}
	fallback := CalculateRiskSnapshot(7, 3, events, now)
	require.Positive(t, fallback.RepeatedInput)
	for index := range events {
		events[index].InputSignature = strings.Repeat("b", 64)
	}
	exact := CalculateRiskSnapshot(7, 3, events, now)
	require.Greater(t, exact.RepeatedInput, fallback.RepeatedInput)
}

func TestRiskNewAPIQuotaExhaustionCountsWithoutChangingHTTPStatus(t *testing.T) {
	now := time.Now().UTC()
	events := []model.RiskEvent{
		{ID: 1, UserID: 7, OccurredAt: now.Unix() - 1, StatusCode: 403, QuotaExhausted: true},
		{ID: 2, UserID: 7, OccurredAt: now.Unix(), StatusCode: 403, QuotaExhausted: true},
		{ID: 3, UserID: 7, OccurredAt: now.Unix(), StatusCode: 403, QuotaExhausted: true},
		{ID: 4, UserID: 7, OccurredAt: now.Unix(), StatusCode: 403, QuotaExhausted: true},
	}
	snapshot := CalculateRiskSnapshot(7, 3, events, now)
	require.Equal(t, 3, snapshot.Post402Requests)
	require.Positive(t, snapshot.Post402)
	for index := range events {
		events[index].QuotaExhausted = false
	}
	require.Zero(t, CalculateRiskSnapshot(7, 3, events, now).Post402)
}

func TestRiskAssessmentExposesOnlyAggregateEvidence(t *testing.T) {
	assessment := riskAssessment(model.RiskSnapshot{UserID: 7, Score: 42, Level: "medium", SampleLimited: true}, "risk-user", "")
	encoded, err := json.Marshal(assessment)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(encoded, &payload))
	require.Equal(t, "unreviewed", payload["status"])
	signals := payload["signals"].(map[string]any)
	require.Len(t, signals, 9)
	for _, field := range []string{"input_signature", "ip_fingerprint", "request_id", "prompt", "body", "token_id"} {
		require.NotContains(t, string(encoded), `"`+field+`"`)
	}
	require.True(t, payload["evidence"].(map[string]any)["sample_limited"].(bool))
}

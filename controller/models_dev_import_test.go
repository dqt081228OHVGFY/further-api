package controller

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/stretchr/testify/require"
)

type devImportTransport func(*http.Request) (*http.Response, error)

func (f devImportTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func fixtureDevImport(t *testing.T) {
	t.Helper()
	db := modelManagementDB(t, "sqlite", "")
	previousClient, previousUnit := modelsDevImportClient, common.QuotaPerUnit
	common.QuotaPerUnit = 1000000
	modelsDevImportClient = &http.Client{Transport: devImportTransport(func(request *http.Request) (*http.Response, error) {
		if err := request.Context().Err(); err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"openai":{"name":"OpenAI","models":{"gpt-4o":{"id":"gpt-4o","cost":{"input":2.5,"output":10}},"gpt-free":{"cost":{"input":4,"output":8}},"gpt-tiers":{"cost":{"input":2,"output":3,"tiers":[{"input":4}]}},"request-model":{"cost":{"input":1,"output":2}},"unbound-model":{"cost":{"input":1,"output":2}}}},"reseller":{"name":"Reseller","models":{"gpt-4o":{"cost":{"input":0.5,"output":1}}}}}`)), Header: make(http.Header)}, nil
	})}
	devImportSnapshots.Lock()
	devImportSnapshots.entries = map[string]*devImportSnapshot{}
	devImportSnapshots.Unlock()
	t.Cleanup(func() {
		modelsDevImportClient, common.QuotaPerUnit = previousClient, previousUnit
		devImportSnapshots.Lock()
		devImportSnapshots.entries = map[string]*devImportSnapshot{}
		devImportSnapshots.Unlock()
	})
	require.NoError(t, db.Create(&model.Channel{Name: "pricing-source", Type: 1, Models: "gpt-4o,gpt-free,gpt-tiers,request-model", Key: "never-export-this-key", Group: "default", Status: 1}).Error)
	require.NoError(t, db.Create(&model.Model{ModelName: "unbound-model", Status: 1}).Error)
	require.NoError(t, model.UpdateOption("ModelRatio", `{"gpt-4o":4,"gpt-free":0}`))
	require.NoError(t, model.UpdateOption("CompletionRatio", `{"gpt-4o":3}`))
	require.NoError(t, model.UpdateOption("CacheRatio", `{"gpt-4o":0.25}`))
	require.NoError(t, model.UpdateOption("CreateCacheRatio", `{"gpt-4o":1.25}`))
	require.NoError(t, model.UpdateOption("ModelPrice", `{"request-model":0.15}`))
}

func previewDevImport(t *testing.T, owner int) *devImportSnapshot {
	t.Helper()
	ctx, response := newAuthenticatedContext(t, http.MethodGet, "/api/further/models/pricing/models-dev", nil, owner)
	PreviewModelsDevImport(ctx)
	require.Equal(t, 200, response.Code)
	var envelope struct {
		Success bool              `json:"success"`
		Data    devImportSnapshot `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
	require.True(t, envelope.Success)
	require.NotContains(t, response.Body.String(), "never-export-this-key")
	return &envelope.Data
}

func TestModelsDevImportSelectsOfficialAndPreservesProtectedModels(t *testing.T) {
	fixtureDevImport(t)
	snapshot := previewDevImport(t, 51)
	items := map[string]devImportItem{}
	for _, item := range snapshot.Items {
		items[item.Model] = item
	}
	require.Equal(t, "matched", items["gpt-4o"].Status)
	require.Equal(t, 1, items["gpt-4o"].ChannelCount)
	require.Equal(t, 4.0, *items["gpt-4o"].Current.Input)
	require.Equal(t, 2, len(items["gpt-4o"].Candidates))
	for _, candidate := range items["gpt-4o"].Candidates {
		require.Equal(t, candidate.ProviderID == "openai", candidate.Recommended)
		require.NotNil(t, candidate.EffectivePrice)
		require.Equal(t, 1.0, *candidate.EffectivePrice.Read)
		require.Equal(t, 5.0, *candidate.EffectivePrice.Write)
	}
	require.Equal(t, "protected", items["gpt-free"].Status)
	require.Equal(t, "protected", items["request-model"].Status)
	require.Equal(t, "unmatched", items["gpt-tiers"].Status)
	require.Equal(t, "unbound", items["unbound-model"].Status)
	input := map[string]any{"snapshot_id": snapshot.ID, "selections": []devImportSelection{{Model: "gpt-4o", ProviderID: "openai", ModelID: "gpt-4o"}}}
	ctx, response := newAuthenticatedContext(t, http.MethodPost, "/api/further/models/pricing/models-dev", input, 52)
	ApplyModelsDevImport(ctx)
	require.Equal(t, 409, response.Code)
	ctx, response = newAuthenticatedContext(t, http.MethodPost, "/api/further/models/pricing/models-dev", input, 51)
	ApplyModelsDevImport(ctx)
	require.Equal(t, 200, response.Code)
	state, err := model.GetModelPricingSnapshot([]string{"gpt-4o", "gpt-free", "request-model"})
	require.NoError(t, err)
	for _, entry := range state.Entries {
		switch entry.ModelName {
		case "gpt-4o":
			require.Equal(t, 2.5, entry.Effective["ModelRatio"])
			require.Equal(t, 4.0, entry.Effective["CompletionRatio"])
			require.Equal(t, 0.4, entry.Effective["CacheRatio"])
			require.Equal(t, 2.0, entry.Effective["CreateCacheRatio"])
		case "gpt-free":
			require.Equal(t, 0.0, entry.Effective["ModelRatio"])
		case "request-model":
			require.Equal(t, 0.15, entry.Effective["ModelPrice"])
		}
	}
	ctx, response = newAuthenticatedContext(t, http.MethodPost, "/api/further/models/pricing/models-dev", input, 51)
	ApplyModelsDevImport(ctx)
	require.Equal(t, 200, response.Code, "duplicate delivery returns the same applied result")
}

func TestModelsDevImportRejectsChangedPricesAndExpiredSnapshot(t *testing.T) {
	fixtureDevImport(t)
	snapshot := previewDevImport(t, 51)
	require.NoError(t, model.UpdateOption("ModelRatio", `{"gpt-4o":8,"gpt-free":0}`))
	input := map[string]any{"snapshot_id": snapshot.ID, "selections": []devImportSelection{{Model: "gpt-4o", ProviderID: "openai", ModelID: "gpt-4o"}}}
	ctx, response := newAuthenticatedContext(t, http.MethodPost, "/api/further/models/pricing/models-dev", input, 51)
	ApplyModelsDevImport(ctx)
	require.Equal(t, 409, response.Code)
	state, err := model.GetModelPricingSnapshot([]string{"gpt-4o"})
	require.NoError(t, err)
	require.Equal(t, 8.0, state.Entries[0].Effective["ModelRatio"])
	devImportSnapshots.Lock()
	devImportSnapshots.entries[snapshot.ID].expiry = time.Now().Add(-time.Second)
	devImportSnapshots.Unlock()
	ctx, response = newAuthenticatedContext(t, http.MethodPost, "/api/further/models/pricing/models-dev", input, 51)
	ApplyModelsDevImport(ctx)
	require.Equal(t, 409, response.Code)
}

func TestModelsDevImportCancelsBeforeReadingSource(t *testing.T) {
	fixtureDevImport(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := readDevImportCatalog(ctx)
	require.ErrorIs(t, err, context.Canceled)
}

func TestModelsDevImportRejectsProtectedBatchWithoutPartialWrite(t *testing.T) {
	fixtureDevImport(t)
	snapshot := previewDevImport(t, 51)
	input := map[string]any{"snapshot_id": snapshot.ID, "selections": []devImportSelection{{Model: "gpt-4o", ProviderID: "openai", ModelID: "gpt-4o"}, {Model: "gpt-free", ProviderID: "openai", ModelID: "gpt-free"}}}
	ctx, response := newAuthenticatedContext(t, http.MethodPost, "/api/further/models/pricing/models-dev", input, 51)
	ApplyModelsDevImport(ctx)
	require.Equal(t, 400, response.Code)
	state, err := model.GetModelPricingSnapshot([]string{"gpt-4o"})
	require.NoError(t, err)
	require.Equal(t, 4.0, state.Entries[0].Effective["ModelRatio"])
	common.QuotaPerUnit = 500000
	input["selections"] = []devImportSelection{{Model: "gpt-4o", ProviderID: "openai", ModelID: "gpt-4o"}}
	ctx, response = newAuthenticatedContext(t, http.MethodPost, "/api/further/models/pricing/models-dev", input, 51)
	ApplyModelsDevImport(ctx)
	require.Equal(t, 409, response.Code, "changing the quota unit invalidates the displayed comparison")
}

func TestModelsDevMissingOnlyKeepsExistingPricesAndRejectsForgedSelection(t *testing.T) {
	fixtureDevImport(t)
	ctx, response := newAuthenticatedContext(t, http.MethodGet, "/api/further/models/pricing/models-dev?missing_only=true", nil, 51)
	PreviewModelsDevImport(ctx)
	require.Equal(t, http.StatusOK, response.Code)
	var envelope struct {
		Data devImportSnapshot `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
	require.True(t, envelope.Data.MissingOnly)
	for _, item := range envelope.Data.Items {
		if item.Model == "gpt-4o" {
			require.Equal(t, "configured", item.Status)
			require.Empty(t, item.Candidates)
		}
	}
	input := map[string]any{"snapshot_id": envelope.Data.ID, "selections": []devImportSelection{{Model: "gpt-4o", ProviderID: "openai", ModelID: "gpt-4o"}}}
	ctx, response = newAuthenticatedContext(t, http.MethodPost, "/api/further/models/pricing/models-dev", input, 51)
	ApplyModelsDevImport(ctx)
	require.Equal(t, http.StatusBadRequest, response.Code)
	state, err := model.GetModelPricingSnapshot([]string{"gpt-4o"})
	require.NoError(t, err)
	require.Equal(t, 4.0, state.Entries[0].Effective["ModelRatio"])
}

func TestModelsDevMissingOnlyImportsMissingChannelPrice(t *testing.T) {
	fixtureDevImport(t)
	require.NoError(t, model.UpdateOption("ModelRatio", `{"gpt-free":0}`))
	ctx, response := newAuthenticatedContext(t, http.MethodGet, "/api/further/models/pricing/models-dev?missing_only=true", nil, 51)
	PreviewModelsDevImport(ctx)
	require.Equal(t, http.StatusOK, response.Code)
	var envelope struct {
		Data devImportSnapshot `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
	input := map[string]any{"snapshot_id": envelope.Data.ID, "selections": []devImportSelection{{Model: "gpt-4o", ProviderID: "openai", ModelID: "gpt-4o"}}}
	ctx, response = newAuthenticatedContext(t, http.MethodPost, "/api/further/models/pricing/models-dev", input, 51)
	ApplyModelsDevImport(ctx)
	require.Equal(t, http.StatusOK, response.Code)
	state, err := model.GetModelPricingSnapshot([]string{"gpt-4o"})
	require.NoError(t, err)
	require.Equal(t, 2.5, state.Entries[0].Effective["ModelRatio"])
}

func TestModelsDevDraftReferenceDoesNotWriteOrImportUnboundModel(t *testing.T) {
	fixtureDevImport(t)
	ctx, response := newAuthenticatedContext(t, http.MethodGet, "/api/further/models/pricing/models-dev?missing_only=true&model=unbound-model", nil, 51)
	PreviewModelsDevImport(ctx)
	require.Equal(t, http.StatusOK, response.Code)
	var envelope struct {
		Data devImportSnapshot `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
	require.Len(t, envelope.Data.Items, 1)
	require.Equal(t, "reference", envelope.Data.Items[0].Status)
	require.Len(t, envelope.Data.Items[0].Candidates, 1)
	input := map[string]any{"snapshot_id": envelope.Data.ID, "selections": []devImportSelection{{Model: "unbound-model", ProviderID: "openai", ModelID: "unbound-model"}}}
	ctx, response = newAuthenticatedContext(t, http.MethodPost, "/api/further/models/pricing/models-dev", input, 51)
	ApplyModelsDevImport(ctx)
	require.Equal(t, http.StatusBadRequest, response.Code)
	state, err := model.GetModelPricingSnapshot([]string{"unbound-model"})
	require.NoError(t, err)
	require.NotContains(t, state.Entries[0].Configured, "ModelRatio")
}

func TestModelsDevMissingOnlyRejectsConcurrentPricingAddition(t *testing.T) {
	fixtureDevImport(t)
	require.NoError(t, model.UpdateOption("ModelRatio", `{"gpt-free":0}`))
	snapshot, err := buildDevImportSnapshot(context.Background(), 51, true, "")
	require.NoError(t, err)
	devImportSnapshots.Lock()
	devImportSnapshots.entries[snapshot.ID] = snapshot
	devImportSnapshots.Unlock()
	require.NoError(t, model.UpdateOption("ModelRatio", `{"gpt-4o":9,"gpt-free":0}`))
	input := map[string]any{"snapshot_id": snapshot.ID, "selections": []devImportSelection{{Model: "gpt-4o", ProviderID: "openai", ModelID: "gpt-4o"}}}
	ctx, response := newAuthenticatedContext(t, http.MethodPost, "/api/further/models/pricing/models-dev", input, 51)
	ApplyModelsDevImport(ctx)
	require.Equal(t, http.StatusConflict, response.Code)
	state, err := model.GetModelPricingSnapshot([]string{"gpt-4o"})
	require.NoError(t, err)
	require.Equal(t, 9.0, state.Entries[0].Effective["ModelRatio"])
}

func TestModelsDevReferenceReadsExistingPriceWithoutWriting(t *testing.T) {
	fixtureDevImport(t)
	ctx, response := newAuthenticatedContext(t, http.MethodGet, "/api/further/models/pricing/models-dev?model=gpt-4o", nil, 51)
	PreviewModelsDevImport(ctx)
	require.Equal(t, http.StatusOK, response.Code)
	var envelope struct {
		Data devImportSnapshot `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
	require.Len(t, envelope.Data.Items, 1)
	require.Equal(t, "matched", envelope.Data.Items[0].Status)
	require.Len(t, envelope.Data.Items[0].Candidates, 2)
	state, err := model.GetModelPricingSnapshot([]string{"gpt-4o"})
	require.NoError(t, err)
	require.Equal(t, 4.0, state.Entries[0].Effective["ModelRatio"])
}

func TestModelsDevRejectsPricesThatCannotMatchEffectiveBilling(t *testing.T) {
	fixtureDevImport(t)
	name := "gpt-4o-2024-05-13"
	require.NoError(t, model.DB.Create(&model.Channel{Name: "locked-output", Type: 1, Models: name, Group: "default", Status: 1}).Error)
	modelsDevImportClient = &http.Client{Transport: devImportTransport(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"openai":{"models":{"gpt-4o-2024-05-13":{"cost":{"input":5,"output":15}}}},"reseller":{"models":{"gpt-4o-2024-05-13":{"cost":{"input":5,"output":50}}}}}`)), Header: make(http.Header)}, nil
	})}
	snapshot, err := buildDevImportSnapshot(context.Background(), 51, false, name)
	require.NoError(t, err)
	require.Len(t, snapshot.Items, 1)
	item := snapshot.Items[0]
	require.True(t, item.CompletionRatioInfo.Locked)
	require.Equal(t, 3.0, item.CompletionRatioInfo.Ratio)
	require.Len(t, item.Candidates, 1)
	require.Equal(t, "openai", item.Candidates[0].ProviderID)
	_, err = devImportDraft(item, devImportCandidate{Input: 5, Output: 50})
	require.ErrorContains(t, err, "无法")
	common.QuotaPerUnit = 500000
	_, err = devImportDraft(devImportItem{Model: "tiny"}, devImportCandidate{Input: 5e-324, Output: 0})
	require.ErrorContains(t, err, "无法")
}

func TestModelsDevRejectsOutputAndCacheRatiosThatLosePositivePrices(t *testing.T) {
	previousUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = previousUnit })
	tiny := math.SmallestNonzeroFloat64
	for _, test := range []struct {
		name      string
		candidate devImportCandidate
	}{
		{"output underflow", devImportCandidate{Input: 1e308, Output: tiny}},
		{"output overflow", devImportCandidate{Input: 1e-308, Output: 1e308}},
		{"cache read underflow", devImportCandidate{Input: 1e308, Output: 1e308, Read: &tiny}},
		{"cache write underflow", devImportCandidate{Input: 1e308, Output: 1e308, Write: &tiny}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := devImportDraft(devImportItem{Model: "custom-ratio-model"}, test.candidate)
			require.Error(t, err, "a positive source price must not become a free output/cache rate or an infinite ratio")
		})
	}
	draft, err := devImportDraft(devImportItem{Model: "custom-ratio-model"}, devImportCandidate{Input: 2, Output: 0, Read: new(float64), Write: new(float64)})
	require.NoError(t, err)
	require.Equal(t, float64(0), draft["CompletionRatio"])
	require.Equal(t, float64(0), draft["CacheRatio"])
	require.Equal(t, float64(0), draft["CreateCacheRatio"])
}

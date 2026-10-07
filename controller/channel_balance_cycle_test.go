package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/stretchr/testify/require"
)

func TestBalanceCycleBoundsConcurrentRequestsAndPersistsEveryResult(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	previousInterval := common.RequestInterval
	common.RequestInterval = 0
	t.Cleanup(func() { common.RequestInterval = previousInterval })
	entered := make(chan struct{}, 8)
	release := make(chan struct{})
	var releaseOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		entered <- struct{}{}
		select {
		case <-request.Context().Done():
			return
		case <-release:
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"object\":\"credit_summary\",\"total_available\":10}"))
	}))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		releaseOnce.Do(func() { close(release) })
		server.Close()
	})
	for range 5 {
		channel := &model.Channel{Name: "bounded-balance", Type: constant.ChannelTypeAdvancedCustom, Status: common.ChannelStatusEnabled, BaseURL: &server.URL, Key: "unused-key"}
		channel.SetOtherSettings(dto.ChannelOtherSettings{AdvancedCustom: &dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{IncomingPath: dto.AdvancedCustomBalancePath, UpstreamPath: "/balance", Converter: "none"}}}})
		require.NoError(t, db.Create(channel).Error)
	}
	type cycleResult struct {
		summary channelBalanceCycleSummary
		err     error
	}
	completed := make(chan cycleResult, 1)
	go func() { summary, err := runChannelBalanceCycle(ctx); completed <- cycleResult{summary, err} }()
	for range 4 {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			t.Fatal("independent balance requests did not run concurrently")
		}
	}
	select {
	case <-entered:
		t.Fatal("more than four balance requests ran concurrently")
	case <-time.After(50 * time.Millisecond):
	}
	releaseOnce.Do(func() { close(release) })
	select {
	case result := <-completed:
		require.NoError(t, result.err)
		require.Equal(t, 5, result.summary.Checked)
		require.Equal(t, 5, result.summary.Succeeded)
		require.Zero(t, result.summary.Failed)
	case <-time.After(2 * time.Second):
		t.Fatal("balance cycle did not finish after all responses completed")
	}
	var persisted int64
	require.NoError(t, db.Model(&model.Channel{}).Where("balance = ?", 10).Count(&persisted).Error)
	require.EqualValues(t, 5, persisted)
}

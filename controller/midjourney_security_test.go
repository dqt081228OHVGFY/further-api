package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func midjourneySecurityDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	previousDB, previousRedis := model.DB, common.RedisEnabled
	model.DB, common.RedisEnabled = db, false
	t.Cleanup(func() {
		model.DB, common.RedisEnabled = previousDB, previousRedis
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Midjourney{}))
	service.InitHttpClient()
	return db
}

func TestMidjourneyPollingSeparatesDuplicateIDsAndRedactsReflectedCredentials(t *testing.T) {
	db := midjourneySecurityDB(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "same-job", "status": "FAILURE", "progress": "100%", "failReason": "Rejected " + r.Header.Get("mj-api-secret"), "submitTime": time.Now().UnixMilli()}})
	}))
	defer upstream.Close()
	first := &model.Channel{Id: 84050, Type: constant.ChannelTypeMidjourney, Key: "opaque_mj_key_a_84050", Status: common.ChannelStatusEnabled, BaseURL: &upstream.URL}
	second := &model.Channel{Id: 84051, Type: constant.ChannelTypeMidjourney, Key: "opaque_mj_key_b_84051", Status: common.ChannelStatusEnabled, BaseURL: &upstream.URL}
	for _, ch := range []*model.Channel{first, second} {
		require.NoError(t, db.Create(ch).Error)
		require.NoError(t, db.Create(&model.Midjourney{ChannelId: ch.Id, UserId: ch.Id, MjId: "same-job", Status: "IN_PROGRESS", Progress: "30%", SubmitTime: time.Now().UnixMilli()}).Error)
	}
	summary := runMidjourneyTaskUpdateOnce(context.Background(), nil)
	require.Equal(t, 2, summary.ChannelsScanned)
	var tasks []model.Midjourney
	require.NoError(t, db.Order("id asc").Find(&tasks).Error)
	for _, task := range tasks {
		require.Equal(t, "FAILURE", task.Status)
		require.Contains(t, task.FailReason, "REDACTED")
		require.NotContains(t, task.FailReason, first.Key)
		require.NotContains(t, task.FailReason, second.Key)
	}
}

func TestMidjourneyPollingBoundsResponseBytesAndPendingRows(t *testing.T) {
	db := midjourneySecurityDB(t)
	previousLimit := constant.TaskQueryLimit
	constant.TaskQueryLimit = 1
	t.Cleanup(func() { constant.TaskQueryLimit = previousLimit })
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("[]" + strings.Repeat(" ", 4<<20)))
	}))
	defer upstream.Close()
	ch := &model.Channel{Id: 84052, Type: constant.ChannelTypeMidjourney, Key: "opaque_mj_key_84052", Status: common.ChannelStatusEnabled, BaseURL: &upstream.URL}
	require.NoError(t, db.Create(ch).Error)
	for _, id := range []string{"pending-first", "pending-second"} {
		require.NoError(t, db.Create(&model.Midjourney{ChannelId: ch.Id, MjId: id, Status: "IN_PROGRESS", Progress: "30%", SubmitTime: time.Now().UnixMilli()}).Error)
	}
	require.Len(t, model.GetAllUnFinishTasks(), 1)
	summary := runMidjourneyTaskUpdateOnce(context.Background(), nil)
	require.Equal(t, 1, summary.UnfinishedTasks)
	var tasks []model.Midjourney
	require.NoError(t, db.Find(&tasks).Error)
	for _, task := range tasks {
		require.Equal(t, "IN_PROGRESS", task.Status)
	}
}

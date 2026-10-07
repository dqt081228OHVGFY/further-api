package controller

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestChannelModelsImportKeepsAliasesCatalogAndPrices(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/models", r.URL.Path)
		require.Equal(t, "Bearer saved-secret", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"data":[{"id":"new-model"},{"id":"old-model"},{"id":"other-model"},{"id":"new-model"}]}`))
	}))
	defer server.Close()
	url, mapping := server.URL+"/v1", `{"public-alias":"old-model"}`
	ch := &model.Channel{Name: "saved", Type: constant.ChannelTypeOpenAI, Key: "saved-secret", BaseURL: &url, Models: "public-alias,old-model", ModelMapping: &mapping, Status: common.ChannelStatusEnabled, Group: "default,vip"}
	require.NoError(t, ch.Insert())
	metadata := model.Model{ModelName: "old-model", Status: 1, Description: "existing metadata"}
	require.NoError(t, db.Create(&metadata).Error)
	option := model.Option{Key: "ModelRatio", Value: `{"old-model":2.75}`}
	require.NoError(t, db.Create(&option).Error)
	for index := 0; index < 2; index++ {
		added, final, err := importChannelModelsContext(context.Background(), ch, []string{" new-model ", "new-model"})
		require.NoError(t, err)
		require.Equal(t, []string{"public-alias", "old-model", "new-model"}, final)
		if index == 0 {
			require.Equal(t, []string{"new-model"}, added)
		} else {
			require.Empty(t, added)
		}
	}
	stored, err := model.GetChannelById(ch.Id, true)
	require.NoError(t, err)
	require.Equal(t, mapping, stored.GetModelMapping())
	require.Equal(t, "saved-secret", stored.Key)
	var abilities []model.Ability
	require.NoError(t, db.Where("channel_id = ?", ch.Id).Find(&abilities).Error)
	require.Len(t, abilities, 6)
	var still model.Option
	require.NoError(t, db.First(&still, "key = ?", option.Key).Error)
	require.Equal(t, option.Value, still.Value)
	var count int64
	require.NoError(t, db.Model(&model.Model{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	rows, _, err := model.SearchModelsWithChannels("new-model", "", "", "", 0, 50)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "new-model", rows[0].ModelName)
}

func TestChannelModelsImportRejectsStaleSelectionAndRollsBackIndexFailure(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"data":[{"id":"new-model"}]}`)) }))
	defer server.Close()
	url := server.URL
	ch := &model.Channel{Type: 1, Key: "secret", BaseURL: &url, Models: "old-model", Status: 1, Group: "default"}
	require.NoError(t, ch.Insert())
	_, _, err := importChannelModelsContext(context.Background(), ch, []string{"nonexistent"})
	require.ErrorContains(t, err, "不在上游目录")
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("reject_import_index", func(tx *gorm.DB) {
		if tx.Statement.Table == "abilities" {
			tx.AddError(errors.New("index write rejected"))
		}
	}))
	defer db.Callback().Create().Remove("reject_import_index")
	_, _, err = importChannelModelsContext(context.Background(), ch, []string{"new-model"})
	require.ErrorContains(t, err, "index write rejected")
	stored, err := model.GetChannelById(ch.Id, true)
	require.NoError(t, err)
	require.Equal(t, "old-model", stored.Models)
	var abilities []model.Ability
	require.NoError(t, db.Where("channel_id = ?", ch.Id).Find(&abilities).Error)
	require.Len(t, abilities, 1)
	require.Equal(t, "old-model", abilities[0].Model)
}

func TestChannelModelsImportMergesConcurrentModelsButRejectsChangedCredentials(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	var update func()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		update()
		_, _ = w.Write([]byte(`{"data":[{"id":"selected"}]}`))
	}))
	defer server.Close()
	url := server.URL
	ch := &model.Channel{Type: 1, Key: "secret", BaseURL: &url, Models: "old", Group: "default", Status: 1}
	require.NoError(t, ch.Insert())
	update = func() {
		require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", ch.Id).Update("models", "old,concurrent").Error)
	}
	_, final, err := importChannelModelsContext(context.Background(), ch, []string{"selected"})
	require.NoError(t, err)
	require.Equal(t, []string{"old", "concurrent", "selected"}, final)
	update = func() {
		require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", ch.Id).Update("key", "new-secret").Error)
	}
	_, _, err = importChannelModelsContext(context.Background(), ch, []string{"selected"})
	require.ErrorContains(t, err, "连接配置已更新")
}

func TestFetchOrdinarySavedModelsUsesServerCredentialAndRequestContext(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer enabled", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"data":[{"id":"saved-model"}]}`))
	}))
	defer server.Close()
	url := server.URL
	ch := &model.Channel{Type: 1, Key: "disabled\nenabled", BaseURL: &url, Models: "unchanged", ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeyStatusList: map[int]int{0: common.ChannelStatusManuallyDisabled, 1: common.ChannelStatusEnabled}}}
	require.NoError(t, db.Create(ch).Error)
	body, err := common.Marshal(fetchModelsRequest{ChannelID: ch.Id, Type: 1, Key: ""})
	require.NoError(t, err)
	call := func(ctx context.Context) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/fetch_models", bytes.NewReader(body)).WithContext(ctx)
		c.Request.Header.Set("Content-Type", "application/json")
		FetchModels(c)
		return recorder
	}
	result := call(context.Background())
	require.JSONEq(t, `{"success":true,"message":"","data":["saved-model"]}`, result.Body.String())
	require.NotContains(t, result.Body.String(), "enabled")
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	result = call(cancelled)
	require.Contains(t, result.Body.String(), "context canceled")
	stored, err := model.GetChannelById(ch.Id, true)
	require.NoError(t, err)
	require.Equal(t, "unchanged", stored.Models)
}

func TestChannelModelsSelectionAndEndpointValidation(t *testing.T) {
	for _, values := range [][]string{nil, {"bad,id"}, {"bad\nmodel"}} {
		_, err := validSelectedChannelModels(values)
		require.Error(t, err)
	}
	require.Equal(t, "https://upstream.test/v1/models", channelModelsEndpoint("https://upstream.test/v1/", "/v1/models"))
	require.Equal(t, "https://upstream.test/compatible-mode/v1/models", channelModelsEndpoint("https://upstream.test/compatible-mode/v1", "/compatible-mode/v1/models"))
}

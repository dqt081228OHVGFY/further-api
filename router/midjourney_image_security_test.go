package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/dqt081228OHVGFY/further-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMidjourneyImageRequiresRowCapabilityOrOwnerToken(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Midjourney{}, &model.Channel{}))
	priorSecret := common.CryptoSecret
	priorFetch := *system_setting.GetFetchSetting()
	common.CryptoSecret = "isolated-midjourney-image-security-secret"
	// Local fixtures exercise the HTTP boundary without reaching a real upstream.
	system_setting.GetFetchSetting().EnableSSRFProtection = false
	t.Cleanup(func() { common.CryptoSecret = priorSecret; *system_setting.GetFetchSetting() = priorFetch })
	service.InitHttpClient()
	owner := model.User{Username: "image-owner", AffCode: "img-owner", Status: common.UserStatusEnabled, Quota: 100, Group: "default"}
	other := model.User{Username: "image-other", AffCode: "img-other", Status: common.UserStatusEnabled, Quota: 100, Group: "default"}
	require.NoError(t, model.DB.Create(&owner).Error)
	require.NoError(t, model.DB.Create(&other).Error)
	for _, pair := range []struct {
		id  int
		key string
	}{{owner.Id, "imageownerkey"}, {other.Id, "imageotherkey"}} {
		require.NoError(t, model.DB.Create(&model.Token{UserId: pair.id, Key: pair.key, Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}).Error)
	}
	var requests atomic.Int64
	var activeContent atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if activeContent.Load() {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<script>window.compromised=true</script>"))
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("isolated-image-fixture"))
	}))
	defer upstream.Close()
	task := model.Midjourney{UserId: owner.Id, MjId: "predictable-upstream-id", ImageUrl: upstream.URL, Status: "SUCCESS"}
	require.NoError(t, model.DB.Create(&task).Error)
	duplicate := model.Midjourney{UserId: other.Id, MjId: task.MjId, ImageUrl: upstream.URL, Status: "SUCCESS"}
	require.NoError(t, model.DB.Create(&duplicate).Error)
	access, err := service.IssueTaskArtifactAccess(service.MidjourneyImageAccessTaskID(strconv.Itoa(task.Id)), service.MidjourneyImageArtifactKey)
	require.NoError(t, err)
	engine := gin.New()
	SetRelayRouter(engine)
	request := func(path, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, r)
		return w
	}
	path := fmt.Sprintf("/mj/image/%d", task.Id)
	for _, test := range []struct {
		name, path, token string
		status            int
	}{
		{"no credentials", path, "", http.StatusUnauthorized},
		{"legacy upstream id", "/mj/image/" + task.MjId, "", http.StatusUnauthorized},
		{"other owner", path, "imageotherkey", http.StatusNotFound},
		{"tampered capability", path + "?access=invalid", "", http.StatusNotFound},
		{"other row same upstream id", fmt.Sprintf("/mj/image/%d?access=%s", duplicate.Id, url.QueryEscape(access)), "", http.StatusNotFound},
	} {
		t.Run(test.name, func(t *testing.T) { require.Equal(t, test.status, request(test.path, test.token).Code) })
	}
	require.Zero(t, requests.Load(), "rejected callers must not contact upstream")
	capabilityPath := path + "?access=" + url.QueryEscape(access)
	for _, test := range []struct{ name, path, token string }{{"capability", capabilityPath, ""}, {"owner token", path, "imageownerkey"}} {
		t.Run(test.name, func(t *testing.T) {
			response := request(test.path, test.token)
			require.Equal(t, http.StatusOK, response.Code)
			require.Equal(t, "isolated-image-fixture", response.Body.String())
			require.Equal(t, "sandbox; default-src 'none'", response.Header().Get("Content-Security-Policy"))
			require.Equal(t, "nosniff", response.Header().Get("X-Content-Type-Options"))
			require.Equal(t, "private, no-store", response.Header().Get("Cache-Control"))
		})
	}
	activeContent.Store(true)
	response := request(capabilityPath, "")
	require.Equal(t, http.StatusBadGateway, response.Code)
	require.NotContains(t, response.Body.String(), "<script>")
	require.NoError(t, model.DB.Model(&owner).Update("status", common.UserStatusDisabled).Error)
	before := requests.Load()
	require.Equal(t, http.StatusNotFound, request(capabilityPath, "").Code)
	require.Equal(t, before, requests.Load(), "disabled owners revoke capability access immediately")
}

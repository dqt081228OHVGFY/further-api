package router

import (
	"net/http"
	"net/url"
	"testing"
	"testing/fstest"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestFeatureLinksServeSelectedWorkspaceWithoutAnotherDocument(t *testing.T) {
	previous := common.GlobalWebRateLimitEnable
	common.GlobalWebRateLimitEnable = false
	t.Cleanup(func() { common.GlobalWebRateLimitEnable = previous })
	assets := fstest.MapFS{
		"web/dist/index.html":         {Data: []byte("<html><head></head><body>public home</body></html>")},
		"web/dist/admin/index.html":   {Data: []byte("<html><head></head><body>admin workspace</body></html>")},
		"web/dist/console/index.html": {Data: []byte("<html><head></head><body>account workspace</body></html>")},
	}
	engine := gin.New()
	SetWebRouter(engine, WebAssets{BuildFS: assets}, nil)
	for _, test := range []struct{ path, document, destination string }{
		{"/security", "account workspace", "/console/#profile"},
		{"/risk-control", "admin workspace", "/admin/#risk"},
		{"/subscriptions", "admin workspace", "/admin/#subscriptions/paid"},
		{"/activities/national_day", "account workspace", "/console/#activities/national_day"},
		{"/system-settings/billing/checkin", "admin workspace", "/admin/#activities"},
		{"/models/metadata", "admin workspace", "/admin/#models"},
		{"/models/deployments", "admin workspace", "/admin/#models"},
		{"/keys", "account workspace", "/console/#tokens"},
		{"/profile", "account workspace", "/console/#profile"},
		{"/usage-logs/common", "admin workspace", "/admin/#logs"},
		{"/dashboard/models", "admin workspace", "/admin/#overview/analysis"},
		{"/system-settings/site", "admin workspace", "/admin/#settings"},
		{"/system-settings/auth", "admin workspace", "/admin/#settings"},
		{"/system-settings/security", "admin workspace", "/admin/#settings"},
		{"/system-settings/models", "admin workspace", "/admin/#settings"},
		{"/system-settings/billing", "admin workspace", "/admin/#settings"},
		{"/system-settings/content", "admin workspace", "/admin/#settings"},
		{"/system-settings/operations", "admin workspace", "/admin/#settings"},
		{"/system-info", "admin workspace", "/admin/#settings/system-info"},
		{"/system-settings/site/header-navigation", "admin workspace", "/admin/#settings"},
		{"/system-settings/site/sidebar-modules", "admin workspace", "/admin/#settings"},
		{"/system-settings/site/system-info", "admin workspace", "/admin/#settings/site-system-info"},
		{"/system-settings/content/announcements", "admin workspace", "/admin/#settings/announcements"},
		{"/system-settings/content/api-info", "admin workspace", "/admin/#settings/site-system-info"},
		{"/system-settings/content/faq", "admin workspace", "/admin/#settings/site-system-info"},
		{"/system-settings/content/uptime-kuma", "admin workspace", "/admin/#settings/system-info"},
		{"/system-settings/request-policies", "admin workspace", "/admin/#settings/routing"},
		{"/system-settings/site/notice", "admin workspace", "/admin/#settings/announcements"},
		{"/system-settings/request-policies/routing", "admin workspace", "/admin/#settings/routing"},
		{"/system-settings/billing/model-pricing", "admin workspace", "/admin/#models"},
		{"/system-settings/billing/group-pricing", "admin workspace", "/admin/#models"},
	} {
		t.Run(test.path, func(t *testing.T) {
			response := webResourceRequest(engine, http.MethodGet, test.path, freshWebResourceTestIP())
			require.Equal(t, http.StatusOK, response.Code)
			require.Empty(t, response.Header().Get("Location"))
			require.Contains(t, response.Body.String(), test.document)
			require.NotContains(t, response.Body.String(), "public home")
			require.Contains(t, response.Body.String(), "history.replaceState")
			require.Contains(t, response.Body.String(), test.destination)
		})
	}
}

func TestWorkspaceTargetKeepsQueryInsideEncodedFeaturePath(t *testing.T) {
	requestURL, err := url.Parse("/system-settings/security/ssrf?name=%3C%2Fscript%3E")
	require.NoError(t, err)
	document, target := workspaceDocumentTarget(requestURL)
	require.Equal(t, "admin/index.html", document)
	require.Contains(t, target, "%3Fname=")
	require.NotContains(t, string(workspaceNavigationScript(target)), "</script></script>")
	unknown, _ := url.Parse("/pricing/abc")
	document, target = workspaceDocumentTarget(unknown)
	require.Empty(t, document)
	require.Empty(t, target)
}

func TestWelcomeRouteServesItsOwnDocumentWithoutHomepageFlash(t *testing.T) {
	previous := common.GlobalWebRateLimitEnable
	common.GlobalWebRateLimitEnable = false
	t.Cleanup(func() { common.GlobalWebRateLimitEnable = previous })
	assets := fstest.MapFS{
		"web/dist/index.html":         {Data: []byte("<html><head></head><body>public home</body></html>")},
		"web/dist/welcome/index.html": {Data: []byte("<html><head></head><body>welcome application</body></html>")},
	}
	engine := gin.New()
	SetWebRouter(engine, WebAssets{BuildFS: assets}, nil)
	for _, pathname := range []string{"/welcome?next=%2Fconsole%2F%23tokens", "/welcome/"} {
		response := webResourceRequest(engine, http.MethodGet, pathname, freshWebResourceTestIP())
		require.Equal(t, http.StatusOK, response.Code)
		require.Empty(t, response.Header().Get("Location"))
		require.Contains(t, response.Body.String(), "welcome application")
		require.NotContains(t, response.Body.String(), "public home")
	}
}

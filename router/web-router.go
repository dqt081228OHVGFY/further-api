package router

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
)

// WebAssets holds the embedded dashboard frontend assets.
type WebAssets struct {
	BuildFS   fs.FS
	IndexPage []byte
}

func SetWebRouter(router *gin.Engine, assets WebAssets, pluginDispatcher gin.HandlerFunc) {
	if assets.BuildFS == nil {
		assets.BuildFS = embed.FS{}
	}
	frontendFS := common.EmbedFolder(assets.BuildFS, "web/dist")
	router.GET("/", middleware.RouteTag("web"), gzip.Gzip(gzip.DefaultCompression), middleware.GlobalWebRateLimit(), func(c *gin.Context) {
		page, err := fs.ReadFile(assets.BuildFS, "web/dist/index.html")
		if err != nil {
			page = assets.IndexPage
		}
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", relayForSeasonalHTML(page, time.Now()))
	})
	for _, route := range []string{"/admin", "/console", "/prepaid"} {
		router.GET(route, func(c *gin.Context) { c.Redirect(http.StatusTemporaryRedirect, c.Request.URL.Path+"/") })
		assetPath := "web/dist" + route + "/index.html"
		router.GET(route+"/", middleware.RouteTag("web"), gzip.Gzip(gzip.DefaultCompression), middleware.GlobalWebRateLimit(), func(c *gin.Context) {
			page, err := fs.ReadFile(assets.BuildFS, assetPath)
			if err != nil {
				c.Status(http.StatusNotFound)
				return
			}
			c.Header("Cache-Control", "no-cache")
			c.Data(http.StatusOK, "text/html; charset=utf-8", relayForSeasonalHTML(page, time.Now()))
		})
	}
	// RelayFor's original auth document owns both sign-in and registration.
	// Keep the New API route names as aliases so existing links and API 401
	// redirects still land on the restored template instead of the React shell.
	authPage, authErr := fs.ReadFile(assets.BuildFS, "web/dist/auth/index.html")
	serveAuth := func(c *gin.Context) {
		if authErr != nil {
			c.Status(http.StatusNotFound)
			return
		}
		page := authPage
		// /sign-up is an alias for the original document's register mode. The
		// page itself reads mode from the URL and keeps all existing animations.
		if c.Request.URL.Path == "/sign-up" || c.Request.URL.Path == "/sign-up/" {
			page = bytes.Replace(page, []byte(`<body data-page="auth"`), []byte(`<body data-page="auth" data-auth-mode="register"`), 1)
		}
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", relayForSeasonalHTML(page, time.Now()))
	}
	legacyAuth := func(c *gin.Context) {
		// /auth?mode=register is already the canonical template URL. The
		// aliases are served directly as well, so deep links remain stable.
		if c.Request.URL.Path == "/auth" {
			serveAuth(c)
			return
		}
		serveAuth(c)
	}
	router.GET("/auth", middleware.RouteTag("web"), gzip.Gzip(gzip.DefaultCompression), middleware.GlobalWebRateLimit(), legacyAuth)
	router.GET("/auth/", middleware.RouteTag("web"), gzip.Gzip(gzip.DefaultCompression), middleware.GlobalWebRateLimit(), legacyAuth)
	router.GET("/sign-in", middleware.RouteTag("web"), gzip.Gzip(gzip.DefaultCompression), middleware.GlobalWebRateLimit(), legacyAuth)
	router.GET("/sign-up", middleware.RouteTag("web"), gzip.Gzip(gzip.DefaultCompression), middleware.GlobalWebRateLimit(), legacyAuth)
	router.GET("/sign-in/", middleware.RouteTag("web"), gzip.Gzip(gzip.DefaultCompression), middleware.GlobalWebRateLimit(), legacyAuth)
	router.GET("/sign-up/", middleware.RouteTag("web"), gzip.Gzip(gzip.DefaultCompression), middleware.GlobalWebRateLimit(), legacyAuth)

	router.NoRoute(
		pluginDispatcher,
		middleware.RouteTag("web"),
		gzip.Gzip(gzip.DefaultCompression),
		middleware.AccessTokenAudit(),
		serveEmbeddedResources(frontendFS),
		middleware.GlobalWebRateLimit(),
		middleware.Cache(),
		serveFrontendFiles(frontendFS),
		func(c *gin.Context) {
			if backendWebPath(c.Request.URL.Path) || strings.HasPrefix(c.Request.URL.Path, "/assets") {
				controller.RelayNotFound(c)
				return
			}
			c.Header("Cache-Control", "no-cache")
			c.Data(http.StatusOK, "text/html; charset=utf-8", relayForSeasonalHTML(assets.IndexPage, time.Now()))
		},
	)
}

// Render the server-confirmed season into the document before stylesheets are
// parsed. This also works on a first visit with no local storage or JS cache.
func relayForSeasonalHTML(page []byte, now time.Time) []byte {
	if model.DB == nil {
		return page
	}
	campaigns, err := model.ListRelayForHolidayCampaigns()
	if err != nil {
		return page
	}
	for _, campaign := range campaigns {
		if _, _, active := model.RelayForHolidayWindow(campaign, now); active {
			// Campaign keys come exclusively from the server's fixed registry.
			return bytes.Replace(page, []byte("<html"), []byte(`<html data-holiday="`+campaign.Key+`"`), 1)
		}
	}
	return page
}

func backendWebPath(name string) bool {
	return strings.HasPrefix(name, "/api") || strings.HasPrefix(name, "/v1") || strings.HasPrefix(name, "/mj") || strings.HasPrefix(name, "/pg")
}

// Only existing regular resource files use this fast path. Page documents,
// missing files and backend URLs continue through their existing rate limits.
func serveEmbeddedResources(frontendFS http.FileSystem) gin.HandlerFunc {
	server := http.FileServer(frontendFS)
	return func(c *gin.Context) {
		name := c.Request.URL.Path
		if (c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead) || backendWebPath(name) {
			c.Next()
			return
		}
		switch strings.ToLower(path.Ext(name)) {
		case ".js", ".mjs", ".css", ".map", ".json", ".wasm", ".woff", ".woff2", ".ttf", ".otf", ".eot", ".svg", ".png", ".jpg", ".jpeg", ".gif", ".webp", ".avif", ".ico", ".mp4", ".webm", ".mp3", ".wav", ".ogg":
		default:
			c.Next()
			return
		}
		file, err := frontendFS.Open(name)
		if err != nil {
			c.Next()
			return
		}
		info, err := file.Stat()
		_ = file.Close()
		if err != nil || info.IsDir() {
			c.Next()
			return
		}
		middleware.CacheHeaders(c)
		server.ServeHTTP(c.Writer, c.Request)
		c.Abort()
	}
}

func serveFrontendFiles(frontendFS static.ServeFileSystem) gin.HandlerFunc {
	serve := static.Serve("/", frontendFS)
	return func(c *gin.Context) {
		if backendWebPath(c.Request.URL.Path) {
			c.Next()
			return
		}
		serve(c)
	}
}

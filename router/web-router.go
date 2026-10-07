package router

import (
	"bytes"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/controller"
	"github.com/dqt081228OHVGFY/further-api/middleware"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

// WebAssets holds the embedded dashboard frontend assets.
type WebAssets struct {
	BuildFS   fs.FS
	IndexPage []byte
}

func SetWebRouter(router *gin.Engine, assets WebAssets, pluginDispatcher gin.HandlerFunc) {
	frontend := newEmbeddedFrontend(assets)
	serveDocument := frontend.serveDocument
	for _, route := range []string{"/", "/admin", "/admin/", "/console", "/console/", "/prepaid", "/prepaid/", "/auth", "/auth/", "/sign-in", "/sign-in/", "/sign-up", "/sign-up/", "/welcome", "/welcome/"} {
		router.GET(route, middleware.RouteTag("web"), gzip.Gzip(gzip.DefaultCompression), middleware.GlobalWebRateLimit(), serveDocument)
		router.HEAD(route, middleware.RouteTag("web"), middleware.GlobalWebRateLimit(), serveDocument)
	}
	if pluginDispatcher == nil {
		pluginDispatcher = func(c *gin.Context) { c.Next() }
	}
	router.NoRoute(
		pluginDispatcher,
		middleware.RouteTag("web"),
		gzip.Gzip(gzip.DefaultCompression),
		middleware.AccessTokenAudit(),
		serveEmbeddedResources(frontend.files),
		middleware.GlobalWebRateLimit(),
		serveDocument,
	)
}

type embeddedFrontend struct {
	files http.FileSystem
	build fs.FS
	entry []byte
}

func newEmbeddedFrontend(assets WebAssets) *embeddedFrontend {
	frontend := &embeddedFrontend{build: assets.BuildFS, entry: assets.IndexPage}
	if assets.BuildFS != nil {
		frontend.files = common.EmbedFolder(assets.BuildFS, "web/dist")
		if len(bytes.TrimSpace(frontend.entry)) == 0 {
			if entry, err := fs.ReadFile(assets.BuildFS, "web/dist/index.html"); err == nil {
				frontend.entry = entry
			}
		}
	}
	return frontend
}

func (frontend *embeddedFrontend) readDocument(name string) ([]byte, bool) {
	if frontend.build == nil {
		return nil, false
	}
	page, err := fs.ReadFile(frontend.build, "web/dist/"+name)
	return page, err == nil && len(bytes.TrimSpace(page)) > 0
}

func (frontend *embeddedFrontend) serveDocument(c *gin.Context) {
	name := c.Request.URL.Path
	if (c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead) || backendWebPath(name) || frontendResourcePath(name) || path.Clean(name) != strings.TrimSuffix(name, "/") && name != "/" {
		controller.RelayNotFound(c)
		return
	}
	page := frontend.entry
	canonical := strings.Trim(name, "/")
	if canonical == "gateway.html" {
		controller.RelayNotFound(c)
		return
	}
	section, _, _ := strings.Cut(canonical, "/")
	if path.Ext(canonical) == ".html" {
		var found bool
		page, found = frontend.readDocument(canonical)
		if !found {
			controller.RelayNotFound(c)
			return
		}
	} else {
		if document, destination := workspaceDocumentTarget(c.Request.URL); document != "" {
			var found bool
			page, found = frontend.readDocument(document)
			if !found {
				frontend.unavailable(c)
				return
			}
			page = bytes.Replace(page, []byte("<head>"), append([]byte("<head>"), workspaceNavigationScript(destination)...), 1)
		}
		switch section {
		case "auth", "sign-in", "sign-up":
			var found bool
			page, found = frontend.readDocument("auth/index.html")
			if !found {
				frontend.unavailable(c)
				return
			}
			if section == "sign-up" {
				page = bytes.Replace(page, []byte(`<body data-page="auth"`), []byte(`<body data-page="auth" data-auth-mode="register"`), 1)
			}
		case "admin", "console", "prepaid", "welcome":
			var found bool
			page, found = frontend.readDocument(section + "/index.html")
			if !found {
				frontend.unavailable(c)
				return
			}
		}
	}
	if len(bytes.TrimSpace(page)) == 0 {
		frontend.unavailable(c)
		return
	}
	c.Header("Cache-Control", "no-cache")
	page = furtherSeasonalHTML(page, time.Now())
	if c.Request.Method == http.MethodHead {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.Status(http.StatusOK)
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", page)
}

func (frontend *embeddedFrontend) unavailable(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "code": "FRONTEND_UNAVAILABLE", "message": "further-api frontend assets are unavailable"})
}

func frontendResourcePath(name string) bool {
	if strings.HasPrefix(name, "/assets/") || strings.HasPrefix(name, "/static/") || strings.HasPrefix(name, "/further-assets/") {
		return true
	}
	ext := strings.ToLower(path.Ext(name))
	return ext != "" && ext != ".html"
}

// Render the server-confirmed season into the document before stylesheets are
// parsed. This also works on a first visit with no local storage or JS cache.
func furtherSeasonalHTML(page []byte, now time.Time) []byte {
	if model.DB == nil {
		return page
	}
	campaigns, err := model.ListFurtherHolidayCampaigns()
	if err != nil {
		return page
	}
	for _, campaign := range campaigns {
		if _, _, active := model.FurtherHolidayWindow(campaign, now); active {
			// Campaign keys come exclusively from the server's fixed registry.
			return bytes.Replace(page, []byte("<html"), []byte(`<html data-holiday="`+campaign.Key+`"`), 1)
		}
	}
	return page
}

func backendWebPath(name string) bool {
	for _, prefix := range []string{"/api", "/v1", "/v1beta", "/mj", "/pg", "/dashboard/billing"} {
		if name == prefix || strings.HasPrefix(name, prefix+"/") {
			return true
		}
	}
	_, suffix, found := strings.Cut(strings.TrimPrefix(name, "/"), "/")
	return found && (suffix == "mj" || strings.HasPrefix(suffix, "mj/"))
}

// Only existing regular resource files use this fast path. Page documents,
// missing files and backend URLs continue through their existing rate limits.
func serveEmbeddedResources(frontendFS http.FileSystem) gin.HandlerFunc {
	var server http.Handler
	if frontendFS != nil {
		server = http.FileServer(frontendFS)
	}
	return func(c *gin.Context) {
		name := c.Request.URL.Path
		if server == nil || (c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead) || backendWebPath(name) || path.Clean(name) != name {
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

package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/controller"
	"github.com/dqt081228OHVGFY/further-api/i18n"
	"github.com/dqt081228OHVGFY/further-api/logger"
	"github.com/dqt081228OHVGFY/further-api/middleware"
	"github.com/dqt081228OHVGFY/further-api/model"
	"github.com/dqt081228OHVGFY/further-api/oauth"
	"github.com/dqt081228OHVGFY/further-api/pkg/jsplugin"
	perfmetrics "github.com/dqt081228OHVGFY/further-api/pkg/perf_metrics"
	"github.com/dqt081228OHVGFY/further-api/pkg/wsmanager"
	"github.com/dqt081228OHVGFY/further-api/relay"
	kitutil "github.com/dqt081228OHVGFY/further-api/relaykit/relayconvert/kitutil"
	"github.com/dqt081228OHVGFY/further-api/router"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/dqt081228OHVGFY/further-api/service/authz"
	_ "github.com/dqt081228OHVGFY/further-api/setting/performance_setting"
	"github.com/dqt081228OHVGFY/further-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	_ "net/http/pprof"
)

//go:embed web/dist
var buildFS embed.FS

//go:embed web/dist/index.html
var indexPage []byte

func main() {
	if len(os.Args) > 1 && os.Args[1] == "plugin" {
		os.Exit(jsplugin.RunCLI(os.Args[2:], os.Stdout, os.Stderr))
	}
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	err := runGateway(ctx)
	stop()
	if err != nil {
		common.SysError("further-api exited: " + err.Error())
		os.Exit(1)
	}
}

func loadProcessEnvironment() error {
	if err := godotenv.Load(".env"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load .env: %w", err)
	}
	if err := common.LoadEnvironment(); err != nil {
		return fmt.Errorf("load environment: %w", err)
	}
	if _, err := common.ReadRuntimeConfig(); err != nil {
		return err
	}
	return nil
}

func runGateway(shutdown context.Context) (err error) {
	started := time.Now()
	if *common.PrintHelp {
		fmt.Println("further-api " + common.Version + " - AI model gateway")
		fmt.Println("Usage: further-api [--port <port>] [--log-dir <directory>] [--version] [--help]")
		return nil
	}
	if *common.PrintVersion {
		if version := os.Getenv("VERSION"); version != "" {
			fmt.Println(version)
		} else {
			fmt.Println(common.Version)
		}
		return nil
	}
	workers, cancelWorkers := context.WithCancel(context.Background())
	common.SetBackgroundContext(workers)
	defer cancelWorkers()
	kitutil.SetLogging(common.SysLog, func(message string) { logger.LogError(nil, message) })
	kitutil.SetSystemErrorLogging(common.SysError)
	if err := InitResources(); err != nil {
		return err
	}
	defer func() { err = errors.Join(err, gatewayResources.close()) }()
	cfg, err := common.ReadRuntimeConfig()
	if err != nil {
		return err
	}
	if os.Getenv("GIN_MODE") != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}
	kitutil.Debug.Store(common.DebugEnabled)
	handler, err := buildHTTPHandler()
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", cfg.Address)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Address, err)
	}
	defer listener.Close()
	defer func() {
		cancelWorkers()
		ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if waitErr := common.WaitBackgroundTasks(ctx); waitErr != nil {
			err = errors.Join(err, fmt.Errorf("wait for background tasks: %w", waitErr))
		}
		common.StopSystemMonitor()
		flushContext, stopFlush := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer stopFlush()
		if common.DataExportEnabled {
			if flushErr := model.FlushQuotaDataCacheContext(flushContext); flushErr != nil {
				err = errors.Join(err, fmt.Errorf("flush quota statistics: %w", flushErr))
			}
		}
		if flushErr := model.FlushBatchUpdatesContext(flushContext); flushErr != nil {
			err = errors.Join(err, fmt.Errorf("flush accounting updates: %w", flushErr))
		}
		err = errors.Join(err, common.StopPyroScope())
	}()
	if err := startGatewayWorkers(cfg); err != nil {
		return err
	}
	if err := common.StartPyroScope(); err != nil {
		common.SysError("start pyroscope: " + err.Error())
	}
	if cfg.PprofEnabled {
		startProfilingServer(cfg, workers)
	}
	srv := newHTTPServer(cfg, handler)
	common.SysLog("further-api " + common.Version + " started")
	if common.DebugEnabled {
		common.SysLog("running in debug mode")
	}
	return serveHTTP(shutdown, srv, listener, cfg.ShutdownTimeout, func() {
		common.LogStartupSuccess(started, cfg.Port)
	})
}

func newHTTPServer(cfg common.RuntimeConfig, handler http.Handler) *http.Server {
	return &http.Server{
		Addr: cfg.Address, Handler: handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		IdleTimeout:       cfg.IdleTimeout, MaxHeaderBytes: cfg.MaxHeaderBytes,
	}
}

// The listener is bound before workers start and readiness is announced.
// Read/write timeouts intentionally do not impose a lifetime on SSE streams.
func serveHTTP(ctx context.Context, server *http.Server, listener net.Listener, timeout time.Duration, ready func()) error {
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	if ready != nil {
		ready()
	}
	select {
	case err := <-served:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
		return nil
	case <-ctx.Done():
		common.SysLog("shutdown requested; waiting for active HTTP requests")
	}
	drain, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	shutdownErr := server.Shutdown(drain)
	if shutdownErr != nil {
		shutdownErr = errors.Join(shutdownErr, server.Close())
	}
	serveErr := <-served
	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	return errors.Join(shutdownErr, serveErr)
}

func buildHTTPHandler() (http.Handler, error) {
	server := gin.New()
	if err := middleware.ConfigureTrustedProxies(server); err != nil {
		return nil, fmt.Errorf("configure trusted proxies: %w", err)
	}
	server.Use(middleware.RequestId(), middleware.RelayPanicRecover(), middleware.SecurityHeaders(), middleware.Version(), middleware.I18n())
	middleware.SetUpLogger(server)
	InjectUmamiAnalytics()
	InjectGoogleAnalytics()
	router.SetRouter(server, router.WebAssets{BuildFS: buildFS, IndexPage: indexPage})
	return server, nil
}

func startGatewayWorkers(cfg common.RuntimeConfig) error {
	if common.RedisEnabled {
		common.MemoryCacheEnabled = true
	}
	if common.MemoryCacheEnabled {
		if err := initializeChannelCache(); err != nil {
			return err
		}
		common.StartBackgroundTask("channel cache sync", func(context.Context) { model.SyncChannelCache(common.SyncFrequency) })
	}
	wsmanager.StartSubscriber(common.BackgroundContext())
	model.GetPricing()
	common.StartBackgroundTask("option sync", func(context.Context) { model.SyncOptions(common.SyncFrequency) })
	common.StartBackgroundTask("task plugin sync", func(context.Context) { controller.SyncTaskPlugins() })
	common.StartBackgroundTask("authorization policy sync", func(context.Context) { authz.StartPolicySync(common.SyncFrequency) })
	common.StartBackgroundTask("quota statistics", func(context.Context) { model.UpdateQuotaData() })
	if cfg.ChannelUpdateFrequency > 0 {
		common.StartBackgroundTask("channel billing sync", func(context.Context) { controller.AutomaticallyUpdateChannels(cfg.ChannelUpdateFrequency) })
	}
	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor {
		adaptor := relay.GetTaskAdaptor(platform)
		if adaptor == nil {
			return nil
		}
		return adaptor
	}
	controller.RegisterScheduledSystemTasks()
	service.StartAuthArtifactCleanup()
	service.StartRiskObservationWorkers()
	service.StartCodexCredentialAutoRefreshTask()
	service.StartSubscriptionQuotaResetTask()
	service.StartSystemInstanceReporter()
	service.StartSystemTaskRunner()
	if cfg.BatchUpdateEnabled {
		common.BatchUpdateEnabled = true
		model.InitBatchUpdater()
		common.SysLog("batch update enabled with interval " + strconv.Itoa(common.BatchUpdateInterval) + "s")
	}
	perfmetrics.Init()
	common.StartSystemMonitor()
	return nil
}

func initializeChannelCache() error {
	initialize := func() (err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				err = fmt.Errorf("initialize channel cache: %v", recovered)
			}
		}()
		model.InitChannelCache()
		return nil
	}
	if err := initialize(); err != nil {
		common.SysError(err.Error())
		if _, _, repairErr := model.FixAbility(); repairErr != nil {
			return errors.Join(err, repairErr)
		}
		return initialize()
	}
	return nil
}

func startProfilingServer(cfg common.RuntimeConfig, ctx context.Context) {
	listener, err := net.Listen("tcp", cfg.PprofAddress)
	if err != nil {
		common.SysError("start pprof listener: " + err.Error())
		return
	}
	server := &http.Server{Handler: http.DefaultServeMux, ReadHeaderTimeout: cfg.ReadHeaderTimeout}
	common.StartBackgroundTask("pprof server", func(context.Context) {
		if err := serveHTTP(ctx, server, listener, cfg.ShutdownTimeout, nil); err != nil {
			common.SysError("pprof server: " + err.Error())
		}
	})
	common.StartBackgroundTask("CPU profiling", common.MonitorContext)
	common.SysLog("pprof enabled")
}

type resourceStep struct {
	name       string
	initialize func() error
	close      func() error
}

type resourceSet struct {
	steps []resourceStep
	once  sync.Once
	err   error
}

func (resources *resourceSet) close() error {
	if resources == nil {
		return nil
	}
	resources.once.Do(func() {
		for index := len(resources.steps) - 1; index >= 0; index-- {
			step := resources.steps[index]
			if step.close != nil {
				if err := step.close(); err != nil {
					resources.err = errors.Join(resources.err, fmt.Errorf("close %s: %w", step.name, err))
				}
			}
		}
	})
	return resources.err
}

func initializeResourcePlan(steps []resourceStep) (*resourceSet, error) {
	resources := &resourceSet{}
	for _, step := range steps {
		// Even a failed initializer can have opened a connection before its
		// later schema checks fail. Register its cleanup before invoking it.
		resources.steps = append(resources.steps, step)
		if err := step.initialize(); err != nil {
			return nil, errors.Join(fmt.Errorf("initialize %s: %w", step.name, err), resources.close())
		}
	}
	return resources, nil
}

var gatewayResources *resourceSet

// InitResources preserves the exported initialization entry point. It returns
// failures to the caller and rolls back opened resources in reverse order.
func InitResources() error {
	if err := loadProcessEnvironment(); err != nil {
		return err
	}
	logger.SetupLogger()
	ratio_setting.InitRatioSettings()
	service.InitHttpClient()
	service.InitTokenEncoders()
	resources, err := initializeResourcePlan([]resourceStep{
		{name: "database", initialize: model.InitDB, close: model.CloseDB},
		{name: "authorization", initialize: func() error { return authz.Init(model.DB) }},
		{name: "password encryption", initialize: func() error {
			if common.PasswordLoginEncryptionEnabled {
				return model.InitPasswordEncryption()
			}
			return nil
		}},
		{name: "application settings", initialize: func() error {
			model.CheckSetup()
			if common.IsMasterNode {
				if err := model.MigrateRetiredFrontendOptions(); err != nil {
					return err
				}
			}
			model.InitOptionMap()
			common.CleanupOldCacheFiles()
			return nil
		}},
		{name: "log database", initialize: model.InitLogDB},
		{name: "Redis", initialize: common.InitRedisClient, close: common.CloseRedisClient},
	})
	if err != nil {
		return err
	}
	gatewayResources = resources
	if err := i18n.Init(); err != nil {
		common.SysError("initialize translations: " + err.Error())
	}
	i18n.SetUserLangLoader(model.GetUserLanguage)
	if err := oauth.LoadCustomProviders(); err != nil {
		common.SysError("load custom OAuth providers: " + err.Error())
	}
	return nil
}

func injectAnalytics(page []byte, marker string, script string) []byte {
	if script == "" {
		return bytes.ReplaceAll(page, []byte(marker), nil)
	}
	if bytes.Contains(page, []byte(marker)) {
		return bytes.ReplaceAll(page, []byte(marker), []byte(script))
	}
	return bytes.Replace(page, []byte("</head>"), []byte(script+"</head>"), 1)
}

func umamiScript(getenv func(string) string) string {
	websiteID := getenv("UMAMI_WEBSITE_ID")
	if websiteID == "" {
		return ""
	}
	source := getenv("UMAMI_SCRIPT_URL")
	if source == "" {
		source = "https://analytics.umami.is/script.js"
	}
	parsed, err := url.Parse(source)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil {
		common.SysError("UMAMI_SCRIPT_URL must be an absolute HTTP or HTTPS URL")
		return ""
	}
	return `<script defer src="` + html.EscapeString(source) + `" data-website-id="` + html.EscapeString(websiteID) + `"></script>`
}

var googleAnalyticsID = regexp.MustCompile(`^(G|UA|AW|DC)-[A-Za-z0-9-]+$`)

func googleAnalyticsScript(getenv func(string) string) string {
	id := getenv("GOOGLE_ANALYTICS_ID")
	if id == "" {
		return ""
	}
	if !googleAnalyticsID.MatchString(id) {
		common.SysError("GOOGLE_ANALYTICS_ID is invalid")
		return ""
	}
	encodedID, _ := json.Marshal(id)
	return `<script async src="https://www.googletagmanager.com/gtag/js?id=` + url.QueryEscape(id) + `"></script><script>window.dataLayer=window.dataLayer||[];function gtag(){dataLayer.push(arguments);}gtag('js',new Date());gtag('config',` + string(encodedID) + `);</script>`
}

func InjectUmamiAnalytics() {
	indexPage = injectAnalytics(indexPage, "<!--umami-->", umamiScript(os.Getenv))
}

func InjectGoogleAnalytics() {
	indexPage = injectAnalytics(indexPage, "<!--Google Analytics-->", googleAnalyticsScript(os.Getenv))
}

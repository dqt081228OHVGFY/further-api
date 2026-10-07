package common

import (
	"context"
	"fmt"
	"math"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RuntimeConfig describes process-level settings. A zero relay timeout remains
// valid because a total request timeout would interrupt long SSE responses.
type RuntimeConfig struct {
	Address                string
	Port                   string
	ShutdownTimeout        time.Duration
	ReadHeaderTimeout      time.Duration
	IdleTimeout            time.Duration
	MaxHeaderBytes         int
	ChannelUpdateFrequency int
	BatchUpdateEnabled     bool
	PprofEnabled           bool
	PprofAddress           string
}

func ReadRuntimeConfig() (RuntimeConfig, error) {
	return readRuntimeConfig(os.Getenv, *Port)
}

func readRuntimeConfig(getenv func(string) string, cliPort int) (RuntimeConfig, error) {
	port := cliPort
	if raw := getenv("PORT"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return RuntimeConfig{}, fmt.Errorf("PORT must be an integer")
		}
		port = value
	}
	if port < 1 || port > 65535 {
		return RuntimeConfig{}, fmt.Errorf("listening port must be between 1 and 65535")
	}
	host := strings.TrimSpace(getenv("BIND_HOST"))
	if strings.ContainsAny(host, "/\\\r\n\t ") || (strings.Contains(host, ":") && net.ParseIP(host) == nil) {
		return RuntimeConfig{}, fmt.Errorf("BIND_HOST must be a hostname or IP address without a port")
	}
	cfg := RuntimeConfig{Port: strconv.Itoa(port), Address: net.JoinHostPort(host, strconv.Itoa(port)), PprofAddress: "127.0.0.1:8005"}
	var err error
	if cfg.ShutdownTimeout, err = runtimeSeconds(getenv, "SHUTDOWN_TIMEOUT_SECONDS", 120, 1); err != nil {
		return RuntimeConfig{}, err
	}
	if cfg.ReadHeaderTimeout, err = runtimeSeconds(getenv, "HTTP_READ_HEADER_TIMEOUT_SECONDS", 15, 0); err != nil {
		return RuntimeConfig{}, err
	}
	if cfg.IdleTimeout, err = runtimeSeconds(getenv, "HTTP_IDLE_TIMEOUT_SECONDS", 120, 0); err != nil {
		return RuntimeConfig{}, err
	}
	if cfg.MaxHeaderBytes, err = runtimeInteger(getenv, "HTTP_MAX_HEADER_BYTES", 1<<20, 1024); err != nil {
		return RuntimeConfig{}, err
	}
	if raw := getenv("CHANNEL_UPDATE_FREQUENCY"); raw != "" {
		cfg.ChannelUpdateFrequency, err = runtimeInteger(getenv, "CHANNEL_UPDATE_FREQUENCY", 0, 1)
		if err != nil {
			return RuntimeConfig{}, err
		}
	}
	if cfg.BatchUpdateEnabled, err = runtimeBoolean(getenv, "BATCH_UPDATE_ENABLED", false); err != nil {
		return RuntimeConfig{}, err
	}
	if cfg.PprofEnabled, err = runtimeBoolean(getenv, "ENABLE_PPROF", false); err != nil {
		return RuntimeConfig{}, err
	}
	return cfg, nil
}

func runtimeInteger(getenv func(string) string, name string, fallback, minimum int) (int, error) {
	raw := getenv(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < minimum {
		return 0, fmt.Errorf("%s must be an integer greater than or equal to %d", name, minimum)
	}
	return value, nil
}

func runtimeSeconds(getenv func(string) string, name string, fallback, minimum int) (time.Duration, error) {
	value, err := runtimeInteger(getenv, name, fallback, minimum)
	if err != nil {
		return 0, err
	}
	if int64(value) > math.MaxInt64/int64(time.Second) {
		return 0, fmt.Errorf("%s exceeds the supported duration", name)
	}
	return time.Duration(value) * time.Second, nil
}

func runtimeBoolean(getenv func(string) string, name string, fallback bool) (bool, error) {
	raw := getenv(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean", name)
	}
	return value, nil
}

// ValidateEnvironment prevents invalid timer and capacity settings from
// reaching worker loops or causing an overflow when converted to durations.
func ValidateEnvironment() error {
	for _, name := range []string{"FILE_FETCH_REQUEST_MAX_MB", "EXTERNAL_PAYLOAD_MEMORY_BUDGET_MB", "TASK_POLL_MAX_RESPONSE_MB"} {
		if _, err := runtimeInteger(os.Getenv, name, 1, 1); err != nil {
			return err
		}
	}
	for _, name := range []string{"FILE_FETCH_MAX_CONCURRENCY", "TASK_POLL_MAX_CONCURRENCY"} {
		if _, err := runtimeInteger(os.Getenv, name, 1, 1); err != nil {
			return err
		}
	}
	for _, name := range []string{"REQUEST_JSON_MAX_NODES", "REQUEST_JSON_MAX_DEPTH"} {
		if _, err := runtimeInteger(os.Getenv, name, 1, 1); err != nil {
			return err
		}
	}
	positive := []string{"SYNC_FREQUENCY", "BATCH_UPDATE_INTERVAL", "REDIS_POOL_SIZE", "GLOBAL_API_RATE_LIMIT", "GLOBAL_API_RATE_LIMIT_DURATION", "GLOBAL_WEB_RATE_LIMIT", "GLOBAL_WEB_RATE_LIMIT_DURATION", "CRITICAL_RATE_LIMIT", "CRITICAL_RATE_LIMIT_DURATION", "SEARCH_RATE_LIMIT", "SEARCH_RATE_LIMIT_DURATION", "MAX_FILE_DOWNLOAD_MB", "STREAM_SCANNER_MAX_BUFFER_MB", "MAX_REQUEST_BODY_MB", "ANONYMOUS_REQUEST_BODY_LIMIT_KB", "TASK_QUERY_LIMIT", "TASK_POLL_MAX_FAILURES", "TASK_PLUGIN_PROTOCOL_TIMEOUT_SECONDS", "TASK_PLUGIN_PROTOCOL_TICK_MILLISECONDS", "TASK_PLUGIN_PROTOCOL_HEARTBEAT_SECONDS"}
	for _, name := range positive {
		if _, err := runtimeInteger(os.Getenv, name, 1, 1); err != nil {
			return err
		}
	}
	nonnegative := []string{"RELAY_MAX_IDLE_CONNS", "RELAY_MAX_IDLE_CONNS_PER_HOST", "TASK_TIMEOUT_MINUTES", "TASK_PLUGIN_PROTOCOL_TICK_JITTER_MILLISECONDS", "PYROSCOPE_MUTEX_RATE", "PYROSCOPE_BLOCK_RATE"}
	for _, name := range nonnegative {
		if _, err := runtimeInteger(os.Getenv, name, 0, 0); err != nil {
			return err
		}
	}
	for _, name := range []string{"SYNC_FREQUENCY", "BATCH_UPDATE_INTERVAL", "RELAY_TIMEOUT", "RELAY_IDLE_CONN_TIMEOUT", "RELAY_RESPONSE_HEADER_TIMEOUT", "STREAMING_TIMEOUT", "POLLING_INTERVAL"} {
		if _, err := runtimeSeconds(os.Getenv, name, 0, 0); err != nil {
			return err
		}
	}
	return nil
}

var backgroundLifecycle = struct {
	sync.RWMutex
	ctx   context.Context
	tasks *backgroundTasks
}{ctx: context.Background(), tasks: newBackgroundTasks()}

type backgroundTasks struct {
	sync.Mutex
	count int
	done  chan struct{}
}

func newBackgroundTasks() *backgroundTasks {
	completed := make(chan struct{})
	close(completed)
	return &backgroundTasks{done: completed}
}

func (tasks *backgroundTasks) add() {
	tasks.Lock()
	defer tasks.Unlock()
	if tasks.count == 0 {
		tasks.done = make(chan struct{})
	}
	tasks.count++
}

func (tasks *backgroundTasks) finish() {
	tasks.Lock()
	defer tasks.Unlock()
	tasks.count--
	if tasks.count == 0 {
		close(tasks.done)
	}
}

// SetBackgroundContext is set before workers start. Requests keep their own
// contexts so active streams can drain before background workers are stopped.
func SetBackgroundContext(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	backgroundLifecycle.Lock()
	backgroundLifecycle.ctx = ctx
	backgroundLifecycle.tasks = newBackgroundTasks()
	backgroundLifecycle.Unlock()
}

func BackgroundContext() context.Context {
	backgroundLifecycle.RLock()
	defer backgroundLifecycle.RUnlock()
	return backgroundLifecycle.ctx
}

// StartBackgroundTask tracks owned workers so the database can stay open
// until their final writes complete. Callers must observe ctx cancellation.
func StartBackgroundTask(name string, task func(context.Context)) bool {
	backgroundLifecycle.RLock()
	ctx := backgroundLifecycle.ctx
	if task == nil || ctx.Err() != nil {
		backgroundLifecycle.RUnlock()
		return false
	}
	tasks := backgroundLifecycle.tasks
	tasks.add()
	backgroundLifecycle.RUnlock()
	go func() {
		defer tasks.finish()
		defer func() {
			if recovered := recover(); recovered != nil {
				SysError(fmt.Sprintf("background task %s panicked: %v", name, recovered))
			}
		}()
		task(ctx)
	}()
	return true
}

// WaitBackgroundTasks is called after cancelling BackgroundContext and before
// closing shared resources. Startup must be complete before waiting.
func WaitBackgroundTasks(ctx context.Context) error {
	// Synchronize with a starter that checked the context just before the owner
	// cancelled it. Registration must finish before we observe an empty group.
	backgroundLifecycle.Lock()
	tasks := backgroundLifecycle.tasks
	tasks.Lock()
	done := tasks.done
	tasks.Unlock()
	backgroundLifecycle.Unlock()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

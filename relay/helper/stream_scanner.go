package helper

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/dqt081228OHVGFY/further-api/constant"
	"github.com/dqt081228OHVGFY/further-api/logger"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/dqt081228OHVGFY/further-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

const (
	InitialScannerBufferSize    = 64 << 10
	DefaultMaxScannerBufferSize = 128 << 20
	DefaultPingInterval         = 10 * time.Second
	streamWriteTimeout          = 30 * time.Second
)

func getScannerBufferSize() int {
	mb := constant.StreamScannerMaxBufferMB
	if mb > 0 && mb <= math.MaxInt>>20 {
		return mb << 20
	}
	return DefaultMaxScannerBufferSize
}

func NewStreamScanner(reader io.Reader, maxBytes ...int) *bufio.Scanner {
	limit := getScannerBufferSize()
	if len(maxBytes) > 0 && maxBytes[0] > 0 {
		limit = min(limit, maxBytes[0])
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, min(InitialScannerBufferSize, limit)), limit)
	return scanner
}

func copyCodexSSEHeaders(c *gin.Context, resp *http.Response) {
	if c == nil || c.Writer == nil || resp == nil {
		return
	}
	for _, name := range []string{"X-Reasoning-Included", "X-Codex-Turn-State"} {
		values := resp.Header.Values(name)
		if !service.ShouldCopyUpstreamHeader(c, name, values) {
			continue
		}
		for _, value := range values {
			if value != "" {
				c.Writer.Header().Add(name, value)
			}
		}
	}
}

func ExtendWriteDeadline(c *gin.Context) {
	if c != nil && c.Writer != nil {
		_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(streamWriteTimeout))
	}
}

type upstreamStreamFrame struct {
	line  string
	err   error
	end   bool
	panic bool
}

// The reader never touches Gin, relay state, or the downstream writer. One
// buffered line provides backpressure without growing a queue of token chunks.
func readUpstreamStream(ctx context.Context, body io.Reader, frames chan<- upstreamStreamFrame, done chan<- struct{}) {
	defer close(done)
	defer close(frames)
	deliver := func(frame upstreamStreamFrame) bool {
		select {
		case frames <- frame:
			return true
		case <-ctx.Done():
			return false
		}
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			deliver(upstreamStreamFrame{err: fmt.Errorf("upstream stream reader panic: %v", recovered), end: true, panic: true})
		}
	}()
	scanner := NewStreamScanner(body)
	for scanner.Scan() {
		if !deliver(upstreamStreamFrame{line: scanner.Text()}) {
			return
		}
	}
	deliver(upstreamStreamFrame{err: scanner.Err(), end: true})
}

func streamSeconds(seconds int, fallback time.Duration) time.Duration {
	if seconds <= 0 {
		return fallback
	}
	return time.Duration(min(int64(seconds), math.MaxInt64/int64(time.Second))) * time.Second
}

func resetStreamIdleTimer(timer *time.Timer, duration time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(duration)
}

// A single loop owns all response writes, heartbeat writes, usage counters,
// callbacks and end-state changes. EOF is delivered after earlier frames,
// so fast providers cannot terminate the loop before buffered chunks settle.
func StreamScannerHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, dataHandler func(string, *StreamResult)) {
	if c == nil || c.Request == nil || c.Writer == nil || resp == nil || resp.Body == nil || info == nil || dataHandler == nil {
		return
	}
	info.StreamStatus = relaycommon.NewStreamStatus()
	status := info.StreamStatus
	ctx, cancel := context.WithCancel(c.Request.Context())
	frames := make(chan upstreamStreamFrame, 1)
	readerDone := make(chan struct{})
	idleTimeout := streamSeconds(constant.StreamingTimeout, 5*time.Minute)
	idle := time.NewTimer(idleTimeout)
	setting := operation_setting.GetGeneralSetting()
	var heartbeat *time.Ticker
	var ping <-chan time.Time
	if setting.PingIntervalEnabled && !info.DisablePing {
		heartbeat = time.NewTicker(streamSeconds(setting.PingIntervalSeconds, DefaultPingInterval))
		ping = heartbeat.C
	}
	go readUpstreamStream(ctx, resp.Body, frames, readerDone)
	defer func() {
		cancel()
		_ = resp.Body.Close()
		<-readerDone
		idle.Stop()
		if heartbeat != nil {
			heartbeat.Stop()
		}
		if recovered := recover(); recovered != nil {
			status.SetEndReason(relaycommon.StreamEndReasonPanic, fmt.Errorf("stream callback panic: %v", recovered))
		}
		_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Time{})
		if status.IsNormalEnd() && !status.HasErrors() {
			logger.LogInfo(c, "stream ended: "+status.Summary())
		} else {
			logger.LogError(c, fmt.Sprintf("stream ended: %s, received=%d", status.Summary(), info.ReceivedResponseCount))
		}
	}()
	if ctx.Err() != nil {
		status.SetEndReason(relaycommon.StreamEndReasonClientGone, ctx.Err())
		return
	}
	copyCodexSSEHeaders(c, resp)
	SetEventStreamHeaders(c)
	result := newStreamResult(status)
	for {
		// Prefer cancellation even when the provider has filled the line buffer.
		if ctx.Err() != nil {
			status.SetEndReason(relaycommon.StreamEndReasonClientGone, ctx.Err())
			return
		}
		select {
		case <-ctx.Done():
			status.SetEndReason(relaycommon.StreamEndReasonClientGone, ctx.Err())
			return
		case <-idle.C:
			status.SetEndReason(relaycommon.StreamEndReasonTimeout, context.DeadlineExceeded)
			return
		case <-ping:
			ExtendWriteDeadline(c)
			if err := PingData(c); err != nil {
				status.SetEndReason(relaycommon.StreamEndReasonPingFail, err)
				return
			}
		case frame, open := <-frames:
			if ctx.Err() != nil {
				status.SetEndReason(relaycommon.StreamEndReasonClientGone, ctx.Err())
				return
			}
			if !open || frame.end {
				reason := relaycommon.StreamEndReasonEOF
				if frame.err != nil {
					reason = relaycommon.StreamEndReasonScannerErr
				}
				if frame.panic {
					reason = relaycommon.StreamEndReasonPanic
				}
				status.SetEndReason(reason, frame.err)
				return
			}
			resetStreamIdleTimer(idle, idleTimeout)
			line := strings.TrimSpace(frame.line)
			if line == "[DONE]" {
				status.SetEndReason(relaycommon.StreamEndReasonDone, nil)
				return
			}
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "" {
				continue
			}
			if data == "[DONE]" {
				status.SetEndReason(relaycommon.StreamEndReasonDone, nil)
				return
			}
			info.SetFirstResponseTime()
			info.ReceivedResponseCount++
			result.reset()
			ExtendWriteDeadline(c)
			dataHandler(data, result)
			if result.IsStopped() {
				return
			}
			resetStreamIdleTimer(idle, idleTimeout)
		}
	}
}

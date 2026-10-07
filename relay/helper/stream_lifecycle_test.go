package helper

import (
	"io"
	"strings"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/constant"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/stretchr/testify/require"
)

func TestStreamCallbackPanicClosesUpstreamAndStopsLaterChunks(t *testing.T) {
	c, response, info := setupStreamTest(t, strings.NewReader(buildSSEBody(20)))
	count := 0
	require.NotPanics(t, func() {
		StreamScannerHandler(c, response, info, func(string, *StreamResult) {
			count++
			panic("callback failed")
		})
	})
	require.Equal(t, 1, count)
	require.Equal(t, relaycommon.StreamEndReasonPanic, info.StreamStatus.EndReason)
}

func TestStreamBareDoneStopsBeforeFollowingChunks(t *testing.T) {
	c, response, info := setupStreamTest(t, strings.NewReader("data: first\n[DONE]\ndata: second\n"))
	var chunks []string
	StreamScannerHandler(c, response, info, func(data string, _ *StreamResult) { chunks = append(chunks, data) })
	require.Equal(t, []string{"first"}, chunks)
	require.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
}

func TestStreamZeroTimeoutDoesNotPanic(t *testing.T) {
	previous := constant.StreamingTimeout
	constant.StreamingTimeout = 0
	t.Cleanup(func() { constant.StreamingTimeout = previous })
	c, response, info := setupStreamTest(t, strings.NewReader(""))
	response.Body = io.NopCloser(strings.NewReader(""))
	require.NotPanics(t, func() { StreamScannerHandler(c, response, info, func(string, *StreamResult) {}) })
	require.Equal(t, relaycommon.StreamEndReasonEOF, info.StreamStatus.EndReason)
}

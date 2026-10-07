package volcengine

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dqt081228OHVGFY/further-api/constant"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	relayconstant "github.com/dqt081228OHVGFY/further-api/relay/constant"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func volcTestContext(t *testing.T, base string) (*gin.Context, *relaycommon.RelayInfo, *httptest.ResponseRecorder) {
	t.Helper()
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", nil)
	info := &relaycommon.RelayInfo{UserId: 7, OriginModelName: "speech-model", RelayFormat: types.RelayFormatOpenAIAudio,
		RelayMode: relayconstant.RelayModeAudioSpeech, ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 1,
			ChannelType: constant.ChannelTypeVolcEngine, ChannelBaseUrl: base, ApiKey: "app|test-token", UpstreamModelName: "speech-model"}}
	info.SetEstimatePromptTokens(12)
	return c, info, writer
}

func TestVolcengineCodecPreservesSequenceAndCompressionAndRejectsTruncation(t *testing.T) {
	frame, err := NewMessage(MsgTypeAudioOnlyServer, MsgTypeFlagNegativeSeq)
	require.NoError(t, err)
	frame.Sequence, frame.Compression, frame.Serialization, frame.Payload = -4, 1, 0, []byte("audio fixture")
	encoded, err := frame.Marshal()
	require.NoError(t, err)
	decoded, err := NewMessageFromBytes(encoded)
	require.NoError(t, err)
	require.EqualValues(t, 0, decoded.Serialization)
	require.EqualValues(t, 1, decoded.Compression)
	require.EqualValues(t, -4, decoded.Sequence)
	require.Equal(t, frame.Payload, decoded.Payload)
	for length := range len(encoded) {
		_, err := NewMessageFromBytes(encoded[:length])
		require.Error(t, err, "every truncated declared frame must fail")
	}
	oversized := append([]byte(nil), encoded...)
	binary.BigEndian.PutUint32(oversized[8:12], uint32(maxVolcengineFramePayload+1))
	_, err = NewMessageFromBytes(oversized)
	require.Error(t, err)
}

func TestVolcengineCodecPreservesConnectionIdentifiers(t *testing.T) {
	frame, err := NewMessage(MsgTypeFullServerResponse, MsgTypeFlagWithEvent)
	require.NoError(t, err)
	frame.EventType, frame.ConnectID, frame.Payload = EventType_ConnectionStarted, "connection-7", []byte("{}")
	encoded, err := frame.Marshal()
	require.NoError(t, err)
	decoded, err := NewMessageFromBytes(encoded)
	require.NoError(t, err)
	require.Equal(t, "connection-7", decoded.ConnectID)
	require.Equal(t, EventType_ConnectionStarted, decoded.EventType)
}

func TestVolcengineHTTPQueryFreezesIdentityAndBilledTextAndPreservesOpus(t *testing.T) {
	seen := make(chan VolcengineTTSRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer;test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var submitted VolcengineTTSRequest
		if json.NewDecoder(request.Body).Decode(&submitted) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		seen <- submitted
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(VolcengineTTSResponse{Code: 3000, Data: base64.StdEncoding.EncodeToString([]byte("opus fixture"))})
	}))
	defer server.Close()
	c, info, writer := volcTestContext(t, server.URL)
	adapter := &Adaptor{}
	request := dto.AudioRequest{Input: "original billed text", Voice: "alloy", ResponseFormat: "opus",
		Metadata: json.RawMessage("{\"app\":{\"appid\":\"foreign\",\"token\":\"foreign\",\"cluster\":\"custom\"},\"request\":{\"operation\":\"query\",\"text\":\"unbilled text\"}}")}
	body, err := adapter.ConvertAudioRequest(c, info, request)
	require.NoError(t, err)
	require.False(t, info.IsStream)
	raw, err := adapter.DoRequest(c, info, body)
	require.NoError(t, err)
	usage, failure := adapter.DoResponse(c, raw.(*http.Response), info)
	require.Nil(t, failure)
	require.Equal(t, 12, usage.(*dto.Usage).TotalTokens)
	require.Equal(t, "opus fixture", writer.Body.String())
	require.Equal(t, "audio/ogg", writer.Header().Get("Content-Type"))
	submitted := <-seen
	require.Equal(t, "app", submitted.App.AppID)
	require.Equal(t, "test-token", submitted.App.Token)
	require.Equal(t, "custom", submitted.App.Cluster)
	require.Equal(t, "further-api-user-7", submitted.User.UID)
	require.Equal(t, "original billed text", submitted.Request.Text)
	require.Equal(t, 1.0, submitted.Audio.SpeedRatio)
}

func TestVolcengineWebSocketConsumesFinalCompressedAudioBeforeClosing(t *testing.T) {
	seen := make(chan VolcengineTTSRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		socket, err := (&websocket.Upgrader{}).Upgrade(w, request, nil)
		if err != nil {
			return
		}
		defer socket.Close()
		_, raw, err := socket.ReadMessage()
		if err != nil {
			return
		}
		message, err := NewMessageFromBytes(raw)
		if err != nil {
			return
		}
		var submitted VolcengineTTSRequest
		if json.Unmarshal(message.Payload, &submitted) != nil {
			return
		}
		seen <- submitted
		first, _ := NewMessage(MsgTypeAudioOnlyServer, MsgTypeFlagPositiveSeq)
		first.Sequence, first.Payload = 1, []byte("first-")
		last, _ := NewMessage(MsgTypeAudioOnlyServer, MsgTypeFlagNegativeSeq)
		last.Sequence, last.Payload, last.Compression = -2, []byte("last"), 1
		for _, message := range []*Message{first, last} {
			frame, err := message.Marshal()
			if err != nil || socket.WriteMessage(websocket.BinaryMessage, frame) != nil {
				return
			}
		}
	}))
	defer server.Close()
	c, info, writer := volcTestContext(t, strings.Replace(server.URL, "http://", "ws://", 1))
	adapter := &Adaptor{}
	body, err := adapter.ConvertAudioRequest(c, info, dto.AudioRequest{Input: "hello", Voice: "nova", ResponseFormat: "mp3"})
	require.NoError(t, err)
	require.True(t, info.IsStream)
	raw, err := adapter.DoRequest(c, info, body)
	require.NoError(t, err)
	transport := raw.(*http.Response)
	defer transport.Body.Close()
	usage, failure := adapter.DoResponse(c, transport, info)
	require.Nil(t, failure)
	require.Equal(t, "first-last", writer.Body.String())
	require.Equal(t, "audio/mpeg", writer.Header().Get("Content-Type"))
	require.Equal(t, 12, usage.(*dto.Usage).TotalTokens)
	require.Equal(t, "completed", info.StreamStatus.ResponseOutcome())
	require.Equal(t, "submit", (<-seen).Request.Operation)
}

func TestVolcengineCancellationStopsBlockedReaderAndClosesSocket(t *testing.T) {
	ready, closed := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		socket, err := (&websocket.Upgrader{}).Upgrade(w, request, nil)
		if err != nil {
			return
		}
		defer socket.Close()
		_, _, _ = socket.ReadMessage()
		close(ready)
		_, _, _ = socket.ReadMessage()
		close(closed)
	}))
	defer server.Close()
	c, info, writer := volcTestContext(t, strings.Replace(server.URL, "http://", "ws://", 1))
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	adapter := &Adaptor{}
	body, err := adapter.ConvertAudioRequest(c, info, dto.AudioRequest{Input: "hello", Voice: "alloy"})
	require.NoError(t, err)
	raw, err := adapter.DoRequest(c, info, body)
	require.NoError(t, err)
	<-ready
	completed := make(chan *types.GatewayError, 1)
	go func() { _, failure := adapter.DoResponse(c, raw.(*http.Response), info); completed <- failure }()
	cancel()
	select {
	case failure := <-completed:
		require.ErrorIs(t, failure, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled speech reader did not stop")
	}
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled speech socket remained open")
	}
	require.Empty(t, writer.Body.String())
	require.Equal(t, "cancelled", info.StreamStatus.ResponseOutcome())
}

func TestVolcengineErrorFrameDoesNotBecomeSuccessfulAudioTermination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		socket, err := (&websocket.Upgrader{}).Upgrade(w, request, nil)
		if err != nil {
			return
		}
		defer socket.Close()
		_, _, _ = socket.ReadMessage()
		first, _ := NewMessage(MsgTypeAudioOnlyServer, MsgTypeFlagPositiveSeq)
		first.Sequence, first.Payload = 1, []byte("partial-audio")
		rejected, _ := NewMessage(MsgTypeError, MsgTypeFlagNoSeq)
		rejected.ErrorCode, rejected.Payload = 3010, []byte("synthesis failed")
		for _, message := range []*Message{first, rejected} {
			frame, err := message.Marshal()
			if err != nil || socket.WriteMessage(websocket.BinaryMessage, frame) != nil {
				return
			}
		}
	}))
	defer server.Close()
	c, info, writer := volcTestContext(t, strings.Replace(server.URL, "http://", "ws://", 1))
	adapter := &Adaptor{}
	body, err := adapter.ConvertAudioRequest(c, info, dto.AudioRequest{Input: "hello", Voice: "alloy"})
	require.NoError(t, err)
	raw, err := adapter.DoRequest(c, info, body)
	require.NoError(t, err)
	usage, failure := adapter.DoResponse(c, raw.(*http.Response), info)
	require.ErrorContains(t, failure, "3010")
	require.Equal(t, "partial-audio", writer.Body.String())
	require.NotNil(t, usage)
	require.True(t, info.StreamStatus.ResponseFailed())
}

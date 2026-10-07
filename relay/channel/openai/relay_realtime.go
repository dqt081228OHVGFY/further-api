package openai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/dqt081228OHVGFY/further-api/common"
	relaycommon "github.com/dqt081228OHVGFY/further-api/relay/common"
	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/dqt081228OHVGFY/further-api/service"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/tidwall/gjson"
)

type realtimePacket struct {
	client bool
	kind   int
	body   []byte
	err    error
}
type realtimeLedger struct {
	total     dto.RealtimeUsage
	pending   dto.RealtimeUsage
	responses map[string]*dto.RealtimeUsage
	completed map[string]struct{}
	active    string
	sequence  int
}

func newRealtimeLedger() *realtimeLedger {
	return &realtimeLedger{responses: make(map[string]*dto.RealtimeUsage), completed: make(map[string]struct{})}
}
func realtimeSnapshot(value *dto.RealtimeUsage) *dto.RealtimeUsage {
	if value == nil {
		return &dto.RealtimeUsage{}
	}
	copy := *value
	copy.InputTokenDetails = value.InputTokenDetails.Clone()
	return &copy
}
func addRealtimeUsage(total, value *dto.RealtimeUsage) {
	total.InputTokens += value.InputTokens
	total.OutputTokens += value.OutputTokens
	total.TotalTokens = total.InputTokens + total.OutputTokens
	total.InputTokenDetails.CachedTokens += value.InputTokenDetails.CachedTokens
	total.InputTokenDetails.TextTokens += value.InputTokenDetails.TextTokens
	total.InputTokenDetails.AudioTokens += value.InputTokenDetails.AudioTokens
	total.OutputTokenDetails.TextTokens += value.OutputTokenDetails.TextTokens
	total.OutputTokenDetails.AudioTokens += value.OutputTokenDetails.AudioTokens
}
func (ledger *realtimeLedger) response(id string) *dto.RealtimeUsage {
	if id == "" {
		id = ledger.active
	}
	if _, done := ledger.completed[id]; id != "" && done {
		return &dto.RealtimeUsage{}
	}
	if id == "" {
		ledger.sequence++
		id = fmt.Sprintf("local-%d", ledger.sequence)
	}
	if value := ledger.responses[id]; value != nil {
		ledger.active = id
		return value
	}
	value := realtimeSnapshot(&ledger.pending)
	ledger.pending = dto.RealtimeUsage{}
	ledger.responses[id] = value
	ledger.active = id
	return value
}
func (ledger *realtimeLedger) observed() *dto.RealtimeUsage {
	result := realtimeSnapshot(&ledger.total)
	for id, value := range ledger.responses {
		if _, done := ledger.completed[id]; !done {
			addRealtimeUsage(result, value)
		}
	}
	return result
}

func (ledger *realtimeLedger) finish(c *gin.Context, info *relaycommon.RelayInfo, event dto.RealtimeEvent, id string) error {
	if info == nil || !info.HasChannelMeta() {
		return errors.New("realtime channel metadata is missing")
	}
	if event.Response == nil {
		return errors.New("realtime completion is missing its response")
	}
	if id == "" {
		id = ledger.active
	}
	identity := id
	if identity == "" {
		identity = event.EventId
	}
	if _, exists := ledger.completed[identity]; identity != "" && exists {
		return nil
	}
	estimated := ledger.response(id)
	if id == "" {
		id = ledger.active
	}
	if identity == "" {
		identity = id
	}
	usage := realtimeSnapshot(event.Response.Usage)
	if event.Response.Usage == nil {
		usage = realtimeSnapshot(estimated)
		text, audio, err := service.CountTokenRealtime(info, event, info.UpstreamModelName)
		if err != nil {
			return errors.New("realtime response usage could not be measured")
		}
		usage.InputTokens += text + audio
		usage.InputTokenDetails.TextTokens += text
		usage.InputTokenDetails.AudioTokens += audio
	}
	for _, count := range []int{usage.TotalTokens, usage.InputTokens, usage.OutputTokens, usage.InputTokenDetails.CachedTokens, usage.InputTokenDetails.TextTokens, usage.InputTokenDetails.AudioTokens, usage.OutputTokenDetails.TextTokens, usage.OutputTokenDetails.AudioTokens} {
		if count < 0 {
			return errors.New("realtime provider reported invalid token usage")
		}
	}
	if usage.InputTokens == 0 {
		usage.InputTokens = usage.InputTokenDetails.TextTokens + usage.InputTokenDetails.AudioTokens
	}
	if usage.OutputTokens == 0 {
		usage.OutputTokens = usage.OutputTokenDetails.TextTokens + usage.OutputTokenDetails.AudioTokens
	}
	if usage.InputTokens+usage.OutputTokens == 0 && usage.TotalTokens > 0 {
		usage.InputTokens = usage.TotalTokens
	}
	if usage.InputTokenDetails.TextTokens+usage.InputTokenDetails.AudioTokens == 0 {
		usage.InputTokenDetails.TextTokens = usage.InputTokens
	}
	if usage.OutputTokenDetails.TextTokens+usage.OutputTokenDetails.AudioTokens == 0 {
		usage.OutputTokenDetails.TextTokens = usage.OutputTokens
	}
	usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	addRealtimeUsage(&ledger.total, usage)
	ledger.completed[identity] = struct{}{}
	ledger.completed[id] = struct{}{}
	delete(ledger.responses, id)
	if ledger.active == id {
		ledger.active = ""
	}
	if err := service.PreWssConsumeQuota(c, info, usage); err != nil {
		return err
	}
	info.IsFirstRequest = false
	return nil
}

func readRealtimePackets(ctx context.Context, socket *websocket.Conn, client bool, packets chan<- realtimePacket, wait *sync.WaitGroup) {
	defer wait.Done()
	send := func(packet realtimePacket) {
		select {
		case packets <- packet:
		case <-ctx.Done():
		}
	}
	defer func() {
		if recover() != nil {
			send(realtimePacket{client: client, err: errors.New("realtime socket reader failed")})
		}
	}()
	socket.SetReadLimit(32 << 20)
	for {
		kind, body, err := socket.ReadMessage()
		select {
		case <-ctx.Done():
			return
		default:
		}
		send(realtimePacket{client: client, kind: kind, body: body, err: err})
		if err != nil {
			return
		}
	}
}
func writeRealtimePacket(ctx context.Context, socket *websocket.Conn, packet realtimePacket) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	deadline := time.Now().Add(10 * time.Second)
	if limit, ok := ctx.Deadline(); ok && limit.Before(deadline) {
		deadline = limit
	}
	if err := socket.SetWriteDeadline(deadline); err != nil {
		return err
	}
	return socket.WriteMessage(packet.kind, packet.body)
}
func closeRealtimePeer(socket *websocket.Conn, code int) {
	_ = socket.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, "session closed"), time.Now().Add(time.Second))
}

func OpenaiRealtimeHandler(c *gin.Context, info *relaycommon.RelayInfo) (*types.GatewayError, *dto.RealtimeUsage) {
	if c == nil || c.Request == nil || info == nil || !info.HasChannelMeta() || info.ClientWs == nil || info.TargetWs == nil || info.ClientWs == info.TargetWs {
		return types.NewOpenAIError(errors.New("invalid realtime connection context"), types.ErrorCodeBadResponse, http.StatusBadGateway), nil
	}
	ctx, cancel := context.WithCancel(c.Request.Context())
	client, upstream := info.ClientWs, info.TargetWs
	closeSockets := func() { _ = client.Close(); _ = upstream.Close() }
	stop := context.AfterFunc(ctx, closeSockets)
	packets := make(chan realtimePacket, 32)
	var readers sync.WaitGroup
	readers.Add(2)
	go readRealtimePackets(ctx, client, true, packets, &readers)
	go readRealtimePackets(ctx, upstream, false, packets, &readers)
	defer func() { cancel(); closeSockets(); readers.Wait(); stop() }()
	info.IsStream = true
	ledger := newRealtimeLedger()
	failure := func(err error) (*types.GatewayError, *dto.RealtimeUsage) {
		closeRealtimePeer(client, websocket.CloseInternalServerErr)
		return types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusBadGateway), ledger.observed()
	}
	for {
		select {
		case <-ctx.Done():
			return nil, ledger.observed()
		case packet := <-packets:
			if packet.err != nil {
				if websocket.IsCloseError(packet.err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived) || ctx.Err() != nil {
					peer := client
					if packet.client {
						peer = upstream
					}
					closeRealtimePeer(peer, websocket.CloseNormalClosure)
					return nil, ledger.observed()
				}
				return failure(errors.New("realtime connection ended unexpectedly"))
			}
			var event dto.RealtimeEvent
			if err := common.Unmarshal(packet.body, &event); err != nil {
				return failure(errors.New("realtime event is not valid JSON"))
			}
			if event.Session != nil {
				if event.Session.Tools != nil {
					info.RealtimeTools = append([]dto.RealTimeTool(nil), event.Session.Tools...)
				}
				if event.Session.InputAudioFormat != "" {
					info.InputAudioFormat = event.Session.InputAudioFormat
				}
				if event.Session.OutputAudioFormat != "" {
					info.OutputAudioFormat = event.Session.OutputAudioFormat
				}
			}
			target := client
			if packet.client {
				target = upstream
				metering := event
				if metering.Type == dto.RealtimeEventTypeConversationCreate {
					metering.Type = dto.RealtimeEventConversationItemCreated
				}
				text, audio, err := service.CountTokenRealtime(info, metering, info.UpstreamModelName)
				if err != nil {
					return failure(errors.New("realtime input usage could not be measured"))
				}
				ledger.pending.InputTokens += text + audio
				ledger.pending.TotalTokens = ledger.pending.InputTokens
				ledger.pending.InputTokenDetails.TextTokens += text
				ledger.pending.InputTokenDetails.AudioTokens += audio
			} else {
				info.SetFirstResponseTime()
				id := gjson.GetBytes(packet.body, "response.id").String()
				if id == "" {
					id = gjson.GetBytes(packet.body, "response_id").String()
				}
				switch event.Type {
				case "response.created":
					ledger.response(id)
				case dto.RealtimeEventTypeResponseDone:
					if err := ledger.finish(c, info, event, id); err != nil {
						return failure(err)
					}
				case dto.RealtimeEventTypeSessionCreated, dto.RealtimeEventTypeSessionUpdated, dto.RealtimeEventConversationItemCreated:
				default:
					metering := event
					if metering.Type == "response.output_audio.delta" {
						metering.Type = dto.RealtimeEventResponseAudioDelta
					}
					if metering.Type == "response.output_audio_transcript.delta" {
						metering.Type = dto.RealtimeEventResponseAudioTranscriptionDelta
					}
					text, audio, err := service.CountTokenRealtime(info, metering, info.UpstreamModelName)
					if err != nil {
						return failure(errors.New("realtime output usage could not be measured"))
					}
					if event.Type == "response.output_text.delta" || event.Type == "response.text.delta" {
						text = service.CountTextToken(event.Delta, info.UpstreamModelName)
					}
					if text+audio > 0 {
						estimate := ledger.response(id)
						estimate.OutputTokens += text + audio
						estimate.TotalTokens = estimate.InputTokens + estimate.OutputTokens
						estimate.OutputTokenDetails.TextTokens += text
						estimate.OutputTokenDetails.AudioTokens += audio
					}
				}
			}
			if err := writeRealtimePacket(ctx, target, packet); err != nil {
				if ctx.Err() != nil {
					return nil, ledger.observed()
				}
				return failure(errors.New("realtime event could not be forwarded"))
			}
			if !packet.client {
				info.ReceivedResponseCount++
			}
		}
	}
}

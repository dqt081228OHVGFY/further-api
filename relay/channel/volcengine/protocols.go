package volcengine

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"io"
)

type (
	EventType         int32
	MsgType           uint8
	MsgTypeFlagBits   uint8
	VersionBits       uint8
	HeaderSizeBits    uint8
	SerializationBits uint8
	CompressionBits   uint8
)

const (
	MsgTypeFlagNoSeq       MsgTypeFlagBits = 0
	MsgTypeFlagPositiveSeq MsgTypeFlagBits = 0b1
	MsgTypeFlagNegativeSeq MsgTypeFlagBits = 0b11
	MsgTypeFlagWithEvent   MsgTypeFlagBits = 0b100
)

const (
	Version1 VersionBits = iota + 1
)

const (
	HeaderSize4 HeaderSizeBits = iota + 1
)

const (
	SerializationJSON SerializationBits = 0b1
)

const (
	CompressionNone CompressionBits = 0
)

const (
	MsgTypeFullClientRequest    MsgType = 0b1
	MsgTypeAudioOnlyClient      MsgType = 0b10
	MsgTypeFullServerResponse   MsgType = 0b1001
	MsgTypeAudioOnlyServer      MsgType = 0b1011
	MsgTypeFrontEndResultServer MsgType = 0b1100
	MsgTypeError                MsgType = 0b1111
)

func (t MsgType) String() string {
	switch t {
	case MsgTypeFullClientRequest:
		return "MsgType_FullClientRequest"
	case MsgTypeAudioOnlyClient:
		return "MsgType_AudioOnlyClient"
	case MsgTypeFullServerResponse:
		return "MsgType_FullServerResponse"
	case MsgTypeAudioOnlyServer:
		return "MsgType_AudioOnlyServer"
	case MsgTypeError:
		return "MsgType_Error"
	case MsgTypeFrontEndResultServer:
		return "MsgType_FrontEndResultServer"
	default:
		return fmt.Sprintf("MsgType_(%d)", t)
	}
}

const (
	EventType_None EventType = 0

	EventType_StartConnection  EventType = 1
	EventType_FinishConnection EventType = 2

	EventType_ConnectionStarted  EventType = 50
	EventType_ConnectionFailed   EventType = 51
	EventType_ConnectionFinished EventType = 52

	EventType_StartSession  EventType = 100
	EventType_CancelSession EventType = 101
	EventType_FinishSession EventType = 102

	EventType_SessionStarted  EventType = 150
	EventType_SessionCanceled EventType = 151
	EventType_SessionFinished EventType = 152
	EventType_SessionFailed   EventType = 153

	EventType_UsageResponse EventType = 154

	EventType_TaskRequest  EventType = 200
	EventType_UpdateConfig EventType = 201

	EventType_AudioMuted EventType = 250

	EventType_SayHello EventType = 300

	EventType_TTSSentenceStart     EventType = 350
	EventType_TTSSentenceEnd       EventType = 351
	EventType_TTSResponse          EventType = 352
	EventType_TTSEnded             EventType = 359
	EventType_PodcastRoundStart    EventType = 360
	EventType_PodcastRoundResponse EventType = 361
	EventType_PodcastRoundEnd      EventType = 362

	EventType_ASRInfo     EventType = 450
	EventType_ASRResponse EventType = 451
	EventType_ASREnded    EventType = 459

	EventType_ChatTTSText EventType = 500

	EventType_ChatResponse EventType = 550
	EventType_ChatEnded    EventType = 559

	EventType_SourceSubtitleStart    EventType = 650
	EventType_SourceSubtitleResponse EventType = 651
	EventType_SourceSubtitleEnd      EventType = 652

	EventType_TranslationSubtitleStart    EventType = 653
	EventType_TranslationSubtitleResponse EventType = 654
	EventType_TranslationSubtitleEnd      EventType = 655
)

func (t EventType) String() string {
	switch t {
	case EventType_None:
		return "EventType_None"
	case EventType_StartConnection:
		return "EventType_StartConnection"
	case EventType_FinishConnection:
		return "EventType_FinishConnection"
	case EventType_ConnectionStarted:
		return "EventType_ConnectionStarted"
	case EventType_ConnectionFailed:
		return "EventType_ConnectionFailed"
	case EventType_ConnectionFinished:
		return "EventType_ConnectionFinished"
	case EventType_StartSession:
		return "EventType_StartSession"
	case EventType_CancelSession:
		return "EventType_CancelSession"
	case EventType_FinishSession:
		return "EventType_FinishSession"
	case EventType_SessionStarted:
		return "EventType_SessionStarted"
	case EventType_SessionCanceled:
		return "EventType_SessionCanceled"
	case EventType_SessionFinished:
		return "EventType_SessionFinished"
	case EventType_SessionFailed:
		return "EventType_SessionFailed"
	case EventType_UsageResponse:
		return "EventType_UsageResponse"
	case EventType_TaskRequest:
		return "EventType_TaskRequest"
	case EventType_UpdateConfig:
		return "EventType_UpdateConfig"
	case EventType_AudioMuted:
		return "EventType_AudioMuted"
	case EventType_SayHello:
		return "EventType_SayHello"
	case EventType_TTSSentenceStart:
		return "EventType_TTSSentenceStart"
	case EventType_TTSSentenceEnd:
		return "EventType_TTSSentenceEnd"
	case EventType_TTSResponse:
		return "EventType_TTSResponse"
	case EventType_TTSEnded:
		return "EventType_TTSEnded"
	case EventType_PodcastRoundStart:
		return "EventType_PodcastRoundStart"
	case EventType_PodcastRoundResponse:
		return "EventType_PodcastRoundResponse"
	case EventType_PodcastRoundEnd:
		return "EventType_PodcastRoundEnd"
	case EventType_ASRInfo:
		return "EventType_ASRInfo"
	case EventType_ASRResponse:
		return "EventType_ASRResponse"
	case EventType_ASREnded:
		return "EventType_ASREnded"
	case EventType_ChatTTSText:
		return "EventType_ChatTTSText"
	case EventType_ChatResponse:
		return "EventType_ChatResponse"
	case EventType_ChatEnded:
		return "EventType_ChatEnded"
	case EventType_SourceSubtitleStart:
		return "EventType_SourceSubtitleStart"
	case EventType_SourceSubtitleResponse:
		return "EventType_SourceSubtitleResponse"
	case EventType_SourceSubtitleEnd:
		return "EventType_SourceSubtitleEnd"
	case EventType_TranslationSubtitleStart:
		return "EventType_TranslationSubtitleStart"
	case EventType_TranslationSubtitleResponse:
		return "EventType_TranslationSubtitleResponse"
	case EventType_TranslationSubtitleEnd:
		return "EventType_TranslationSubtitleEnd"
	default:
		return fmt.Sprintf("EventType_(%d)", t)
	}
}

type Message struct {
	Version              VersionBits
	HeaderSize           HeaderSizeBits
	MsgType              MsgType
	MsgTypeFlag          MsgTypeFlagBits
	Serialization        SerializationBits
	Compression          CompressionBits
	EventType            EventType
	SessionID, ConnectID string
	Sequence             int32
	ErrorCode            uint32
	Payload              []byte
}

const maxVolcengineFramePayload = 32 << 20

func NewMessage(kind MsgType, flags MsgTypeFlagBits) (*Message, error) {
	if !validVolcengineKind(kind) || flags > MsgTypeFlagWithEvent {
		return nil, errors.New("语音帧类型或标记无效。")
	}
	return &Message{Version: Version1, HeaderSize: HeaderSize4, MsgType: kind, MsgTypeFlag: flags, Serialization: SerializationJSON, Compression: CompressionNone}, nil
}
func NewMessageFromBytes(data []byte) (*Message, error) {
	message := &Message{}
	if err := message.Unmarshal(data); err != nil {
		return nil, err
	}
	return message, nil
}
func validVolcengineKind(kind MsgType) bool {
	switch kind {
	case MsgTypeFullClientRequest, MsgTypeAudioOnlyClient, MsgTypeFullServerResponse, MsgTypeAudioOnlyServer, MsgTypeFrontEndResultServer, MsgTypeError:
		return true
	}
	return false
}
func (message *Message) String() string {
	if message == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%s, %s, Sequence: %d, ErrorCode: %d, PayloadSize: %d", message.MsgType, message.EventType, message.Sequence, message.ErrorCode, len(message.Payload))
}
func connectionEvent(event EventType) bool {
	return event == EventType_StartConnection || event == EventType_FinishConnection || event == EventType_ConnectionStarted || event == EventType_ConnectionFailed || event == EventType_ConnectionFinished
}
func responseConnectionEvent(event EventType) bool {
	return event == EventType_ConnectionStarted || event == EventType_ConnectionFailed || event == EventType_ConnectionFinished
}

func (message *Message) Marshal() ([]byte, error) {
	if message == nil || !validVolcengineKind(message.MsgType) || message.MsgTypeFlag > MsgTypeFlagWithEvent || len(message.Payload) > maxVolcengineFramePayload {
		return nil, errors.New("语音帧参数无效。")
	}
	version, words := message.Version, message.HeaderSize
	if version == 0 {
		version = Version1
	}
	if words == 0 {
		words = HeaderSize4
	}
	if version != Version1 || words > 15 || message.Serialization > 1 || message.Compression > 1 {
		return nil, errors.New("语音帧协议版本或编码无效。")
	}
	body := append([]byte(nil), message.Payload...)
	if message.Compression == 1 {
		var compressed bytes.Buffer
		writer := gzip.NewWriter(&compressed)
		if _, err := writer.Write(body); err != nil {
			return nil, err
		}
		if err := writer.Close(); err != nil {
			return nil, err
		}
		body = compressed.Bytes()
		if len(body) > maxVolcengineFramePayload {
			return nil, errors.New("压缩语音帧超过容量限制。")
		}
	}
	data := make([]byte, int(words)*4)
	data[0] = byte(version)<<4 | byte(words)
	data[1] = byte(message.MsgType)<<4 | byte(message.MsgTypeFlag)
	data[2] = byte(message.Serialization)<<4 | byte(message.Compression)
	integer := func(value uint32) { data = binary.BigEndian.AppendUint32(data, value) }
	field := func(value string) error {
		if len(value) > 8192 {
			return errors.New("语音帧标识符超过容量限制。")
		}
		integer(uint32(len(value)))
		data = append(data, value...)
		return nil
	}
	if message.MsgType == MsgTypeError {
		integer(message.ErrorCode)
	} else if message.MsgTypeFlag&1 != 0 {
		integer(uint32(message.Sequence))
	}
	if message.MsgTypeFlag == MsgTypeFlagWithEvent {
		integer(uint32(message.EventType))
		if responseConnectionEvent(message.EventType) {
			if err := field(message.ConnectID); err != nil {
				return nil, err
			}
		} else if !connectionEvent(message.EventType) {
			if err := field(message.SessionID); err != nil {
				return nil, err
			}
		}
	}
	integer(uint32(len(body)))
	data = append(data, body...)
	return data, nil
}

func (message *Message) Unmarshal(data []byte) error {
	if message == nil || len(data) < 4 {
		return io.ErrUnexpectedEOF
	}
	decoded := Message{Version: VersionBits(data[0] >> 4), HeaderSize: HeaderSizeBits(data[0] & 15), MsgType: MsgType(data[1] >> 4), MsgTypeFlag: MsgTypeFlagBits(data[1] & 15), Serialization: SerializationBits(data[2] >> 4), Compression: CompressionBits(data[2] & 15)}
	header := int(decoded.HeaderSize) * 4
	if decoded.Version != Version1 || header < 4 || header > len(data) || !validVolcengineKind(decoded.MsgType) || decoded.MsgTypeFlag > MsgTypeFlagWithEvent || decoded.Serialization > 1 || decoded.Compression > 1 {
		return errors.New("语音帧头无效。")
	}
	cursor := header
	integer := func() (uint32, error) {
		if len(data)-cursor < 4 {
			return 0, io.ErrUnexpectedEOF
		}
		value := binary.BigEndian.Uint32(data[cursor : cursor+4])
		cursor += 4
		return value, nil
	}
	field := func(limit int) ([]byte, error) {
		size, err := integer()
		if err != nil {
			return nil, err
		}
		if uint64(size) > uint64(limit) {
			return nil, errors.New("语音帧字段超过容量限制。")
		}
		if uint64(size) > uint64(len(data)-cursor) {
			return nil, io.ErrUnexpectedEOF
		}
		value := data[cursor : cursor+int(size)]
		cursor += int(size)
		return value, nil
	}
	if decoded.MsgType == MsgTypeError {
		value, err := integer()
		if err != nil {
			return err
		}
		decoded.ErrorCode = value
	} else if decoded.MsgTypeFlag&1 != 0 {
		value, err := integer()
		if err != nil {
			return err
		}
		decoded.Sequence = int32(value)
	}
	if decoded.MsgTypeFlag == MsgTypeFlagWithEvent {
		event, err := integer()
		if err != nil {
			return err
		}
		decoded.EventType = EventType(int32(event))
		if responseConnectionEvent(decoded.EventType) {
			value, err := field(8192)
			if err != nil {
				return err
			}
			decoded.ConnectID = string(value)
		} else if !connectionEvent(decoded.EventType) {
			value, err := field(8192)
			if err != nil {
				return err
			}
			decoded.SessionID = string(value)
		}
	}
	payload, err := field(maxVolcengineFramePayload)
	if err != nil {
		return err
	}
	if cursor != len(data) {
		return errors.New("语音帧包含未声明的尾部数据。")
	}
	if decoded.Compression == 1 {
		reader, err := gzip.NewReader(bytes.NewReader(payload))
		if err != nil {
			return err
		}
		decompressed, readErr := io.ReadAll(io.LimitReader(reader, maxVolcengineFramePayload+1))
		closeErr := reader.Close()
		if err := errors.Join(readErr, closeErr); err != nil {
			return err
		}
		if len(decompressed) > maxVolcengineFramePayload {
			return errors.New("语音帧解压后超过容量限制。")
		}
		decoded.Payload = decompressed
	} else {
		decoded.Payload = append([]byte(nil), payload...)
	}
	*message = decoded
	return nil
}

func ReceiveMessage(socket *websocket.Conn) (*Message, error) {
	if socket == nil {
		return nil, errors.New("语音连接为空。")
	}
	kind, frame, err := socket.ReadMessage()
	if err != nil {
		return nil, err
	}
	if kind != websocket.BinaryMessage && kind != websocket.TextMessage {
		return nil, errors.New("语音连接返回了非数据帧。")
	}
	return NewMessageFromBytes(frame)
}
func FullClientRequest(socket *websocket.Conn, payload []byte) error {
	message, err := NewMessage(MsgTypeFullClientRequest, MsgTypeFlagNoSeq)
	if err != nil {
		return err
	}
	message.Payload = payload
	frame, err := message.Marshal()
	if err != nil {
		return err
	}
	return socket.WriteMessage(websocket.BinaryMessage, frame)
}

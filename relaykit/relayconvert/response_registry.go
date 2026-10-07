package relayconvert

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/relayconvert/convmeta"
	claudemessages "github.com/dqt081228OHVGFY/further-api/relaykit/relayconvert/internal/claude_messages"
	geminichat "github.com/dqt081228OHVGFY/further-api/relaykit/relayconvert/internal/gemini_chat"
	oaichat "github.com/dqt081228OHVGFY/further-api/relaykit/relayconvert/internal/oai_chat"
	oairesponses "github.com/dqt081228OHVGFY/further-api/relaykit/relayconvert/internal/oai_responses"
	kitutil "github.com/dqt081228OHVGFY/further-api/relaykit/relayconvert/kitutil"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
)

type ResponseConverterFunc func(c context.Context, info convmeta.Meta, response any) (any, *dto.Usage, error)

type ResponseStreamConverterFunc func(c context.Context, info convmeta.Meta, response any) (any, *dto.Usage, error)

type ResponseStreamStateFactory func(options ResponseStreamOptions) any

type ResponseStreamChunkConverterFunc func(c context.Context, info convmeta.Meta, response any, state any) ([]any, *dto.Usage, error)

type ResponseStreamFinalizerFunc func(c context.Context, info convmeta.Meta, state any) ([]any, *dto.Usage, error)

type ResponseConverterQuality string

const (
	ResponseConverterQualityGood        ResponseConverterQuality = "good"
	ResponseConverterQualityFair        ResponseConverterQuality = "fair"
	ResponseConverterQualityDiscouraged ResponseConverterQuality = "discouraged"
)

type ResponseStep struct {
	Converter string
	From      types.RelayFormat
	To        types.RelayFormat
}

type ResponseResult struct {
	Value       any
	Usage       *dto.Usage
	From        types.RelayFormat
	To          types.RelayFormat
	Converter   string
	Quality     ResponseConverterQuality
	Steps       []ResponseStep
	Stream      bool
	Diagnostics []types.ConversionDiagnostic
}

type ResponseConverterSpec struct {
	ID                 string
	From               types.RelayFormat
	To                 types.RelayFormat
	Quality            ResponseConverterQuality
	Convert            ResponseConverterFunc
	ConvertStream      ResponseStreamConverterFunc
	NewStreamState     ResponseStreamStateFactory
	ConvertStreamChunk ResponseStreamChunkConverterFunc
	FinalizeStream     ResponseStreamFinalizerFunc
	StepConverters     []string
}

type responseConverterRoute struct {
	from types.RelayFormat
	to   types.RelayFormat
}

type ResponseStreamOptions struct {
	ID           string
	Model        string
	Created      int64
	IncludeUsage bool
	// EmitSequenceNumber opts into the current Responses SSE wire contract.
	// It is explicit so relaykit callers that depend on the historical zero-value
	// output are not changed merely by upgrading the module.
	EmitSequenceNumber bool
}

type conversionDiagnosticKey struct {
	code     string
	path     string
	severity types.ConversionDiagnosticSeverity
	from     types.RelayFormat
	to       types.RelayFormat
}

type ResponseStreamState struct {
	mu        sync.Mutex
	closed    bool
	failed    error
	From      types.RelayFormat
	To        types.RelayFormat
	Converter string
	Quality   ResponseConverterQuality
	Steps     []ResponseStep

	specs              []ResponseConverterSpec
	stepStates         []any
	usage              *dto.Usage
	diagnostics        []types.ConversionDiagnostic
	pendingDiagnostics []types.ConversionDiagnostic
	seenDiagnostics    map[conversionDiagnosticKey]struct{}
	fallbackInfo       *convmeta.Values
}

type responseStreamUsageCarrier interface {
	StreamUsage() *dto.Usage
	SetStreamUsage(*dto.Usage)
}

const (
	ResponseConverterOAIChatToOAIResponses   = "oai_chat_to_oai_responses_resp"
	ResponseConverterOAIResponsesToOAIChat   = "oai_responses_to_oai_chat_resp"
	ResponseConverterOAIChatToClaudeMessages = "oai_chat_to_claude_messages_resp"
	ResponseConverterOAIChatToGeminiChat     = "oai_chat_to_gemini_chat_resp"
	ResponseConverterClaudeMessagesToOAIChat = "claude_messages_to_oai_chat_resp"
	ResponseConverterGeminiChatToOAIChat     = "gemini_chat_to_oai_chat_resp"

	responseConverterClaudeToGemini    = "claude_messages_to_gemini_chat_resp"
	responseConverterClaudeToResponses = "claude_messages_to_oai_responses_resp"
	responseConverterGeminiToClaude    = "gemini_chat_to_claude_messages_resp"
	responseConverterGeminiToResponses = "gemini_chat_to_oai_responses_resp"
	responseConverterResponsesToClaude = "oai_responses_to_claude_messages_resp"
	responseConverterResponsesToGemini = "oai_responses_to_gemini_chat_resp"
)

var (
	responseConverterMu      sync.RWMutex
	responseConverters       = make(map[string]ResponseConverterSpec)
	responseConverterAliases = make(map[string]string)
	responseConverterRoutes  = make(map[responseConverterRoute]string)
)

func registerBuiltinResponseConverter(spec ResponseConverterSpec) {
	spec.ID = strings.TrimSpace(spec.ID)
	if spec.ID == "" {
		panic("response converter ID is required")
	}
	if spec.From == "" || spec.To == "" {
		panic(fmt.Sprintf("response converter %q must declare from and to formats", spec.ID))
	}
	if spec.Quality == "" {
		panic(fmt.Sprintf("response converter %q must declare quality", spec.ID))
	}
	if spec.Convert == nil &&
		spec.ConvertStream == nil &&
		spec.ConvertStreamChunk == nil &&
		len(spec.StepConverters) == 0 {
		panic(fmt.Sprintf("response converter %q must declare convert, stream convert, or step converters", spec.ID))
	}
	if len(spec.StepConverters) > 0 &&
		(spec.Convert != nil || spec.ConvertStream != nil || spec.NewStreamState != nil || spec.ConvertStreamChunk != nil || spec.FinalizeStream != nil) {
		panic(fmt.Sprintf("response converter %q cannot declare direct implementations and step converters together", spec.ID))
	}
	if _, exists := responseConverters[spec.ID]; exists {
		panic(fmt.Sprintf("response converter %q is already registered", spec.ID))
	}
	route := responseConverterRoute{from: spec.From, to: spec.To}
	if existingID, exists := responseConverterRoutes[route]; exists {
		panic(fmt.Sprintf("response converter route from %s to %s is already registered by %q", spec.From, spec.To, existingID))
	}

	if len(spec.StepConverters) > 0 {
		stepConverters := make([]string, 0, len(spec.StepConverters))
		current := spec.From
		for _, converterID := range spec.StepConverters {
			step, ok := responseConverters[converterID]
			if !ok {
				panic(fmt.Sprintf("response converter %q references unknown step converter %q", spec.ID, converterID))
			}
			if len(step.StepConverters) > 0 {
				panic(fmt.Sprintf("response converter %q step %q must be a direct converter", spec.ID, converterID))
			}
			if step.From != current {
				panic(fmt.Sprintf("response converter %q step %q expects %s after %s", spec.ID, converterID, step.From, current))
			}
			stepConverters = append(stepConverters, converterID)
			current = step.To
		}
		if current != spec.To {
			panic(fmt.Sprintf("response converter %q ends at %s, expected %s", spec.ID, current, spec.To))
		}
		spec.StepConverters = stepConverters
	}

	responseConverters[spec.ID] = spec
	responseConverterRoutes[route] = spec.ID
}

func registerResponseConverterAlias(alias string, converter string) {
	alias = strings.TrimSpace(alias)
	converter = strings.TrimSpace(converter)
	if alias == "" {
		panic("response converter alias is required")
	}
	if converter == "" {
		panic(fmt.Sprintf("response converter alias %q target is required", alias))
	}
	if alias == converter {
		return
	}
	if _, exists := responseConverters[alias]; exists {
		panic(fmt.Sprintf("response converter alias %q conflicts with registered converter", alias))
	}
	if _, exists := responseConverters[converter]; !exists {
		panic(fmt.Sprintf("response converter alias %q references unknown converter %q", alias, converter))
	}
	if existing, exists := responseConverterAliases[alias]; exists && existing != converter {
		panic(fmt.Sprintf("response converter alias %q is already registered for %q", alias, existing))
	}
	responseConverterAliases[alias] = converter
}

func LookupResponseConverter(converter string) (ResponseConverterSpec, bool) {
	responseConverterMu.RLock()
	defer responseConverterMu.RUnlock()

	converterID := resolveResponseConverterID(converter)
	spec, ok := responseConverters[converterID]
	if !ok {
		return ResponseConverterSpec{}, false
	}
	return cloneResponseConverterSpec(spec), true
}

func NewResponseStreamState(from types.RelayFormat, target types.RelayFormat, options ResponseStreamOptions) (*ResponseStreamState, error) {
	if from == "" {
		return nil, errors.New("source relay format is required")
	}
	if target == "" {
		return nil, errors.New("target relay format is required")
	}
	if from == target {
		return &ResponseStreamState{
			From: from,
			To:   target,
		}, nil
	}

	spec, ok := lookupResponseRoute(from, target)
	if !ok {
		return nil, fmt.Errorf("response converter from %s to %s is not registered", from, target)
	}
	return newResponseStreamStateFromSpec(from, target, options, spec)
}

func NewResponseStreamStateByID(converter string, options ResponseStreamOptions) (*ResponseStreamState, error) {
	spec, ok := LookupResponseConverter(converter)
	if !ok {
		return nil, fmt.Errorf("response converter %q is not registered", strings.TrimSpace(converter))
	}
	return newResponseStreamStateFromSpec(spec.From, spec.To, options, spec)
}

func newResponseStreamStateFromSpec(from types.RelayFormat, target types.RelayFormat, options ResponseStreamOptions, spec ResponseConverterSpec) (*ResponseStreamState, error) {
	steps, err := expandResponseConverterSteps(spec)
	if err != nil {
		return nil, err
	}
	stepStates := make([]any, len(steps))
	resultSteps := make([]ResponseStep, 0, len(steps))
	for i, step := range steps {
		if step.NewStreamState != nil {
			stepStates[i] = step.NewStreamState(options)
		}
		resultSteps = append(resultSteps, ResponseStep{
			Converter: step.ID,
			From:      step.From,
			To:        step.To,
		})
	}
	return &ResponseStreamState{
		From:       from,
		To:         target,
		Converter:  spec.ID,
		Quality:    spec.Quality,
		Steps:      resultSteps,
		specs:      steps,
		stepStates: stepStates,
	}, nil
}

func prepareResponseStreamInfo(info convmeta.Meta, spec ResponseConverterSpec) {
	if info == nil {
		return
	}
	if spec.From != types.RelayFormatOpenAI {
		return
	}
	if spec.To != types.RelayFormatClaude && spec.To != types.RelayFormatGemini {
		return
	}
	info.IncrSendResponseCount()
}

func (s *ResponseStreamState) rememberUsage(usage *dto.Usage) {
	if s != nil && usage != nil {
		s.usage = dto.MergeUsageNonZero(s.usage, snapshotUsage(usage))
	}
}

func (s *ResponseStreamState) rememberDiagnostics(diagnostics []types.ConversionDiagnostic) {
	if s == nil || len(diagnostics) == 0 {
		return
	}
	if s.seenDiagnostics == nil {
		s.seenDiagnostics = make(map[conversionDiagnosticKey]struct{})
	}
	for _, diagnostic := range diagnostics {
		key := conversionDiagnosticKey{
			code:     diagnostic.Code,
			path:     diagnostic.Path,
			severity: diagnostic.Severity,
			from:     diagnostic.From,
			to:       diagnostic.To,
		}
		if _, exists := s.seenDiagnostics[key]; exists {
			continue
		}
		s.seenDiagnostics[key] = struct{}{}
		s.diagnostics = append(s.diagnostics, diagnostic)
		s.pendingDiagnostics = append(s.pendingDiagnostics, diagnostic)
	}
}

func (s *ResponseStreamState) takeDiagnostics(hasOutput bool) []types.ConversionDiagnostic {
	if s == nil || !hasOutput || len(s.pendingDiagnostics) == 0 {
		return nil
	}
	diagnostics := append([]types.ConversionDiagnostic{}, s.pendingDiagnostics...)
	s.pendingDiagnostics = nil
	return diagnostics
}

func streamValuesFromAny(value any) []any {
	if value == nil {
		return nil
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() == reflect.Pointer && rv.IsNil() {
		return nil
	}
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return []any{value}
	}
	if rv.Type().Elem().Kind() == reflect.Uint8 {
		return []any{value}
	}
	values := make([]any, 0, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		item := rv.Index(i)
		if item.Kind() == reflect.Pointer && item.IsNil() {
			continue
		}
		values = append(values, item.Interface())
	}
	return values
}

func expandResponseConverterSteps(spec ResponseConverterSpec) ([]ResponseConverterSpec, error) {
	if len(spec.StepConverters) == 0 {
		if spec.Convert == nil && spec.ConvertStream == nil && spec.ConvertStreamChunk == nil {
			return nil, fmt.Errorf("response converter %q has no registered implementation", spec.ID)
		}
		return []ResponseConverterSpec{spec}, nil
	}

	steps := make([]ResponseConverterSpec, 0, len(spec.StepConverters))
	current := spec.From
	for _, converterID := range spec.StepConverters {
		step, ok := LookupResponseConverter(converterID)
		if !ok {
			return nil, fmt.Errorf("response converter %q references missing step converter %q", spec.ID, converterID)
		}
		if len(step.StepConverters) > 0 {
			return nil, fmt.Errorf("response converter %q step %q is not a direct converter", spec.ID, converterID)
		}
		if step.From != current {
			return nil, fmt.Errorf("response converter %q step %q expects %s response, got %s", spec.ID, converterID, step.From, current)
		}
		steps = append(steps, step)
		current = step.To
	}
	if current != spec.To {
		return nil, fmt.Errorf("response converter %q ends at %s, expected %s", spec.ID, current, spec.To)
	}
	return steps, nil
}

func lookupResponseRoute(from types.RelayFormat, to types.RelayFormat) (ResponseConverterSpec, bool) {
	responseConverterMu.RLock()
	defer responseConverterMu.RUnlock()

	converterID, ok := responseConverterRoutes[responseConverterRoute{from: from, to: to}]
	if !ok {
		return ResponseConverterSpec{}, false
	}
	spec, ok := responseConverters[converterID]
	return cloneResponseConverterSpec(spec), ok
}

func resolveResponseConverterID(converter string) string {
	converter = strings.TrimSpace(converter)
	if canonical, ok := responseConverterAliases[converter]; ok {
		return canonical
	}
	return converter
}

func cloneResponseConverterSpec(spec ResponseConverterSpec) ResponseConverterSpec {
	if len(spec.StepConverters) > 0 {
		spec.StepConverters = append([]string{}, spec.StepConverters...)
	}
	return spec
}

func inferResponseRelayFormat(response any) (types.RelayFormat, error) {
	if isNilResponse(response) {
		return "", errors.New("response is nil")
	}
	switch response.(type) {
	case *dto.OpenAITextResponse, dto.OpenAITextResponse, *dto.ChatCompletionsStreamResponse, dto.ChatCompletionsStreamResponse:
		return types.RelayFormatOpenAI, nil
	case *dto.OpenAIResponsesResponse, dto.OpenAIResponsesResponse, *dto.ResponsesStreamResponse, dto.ResponsesStreamResponse:
		return types.RelayFormatOpenAIResponses, nil
	case *dto.ClaudeResponse, dto.ClaudeResponse:
		return types.RelayFormatClaude, nil
	case *dto.GeminiChatResponse, dto.GeminiChatResponse:
		return types.RelayFormatGemini, nil
	default:
		return "", fmt.Errorf("unsupported response type %T", response)
	}
}

func isNilResponse(response any) bool {
	if response == nil {
		return true
	}
	value := reflect.ValueOf(response)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func canonicalUsageFromResponse(response any) *dto.Usage {
	switch resp := response.(type) {
	case dto.OpenAITextResponse:
		response = &resp
	case dto.ChatCompletionsStreamResponse:
		response = &resp
	case dto.OpenAIResponsesResponse:
		response = &resp
	case dto.ResponsesStreamResponse:
		response = &resp
	case dto.ClaudeResponse:
		response = &resp
	case dto.GeminiChatResponse:
		response = &resp
	}
	switch resp := response.(type) {
	case *dto.OpenAITextResponse:
		return UsageFromChatUsage(&resp.Usage)
	case *dto.ChatCompletionsStreamResponse:
		if resp.Usage == nil {
			return nil
		}
		return UsageFromChatUsage(resp.Usage)
	case *dto.OpenAIResponsesResponse:
		return UsageFromResponsesUsage(resp.Usage)
	case *dto.ResponsesStreamResponse:
		if resp.Response == nil {
			return nil
		}
		return UsageFromResponsesUsage(resp.Response.Usage)
	case *dto.ClaudeResponse:
		return usageFromClaudeResponse(resp)
	case *dto.GeminiChatResponse:
		return UsageFromGeminiMetadata(resp.GetUsageMetadata(), 0)
	default:
		return nil
	}
}

func usageFromClaudeResponse(resp *dto.ClaudeResponse) *dto.Usage {
	if resp == nil {
		return nil
	}
	if resp.Usage != nil {
		return UsageFromClaudeAPIUsage(resp.Usage)
	}
	if resp.Message != nil && resp.Message.Usage != nil {
		return UsageFromClaudeAPIUsage(resp.Message.Usage)
	}
	return nil
}

func convertOAIChatResponseToOAIResponses(_ context.Context, _ convmeta.Meta, response any) (any, *dto.Usage, error) {
	chatResponse, err := asOAIChatResponse(response)
	if err != nil {
		return nil, nil, err
	}
	id := strings.TrimSpace(chatResponse.Id)
	if id == "" {
		id = fmt.Sprintf("resp_%s", kitutil.GetUUID())
	}
	return ChatCompletionsResponseToResponsesResponse(chatResponse, id)
}

func convertOAIResponsesResponseToOAIChat(_ context.Context, _ convmeta.Meta, response any) (any, *dto.Usage, error) {
	responsesResponse, err := asOAIResponsesResponse(response)
	if err != nil {
		return nil, nil, err
	}
	id := strings.TrimSpace(responsesResponse.ID)
	if id == "" {
		id = fmt.Sprintf("chatcmpl-%s", kitutil.GetUUID())
	}
	return ResponsesResponseToChatCompletionsResponse(responsesResponse, id)
}

func convertOAIResponsesResponseToClaudeMessages(_ context.Context, _ convmeta.Meta, response any) (any, *dto.Usage, error) {
	responsesResponse, err := asOAIResponsesResponse(response)
	if err != nil {
		return nil, nil, err
	}
	return oairesponses.ResponsesResponseToClaudeMessagesResponse(responsesResponse)
}

func newOAIChatToOAIResponsesStreamState(options ResponseStreamOptions) any {
	id := strings.TrimSpace(options.ID)
	if id == "" {
		id = fmt.Sprintf("resp_%s", kitutil.GetUUID())
	}
	state := NewChatToResponsesStreamState(id, strings.TrimSpace(options.Model))
	state.EmitSequenceNumber = options.EmitSequenceNumber
	if options.Created != 0 {
		state.Created = options.Created
	}
	return state
}

func convertOAIChatStreamResponseToOAIResponses(_ context.Context, _ convmeta.Meta, response any, state any) ([]any, *dto.Usage, error) {
	chatResponse, err := asOAIChatStreamResponse(response)
	if err != nil {
		return nil, nil, err
	}
	streamState, ok := state.(*ChatToResponsesStreamState)
	if !ok || streamState == nil {
		return nil, nil, errors.New("OAI chat to OAI responses stream state is required")
	}
	events, err := ChatCompletionsStreamChunkToResponsesEvents(chatResponse, streamState)
	if err != nil {
		return nil, nil, err
	}
	return streamValuesFromAny(events), streamState.Usage, nil
}

func finalizeOAIChatStreamResponseToOAIResponses(_ context.Context, _ convmeta.Meta, state any) ([]any, *dto.Usage, error) {
	streamState, ok := state.(*ChatToResponsesStreamState)
	if !ok || streamState == nil {
		return nil, nil, errors.New("OAI chat to OAI responses stream state is required")
	}
	events := FinalizeChatCompletionsStreamToResponses(streamState)
	return streamValuesFromAny(events), streamState.Usage, nil
}

func newOAIResponsesToOAIChatStreamState(options ResponseStreamOptions) any {
	state := NewResponsesToChatStreamState(strings.TrimSpace(options.Model), options.IncludeUsage)
	state.ID = strings.TrimSpace(options.ID)
	if options.Created != 0 {
		state.Created = options.Created
	}
	return state
}

func convertOAIResponsesStreamResponseToOAIChat(_ context.Context, _ convmeta.Meta, response any, state any) ([]any, *dto.Usage, error) {
	responsesResponse, err := asOAIResponsesStreamResponse(response)
	if err != nil {
		return nil, nil, err
	}
	streamState, ok := state.(*ResponsesToChatStreamState)
	if !ok || streamState == nil {
		return nil, nil, errors.New("OAI responses to OAI chat stream state is required")
	}
	chunks, err := ResponsesStreamEventToChatChunks(responsesResponse, streamState)
	if err != nil {
		return nil, nil, err
	}
	return streamValuesFromAny(chunks), streamState.Usage, nil
}

func finalizeOAIResponsesStreamResponseToOAIChat(_ context.Context, _ convmeta.Meta, state any) ([]any, *dto.Usage, error) {
	streamState, ok := state.(*ResponsesToChatStreamState)
	if !ok || streamState == nil {
		return nil, nil, errors.New("OAI responses to OAI chat stream state is required")
	}
	chunks := FinalizeResponsesToChatStream(streamState)
	return streamValuesFromAny(chunks), streamState.Usage, nil
}

func newOAIResponsesToClaudeMessagesStreamState(options ResponseStreamOptions) any {
	return oairesponses.NewResponsesToClaudeStreamState(options.ID, options.Model)
}

func convertOAIResponsesStreamResponseToClaudeMessages(_ context.Context, info convmeta.Meta, response any, state any) ([]any, *dto.Usage, error) {
	responsesResponse, err := asOAIResponsesStreamResponse(response)
	if err != nil {
		return nil, nil, err
	}
	streamState, ok := state.(*oairesponses.ResponsesToClaudeStreamState)
	if !ok || streamState == nil {
		return nil, nil, errors.New("OAI responses to Claude stream state is required")
	}
	estimatedInputTokens := 0
	if info != nil {
		estimatedInputTokens = info.GetEstimatePromptTokens()
	}
	responses, usage, err := streamState.ConvertChunk(responsesResponse, estimatedInputTokens)
	if err != nil {
		return nil, usage, err
	}
	if info != nil && streamState.Done() {
		claudeInfo := info.EnsureClaudeConvertInfo()
		claudeInfo.Done = true
		if claudeInfo.Usage == nil {
			claudeInfo.Usage = usage
		}
	}
	return streamValuesFromAny(responses), usage, nil
}

func finalizeOAIResponsesStreamResponseToClaudeMessages(_ context.Context, info convmeta.Meta, state any) ([]any, *dto.Usage, error) {
	streamState, ok := state.(*oairesponses.ResponsesToClaudeStreamState)
	if !ok || streamState == nil {
		return nil, nil, errors.New("OAI responses to Claude stream state is required")
	}
	estimatedInputTokens := 0
	if info != nil {
		estimatedInputTokens = info.GetEstimatePromptTokens()
		if usage := info.EnsureClaudeConvertInfo().Usage; usage != nil {
			streamState.SetUsage(usage)
		}
	}
	responses, err := streamState.Finalize(estimatedInputTokens)
	if info != nil && streamState.Done() {
		info.EnsureClaudeConvertInfo().Done = true
	}
	return streamValuesFromAny(responses), streamState.Usage, err
}

func convertOAIChatResponseToClaudeMessages(_ context.Context, info convmeta.Meta, response any) (any, *dto.Usage, error) {
	chatResponse, err := asOAIChatResponse(response)
	if err != nil {
		return nil, nil, err
	}
	return ResponseOpenAI2Claude(chatResponse, info), UsageFromChatUsage(&chatResponse.Usage), nil
}

func convertOAIChatStreamResponseToClaudeMessages(_ context.Context, info convmeta.Meta, response any) (any, *dto.Usage, error) {
	chatResponse, err := asOAIChatStreamResponse(response)
	if err != nil {
		return nil, nil, err
	}
	return StreamResponseOpenAI2Claude(chatResponse, info), canonicalUsageFromResponse(chatResponse), nil
}

func finalizeOAIChatStreamResponseToClaudeMessages(_ context.Context, info convmeta.Meta, _ any) ([]any, *dto.Usage, error) {
	if info == nil {
		info = &convmeta.Values{}
	}
	usage := info.EnsureClaudeConvertInfo().Usage
	responses := oaichat.FinalizeStreamResponseOpenAI2Claude(info)
	return streamValuesFromAny(responses), usage, nil
}

func convertOAIChatResponseToGeminiChat(_ context.Context, info convmeta.Meta, response any) (any, *dto.Usage, error) {
	chatResponse, err := asOAIChatResponse(response)
	if err != nil {
		return nil, nil, err
	}
	return ResponseOpenAI2Gemini(chatResponse, info), UsageFromChatUsage(&chatResponse.Usage), nil
}

func convertOAIChatStreamResponseToGeminiChat(_ context.Context, info convmeta.Meta, response any) (any, *dto.Usage, error) {
	chatResponse, err := asOAIChatStreamResponse(response)
	if err != nil {
		return nil, nil, err
	}
	return StreamResponseOpenAI2Gemini(chatResponse, info), canonicalUsageFromResponse(chatResponse), nil
}

func newOAIChatToGeminiStreamState(_ ResponseStreamOptions) any {
	return oaichat.NewChatToGeminiStreamState()
}

func convertOAIChatStreamResponseChunkToGeminiChat(_ context.Context, info convmeta.Meta, response any, state any) ([]any, *dto.Usage, error) {
	chatResponse, err := asOAIChatStreamResponse(response)
	if err != nil {
		return nil, nil, err
	}
	streamState, ok := state.(*oaichat.ChatToGeminiStreamState)
	if !ok || streamState == nil {
		return nil, nil, errors.New("OAI chat to Gemini stream state is required")
	}
	responses, err := streamState.ConvertChunk(chatResponse, info)
	if err != nil {
		return nil, nil, err
	}
	return streamValuesFromAny(responses), canonicalUsageFromResponse(chatResponse), nil
}

func finalizeOAIChatStreamResponseToGeminiChat(_ context.Context, info convmeta.Meta, state any) ([]any, *dto.Usage, error) {
	streamState, ok := state.(*oaichat.ChatToGeminiStreamState)
	if !ok || streamState == nil {
		return nil, nil, errors.New("OAI chat to Gemini stream state is required")
	}
	responses, err := streamState.Finalize(info)
	if err != nil {
		return nil, nil, err
	}
	return streamValuesFromAny(responses), streamState.Usage(), nil
}

func convertClaudeMessagesResponseToOAIChat(_ context.Context, _ convmeta.Meta, response any) (any, *dto.Usage, error) {
	claudeResponse, err := asClaudeResponse(response)
	if err != nil {
		return nil, nil, err
	}
	usage := usageFromClaudeResponse(claudeResponse)
	openAIResponse := ResponseClaude2OpenAI(claudeResponse)
	if usage != nil {
		openAIResponse.Usage = *usage
	}
	return openAIResponse, usage, nil
}

func convertClaudeMessagesStreamResponseToOAIChat(_ context.Context, _ convmeta.Meta, response any) (any, *dto.Usage, error) {
	claudeResponse, err := asClaudeResponse(response)
	if err != nil {
		return nil, nil, err
	}
	openAIResponse := StreamResponseClaude2OpenAI(claudeResponse)
	usage := usageFromClaudeResponse(claudeResponse)
	if openAIResponse != nil && usage != nil {
		openAIResponse.Usage = usage
	}
	return openAIResponse, usage, nil
}

func newClaudeMessagesToOAIChatStreamState(_ ResponseStreamOptions) any {
	return claudemessages.NewClaudeToChatStreamState()
}

func convertClaudeMessagesStreamResponseChunkToOAIChat(_ context.Context, _ convmeta.Meta, response any, state any) ([]any, *dto.Usage, error) {
	claudeResponse, err := asClaudeResponse(response)
	if err != nil {
		return nil, nil, err
	}
	streamState, ok := state.(*claudemessages.ClaudeToChatStreamState)
	if !ok || streamState == nil {
		return nil, nil, errors.New("Claude-to-Chat stream state is required")
	}
	openAIResponse, err := streamState.ConvertChunk(claudeResponse)
	if err != nil {
		return nil, nil, err
	}
	usage := usageFromClaudeResponse(claudeResponse)
	if openAIResponse != nil && usage != nil {
		openAIResponse.Usage = usage
	}
	return streamValuesFromAny(openAIResponse), usage, nil
}

func convertGeminiChatResponseToOAIChat(_ context.Context, info convmeta.Meta, response any) (any, *dto.Usage, error) {
	geminiResponse, err := asGeminiChatResponse(response)
	if err != nil {
		return nil, nil, err
	}
	usage := UsageFromGeminiMetadata(geminiResponse.GetUsageMetadata(), fallbackPromptTokens(info))
	openAIResponse := ResponseGeminiChat2OpenAI(fmt.Sprintf("chatcmpl-%s", kitutil.GetUUID()), kitutil.GetTimestamp(), geminiResponse)
	if info != nil && info.HasChannelMeta() {
		openAIResponse.Model = info.GetUpstreamModelName()
	}
	if usage != nil {
		openAIResponse.Usage = *usage
	}
	return openAIResponse, usage, nil
}

func newGeminiChatToOAIChatStreamState(options ResponseStreamOptions) any {
	return geminichat.NewGeminiToChatStreamState(options.ID, options.Created)
}

func convertGeminiChatStreamResponseChunkToOAIChat(_ context.Context, info convmeta.Meta, response any, state any) ([]any, *dto.Usage, error) {
	geminiResponse, err := asGeminiChatResponse(response)
	if err != nil {
		return nil, nil, err
	}
	streamState, ok := state.(*geminichat.GeminiToChatStreamState)
	if !ok || streamState == nil {
		return nil, nil, errors.New("Gemini chat to OAI chat stream state is required")
	}
	usage := UsageFromGeminiMetadata(geminiResponse.GetUsageMetadata(), fallbackPromptTokens(info))
	model := ""
	if info != nil && info.HasChannelMeta() {
		model = info.GetUpstreamModelName()
	}
	responses, err := streamState.ConvertChunk(geminiResponse, model, usage)
	if err != nil {
		return nil, nil, err
	}
	return streamValuesFromAny(responses), usage, nil
}

func finalizeGeminiChatStreamResponseToOAIChat(_ context.Context, info convmeta.Meta, state any) ([]any, *dto.Usage, error) {
	streamState, ok := state.(*geminichat.GeminiToChatStreamState)
	if !ok || streamState == nil {
		return nil, nil, errors.New("Gemini chat to OAI chat stream state is required")
	}
	model := ""
	if info != nil && info.HasChannelMeta() {
		model = info.GetUpstreamModelName()
	}
	responses, err := streamState.Finalize(model)
	if err != nil {
		return nil, nil, err
	}
	return streamValuesFromAny(responses), streamState.Usage(), nil
}

func convertGeminiChatStreamResponseToOAIChat(_ context.Context, info convmeta.Meta, response any) (any, *dto.Usage, error) {
	geminiResponse, err := asGeminiChatResponse(response)
	if err != nil {
		return nil, nil, err
	}
	openAIResponse, _ := StreamResponseGeminiChat2OpenAI(geminiResponse)
	usage := UsageFromGeminiMetadata(geminiResponse.GetUsageMetadata(), fallbackPromptTokens(info))
	if openAIResponse != nil {
		openAIResponse.Id = fmt.Sprintf("chatcmpl-%s", kitutil.GetUUID())
		openAIResponse.Created = kitutil.GetTimestamp()
		if info != nil && info.HasChannelMeta() {
			openAIResponse.Model = info.GetUpstreamModelName()
		}
		openAIResponse.Usage = usage
	}
	return openAIResponse, usage, nil
}

func fallbackPromptTokens(info convmeta.Meta) int {
	if info == nil {
		return 0
	}
	return info.GetEstimatePromptTokens()
}

func asOAIChatResponse(response any) (*dto.OpenAITextResponse, error) {
	switch resp := response.(type) {
	case *dto.OpenAITextResponse:
		return resp, nil
	case dto.OpenAITextResponse:
		return &resp, nil
	default:
		return nil, fmt.Errorf("expected OAI chat response, got %T", response)
	}
}

func asOAIChatStreamResponse(response any) (*dto.ChatCompletionsStreamResponse, error) {
	switch resp := response.(type) {
	case *dto.ChatCompletionsStreamResponse:
		return resp, nil
	case dto.ChatCompletionsStreamResponse:
		return &resp, nil
	default:
		return nil, fmt.Errorf("expected OAI chat stream response, got %T", response)
	}
}

func asOAIResponsesResponse(response any) (*dto.OpenAIResponsesResponse, error) {
	switch resp := response.(type) {
	case *dto.OpenAIResponsesResponse:
		return resp, nil
	case dto.OpenAIResponsesResponse:
		return &resp, nil
	default:
		return nil, fmt.Errorf("expected OAI responses response, got %T", response)
	}
}

func asOAIResponsesStreamResponse(response any) (*dto.ResponsesStreamResponse, error) {
	switch resp := response.(type) {
	case *dto.ResponsesStreamResponse:
		return resp, nil
	case dto.ResponsesStreamResponse:
		return &resp, nil
	default:
		return nil, fmt.Errorf("expected OAI responses stream response, got %T", response)
	}
}

func asClaudeResponse(response any) (*dto.ClaudeResponse, error) {
	switch resp := response.(type) {
	case *dto.ClaudeResponse:
		return resp, nil
	case dto.ClaudeResponse:
		return &resp, nil
	default:
		return nil, fmt.Errorf("expected Claude messages response, got %T", response)
	}
}

func asGeminiChatResponse(response any) (*dto.GeminiChatResponse, error) {
	switch resp := response.(type) {
	case *dto.GeminiChatResponse:
		return resp, nil
	case dto.GeminiChatResponse:
		return &resp, nil
	default:
		return nil, fmt.Errorf("expected Gemini chat response, got %T", response)
	}
}

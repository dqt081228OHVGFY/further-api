package relayconvert

import (
	"context"
	"errors"
	"fmt"

	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/relayconvert/convmeta"
	"github.com/dqt081228OHVGFY/further-api/relaykit/relayconvert/internal/toolconv"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
)

type protocolBatch struct {
	values []any
	usage  *dto.Usage
}

// The stream owner serializes conversion, usage snapshots and terminal events.
// Completion runs once even when a disconnect and normal EOF arrive together.
func ConvertStreamResponseChunk(ctx context.Context, info convmeta.Meta, state *ResponseStreamState, response any) ([]ResponseResult, error) {
	if state == nil {
		return nil, errors.New("response stream state is required")
	}
	ctx = conversionContext(ctx)
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.closed {
		return nil, nil
	}
	if state.failed != nil {
		return nil, state.failed
	}
	if err := ctx.Err(); err != nil {
		state.failed = err
		return nil, err
	}
	from, err := inferResponseRelayFormat(response)
	if err != nil {
		return nil, err
	}
	if from != state.From {
		return nil, fmt.Errorf("response stream converter %q expects %s response, got %s", state.Converter, state.From, from)
	}
	info = state.streamMeta(info)
	state.rememberDiagnostics(toolconv.InspectStreamResponse(state.From, state.To, response))
	values, usage := streamValuesFromAny(response), canonicalUsageFromResponse(response)
	if state.From != state.To {
		values, usage, err = executeResponseStreamSteps(ctx, info, state, []any{response}, 0)
		if err != nil {
			state.failed = err
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		state.failed = err
		return nil, err
	}
	state.rememberUsage(usage)
	return responseStreamResults(state, values, usage, state.takeDiagnostics(len(values) != 0)), nil
}

func FinalizeStreamResponse(ctx context.Context, info convmeta.Meta, state *ResponseStreamState) ([]ResponseResult, error) {
	if state == nil {
		return nil, errors.New("response stream state is required")
	}
	ctx = conversionContext(ctx)
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.closed {
		return nil, nil
	}
	if state.failed != nil {
		return nil, state.failed
	}
	if err := ctx.Err(); err != nil {
		state.failed = err
		return nil, err
	}
	info = state.streamMeta(info)
	if state.From == state.To {
		state.closed = true
		return nil, nil
	}
	if state.To == types.RelayFormatClaude {
		details := info.EnsureClaudeConvertInfo()
		if details.Usage == nil {
			details.Usage = state.usageSnapshot()
		}
	}
	output := make([]any, 0)
	var usage *dto.Usage
	for index, spec := range state.specs {
		tail, tailUsage, err := finalizeResponseStreamStep(ctx, info, spec, state.stepStates[index])
		if err != nil {
			state.failed = err
			return nil, err
		}
		if tailUsage != nil {
			usage = tailUsage
			state.rememberUsage(tailUsage)
		}
		if len(tail) == 0 {
			continue
		}
		values, finalUsage, err := executeResponseStreamSteps(ctx, info, state, tail, index+1)
		if err != nil {
			state.failed = err
			return nil, err
		}
		output = append(output, values...)
		if finalUsage != nil {
			usage = finalUsage
			state.rememberUsage(finalUsage)
		}
	}
	if err := ctx.Err(); err != nil {
		state.failed = err
		return nil, err
	}
	state.closed = true
	return responseStreamResults(state, output, usage, state.takeDiagnostics(len(output) != 0)), nil
}

func (s *ResponseStreamState) streamMeta(info convmeta.Meta) convmeta.Meta {
	if info != nil {
		return info
	}
	if s.fallbackInfo == nil {
		s.fallbackInfo = &convmeta.Values{}
	}
	return s.fallbackInfo
}

func (s *ResponseStreamState) Usage() *dto.Usage {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.usageSnapshot()
}
func (s *ResponseStreamState) usageSnapshot() *dto.Usage {
	if s.usage != nil {
		return snapshotUsage(s.usage)
	}
	for _, stage := range s.stepStates {
		if carrier, ok := stage.(responseStreamUsageCarrier); ok {
			if usage := carrier.StreamUsage(); usage != nil {
				return snapshotUsage(usage)
			}
		}
	}
	return nil
}
func snapshotUsage(source *dto.Usage) *dto.Usage {
	if source == nil {
		return nil
	}
	value := *source
	value.PromptTokensDetails = source.PromptTokensDetails.Clone()
	if source.InputTokensDetails != nil {
		copy := source.InputTokensDetails.Clone()
		value.InputTokensDetails = &copy
	}
	if source.OutputTokensDetails != nil {
		copy := *source.OutputTokensDetails
		value.OutputTokensDetails = &copy
	}
	value.BillingUsage = dto.CloneBillingUsage(source.BillingUsage)
	return &value
}
func (s *ResponseStreamState) SetUsage(usage *dto.Usage) {
	if s == nil || usage == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usage = snapshotUsage(usage)
	for _, stage := range s.stepStates {
		if carrier, ok := stage.(responseStreamUsageCarrier); ok {
			carrier.SetStreamUsage(snapshotUsage(s.usage))
		}
	}
}

func (s *ResponseStreamState) FailResponsesStream(code, message, param string) ([]ResponseResult, bool) {
	if s == nil || s.To != types.RelayFormatOpenAIResponses {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, true
	}
	for _, stage := range s.stepStates {
		if stream, ok := stage.(*ChatToResponsesStreamState); ok {
			events := stream.Fail(code, message, param)
			s.closed = true
			return responseStreamResults(s, streamValuesFromAny(events), s.usageSnapshot(), s.takeDiagnostics(len(events) != 0)), true
		}
	}
	return nil, false
}
func (s *ResponseStreamState) UsageText() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, stage := range s.stepStates {
		if text, ok := stage.(interface{ UsageText() string }); ok {
			if value := text.UsageText(); value != "" {
				return value
			}
		}
	}
	return ""
}
func (s *ResponseStreamState) Diagnostics() []types.ConversionDiagnostic {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]types.ConversionDiagnostic(nil), s.diagnostics...)
}

func executeResponseStreamSteps(ctx context.Context, info convmeta.Meta, state *ResponseStreamState, values []any, start int) ([]any, *dto.Usage, error) {
	current := values
	var usage *dto.Usage
	for index := start; index < len(state.specs); index++ {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		spec := state.specs[index]
		batch := make([]any, 0, len(current))
		for _, input := range current {
			if err := ctx.Err(); err != nil {
				return nil, nil, err
			}
			prepareResponseStreamInfo(info, spec)
			output, stepUsage, err := executeResponseStreamStep(ctx, info, spec, state.stepStates[index], input)
			if err != nil {
				return nil, nil, err
			}
			batch = append(batch, output...)
			if stepUsage != nil {
				usage = stepUsage
				state.rememberUsage(stepUsage)
			}
		}
		current = batch
		if len(current) == 0 {
			break
		}
	}
	return current, usage, nil
}
func executeResponseStreamStep(ctx context.Context, info convmeta.Meta, spec ResponseConverterSpec, state, input any) ([]any, *dto.Usage, error) {
	if spec.ConvertStreamChunk == nil && spec.ConvertStream == nil {
		return nil, nil, fmt.Errorf("response converter %q has no stream implementation", spec.ID)
	}
	batch, err := conversionCall(ctx, spec.ID, func(ctx context.Context) (protocolBatch, error) {
		if spec.ConvertStreamChunk != nil {
			values, usage, err := spec.ConvertStreamChunk(ctx, info, input, state)
			return protocolBatch{values, usage}, err
		}
		value, usage, err := spec.ConvertStream(ctx, info, input)
		return protocolBatch{streamValuesFromAny(value), usage}, err
	})
	return batch.values, batch.usage, err
}
func finalizeResponseStreamStep(ctx context.Context, info convmeta.Meta, spec ResponseConverterSpec, state any) ([]any, *dto.Usage, error) {
	if spec.FinalizeStream == nil {
		return nil, nil, nil
	}
	batch, err := conversionCall(ctx, spec.ID, func(ctx context.Context) (protocolBatch, error) {
		values, usage, err := spec.FinalizeStream(ctx, info, state)
		return protocolBatch{values, usage}, err
	})
	return batch.values, batch.usage, err
}
func responseStreamResults(state *ResponseStreamState, values []any, usage *dto.Usage, diagnostics []types.ConversionDiagnostic) []ResponseResult {
	if len(values) == 0 {
		return nil
	}
	result := make([]ResponseResult, len(values))
	for index, value := range values {
		result[index] = ResponseResult{Value: value, Usage: snapshotUsage(usage), From: state.From, To: state.To, Converter: state.Converter, Quality: state.Quality, Steps: append([]ResponseStep(nil), state.Steps...), Stream: true}
		if index == 0 {
			result[index].Diagnostics = append([]types.ConversionDiagnostic(nil), diagnostics...)
		}
	}
	return result
}

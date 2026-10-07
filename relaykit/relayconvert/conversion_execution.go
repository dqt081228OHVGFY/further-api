package relayconvert

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/relayconvert/convmeta"
	"github.com/dqt081228OHVGFY/further-api/relaykit/relayconvert/internal/convdiag"
	"github.com/dqt081228OHVGFY/further-api/relaykit/relayconvert/internal/toolconv"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
)

func conversionContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	// Hosts sometimes hold a nil request context in an interface. Normalize it
	// before invoking Err or Value, just as an untyped nil context is normalized.
	value := reflect.ValueOf(ctx)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if value.IsNil() {
			return context.Background()
		}
	}
	return ctx
}

// Each protocol operation has one cancellation boundary and a contained error
// boundary. A provider codec panic cannot leak its input or terminate a stream.
func conversionCall[T any](ctx context.Context, id string, run func(context.Context) (T, error)) (value T, err error) {
	ctx = conversionContext(ctx)
	if err = ctx.Err(); err != nil {
		return value, err
	}
	defer func() {
		if recover() != nil {
			var empty T
			value = empty
			err = fmt.Errorf("protocol converter %q failed internally", id)
		}
	}()
	value, err = run(ctx)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		var empty T
		value = empty
	}
	return value, err
}

func ConvertRequest(ctx context.Context, info convmeta.Meta, target types.RelayFormat, request any) (*RequestResult, error) {
	ctx = conversionContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	from, err := inferRequestRelayFormat(request)
	if err != nil {
		return nil, err
	}
	if target == "" {
		return nil, errors.New("target relay format is required")
	}
	if from == target {
		return &RequestResult{Value: request, From: from, To: target}, nil
	}
	spec, ok := lookupRequestRoute(from, target)
	if !ok {
		return nil, fmt.Errorf("request converter from %s to %s is not registered", from, target)
	}
	return executeRequestSpec(ctx, info, from, target, request, spec)
}

func ConvertRequestVia(ctx context.Context, info convmeta.Meta, request any, path ...types.RelayFormat) (*RequestResult, error) {
	ctx = conversionContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	from, err := inferRequestRelayFormat(request)
	if err != nil {
		return nil, err
	}
	if len(path) == 0 {
		return nil, errors.New("request conversion path is required")
	}
	for _, target := range path {
		if target == "" {
			return nil, errors.New("request conversion path contains empty relay format")
		}
	}
	targets := path
	if targets[0] == from {
		targets = targets[1:]
	}
	if len(targets) == 0 {
		return &RequestResult{Value: request, From: from, To: from}, nil
	}
	plan := make([]RequestConverterSpec, 0, len(targets))
	current := from
	for _, target := range targets {
		step, ok := lookupRequestDirectRoute(current, target)
		if !ok {
			return nil, fmt.Errorf("request converter from %s to %s is not registered", current, target)
		}
		plan = append(plan, step)
		current = target
	}
	return executeRequestSteps(ctx, info, from, current, request, "", "", plan)
}

func ConvertRequestByID(ctx context.Context, info convmeta.Meta, id string, request any) (*RequestResult, error) {
	ctx = conversionContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	from, err := inferRequestRelayFormat(request)
	if err != nil {
		return nil, err
	}
	spec, ok := LookupRequestConverter(id)
	if !ok {
		return nil, fmt.Errorf("request converter %q is not registered", strings.TrimSpace(id))
	}
	if spec.From != "" && spec.From != from {
		return nil, fmt.Errorf("request converter %q expects %s request, got %s", spec.ID, spec.From, from)
	}
	return executeRequestSpec(ctx, info, from, spec.To, request, spec)
}

func executeRequestSpec(ctx context.Context, info convmeta.Meta, from, to types.RelayFormat, request any, spec RequestConverterSpec) (*RequestResult, error) {
	steps, err := expandRequestConverterSteps(spec)
	if err != nil {
		return nil, err
	}
	return executeRequestSteps(ctx, info, from, to, request, spec.ID, spec.Quality, steps)
}

func executeRequestSteps(ctx context.Context, info convmeta.Meta, from, to types.RelayFormat, request any, id string, quality RequestConverterQuality, plan []RequestConverterSpec) (*RequestResult, error) {
	ctx = conversionContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ctx, diagnostics := convdiag.WithCollector(ctx)
	current, tools, err := toolconv.ExtractRequest(from, request)
	if err != nil {
		return nil, err
	}
	result := &RequestResult{From: from, To: to, Converter: id, Quality: quality, Steps: make([]RequestStep, 0, len(plan))}
	ids := make([]string, 0, len(plan))
	for _, spec := range plan {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		prepared, err := prepareRequestForStep(current, spec, to)
		if err != nil {
			return nil, err
		}
		converted, step, err := executeRequestStep(ctx, info, spec, prepared)
		if err != nil {
			return nil, err
		}
		current = converted
		result.Steps = append(result.Steps, step)
		ids = append(ids, step.Converter)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	current, toolDiagnostics, err := toolconv.AttachRequest(to, current, tools, convmeta.OptionsOf(info))
	result.Value = current
	result.Diagnostics = append(diagnostics.Diagnostics(), toolDiagnostics...)
	for index := range result.Diagnostics {
		if result.Diagnostics[index].From == "" {
			result.Diagnostics[index].From = from
		}
		if result.Diagnostics[index].To == "" {
			result.Diagnostics[index].To = to
		}
	}
	if err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if result.Converter == "" {
		result.Converter = strings.Join(ids, ",")
	}
	if info != nil {
		for _, step := range result.Steps {
			info.AppendRequestConversion(step.To)
		}
	}
	return result, nil
}

func executeRequestStep(ctx context.Context, info convmeta.Meta, spec RequestConverterSpec, request any) (any, RequestStep, error) {
	if spec.Convert == nil {
		return nil, RequestStep{}, fmt.Errorf("request converter %q has no registered implementation", spec.ID)
	}
	value, err := conversionCall(ctx, spec.ID, func(ctx context.Context) (any, error) { return spec.Convert(ctx, info, request) })
	if err != nil {
		return nil, RequestStep{}, err
	}
	return value, RequestStep{Converter: spec.ID, From: spec.From, To: spec.To}, nil
}

func ConvertResponse(ctx context.Context, info convmeta.Meta, to types.RelayFormat, response any) (*ResponseResult, error) {
	return routeResponse(ctx, info, to, response, false)
}
func ConvertStreamResponse(ctx context.Context, info convmeta.Meta, to types.RelayFormat, response any) (*ResponseResult, error) {
	return routeResponse(ctx, info, to, response, true)
}
func routeResponse(ctx context.Context, info convmeta.Meta, to types.RelayFormat, response any, stream bool) (*ResponseResult, error) {
	ctx = conversionContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	from, err := inferResponseRelayFormat(response)
	if err != nil {
		return nil, err
	}
	if to == "" {
		return nil, errors.New("target relay format is required")
	}
	if from == to {
		return &ResponseResult{Value: response, Usage: canonicalUsageFromResponse(response), From: from, To: to, Stream: stream}, nil
	}
	spec, ok := lookupResponseRoute(from, to)
	if !ok {
		return nil, fmt.Errorf("response converter from %s to %s is not registered", from, to)
	}
	if stream {
		return executeStatelessStreamResponseSpec(ctx, info, from, to, response, spec)
	}
	return executeResponseSpec(ctx, info, from, to, response, spec)
}

func ConvertResponseByID(ctx context.Context, info convmeta.Meta, id string, response any) (*ResponseResult, error) {
	ctx = conversionContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	from, err := inferResponseRelayFormat(response)
	if err != nil {
		return nil, err
	}
	spec, ok := LookupResponseConverter(id)
	if !ok {
		return nil, fmt.Errorf("response converter %q is not registered", strings.TrimSpace(id))
	}
	if spec.From != "" && spec.From != from {
		return nil, fmt.Errorf("response converter %q expects %s response, got %s", spec.ID, spec.From, from)
	}
	return executeResponseSpec(ctx, info, from, spec.To, response, spec)
}

type protocolValue struct {
	value any
	usage *dto.Usage
}

func executeResponseSpec(ctx context.Context, info convmeta.Meta, from, to types.RelayFormat, response any, spec ResponseConverterSpec) (*ResponseResult, error) {
	plan, err := expandResponseConverterSteps(spec)
	if err != nil {
		return nil, err
	}
	return executeResponseSteps(ctx, info, from, to, response, spec.ID, spec.Quality, plan)
}
func executeResponseSteps(ctx context.Context, info convmeta.Meta, from, to types.RelayFormat, response any, id string, quality ResponseConverterQuality, plan []ResponseConverterSpec) (*ResponseResult, error) {
	ctx = conversionContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	diagnostics := toolconv.InspectResponse(from, to, response)
	current, hosted, err := toolconv.ExtractHostedResponse(from, response)
	if err != nil {
		return nil, err
	}
	result := &ResponseResult{From: from, To: to, Converter: id, Quality: quality, Steps: make([]ResponseStep, 0, len(plan))}
	ids := make([]string, 0, len(plan))
	for _, spec := range plan {
		converted, usage, step, err := executeResponseStep(ctx, info, spec, current)
		if err != nil {
			return nil, err
		}
		current = converted
		result.Usage = usage
		result.Steps = append(result.Steps, step)
		ids = append(ids, step.Converter)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	value, hostedDiagnostics, err := toolconv.AttachHostedResponse(to, current, hosted, convmeta.OptionsOf(info))
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result.Value = value
	result.Diagnostics = append(diagnostics, hostedDiagnostics...)
	if result.Converter == "" {
		result.Converter = strings.Join(ids, ",")
	}
	return result, nil
}
func executeResponseStep(ctx context.Context, info convmeta.Meta, spec ResponseConverterSpec, response any) (any, *dto.Usage, ResponseStep, error) {
	if spec.Convert == nil {
		return nil, nil, ResponseStep{}, fmt.Errorf("response converter %q has no non-stream implementation", spec.ID)
	}
	converted, err := conversionCall(ctx, spec.ID, func(ctx context.Context) (protocolValue, error) {
		value, usage, err := spec.Convert(ctx, info, response)
		return protocolValue{value, usage}, err
	})
	if err != nil {
		return nil, nil, ResponseStep{}, err
	}
	return converted.value, converted.usage, ResponseStep{Converter: spec.ID, From: spec.From, To: spec.To}, nil
}
func executeStatelessStreamResponseSpec(ctx context.Context, info convmeta.Meta, from, to types.RelayFormat, response any, spec ResponseConverterSpec) (*ResponseResult, error) {
	ctx = conversionContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	plan, err := expandResponseConverterSteps(spec)
	if err != nil {
		return nil, err
	}
	result := &ResponseResult{Value: response, From: from, To: to, Converter: spec.ID, Quality: spec.Quality, Stream: true, Diagnostics: toolconv.InspectStreamResponse(from, to, response), Steps: make([]ResponseStep, 0, len(plan))}
	for _, step := range plan {
		if step.ConvertStream == nil {
			return nil, fmt.Errorf("response converter %q has no stream implementation", step.ID)
		}
		converted, err := conversionCall(ctx, step.ID, func(ctx context.Context) (protocolValue, error) {
			value, usage, err := step.ConvertStream(ctx, info, result.Value)
			return protocolValue{value, usage}, err
		})
		if err != nil {
			return nil, err
		}
		result.Value = converted.value
		result.Usage = converted.usage
		result.Steps = append(result.Steps, ResponseStep{Converter: step.ID, From: step.From, To: step.To})
	}
	return result, nil
}

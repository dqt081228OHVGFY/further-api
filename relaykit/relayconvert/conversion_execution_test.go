package relayconvert

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/relayconvert/convmeta"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
	"github.com/stretchr/testify/require"
)

func TestCanceledConversionsNeverPublishEvenOnIdentityRoutes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := &dto.GeneralOpenAIRequest{Model: "test-model"}
	response := textRegistryChatResponse()
	operations := map[string]func() error{
		"request identity": func() error { _, err := ConvertRequest(ctx, nil, types.RelayFormatOpenAI, request); return err },
		"request path":     func() error { _, err := ConvertRequestVia(ctx, nil, request, types.RelayFormatOpenAI); return err },
		"request converter": func() error {
			_, err := ConvertRequestByID(ctx, nil, ConverterOpenAIChatToOpenAIResponses, request)
			return err
		},
		"response identity": func() error { _, err := ConvertResponse(ctx, nil, types.RelayFormatOpenAI, response); return err },
		"response converter": func() error {
			_, err := ConvertResponseByID(ctx, nil, ResponseConverterOAIChatToOAIResponses, response)
			return err
		},
		"stream identity": func() error { _, err := ConvertStreamResponse(ctx, nil, types.RelayFormatOpenAI, response); return err },
	}
	for name, run := range operations {
		t.Run(name, func(t *testing.T) { require.ErrorIs(t, run(), context.Canceled) })
	}
}

func TestTypedNilHostingContextHasTheSameContractAsNilContext(t *testing.T) {
	type absentContext struct{ context.Context }
	var absent *absentContext
	request := &dto.GeneralOpenAIRequest{Model: "test-model"}
	result, err := ConvertRequest(absent, nil, types.RelayFormatOpenAI, request)
	require.NoError(t, err)
	require.Same(t, request, result.Value)
}

func TestConversionCancellationStopsLaterStepsAndMetadataPublication(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	secondCalled := false
	info := &convmeta.Values{}
	plan := []RequestConverterSpec{
		{ID: "cancel-first", From: types.RelayFormatOpenAI, To: types.RelayFormatOpenAI, Convert: func(_ context.Context, _ convmeta.Meta, value any) (any, error) { cancel(); return value, nil }},
		{ID: "must-not-run", From: types.RelayFormatOpenAI, To: types.RelayFormatOpenAI, Convert: func(_ context.Context, _ convmeta.Meta, value any) (any, error) {
			secondCalled = true
			return value, nil
		}},
	}
	result, err := executeRequestSteps(ctx, info, types.RelayFormatOpenAI, types.RelayFormatOpenAI, &dto.GeneralOpenAIRequest{Model: "test-model"}, "", "", plan)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, result)
	require.False(t, secondCalled)
	require.Empty(t, info.ConversionChain)
}

func TestProviderCodecPanicIsContainedWithoutPublishingPrivateInput(t *testing.T) {
	spec := RequestConverterSpec{ID: "failing-codec", Convert: func(context.Context, convmeta.Meta, any) (any, error) { panic("private-customer-input") }}
	value, _, err := executeRequestStep(context.Background(), nil, spec, &dto.GeneralOpenAIRequest{})
	require.Nil(t, value)
	require.ErrorContains(t, err, "failing-codec")
	require.NotContains(t, err.Error(), "private-customer-input")
}

func TestConcurrentStreamCompletionRunsFinalizerOnlyOnce(t *testing.T) {
	var calls, output atomic.Int32
	state := &ResponseStreamState{From: types.RelayFormatOpenAI, To: types.RelayFormatOpenAIResponses,
		specs: []ResponseConverterSpec{{ID: "terminal", FinalizeStream: func(context.Context, convmeta.Meta, any) ([]any, *dto.Usage, error) {
			calls.Add(1)
			return []any{"terminal-event"}, nil, nil
		}}},
		stepStates: []any{nil},
	}
	var wait sync.WaitGroup
	for index := 0; index < 24; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			values, err := FinalizeStreamResponse(context.Background(), nil, state)
			if err != nil {
				t.Errorf("unexpected finalization error: %v", err)
				return
			}
			output.Add(int32(len(values)))
		}()
	}
	wait.Wait()
	require.Equal(t, int32(1), calls.Load())
	require.Equal(t, int32(1), output.Load())
	values, err := ConvertStreamResponseChunk(context.Background(), nil, state, textRegistryChatResponse())
	require.NoError(t, err)
	require.Empty(t, values)
}

func TestStreamUsageSnapshotsCannotChangeTheFinalBill(t *testing.T) {
	state, err := NewResponseStreamState(types.RelayFormatOpenAI, types.RelayFormatOpenAI, ResponseStreamOptions{})
	require.NoError(t, err)
	usage := &dto.Usage{TotalTokens: 9, InputTokensDetails: &dto.InputTokenDetails{CachedTokens: 4}}
	state.SetUsage(usage)
	usage.TotalTokens = 999
	usage.InputTokensDetails.CachedTokens = 999
	snapshot := state.Usage()
	require.Equal(t, 9, snapshot.TotalTokens)
	require.Equal(t, 4, snapshot.InputTokensDetails.CachedTokens)
	snapshot.TotalTokens = 888
	snapshot.InputTokensDetails.CachedTokens = 888
	require.Equal(t, 9, state.Usage().TotalTokens)
	require.Equal(t, 4, state.Usage().InputTokensDetails.CachedTokens)
}

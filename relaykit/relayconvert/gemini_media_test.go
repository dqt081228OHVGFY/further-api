package relayconvert

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/relaykit/dto"
	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
)

func TestOpenAIChatToGeminiPreservesTextAroundDataImages(t *testing.T) {
	SetMediaResolver(MediaResolver{
		DecodeBase64FileData: func(dataURL string) (string, string, error) {
			if !strings.HasPrefix(dataURL, "data:image/png;base64,") {
				return "", "", errors.New("unexpected data URL")
			}
			return "image/png", strings.TrimPrefix(dataURL, "data:image/png;base64,"), nil
		},
	})
	defer SetMediaResolver(goldenMediaResolver)

	request := &dto.GeneralOpenAIRequest{
		Model: "gemini-test",
		Messages: []dto.Message{{
			Role:    "user",
			Content: "before ![one](data:image/png;base64,YQ==) middle ![two](data:image/png;base64,Yg==) after",
		}},
	}
	result, err := ConvertRequest(context.Background(), nil, types.RelayFormatGemini, request)
	if err != nil {
		t.Fatal(err)
	}
	geminiRequest := result.Value.(*dto.GeminiChatRequest)
	if len(geminiRequest.Contents) != 1 {
		t.Fatalf("got %d Gemini contents, want 1", len(geminiRequest.Contents))
	}
	parts := geminiRequest.Contents[0].Parts
	if len(parts) != 5 {
		t.Fatalf("got %d Gemini parts, want 5: %#v", len(parts), parts)
	}
	if parts[0].Text != "before " || parts[2].Text != " middle " || parts[4].Text != " after" {
		t.Fatalf("text spans changed: %#v", parts)
	}
	if parts[1].InlineData == nil || parts[1].InlineData.Data != "YQ==" ||
		parts[3].InlineData == nil || parts[3].InlineData.Data != "Yg==" {
		t.Fatalf("image order changed: %#v", parts)
	}
}

func TestOpenAIChatToGeminiKeepsNonDataMarkdownLiteral(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{
		Model: "gemini-test",
		Messages: []dto.Message{{
			Role:    "user",
			Content: "see ![remote](https://example.com/image.png) and ![unfinished](data:image/png;base64,",
		}},
	}
	result, err := ConvertRequest(context.Background(), nil, types.RelayFormatGemini, request)
	if err != nil {
		t.Fatal(err)
	}
	parts := result.Value.(*dto.GeminiChatRequest).Contents[0].Parts
	if len(parts) != 1 || parts[0].Text != request.Messages[0].Content {
		t.Fatalf("non-data Markdown changed: %#v", parts)
	}
}

func TestOpenAIResponsesToGeminiUsesSharedMediaBoundary(t *testing.T) {
	SetMediaResolver(MediaResolver{
		GetBase64Data: func(_ context.Context, source types.FileSource, _ ...string) (string, string, error) {
			if source.GetRawData() != "data:image/png;base64,YQ==" {
				return "", "", errors.New("unexpected media source")
			}
			return "YQ==", "image/png", nil
		},
	})
	defer SetMediaResolver(goldenMediaResolver)

	input, err := json.Marshal([]any{map[string]any{
		"role": "user",
		"content": []any{map[string]any{
			"type":      "input_image",
			"image_url": "data:image/png;base64,YQ==",
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	request := &dto.OpenAIResponsesRequest{Model: "gemini-test", Input: input}
	result, err := ConvertRequest(context.Background(), nil, types.RelayFormatGemini, request)
	if err != nil {
		t.Fatal(err)
	}
	parts := result.Value.(*dto.GeminiChatRequest).Contents[0].Parts
	if len(parts) != 1 || parts[0].InlineData == nil || parts[0].InlineData.Data != "YQ==" {
		t.Fatalf("Responses media changed: %#v", parts)
	}
}

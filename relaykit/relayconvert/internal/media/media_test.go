package media

import (
	"context"
	"strings"
	"testing"

	"github.com/dqt081228OHVGFY/further-api/relaykit/types"
)

func TestInlineMediaWithoutHostResolver(t *testing.T) {
	SetMediaResolver(MediaResolver{})
	t.Cleanup(func() { SetMediaResolver(MediaResolver{}) })

	data, mimeType, err := ResolveBase64Data(context.Background(), types.NewBase64FileSource("data:image/png;base64,YQ==", ""))
	if err != nil || data != "YQ==" || mimeType != "image/png" {
		t.Fatalf("data URL: data=%q mime=%q err=%v", data, mimeType, err)
	}

	data, mimeType, err = ResolveBase64Data(context.Background(), types.NewBase64FileSource("Yg==", "audio/wav"))
	if err != nil || data != "Yg==" || mimeType != "audio/wav" {
		t.Fatalf("raw base64: data=%q mime=%q err=%v", data, mimeType, err)
	}

	mimeType, data, err = DecodeBase64FileData("data:image/png;charset=UTF-8;base64,Yw==")
	if err != nil || data != "Yw==" || mimeType != "image/png" {
		t.Fatalf("Markdown image: data=%q mime=%q err=%v", data, mimeType, err)
	}
}

func TestInlineMediaRejectsInvalidData(t *testing.T) {
	SetMediaResolver(MediaResolver{})
	t.Cleanup(func() { SetMediaResolver(MediaResolver{}) })

	for _, input := range []string{
		"data:image/png;base64,@@@",
		"data:image/png,YQ==",
		"data:;base64,YQ==",
		"data:image/png;base64",
	} {
		t.Run(input, func(t *testing.T) {
			_, _, err := ResolveBase64Data(context.Background(), types.NewBase64FileSource(input, ""))
			if err == nil {
				t.Fatal("invalid inline data was accepted")
			}
		})
	}
	_, _, err := ResolveBase64Data(context.Background(), types.NewURLFileSource("https://example.com/image.png"))
	if err == nil || !strings.Contains(err.Error(), "resolver is not configured") {
		t.Fatalf("remote URL without host resolver: %v", err)
	}
}

func TestConfiguredResolverRetainsControl(t *testing.T) {
	SetMediaResolver(MediaResolver{
		GetBase64Data: func(_ context.Context, _ types.FileSource, _ ...string) (string, string, error) {
			return "host-data", "image/webp", nil
		},
	})
	t.Cleanup(func() { SetMediaResolver(MediaResolver{}) })

	data, mimeType, err := ResolveBase64Data(context.Background(), types.NewBase64FileSource("data:image/png;base64,YQ==", ""))
	if err != nil || data != "host-data" || mimeType != "image/webp" {
		t.Fatalf("host resolver bypassed: data=%q mime=%q err=%v", data, mimeType, err)
	}
}

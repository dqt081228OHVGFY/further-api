package claude

import "testing"

func TestMediaMessageUsesAnthropicMediaTypes(t *testing.T) {
	for _, tt := range []struct {
		input string
		kind  string
		mime  string
	}{
		{"IMAGE/PNG; charset=utf-8", "image", "image/png"},
		{"image/jpg", "image", "image/jpeg"},
		{"image/gif", "image", "image/gif"},
		{"application/pdf", "document", "application/pdf"},
	} {
		t.Run(tt.input, func(t *testing.T) {
			message, err := MediaMessage("YQ==", tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if message.Type != tt.kind || message.Source == nil || message.Source.MediaType != tt.mime || message.Source.Data != "YQ==" {
				t.Fatalf("unexpected media block: %#v", message)
			}
		})
	}
	for _, mediaType := range []string{"audio/wav", "video/mp4", "image/heic", "text/plain", ""} {
		t.Run(mediaType, func(t *testing.T) {
			if _, err := MediaMessage("YQ==", mediaType); err == nil {
				t.Fatal("unsupported media was sent to Anthropic")
			}
		})
	}
}

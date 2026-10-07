package gemini

import "testing"

func TestInlineDataPartNormalizesMimeType(t *testing.T) {
	part, err := inlineDataPart(" IMAGE/PNG; charset=UTF-8 ", "YQ==", "test")
	if err != nil {
		t.Fatal(err)
	}
	if part.InlineData == nil || part.InlineData.MimeType != "image/png" || part.InlineData.Data != "YQ==" {
		t.Fatalf("unexpected Gemini inline media: %#v", part)
	}
}

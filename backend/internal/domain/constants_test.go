package domain

import "testing"

func TestDefaultAntigravityModelMapping_ImageCompatibilityAliases(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"claude-opus-4-8":                "claude-opus-4-8",
		"claude-opus-4-7":                "claude-opus-4-7",
		"gemini-2.5-flash-image":         "gemini-2.5-flash-image",
		"gemini-2.5-flash-image-preview": "gemini-2.5-flash-image",
		"gemini-3.1-flash-image":         "gemini-3.1-flash-image",
		"gemini-3.1-flash-image-preview": "gemini-3.1-flash-image",
		"gemini-3-pro-image":             "gemini-3.1-flash-image",
		"gemini-3-pro-image-preview":     "gemini-3.1-flash-image",
	}

	for from, want := range cases {
		got, ok := DefaultAntigravityModelMapping[from]
		if !ok {
			t.Fatalf("expected mapping for %q to exist", from)
		}
		if got != want {
			t.Fatalf("unexpected mapping for %q: got %q want %q", from, got, want)
		}
	}
}

func TestDefaultBedrockModelMapping_IncludesClaudeOpus48(t *testing.T) {
	t.Parallel()

	got, ok := DefaultBedrockModelMapping["claude-opus-4-8"]
	if !ok {
		t.Fatalf("expected bedrock mapping for claude-opus-4-8 to exist")
	}
	if got != "us.anthropic.claude-opus-4-8-v1" {
		t.Fatalf("unexpected bedrock mapping for claude-opus-4-8: got %q", got)
	}
}

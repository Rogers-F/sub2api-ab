package claude

import "testing"

func TestDefaultModels_ContainsModernOpusModels(t *testing.T) {
	t.Parallel()

	models := make(map[string]string, len(DefaultModels))
	for _, model := range DefaultModels {
		models[model.ID] = model.DisplayName
	}
	for id, displayName := range map[string]string{
		"claude-opus-4-8": "Claude Opus 4.8",
		"claude-opus-5":   "Claude Opus 5",
	} {
		if got := models[id]; got != displayName {
			t.Fatalf("unexpected display name for %s: got %q want %q", id, got, displayName)
		}
	}
}

func TestDefaultModels_ContainsClaudeFable5(t *testing.T) {
	t.Parallel()

	for _, model := range DefaultModels {
		if model.ID == "claude-fable-5" {
			if model.DisplayName != "Claude Fable 5" {
				t.Fatalf("unexpected display name for claude-fable-5: got %q", model.DisplayName)
			}
			return
		}
	}

	t.Fatalf("expected claude-fable-5 in DefaultModels")
}

func TestDefaultModels_ContainsClaudeSonnet5(t *testing.T) {
	t.Parallel()

	for _, model := range DefaultModels {
		if model.ID == "claude-sonnet-5" {
			if model.DisplayName != "Claude Sonnet 5" {
				t.Fatalf("unexpected display name for claude-sonnet-5: got %q", model.DisplayName)
			}
			return
		}
	}

	t.Fatalf("expected claude-sonnet-5 in DefaultModels")
}

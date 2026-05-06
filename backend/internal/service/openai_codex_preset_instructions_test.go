package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsOpenAICodexInstructionsModelMatchesPrettyAPIFamily(t *testing.T) {
	cases := map[string]bool{
		"gpt-5.5":                   true,
		"gpt-5.5-pro":               true,
		"gpt-5.4":                   true,
		"gpt-5.2-codex":             true,
		"gpt-5.1-codex-max":         true,
		"codex-mini-latest":         true,
		"openai/gpt-5.5-2026-04-23": true,
		"bengalfox":                 true,
		"boomslang":                 true,
		"gpt-4o":                    false,
		"claude-3-5-sonnet":         false,
		"":                          false,
	}

	for model, want := range cases {
		require.Equal(t, want, isOpenAICodexInstructionsModel(model), model)
	}
}

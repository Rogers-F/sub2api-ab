package openai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodexPresetInstructionsForModelMatchesPrettyAPIPriority(t *testing.T) {
	cases := []struct {
		model string
		want  string
	}{
		{model: "gpt-5.5", want: "You are Codex, a coding agent based on GPT-5"},
		{model: "gpt-5.5-pro", want: "You are Codex, a coding agent based on GPT-5"},
		{model: "gpt-5.3-codex", want: "You are Codex, based on GPT-5"},
		{model: "gpt-5.2", want: "You are GPT-5.2 running in the Codex CLI"},
		{model: "gpt-5.1-codex-max", want: "You are Codex, based on GPT-5"},
		{model: "codex-mini-latest", want: "You are Codex, based on GPT-5"},
	}

	for _, tc := range cases {
		instructions, ok := CodexPresetInstructionsForModel(tc.model)
		require.True(t, ok, tc.model)
		require.Contains(t, instructions, tc.want, tc.model)
	}
}

func TestCodexPresetInstructionsForModelUnmatched(t *testing.T) {
	_, ok := CodexPresetInstructionsForModel("gpt-4o")
	require.False(t, ok)
}

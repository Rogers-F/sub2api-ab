package openai

import (
	"embed"
	"strings"
)

//go:embed codex_prompts/*.md
var codexPromptFiles embed.FS

type codexPresetRule struct {
	prefixes []string
	file     string
}

var codexPresetRules = []codexPresetRule{
	{prefixes: []string{"gpt-5.4"}, file: "gpt-5.4_prompt.md"},
	{prefixes: []string{"gpt-5.5-pro"}, file: "gpt-5.4_prompt.md"},
	{prefixes: []string{"gpt-5.5"}, file: "gpt-5.4_prompt.md"},
	{prefixes: []string{"gpt-5.3-codex"}, file: "gpt-5.3-codex_prompt.md"},
	{prefixes: []string{"gpt-5.2-codex", "bengalfox"}, file: "gpt-5.2-codex_prompt.md"},
	{prefixes: []string{"gpt-5.1-codex-max"}, file: "gpt-5.1-codex-max_prompt.md"},
	{prefixes: []string{"gpt-5-codex", "gpt-5.1-codex", "codex-"}, file: "gpt_5_codex_prompt.md"},
	{prefixes: []string{"gpt-5.2", "boomslang"}, file: "gpt_5_2_prompt.md"},
	{prefixes: []string{"gpt-5.1"}, file: "gpt_5_1_prompt.md"},
}

// CodexPresetInstructionsForModel returns the Codex preset instructions selected
// by the same model-prefix priority used by pretty-api.
func CodexPresetInstructionsForModel(model string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if normalized == "" {
		return "", false
	}
	if strings.Contains(normalized, "/") {
		parts := strings.Split(normalized, "/")
		normalized = parts[len(parts)-1]
	}
	for _, rule := range codexPresetRules {
		for _, prefix := range rule.prefixes {
			if strings.HasPrefix(normalized, prefix) {
				data, err := codexPromptFiles.ReadFile("codex_prompts/" + rule.file)
				if err != nil {
					return "", false
				}
				return string(data), true
			}
		}
	}
	return "", false
}

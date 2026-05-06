package service

import (
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func shouldInjectOpenAICodexPresetInstructions(account *Account, model string) bool {
	return account != nil &&
		account.IsOpenAICodexPresetInstructionsEnabled() &&
		isOpenAICodexInstructionsModel(model)
}

func isOpenAICodexInstructionsModel(model string) bool {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if normalized == "" {
		return false
	}
	if strings.Contains(normalized, "/") {
		parts := strings.Split(normalized, "/")
		normalized = parts[len(parts)-1]
	}
	return strings.HasPrefix(normalized, "gpt-5") ||
		strings.HasPrefix(normalized, "codex-") ||
		strings.HasPrefix(normalized, "bengalfox") ||
		strings.HasPrefix(normalized, "boomslang")
}

func openAICodexPresetInstructions() string {
	return strings.TrimSpace(openai.DefaultInstructions)
}

func injectOpenAICodexPresetInstructionsIntoMap(reqBody map[string]any, account *Account, model string) bool {
	if !shouldInjectOpenAICodexPresetInstructions(account, model) || !isInstructionsEmpty(reqBody) {
		return false
	}
	reqBody["instructions"] = openAICodexPresetInstructions()
	return true
}

func injectOpenAICodexPresetInstructionsIntoJSON(body []byte, account *Account, model string) ([]byte, bool, error) {
	if len(body) == 0 || !shouldInjectOpenAICodexPresetInstructions(account, model) {
		return body, false, nil
	}

	instructions := gjson.GetBytes(body, "instructions")
	if instructions.Exists() {
		if instructions.Type != gjson.String {
			return body, false, nil
		}
		if strings.TrimSpace(instructions.String()) != "" {
			return body, false, nil
		}
	}

	next, err := sjson.SetBytes(body, "instructions", openAICodexPresetInstructions())
	if err != nil {
		return body, false, fmt.Errorf("inject codex preset instructions: %w", err)
	}
	return next, true, nil
}

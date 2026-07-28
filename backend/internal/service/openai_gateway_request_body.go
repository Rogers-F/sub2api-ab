package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
)

// SanitizeOpenAICrossModeFailoverReasoning derives a new attempt body by
// dropping provider-specific encrypted reasoning input items in full. The
// canonical input slice is never mutated.
func SanitizeOpenAICrossModeFailoverReasoning(body []byte) (sanitized []byte, changed bool, err error) {
	if len(body) == 0 || !gjson.GetBytes(body, "input").Exists() {
		return body, false, nil
	}

	var decoded map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return body, false, fmt.Errorf("decode cross-mode failover body: %w", err)
	}
	if !dropOpenAIEncryptedReasoningInputItems(decoded) {
		return body, false, nil
	}

	out, err := json.Marshal(decoded)
	if err != nil {
		return body, false, fmt.Errorf("serialize cross-mode failover body: %w", err)
	}
	return out, true, nil
}

func dropOpenAIEncryptedReasoningInputItems(reqBody map[string]any) bool {
	if len(reqBody) == 0 {
		return false
	}
	inputValue, exists := reqBody["input"]
	if !exists {
		return false
	}

	switch input := inputValue.(type) {
	case []any:
		filtered := input[:0]
		changed := false
		for _, item := range input {
			if isOpenAIEncryptedReasoningInputItem(item) {
				changed = true
				continue
			}
			filtered = append(filtered, item)
		}
		if !changed {
			return false
		}
		if len(filtered) == 0 {
			delete(reqBody, "input")
		} else {
			reqBody["input"] = filtered
		}
		return true
	case []map[string]any:
		filtered := input[:0]
		changed := false
		for _, item := range input {
			if isOpenAIEncryptedReasoningInputItem(item) {
				changed = true
				continue
			}
			filtered = append(filtered, item)
		}
		if !changed {
			return false
		}
		if len(filtered) == 0 {
			delete(reqBody, "input")
		} else {
			reqBody["input"] = filtered
		}
		return true
	case map[string]any:
		if !isOpenAIEncryptedReasoningInputItem(input) {
			return false
		}
		delete(reqBody, "input")
		return true
	default:
		return false
	}
}

func isOpenAIEncryptedReasoningInputItem(item any) bool {
	inputItem, ok := item.(map[string]any)
	if !ok {
		return false
	}
	itemType, _ := inputItem["type"].(string)
	if strings.TrimSpace(itemType) != "reasoning" {
		return false
	}
	_, hasEncryptedContent := inputItem["encrypted_content"]
	return hasEncryptedContent
}

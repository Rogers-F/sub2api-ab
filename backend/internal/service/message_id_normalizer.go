package service

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const bedrockMessageIDPrefix = "msg_bdrk_"

func NormalizeClaudeMessageIDForBedrock(id string) string {
	id = strings.TrimSpace(id)
	if strings.HasPrefix(id, bedrockMessageIDPrefix) {
		return id
	}
	if strings.HasPrefix(id, "msg_") {
		suffix := strings.TrimPrefix(id, "msg_")
		if suffix != "" {
			return bedrockMessageIDPrefix + suffix
		}
	}
	return bedrockMessageIDPrefix + strings.ReplaceAll(uuid.NewString(), "-", "")
}

func NormalizeClaudeMessageIDInJSONBody(body []byte) []byte {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return body
	}
	id := gjson.GetBytes(body, "id")
	if !id.Exists() {
		return body
	}
	normalized := NormalizeClaudeMessageIDForBedrock(id.String())
	out, err := sjson.SetBytes(body, "id", normalized)
	if err != nil {
		return body
	}
	return out
}

func NormalizeClaudeMessageIDInSSEData(data []byte) []byte {
	if len(data) == 0 || !gjson.ValidBytes(data) {
		return data
	}
	if gjson.GetBytes(data, "type").String() != "message_start" {
		return data
	}
	id := gjson.GetBytes(data, "message.id")
	if !id.Exists() {
		return data
	}
	normalized := NormalizeClaudeMessageIDForBedrock(id.String())
	out, err := sjson.SetBytes(data, "message.id", normalized)
	if err != nil {
		return data
	}
	return out
}

func NormalizeClaudeMessageIDEnabledForContext(ctx context.Context) bool {
	group, ok := ctx.Value(ctxkey.Group).(*Group)
	if !ok || !IsGroupContextValid(group) {
		return false
	}
	return group.NormalizeMessageIDEnabled
}

func NormalizeClaudeMessageIDInSSELine(line string) string {
	data, ok := extractAnthropicSSEDataLine(line)
	if !ok {
		return line
	}
	trimmed := strings.TrimSpace(data)
	if trimmed == "" || trimmed == "[DONE]" {
		return line
	}
	normalized := NormalizeClaudeMessageIDInSSEData([]byte(data))
	if string(normalized) == data {
		return line
	}
	prefixLen := strings.Index(line, data)
	if prefixLen < 0 {
		return line
	}
	return line[:prefixLen] + string(normalized)
}

func normalizeClaudeMessageIDInSSEBlock(block string) string {
	if block == "" {
		return block
	}
	lines := strings.SplitAfter(block, "\n")
	for i, line := range lines {
		lineEnding := ""
		lineBody := line
		if strings.HasSuffix(lineBody, "\n") {
			lineEnding = "\n"
			lineBody = strings.TrimSuffix(lineBody, "\n")
			if strings.HasSuffix(lineBody, "\r") {
				lineBody = strings.TrimSuffix(lineBody, "\r")
				lineEnding = "\r\n"
			}
		}
		lines[i] = NormalizeClaudeMessageIDInSSELine(lineBody) + lineEnding
	}
	return strings.Join(lines, "")
}

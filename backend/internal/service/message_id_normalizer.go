package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	anthropicMessageIDPrefix      = "msg_"
	bedrockMessageIDPrefix        = "msg_bdrk_"
	currentMessageIDVersionPrefix = "01"
	currentMessageIDRandomLength  = 22
	modernMessageIDRandomBytes    = 32
	messageIDBase62Alphabet       = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	maxUnbiasedBase62RandomByte   = 248 // Largest multiple of 62 that fits in one byte.
)

var messageIDFallbackCounter atomic.Uint64

var bedrockMessageIDBase32Encoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateClaudeMessageID returns the currently observed Anthropic Messages ID shape.
func GenerateClaudeMessageID() string {
	return anthropicMessageIDPrefix + currentMessageIDVersionPrefix + generateMessageIDBase62(currentMessageIDRandomLength)
}

// GenerateBedrockMessageID returns the legacy Bedrock Claude Messages ID shape.
// Use GenerateBedrockMessageIDForModel when the response model is known.
func GenerateBedrockMessageID() string {
	return bedrockMessageIDPrefix + currentMessageIDVersionPrefix + generateMessageIDBase62(currentMessageIDRandomLength)
}

// GenerateBedrockMessageIDForModel returns the Bedrock ID shape observed for the model generation.
func GenerateBedrockMessageIDForModel(model string) string {
	if UsesModernBedrockMessageSchema(model) {
		return bedrockMessageIDPrefix + generateModernBedrockMessageIDSuffix()
	}
	return GenerateBedrockMessageID()
}

// UsesModernBedrockMessageSchema reports whether the model uses the schema observed on Claude 4.7+.
func UsesModernBedrockMessageSchema(model string) bool {
	_, major, minor, ok := parseClaudeModelVersion(model)
	if !ok {
		return false
	}
	return major >= 5 || (major == 4 && minor >= 7)
}

// OmitsBedrockStopDetails reports the observed Haiku 4.5 response exception.
func OmitsBedrockStopDetails(model string) bool {
	family, major, minor, ok := parseClaudeModelVersion(model)
	if !ok {
		return false
	}
	return family == "haiku" && major == 4 && minor == 5
}

func NormalizeClaudeMessageIDForBedrock(id string) string {
	return NormalizeClaudeMessageIDForBedrockModel(id, "")
}

func NormalizeClaudeMessageIDForBedrockModel(id, model string) string {
	id = strings.TrimSpace(id)
	if strings.HasPrefix(id, bedrockMessageIDPrefix) {
		// IDs received from Bedrock are opaque and may change format over time.
		return id
	}
	if UsesModernBedrockMessageSchema(model) {
		return GenerateBedrockMessageIDForModel(model)
	}
	if strings.HasPrefix(id, anthropicMessageIDPrefix) {
		suffix := strings.TrimPrefix(id, anthropicMessageIDPrefix)
		if isCurrentMessageIDSuffix(suffix) {
			return bedrockMessageIDPrefix + suffix
		}
	}
	return GenerateBedrockMessageIDForModel(model)
}

func isCurrentMessageIDSuffix(suffix string) bool {
	if len(suffix) != len(currentMessageIDVersionPrefix)+currentMessageIDRandomLength ||
		!strings.HasPrefix(suffix, currentMessageIDVersionPrefix) {
		return false
	}
	for _, ch := range suffix[len(currentMessageIDVersionPrefix):] {
		if !strings.ContainsRune(messageIDBase62Alphabet, ch) {
			return false
		}
	}
	return true
}

func generateMessageIDBase62(length int) string {
	result := make([]byte, length)
	var randomBytes [32]byte
	for offset := 0; offset < length; {
		if _, err := rand.Read(randomBytes[:]); err != nil {
			return generateMessageIDFallback(length)
		}
		for _, value := range randomBytes {
			if value >= maxUnbiasedBase62RandomByte {
				continue
			}
			result[offset] = messageIDBase62Alphabet[int(value)%len(messageIDBase62Alphabet)]
			offset++
			if offset == length {
				break
			}
		}
	}
	return string(result)
}

func generateMessageIDFallback(length int) string {
	value := strconv.FormatInt(time.Now().UnixNano(), 36) +
		strconv.FormatUint(messageIDFallbackCounter.Add(1), 36)
	if len(value) < length {
		value = strings.Repeat("0", length-len(value)) + value
	}
	return value[len(value)-length:]
}

func generateModernBedrockMessageIDSuffix() string {
	randomBytes := make([]byte, modernMessageIDRandomBytes)
	if _, err := rand.Read(randomBytes); err != nil {
		fallback := sha256.Sum256([]byte(strconv.FormatInt(time.Now().UnixNano(), 36) +
			strconv.FormatUint(messageIDFallbackCounter.Add(1), 36)))
		randomBytes = fallback[:]
	}
	return strings.ToLower(bedrockMessageIDBase32Encoding.EncodeToString(randomBytes))
}

func NormalizeClaudeMessageIDInJSONBody(body []byte) []byte {
	return NormalizeClaudeMessageIDInJSONBodyForModel(body, "")
}

func NormalizeClaudeMessageIDInJSONBodyForModel(body []byte, model string) []byte {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return body
	}
	id := gjson.GetBytes(body, "id")
	if !id.Exists() {
		return body
	}
	if strings.TrimSpace(model) == "" {
		model = gjson.GetBytes(body, "model").String()
	}
	normalized := NormalizeClaudeMessageIDForBedrockModel(id.String(), model)
	out, err := sjson.SetBytes(body, "id", normalized)
	if err != nil {
		return body
	}
	return out
}

func NormalizeClaudeMessageIDInSSEData(data []byte) []byte {
	return NormalizeClaudeMessageIDInSSEDataForModel(data, "")
}

func NormalizeClaudeMessageIDInSSEDataForModel(data []byte, model string) []byte {
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
	if strings.TrimSpace(model) == "" {
		model = gjson.GetBytes(data, "message.model").String()
	}
	normalized := NormalizeClaudeMessageIDForBedrockModel(id.String(), model)
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
	return NormalizeClaudeMessageIDInSSELineForModel(line, "")
}

func NormalizeClaudeMessageIDInSSELineForModel(line, model string) string {
	data, ok := extractAnthropicSSEDataLine(line)
	if !ok {
		return line
	}
	trimmed := strings.TrimSpace(data)
	if trimmed == "" || trimmed == "[DONE]" {
		return line
	}
	normalized := NormalizeClaudeMessageIDInSSEDataForModel([]byte(data), model)
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

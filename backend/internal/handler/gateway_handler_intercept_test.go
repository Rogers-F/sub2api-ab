package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

var observedBedrockMessageIDPattern = regexp.MustCompile(`^msg_bdrk_01[A-Za-z0-9]{22}$`)
var observedModernBedrockMessageIDPattern = regexp.MustCompile(`^msg_bdrk_[a-z2-7]{51}[aq]$`)

func TestDetectInterceptType_MaxTokensOneHaikuRequiresClaudeCodeClient(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`)

	notClaudeCode := detectInterceptType(body, "claude-haiku-4-5", 1, false, false)
	require.Equal(t, InterceptTypeNone, notClaudeCode)

	isClaudeCode := detectInterceptType(body, "claude-haiku-4-5", 1, false, true)
	require.Equal(t, InterceptTypeMaxTokensOneHaiku, isClaudeCode)
}

func TestDetectInterceptType_SuggestionModeUnaffected(t *testing.T) {
	body := []byte(`{
		"messages":[{
			"role":"user",
			"content":[{"type":"text","text":"[SUGGESTION MODE:foo]"}]
		}],
		"system":[]
	}`)

	got := detectInterceptType(body, "claude-sonnet-4-5", 256, false, false)
	require.Equal(t, InterceptTypeSuggestionMode, got)
}

func TestSendMockInterceptResponse_MaxTokensOneHaiku(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)

	sendMockInterceptResponse(ctx, "claude-haiku-4-5", InterceptTypeMaxTokensOneHaiku)

	require.Equal(t, http.StatusOK, rec.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.Equal(t, "max_tokens", response["stop_reason"])

	id, ok := response["id"].(string)
	require.True(t, ok)
	require.Regexp(t, observedBedrockMessageIDPattern, id)
	require.Len(t, id, 33)

	content, ok := response["content"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, content)

	firstBlock, ok := content[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "#", firstBlock["text"])

	usage, ok := response["usage"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(1), usage["output_tokens"])
	require.NotContains(t, usage, "total_tokens")
	require.NotContains(t, usage, "output_tokens_details")
	require.NotContains(t, usage, "service_tier")
	require.NotContains(t, response, "stop_details")
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
}

func TestSendMockInterceptResponse_BedrockSchemas(t *testing.T) {
	tests := []struct {
		name        string
		model       string
		idPattern   *regexp.Regexp
		modern      bool
		stopDetails bool
	}{
		{name: "legacy opus 4.6", model: "claude-opus-4-6", idPattern: observedBedrockMessageIDPattern, stopDetails: true},
		{name: "legacy sonnet 4.5", model: "claude-sonnet-4-5-20250929", idPattern: observedBedrockMessageIDPattern, stopDetails: true},
		{name: "modern opus 4.8", model: "claude-opus-4-8", idPattern: observedModernBedrockMessageIDPattern, modern: true, stopDetails: true},
		{name: "modern opus 5", model: "claude-opus-5", idPattern: observedModernBedrockMessageIDPattern, modern: true, stopDetails: true},
		{name: "modern sonnet 5", model: "claude-sonnet-5", idPattern: observedModernBedrockMessageIDPattern, modern: true, stopDetails: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rec)

			sendMockInterceptResponse(ctx, tt.model, InterceptTypeWarmup)

			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
			var response map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
			require.Regexp(t, tt.idPattern, response["id"])
			stopDetails, hasStopDetails := response["stop_details"]
			require.Equal(t, tt.stopDetails, hasStopDetails)
			if hasStopDetails {
				require.Nil(t, stopDetails)
			}

			usage, ok := response["usage"].(map[string]any)
			require.True(t, ok)
			require.NotContains(t, usage, "total_tokens")
			require.Equal(t, float64(0), usage["cache_creation_input_tokens"])
			require.Equal(t, float64(0), usage["cache_read_input_tokens"])
			cacheCreation, ok := usage["cache_creation"].(map[string]any)
			require.True(t, ok)
			require.Equal(t, float64(0), cacheCreation["ephemeral_5m_input_tokens"])
			require.Equal(t, float64(0), cacheCreation["ephemeral_1h_input_tokens"])
			if tt.modern {
				require.Equal(t, "standard", usage["service_tier"])
				details, ok := usage["output_tokens_details"].(map[string]any)
				require.True(t, ok)
				require.Equal(t, float64(0), details["thinking_tokens"])
			} else {
				require.NotContains(t, usage, "service_tier")
				require.NotContains(t, usage, "output_tokens_details")
			}
		})
	}
}

func TestSendMockInterceptStream_BedrockSchemas(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		idPattern *regexp.Regexp
		modern    bool
	}{
		{name: "legacy", model: "claude-opus-4-6", idPattern: observedBedrockMessageIDPattern},
		{name: "modern", model: "claude-opus-4-8", idPattern: observedModernBedrockMessageIDPattern, modern: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rec)

			sendMockInterceptStream(ctx, tt.model, InterceptTypeWarmup)

			events := decodeInterceptSSEEvents(t, rec.Body.String())
			messageStart := events["message_start"]
			message, ok := messageStart["message"].(map[string]any)
			require.True(t, ok)
			require.Regexp(t, tt.idPattern, message["id"])
			startUsage, ok := message["usage"].(map[string]any)
			require.True(t, ok)
			require.Equal(t, float64(1), startUsage["output_tokens"])
			assertFullBedrockMockUsage(t, startUsage)

			messageDelta := events["message_delta"]
			deltaUsage, ok := messageDelta["usage"].(map[string]any)
			require.True(t, ok)
			require.Equal(t, float64(2), deltaUsage["output_tokens"])
			assertFullBedrockMockUsage(t, deltaUsage)
			if tt.modern {
				require.Equal(t, "standard", startUsage["service_tier"])
				require.Contains(t, startUsage, "output_tokens_details")
				require.NotContains(t, deltaUsage, "service_tier")
				require.Contains(t, deltaUsage, "output_tokens_details")
			} else {
				require.NotContains(t, startUsage, "service_tier")
				require.NotContains(t, startUsage, "output_tokens_details")
				require.NotContains(t, deltaUsage, "service_tier")
				require.NotContains(t, deltaUsage, "output_tokens_details")
			}

			messageStop := events["message_stop"]
			metrics, ok := messageStop["amazon-bedrock-invocationMetrics"].(map[string]any)
			require.True(t, ok)
			require.Equal(t, float64(10), metrics["inputTokenCount"])
			require.Equal(t, float64(2), metrics["outputTokenCount"])
			require.GreaterOrEqual(t, metrics["invocationLatency"].(float64), metrics["firstByteLatency"].(float64))
		})
	}
}

func decodeInterceptSSEEvents(t *testing.T, body string) map[string]map[string]any {
	t.Helper()
	events := make(map[string]map[string]any)
	for _, block := range strings.Split(strings.TrimSpace(body), "\n\n") {
		var eventType, data string
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "event: "):
				eventType = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				data = strings.TrimPrefix(line, "data: ")
			}
		}
		if eventType == "" || data == "" {
			continue
		}
		var payload map[string]any
		require.NoError(t, json.Unmarshal([]byte(data), &payload))
		events[eventType] = payload
	}
	return events
}

func assertFullBedrockMockUsage(t *testing.T, usage map[string]any) {
	t.Helper()
	require.Equal(t, float64(10), usage["input_tokens"])
	require.Equal(t, float64(0), usage["cache_creation_input_tokens"])
	require.Equal(t, float64(0), usage["cache_read_input_tokens"])
	cacheCreation, ok := usage["cache_creation"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(0), cacheCreation["ephemeral_5m_input_tokens"])
	require.Equal(t, float64(0), cacheCreation["ephemeral_1h_input_tokens"])
}

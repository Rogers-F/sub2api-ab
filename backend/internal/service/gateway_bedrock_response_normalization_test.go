package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newBedrockNormalizationTestRequest() *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	group := &Group{
		ID:                        1,
		Hydrated:                  true,
		Platform:                  PlatformAnthropic,
		Status:                    StatusActive,
		NormalizeMessageIDEnabled: true,
	}
	return req.WithContext(context.WithValue(req.Context(), ctxkey.Group, group))
}

func TestAnthropicPassthroughNormalizesBedrockRequestAndJSONResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = newBedrockNormalizationTestRequest()

	requestBody := []byte(`{"model":"claude-opus-4-8","messages":[{"role":"assistant","content":[{"type":"tool_use","id":"toolu_bdrk_01AbCdEfGhIjKlMnOpQrStUv","name":"lookup","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_bdrk_01AbCdEfGhIjKlMnOpQrStUv","content":"ok"}]}]}`)
	upstreamBody := `{"model":"claude-opus-4-8","id":"msg_01AbCdEfGhIjKlMnOpQrStUv","type":"message","role":"assistant","content":[{"type":"tool_use","id":"toolu_01AbCdEfGhIjKlMnOpQrStUv","name":"lookup","input":{},"caller":{"type":"direct"}}],"stop_reason":"tool_use","stop_sequence":null,"usage":{"input_tokens":12,"output_tokens":7,"cached_tokens":4,"iterations":2}}`
	upstream := &anthropicHTTPUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &GatewayService{
		cfg:              &config.Config{},
		httpUpstream:     upstream,
		rateLimitService: &RateLimitService{},
	}

	result, err := svc.forwardAnthropicAPIKeyPassthrough(context.Background(), c, newAnthropicAPIKeyAccountForTest(), requestBody, "claude-opus-4-8", "claude-opus-4-8", false, time.Now())
	require.NoError(t, err)
	require.Equal(t, "toolu_01AbCdEfGhIjKlMnOpQrStUv", gjson.GetBytes(upstream.lastBody, "messages.0.content.0.id").String())
	require.Equal(t, "toolu_01AbCdEfGhIjKlMnOpQrStUv", gjson.GetBytes(upstream.lastBody, "messages.1.content.0.tool_use_id").String())

	response := rec.Body.Bytes()
	require.Regexp(t, modernBedrockMessageIDPattern, gjson.GetBytes(response, "id").String())
	require.Equal(t, "toolu_bdrk_01AbCdEfGhIjKlMnOpQrStUv", gjson.GetBytes(response, "content.0.id").String())
	require.False(t, gjson.GetBytes(response, "content.0.caller").Exists())
	require.False(t, gjson.GetBytes(response, "usage.cached_tokens").Exists())
	require.False(t, gjson.GetBytes(response, "usage.iterations").Exists())
	require.Equal(t, "standard", gjson.GetBytes(response, "usage.service_tier").String())
	require.Equal(t, int64(0), gjson.GetBytes(response, "usage.output_tokens_details.thinking_tokens").Int())
	require.Equal(t, 4, result.Usage.CacheReadInputTokens)
}

func TestAnthropicPassthroughNormalizesBedrockSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = newBedrockNormalizationTestRequest()

	stream := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"model":"claude-opus-4-8","id":"msg_01AbCdEfGhIjKlMnOpQrStUv","type":"message","role":"assistant","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":11,"output_tokens":0,"output_tokens_details":{"thinking_tokens":0}}}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_01AbCdEfGhIjKlMnOpQrStUv","name":"lookup","input":{},"caller":{"type":"direct"}}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":5,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":0},"service_tier":"source"}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
		``,
	}, "\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(stream)),
	}
	svc := &GatewayService{
		cfg:              &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		rateLimitService: &RateLimitService{},
	}

	result, err := svc.handleStreamingResponseAnthropicAPIKeyPassthrough(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "claude-opus-4-8")
	require.NoError(t, err)
	require.Equal(t, 11, result.usage.InputTokens)
	require.Equal(t, 5, result.usage.OutputTokens)

	start := sseEventData(t, rec.Body.String(), "message_start")
	require.Regexp(t, modernBedrockMessageIDPattern, gjson.Get(start, "message.id").String())
	require.False(t, gjson.Get(start, "message.usage.output_tokens_details").Exists())
	require.Equal(t, "standard", gjson.Get(start, "message.usage.service_tier").String())
	require.True(t, gjson.Get(start, "message.stop_details").Exists())

	block := sseEventData(t, rec.Body.String(), "content_block_start")
	require.Equal(t, "toolu_bdrk_01AbCdEfGhIjKlMnOpQrStUv", gjson.Get(block, "content_block.id").String())
	require.False(t, gjson.Get(block, "content_block.caller").Exists())

	delta := sseEventData(t, rec.Body.String(), "message_delta")
	require.False(t, gjson.Get(delta, "usage.cache_creation").Exists())
	require.False(t, gjson.Get(delta, "usage.service_tier").Exists())
	require.Equal(t, int64(0), gjson.Get(delta, "usage.output_tokens_details.thinking_tokens").Int())
	require.True(t, gjson.Get(delta, "delta.stop_details").Exists())

	stop := sseEventData(t, rec.Body.String(), "message_stop")
	require.Equal(t, int64(11), gjson.Get(stop, "amazon-bedrock-invocationMetrics.inputTokenCount").Int())
	require.Equal(t, int64(5), gjson.Get(stop, "amazon-bedrock-invocationMetrics.outputTokenCount").Int())
}

func TestGenericAnthropicStreamUsesBedrockNormalizer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = newBedrockNormalizationTestRequest()

	stream := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"model":"claude-opus-4-6","id":"msg_01AbCdEfGhIjKlMnOpQrStUv","type":"message","role":"assistant","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":9,"output_tokens":0}}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_01AbCdEfGhIjKlMnOpQrStUv","name":"lookup","input":{}}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":3,"cache_creation":{"ephemeral_5m_input_tokens":2,"ephemeral_1h_input_tokens":0}}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
		``,
	}, "\n")
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(stream))}
	svc := &GatewayService{
		cfg:              &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		rateLimitService: &RateLimitService{},
	}

	result, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "claude-opus-4-6", "claude-opus-4-6", false)
	require.NoError(t, err)
	require.Equal(t, 9, result.usage.InputTokens)
	require.Equal(t, 3, result.usage.OutputTokens)
	require.Equal(t, 2, result.usage.CacheCreation5mTokens)
	require.Regexp(t, currentBedrockMessageIDPattern, gjson.Get(sseEventData(t, rec.Body.String(), "message_start"), "message.id").String())
	require.Equal(t, "toolu_bdrk_01AbCdEfGhIjKlMnOpQrStUv", gjson.Get(sseEventData(t, rec.Body.String(), "content_block_start"), "content_block.id").String())
	require.False(t, gjson.Get(sseEventData(t, rec.Body.String(), "message_delta"), "usage.cache_creation").Exists())
	require.True(t, gjson.Get(sseEventData(t, rec.Body.String(), "message_stop"), "amazon-bedrock-invocationMetrics").Exists())
}

func sseEventData(t *testing.T, stream, wanted string) string {
	t.Helper()
	for _, block := range strings.Split(stream, "\n\n") {
		event := ""
		data := ""
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "event:"):
				event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			case strings.HasPrefix(line, "data:"):
				data = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			}
		}
		if event == wanted {
			return data
		}
	}
	t.Fatalf("SSE event %q not found in %s", wanted, stream)
	return ""
}

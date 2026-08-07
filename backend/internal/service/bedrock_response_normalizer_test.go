package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNormalizeBedrockResponseJSONProfiles(t *testing.T) {
	input := []byte(`{"model":"claude-opus-4-6","id":"msg_01AbCdEfGhIjKlMnOpQrStUv","type":"message","role":"assistant","content":[{"type":"tool_use","id":"toolu_01AbCdEfGhIjKlMnOpQrStUv","name":"lookup","input":{},"caller":{"type":"direct"}}],"context_management":{"edits":[]},"usage":{"input_tokens":2,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":0},"output_tokens":3,"iterations":2,"inference_geo":"global","service_tier":"standard","output_tokens_details":{"thinking_tokens":0}},"stop_reason":"tool_use","stop_sequence":null,"stop_details":{"unexpected":true}}`)
	got := NormalizeBedrockResponseJSON(input, "claude-opus-4-6")

	require.Equal(t, "msg_bdrk_01AbCdEfGhIjKlMnOpQrStUv", gjson.GetBytes(got, "id").String())
	require.Equal(t, "toolu_bdrk_01AbCdEfGhIjKlMnOpQrStUv", gjson.GetBytes(got, "content.0.id").String())
	require.False(t, gjson.GetBytes(got, "content.0.caller").Exists())
	require.Equal(t, `{"edits":[]}`, gjson.GetBytes(got, "context_management").Raw)
	require.False(t, gjson.GetBytes(got, "usage.iterations").Exists())
	require.False(t, gjson.GetBytes(got, "usage.inference_geo").Exists())
	require.False(t, gjson.GetBytes(got, "usage.service_tier").Exists())
	require.False(t, gjson.GetBytes(got, "usage.output_tokens_details").Exists())
	require.Nil(t, gjson.GetBytes(got, "stop_details").Value())

	modern := NormalizeBedrockResponseJSON(input, "claude-opus-4-8")
	require.Equal(t, "standard", gjson.GetBytes(modern, "usage.service_tier").String())
	require.Equal(t, int64(0), gjson.GetBytes(modern, "usage.output_tokens_details.thinking_tokens").Int())
	require.Contains(t, gjson.GetBytes(modern, "id").String(), "msg_bdrk_")
}

func TestNormalizeBedrockResponseJSONThinkingAndHaiku(t *testing.T) {
	input := []byte(`{"model":"claude-haiku-4-5-20251001","id":"msg_01AbCdEfGhIjKlMnOpQrStUv","type":"message","role":"assistant","content":[{"type":"thinking","thinking":"reason","signature":"sig"},{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":2,"output_tokens_details":{"thinking_tokens":1},"service_tier":"standard"},"stop_reason":"end_turn","stop_sequence":null,"stop_details":null}`)
	got := NormalizeBedrockResponseJSON(input, "claude-haiku-4-5-20251001")
	require.True(t, gjson.GetBytes(got, "usage.output_tokens_details.thinking_tokens").Exists())
	require.False(t, gjson.GetBytes(got, "usage.service_tier").Exists())
	require.False(t, gjson.GetBytes(got, "stop_details").Exists())
	require.Regexp(t, currentBedrockMessageIDPattern, gjson.GetBytes(got, "id").String())
}

func TestNormalizeBedrockResponseJSONHaikuServerToolEnvelope(t *testing.T) {
	input := []byte(`{"model":"claude-haiku-4-5-20251001","id":"msg_01AbCdEfGhIjKlMnOpQrStUv","type":"message","role":"assistant","content":[{"type":"server_tool_use","id":"srvtoolu_01AbCdEfGhIjKlMnOpQrStUv","name":"tool_search_tool_regex","input":{}},{"type":"tool_search_tool_result","tool_use_id":"srvtoolu_01AbCdEfGhIjKlMnOpQrStUv","content":{"type":"tool_search_tool_search_result","tool_references":[]}}],"usage":{"input_tokens":1,"output_tokens":2},"stop_reason":"tool_use","stop_sequence":null}`)
	got := NormalizeBedrockResponseJSON(input, "claude-haiku-4-5-20251001")

	require.Regexp(t, modernBedrockMessageIDPattern, gjson.GetBytes(got, "id").String())
	require.True(t, gjson.GetBytes(got, "stop_details").Exists())
	require.Nil(t, gjson.GetBytes(got, "stop_details").Value())
	require.Equal(t, "standard", gjson.GetBytes(got, "usage.service_tier").String())
	require.False(t, gjson.GetBytes(got, "usage.output_tokens_details").Exists())
	require.Equal(t, int64(0), gjson.GetBytes(got, "usage.server_tool_use.web_search_requests").Int())
}

func TestNormalizeBedrockResponseJSONCompletesObservedUsageFields(t *testing.T) {
	input := []byte(`{"model":"claude-opus-4-6","id":"msg_01AbCdEfGhIjKlMnOpQrStUv","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":2},"stop_reason":"end_turn","stop_sequence":null}`)
	got := NormalizeBedrockResponseJSON(input, "claude-opus-4-6")

	require.Equal(t, int64(0), gjson.GetBytes(got, "usage.cache_creation_input_tokens").Int())
	require.Equal(t, int64(0), gjson.GetBytes(got, "usage.cache_read_input_tokens").Int())
	require.Equal(t, int64(0), gjson.GetBytes(got, "usage.cache_creation.ephemeral_5m_input_tokens").Int())
	require.Equal(t, int64(0), gjson.GetBytes(got, "usage.cache_creation.ephemeral_1h_input_tokens").Int())
}

func TestNormalizeBedrockResponseJSONRebuildsWrongBedrockIDProfile(t *testing.T) {
	legacyID := "msg_bdrk_01AbCdEfGhIjKlMnOpQrStUv"
	modernInput := []byte(`{"model":"claude-opus-4-8","id":"` + legacyID + `","type":"message","content":[],"usage":{"input_tokens":1,"output_tokens":1}}`)
	modern := NormalizeBedrockResponseJSON(modernInput, "claude-opus-4-8")
	require.Regexp(t, modernBedrockMessageIDPattern, gjson.GetBytes(modern, "id").String())

	modernID := gjson.GetBytes(modern, "id").String()
	legacyInput := []byte(`{"model":"claude-opus-4-6","id":"` + modernID + `","type":"message","content":[],"usage":{"input_tokens":1,"output_tokens":1}}`)
	legacy := NormalizeBedrockResponseJSON(legacyInput, "claude-opus-4-6")
	require.Regexp(t, currentBedrockMessageIDPattern, gjson.GetBytes(legacy, "id").String())
}

func TestNormalizeBedrockFableOnlyChangesMessageID(t *testing.T) {
	input := []byte(`{"model":"claude-fable-5","id":"msg_01AbCdEfGhIjKlMnOpQrStUv","type":"message","content":[{"type":"tool_use","id":"toolu_01AbCdEfGhIjKlMnOpQrStUv","caller":{"type":"direct"}}],"context_management":{"x":1},"usage":{"service_tier":"source","iterations":3}}`)
	got := NormalizeBedrockResponseJSON(input, "claude-fable-5")
	require.NotEqual(t, gjson.GetBytes(input, "id").String(), gjson.GetBytes(got, "id").String())
	require.Equal(t, gjson.GetBytes(input, "content.0.id").String(), gjson.GetBytes(got, "content.0.id").String())
	require.Equal(t, gjson.GetBytes(input, "context_management").Raw, gjson.GetBytes(got, "context_management").Raw)
	require.Equal(t, gjson.GetBytes(input, "usage.service_tier").String(), gjson.GetBytes(got, "usage.service_tier").String())
}

func TestNormalizeBedrockToolIDsRoundTrip(t *testing.T) {
	input := []byte(`{"messages":[{"role":"assistant","content":[{"type":"server_tool_use","id":"srvtoolu_bdrk_01AbCdEfGhIjKlMnOpQrStUv","name":"search","input":{}},{"type":"tool_use","id":"toolu_bdrk_01AbCdEfGhIjKlMnOpQrStUv","name":"lookup","input":{"type":"tool_use","id":"toolu_bdrk_01AbCdEfGhIjKlMnOpQrStUv","tool_use_id":"toolu_bdrk_01AbCdEfGhIjKlMnOpQrStUv"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_bdrk_01AbCdEfGhIjKlMnOpQrStUv","content":"ok"}]}]}`)
	upstream := NormalizeBedrockRequestToolIDs(input, "claude-opus-4-6")
	require.Equal(t, "srvtoolu_01AbCdEfGhIjKlMnOpQrStUv", gjson.GetBytes(upstream, "messages.0.content.0.id").String())
	require.Equal(t, "toolu_01AbCdEfGhIjKlMnOpQrStUv", gjson.GetBytes(upstream, "messages.0.content.1.id").String())
	require.Equal(t, "toolu_bdrk_01AbCdEfGhIjKlMnOpQrStUv", gjson.GetBytes(upstream, "messages.0.content.1.input.id").String())
	require.Equal(t, "toolu_bdrk_01AbCdEfGhIjKlMnOpQrStUv", gjson.GetBytes(upstream, "messages.0.content.1.input.tool_use_id").String())
	require.Equal(t, "toolu_01AbCdEfGhIjKlMnOpQrStUv", gjson.GetBytes(upstream, "messages.1.content.0.tool_use_id").String())

	response := []byte(`{"model":"claude-opus-4-6","id":"msg_01AbCdEfGhIjKlMnOpQrStUv","type":"message","content":[{"type":"server_tool_use","id":"srvtoolu_01AbCdEfGhIjKlMnOpQrStUv","name":"search","input":{}},{"type":"tool_search_tool_result","tool_use_id":"srvtoolu_01AbCdEfGhIjKlMnOpQrStUv","content":{}},{"type":"tool_use","id":"toolu_01AbCdEfGhIjKlMnOpQrStUv","name":"lookup","input":{"type":"thinking","id":"toolu_01AbCdEfGhIjKlMnOpQrStUv"}}],"usage":{"input_tokens":1,"output_tokens":1,"service_tier":"source"}}`)
	aws := NormalizeBedrockResponseJSON(response, "claude-opus-4-6")
	require.Equal(t, "srvtoolu_bdrk_01AbCdEfGhIjKlMnOpQrStUv", gjson.GetBytes(aws, "content.0.id").String())
	require.Equal(t, "srvtoolu_bdrk_01AbCdEfGhIjKlMnOpQrStUv", gjson.GetBytes(aws, "content.1.tool_use_id").String())
	require.Equal(t, "toolu_bdrk_01AbCdEfGhIjKlMnOpQrStUv", gjson.GetBytes(aws, "content.2.id").String())
	require.Equal(t, "toolu_01AbCdEfGhIjKlMnOpQrStUv", gjson.GetBytes(aws, "content.2.input.id").String())
	require.False(t, gjson.GetBytes(aws, "usage.service_tier").Exists())
	require.False(t, gjson.GetBytes(aws, "usage.output_tokens_details").Exists())
}

func TestBedrockSSENormalizerMatchesObservedEventShapes(t *testing.T) {
	n := NewBedrockSSENormalizer("claude-opus-4-8")
	start := n.NormalizeData([]byte(`{"type":"message_start","message":{"model":"claude-opus-4-8","id":"msg_01AbCdEfGhIjKlMnOpQrStUv","type":"message","content":[],"stop_reason":null,"stop_sequence":null,"stop_details":null,"usage":{"input_tokens":1,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":0},"output_tokens":0,"output_tokens_details":{"thinking_tokens":0},"service_tier":"standard"}}}`), BedrockSSENormalizationOptions{})
	require.False(t, gjson.GetBytes(start, "message.usage.output_tokens_details").Exists())
	require.Equal(t, "standard", gjson.GetBytes(start, "message.usage.service_tier").String())

	blockStart := n.NormalizeData([]byte(`{"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","id":"srvtoolu_01AbCdEfGhIjKlMnOpQrStUv","name":"tool_search_tool_regex","input":{},"caller":{"type":"direct"}}}`), BedrockSSENormalizationOptions{})
	require.Equal(t, "srvtoolu_bdrk_01AbCdEfGhIjKlMnOpQrStUv", gjson.GetBytes(blockStart, "content_block.id").String())
	require.False(t, gjson.GetBytes(blockStart, "content_block.caller").Exists())

	delta := n.NormalizeData([]byte(`{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null,"stop_details":{"x":1}},"context_management":{"x":1},"usage":{"input_tokens":1,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":0},"output_tokens":4,"service_tier":"standard"}}`), BedrockSSENormalizationOptions{})
	require.Equal(t, `{"x":1}`, gjson.GetBytes(delta, "context_management").Raw)
	require.False(t, gjson.GetBytes(delta, "usage.cache_creation").Exists())
	require.False(t, gjson.GetBytes(delta, "usage.service_tier").Exists())
	require.Equal(t, int64(0), gjson.GetBytes(delta, "usage.output_tokens_details.thinking_tokens").Int())
	require.Nil(t, gjson.GetBytes(delta, "delta.stop_details").Value())

	stop := n.NormalizeData([]byte(`{"type":"message_stop"}`), BedrockSSENormalizationOptions{AddInvocationMetrics: true, InputTokens: 1, OutputTokens: 4, InvocationLatency: 12, FirstByteLatency: 5})
	require.Equal(t, int64(1), gjson.GetBytes(stop, "amazon-bedrock-invocationMetrics.inputTokenCount").Int())
	require.Equal(t, int64(4), gjson.GetBytes(stop, "amazon-bedrock-invocationMetrics.outputTokenCount").Int())
	require.Equal(t, int64(12), gjson.GetBytes(stop, "amazon-bedrock-invocationMetrics.invocationLatency").Int())
}

func TestBedrockSSENormalizerHaikuOmitsStopDetails(t *testing.T) {
	n := NewBedrockSSENormalizer("claude-haiku-4-5-20251001")
	data := n.NormalizeData([]byte(`{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null,"stop_details":null},"usage":{"input_tokens":1,"output_tokens":1,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":0}}}`), BedrockSSENormalizationOptions{})
	require.False(t, gjson.GetBytes(data, "delta.stop_details").Exists())
	require.False(t, gjson.GetBytes(data, "usage.cache_creation").Exists())
}

func TestBedrockSSENormalizerInvocationMetricsUseInitialInputTokens(t *testing.T) {
	n := NewBedrockSSENormalizer("claude-opus-4-8")
	n.NormalizeData([]byte(`{"type":"message_start","message":{"id":"msg_01AbCdEfGhIjKlMnOpQrStUv","type":"message","content":[],"usage":{"input_tokens":11,"output_tokens":0}}}`), BedrockSSENormalizationOptions{})
	n.NormalizeData([]byte(`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"input_tokens":29,"output_tokens":7}}`), BedrockSSENormalizationOptions{})

	stop := n.NormalizeData([]byte(`{"type":"message_stop"}`), BedrockSSENormalizationOptions{
		AddInvocationMetrics: true,
		InputTokens:          29,
		OutputTokens:         7,
	})
	require.Equal(t, int64(11), gjson.GetBytes(stop, "amazon-bedrock-invocationMetrics.inputTokenCount").Int())
	require.Equal(t, int64(7), gjson.GetBytes(stop, "amazon-bedrock-invocationMetrics.outputTokenCount").Int())
}

func TestBedrockSSENormalizerKeepsToolIDReferencesStable(t *testing.T) {
	n := NewBedrockSSENormalizer("claude-opus-4-6")
	start := n.NormalizeData([]byte(`{"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","id":"srvtoolu_nonstandard","name":"tool_search_tool_regex","input":{}}}`), BedrockSSENormalizationOptions{})
	id := gjson.GetBytes(start, "content_block.id").String()
	require.Regexp(t, `^srvtoolu_bdrk_01[A-Za-z0-9]{22}$`, id)

	result := n.NormalizeData([]byte(`{"type":"content_block_start","index":1,"content_block":{"type":"tool_search_tool_result","tool_use_id":"srvtoolu_nonstandard","content":{"type":"tool_search_tool_search_result","tool_references":[]}}}`), BedrockSSENormalizationOptions{})
	require.Equal(t, id, gjson.GetBytes(result, "content_block.tool_use_id").String())
}

func TestNormalizeBedrockSSEBlockPreservesEventLines(t *testing.T) {
	n := NewBedrockSSENormalizer("claude-opus-4-6")
	block := "event: content_block_start\n" +
		"data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_01AbCdEfGhIjKlMnOpQrStUv\",\"name\":\"x\",\"input\":{}}}\n\n"
	got := n.NormalizeBlock(block, BedrockSSENormalizationOptions{})
	require.True(t, strings.HasPrefix(got, "event: content_block_start\n"))
	require.Equal(t, "toolu_bdrk_01AbCdEfGhIjKlMnOpQrStUv", gjson.Get(strings.TrimSpace(strings.TrimPrefix(got, "event: content_block_start\n")), "content_block.id").String())
}

func TestNormalizeBedrockResponseJSONKeepsValidJSONNumbers(t *testing.T) {
	body := NormalizeBedrockResponseJSON([]byte(`{"model":"claude-opus-4-6","id":"msg_01AbCdEfGhIjKlMnOpQrStUv","type":"message","content":[],"usage":{"input_tokens":9007199254740991,"output_tokens":1}}`), "claude-opus-4-6")
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(body, &decoded))
	require.Equal(t, float64(9007199254740991), decoded["usage"].(map[string]any)["input_tokens"])
}

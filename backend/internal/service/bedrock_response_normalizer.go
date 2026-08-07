package service

import (
	"bytes"
	"encoding/json"
	"strings"
)

// BedrockSSENormalizationOptions controls the fields synthesized for a
// message_stop event when an upstream Anthropic-compatible stream omits them.
type BedrockSSENormalizationOptions struct {
	AddInvocationMetrics bool
	// AllowLegacyServiceTier preserves an observed service_tier on legacy
	// server-tool responses. Ordinary legacy Claude responses omit it.
	AllowLegacyServiceTier bool
	InputTokens            int
	OutputTokens           int
	InvocationLatency      int
	FirstByteLatency       int
}

// BedrockSSENormalizer keeps the small amount of state needed to decide
// whether a legacy model stream contains thinking tokens.
type BedrockSSENormalizer struct {
	model                  string
	modern                 bool
	omitStopDetail         bool
	fable                  bool
	thinkingSeen           bool
	serverToolSeen         bool
	initialInputTokens     int
	initialInputTokensSeen bool
	toolIDs                map[string]string
}

// NewBedrockSSENormalizer creates a stateful normalizer for one response
// stream. A stream must not be shared between models.
func NewBedrockSSENormalizer(model string) *BedrockSSENormalizer {
	return &BedrockSSENormalizer{
		model:          model,
		modern:         UsesModernBedrockMessageSchema(model),
		omitStopDetail: OmitsBedrockStopDetails(model),
		fable:          IsBedrockFableModel(model),
		toolIDs:        make(map[string]string),
	}
}

// IsBedrockFableModel identifies the temporarily unavailable Fable fixture.
// Fable is intentionally exempt from schema cleanup; only its message ID is
// normalized so callers can keep the same conversation key shape.
func IsBedrockFableModel(model string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(model)), "fable")
}

func bedrockSchemaModel(originalModel, mappedModel string) string {
	if IsBedrockFableModel(originalModel) {
		return originalModel
	}
	if _, _, _, ok := parseClaudeModelVersion(originalModel); ok {
		return originalModel
	}
	if strings.TrimSpace(mappedModel) == "" {
		return originalModel
	}
	return mappedModel
}

// NormalizeBedrockResponseJSON converts a non-streaming Claude message to the
// schema observed from AWS Bedrock. Invalid or non-message JSON is returned
// byte-for-byte unchanged.
func NormalizeBedrockResponseJSON(body []byte, model string) []byte {
	if len(body) == 0 {
		return body
	}
	if IsBedrockFableModel(model) {
		return NormalizeClaudeMessageIDInJSONBodyForModel(body, model)
	}

	var root any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		return body
	}
	object, ok := root.(map[string]any)
	if !ok || object["type"] != "message" {
		return body
	}

	profile := NewBedrockSSENormalizer(model)
	changed := profile.normalizeMessageObject(object)
	if !changed {
		return body
	}
	encoded, err := json.Marshal(root)
	if err != nil {
		return body
	}
	return encoded
}

// NormalizeBedrockRequestToolIDs converts IDs emitted to Bedrock-shaped
// clients back to the upstream Anthropic form. Native Bedrock requests should
// skip this function because Bedrock expects the bdrk IDs unchanged.
func NormalizeBedrockRequestToolIDs(body []byte, model string) []byte {
	if len(body) == 0 || IsBedrockFableModel(model) {
		return body
	}
	var root any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		return body
	}
	changed := normalizeToolIDsInValue(root)
	if !changed {
		return body
	}
	encoded, err := json.Marshal(root)
	if err != nil {
		return body
	}
	return encoded
}

// NormalizeBedrockSSEData normalizes one JSON data line. It is useful for
// passthrough handlers that process SSE line-by-line instead of buffering a
// complete event block.
func (n *BedrockSSENormalizer) NormalizeData(data []byte, options BedrockSSENormalizationOptions) []byte {
	if len(data) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]")) {
		return data
	}
	var event map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&event); err != nil || event == nil {
		return data
	}
	if !n.NormalizeEvent(event, options) {
		return data
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		return data
	}
	return encoded
}

// NormalizeBedrockSSEBlock normalizes every data line in one SSE event block
// while retaining event lines and the original line endings.
func (n *BedrockSSENormalizer) NormalizeBlock(block string, options BedrockSSENormalizationOptions) string {
	if block == "" {
		return block
	}
	lines := strings.SplitAfter(block, "\n")
	for i, line := range lines {
		ending := ""
		body := line
		if strings.HasSuffix(body, "\n") {
			body = strings.TrimSuffix(body, "\n")
			ending = "\n"
			if strings.HasSuffix(body, "\r") {
				body = strings.TrimSuffix(body, "\r")
				ending = "\r\n"
			}
		}
		if data, ok := extractAnthropicSSEDataLine(body); ok {
			normalized := n.NormalizeData([]byte(data), options)
			if string(normalized) != data {
				prefix := body[:len(body)-len(data)]
				body = prefix + string(normalized)
			}
		}
		lines[i] = body + ending
	}
	return strings.Join(lines, "")
}

// NormalizeEvent mutates one decoded SSE event and reports whether anything
// changed. It is intentionally exported for the gateway's buffered SSE path.
func (n *BedrockSSENormalizer) NormalizeEvent(event map[string]any, options BedrockSSENormalizationOptions) bool {
	if event == nil {
		return false
	}
	if n.fable {
		return n.normalizeFableEventID(event)
	}

	changed := n.normalizeResponseIDsAndCallers(event)
	eventType, _ := event["type"].(string)
	switch eventType {
	case "message_start":
		message, _ := event["message"].(map[string]any)
		if message == nil {
			return changed
		}
		if normalizeMessageIDMap(message, n.model) {
			changed = true
		}
		if contentContainsType(message["content"], "thinking") {
			n.thinkingSeen = true
		}
		if n.normalizeStopDetails(message, false) {
			changed = true
		}
		if usage, ok := message["usage"].(map[string]any); ok {
			n.captureInitialInputTokens(usage)
			if normalizeBedrockUsage(usage, n.modern, n.omitStopDetail, bedrockUsageMessageStart, n.thinkingSeen, options.AllowLegacyServiceTier) {
				changed = true
			}
		}
	case "content_block_start":
		block, _ := event["content_block"].(map[string]any)
		if block != nil {
			if typ, _ := block["type"].(string); typ == "thinking" {
				n.thinkingSeen = true
			}
			if typ, _ := block["type"].(string); strings.HasPrefix(typ, "server_tool_use") {
				n.serverToolSeen = true
			}
		}
	case "content_block_delta":
		delta, _ := event["delta"].(map[string]any)
		if delta != nil {
			if typ, _ := delta["type"].(string); typ == "thinking_delta" {
				n.thinkingSeen = true
			}
		}
	case "message_delta":
		if delta, ok := event["delta"].(map[string]any); ok {
			if n.normalizeStopDetails(delta, false) {
				changed = true
			}
		}
		if usage, ok := event["usage"].(map[string]any); ok {
			if normalizeBedrockUsage(usage, n.modern, n.omitStopDetail, bedrockUsageMessageDelta, n.thinkingSeen, options.AllowLegacyServiceTier) {
				changed = true
			}
		}
	case "message_stop":
		if n.initialInputTokensSeen {
			options.InputTokens = n.initialInputTokens
		}
		if options.AddInvocationMetrics && addBedrockInvocationMetrics(event, options) {
			changed = true
		}
	}

	if eventType == "message_delta" && usageHasServerToolUse(event["usage"]) {
		n.serverToolSeen = true
	}
	if n.serverToolSeen && eventType == "message_delta" && messageDeltaIsTerminal(event) {
		if usage, ok := event["usage"].(map[string]any); ok {
			if _, exists := usage["server_tool_use"]; !exists {
				usage["server_tool_use"] = map[string]any{"web_search_requests": 0}
				changed = true
			}
		}
	}
	return changed
}

type bedrockUsageMode uint8

const (
	bedrockUsageMessage bedrockUsageMode = iota
	bedrockUsageMessageStart
	bedrockUsageMessageDelta
)

func (n *BedrockSSENormalizer) normalizeMessageObject(message map[string]any) bool {
	changed := n.normalizeResponseIDsAndCallers(message)
	content := message["content"]
	thinking := contentContainsType(content, "thinking")
	serverTool := contentContainsType(content, "server_tool_use") || contentContainsType(content, "tool_search_tool_result")
	haikuServerToolEnvelope := n.omitStopDetail && serverTool
	if normalizeMessageIDMapForProfile(message, n.model, haikuServerToolEnvelope) {
		changed = true
	}
	if thinking {
		n.thinkingSeen = true
	}
	if n.normalizeStopDetails(message, haikuServerToolEnvelope) {
		changed = true
	}
	if usage, ok := message["usage"].(map[string]any); ok {
		if normalizeBedrockUsage(usage, n.modern, n.omitStopDetail, bedrockUsageMessage, thinking, haikuServerToolEnvelope) {
			changed = true
		}
		if haikuServerToolEnvelope && usage["service_tier"] != "standard" {
			usage["service_tier"] = "standard"
			changed = true
		}
		if serverTool {
			if _, exists := usage["server_tool_use"]; !exists {
				usage["server_tool_use"] = map[string]any{"web_search_requests": 0}
				changed = true
			}
		}
	}
	return changed
}

func (n *BedrockSSENormalizer) captureInitialInputTokens(usage map[string]any) {
	if n.initialInputTokensSeen {
		return
	}
	if value, ok := parseSSEUsageInt(usage["input_tokens"]); ok {
		n.initialInputTokens = value
		n.initialInputTokensSeen = true
	}
}

func (n *BedrockSSENormalizer) normalizeFableEventID(event map[string]any) bool {
	if eventType, _ := event["type"].(string); eventType == "message_start" {
		if message, ok := event["message"].(map[string]any); ok {
			return normalizeMessageIDMap(message, n.model)
		}
	}
	return false
}

func (n *BedrockSSENormalizer) normalizeStopDetails(object map[string]any, forceInclude bool) bool {
	if n.omitStopDetail && !forceInclude {
		return deleteMapKey(object, "stop_details")
	}
	old, exists := object["stop_details"]
	if !exists || old != nil {
		object["stop_details"] = nil
		return true
	}
	return false
}

func normalizeBedrockUsage(usage map[string]any, modern, omitStopDetails bool, mode bedrockUsageMode, thinking, allowLegacyServiceTier bool) bool {
	changed := false
	for _, key := range []string{"input_tokens", "cache_creation_input_tokens", "cache_read_input_tokens", "output_tokens"} {
		if _, exists := usage[key]; !exists {
			usage[key] = 0
			changed = true
		}
	}
	for _, key := range []string{"iterations", "inference_geo", "cached_tokens", "total_tokens"} {
		if deleteMapKey(usage, key) {
			changed = true
		}
	}

	switch mode {
	case bedrockUsageMessageStart:
		// AWS message_start never carries output_tokens_details. Modern models
		// do carry service_tier here; legacy models do not.
		if deleteMapKey(usage, "output_tokens_details") {
			changed = true
		}
		if normalizeBedrockCacheCreation(usage) {
			changed = true
		}
		if modern {
			if usage["service_tier"] != "standard" {
				usage["service_tier"] = "standard"
				changed = true
			}
		} else if !allowLegacyServiceTier && deleteMapKey(usage, "service_tier") {
			changed = true
		}
	case bedrockUsageMessageDelta:
		// AWS message_delta has scalar cache counters only.
		if deleteMapKey(usage, "cache_creation") {
			changed = true
		}
		if deleteMapKey(usage, "service_tier") {
			changed = true
		}
		if modern || thinking {
			if normalizeBedrockOutputDetails(usage) {
				changed = true
			}
		} else if deleteMapKey(usage, "output_tokens_details") {
			changed = true
		}
	default:
		if normalizeBedrockCacheCreation(usage) {
			changed = true
		}
		if modern {
			if usage["service_tier"] != "standard" {
				usage["service_tier"] = "standard"
				changed = true
			}
		} else if !allowLegacyServiceTier && deleteMapKey(usage, "service_tier") {
			changed = true
		}
		if modern || thinking {
			if normalizeBedrockOutputDetails(usage) {
				changed = true
			}
		} else if deleteMapKey(usage, "output_tokens_details") {
			changed = true
		}
	}

	// Keep the parameter for call-site readability and future model profiles.
	_ = omitStopDetails
	return changed
}

func normalizeBedrockCacheCreation(usage map[string]any) bool {
	changed := false
	cache, ok := usage["cache_creation"].(map[string]any)
	if !ok {
		cache = map[string]any{}
		usage["cache_creation"] = cache
		changed = true
	}
	for _, key := range []string{"ephemeral_5m_input_tokens", "ephemeral_1h_input_tokens"} {
		if _, exists := cache[key]; !exists {
			cache[key] = 0
			changed = true
		}
	}
	for key := range cache {
		if key != "ephemeral_5m_input_tokens" && key != "ephemeral_1h_input_tokens" {
			delete(cache, key)
			changed = true
		}
	}
	return changed
}

func normalizeBedrockOutputDetails(usage map[string]any) bool {
	details, ok := usage["output_tokens_details"].(map[string]any)
	changed := false
	if !ok {
		details = map[string]any{}
		usage["output_tokens_details"] = details
		changed = true
	}
	if _, exists := details["thinking_tokens"]; !exists {
		details["thinking_tokens"] = 0
		changed = true
	}
	for key := range details {
		if key != "thinking_tokens" {
			delete(details, key)
			changed = true
		}
	}
	return changed
}

func addBedrockInvocationMetrics(event map[string]any, options BedrockSSENormalizationOptions) bool {
	metrics, ok := event["amazon-bedrock-invocationMetrics"].(map[string]any)
	changed := false
	if !ok {
		metrics = map[string]any{}
		event["amazon-bedrock-invocationMetrics"] = metrics
		changed = true
	}
	invocationLatency := options.InvocationLatency
	if invocationLatency < options.FirstByteLatency {
		invocationLatency = options.FirstByteLatency
	}
	values := map[string]int{
		"inputTokenCount":   options.InputTokens,
		"outputTokenCount":  options.OutputTokens,
		"invocationLatency": invocationLatency,
		"firstByteLatency":  options.FirstByteLatency,
	}
	for key, value := range values {
		if _, exists := metrics[key]; !exists {
			metrics[key] = value
			changed = true
		}
	}
	return changed
}

func normalizeMessageIDMap(object map[string]any, model string) bool {
	return normalizeMessageIDMapForProfile(object, model, false)
}

func normalizeMessageIDMapForProfile(object map[string]any, model string, forceModern bool) bool {
	id, ok := object["id"].(string)
	if !ok || id == "" {
		return false
	}
	modern := forceModern || UsesModernBedrockMessageSchema(model)
	if strings.HasPrefix(id, bedrockMessageIDPrefix) {
		if (modern && isModernBedrockMessageID(id)) || (!modern && isLegacyBedrockMessageID(id)) {
			return false
		}
	}
	normalized := NormalizeClaudeMessageIDForBedrockModel(id, model)
	if modern {
		normalized = bedrockMessageIDPrefix + generateModernBedrockMessageIDSuffix()
	} else if strings.HasPrefix(id, bedrockMessageIDPrefix) {
		normalized = GenerateBedrockMessageID()
	}
	if normalized == id {
		return false
	}
	object["id"] = normalized
	return true
}

func isLegacyBedrockMessageID(id string) bool {
	if !strings.HasPrefix(id, bedrockMessageIDPrefix) {
		return false
	}
	return isCurrentMessageIDSuffix(strings.TrimPrefix(id, bedrockMessageIDPrefix))
}

func isModernBedrockMessageID(id string) bool {
	if !strings.HasPrefix(id, bedrockMessageIDPrefix) {
		return false
	}
	suffix := strings.TrimPrefix(id, bedrockMessageIDPrefix)
	if len(suffix) != 52 || (suffix[len(suffix)-1] != 'a' && suffix[len(suffix)-1] != 'q') {
		return false
	}
	for _, ch := range suffix {
		if (ch < 'a' || ch > 'z') && (ch < '2' || ch > '7') {
			return false
		}
	}
	return true
}

func (n *BedrockSSENormalizer) normalizeResponseIDsAndCallers(value any) bool {
	changed := false
	var visit func(any)
	visit = func(current any) {
		switch object := current.(type) {
		case []any:
			for _, item := range object {
				visit(item)
			}
		case map[string]any:
			typ, _ := object["type"].(string)
			field, isToolBlock := bedrockToolIDField(typ)
			if isToolBlock {
				if deleteMapKey(object, "caller") {
					changed = true
				}
				if id, ok := object[field].(string); ok {
					normalized := n.normalizeToolID(id)
					if normalized != id {
						object[field] = normalized
						changed = true
					}
				}
			}
			for key, item := range object {
				if isToolBlock && isBedrockToolPayloadField(field, key) {
					continue
				}
				visit(item)
			}
		}
	}
	visit(value)
	return changed
}

func (n *BedrockSSENormalizer) normalizeToolID(id string) string {
	if normalized, ok := n.toolIDs[id]; ok {
		return normalized
	}
	normalized := normalizeBedrockToolID(id)
	if normalized != id {
		n.toolIDs[id] = normalized
	}
	return normalized
}

func normalizeToolIDsInValue(value any) bool {
	changed := false
	var visit func(any)
	visit = func(current any) {
		switch object := current.(type) {
		case []any:
			for _, item := range object {
				visit(item)
			}
		case map[string]any:
			typ, _ := object["type"].(string)
			field, isToolBlock := bedrockToolIDField(typ)
			if isToolBlock {
				if id, ok := object[field].(string); ok {
					normalized := denormalizeBedrockToolID(id)
					if normalized != id {
						object[field] = normalized
						changed = true
					}
				}
			}
			for key, item := range object {
				if isToolBlock && isBedrockToolPayloadField(field, key) {
					continue
				}
				visit(item)
			}
		}
	}
	visit(value)
	return changed
}

func bedrockToolIDField(contentType string) (string, bool) {
	switch {
	case contentType == "tool_use" || strings.HasSuffix(contentType, "_tool_use"):
		return "id", true
	case contentType == "tool_result" || strings.HasSuffix(contentType, "_tool_result"):
		return "tool_use_id", true
	default:
		return "", false
	}
}

func isBedrockToolPayloadField(idField, field string) bool {
	return (idField == "id" && field == "input") || (idField == "tool_use_id" && field == "content")
}

func normalizeBedrockToolID(id string) string {
	switch {
	case strings.HasPrefix(id, "toolu_bdrk_") || strings.HasPrefix(id, "srvtoolu_bdrk_"):
		return id
	case strings.HasPrefix(id, "toolu_"):
		suffix := strings.TrimPrefix(id, "toolu_")
		if !isCurrentToolIDSuffix(suffix) {
			suffix = currentMessageIDVersionPrefix + generateMessageIDBase62(currentMessageIDRandomLength)
		}
		return "toolu_bdrk_" + suffix
	case strings.HasPrefix(id, "srvtoolu_"):
		suffix := strings.TrimPrefix(id, "srvtoolu_")
		if !isCurrentToolIDSuffix(suffix) {
			suffix = currentMessageIDVersionPrefix + generateMessageIDBase62(currentMessageIDRandomLength)
		}
		return "srvtoolu_bdrk_" + suffix
	default:
		return id
	}
}

func denormalizeBedrockToolID(id string) string {
	switch {
	case strings.HasPrefix(id, "toolu_bdrk_"):
		return "toolu_" + strings.TrimPrefix(id, "toolu_bdrk_")
	case strings.HasPrefix(id, "srvtoolu_bdrk_"):
		return "srvtoolu_" + strings.TrimPrefix(id, "srvtoolu_bdrk_")
	default:
		return id
	}
}

func isCurrentToolIDSuffix(suffix string) bool {
	return isCurrentMessageIDSuffix(suffix)
}

func contentContainsType(value any, wanted string) bool {
	content, ok := value.([]any)
	if !ok {
		return false
	}
	for _, item := range content {
		if block, ok := item.(map[string]any); ok {
			if typ, _ := block["type"].(string); typ == wanted {
				return true
			}
		}
	}
	return false
}

func usageHasServerToolUse(value any) bool {
	object, ok := value.(map[string]any)
	if !ok {
		return false
	}
	_, ok = object["server_tool_use"]
	return ok
}

func messageDeltaIsTerminal(event map[string]any) bool {
	delta, ok := event["delta"].(map[string]any)
	if !ok {
		return false
	}
	stopReason, ok := delta["stop_reason"].(string)
	return ok && strings.TrimSpace(stopReason) != ""
}

func deleteMapKey(object map[string]any, key string) bool {
	if _, exists := object[key]; !exists {
		return false
	}
	delete(object, key)
	return true
}

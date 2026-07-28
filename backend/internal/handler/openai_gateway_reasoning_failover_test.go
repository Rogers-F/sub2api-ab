package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const openAICrossModeCanonicalBody = `{"model":"gpt-5.6-sol","stream":false,"input":[` +
	`{"type":"message","role":"user","content":"hello"},` +
	`{"type":"reasoning","id":"rs_provider_abc","encrypted_content":"ENC_BLOB","summary":[{"type":"summary_text","text":"thinking"}]},` +
	`{"type":"message","role":"assistant","content":"hi"}` +
	`]}`

func newOpenAIFailoverModeAccount(id int64, passthrough bool) *service.Account {
	return &service.Account{
		ID:       id,
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeAPIKey,
		Extra:    map[string]any{"openai_passthrough": passthrough},
	}
}

func encryptedReasoningItemCount(body []byte) int {
	count := 0
	gjson.GetBytes(body, "input").ForEach(func(_, item gjson.Result) bool {
		if item.Get("type").String() == "reasoning" && item.Get("encrypted_content").Exists() {
			count++
		}
		return true
	})
	return count
}

func TestOpenAIReasoningFailoverStripsEncryptedItemAcrossModes(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	canonical := []byte(openAICrossModeCanonicalBody)
	state := &openAIPassthroughFailoverState{}

	first := h.deriveOpenAIForwardAttemptBody(nil, canonical, newOpenAIFailoverModeAccount(1, true), state)
	second := h.deriveOpenAIForwardAttemptBody(nil, canonical, newOpenAIFailoverModeAccount(2, false), state)

	require.Equal(t, 1, encryptedReasoningItemCount(first))
	require.Equal(t, 0, encryptedReasoningItemCount(second))
	require.NotContains(t, string(second), "rs_provider_abc")
	require.Equal(t, int64(2), gjson.GetBytes(second, "input.#").Int())
	require.JSONEq(t, openAICrossModeCanonicalBody, string(canonical))
}

func TestOpenAIReasoningFailoverSameModePreservesCanonicalBody(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	canonical := []byte(openAICrossModeCanonicalBody)

	for _, tc := range []struct {
		name  string
		modes []bool
	}{
		{name: "non passthrough retries", modes: []bool{false, false}},
		{name: "passthrough retries", modes: []bool{true, true}},
		{name: "switch into passthrough", modes: []bool{false, true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := &openAIPassthroughFailoverState{}
			for idx, passthrough := range tc.modes {
				got := h.deriveOpenAIForwardAttemptBody(nil, canonical, newOpenAIFailoverModeAccount(int64(idx+10), passthrough), state)
				require.Equal(t, 1, encryptedReasoningItemCount(got))
				require.JSONEq(t, openAICrossModeCanonicalBody, string(got))
			}
		})
	}
}

func TestOpenAIReasoningFailoverSanitizationSticksForLaterNonPassthroughAttempts(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	canonical := []byte(openAICrossModeCanonicalBody)
	state := &openAIPassthroughFailoverState{}

	_ = h.deriveOpenAIForwardAttemptBody(nil, canonical, newOpenAIFailoverModeAccount(20, true), state)
	for _, id := range []int64{21, 21, 22} {
		got := h.deriveOpenAIForwardAttemptBody(nil, canonical, newOpenAIFailoverModeAccount(id, false), state)
		require.Equal(t, 0, encryptedReasoningItemCount(got))
	}
	require.JSONEq(t, openAICrossModeCanonicalBody, string(canonical))
}

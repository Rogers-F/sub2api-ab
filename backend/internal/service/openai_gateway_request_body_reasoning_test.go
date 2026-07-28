package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSanitizeOpenAICrossModeFailoverReasoningDropsWholeEncryptedItem(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","input":[` +
		`{"type":"message","role":"user","content":"hi"},` +
		`{"type":"reasoning","id":"rs_provider_1","encrypted_content":"ENC","summary":[{"type":"summary_text","text":"t"}]},` +
		`{"type":"message","role":"assistant","content":"yo"}` +
		`]}`)

	sanitized, changed, err := SanitizeOpenAICrossModeFailoverReasoning(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.NotContains(t, string(sanitized), "rs_provider_1")
	require.NotContains(t, string(sanitized), "encrypted_content")
	require.Equal(t, int64(2), gjson.GetBytes(sanitized, "input.#").Int())
	require.Contains(t, string(sanitized), `"content":"hi"`)
	require.Contains(t, string(sanitized), `"content":"yo"`)
	require.Contains(t, string(body), "rs_provider_1", "canonical input must remain immutable")
}

func TestSanitizeOpenAICrossModeFailoverReasoningNoEncryptedItemIsNoop(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","input":[{"type":"reasoning","summary":[{"type":"summary_text","text":"t"}]}]}`)

	sanitized, changed, err := SanitizeOpenAICrossModeFailoverReasoning(body)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, string(body), string(sanitized))
}

func TestSanitizeOpenAICrossModeFailoverReasoningPreservesLargeIntegers(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","input":[` +
		`{"type":"reasoning","id":"rs_provider_1","encrypted_content":"ENC"},` +
		`{"type":"message","role":"user","content":"hi"}` +
		`],"tools":[{"type":"function","parameters":{"type":"object","properties":{"id":{"const":9007199254740993}}}}]}`)

	sanitized, changed, err := SanitizeOpenAICrossModeFailoverReasoning(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.Contains(t, string(sanitized), `"const":9007199254740993`)
}

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

func TestSanitizeOpenAICrossModeFailoverReasoningPreservesOrdinaryReasoning(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","input":[` +
		`{"type":"reasoning","summary":[{"type":"summary_text","text":"plain"}]},` +
		`{"type":"reasoning","encrypted_content":"ENC","id":"rs_provider"},` +
		`{"type":"message","role":"user","content":"hello"}` +
		`]}`)

	sanitized, changed, err := SanitizeOpenAICrossModeFailoverReasoning(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, int64(2), gjson.GetBytes(sanitized, "input.#").Int())
	require.Equal(t, "reasoning", gjson.GetBytes(sanitized, "input.0.type").String())
	require.Equal(t, "plain", gjson.GetBytes(sanitized, "input.0.summary.0.text").String())
	require.Equal(t, "message", gjson.GetBytes(sanitized, "input.1.type").String())
}

func TestSanitizeOpenAICrossModeFailoverReasoningDropsEncryptedObjectInput(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","input":{"type":"reasoning","id":"rs_provider","encrypted_content":"ENC"}}`)

	sanitized, changed, err := SanitizeOpenAICrossModeFailoverReasoning(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(sanitized, "input").Exists())
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

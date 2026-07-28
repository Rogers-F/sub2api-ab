package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIWSReasoningEffortPolicyAppliesIndependentlyToEveryTurn(t *testing.T) {
	hooks := &OpenAIWSIngressHooks{
		MaxReasoningEffort: "xhigh",
		ReasoningEffortMappings: []ReasoningEffortMapping{
			{From: "max", To: "high"},
		},
	}

	first := applyOpenAIWSIngressReasoningEffortPolicy(hooks, []byte(`{"type":"response.create","model":"gpt-5.6-sol","reasoning":{"effort":"max"}}`))
	second := applyOpenAIWSIngressReasoningEffortPolicy(hooks, []byte(`{"type":"response.create","model":"gpt-5.6-sol","reasoning":{"effort":"low"}}`))
	omitted := applyOpenAIWSIngressReasoningEffortPolicy(hooks, []byte(`{"type":"response.create","model":"gpt-5.6-sol"}`))

	require.Equal(t, "high", gjson.GetBytes(first, "reasoning.effort").String())
	require.Equal(t, "low", gjson.GetBytes(second, "reasoning.effort").String())
	require.False(t, gjson.GetBytes(omitted, "reasoning.effort").Exists())
}

func TestOpenAIWSReasoningEffortPolicyLeavesNeutralPayloadUntouched(t *testing.T) {
	body := []byte(`{"type":"response.create","model":"gpt-5.6-sol","reasoning":{"effort":"max"}}`)
	require.JSONEq(t, string(body), string(applyOpenAIWSIngressReasoningEffortPolicy(nil, body)))
	require.JSONEq(t, string(body), string(applyOpenAIWSIngressReasoningEffortPolicy(&OpenAIWSIngressHooks{}, body)))
}

func TestOpenAIWSPassthroughUsageMetadataUsesOriginalModelCandidate(t *testing.T) {
	metadata := &openAIWSPassthroughUsageMetadata{}
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"gpt-5.6-sol": "gpt-5.6-terra",
			},
		},
	}

	metadata.update(
		[]byte(`{"type":"response.create","model":"gpt-5.6-sol","reasoning":{"effort":"max"}}`),
		account,
		"provider/gpt-5.6-sol-max",
	)
	model, upstreamModel, _, effort := metadata.snapshot()

	require.Equal(t, "gpt-5.6-sol", model)
	require.Equal(t, "gpt-5.6-terra", upstreamModel)
	require.NotNil(t, effort)
	require.Equal(t, "max", *effort)
}

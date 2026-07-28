package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIReasoningEffortPolicyResponsesIngress(t *testing.T) {
	apiKey := &service.APIKey{Group: &service.Group{
		Platform:           service.PlatformOpenAI,
		MaxReasoningEffort: "xhigh",
		ReasoningEffortMappings: []service.ReasoningEffortMapping{
			{From: "max", To: "high"},
		},
	}}

	body := []byte(`{"model":"gpt-5.6-sol","reasoning":{"effort":"max"}}`)
	got := applyOpenAIGroupReasoningEffortPolicy(apiKey, body)

	require.Equal(t, "high", gjson.GetBytes(got, "reasoning.effort").String())
	require.Equal(t, "gpt-5.6-sol", gjson.GetBytes(got, "model").String())
}

func TestOpenAIReasoningEffortPolicyChatCompletionsIngress(t *testing.T) {
	apiKey := &service.APIKey{Group: &service.Group{
		Platform:           service.PlatformOpenAI,
		MaxReasoningEffort: "xhigh",
	}}

	body := []byte(`{"model":"gpt-5.6-sol","messages":[],"reasoning_effort":"max"}`)
	got := applyOpenAIGroupReasoningEffortPolicy(apiKey, body)

	require.Equal(t, "xhigh", gjson.GetBytes(got, "reasoning_effort").String())
}

func TestOpenAIReasoningEffortPolicyIngressPreservesNeutralBodies(t *testing.T) {
	tests := []struct {
		name   string
		apiKey *service.APIKey
		body   string
		raw    bool
	}{
		{
			name: "omitted effort stays omitted",
			apiKey: &service.APIKey{Group: &service.Group{
				Platform:           service.PlatformOpenAI,
				MaxReasoningEffort: "low",
			}},
			body: `{"model":"gpt-5.6-sol"}`,
		},
		{
			name: "non OpenAI group stays neutral",
			apiKey: &service.APIKey{Group: &service.Group{
				Platform:           service.PlatformAnthropic,
				MaxReasoningEffort: "low",
			}},
			body: `{"model":"gpt-5.6-sol","reasoning":{"effort":"max"}}`,
		},
		{
			name: "malformed JSON keeps existing parse error path",
			apiKey: &service.APIKey{Group: &service.Group{
				Platform:           service.PlatformOpenAI,
				MaxReasoningEffort: "low",
			}},
			body: `{"model":`,
			raw:  true,
		},
		{
			name:   "missing group stays neutral",
			apiKey: &service.APIKey{},
			body:   `{"model":"gpt-5.6-sol","reasoning":{"effort":"max"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyOpenAIGroupReasoningEffortPolicy(tt.apiKey, []byte(tt.body))
			if tt.raw {
				require.Equal(t, tt.body, string(got))
				return
			}
			require.JSONEq(t, tt.body, string(got))
		})
	}
}

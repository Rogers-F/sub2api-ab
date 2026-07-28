package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNormalizeMaxReasoningEffort(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "minimal", in: " minimal ", want: "minimal"},
		{name: "hyphen alias", in: "x-high", want: "xhigh"},
		{name: "underscore alias", in: "X_HIGH", want: "xhigh"},
		{name: "long alias", in: "extra-high", want: "xhigh"},
		{name: "max remains distinct", in: "MAX", want: "max"},
		{name: "none is excluded", in: "none", want: ""},
		{name: "ultra is excluded", in: "ultra", want: ""},
		{name: "unknown", in: "future", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, NormalizeMaxReasoningEffort(tt.in))
		})
	}
}

func TestNormalizeMaxReasoningEffortForPlatform(t *testing.T) {
	value, err := normalizeMaxReasoningEffortForPlatform(PlatformOpenAI, " MAX ")
	require.NoError(t, err)
	require.Equal(t, "max", value)

	value, err = normalizeMaxReasoningEffortForPlatform(PlatformAnthropic, "")
	require.NoError(t, err)
	require.Empty(t, value)

	for _, platform := range []string{PlatformAnthropic, PlatformGemini, PlatformAntigravity} {
		_, err = normalizeMaxReasoningEffortForPlatform(platform, "low")
		require.ErrorContains(t, err, `only supported for platform "openai"`)
	}

	_, err = normalizeMaxReasoningEffortForPlatform(PlatformOpenAI, "none")
	require.ErrorContains(t, err, "not supported")
	_, err = normalizeMaxReasoningEffortForPlatform(PlatformOpenAI, "ultra")
	require.ErrorContains(t, err, "not supported")
}

func TestNormalizeReasoningEffortMappings(t *testing.T) {
	t.Run("canonicalizes fixed OpenAI values", func(t *testing.T) {
		got, err := NormalizeReasoningEffortMappings(PlatformOpenAI, []ReasoningEffortMapping{
			{From: " MAX ", To: " x-high "},
			{From: "minimal", To: "HIGH"},
		})
		require.NoError(t, err)
		require.Equal(t, []ReasoningEffortMapping{
			{From: "max", To: "xhigh"},
			{From: "minimal", To: "high"},
		}, got)
	})

	t.Run("normalizes nil to an independent empty slice", func(t *testing.T) {
		got, err := NormalizeReasoningEffortMappings(PlatformOpenAI, nil)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Empty(t, got)
	})

	t.Run("rejects empty and unknown endpoints", func(t *testing.T) {
		for _, mapping := range []ReasoningEffortMapping{
			{From: "max"},
			{From: "future", To: "low"},
			{From: "low", To: "ultra"},
			{From: "none", To: "low"},
		} {
			_, err := NormalizeReasoningEffortMappings(PlatformOpenAI, []ReasoningEffortMapping{mapping})
			require.ErrorContains(t, err, "empty or unknown")
		}
	})

	t.Run("rejects duplicate normalized sources", func(t *testing.T) {
		_, err := NormalizeReasoningEffortMappings(PlatformOpenAI, []ReasoningEffortMapping{
			{From: "x-high", To: "low"},
			{From: " X_HIGH ", To: "medium"},
		})
		require.ErrorContains(t, err, "duplicate")
	})

	t.Run("rejects non OpenAI policies", func(t *testing.T) {
		_, err := NormalizeReasoningEffortMappings(PlatformGemini, []ReasoningEffortMapping{{From: "low", To: "high"}})
		require.ErrorContains(t, err, `only supported for platform "openai"`)
	})

	t.Run("allows an empty neutral policy on other platforms", func(t *testing.T) {
		got, err := NormalizeReasoningEffortMappings(PlatformAnthropic, nil)
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("bounds mapping count", func(t *testing.T) {
		mappings := make([]ReasoningEffortMapping, 65)
		for i := range mappings {
			mappings[i] = ReasoningEffortMapping{From: "low", To: "high"}
		}
		_, err := NormalizeReasoningEffortMappings(PlatformOpenAI, mappings)
		require.ErrorContains(t, err, "cannot exceed 64")
	})

	t.Run("bounds raw values before canonicalization", func(t *testing.T) {
		_, err := NormalizeReasoningEffortMappings(PlatformOpenAI, []ReasoningEffortMapping{
			{From: strings.Repeat("x", 65), To: "high"},
		})
		require.ErrorContains(t, err, "cannot exceed 64 characters")

		_, err = NormalizeReasoningEffortMappings(PlatformOpenAI, []ReasoningEffortMapping{
			{From: "low", To: strings.Repeat(" ", 65) + "high"},
		})
		require.ErrorContains(t, err, "cannot exceed 64 characters")
	})
}

func TestApplyOpenAIReasoningEffortPolicy(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		max      string
		mappings []ReasoningEffortMapping
		path     string
		want     string
		changed  bool
	}{
		{name: "nested caps high", body: `{"reasoning":{"effort":"xhigh"}}`, max: "medium", path: "reasoning.effort", want: "medium", changed: true},
		{name: "flat caps high", body: `{"reasoning_effort":"high"}`, max: "low", path: "reasoning_effort", want: "low", changed: true},
		{name: "does not inject omitted nested", body: `{"model":"gpt-5.6"}`, max: "low", path: "reasoning.effort", want: "", changed: false},
		{name: "does not inject omitted flat", body: `{"model":"gpt-5.6"}`, max: "low", path: "reasoning_effort", want: "", changed: false},
		{name: "keeps lower value", body: `{"reasoning_effort":"low"}`, max: "high", path: "reasoning_effort", want: "low", changed: false},
		{name: "canonicalizes explicit alias", body: `{"reasoning_effort":"x-high"}`, max: "xhigh", path: "reasoning_effort", want: "xhigh", changed: true},
		{name: "caps max below distinct rank", body: `{"reasoning_effort":"max"}`, max: "xhigh", path: "reasoning_effort", want: "xhigh", changed: true},
		{name: "keeps xhigh below max", body: `{"reasoning_effort":"xhigh"}`, max: "max", path: "reasoning_effort", want: "xhigh", changed: false},
		{name: "ignores stale ceiling", body: `{"reasoning_effort":"high"}`, max: "none", path: "reasoning_effort", want: "high", changed: false},
		{name: "caps both request shapes", body: `{"reasoning":{"effort":"high"},"reasoning_effort":"xhigh"}`, max: "low", path: "reasoning.effort", want: "low", changed: true},
		{name: "maps before cap", body: `{"reasoning":{"effort":"MAX"}}`, max: "medium", mappings: []ReasoningEffortMapping{{From: "max", To: "xhigh"}}, path: "reasoning.effort", want: "medium", changed: true},
		{name: "mapping may land below cap", body: `{"reasoning_effort":"max"}`, max: "xhigh", mappings: []ReasoningEffortMapping{{From: "max", To: "high"}}, path: "reasoning_effort", want: "high", changed: true},
		{name: "does not chain mappings", body: `{"reasoning_effort":"max"}`, mappings: []ReasoningEffortMapping{{From: "max", To: "xhigh"}, {From: "xhigh", To: "low"}}, path: "reasoning_effort", want: "xhigh", changed: true},
		{name: "keeps unknown future value", body: `{"reasoning_effort":"future"}`, max: "low", path: "reasoning_effort", want: "future", changed: false},
		{name: "keeps non string value", body: `{"reasoning_effort":{"level":"high"}}`, max: "low", path: "reasoning_effort.level", want: "high", changed: false},
		{name: "keeps empty string", body: `{"reasoning_effort":""}`, max: "low", path: "reasoning_effort", want: "", changed: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := ApplyOpenAIReasoningEffortPolicy([]byte(tt.body), tt.max, tt.mappings)
			require.Equal(t, tt.changed, changed)
			require.Equal(t, tt.want, gjson.GetBytes(got, tt.path).String())
		})
	}
}

func TestSanitizeGroupReasoningEffortPolicy(t *testing.T) {
	group := &Group{
		Platform:                PlatformOpenAI,
		MaxReasoningEffort:      " X-HIGH ",
		ReasoningEffortMappings: []ReasoningEffortMapping{{From: "MAX", To: "HIGH"}},
	}
	sanitizeGroupReasoningEffortPolicy(group)
	require.Equal(t, "xhigh", group.MaxReasoningEffort)
	require.Equal(t, []ReasoningEffortMapping{{From: "max", To: "high"}}, group.ReasoningEffortMappings)

	group.Platform = PlatformGemini
	sanitizeGroupReasoningEffortPolicy(group)
	require.Empty(t, group.MaxReasoningEffort)
	require.NotNil(t, group.ReasoningEffortMappings)
	require.Empty(t, group.ReasoningEffortMappings)
}

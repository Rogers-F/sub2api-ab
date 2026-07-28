package dto

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupFromServiceAdmin_ExposesReasoningEffortPolicy(t *testing.T) {
	group := GroupFromServiceAdmin(&service.Group{
		ID:                 1,
		Name:               "openai",
		Platform:           service.PlatformOpenAI,
		MaxReasoningEffort: "xhigh",
		ReasoningEffortMappings: []service.ReasoningEffortMapping{
			{From: "max", To: "xhigh"},
		},
	})

	require.NotNil(t, group)
	require.Equal(t, "xhigh", group.MaxReasoningEffort)
	require.Equal(t, []service.ReasoningEffortMapping{{From: "max", To: "xhigh"}}, group.ReasoningEffortMappings)
}

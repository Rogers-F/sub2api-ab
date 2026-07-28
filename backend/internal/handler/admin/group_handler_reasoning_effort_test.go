package admin

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupReasoningEffortPolicyRequestBinding(t *testing.T) {
	var create CreateGroupRequest
	err := json.Unmarshal([]byte(`{
		"name":"openai",
		"platform":"openai",
		"max_reasoning_effort":"x-high",
		"reasoning_effort_mappings":[{"from":"max","to":"xhigh"}]
	}`), &create)
	require.NoError(t, err)
	require.Equal(t, "x-high", create.MaxReasoningEffort)
	require.Equal(t, []service.ReasoningEffortMapping{{From: "max", To: "xhigh"}}, create.ReasoningEffortMappings)

	var update UpdateGroupRequest
	err = json.Unmarshal([]byte(`{
		"max_reasoning_effort":"max",
		"reasoning_effort_mappings":[{"from":"max","to":"high"}]
	}`), &update)
	require.NoError(t, err)
	require.NotNil(t, update.MaxReasoningEffort)
	require.Equal(t, "max", *update.MaxReasoningEffort)
	require.NotNil(t, update.ReasoningEffortMappings)
	require.Equal(t, []service.ReasoningEffortMapping{{From: "max", To: "high"}}, *update.ReasoningEffortMappings)
}

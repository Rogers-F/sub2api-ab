package service

import (
	"context"
	"regexp"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

var currentBedrockMessageIDPattern = regexp.MustCompile(`^msg_bdrk_01[A-Za-z0-9]{22}$`)

func TestGenerateMessageIDs(t *testing.T) {
	seen := make(map[string]struct{}, 100)
	for i := 0; i < 100; i++ {
		bedrockID := GenerateBedrockMessageID()
		require.Regexp(t, currentBedrockMessageIDPattern, bedrockID)
		require.Len(t, bedrockID, 33)
		_, duplicate := seen[bedrockID]
		require.False(t, duplicate, bedrockID)
		seen[bedrockID] = struct{}{}

		claudeID := GenerateClaudeMessageID()
		require.Regexp(t, `^msg_01[A-Za-z0-9]{22}$`, claudeID)
		require.Len(t, claudeID, 28)
		require.Equal(t, "msg_bdrk_"+claudeID[len("msg_"):], NormalizeClaudeMessageIDForBedrock(claudeID))
	}
}

func TestNormalizeClaudeMessageIDForBedrock(t *testing.T) {
	require.Equal(t,
		"msg_bdrk_01YHoj1RZ1f1QXTFcwtrrGuD",
		NormalizeClaudeMessageIDForBedrock("msg_01YHoj1RZ1f1QXTFcwtrrGuD"),
	)
	require.Equal(t,
		"msg_bdrk_existing",
		NormalizeClaudeMessageIDForBedrock("msg_bdrk_existing"),
	)

	generated := NormalizeClaudeMessageIDForBedrock("")
	require.Regexp(t, currentBedrockMessageIDPattern, generated)

	nonMessageGenerated := NormalizeClaudeMessageIDForBedrock("response_123")
	require.Regexp(t, currentBedrockMessageIDPattern, nonMessageGenerated)
	require.NotEqual(t, "msg_bdrk_response_123", nonMessageGenerated)

	malformedMessageGenerated := NormalizeClaudeMessageIDForBedrock("msg_abcdef")
	require.Regexp(t, currentBedrockMessageIDPattern, malformedMessageGenerated)
	require.NotEqual(t, "msg_bdrk_abcdef", malformedMessageGenerated)
}

func TestNormalizeClaudeMessageIDInJSONBody(t *testing.T) {
	body := []byte(`{"id":"msg_01YHoj1RZ1f1QXTFcwtrrGuD","type":"message","content":[{"type":"tool_use","id":"toolu_123"}]}`)

	got := NormalizeClaudeMessageIDInJSONBody(body)

	require.JSONEq(t, `{"id":"msg_bdrk_01YHoj1RZ1f1QXTFcwtrrGuD","type":"message","content":[{"type":"tool_use","id":"toolu_123"}]}`, string(got))
	require.Equal(t, "toolu_123", gjson.GetBytes(got, "content.0.id").String())
}

func TestNormalizeClaudeMessageIDInSSEData(t *testing.T) {
	data := []byte(`{"type":"message_start","message":{"id":"msg_01YHoj1RZ1f1QXTFcwtrrGuD","type":"message","content":[]}}`)

	got := NormalizeClaudeMessageIDInSSEData(data)

	require.Equal(t, "msg_bdrk_01YHoj1RZ1f1QXTFcwtrrGuD", gjson.GetBytes(got, "message.id").String())
}

func TestNormalizeClaudeMessageIDInSSEDataLeavesOtherEventsUnchanged(t *testing.T) {
	data := []byte(`{"type":"content_block_start","content_block":{"id":"toolu_123","type":"tool_use"}}`)

	got := NormalizeClaudeMessageIDInSSEData(data)

	require.Equal(t, string(data), string(got))
}

func TestNormalizeClaudeMessageIDEnabledForContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxkey.Group, &Group{
		ID:                        1,
		Hydrated:                  true,
		Platform:                  PlatformAnthropic,
		Status:                    StatusActive,
		NormalizeMessageIDEnabled: true,
	})

	require.True(t, NormalizeClaudeMessageIDEnabledForContext(ctx))
	require.False(t, NormalizeClaudeMessageIDEnabledForContext(context.Background()))
}

func TestNormalizeClaudeMessageIDInSSELine(t *testing.T) {
	line := `data: {"type":"message_start","message":{"id":"msg_01YHoj1RZ1f1QXTFcwtrrGuD","type":"message"}}`

	got := NormalizeClaudeMessageIDInSSELine(line)

	require.Contains(t, got, `"id":"msg_bdrk_01YHoj1RZ1f1QXTFcwtrrGuD"`)
}

func TestNormalizeClaudeMessageIDInSSEBlock(t *testing.T) {
	block := "event: message_start\n" +
		"data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_01YHoj1RZ1f1QXTFcwtrrGuD\",\"type\":\"message\"}}\n\n"

	got := normalizeClaudeMessageIDInSSEBlock(block)

	require.Contains(t, got, `"id":"msg_bdrk_01YHoj1RZ1f1QXTFcwtrrGuD"`)
	require.Contains(t, got, "event: message_start\n")
}

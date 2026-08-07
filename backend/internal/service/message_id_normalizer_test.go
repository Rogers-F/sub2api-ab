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
var modernBedrockMessageIDPattern = regexp.MustCompile(`^msg_bdrk_[a-z2-7]{51}[aq]$`)

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

func TestGenerateBedrockMessageIDForModel(t *testing.T) {
	for _, model := range []string{
		"claude-opus-4-7",
		"claude-opus-4-8",
		"claude-opus-5",
		"claude-sonnet-5",
		"claude-fable-5",
		"us.anthropic.claude-opus-4-8-v1",
	} {
		t.Run(model, func(t *testing.T) {
			seen := make(map[string]struct{}, 100)
			for i := 0; i < 100; i++ {
				id := GenerateBedrockMessageIDForModel(model)
				require.Regexp(t, modernBedrockMessageIDPattern, id)
				require.Len(t, id, 61)
				_, duplicate := seen[id]
				require.False(t, duplicate, id)
				seen[id] = struct{}{}
			}
		})
	}

	for _, model := range []string{
		"claude-opus-4-6",
		"claude-sonnet-4-5-20250929",
		"claude-haiku-4-5-20251001",
		"claude-opus-4-20250514",
		"claude-3-5-sonnet-20241022",
		"unknown-model",
	} {
		t.Run(model, func(t *testing.T) {
			id := GenerateBedrockMessageIDForModel(model)
			require.Regexp(t, currentBedrockMessageIDPattern, id)
			require.Len(t, id, 33)
		})
	}
}

func TestBedrockMessageSchemaModelDetection(t *testing.T) {
	for _, model := range []string{
		"claude-opus-4-7",
		"claude-opus-4.8",
		"claude-opus-5",
		"claude-sonnet-5",
		"claude-fable-5",
		"global.anthropic.claude-opus-5-v1",
	} {
		require.True(t, UsesModernBedrockMessageSchema(model), model)
	}
	for _, model := range []string{
		"claude-opus-4-6",
		"claude-opus-4-5-20251101",
		"claude-sonnet-4-5-20250929",
		"claude-haiku-4-5-20251001",
		"claude-3-5-sonnet-20241022",
	} {
		require.False(t, UsesModernBedrockMessageSchema(model), model)
	}

	require.True(t, OmitsBedrockStopDetails("claude-haiku-4-5-20251001"))
	require.True(t, OmitsBedrockStopDetails("us.anthropic.claude-haiku-4-5-20251001-v1:0"))
	require.False(t, OmitsBedrockStopDetails("claude-sonnet-4-5-20250929"))
	require.False(t, OmitsBedrockStopDetails("claude-haiku-5"))
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

	modernGenerated := NormalizeClaudeMessageIDForBedrockModel("msg_01YHoj1RZ1f1QXTFcwtrrGuD", "claude-opus-4-8")
	require.Regexp(t, modernBedrockMessageIDPattern, modernGenerated)
	require.NotEqual(t, "msg_bdrk_01YHoj1RZ1f1QXTFcwtrrGuD", modernGenerated)

	require.Equal(t,
		"msg_bdrk_opaque-upstream-id",
		NormalizeClaudeMessageIDForBedrockModel("msg_bdrk_opaque-upstream-id", "claude-opus-5"),
	)
}

func TestNormalizeClaudeMessageIDInJSONBody(t *testing.T) {
	body := []byte(`{"id":"msg_01YHoj1RZ1f1QXTFcwtrrGuD","type":"message","content":[{"type":"tool_use","id":"toolu_123"}]}`)

	got := NormalizeClaudeMessageIDInJSONBody(body)

	require.JSONEq(t, `{"id":"msg_bdrk_01YHoj1RZ1f1QXTFcwtrrGuD","type":"message","content":[{"type":"tool_use","id":"toolu_123"}]}`, string(got))
	require.Equal(t, "toolu_123", gjson.GetBytes(got, "content.0.id").String())

	modernBody := []byte(`{"model":"claude-opus-4-8","id":"msg_01YHoj1RZ1f1QXTFcwtrrGuD","type":"message"}`)
	modernGot := NormalizeClaudeMessageIDInJSONBody(modernBody)
	require.Regexp(t, modernBedrockMessageIDPattern, gjson.GetBytes(modernGot, "id").String())
}

func TestNormalizeClaudeMessageIDInSSEData(t *testing.T) {
	data := []byte(`{"type":"message_start","message":{"id":"msg_01YHoj1RZ1f1QXTFcwtrrGuD","type":"message","content":[]}}`)

	got := NormalizeClaudeMessageIDInSSEData(data)

	require.Equal(t, "msg_bdrk_01YHoj1RZ1f1QXTFcwtrrGuD", gjson.GetBytes(got, "message.id").String())

	modernData := []byte(`{"type":"message_start","message":{"model":"claude-sonnet-5","id":"msg_01YHoj1RZ1f1QXTFcwtrrGuD","type":"message"}}`)
	modernGot := NormalizeClaudeMessageIDInSSEData(modernData)
	require.Regexp(t, modernBedrockMessageIDPattern, gjson.GetBytes(modernGot, "message.id").String())
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

package service

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildOAuthMetadataUserID_FallbackWithoutAccountUUID(t *testing.T) {
	svc := &GatewayService{}

	parsed := &ParsedRequest{
		Model:          "claude-sonnet-4-5",
		Stream:         true,
		MetadataUserID: "",
		System:         nil,
		Messages:       nil,
	}

	account := &Account{
		ID:    123,
		Type:  AccountTypeOAuth,
		Extra: map[string]any{}, // intentionally missing account_uuid / claude_user_id
	}

	fp := &Fingerprint{ClientID: "deadbeef"} // should be used as user id in legacy format

	got := svc.buildOAuthMetadataUserID(parsed, account, fp)
	require.NotEmpty(t, got)

	// Legacy format: user_{client}_account__session_{uuid}
	re := regexp.MustCompile(`^user_[a-zA-Z0-9]+_account__session_[a-f0-9-]{36}$`)
	require.True(t, re.MatchString(got), "unexpected user_id format: %s", got)
}

func TestBuildOAuthMetadataUserID_UsesAccountUUIDWhenPresent(t *testing.T) {
	svc := &GatewayService{}

	parsed := &ParsedRequest{
		Model:          "claude-sonnet-4-5",
		Stream:         true,
		MetadataUserID: "",
	}

	account := &Account{
		ID:   123,
		Type: AccountTypeOAuth,
		Extra: map[string]any{
			"account_uuid":      "acc-uuid",
			"claude_user_id":    "clientid123",
			"anthropic_user_id": "",
		},
	}

	got := svc.buildOAuthMetadataUserID(parsed, account, nil)
	require.NotEmpty(t, got)

	// New format: user_{client}_account_{account_uuid}_session_{uuid}
	re := regexp.MustCompile(`^user_clientid123_account_acc-uuid_session_[a-f0-9-]{36}$`)
	require.True(t, re.MatchString(got), "unexpected user_id format: %s", got)
}

func TestBuildOAuthMetadataUserID_SessionIDStableAcrossTurns(t *testing.T) {
	svc := &GatewayService{}
	account := &Account{
		ID:   777,
		Type: AccountTypeOAuth,
		Extra: map[string]any{
			"account_uuid":   "acc-uuid",
			"claude_user_id": "clientid777",
		},
	}
	fp := &Fingerprint{ClientID: "clientid777", UserAgent: "claude-cli/2.1.161 (external, cli)"}
	ctx := &SessionContext{ClientIP: "1.2.3.4", UserAgent: "claude-cli/2.1.161", APIKeyID: 42}

	round1 := &ParsedRequest{
		Model: "claude-sonnet-4-5",
		Messages: []any{
			map[string]any{"role": "user", "content": "first question"},
		},
		SessionContext: ctx,
	}
	round2 := &ParsedRequest{
		Model: "claude-sonnet-4-5",
		Messages: []any{
			map[string]any{"role": "user", "content": "first question"},
			map[string]any{"role": "assistant", "content": "answer 1"},
			map[string]any{"role": "user", "content": "second question"},
		},
		SessionContext: ctx,
	}
	round3 := &ParsedRequest{
		Model: "claude-sonnet-4-5",
		Messages: []any{
			map[string]any{"role": "user", "content": "first question"},
			map[string]any{"role": "assistant", "content": "answer 1"},
			map[string]any{"role": "user", "content": "second question"},
			map[string]any{"role": "assistant", "content": "answer 2"},
			map[string]any{"role": "user", "content": "third question"},
		},
		SessionContext: ctx,
	}

	id1 := svc.buildOAuthMetadataUserID(round1, account, fp)
	id2 := svc.buildOAuthMetadataUserID(round2, account, fp)
	id3 := svc.buildOAuthMetadataUserID(round3, account, fp)

	require.NotEmpty(t, id1)
	require.Equal(t, id1, id2, "metadata.user_id session_id should stay stable as the conversation grows")
	require.Equal(t, id2, id3, "metadata.user_id session_id should stay stable across all turns")

	other := &ParsedRequest{
		Model: "claude-sonnet-4-5",
		Messages: []any{
			map[string]any{"role": "user", "content": "a completely different opener"},
		},
		SessionContext: ctx,
	}
	require.NotEqual(t, id1, svc.buildOAuthMetadataUserID(other, account, fp), "different first user text should derive a different session_id")
}

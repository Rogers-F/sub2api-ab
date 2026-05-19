package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type requestCompatPrefillUpstream struct {
	requestBodies [][]byte
}

func (u *requestCompatPrefillUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	_ = req.Body.Close()
	u.requestBodies = append(u.requestBodies, append([]byte(nil), body...))
	req.Body = io.NopCloser(bytes.NewReader(body))

	if requestCompatLastRole(body) == "assistant" {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(bytes.NewReader([]byte(`{
				"type":"error",
				"error":{
					"type":"invalid_request_error",
					"message":"This model does not support assistant message prefill. The conversation must end with a user message."
				}
			}`))),
		}, nil
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(bytes.NewReader([]byte(`{
			"id":"msg_request_compat",
			"type":"message",
			"role":"assistant",
			"model":"claude-sonnet-4-6",
			"content":[{"type":"text","text":"ok"}],
			"stop_reason":"end_turn",
			"usage":{"input_tokens":1,"output_tokens":1}
		}`))),
	}, nil
}

func (u *requestCompatPrefillUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func TestBuildRequestCompatibilityRetryBody_RemovesFinalAssistantPrefill(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-4-6",
		"max_tokens":100,
		"messages":[
			{"role":"user","content":"Continue this answer"},
			{"role":"assistant","content":[{"type":"text","text":"The answer starts with"}]}
		]
	}`)
	respBody := []byte(`{"type":"error","error":{"message":"This model does not support assistant message prefill. The conversation must end with a user message."}}`)

	out, kind, ok := BuildRequestCompatibilityRetryBody(body, respBody)
	require.True(t, ok)
	require.Equal(t, "assistant_prefill", kind)
	require.Equal(t, int64(1), gjson.GetBytes(out, "messages.#").Int())
	require.Equal(t, "user", requestCompatLastRole(out))
	require.NotContains(t, string(out), "The answer starts with")
}

func TestBuildRequestCompatibilityRetryBody_KeepsAssistantToolUseUnchanged(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-4-6",
		"max_tokens":100,
		"messages":[
			{"role":"user","content":"Use a tool"},
			{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"pwd"}}]}
		]
	}`)
	respBody := []byte(`{"type":"error","error":{"message":"This model does not support assistant message prefill. The conversation must end with a user message."}}`)

	out, _, ok := BuildRequestCompatibilityRetryBody(body, respBody)
	require.False(t, ok)
	require.Equal(t, string(body), string(out))
}

func TestBuildRequestCompatibilityRetryBody_StripsInvalidRedactedThinkingData(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-4-6",
		"max_tokens":100,
		"thinking":{"type":"enabled","budget_tokens":1024},
		"messages":[
			{"role":"assistant","content":[
				{"type":"redacted_thinking","data":"bad"},
				{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"pwd"}},
				{"type":"text","text":"Visible"}
			]}
		]
	}`)
	respBody := []byte(`{"type":"error","error":{"message":"messages.0.content.0: Invalid ` + "`data`" + ` in ` + "`redacted_thinking`" + ` block"}}`)

	out, kind, ok := BuildRequestCompatibilityRetryBody(body, respBody)
	require.True(t, ok)
	require.Equal(t, "redacted_thinking", kind)
	require.False(t, gjson.GetBytes(out, "thinking").Exists())
	require.Equal(t, int64(2), gjson.GetBytes(out, "messages.0.content.#").Int())
	require.Equal(t, "tool_use", gjson.GetBytes(out, "messages.0.content.0.type").String())
	require.Equal(t, "pwd", gjson.GetBytes(out, "messages.0.content.0.input.command").String())
	require.NotContains(t, string(out), "redacted_thinking")
}

func TestGatewayService_Forward_GroupRequestCompatRetriesAssistantPrefill(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Anthropic-Version", "2023-06-01")

	body := []byte(`{
		"model":"claude-sonnet-4-6",
		"max_tokens":100,
		"stream":false,
		"messages":[
			{"role":"user","content":"Continue this answer"},
			{"role":"assistant","content":[{"type":"text","text":"The answer starts with"}]}
		]
	}`)

	upstream := &requestCompatPrefillUpstream{}
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}
	svc := &GatewayService{
		cfg:                  cfg,
		responseHeaderFilter: compileResponseHeaderFilter(cfg),
		httpUpstream:         upstream,
		rateLimitService:     &RateLimitService{},
		settingService:       NewSettingService(signatureCompatSettingRepo{}, &config.Config{}),
	}

	account := &Account{
		ID:          16,
		Name:        "api-key-upstream",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "upstream-anthropic-key",
			"base_url": "https://api.anthropic.com",
		},
		Status:      StatusActive,
		Schedulable: true,
	}

	group := &Group{
		ID:                   7,
		Platform:             PlatformAnthropic,
		Status:               StatusActive,
		Hydrated:             true,
		RequestCompatEnabled: true,
	}
	ctx := context.WithValue(context.Background(), ctxkey.Group, group)

	result, err := svc.Forward(ctx, c, account, &ParsedRequest{
		Body:    body,
		Model:   "claude-sonnet-4-6",
		Stream:  false,
		GroupID: &group.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requestBodies, 2)
	require.Equal(t, "assistant", requestCompatLastRole(upstream.requestBodies[0]))
	require.Equal(t, "user", requestCompatLastRole(upstream.requestBodies[1]))
	require.Contains(t, rec.Body.String(), `"text":"ok"`)
}

func TestGatewayService_Forward_GroupRequestCompatDisabledDoesNotRetryAssistantPrefill(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Anthropic-Version", "2023-06-01")

	body := []byte(`{
		"model":"claude-sonnet-4-6",
		"max_tokens":100,
		"stream":false,
		"messages":[
			{"role":"user","content":"Continue this answer"},
			{"role":"assistant","content":[{"type":"text","text":"The answer starts with"}]}
		]
	}`)

	upstream := &requestCompatPrefillUpstream{}
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}
	svc := &GatewayService{
		cfg:                  cfg,
		responseHeaderFilter: compileResponseHeaderFilter(cfg),
		httpUpstream:         upstream,
		rateLimitService:     &RateLimitService{},
		settingService:       NewSettingService(signatureCompatSettingRepo{}, &config.Config{}),
	}

	account := &Account{
		ID:          16,
		Name:        "api-key-upstream",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "upstream-anthropic-key",
			"base_url": "https://api.anthropic.com",
		},
		Status:      StatusActive,
		Schedulable: true,
	}

	group := &Group{
		ID:                   7,
		Platform:             PlatformAnthropic,
		Status:               StatusActive,
		Hydrated:             true,
		RequestCompatEnabled: false,
	}
	ctx := context.WithValue(context.Background(), ctxkey.Group, group)

	result, err := svc.Forward(ctx, c, account, &ParsedRequest{
		Body:    body,
		Model:   "claude-sonnet-4-6",
		Stream:  false,
		GroupID: &group.ID,
	})
	require.Error(t, err)
	require.Nil(t, result)
	require.Len(t, upstream.requestBodies, 1)
}

func requestCompatLastRole(body []byte) string {
	count := gjson.GetBytes(body, "messages.#").Int()
	if count <= 0 {
		return ""
	}
	return gjson.GetBytes(body, fmt.Sprintf("messages.%d.role", count-1)).String()
}

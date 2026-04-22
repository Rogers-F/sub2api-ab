package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type signatureCompatSettingRepo struct{}

func (signatureCompatSettingRepo) Get(context.Context, string) (*Setting, error) {
	return nil, ErrSettingNotFound
}

func (signatureCompatSettingRepo) GetValue(context.Context, string) (string, error) {
	return "", ErrSettingNotFound
}

func (signatureCompatSettingRepo) Set(context.Context, string, string) error {
	return nil
}

func (signatureCompatSettingRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{}, nil
}

func (signatureCompatSettingRepo) SetMultiple(context.Context, map[string]string) error {
	return nil
}

func (signatureCompatSettingRepo) GetAll(context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}

func (signatureCompatSettingRepo) Delete(context.Context, string) error {
	return nil
}

type signatureCompatUpstream struct {
	requestBodies [][]byte
}

func (u *signatureCompatUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	_ = req.Body.Close()
	u.requestBodies = append(u.requestBodies, append([]byte(nil), body...))
	req.Body = io.NopCloser(bytes.NewReader(body))

	if bytes.Contains(body, []byte(`"thinking":`)) {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"error":{"message":"参数值无效"},"type":"error"}`))),
		}, nil
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(bytes.NewReader([]byte(`{
			"id":"msg_test",
			"type":"message",
			"role":"assistant",
			"model":"claude-sonnet-4-6",
			"content":[{"type":"text","text":"ok"}],
			"stop_reason":"end_turn",
			"usage":{"input_tokens":1,"output_tokens":1}
		}`))),
	}, nil
}

func (u *signatureCompatUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func TestGatewayService_Forward_APIKeyGenericInvalidParamRetriesWithThinkingFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Anthropic-Version", "2023-06-01")

	body := []byte(`{
		"model":"claude-sonnet-4-6",
		"max_tokens":200,
		"stream":false,
		"thinking":{"type":"enabled","budget_tokens":1024},
		"messages":[
			{"role":"user","content":"List files in the current directory"},
			{"role":"assistant","content":[
				{"type":"text","text":"I will list files."},
				{"type":"tool_use","id":"toolu_01XGmNv","name":"Bash","input":{"command":"ls -la"}}
			]},
			{"role":"user","content":[
				{"type":"tool_result","tool_use_id":"toolu_01XGmNv","content":"file1\nfile2"}
			]}
		],
		"tools":[
			{"name":"Bash","description":"Execute bash commands","input_schema":{"type":"object","properties":{"command":{"type":"string"}},"required":["command"]}}
		]
	}`)

	upstream := &signatureCompatUpstream{}
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			MaxLineSize: defaultMaxLineSize,
		},
	}

	svc := &GatewayService{
		cfg:                  cfg,
		responseHeaderFilter: compileResponseHeaderFilter(cfg),
		httpUpstream:         upstream,
		rateLimitService:     &RateLimitService{},
		settingService:       NewSettingService(signatureCompatSettingRepo{}, &config.Config{}),
	}

	account := &Account{
		ID:          16,
		Name:        "lx-1.55-vertex",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "upstream-anthropic-key",
			"base_url": "https://api.anthropic.com",
		},
		Extra: map[string]any{
			"anthropic_invalid_param_rectifier": true,
		},
		Status:      StatusActive,
		Schedulable: true,
	}

	result, err := svc.Forward(context.Background(), c, account, &ParsedRequest{
		Body:   body,
		Model:  "claude-sonnet-4-6",
		Stream: false,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requestBodies, 2, "generic invalid-parameter response should trigger one filtered retry")
	require.Contains(t, string(upstream.requestBodies[0]), `"thinking"`)
	require.NotContains(t, string(upstream.requestBodies[1]), `"thinking"`)
	require.Contains(t, rec.Body.String(), `"text":"ok"`)
}

func TestGatewayService_Forward_APIKeyGenericInvalidParamRetryRequiresAccountFlag(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Anthropic-Version", "2023-06-01")

	body := []byte(`{
		"model":"claude-sonnet-4-6",
		"max_tokens":200,
		"stream":false,
		"thinking":{"type":"enabled","budget_tokens":1024},
		"messages":[
			{"role":"user","content":"List files in the current directory"},
			{"role":"assistant","content":[
				{"type":"text","text":"I will list files."},
				{"type":"tool_use","id":"toolu_01XGmNv","name":"Bash","input":{"command":"ls -la"}}
			]},
			{"role":"user","content":[
				{"type":"tool_result","tool_use_id":"toolu_01XGmNv","content":"file1\nfile2"}
			]}
		],
		"tools":[
			{"name":"Bash","description":"Execute bash commands","input_schema":{"type":"object","properties":{"command":{"type":"string"}},"required":["command"]}}
		]
	}`)

	upstream := &signatureCompatUpstream{}
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			MaxLineSize: defaultMaxLineSize,
		},
	}
	svc := &GatewayService{
		cfg:                  cfg,
		responseHeaderFilter: compileResponseHeaderFilter(cfg),
		httpUpstream:         upstream,
		rateLimitService:     &RateLimitService{},
		settingService:       NewSettingService(signatureCompatSettingRepo{}, &config.Config{}),
	}

	account := &Account{
		ID:          16,
		Name:        "lx-1.55-vertex",
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

	result, err := svc.Forward(context.Background(), c, account, &ParsedRequest{
		Body:   body,
		Model:  "claude-sonnet-4-6",
		Stream: false,
	})
	require.Error(t, err)
	require.Nil(t, result)
	require.Len(t, upstream.requestBodies, 1, "without account flag the compatibility retry must stay disabled")
}

func TestShouldRetryGenericAPIKeySignatureCompat_RequiresThinkingHistory(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-4-6",
		"messages":[
			{"role":"user","content":"List files in the current directory"},
			{"role":"assistant","content":[
				{"type":"text","text":"I will list files."},
				{"type":"tool_use","id":"toolu_01XGmNv","name":"Bash","input":{"command":"ls -la"}}
			]},
			{"role":"user","content":[
				{"type":"tool_result","tool_use_id":"toolu_01XGmNv","content":"file1\nfile2"}
			]}
		]
	}`)
	respBody := []byte(`{"error":{"message":"参数值无效"},"type":"error"}`)

	require.False(t, shouldRetryGenericAPIKeySignatureCompat(body, respBody))
	require.True(t, shouldRetryGenericAPIKeySignatureCompat(
		append([]byte(`{"thinking":{"type":"enabled","budget_tokens":1024},`), body[1:]...),
		respBody,
	))
}

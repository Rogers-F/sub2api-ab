package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
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
	errorBody     []byte
}

func (u *signatureCompatUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	_ = req.Body.Close()
	u.requestBodies = append(u.requestBodies, append([]byte(nil), body...))
	req.Body = io.NopCloser(bytes.NewReader(body))

	if bytes.Contains(body, []byte(`"thinking":`)) || bytes.Contains(body, []byte(`"type":"thinking"`)) {
		errorBody := u.errorBody
		if len(errorBody) == 0 {
			errorBody = []byte(`{"error":{"message":"参数值无效"},"type":"error"}`)
		}
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader(errorBody)),
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

func TestGatewayService_Forward_APIKeyExplicitThinkingSignatureRetriesWithoutToolTextDowngrade(t *testing.T) {
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
				{"type":"thinking","thinking":"secret internal plan","signature":"bad_sig"},
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

	upstream := &signatureCompatUpstream{
		errorBody: []byte(`{"error":{"message":"messages.1.content.0: Invalid ` + "`signature`" + ` in ` + "`thinking`" + ` block","type":"invalid_request_error"}}`),
	}
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
		Name:        "lx-1.55-max",
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
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requestBodies, 2, "explicit thinking signature error should trigger one safe retry")
	retryBody := string(upstream.requestBodies[1])
	require.NotContains(t, retryBody, `"thinking"`)
	require.NotContains(t, retryBody, "secret internal plan")
	require.Contains(t, retryBody, `"type":"tool_use"`)
	require.Contains(t, retryBody, `"type":"tool_result"`)
	require.NotContains(t, retryBody, "(tool_use)")
	require.Contains(t, rec.Body.String(), `"text":"ok"`)
}

func TestGatewayService_Forward_GroupDisablesExplicitThinkingSignatureRetry(t *testing.T) {
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
			{"role":"user","content":"List files"},
			{"role":"assistant","content":[
				{"type":"thinking","thinking":"secret internal plan","signature":"bad_sig"},
				{"type":"tool_use","id":"toolu_01XGmNv","name":"Bash","input":{"command":"ls -la"}}
			]}
		],
		"tools":[
			{"name":"Bash","description":"Execute bash commands","input_schema":{"type":"object","properties":{"command":{"type":"string"}},"required":["command"]}}
		]
	}`)

	upstream := &signatureCompatUpstream{
		errorBody: []byte(`{"error":{"message":"messages.1.content.0: Invalid ` + "`signature`" + ` in ` + "`thinking`" + ` block","type":"invalid_request_error"}}`),
	}
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
		Name:        "lx-1.55-max",
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

	disabled := false
	group := &Group{
		ID:                     7,
		Platform:               PlatformAnthropic,
		Status:                 StatusActive,
		Hydrated:               true,
		SignatureCompatEnabled: &disabled,
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
	require.Len(t, upstream.requestBodies, 1, "group-level disabled switch must block signature compatibility retry")
}

func TestGatewayService_Forward_GroupEnablesGenericInvalidParamSignatureRetry(t *testing.T) {
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

	enabled := true
	group := &Group{
		ID:                     7,
		Platform:               PlatformAnthropic,
		Status:                 StatusActive,
		Hydrated:               true,
		SignatureCompatEnabled: &enabled,
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
	require.Len(t, upstream.requestBodies, 2, "group-level enabled switch should enable generic signature retry without account fallback flag")
	require.Contains(t, rec.Body.String(), `"text":"ok"`)
}

func TestShouldRectifySignatureError_APIKeyExplicitThinkingSignatureUsesDefaultRectifier(t *testing.T) {
	svc := &GatewayService{
		settingService: NewSettingService(signatureCompatSettingRepo{}, &config.Config{}),
	}
	account := &Account{
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
	}
	respBody := []byte(`{"error":{"message":"messages.1.content.0: Invalid ` + "`signature`" + ` in ` + "`thinking`" + ` block"}}`)

	require.True(t, svc.shouldRectifySignatureError(context.Background(), account, []byte(`{"messages":[]}`), respBody))
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

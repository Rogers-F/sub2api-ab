//go:build unit

package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

// --- shared test helpers ---

type queuedHTTPUpstream struct {
	responses []*http.Response
	requests  []*http.Request
	tlsFlags  []bool
}

func (u *queuedHTTPUpstream) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return nil, fmt.Errorf("unexpected Do call")
}

func (u *queuedHTTPUpstream) DoWithTLS(req *http.Request, _ string, _ int64, _ int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	u.requests = append(u.requests, req)
	u.tlsFlags = append(u.tlsFlags, profile != nil)
	if len(u.responses) == 0 {
		return nil, fmt.Errorf("no mocked response")
	}
	resp := u.responses[0]
	u.responses = u.responses[1:]
	return resp, nil
}

func newJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// --- test functions ---

func newTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/1/test", nil)
	return c, rec
}

type openAIAccountTestRepo struct {
	mockAccountRepoForGemini
	updatedExtra  map[string]any
	rateLimitedID int64
	rateLimitedAt *time.Time
}

func (r *openAIAccountTestRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.updatedExtra = updates
	return nil
}

func (r *openAIAccountTestRepo) SetRateLimited(_ context.Context, id int64, resetAt time.Time) error {
	r.rateLimitedID = id
	r.rateLimitedAt = &resetAt
	return nil
}

func TestAccountTestService_OpenAISuccessPersistsSnapshotFromHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, recorder := newTestContext()

	resp := newJSONResponse(http.StatusOK, "")
	resp.Body = io.NopCloser(strings.NewReader(`data: {"type":"response.completed"}

`))
	resp.Header.Set("x-codex-primary-used-percent", "88")
	resp.Header.Set("x-codex-primary-reset-after-seconds", "604800")
	resp.Header.Set("x-codex-primary-window-minutes", "10080")
	resp.Header.Set("x-codex-secondary-used-percent", "42")
	resp.Header.Set("x-codex-secondary-reset-after-seconds", "18000")
	resp.Header.Set("x-codex-secondary-window-minutes", "300")

	repo := &openAIAccountTestRepo{}
	upstream := &queuedHTTPUpstream{responses: []*http.Response{resp}}
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream}
	account := &Account{
		ID:          89,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-token"},
	}

	err := svc.testOpenAIAccountConnection(ctx, account, "gpt-5.4", "")
	require.NoError(t, err)
	require.NotEmpty(t, repo.updatedExtra)
	require.Equal(t, 42.0, repo.updatedExtra["codex_5h_used_percent"])
	require.Equal(t, 88.0, repo.updatedExtra["codex_7d_used_percent"])
	require.Contains(t, recorder.Body.String(), "test_complete")
}

func TestAccountTestService_OpenAI429PersistsSnapshotWithoutRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := newTestContext()

	resp := newJSONResponse(http.StatusTooManyRequests, `{"error":{"type":"usage_limit_reached","message":"limit reached"}}`)
	resp.Header.Set("x-codex-primary-used-percent", "100")
	resp.Header.Set("x-codex-primary-reset-after-seconds", "604800")
	resp.Header.Set("x-codex-primary-window-minutes", "10080")
	resp.Header.Set("x-codex-secondary-used-percent", "100")
	resp.Header.Set("x-codex-secondary-reset-after-seconds", "18000")
	resp.Header.Set("x-codex-secondary-window-minutes", "300")

	repo := &openAIAccountTestRepo{}
	upstream := &queuedHTTPUpstream{responses: []*http.Response{resp}}
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream}
	account := &Account{
		ID:          88,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-token"},
	}

	err := svc.testOpenAIAccountConnection(ctx, account, "gpt-5.4", "")
	require.Error(t, err)
	require.NotEmpty(t, repo.updatedExtra)
	require.Equal(t, 100.0, repo.updatedExtra["codex_5h_used_percent"])
	require.Zero(t, repo.rateLimitedID)
	require.Nil(t, repo.rateLimitedAt)
	require.Nil(t, account.RateLimitResetAt)
}

func TestAccountTestService_OpenAIImageModelUsesImagesAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, recorder := newTestContext()

	resp := newJSONResponse(http.StatusOK, `{"data":[{"b64_json":"ZmFrZQ==","revised_prompt":"orange cat astronaut"}]}`)

	upstream := &queuedHTTPUpstream{responses: []*http.Response{resp}}
	svc := &AccountTestService{
		httpUpstream: upstream,
		cfg: &config.Config{
			Security: config.SecurityConfig{
				URLAllowlist: config.URLAllowlistConfig{},
			},
		},
	}
	account := &Account{
		ID:          90,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://api.openai.com"},
	}

	err := svc.testOpenAIAccountConnection(ctx, account, "gpt-image-2", "draw a tiny orange cat astronaut")
	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "https://api.openai.com/v1/images/generations", upstream.requests[0].URL.String())
	body, err := io.ReadAll(upstream.requests[0].Body)
	require.NoError(t, err)
	require.Contains(t, string(body), `"prompt":"draw a tiny orange cat astronaut"`)
	require.Contains(t, recorder.Body.String(), `"type":"image"`)
	require.Contains(t, recorder.Body.String(), `"type":"test_complete"`)
}

func TestAccountTestService_OpenAIImageModelOAuthUsesChatGPTBackend(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, recorder := newTestContext()

	origNewClient := newOpenAIBackendAPIClientForAccountTest
	origBootstrap := bootstrapOpenAIBackendAPIForAccountTest
	origFetchReqs := fetchOpenAIChatRequirementsForAccountTest
	origInitConversation := initializeOpenAIImageConversationForAccountTest
	origPrepareConversation := prepareOpenAIImageConversationForAccountTest
	origPostConversation := postOpenAIImageConversationForAccountTest
	origReadStream := readOpenAIImageConversationStreamForAccountTest
	origFetchDownloadURL := fetchOpenAIImageDownloadURLForAccountTest
	origDownloadBytes := downloadOpenAIImageBytesForAccountTest
	t.Cleanup(func() {
		newOpenAIBackendAPIClientForAccountTest = origNewClient
		bootstrapOpenAIBackendAPIForAccountTest = origBootstrap
		fetchOpenAIChatRequirementsForAccountTest = origFetchReqs
		initializeOpenAIImageConversationForAccountTest = origInitConversation
		prepareOpenAIImageConversationForAccountTest = origPrepareConversation
		postOpenAIImageConversationForAccountTest = origPostConversation
		readOpenAIImageConversationStreamForAccountTest = origReadStream
		fetchOpenAIImageDownloadURLForAccountTest = origFetchDownloadURL
		downloadOpenAIImageBytesForAccountTest = origDownloadBytes
	})

	newOpenAIBackendAPIClientForAccountTest = func(string) (*req.Client, error) {
		return req.C(), nil
	}
	bootstrapOpenAIBackendAPIForAccountTest = func(context.Context, *req.Client, http.Header) error {
		return nil
	}
	fetchOpenAIChatRequirementsForAccountTest = func(context.Context, *req.Client, http.Header) (*openAIChatRequirements, error) {
		return &openAIChatRequirements{Token: "chat-token"}, nil
	}
	initializeOpenAIImageConversationForAccountTest = func(context.Context, *req.Client, http.Header) error {
		return nil
	}
	prepareOpenAIImageConversationForAccountTest = func(context.Context, *req.Client, http.Header, string, string, string, string) (string, error) {
		return "conduit-token", nil
	}
	postOpenAIImageConversationForAccountTest = func(context.Context, *req.Client, http.Header, any) (*req.Response, error) {
		return &req.Response{
			Response: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("")),
			},
		}, nil
	}
	readOpenAIImageConversationStreamForAccountTest = func(*req.Response, time.Time) (string, []openAIImagePointerInfo, OpenAIUsage, *int, error) {
		return "conv-1", []openAIImagePointerInfo{{
			Pointer: "file-service://image-1",
			Prompt:  "orange cat astronaut",
		}}, OpenAIUsage{}, nil, nil
	}
	fetchOpenAIImageDownloadURLForAccountTest = func(context.Context, *req.Client, http.Header, string, string) (string, error) {
		return "https://files.example/image.png", nil
	}
	downloadOpenAIImageBytesForAccountTest = func(context.Context, *req.Client, http.Header, string) ([]byte, error) {
		return []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, nil
	}

	svc := &AccountTestService{}
	account := &Account{
		ID:          91,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-token"},
	}

	err := svc.testOpenAIAccountConnection(ctx, account, "gpt-image-2", "")
	require.NoError(t, err)
	require.Contains(t, recorder.Body.String(), `"type":"image"`)
	require.Contains(t, recorder.Body.String(), `"mime_type":"image/png"`)
	require.Contains(t, recorder.Body.String(), `"type":"test_complete"`)
}

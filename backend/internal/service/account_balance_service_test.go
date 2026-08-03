package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type accountBalanceUpstreamStub struct {
	mu        sync.Mutex
	responses map[string]struct {
		status int
		body   string
	}
	requests []*http.Request
	proxies  []string
}

func (s *accountBalanceUpstreamStub) Do(
	req *http.Request,
	proxyURL string,
	_ int64,
	_ int,
) (*http.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, req)
	s.proxies = append(s.proxies, proxyURL)
	stub, ok := s.responses[req.URL.Path]
	if !ok {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"error":"not found"}`)),
		}, nil
	}
	return &http.Response{
		StatusCode: stub.status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(stub.body)),
	}, nil
}

func (s *accountBalanceUpstreamStub) DoWithTLS(
	req *http.Request,
	proxyURL string,
	accountID int64,
	accountConcurrency int,
	_ *tlsfingerprint.Profile,
) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

func newAccountBalanceTestService(upstream HTTPUpstream) *AccountTestService {
	return &AccountTestService{
		httpUpstream: upstream,
		cfg: &config.Config{Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: false},
		}},
	}
}

func TestBuildAccountBalanceURLs(t *testing.T) {
	t.Parallel()

	sub2apiURL, err := buildAccountBalanceURL("https://relay.example.com/prefix/v1", AccountBalanceProviderSub2API)
	require.NoError(t, err)
	require.Equal(t, "https://relay.example.com/prefix/v1/usage", sub2apiURL)

	newAPIURL, err := buildAccountBalanceURL("https://relay.example.com/prefix/v1", AccountBalanceProviderNewAPI)
	require.NoError(t, err)
	require.Equal(t, "https://relay.example.com/prefix/api/usage/token", newAPIURL)

	statusURL, err := buildNewAPIStatusURL("https://relay.example.com/prefix/v1")
	require.NoError(t, err)
	require.Equal(t, "https://relay.example.com/prefix/api/status", statusURL)
}

func TestAccountBalanceFetchSub2API(t *testing.T) {
	t.Parallel()

	upstream := &accountBalanceUpstreamStub{responses: map[string]struct {
		status int
		body   string
	}{
		"/v1/usage": {status: http.StatusOK, body: `{"remaining":12.3456,"unit":"USD"}`},
	}}
	service := newAccountBalanceTestService(upstream)
	account := &Account{
		ID:          11,
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Concurrency: 3,
		Credentials: map[string]any{
			"base_url":           "https://relay.example.com/v1",
			"api_key":            "sk-test",
			"balance_query_type": AccountBalanceProviderSub2API,
		},
		Proxy: &Proxy{Protocol: "http", Host: "127.0.0.1", Port: 8080},
	}

	balance, unit, unlimited, err := service.fetchAccountBalance(context.Background(), account, AccountBalanceProviderSub2API)
	require.NoError(t, err)
	require.NotNil(t, balance)
	require.InDelta(t, 12.3456, *balance, 0.000001)
	require.Equal(t, "USD", unit)
	require.False(t, unlimited)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "Bearer sk-test", upstream.requests[0].Header.Get("Authorization"))
	require.Equal(t, "http://127.0.0.1:8080", upstream.proxies[0])
}

func TestAccountBalanceFetchNewAPIUsesReportedQuotaPerUnit(t *testing.T) {
	t.Parallel()

	upstream := &accountBalanceUpstreamStub{responses: map[string]struct {
		status int
		body   string
	}{
		"/api/usage/token": {
			status: http.StatusOK,
			body:   `{"code":true,"data":{"total_available":750000,"unlimited_quota":false}}`,
		},
		"/api/status": {
			status: http.StatusOK,
			body:   `{"success":true,"data":{"quota_per_unit":250000}}`,
		},
	}}
	service := newAccountBalanceTestService(upstream)
	account := &Account{
		ID:          12,
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Concurrency: 2,
		Credentials: map[string]any{
			"base_url":           "https://new-api.example.com/v1",
			"api_key":            "sk-newapi",
			"balance_query_type": AccountBalanceProviderNewAPI,
		},
	}

	balance, unit, unlimited, err := service.fetchAccountBalance(context.Background(), account, AccountBalanceProviderNewAPI)
	require.NoError(t, err)
	require.NotNil(t, balance)
	require.InDelta(t, 3, *balance, 0.000001)
	require.Equal(t, "USD", unit)
	require.False(t, unlimited)
	require.Len(t, upstream.requests, 2)
	require.Equal(t, "/api/usage/token", upstream.requests[0].URL.Path)
	require.Equal(t, "/api/status", upstream.requests[1].URL.Path)
}

func TestParseSub2APIBalanceSupportsNestedQuota(t *testing.T) {
	t.Parallel()

	balance, unit, unlimited, err := parseSub2APIBalance([]byte(`{"quota":{"remaining":"8.50","unit":"USD"}}`))
	require.NoError(t, err)
	require.NotNil(t, balance)
	require.InDelta(t, 8.5, *balance, 0.000001)
	require.Equal(t, "USD", unit)
	require.False(t, unlimited)
}

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

const (
	AccountBalanceProviderSub2API = "sub2api"
	AccountBalanceProviderNewAPI  = "newapi"

	accountBalanceRequestTimeout = 12 * time.Second
	accountBalanceMaxBodyBytes   = int64(64 * 1024)
	accountBalanceBatchLimit     = 8
	newAPIDefaultQuotaPerUnit    = 500_000.0
	newAPIQuotaMetadataTTL       = 5 * time.Minute
)

// AccountBalanceInfo is returned per account so one failing upstream does not
// prevent balances from other accounts from being displayed.
type AccountBalanceInfo struct {
	AccountID  int64    `json:"account_id"`
	Configured bool     `json:"configured"`
	Provider   string   `json:"provider,omitempty"`
	Balance    *float64 `json:"balance,omitempty"`
	Unit       string   `json:"unit,omitempty"`
	Unlimited  bool     `json:"unlimited,omitempty"`
	QueriedAt  string   `json:"queried_at,omitempty"`
	Error      string   `json:"error,omitempty"`
}

type newAPIQuotaMetadataCacheEntry struct {
	quotaPerUnit float64
	expiresAt    time.Time
}

// GetAccountBalances queries configured Claude Console API Key accounts in
// parallel. Accounts without balance_query_type are returned as unconfigured.
func (s *AccountTestService) GetAccountBalances(ctx context.Context, accountIDs []int64) (map[int64]AccountBalanceInfo, error) {
	if s == nil || s.accountRepo == nil {
		return nil, errors.New("account balance service is not configured")
	}

	accounts, err := s.accountRepo.GetByIDs(ctx, accountIDs)
	if err != nil {
		return nil, fmt.Errorf("get accounts for balance query: %w", err)
	}

	results := make(map[int64]AccountBalanceInfo, len(accountIDs))
	for _, id := range accountIDs {
		results[id] = AccountBalanceInfo{AccountID: id}
	}

	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(accountBalanceBatchLimit)
	for _, account := range accounts {
		account := account
		if account == nil {
			continue
		}

		provider := accountBalanceProvider(account)
		if provider == "" {
			continue
		}

		g.Go(func() error {
			info := AccountBalanceInfo{
				AccountID:  account.ID,
				Configured: true,
				Provider:   provider,
				QueriedAt:  time.Now().UTC().Format(time.RFC3339),
			}
			balance, unit, unlimited, fetchErr := s.fetchAccountBalance(gctx, account, provider)
			if fetchErr != nil {
				info.Error = truncateAccountBalanceError(fetchErr.Error())
			} else {
				info.Balance = balance
				info.Unit = unit
				info.Unlimited = unlimited
			}

			mu.Lock()
			results[account.ID] = info
			mu.Unlock()
			return nil
		})
	}
	_ = g.Wait()

	return results, nil
}

func accountBalanceProvider(account *Account) string {
	if account == nil || account.Platform != PlatformAnthropic || account.Type != AccountTypeAPIKey {
		return ""
	}
	provider := strings.ToLower(strings.TrimSpace(account.GetCredential("balance_query_type")))
	switch provider {
	case AccountBalanceProviderSub2API, AccountBalanceProviderNewAPI:
		return provider
	default:
		return ""
	}
}

func (s *AccountTestService) fetchAccountBalance(
	ctx context.Context,
	account *Account,
	provider string,
) (*float64, string, bool, error) {
	if s == nil || s.httpUpstream == nil {
		return nil, "", false, errors.New("HTTP upstream is not configured")
	}
	if account == nil {
		return nil, "", false, errors.New("account is required")
	}

	apiKey := strings.TrimSpace(account.GetCredential("api_key"))
	if apiKey == "" {
		return nil, "", false, errors.New("API key is missing")
	}
	baseURL := strings.TrimSpace(account.GetCredential("base_url"))
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, "", false, fmt.Errorf("invalid base URL: %w", err)
	}

	queryURL, err := buildAccountBalanceURL(normalizedBaseURL, provider)
	if err != nil {
		return nil, "", false, err
	}

	queryCtx, cancel := context.WithTimeout(ctx, accountBalanceRequestTimeout)
	defer cancel()
	body, err := s.doAccountBalanceRequest(queryCtx, account, queryURL, apiKey)
	if err != nil {
		return nil, "", false, err
	}

	switch provider {
	case AccountBalanceProviderSub2API:
		return parseSub2APIBalance(body)
	case AccountBalanceProviderNewAPI:
		quotaPerUnit := s.getNewAPIQuotaPerUnit(queryCtx, account, normalizedBaseURL)
		return parseNewAPIBalance(body, quotaPerUnit)
	default:
		return nil, "", false, fmt.Errorf("unsupported balance query provider: %s", provider)
	}
}

func (s *AccountTestService) doAccountBalanceRequest(
	ctx context.Context,
	account *Account,
	requestURL string,
	apiKey string,
) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create balance request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("User-Agent", "Sub2API-BalanceQuery/1.0")

	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil, fmt.Errorf("balance request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, accountBalanceMaxBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read balance response: %w", err)
	}
	if int64(len(body)) > accountBalanceMaxBodyBytes {
		return nil, errors.New("balance response is too large")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("upstream returned HTTP %d", resp.StatusCode)
	}
	return body, nil
}

func (s *AccountTestService) getNewAPIQuotaPerUnit(ctx context.Context, account *Account, normalizedBaseURL string) float64 {
	now := time.Now()
	if cached, ok := s.newAPIQuotaMetadataCache.Load(normalizedBaseURL); ok {
		if entry, valid := cached.(newAPIQuotaMetadataCacheEntry); valid && now.Before(entry.expiresAt) && entry.quotaPerUnit > 0 {
			return entry.quotaPerUnit
		}
	}

	statusURL, err := buildNewAPIStatusURL(normalizedBaseURL)
	if err != nil {
		return newAPIDefaultQuotaPerUnit
	}
	body, err := s.doAccountBalanceRequest(ctx, account, statusURL, account.GetCredential("api_key"))
	if err != nil {
		return newAPIDefaultQuotaPerUnit
	}
	root, err := decodeBalanceJSON(body)
	if err != nil {
		return newAPIDefaultQuotaPerUnit
	}
	data := nestedBalanceMap(root, "data")
	quotaPerUnit, ok := balanceNumber(data["quota_per_unit"])
	if !ok || quotaPerUnit <= 0 {
		quotaPerUnit, ok = balanceNumber(root["quota_per_unit"])
	}
	if !ok || quotaPerUnit <= 0 {
		return newAPIDefaultQuotaPerUnit
	}

	s.newAPIQuotaMetadataCache.Store(normalizedBaseURL, newAPIQuotaMetadataCacheEntry{
		quotaPerUnit: quotaPerUnit,
		expiresAt:    now.Add(newAPIQuotaMetadataTTL),
	})
	return quotaPerUnit
}

func buildAccountBalanceURL(baseURL, provider string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", errors.New("invalid balance query base URL")
	}
	path := strings.TrimRight(u.Path, "/")
	switch provider {
	case AccountBalanceProviderSub2API:
		switch {
		case strings.HasSuffix(path, "/v1/usage"):
		case strings.HasSuffix(path, "/v1"):
			path += "/usage"
		default:
			path += "/v1/usage"
		}
	case AccountBalanceProviderNewAPI:
		path = strings.TrimSuffix(path, "/v1")
		switch {
		case strings.HasSuffix(path, "/api/usage/token"):
		case strings.HasSuffix(path, "/api"):
			path += "/usage/token"
		default:
			path += "/api/usage/token"
		}
	default:
		return "", fmt.Errorf("unsupported balance query provider: %s", provider)
	}
	u.Path = ensureLeadingSlash(path)
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/"), nil
}

func buildNewAPIStatusURL(baseURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", errors.New("invalid New API base URL")
	}
	path := strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/v1")
	if strings.HasSuffix(path, "/api") {
		path += "/status"
	} else {
		path += "/api/status"
	}
	u.Path = ensureLeadingSlash(path)
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/"), nil
}

func ensureLeadingSlash(path string) string {
	if path == "" {
		return "/"
	}
	if strings.HasPrefix(path, "/") {
		return path
	}
	return "/" + path
}

func parseSub2APIBalance(body []byte) (*float64, string, bool, error) {
	root, err := decodeBalanceJSON(body)
	if err != nil {
		return nil, "", false, err
	}
	data := nestedBalanceMap(root, "data")
	quota := nestedBalanceMap(root, "quota")
	dataQuota := nestedBalanceMap(data, "quota")

	var balance float64
	found := false
	for _, candidate := range []any{
		root["remaining"], root["balance"], quota["remaining"],
		data["remaining"], data["balance"], dataQuota["remaining"],
	} {
		if value, ok := balanceNumber(candidate); ok {
			balance = value
			found = true
			break
		}
	}
	if !found {
		return nil, "", false, errors.New("Sub2API response does not contain a balance")
	}
	if balance < 0 {
		return nil, balanceUnit(root, data, quota, dataQuota), true, nil
	}
	return &balance, balanceUnit(root, data, quota, dataQuota), false, nil
}

func parseNewAPIBalance(body []byte, quotaPerUnit float64) (*float64, string, bool, error) {
	root, err := decodeBalanceJSON(body)
	if err != nil {
		return nil, "", false, err
	}
	data := nestedBalanceMap(root, "data")
	if balanceBool(data["unlimited_quota"]) || balanceBool(root["unlimited_quota"]) {
		return nil, "USD", true, nil
	}

	for _, candidate := range []any{data["total_available_usd"], root["total_available_usd"]} {
		if value, ok := balanceNumber(candidate); ok {
			if value < 0 {
				return nil, "", false, errors.New("New API returned a negative balance")
			}
			return &value, "USD", false, nil
		}
	}

	raw, ok := balanceNumber(data["total_available"])
	if !ok {
		raw, ok = balanceNumber(root["total_available"])
	}
	if !ok {
		return nil, "", false, errors.New("New API response does not contain total_available")
	}
	if raw < 0 {
		return nil, "", false, errors.New("New API returned a negative balance")
	}
	if quotaPerUnit <= 0 || math.IsNaN(quotaPerUnit) || math.IsInf(quotaPerUnit, 0) {
		quotaPerUnit = newAPIDefaultQuotaPerUnit
	}
	balance := raw / quotaPerUnit
	return &balance, "USD", false, nil
}

func decodeBalanceJSON(body []byte) (map[string]any, error) {
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	var root map[string]any
	if err := decoder.Decode(&root); err != nil {
		return nil, fmt.Errorf("invalid balance response: %w", err)
	}
	if root == nil {
		return nil, errors.New("empty balance response")
	}
	return root, nil
}

func nestedBalanceMap(root map[string]any, key string) map[string]any {
	if root == nil {
		return map[string]any{}
	}
	value, _ := root[key].(map[string]any)
	if value == nil {
		return map[string]any{}
	}
	return value
}

func balanceNumber(value any) (float64, bool) {
	var number float64
	var err error
	switch v := value.(type) {
	case json.Number:
		number, err = v.Float64()
	case float64:
		number = v
	case float32:
		number = float64(v)
	case int:
		number = float64(v)
	case int64:
		number = float64(v)
	case string:
		number, err = strconv.ParseFloat(strings.TrimSpace(v), 64)
	default:
		return 0, false
	}
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, false
	}
	return number, true
}

func balanceBool(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		parsed, _ := strconv.ParseBool(strings.TrimSpace(v))
		return parsed
	default:
		return false
	}
}

func balanceUnit(maps ...map[string]any) string {
	for _, item := range maps {
		if unit, ok := item["unit"].(string); ok && strings.TrimSpace(unit) != "" {
			return strings.ToUpper(strings.TrimSpace(unit))
		}
	}
	return "USD"
}

func truncateAccountBalanceError(message string) string {
	message = strings.TrimSpace(message)
	const maxLength = 180
	if len(message) <= maxLength {
		return message
	}
	return message[:maxLength] + "..."
}

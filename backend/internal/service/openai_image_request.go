package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"

	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
)

type OpenAIImageRequestMeta struct {
	Model      string
	ImageCount int
	ImageSize  string
	Multipart  bool
}

type OpenAIImageForwardInput struct {
	Endpoint      string
	Body          []byte
	ContentType   string
	Meta          OpenAIImageRequestMeta
	ModelOverride string
}

func ParseOpenAIImageRequestMeta(body []byte, contentType string) (OpenAIImageRequestMeta, error) {
	meta := OpenAIImageRequestMeta{
		ImageCount: 1,
		ImageSize:  "1K",
	}

	mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err != nil {
		trimmed := strings.ToLower(strings.TrimSpace(contentType))
		switch {
		case strings.HasPrefix(trimmed, "multipart/form-data"):
			mediaType = "multipart/form-data"
		case trimmed == "" || strings.HasPrefix(trimmed, "application/json"):
			mediaType = "application/json"
		default:
			return meta, fmt.Errorf("unsupported content type: %s", contentType)
		}
	}

	switch mediaType {
	case "", "application/json":
		meta.Model = strings.TrimSpace(gjson.GetBytes(body, "model").String())
		meta.ImageCount = normalizeOpenAIImageCount(int(gjson.GetBytes(body, "n").Int()))
		meta.ImageSize = normalizeOpenAIImageSize(gjson.GetBytes(body, "size").String())
	case "multipart/form-data":
		boundary := strings.TrimSpace(params["boundary"])
		if boundary == "" {
			return meta, fmt.Errorf("multipart boundary is required")
		}
		meta.Multipart = true

		reader := multipart.NewReader(bytes.NewReader(body), boundary)
		for {
			part, partErr := reader.NextPart()
			if partErr == io.EOF {
				break
			}
			if partErr != nil {
				return meta, fmt.Errorf("read multipart body: %w", partErr)
			}

			formName := strings.TrimSpace(part.FormName())
			switch formName {
			case "model":
				value, readErr := readOpenAIFormField(part)
				if readErr != nil {
					_ = part.Close()
					return meta, readErr
				}
				meta.Model = strings.TrimSpace(value)
			case "n":
				value, readErr := readOpenAIFormField(part)
				if readErr != nil {
					_ = part.Close()
					return meta, readErr
				}
				if parsed, parseErr := strconv.Atoi(strings.TrimSpace(value)); parseErr == nil {
					meta.ImageCount = normalizeOpenAIImageCount(parsed)
				}
			case "size":
				value, readErr := readOpenAIFormField(part)
				if readErr != nil {
					_ = part.Close()
					return meta, readErr
				}
				meta.ImageSize = normalizeOpenAIImageSize(value)
			default:
				if _, readErr := io.Copy(io.Discard, part); readErr != nil {
					_ = part.Close()
					return meta, fmt.Errorf("discard multipart part %q: %w", formName, readErr)
				}
			}

			_ = part.Close()
		}
	default:
		return meta, fmt.Errorf("unsupported content type: %s", contentType)
	}

	if meta.Model == "" {
		return meta, fmt.Errorf("model is required")
	}

	return meta, nil
}

func readOpenAIFormField(part *multipart.Part) (string, error) {
	value, err := io.ReadAll(io.LimitReader(part, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read multipart form field: %w", err)
	}
	return string(value), nil
}

func normalizeOpenAIImageCount(count int) int {
	if count <= 0 {
		return 1
	}
	return count
}

func normalizeOpenAIImageSize(size string) string {
	size = strings.TrimSpace(strings.ToUpper(size))
	switch size {
	case "", "AUTO":
		return "1K"
	case "1K", "2K", "4K":
		return size
	}

	parts := strings.Split(size, "X")
	if len(parts) != 2 {
		return "1K"
	}
	width, errW := strconv.Atoi(strings.TrimSpace(parts[0]))
	height, errH := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errW != nil || errH != nil || width <= 0 || height <= 0 {
		return "1K"
	}

	pixels := width * height
	switch {
	case pixels <= 1024*1024:
		return "1K"
	case pixels <= 2048*2048:
		return "2K"
	default:
		return "4K"
	}
}

func extractOpenAIImageCountFromJSONBytes(body []byte, fallback int) int {
	count := len(gjson.GetBytes(body, "data").Array())
	if count > 0 {
		return count
	}
	return normalizeOpenAIImageCount(fallback)
}

func rewriteOpenAIImageRequestModel(body []byte, contentType string, newModel string) ([]byte, string, error) {
	if strings.TrimSpace(newModel) == "" || len(body) == 0 {
		return body, contentType, nil
	}

	mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err != nil {
		trimmed := strings.ToLower(strings.TrimSpace(contentType))
		switch {
		case strings.HasPrefix(trimmed, "multipart/form-data"):
			mediaType = "multipart/form-data"
		default:
			mediaType = "application/json"
		}
	}

	switch mediaType {
	case "", "application/json":
		return ReplaceModelInBody(body, newModel), contentType, nil
	case "multipart/form-data":
		boundary := strings.TrimSpace(params["boundary"])
		if boundary == "" {
			return nil, "", fmt.Errorf("multipart boundary is required")
		}
		return rewriteOpenAIImageMultipartModel(body, boundary, newModel)
	default:
		return nil, "", fmt.Errorf("unsupported content type: %s", contentType)
	}
}

func rewriteOpenAIImageMultipartModel(body []byte, boundary string, newModel string) ([]byte, string, error) {
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	var rewritten bytes.Buffer
	writer := multipart.NewWriter(&rewritten)

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", fmt.Errorf("read multipart body: %w", err)
		}

		header := cloneOpenAIMIMEHeader(part.Header)
		dst, err := writer.CreatePart(header)
		if err != nil {
			_ = part.Close()
			return nil, "", fmt.Errorf("create multipart part: %w", err)
		}

		if strings.TrimSpace(part.FormName()) == "model" && part.FileName() == "" {
			if _, err := io.Copy(io.Discard, part); err != nil {
				_ = part.Close()
				return nil, "", fmt.Errorf("discard multipart model field: %w", err)
			}
			if _, err := io.WriteString(dst, newModel); err != nil {
				_ = part.Close()
				return nil, "", fmt.Errorf("write rewritten multipart model field: %w", err)
			}
		} else {
			if _, err := io.Copy(dst, part); err != nil {
				_ = part.Close()
				return nil, "", fmt.Errorf("copy multipart part: %w", err)
			}
		}

		_ = part.Close()
	}

	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("close multipart writer: %w", err)
	}

	return rewritten.Bytes(), writer.FormDataContentType(), nil
}

func cloneOpenAIMIMEHeader(src textproto.MIMEHeader) textproto.MIMEHeader {
	if len(src) == 0 {
		return textproto.MIMEHeader{}
	}
	dst := make(textproto.MIMEHeader, len(src))
	for key, values := range src {
		copied := make([]string, len(values))
		copy(copied, values)
		dst[key] = copied
	}
	return dst
}

func buildOpenAIEndpointURL(base, endpoint string) string {
	normalizedBase := strings.TrimRight(strings.TrimSpace(base), "/")
	normalizedEndpoint := strings.TrimSpace(endpoint)
	if normalizedEndpoint == "" {
		return buildOpenAIResponsesURL(base)
	}
	if !strings.HasPrefix(normalizedEndpoint, "/") {
		normalizedEndpoint = "/" + normalizedEndpoint
	}
	if strings.HasSuffix(normalizedBase, normalizedEndpoint) {
		return normalizedBase
	}
	if strings.HasSuffix(normalizedBase, "/v1") && strings.HasPrefix(normalizedEndpoint, "/v1/") {
		return normalizedBase + strings.TrimPrefix(normalizedEndpoint, "/v1")
	}
	if strings.HasSuffix(normalizedBase, "/v1") {
		return normalizedBase + normalizedEndpoint
	}
	return normalizedBase + normalizedEndpoint
}

func (s *OpenAIGatewayService) buildUpstreamImageRequest(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	endpoint string,
	body []byte,
	contentType string,
	token string,
) (*http.Request, error) {
	if account == nil || account.Type != AccountTypeAPIKey {
		return nil, fmt.Errorf("openai image endpoints only support api key accounts")
	}

	baseURL := account.GetOpenAIBaseURL()
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	validatedURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, err
	}

	targetURL := buildOpenAIEndpointURL(validatedURL, endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("authorization", "Bearer "+token)
	if ct := strings.TrimSpace(contentType); ct != "" {
		req.Header.Set("content-type", ct)
	}

	if c != nil {
		if accept := strings.TrimSpace(c.GetHeader("Accept")); accept != "" {
			req.Header.Set("accept", accept)
		}
		if acceptLanguage := strings.TrimSpace(c.GetHeader("Accept-Language")); acceptLanguage != "" {
			req.Header.Set("accept-language", acceptLanguage)
		}
	}
	if req.Header.Get("accept") == "" {
		req.Header.Set("accept", "application/json")
	}

	if customUA := account.GetOpenAIUserAgent(); customUA != "" {
		req.Header.Set("user-agent", customUA)
	}
	if s.cfg != nil && s.cfg.Gateway.ForceCodexCLI {
		req.Header.Set("user-agent", codexCLIUserAgent)
	}

	return req, nil
}

func (s *OpenAIGatewayService) ForwardImage(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	input OpenAIImageForwardInput,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	if account == nil {
		return nil, fmt.Errorf("account is required")
	}
	if account.Type != AccountTypeAPIKey {
		if c != nil && c.Writer != nil && !c.Writer.Written() {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": gin.H{
					"type":    "invalid_request_error",
					"message": "OpenAI image endpoints only support API key accounts",
				},
			})
		}
		return nil, fmt.Errorf("openai image endpoints only support api key accounts")
	}

	originalModel := strings.TrimSpace(input.Meta.Model)
	requestModel := originalModel
	requestBody := input.Body
	contentType := input.ContentType

	if overrideModel := strings.TrimSpace(input.ModelOverride); overrideModel != "" && overrideModel != requestModel {
		var rewriteErr error
		requestBody, contentType, rewriteErr = rewriteOpenAIImageRequestModel(requestBody, contentType, overrideModel)
		if rewriteErr != nil {
			return nil, rewriteErr
		}
		requestModel = overrideModel
	}

	upstreamModel := account.GetMappedModel(requestModel)
	if upstreamModel != requestModel {
		var rewriteErr error
		requestBody, contentType, rewriteErr = rewriteOpenAIImageRequestModel(requestBody, contentType, upstreamModel)
		if rewriteErr != nil {
			return nil, rewriteErr
		}
	}

	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}

	setOpsUpstreamRequestBody(c, requestBody)

	upstreamCtx, releaseUpstreamCtx := detachStreamUpstreamContext(ctx, false)
	upstreamReq, err := s.buildUpstreamImageRequest(upstreamCtx, c, account, input.Endpoint, requestBody, contentType, token)
	releaseUpstreamCtx()
	if err != nil {
		return nil, err
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	upstreamStart := time.Now()
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: 0,
			Kind:               "request_error",
			Message:            safeErr,
		})
		if c != nil && c.Writer != nil && !c.Writer.Written() {
			c.JSON(http.StatusBadGateway, gin.H{
				"error": gin.H{
					"type":    "upstream_error",
					"message": "Upstream request failed",
				},
			})
		}
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))

		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
		if s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, upstreamMsg, respBody) {
			s.handleFailoverSideEffects(ctx, resp, account)
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: account.IsPoolMode() && (isPoolModeRetryableStatus(resp.StatusCode) || isOpenAITransientProcessingError(resp.StatusCode, upstreamMsg, respBody)),
			}
		}
		return s.handleErrorResponse(ctx, resp, c, account, requestBody)
	}

	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}

	usage := OpenAIUsage{}
	if parsedUsage, ok := extractOpenAIUsageFromJSONBytes(body); ok {
		usage = parsedUsage
	}
	imageCount := extractOpenAIImageCountFromJSONBytes(body, input.Meta.ImageCount)

	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	responseContentType := resp.Header.Get("Content-Type")
	if responseContentType == "" {
		responseContentType = "application/json"
	}
	c.Data(resp.StatusCode, responseContentType, body)

	return &OpenAIForwardResult{
		RequestID:       resp.Header.Get("x-request-id"),
		Usage:           usage,
		Model:           originalModel,
		UpstreamModel:   upstreamModel,
		Stream:          false,
		ResponseHeaders: resp.Header.Clone(),
		Duration:        time.Since(startTime),
		ImageCount:      imageCount,
		ImageSize:       input.Meta.ImageSize,
	}, nil
}

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGatewayService_HandleNonStreamingResponse_Upstream200ErrorBodyTriggersFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	body := []byte(`{"error":{"message":"upstream http connection faliled: if the destination linkconnection times out, check whether the destination link or network connectivity is normal","type":"voapil error","rid":"2048947557949771776"}}`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
	account := &Account{ID: 7, Name: "custom-relay", Platform: PlatformAnthropic, Type: AccountTypeAPIKey}
	svc := &GatewayService{rateLimitService: &RateLimitService{}}

	usage, err := svc.handleNonStreamingResponse(context.Background(), resp, c, account, "claude-opus-4-6", "claude-opus-4-6")

	require.Nil(t, usage)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusGatewayTimeout, failoverErr.StatusCode)
	require.Equal(t, body, failoverErr.ResponseBody)
	require.False(t, c.Writer.Written(), "service must not write a successful HTTP 200 response")

	statusAny, ok := c.Get(OpsUpstreamStatusCodeKey)
	require.True(t, ok)
	require.Equal(t, http.StatusGatewayTimeout, statusAny)

	msgAny, ok := c.Get(OpsUpstreamErrorMessageKey)
	require.True(t, ok)
	msg, ok := msgAny.(string)
	require.True(t, ok)
	require.Contains(t, msg, "failed")
	require.Contains(t, msg, "link connection")
	require.NotContains(t, msg, "faliled")
	require.NotContains(t, msg, "linkconnection")

	eventsAny, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := eventsAny.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.Len(t, events, 1)
	require.Equal(t, "http_200_error_body", events[0].Kind)
	require.Equal(t, http.StatusGatewayTimeout, events[0].UpstreamStatusCode)
	require.Equal(t, "2048947557949771776", events[0].UpstreamRequestID)
	require.False(t, events[0].Passthrough)
}

func TestGatewayService_AnthropicAPIKeyPassthrough_Upstream200ErrorBodyTriggersFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	body := []byte(`{"error":{"message":"upstream http connection faliled: if the destination linkconnection times out","type":"voapil error","rid":"2048947557949771776"}}`)
	upstream := &anthropicHTTPUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader(body)),
		},
	}
	svc := &GatewayService{
		cfg: &config.Config{
			Security: config.SecurityConfig{
				URLAllowlist: config.URLAllowlistConfig{Enabled: false},
			},
		},
		httpUpstream: upstream,
	}

	result, err := svc.forwardAnthropicAPIKeyPassthrough(context.Background(), c, newAnthropicAPIKeyAccountForTest(), []byte(`{"model":"claude-opus-4-6"}`), "claude-opus-4-6", "claude-opus-4-6", false, time.Now())

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusGatewayTimeout, failoverErr.StatusCode)
	require.False(t, c.Writer.Written(), "passthrough branch must not proxy the fake-success body as HTTP 200")

	eventsAny, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := eventsAny.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.Len(t, events, 1)
	require.Equal(t, "http_200_error_body", events[0].Kind)
	require.True(t, events[0].Passthrough)
	require.Equal(t, "2048947557949771776", events[0].UpstreamRequestID)
}

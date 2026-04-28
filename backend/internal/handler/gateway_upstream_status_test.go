package handler

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGatewayHandler_MapUpstreamError_PreservesGatewayTimeout(t *testing.T) {
	h := &GatewayHandler{}

	status, errType, msg := h.mapUpstreamError(http.StatusGatewayTimeout)

	require.Equal(t, http.StatusGatewayTimeout, status)
	require.Equal(t, "upstream_error", errType)
	require.Equal(t, "Upstream request timed out, please retry later", msg)
}

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type gatewayUsageAccountUserRepo struct {
	service.UserRepository
	user *service.User
}

func (r *gatewayUsageAccountUserRepo) GetByID(context.Context, int64) (*service.User, error) {
	return r.user, nil
}

func (r *gatewayUsageAccountUserRepo) GetUserAvatar(context.Context, int64) (*service.UserAvatar, error) {
	return nil, nil
}

func TestGatewayUsageAccountScopeReturnsWalletInsteadOfKeyQuota(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/usage?scope=account", nil)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		ID:        7,
		UserID:    9,
		Status:    service.StatusAPIKeyActive,
		Quota:     100,
		QuotaUsed: 25,
	})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 9})

	userService := service.NewUserService(&gatewayUsageAccountUserRepo{
		user: &service.User{ID: 9, Balance: 42.75},
	}, nil, nil, nil)
	handler := &GatewayHandler{userService: userService}
	handler.Usage(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "account", response["scope"])
	require.Equal(t, 42.75, response["account_balance"])
	require.NotContains(t, response, "quota")
	require.NotContains(t, response, "remaining")
}

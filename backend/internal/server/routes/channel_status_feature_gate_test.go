//go:build unit

package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newChannelStatusSettings(enabled bool) *service.SettingService {
	value := "false"
	if enabled {
		value = "true"
	}
	return service.NewSettingService(&channelMonitorRouteSettingRepoStub{
		values: map[string]string{
			service.SettingKeyChannelStatusEnabled: value,
		},
	}, &config.Config{})
}

func newChannelStatusRouter(svc *service.SettingService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	statusService := service.NewChannelStatusService(nil, nil, nil)
	handlers := &handler.Handlers{
		ChannelStatus: handler.NewChannelStatusHandler(statusService, svc),
	}
	router := gin.New()
	RegisterChannelStatusRoutes(router.Group("/api/v1"), handlers, middleware.NewPanelRateLimiter(nil, nil))
	return router
}

func TestChannelStatusRouteFeatureGate(t *testing.T) {
	tests := []struct {
		name       string
		enabled    bool
		wantStatus int
	}{
		{name: "disabled returns 404", enabled: false, wantStatus: http.StatusNotFound},
		{name: "enabled returns 200 (empty snapshot)", enabled: true, wantStatus: http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newChannelStatusRouter(newChannelStatusSettings(tt.enabled))
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/channel-status", nil)
			router.ServeHTTP(rec, req)
			require.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantStatus == http.StatusNotFound {
				require.Contains(t, rec.Body.String(), "Channel status is not enabled")
			}
		})
	}
}

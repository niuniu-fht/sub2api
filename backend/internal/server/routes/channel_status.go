package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterChannelStatusRoutes 注册渠道状态统计公开路由。
//
// GET /api/v1/channel-status：匿名可访问（fail-closed，开关关闭由 handler 判定 404），
// 供外部系统与状态页拉取渠道可用性快照（来自后台定时聚合的真实生成记录）。
func RegisterChannelStatusRoutes(
	v1 *gin.RouterGroup,
	h *handler.Handlers,
	panelRateLimiter *middleware.PanelRateLimiter,
) {
	status := v1.Group("/channel-status")
	status.Use(panelRateLimiter.PublicIP())
	{
		status.GET("", h.ChannelStatus.GetChannelStatus)
	}
}

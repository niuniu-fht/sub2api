package handler

import (
	"context"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ChannelStatusHandler 渠道状态统计：公开状态 API + 管理端配置端点。
// 数据来自后台定时聚合的真实生成记录（见 service.ChannelStatusService）。
type ChannelStatusHandler struct {
	statusService  *service.ChannelStatusService
	settingService *service.SettingService
}

func NewChannelStatusHandler(
	statusService *service.ChannelStatusService,
	settingService *service.SettingService,
) *ChannelStatusHandler {
	return &ChannelStatusHandler{
		statusService:  statusService,
		settingService: settingService,
	}
}

// channelStatusEnabled 开关是否启用。默认关闭（opt-in，fail-closed）。
// 直读存储（不做缓存）：公开接口每请求一次轻量 settings 查询，
// PublicIP 限流兜底；聚合器侧另有带缓存的读取。
func (h *ChannelStatusHandler) channelStatusEnabled(c *gin.Context) bool {
	if h.settingService == nil {
		return false
	}
	enabled, _, err := h.settingService.GetChannelStatusSettings(c.Request.Context())
	return err == nil && enabled
}

// GetChannelStatus 公开状态快照。
// GET /api/v1/channel-status（匿名可访问，开关关闭时 404）
func (h *ChannelStatusHandler) GetChannelStatus(c *gin.Context) {
	if !h.channelStatusEnabled(c) {
		response.NotFound(c, "Channel status is not enabled")
		return
	}
	snapshot, err := h.statusService.GetCachedSnapshot(c.Request.Context())
	if err != nil {
		if errors.Is(err, service.ErrChannelStatusNoSnapshot) {
			// 首轮聚合尚未完成：返回空快照而非报错，便于外部系统轮询
			response.Success(c, gin.H{
				"generated_at_ms": time.Now().UnixMilli(),
				"overall":         "unknown",
				"channels":        []any{},
				"ready":           false,
			})
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, snapshot)
}

// ---- 管理端配置 ----

// GetChannelStatusSettings 读取渠道状态统计配置。
// GET /api/v1/admin/settings/channel-status
func (h *ChannelStatusHandler) GetChannelStatusSettings(c *gin.Context) {
	enabled, config, err := h.settingService.GetChannelStatusSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{
		"enabled": enabled,
		"config":  config,
	})
}

type UpdateChannelStatusSettingsRequest struct {
	Enabled bool                     `json:"enabled"`
	Config  service.ChannelStatusConfig `json:"config"`
}

// UpdateChannelStatusSettings 更新渠道状态统计配置（保存后下一轮聚合生效；
// 触发一次立即聚合以尽快产出快照）。
// PUT /api/v1/admin/settings/channel-status
func (h *ChannelStatusHandler) UpdateChannelStatusSettings(c *gin.Context) {
	var req UpdateChannelStatusSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	enabled, normalized, err := h.settingService.SetChannelStatusSettings(c.Request.Context(), req.Enabled, req.Config)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	// 保存后立即聚合一轮，管理端无需等 refresh_seconds
	if enabled && h.statusService != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			if snapshot, err := h.statusService.Aggregate(ctx); err == nil {
				h.statusService.StoreSnapshot(snapshot)
			}
		}()
	}
	response.Success(c, gin.H{
		"enabled": enabled,
		"config":  normalized,
	})
}

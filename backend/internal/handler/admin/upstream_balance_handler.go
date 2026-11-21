package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// UpstreamBalanceHandler 暴露通用上游余额监控端点（对齐 new-api 渠道余额）。
//
//   - GET  /admin/upstream-balances          全账号列表 + 最新余额快照
//   - POST /admin/upstream-balances/refresh  批量刷新（body.account_ids 可选，空=全部可查询账号）
//   - POST /admin/accounts/:id/upstream-balance  查询单个并落快照
type UpstreamBalanceHandler struct {
	balanceService *service.UpstreamBalanceService
}

func NewUpstreamBalanceHandler(balanceService *service.UpstreamBalanceService) *UpstreamBalanceHandler {
	return &UpstreamBalanceHandler{balanceService: balanceService}
}

// List 返回全部账号的余额视图（含 extra.upstream_balance 快照）。
func (h *UpstreamBalanceHandler) List(c *gin.Context) {
	if h == nil || h.balanceService == nil {
		response.BadRequest(c, "upstream balance service is not enabled")
		return
	}
	items, err := h.balanceService.ListBalances(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"items": items})
}

// RefreshOne 查询单个账号余额并落快照。
func (h *UpstreamBalanceHandler) RefreshOne(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	if h == nil || h.balanceService == nil {
		response.BadRequest(c, "upstream balance service is not enabled")
		return
	}
	result, err := h.balanceService.QueryBalance(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// RefreshBatch 批量刷新余额；body 可选 {"account_ids":[1,2]}，缺省刷新全部可查询账号。
func (h *UpstreamBalanceHandler) RefreshBatch(c *gin.Context) {
	if h == nil || h.balanceService == nil {
		response.BadRequest(c, "upstream balance service is not enabled")
		return
	}
	var req struct {
		AccountIDs []int64 `json:"account_ids"`
	}
	_ = c.ShouldBindJSON(&req)
	// 批量可能耗时（账号数 × 限速 / 并发），放宽到 30 分钟。
	ctx := c.Request.Context()
	results, err := h.balanceService.RefreshAll(ctx, req.AccountIDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"items": results})
}

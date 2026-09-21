package handler

// WalletLedgerHandler Final Frontend CLOSURE：
//   - 用户钱包流水（GET /api/v1/wallet/ledger）
//   - 管理端 Reward 发放记录/统计（GET /api/v1/admin/rewards[/stats]）
//   - 管理端套餐变更审计（GET /api/v1/admin/plan-changes）
// 全部为只读查询，复用既有 service 事实表，不改变任何余额/套餐语义。

import (
	"errors"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type WalletLedgerHandler struct {
	ledger      *service.WalletLedgerService
	planChanges *service.PlanChangeService
}

func NewWalletLedgerHandler(ledger *service.WalletLedgerService, planChanges *service.PlanChangeService) *WalletLedgerHandler {
	return &WalletLedgerHandler{ledger: ledger, planChanges: planChanges}
}

func parseOptionalInt64(v string) (*int64, error) {
	if v == "" {
		return nil, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return nil, err
	}
	if n <= 0 {
		return nil, errors.New("must be positive")
	}
	return &n, nil
}

func parseLimitOffset(c *gin.Context, defLimit int) (int, int) {
	limit, _ := strconv.Atoi(c.DefaultQuery("page_size", strconv.Itoa(defLimit)))
	if limit <= 0 || limit > 100 {
		limit = defLimit
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page <= 0 {
		page = 1
	}
	return limit, (page - 1) * limit
}

// UserLedger GET /api/v1/wallet/ledger（用户侧钱包流水）
func (h *WalletLedgerHandler) UserLedger(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	limit, offset := parseLimitOffset(c, 50)
	entries, total, err := h.ledger.ListUserLedger(c.Request.Context(), subject.UserID, limit, offset)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"entries": entries, "total": total, "page": offset/limit + 1, "page_size": limit})
}

// AdminLedger uses the same money-only ledger as the user wallet.
func (h *WalletLedgerHandler) AdminLedger(c *gin.Context) {
	userID, err := parseOptionalInt64(c.Query("user_id"))
	if err != nil || userID == nil {
		response.BadRequest(c, "positive user_id is required")
		return
	}
	limit, offset := parseLimitOffset(c, 20)
	entries, total, err := h.ledger.ListUserLedger(c.Request.Context(), *userID, limit, offset)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"entries": entries, "total": total, "page": offset/limit + 1, "page_size": limit})
}

// AdminRewardList GET /api/v1/admin/rewards?user_id=&page=&page_size=
func (h *WalletLedgerHandler) AdminRewardList(c *gin.Context) {
	userID, err := parseOptionalInt64(c.Query("user_id"))
	if err != nil {
		response.BadRequest(c, "invalid user_id")
		return
	}
	limit, offset := parseLimitOffset(c, 20)
	items, total, err := h.ledger.AdminRewardList(c.Request.Context(), userID, limit, offset)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"items": items, "total": total})
}

// AdminRewardStats GET /api/v1/admin/rewards/stats?user_id=
func (h *WalletLedgerHandler) AdminRewardStats(c *gin.Context) {
	userID, err := parseOptionalInt64(c.Query("user_id"))
	if err != nil {
		response.BadRequest(c, "invalid user_id")
		return
	}
	today, month, err := h.ledger.RewardDayStats(c.Request.Context(), userID, time.Now())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{
		"today_count": today.Count, "today_sum": today.Sum,
		"month_count": month.Count, "month_sum": month.Sum,
	})
}

// AdminPlanChangeList GET /api/v1/admin/plan-changes?user_id=&subscription_id=&status=&page=&page_size=
func (h *WalletLedgerHandler) AdminPlanChangeList(c *gin.Context) {
	userID, err := parseOptionalInt64(c.Query("user_id"))
	if err != nil {
		response.BadRequest(c, "invalid user_id")
		return
	}
	subID, err := parseOptionalInt64(c.Query("subscription_id"))
	if err != nil {
		response.BadRequest(c, "invalid subscription_id")
		return
	}
	status := c.Query("status")
	limit, offset := parseLimitOffset(c, 20)
	records, total, err := h.planChanges.AdminListChanges(c.Request.Context(), service.PlanChangeAdminFilter{
		UserID:         userID,
		SubscriptionID: subID,
		Status:         &status,
		Limit:          limit,
		Offset:         offset,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"items": records, "total": total})
}

package admin

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// 奖励发放记录管理端只读接口（挂 routes/admin.go 管理链）：
//   - GET /api/v1/admin/reward-grants?page=&page_size=&user_id=&campaign=&source_type=
//
// 只读台账：reward_grants 是"系统奖励余额"的唯一事实源（发放走幂等键，本端点不做任何写入）。

// RewardGrantHandler 奖励发放记录管理端 handler
type RewardGrantHandler struct {
	rewardGrantService *service.RewardGrantService
}

// NewRewardGrantHandler 创建奖励发放记录管理端 handler
func NewRewardGrantHandler(rewardGrantService *service.RewardGrantService) *RewardGrantHandler {
	return &RewardGrantHandler{rewardGrantService: rewardGrantService}
}

// rewardGrantAdminResponse 管理端发放记录列表项（导出契约字段）
type rewardGrantAdminResponse struct {
	ID             int64   `json:"id"`
	UserID         int64   `json:"user_id"`
	Email          string  `json:"email"`
	Username       string  `json:"username"`
	IdempotencyKey string  `json:"idempotency_key"`
	SourceType     string  `json:"source_type"`
	SourceID       *int64  `json:"source_id"`
	Campaign       string  `json:"campaign"`
	Amount         float64 `json:"amount"`
	GrantedBy      *int64  `json:"granted_by"`
	GrantedByEmail string  `json:"granted_by_email"`
	CreatedAt      string  `json:"created_at"`
}

// List 分页查询奖励发放记录（user_id/campaign/source_type 可选精确过滤；page 默认 1，page_size 默认 20 上限 100）
// GET /api/v1/admin/reward-grants
func (h *RewardGrantHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	if pageSize > 100 {
		pageSize = 100
	}

	filter := &service.RewardGrantAdminFilter{
		Page:       page,
		PageSize:   pageSize,
		Campaign:   strings.TrimSpace(c.Query("campaign")),
		SourceType: strings.TrimSpace(c.Query("source_type")),
	}
	if v := strings.TrimSpace(c.Query("user_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			response.BadRequest(c, "Invalid user_id")
			return
		}
		filter.UserID = &id
	}

	result, err := h.rewardGrantService.AdminListRewardGrants(c.Request.Context(), filter)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	items := make([]rewardGrantAdminResponse, 0, len(result.Items))
	for i := range result.Items {
		g := &result.Items[i]
		items = append(items, rewardGrantAdminResponse{
			ID:             g.ID,
			UserID:         g.UserID,
			Email:          g.Email,
			Username:       g.Username,
			IdempotencyKey: g.IdempotencyKey,
			SourceType:     g.SourceType,
			SourceID:       g.SourceID,
			Campaign:       g.Campaign,
			Amount:         g.Amount,
			GrantedBy:      g.GrantedBy,
			GrantedByEmail: g.GrantedByEmail,
			CreatedAt:      g.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	response.Paginated(c, items, int64(result.Total), result.Page, result.PageSize)
}

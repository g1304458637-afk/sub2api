package admin

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// 科研优惠登记管理端接口（挂 routes/admin.go 管理链）：
//   - GET  /api/v1/admin/research-applications?status=&page=&page_size= 审核列表
//   - GET  /api/v1/admin/research-applications/:aid/attachments/:attId  附件下载（管理员）
//   - POST /api/v1/admin/research-applications/:aid/approve             通过并静默发放余额
//   - POST /api/v1/admin/research-applications/:aid/reject              驳回（备注必填）

// ResearchHandler 科研优惠登记管理端 handler
type ResearchHandler struct {
	researchService *service.ResearchApplicationService
}

// NewResearchHandler 创建科研优惠登记管理端 handler
func NewResearchHandler(researchService *service.ResearchApplicationService) *ResearchHandler {
	return &ResearchHandler{researchService: researchService}
}

// researchAdminApplicationResponse 管理端申请列表项（冻结契约字段）
type researchAdminApplicationResponse struct {
	ID           int64                            `json:"id"`
	User         *service.ResearchApplicationUser `json:"user"`
	Description  string                           `json:"description"`
	Status       string                           `json:"status"`
	ReviewNotes  *string                          `json:"review_notes"`
	RewardAmount *float64                         `json:"reward_amount"`
	CreatedAt    string                           `json:"created_at"`
	ReviewedAt   *string                          `json:"reviewed_at"`
	Attachments  []service.ResearchAttachmentMeta `json:"attachments"`
}

// List 审核列表（status 可选过滤，缺省全部；page 默认 1，page_size 默认 20 上限 100）
// GET /api/v1/admin/research-applications
func (h *ResearchHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	if pageSize > 100 {
		pageSize = 100
	}
	params := pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortOrder: pagination.SortOrderDesc,
	}

	details, total, err := h.researchService.AdminListApplicationsWithUsers(c.Request.Context(), params, strings.TrimSpace(c.Query("status")))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	items := make([]researchAdminApplicationResponse, 0, len(details))
	for i := range details {
		d := details[i]
		resp := researchAdminApplicationResponse{
			ID:           d.Application.ID,
			User:         d.User,
			Description:  d.Application.Description,
			Status:       d.Application.Status,
			ReviewNotes:  d.Application.ReviewNotes,
			RewardAmount: d.Application.RewardAmount,
			CreatedAt:    d.Application.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
			Attachments:  d.Application.Attachments,
		}
		if resp.Attachments == nil {
			resp.Attachments = []service.ResearchAttachmentMeta{}
		}
		if d.Application.ReviewedAt != nil {
			t := d.Application.ReviewedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
			resp.ReviewedAt = &t
		}
		items = append(items, resp)
	}
	response.Paginated(c, items, total, page, pageSize)
}

// DownloadAttachment 附件下载（权限 = 管理员，由路由层管理链鉴权）
// GET /api/v1/admin/research-applications/:aid/attachments/:attId
func (h *ResearchHandler) DownloadAttachment(c *gin.Context) {
	applicationID, ok := dto.ResearchParseIDParam(c, "aid")
	if !ok {
		return
	}
	attachmentID := strings.TrimSpace(c.Param("attId"))
	if attachmentID == "" {
		response.BadRequest(c, "attachment id is required")
		return
	}

	download, err := h.researchService.GetAttachmentForAdminDownload(c.Request.Context(), applicationID, attachmentID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	dto.ResearchServeAttachment(c, download)
}

// researchApproveRequest POST /:aid/approve 请求体
type researchApproveRequest struct {
	Amount float64 `json:"amount" binding:"required,gt=0,lte=10000"`
	Notes  string  `json:"notes"`
}

// Approve 审核通过并静默发放等额余额兑换券
// POST /api/v1/admin/research-applications/:aid/approve
func (h *ResearchHandler) Approve(c *gin.Context) {
	applicationID, ok := dto.ResearchParseIDParam(c, "aid")
	if !ok {
		return
	}
	var req researchApproveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	adminID, ok := dto.ResearchUserID(c)
	if !ok {
		response.Unauthorized(c, "Admin not authenticated")
		return
	}

	app, err := h.researchService.Approve(c.Request.Context(), adminID, applicationID, service.ResearchApproveInput{
		Amount: req.Amount,
		Notes:  req.Notes,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.ResearchApplicationToResponse(app))
}

// researchRejectRequest POST /:aid/reject 请求体
type researchRejectRequest struct {
	Notes string `json:"notes"`
}

// Reject 驳回申请（备注必填）
// POST /api/v1/admin/research-applications/:aid/reject
func (h *ResearchHandler) Reject(c *gin.Context) {
	applicationID, ok := dto.ResearchParseIDParam(c, "aid")
	if !ok {
		return
	}
	var req researchRejectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	adminID, ok := dto.ResearchUserID(c)
	if !ok {
		response.Unauthorized(c, "Admin not authenticated")
		return
	}

	app, err := h.researchService.Reject(c.Request.Context(), adminID, applicationID, req.Notes)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.ResearchApplicationToResponse(app))
}

package handler

import (
	"io"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// 科研优惠登记用户端接口（挂 routes/user.go 的 JWT 面板鉴权链）：
//   - POST /api/v1/research-applications/attachments  上传附件（≤5MB，白名单类型）
//   - POST /api/v1/research-applications              提交申请
//   - GET  /api/v1/research-applications              本人申请列表（倒序）
//   - GET  /api/v1/research-applications/:aid/attachments/:attId 附件下载（仅本人）

const (
	// researchMaxUploadBytes 单附件字节上限（5MB）
	researchMaxUploadBytes = 5 << 20
	// researchMaxBodyBytes multipart 解析整体上限（5MB 文件 + 表单边界等开销）
	researchMaxBodyBytes = researchMaxUploadBytes + (64 << 10)
)

// ResearchApplicationHandler 科研优惠登记用户端 handler
type ResearchApplicationHandler struct {
	researchService *service.ResearchApplicationService
}

// NewResearchApplicationHandler 创建科研优惠登记用户端 handler
func NewResearchApplicationHandler(researchService *service.ResearchApplicationService) *ResearchApplicationHandler {
	return &ResearchApplicationHandler{researchService: researchService}
}

// UploadAttachment 上传科研身份证明附件
// POST /api/v1/research-applications/attachments （multipart，字段名 file）
func (h *ResearchApplicationHandler) UploadAttachment(c *gin.Context) {
	userID, ok := dto.ResearchUserID(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	// 硬限请求体大小：5MB + multipart 编码开销
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, researchMaxBodyBytes)

	fileHeader, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, "file is required and must be at most 5MB")
		return
	}
	if fileHeader.Size > researchMaxUploadBytes {
		response.BadRequest(c, "attachment exceeds the 5MB size limit")
		return
	}

	f, err := fileHeader.Open()
	if err != nil {
		response.BadRequest(c, "failed to read uploaded file")
		return
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		response.BadRequest(c, "failed to read uploaded file")
		return
	}

	meta, err := h.researchService.SaveAttachment(
		c.Request.Context(),
		userID,
		fileHeader.Filename,
		fileHeader.Header.Get("Content-Type"),
		data,
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{
		"id":   meta.ID,
		"name": meta.Name,
		"mime": meta.Mime,
		"size": meta.Size,
	})
}

// researchCreateApplicationRequest POST /api/v1/research-applications 请求体
type researchCreateApplicationRequest struct {
	Description   string   `json:"description"`
	AttachmentIDs []string `json:"attachment_ids"`
}

// Create 提交科研优惠申请
// POST /api/v1/research-applications
func (h *ResearchApplicationHandler) Create(c *gin.Context) {
	userID, ok := dto.ResearchUserID(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req researchCreateApplicationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	app, err := h.researchService.CreateApplication(c.Request.Context(), userID, service.ResearchCreateApplicationInput{
		Description:   req.Description,
		AttachmentIDs: req.AttachmentIDs,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.ResearchApplicationToResponse(app))
}

// List 本人申请列表（按创建时间倒序）
// GET /api/v1/research-applications
func (h *ResearchApplicationHandler) List(c *gin.Context) {
	userID, ok := dto.ResearchUserID(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	apps, err := h.researchService.ListMyApplications(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	items := make([]dto.ResearchApplicationResponse, 0, len(apps))
	for i := range apps {
		items = append(items, dto.ResearchApplicationToResponse(&apps[i]))
	}
	response.Success(c, gin.H{"items": items})
}

// DownloadAttachment 下载附件（权限 = 仅本人）
// GET /api/v1/research-applications/:aid/attachments/:attId
func (h *ResearchApplicationHandler) DownloadAttachment(c *gin.Context) {
	userID, ok := dto.ResearchUserID(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	applicationID, ok := dto.ResearchParseIDParam(c, "aid")
	if !ok {
		return
	}
	attachmentID := strings.TrimSpace(c.Param("attId"))
	if attachmentID == "" {
		response.BadRequest(c, "attachment id is required")
		return
	}

	download, err := h.researchService.GetAttachmentForUserDownload(c.Request.Context(), userID, applicationID, attachmentID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	dto.ResearchServeAttachment(c, download)
}

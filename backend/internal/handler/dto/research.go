package dto

import (
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// 科研优惠登记 HTTP 层共享辅助：用户端（internal/handler）与管理端
// （internal/handler/admin）都要用，放 dto 包避免两个 handler 包互相 import 成环。

// ResearchApplicationResponse 申请对象（冻结契约字段）
type ResearchApplicationResponse struct {
	ID           int64                            `json:"id"`
	Description  string                           `json:"description"`
	Status       string                           `json:"status"`
	ReviewNotes  *string                          `json:"review_notes"`
	RewardAmount *float64                         `json:"reward_amount"`
	CreatedAt    string                           `json:"created_at"`
	ReviewedAt   *string                          `json:"reviewed_at"`
	Attachments  []service.ResearchAttachmentMeta `json:"attachments"`
}

// ResearchApplicationToResponse 领域对象 → 契约 JSON（时间 RFC3339 UTC）
func ResearchApplicationToResponse(app *service.ResearchApplication) ResearchApplicationResponse {
	resp := ResearchApplicationResponse{
		ID:           app.ID,
		Description:  app.Description,
		Status:       app.Status,
		ReviewNotes:  app.ReviewNotes,
		RewardAmount: app.RewardAmount,
		CreatedAt:    app.CreatedAt.UTC().Format(time.RFC3339),
		Attachments:  app.Attachments,
	}
	if resp.Attachments == nil {
		resp.Attachments = []service.ResearchAttachmentMeta{}
	}
	if app.ReviewedAt != nil {
		t := app.ReviewedAt.UTC().Format(time.RFC3339)
		resp.ReviewedAt = &t
	}
	return resp
}

// ResearchUserID 从 JWT 面板上下文取当前用户 ID
func ResearchUserID(c *gin.Context) (int64, bool) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		return 0, false
	}
	return subject.UserID, true
}

// ResearchParseIDParam 解析 :aid 数字路径参数（非法时直接写 400 响应）
func ResearchParseIDParam(c *gin.Context, name string) (int64, bool) {
	raw := c.Param(name)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid application id")
		return 0, false
	}
	return id, true
}

// ResearchServeAttachment 以存储的 mime + 原始文件名输出附件文件流。
// 文件名遵循 RFC 5987：非 ASCII 时提供 filename* 编码值 + ASCII fallback。
func ResearchServeAttachment(c *gin.Context, download *service.ResearchAttachmentDownload) {
	data, err := os.ReadFile(download.AbsPath)
	if err != nil {
		response.NotFound(c, "attachment not found")
		return
	}

	mime := download.Meta.Mime
	if mime == "" {
		mime = "application/octet-stream"
	}
	c.Header("Content-Disposition", ResearchContentDisposition(download.Meta.Name))
	// 防止浏览器对返回内容做 MIME 嗅探（附件声明为图片/PDF 也一律按附件下发）
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, mime, data)
}

// ResearchContentDisposition 构造 Content-Disposition 头（attachment；RFC 5987 文件名）
func ResearchContentDisposition(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "attachment"
	}
	fallback := ResearchASCIIFallback(name)
	if fallback == name {
		return `attachment; filename="` + strings.ReplaceAll(name, `"`, "'") + `"`
	}
	// RFC 5987：filename* 用 RFC 3986 percent-encoding（UTF-8）
	return `attachment; filename="` + fallback + `"; filename*=UTF-8''` + url.PathEscape(name)
}

// ResearchASCIIFallback 将非 ASCII/危险字符替换为 '_'，得到下载头的 ASCII 兜底文件名
func ResearchASCIIFallback(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r < 0x20 || r > 0x7e || r == '"' || r == '\\' {
			b.WriteByte('_')
			continue
		}
		b.WriteRune(r)
	}
	out := b.String()
	if out == "" {
		out = "attachment"
	}
	return out
}

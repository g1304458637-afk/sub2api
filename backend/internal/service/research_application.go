package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// ─────────────────────────────────────────────────────────────────────────────
// 科研优惠登记（Research Discount Application）
//
// 语义：用户提交科研身份证明（文字说明 + 附件图片/PDF）→ 管理员在管理端审核；
// 通过时后端静默给该用户发放等额余额兑换券（RedeemService.CreateCode +
// RedeemForAdminFulfillment），驳回必须填写备注。
//
// 分层：本文件为领域模型 + 仓储接口 + 服务实现；仓储实现在
// internal/repository/research_application_repo.go；HTTP 层见
// internal/handler/research_application_handler.go（用户端）与
// internal/handler/admin/research_handler.go（管理端）。
// ─────────────────────────────────────────────────────────────────────────────

const (
	// 科研申请状态（与 ent schema 常量保持一致）
	ResearchApplicationStatusPending  = "pending"
	ResearchApplicationStatusApproved = "approved"
	ResearchApplicationStatusRejected = "rejected"

	// researchDescriptionMaxRunes 描述长度上限（字符数，按 rune 计）
	researchDescriptionMaxRunes = 2000
	// researchMaxAttachments 单个申请附件数上限
	researchMaxAttachments = 5
	// researchMaxAttachmentSize 单附件字节上限（5MB）
	researchMaxAttachmentSize = 5 << 20
	// researchMaxRewardAmount 审核通过时发放金额上限
	researchMaxRewardAmount = 10000.0
	// researchStorageSubdir 附件在 DATA_DIR 下的相对存储目录
	researchStorageSubdir = "uploads/research"
)

// 科研附件白名单 MIME（含 ent/migration 校验口径，存储与下载均以此为准）
var researchAllowedAttachmentMimes = map[string]string{
	"image/jpeg":      ".jpg",
	"image/png":       ".png",
	"image/webp":      ".webp",
	"application/pdf": ".pdf",
}

// 科研申请业务错误
var (
	ErrResearchApplicationNotFound   = infraerrors.NotFound("RESEARCH_APPLICATION_NOT_FOUND", "research application not found")
	ErrResearchAttachmentNotFound    = infraerrors.NotFound("RESEARCH_ATTACHMENT_NOT_FOUND", "research attachment not found")
	ErrResearchApplicationNotPending = infraerrors.Conflict("RESEARCH_APPLICATION_NOT_PENDING", "research application has already been reviewed")
	ErrResearchAttachmentInvalid     = infraerrors.BadRequest("RESEARCH_ATTACHMENT_INVALID", "invalid research attachment")
	ErrResearchAttachmentType        = infraerrors.BadRequest("RESEARCH_ATTACHMENT_TYPE_UNSUPPORTED", "unsupported attachment type")
	ErrResearchAttachmentTooLarge    = infraerrors.BadRequest("RESEARCH_ATTACHMENT_TOO_LARGE", "attachment exceeds the 5MB size limit")
	ErrResearchDescriptionRequired   = infraerrors.BadRequest("RESEARCH_DESCRIPTION_REQUIRED", "description is required")
	ErrResearchDescriptionTooLong    = infraerrors.BadRequest("RESEARCH_DESCRIPTION_TOO_LONG", "description must be at most 2000 characters")
	ErrResearchNotesRequired         = infraerrors.BadRequest("RESEARCH_NOTES_REQUIRED", "review notes are required")
	ErrResearchAmountInvalid         = infraerrors.BadRequest("RESEARCH_AMOUNT_INVALID", "amount must be greater than 0 and at most 10000")
	ErrResearchStatusInvalid         = infraerrors.BadRequest("RESEARCH_STATUS_INVALID", "invalid status filter")
	ErrResearchRedeemIssueFailed     = infraerrors.InternalServer("RESEARCH_REDEEM_ISSUE_FAILED", "failed to issue reward redeem code")
)

// ResearchAttachmentMeta 附件元数据（jsonb 冗余存于申请行；不含物理路径）
type ResearchAttachmentMeta struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Mime string `json:"mime"`
	Size int64  `json:"size"`
}

// ResearchApplication 科研优惠申请领域模型
type ResearchApplication struct {
	ID                 int64
	UserID             int64
	Description        string
	Status             string
	ReviewerID         *int64
	ReviewNotes        *string
	RewardAmount       *float64
	IssuedRedeemCodeID *int64
	ReviewedAt         *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
	Attachments        []ResearchAttachmentMeta
}

// ResearchAttachmentUpload 附件上传记录领域模型（含物理路径，仅服务端使用）
type ResearchAttachmentUpload struct {
	ID            string
	UserID        int64
	OriginalName  string
	Mime          string
	Size          int64
	StoragePath   string
	CreatedAt     time.Time
	ApplicationID *int64
}

// ResearchApplicationUser 管理端列表内嵌的申请人信息
type ResearchApplicationUser struct {
	ID       int64  `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
}

// ResearchAttachmentDownload 附件下载所需信息
type ResearchAttachmentDownload struct {
	AbsPath string
	Meta    ResearchAttachmentMeta
}

// ─────────────────────────── 仓储接口（repository 层实现） ───────────────────────────

// ResearchApplicationRepository 科研申请仓储接口
type ResearchApplicationRepository interface {
	// CreateWithAttachmentBinding 在单个事务内创建申请并按条件绑定附件
	// （WHERE id IN (...) AND user_id=? AND application_id IS NULL）。
	// 任一附件不存在/不属于本人/已被绑定则整体失败并返回 ErrResearchAttachmentInvalid。
	CreateWithAttachmentBinding(ctx context.Context, app *ResearchApplication, attachmentIDs []string) error
	GetByID(ctx context.Context, id int64) (*ResearchApplication, error)
	Delete(ctx context.Context, id int64) error
	// ListByUser 本人申请列表，按创建时间倒序
	ListByUser(ctx context.Context, userID int64) ([]ResearchApplication, error)
	// ListByStatus 管理端分页列表；status 为空串返回全部；按创建时间倒序
	ListByStatus(ctx context.Context, params pagination.PaginationParams, status string) ([]ResearchApplication, int64, error)
	// ClaimForApproval 审核判赢：条件更新 WHERE id=? AND status='pending'，
	// 命中则写入审核字段并置为 approved，返回是否抢到（并发/重复 approve 幂等保护）。
	ClaimForApproval(ctx context.Context, id int64, reviewerID int64, notes string, rewardAmount float64, reviewedAt time.Time) (bool, error)
	// SetIssuedRedeemCodeID 回填静默发放的兑换码 ID
	SetIssuedRedeemCodeID(ctx context.Context, id int64, redeemCodeID int64) error
	// RevertToPending 发放失败回滚：清空审核字段并恢复 pending（条件更新 WHERE status='approved'）
	RevertToPending(ctx context.Context, id int64) error
	// MarkRejected 驳回判赢：条件更新 WHERE id=? AND status='pending'
	MarkRejected(ctx context.Context, id int64, reviewerID int64, notes string, reviewedAt time.Time) (bool, error)
}

// ResearchAttachmentUploadRepository 附件上传记录仓储接口
type ResearchAttachmentUploadRepository interface {
	Create(ctx context.Context, up *ResearchAttachmentUpload) error
	GetByID(ctx context.Context, id string) (*ResearchAttachmentUpload, error)
}

// ResearchRedeemIssuer 静默发放余额兑换码所需的兑换服务能力
// （*RedeemService 实现；wire.Bind 注入）
type ResearchRedeemIssuer interface {
	CreateCode(ctx context.Context, code *RedeemCode) error
	RedeemForAdminFulfillment(ctx context.Context, userID int64, code string) (*RedeemCode, error)
}

// ResearchUserReader 管理端列表回填申请人信息所需能力（*UserService 实现）
type ResearchUserReader interface {
	GetByID(ctx context.Context, id int64) (*User, error)
}

// ─────────────────────────── 服务 ───────────────────────────

// ResearchApplicationService 科研优惠登记服务
type ResearchApplicationService struct {
	appRepo    ResearchApplicationRepository
	uploadRepo ResearchAttachmentUploadRepository
	redeem     ResearchRedeemIssuer
	userReader ResearchUserReader
	// dataDir 附件存储根目录（DATA_DIR），由装配层解析注入
	dataDir string
}

// NewResearchApplicationService 创建科研优惠登记服务
func NewResearchApplicationService(
	appRepo ResearchApplicationRepository,
	uploadRepo ResearchAttachmentUploadRepository,
	redeem ResearchRedeemIssuer,
	userReader ResearchUserReader,
	dataDir string,
) *ResearchApplicationService {
	return &ResearchApplicationService{
		appRepo:    appRepo,
		uploadRepo: uploadRepo,
		redeem:     redeem,
		userReader: userReader,
		dataDir:    dataDir,
	}
}

// SaveAttachment 校验并保存附件：白名单 MIME + 5MB 上限 + 魔数嗅探，
// 落盘 DATA_DIR/uploads/research/<uuid><ext> 并写待绑定记录表。
// 返回给前端的元数据（id 为 UUID）。
func (s *ResearchApplicationService) SaveAttachment(ctx context.Context, userID int64, originalName, declaredMime string, data []byte) (*ResearchAttachmentMeta, error) {
	declaredMime = normalizeResearchMime(declaredMime)
	ext, ok := researchAllowedAttachmentMimes[declaredMime]
	if !ok {
		return nil, ErrResearchAttachmentType
	}
	if len(data) == 0 || len(data) > researchMaxAttachmentSize {
		return nil, ErrResearchAttachmentTooLarge
	}
	// 魔数嗅探：防止用白名单 Content-Type 伪装其他类型文件
	if sniffed := http.DetectContentType(sniffPrefix(data)); sniffed != declaredMime {
		return nil, infraerrors.BadRequest("RESEARCH_ATTACHMENT_CONTENT_MISMATCH", "attachment content does not match its declared type")
	}

	attachmentID := newResearchUUID()
	relPath := researchStorageSubdir + "/" + attachmentID + ext
	absPath := filepath.Join(s.dataDir, filepath.FromSlash(relPath))

	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return nil, fmt.Errorf("create research upload dir: %w", err)
	}
	if err := os.WriteFile(absPath, data, 0o644); err != nil {
		return nil, fmt.Errorf("write research attachment: %w", err)
	}

	name := strings.TrimSpace(originalName)
	if name == "" {
		name = attachmentID + ext
	}
	upload := &ResearchAttachmentUpload{
		ID:           attachmentID,
		UserID:       userID,
		OriginalName: name,
		Mime:         declaredMime,
		Size:         int64(len(data)),
		StoragePath:  relPath,
	}
	if err := s.uploadRepo.Create(ctx, upload); err != nil {
		// DB 记录失败时尽力清理已落盘文件，避免孤儿文件
		_ = os.Remove(absPath)
		return nil, fmt.Errorf("create research attachment record: %w", err)
	}

	return &ResearchAttachmentMeta{
		ID:   attachmentID,
		Name: name,
		Mime: declaredMime,
		Size: int64(len(data)),
	}, nil
}

// ResearchCreateApplicationInput POST /api/v1/research-applications 输入
type ResearchCreateApplicationInput struct {
	Description   string
	AttachmentIDs []string
}

// CreateApplication 提交科研优惠申请：校验描述长度与附件归属/绑定状态，
// 单事务内创建申请（status=pending）并绑定附件。
func (s *ResearchApplicationService) CreateApplication(ctx context.Context, userID int64, input ResearchCreateApplicationInput) (*ResearchApplication, error) {
	description := strings.TrimSpace(input.Description)
	runes := utf8.RuneCountInString(description)
	if runes == 0 {
		return nil, ErrResearchDescriptionRequired
	}
	if runes > researchDescriptionMaxRunes {
		return nil, ErrResearchDescriptionTooLong
	}

	attachmentIDs := make([]string, 0, len(input.AttachmentIDs))
	seen := make(map[string]struct{}, len(input.AttachmentIDs))
	for _, id := range input.AttachmentIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			return nil, ErrResearchAttachmentInvalid
		}
		if _, dup := seen[id]; dup {
			return nil, ErrResearchAttachmentInvalid
		}
		seen[id] = struct{}{}
		attachmentIDs = append(attachmentIDs, id)
	}
	if len(attachmentIDs) > researchMaxAttachments {
		return nil, infraerrors.BadRequest("RESEARCH_ATTACHMENT_TOO_MANY", fmt.Sprintf("at most %d attachments are allowed", researchMaxAttachments))
	}

	metas, err := s.resolveAttachmentMetas(ctx, userID, attachmentIDs)
	if err != nil {
		return nil, err
	}

	app := &ResearchApplication{
		UserID:      userID,
		Description: description,
		Status:      ResearchApplicationStatusPending,
		Attachments: metas,
	}
	if err := s.appRepo.CreateWithAttachmentBinding(ctx, app, attachmentIDs); err != nil {
		return nil, err
	}
	return app, nil
}

// ListMyApplications 本人申请列表（按创建时间倒序）
func (s *ResearchApplicationService) ListMyApplications(ctx context.Context, userID int64) ([]ResearchApplication, error) {
	return s.appRepo.ListByUser(ctx, userID)
}

// GetAttachmentForUserDownload 用户端附件下载：权限 = 仅本人（且附件已绑定到本人该申请）
func (s *ResearchApplicationService) GetAttachmentForUserDownload(ctx context.Context, userID int64, applicationID int64, attachmentID string) (*ResearchAttachmentDownload, error) {
	app, err := s.getApplicationForViewer(ctx, applicationID, userID)
	if err != nil {
		return nil, err
	}
	return s.resolveAttachmentDownload(ctx, app, attachmentID)
}

// AdminListApplications 管理端分页列表（status 可选：pending/approved/rejected，缺省全部）
func (s *ResearchApplicationService) AdminListApplications(ctx context.Context, params pagination.PaginationParams, status string) ([]ResearchApplication, int64, error) {
	if status != "" && !isValidResearchStatus(status) {
		return nil, 0, ErrResearchStatusInvalid
	}
	return s.appRepo.ListByStatus(ctx, params, status)
}

// AdminApplicationDetail 管理端列表项（含申请人信息）
type AdminApplicationDetail struct {
	Application ResearchApplication
	User        *ResearchApplicationUser
}

// AdminListApplicationsWithUsers 管理端列表并回填申请人 {id,email,username}
func (s *ResearchApplicationService) AdminListApplicationsWithUsers(ctx context.Context, params pagination.PaginationParams, status string) ([]AdminApplicationDetail, int64, error) {
	apps, total, err := s.AdminListApplications(ctx, params, status)
	if err != nil {
		return nil, 0, err
	}
	details := make([]AdminApplicationDetail, 0, len(apps))
	for i := range apps {
		details = append(details, AdminApplicationDetail{
			Application: apps[i],
			User:        s.applicationUser(ctx, apps[i].UserID),
		})
	}
	return details, total, nil
}

// GetAttachmentForAdminDownload 管理端附件下载：权限 = 管理员（路由层已做管理员鉴权）
func (s *ResearchApplicationService) GetAttachmentForAdminDownload(ctx context.Context, applicationID int64, attachmentID string) (*ResearchAttachmentDownload, error) {
	app, err := s.getApplicationForViewer(ctx, applicationID, 0)
	if err != nil {
		return nil, err
	}
	return s.resolveAttachmentDownload(ctx, app, attachmentID)
}

// ResearchApproveInput POST /api/v1/admin/research-applications/:aid/approve 输入
type ResearchApproveInput struct {
	Amount float64
	Notes  string
}

// Approve 审核通过并静默发放等额余额兑换券。
//
// 并发/重复 approve 幂等：先以条件更新（WHERE status='pending'）判赢抢占审核位，
// 抢占失败说明已被审核，返回 409。发放为"事务性尽力而为"：
// CreateCode / RedeemForAdminFulfillment 任一失败则回滚审核字段（状态恢复 pending）
// 并返回错误；成功后回填 issued_redeem_code_id。
func (s *ResearchApplicationService) Approve(ctx context.Context, adminID int64, applicationID int64, input ResearchApproveInput) (*ResearchApplication, error) {
	if input.Amount <= 0 || input.Amount > researchMaxRewardAmount {
		return nil, ErrResearchAmountInvalid
	}
	app, err := s.getApplicationForViewer(ctx, applicationID, 0)
	if err != nil {
		return nil, err
	}
	if app.Status != ResearchApplicationStatusPending {
		return nil, ErrResearchApplicationNotPending
	}

	notes := strings.TrimSpace(input.Notes)
	now := time.Now()
	won, err := s.appRepo.ClaimForApproval(ctx, app.ID, adminID, notes, input.Amount, now)
	if err != nil {
		return nil, err
	}
	if !won {
		return nil, ErrResearchApplicationNotPending
	}

	code, err := s.issueResearchRedeemCode(ctx, app.UserID, app.ID, input.Amount)
	if err != nil {
		// 发放失败：申请状态不变（恢复 pending），下次可重试
		_ = s.appRepo.RevertToPending(ctx, app.ID)
		return nil, err
	}
	if err := s.appRepo.SetIssuedRedeemCodeID(ctx, app.ID, code.ID); err != nil {
		return nil, err
	}
	return s.getApplicationForViewer(ctx, app.ID, 0)
}

// Reject 审核驳回：备注必填；pending → rejected（条件更新判赢）。
func (s *ResearchApplicationService) Reject(ctx context.Context, adminID int64, applicationID int64, notes string) (*ResearchApplication, error) {
	trimmed := strings.TrimSpace(notes)
	if trimmed == "" {
		return nil, ErrResearchNotesRequired
	}
	app, err := s.getApplicationForViewer(ctx, applicationID, 0)
	if err != nil {
		return nil, err
	}
	if app.Status != ResearchApplicationStatusPending {
		return nil, ErrResearchApplicationNotPending
	}
	won, err := s.appRepo.MarkRejected(ctx, app.ID, adminID, trimmed, time.Now())
	if err != nil {
		return nil, err
	}
	if !won {
		return nil, ErrResearchApplicationNotPending
	}
	return s.getApplicationForViewer(ctx, app.ID, 0)
}

// ─────────────────────────── 内部辅助 ───────────────────────────

// resolveAttachmentMetas 校验每个附件：存在、属于本人、未被其他申请绑定
func (s *ResearchApplicationService) resolveAttachmentMetas(ctx context.Context, userID int64, attachmentIDs []string) ([]ResearchAttachmentMeta, error) {
	metas := make([]ResearchAttachmentMeta, 0, len(attachmentIDs))
	for _, id := range attachmentIDs {
		upload, err := s.uploadRepo.GetByID(ctx, id)
		if err != nil {
			return nil, ErrResearchAttachmentNotFound
		}
		if upload.UserID != userID {
			// 不泄露他人附件存在性，统一按"附件无效"处理
			return nil, ErrResearchAttachmentInvalid
		}
		if upload.ApplicationID != nil {
			return nil, infraerrors.BadRequest("RESEARCH_ATTACHMENT_ALREADY_BOUND", "attachment has already been used by another application")
		}
		metas = append(metas, ResearchAttachmentMeta{
			ID:   upload.ID,
			Name: upload.OriginalName,
			Mime: upload.Mime,
			Size: upload.Size,
		})
	}
	return metas, nil
}

// getApplicationForViewer userID>0 时校验归属（本人之外一律 404，不泄露存在性）
func (s *ResearchApplicationService) getApplicationForViewer(ctx context.Context, applicationID, userID int64) (*ResearchApplication, error) {
	app, err := s.appRepo.GetByID(ctx, applicationID)
	if err != nil {
		return nil, ErrResearchApplicationNotFound
	}
	if userID > 0 && app.UserID != userID {
		return nil, ErrResearchApplicationNotFound
	}
	return app, nil
}

// resolveAttachmentDownload 附件下载路径解析：
// 物理路径只来自 DB storage_path + DATA_DIR（禁止用户输入参与拼接），
// 并做双重防御：附件必须已绑定到该申请，且解析后路径不得逃出存储根目录。
func (s *ResearchApplicationService) resolveAttachmentDownload(ctx context.Context, app *ResearchApplication, attachmentID string) (*ResearchAttachmentDownload, error) {
	upload, err := s.uploadRepo.GetByID(ctx, attachmentID)
	if err != nil {
		return nil, ErrResearchAttachmentNotFound
	}
	if upload.ApplicationID == nil || *upload.ApplicationID != app.ID || upload.UserID != app.UserID {
		return nil, ErrResearchAttachmentNotFound
	}
	absPath := filepath.Join(s.dataDir, filepath.FromSlash(upload.StoragePath))
	rootAbs, err := filepath.Abs(filepath.Join(s.dataDir, researchStorageSubdir))
	if err != nil {
		return nil, ErrResearchAttachmentNotFound
	}
	resolvedAbs, err := filepath.Abs(absPath)
	if err != nil || !strings.HasPrefix(resolvedAbs, rootAbs+string(filepath.Separator)) {
		return nil, ErrResearchAttachmentNotFound
	}
	if _, err := os.Stat(resolvedAbs); err != nil {
		return nil, ErrResearchAttachmentNotFound
	}
	return &ResearchAttachmentDownload{
		AbsPath: resolvedAbs,
		Meta: ResearchAttachmentMeta{
			ID:   upload.ID,
			Name: upload.OriginalName,
			Mime: upload.Mime,
			Size: upload.Size,
		},
	}, nil
}

// issueResearchRedeemCode 生成并发放余额兑换码：
// 码值 "SCI-"+申请ID短串+"-"+6位随机，value=amount，
// notes="research-discount application #<id>"，随后静默到账。
func (s *ResearchApplicationService) issueResearchRedeemCode(ctx context.Context, userID, applicationID int64, amount float64) (*RedeemCode, error) {
	if s.redeem == nil {
		return nil, ErrResearchRedeemIssueFailed
	}
	codeValue, err := newResearchRedeemCodeValue(applicationID)
	if err != nil {
		return nil, ErrResearchRedeemIssueFailed
	}
	code := &RedeemCode{
		Code:   codeValue,
		Type:   RedeemTypeBalance,
		Value:  amount,
		Status: StatusUnused,
		Notes:  fmt.Sprintf("research-discount application #%d", applicationID),
	}
	if err := s.redeem.CreateCode(ctx, code); err != nil {
		return nil, errors.Join(ErrResearchRedeemIssueFailed, err)
	}
	redeemed, err := s.redeem.RedeemForAdminFulfillment(ctx, userID, code.Code)
	if err != nil {
		return nil, errors.Join(ErrResearchRedeemIssueFailed, err)
	}
	return redeemed, nil
}

// applicationUser 回填申请人信息（失败不阻塞列表展示）
func (s *ResearchApplicationService) applicationUser(ctx context.Context, userID int64) *ResearchApplicationUser {
	if s.userReader == nil {
		return nil
	}
	u, err := s.userReader.GetByID(ctx, userID)
	if err != nil || u == nil {
		return nil
	}
	return &ResearchApplicationUser{ID: u.ID, Email: u.Email, Username: u.Username}
}

func normalizeResearchMime(mime string) string {
	// "image/png; charset=binary" → "image/png"
	if idx := strings.Index(mime, ";"); idx >= 0 {
		mime = mime[:idx]
	}
	return strings.ToLower(strings.TrimSpace(mime))
}

func sniffPrefix(data []byte) []byte {
	if len(data) > 512 {
		return data[:512]
	}
	return data
}

func isValidResearchStatus(status string) bool {
	switch status {
	case ResearchApplicationStatusPending, ResearchApplicationStatusApproved, ResearchApplicationStatusRejected:
		return true
	default:
		return false
	}
}

// newResearchUUID 生成附件 ID（uuid v4 字符串）
func newResearchUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand 失败属于系统级异常，直接 panic 与全局策略一致
		panic(fmt.Errorf("generate research attachment uuid: %w", err))
	}
	// RFC 4122 version 4 / variant 10
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// newResearchRedeemCodeValue 生成兑换码值："SCI-"+申请ID短串（base36 大写）+"-"+6位随机大写hex
func newResearchRedeemCodeValue(applicationID int64) (string, error) {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate research redeem code: %w", err)
	}
	return fmt.Sprintf("SCI-%s-%s", strings.ToUpper(strconvFormatIntBase36(applicationID)), strings.ToUpper(hex.EncodeToString(b))), nil
}

func strconvFormatIntBase36(v int64) string {
	const digits = "0123456789abcdefghijklmnopqrstuvwxyz"
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [16]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = digits[v%36]
		v /= 36
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

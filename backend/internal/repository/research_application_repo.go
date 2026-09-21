package repository

import (
	"context"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/researchapplication"
	"github.com/Wei-Shaw/sub2api/ent/researchattachmentupload"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// researchApplicationRepository 科研优惠申请仓储（实现 service.ResearchApplicationRepository）
type researchApplicationRepository struct {
	client *dbent.Client
}

// NewResearchApplicationRepository 创建科研优惠申请仓储
func NewResearchApplicationRepository(client *dbent.Client) service.ResearchApplicationRepository {
	return &researchApplicationRepository{client: client}
}

// researchAttachmentUploadRepository 科研附件上传记录仓储（实现 service.ResearchAttachmentUploadRepository）
type researchAttachmentUploadRepository struct {
	client *dbent.Client
}

// NewResearchAttachmentUploadRepository 创建科研附件上传记录仓储
func NewResearchAttachmentUploadRepository(client *dbent.Client) service.ResearchAttachmentUploadRepository {
	return &researchAttachmentUploadRepository{client: client}
}

// ─────────────────────────── 申请仓储 ───────────────────────────

// CreateWithAttachmentBinding 单事务内创建申请并条件绑定附件：
// UPDATE research_attachment_uploads SET application_id=?
// WHERE id IN (...) AND user_id=? AND application_id IS NULL。
// 影响行数与附件数不一致（不存在/非本人/已被绑定）则整体回滚。
func (r *researchApplicationRepository) CreateWithAttachmentBinding(ctx context.Context, app *service.ResearchApplication, attachmentIDs []string) error {
	if len(attachmentIDs) == 0 {
		// 无附件：直接创建
		created, err := r.client.ResearchApplication.Create().
			SetUserID(app.UserID).
			SetDescription(app.Description).
			SetStatus(app.Status).
			Save(ctx)
		if err != nil {
			return err
		}
		applyResearchApplicationEntity(app, created)
		return nil
	}

	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	created, err := tx.ResearchApplication.Create().
		SetUserID(app.UserID).
		SetDescription(app.Description).
		SetStatus(app.Status).
		SetAttachments(researchAttachmentMetasToEnt(app.Attachments)).
		Save(ctx)
	if err != nil {
		return err
	}

	affected, err := tx.ResearchAttachmentUpload.Update().
		Where(
			researchattachmentupload.IDIn(attachmentIDs...),
			researchattachmentupload.UserIDEQ(app.UserID),
			researchattachmentupload.ApplicationIDIsNil(),
		).
		SetApplicationID(created.ID).
		Save(ctx)
	if err != nil {
		return err
	}
	if affected != len(attachmentIDs) {
		// 任一附件不存在/不属于本人/已被其他申请绑定：整体失败
		return service.ErrResearchAttachmentInvalid
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	applyResearchApplicationEntity(app, created)
	return nil
}

func (r *researchApplicationRepository) GetByID(ctx context.Context, id int64) (*service.ResearchApplication, error) {
	m, err := r.client.ResearchApplication.Query().
		Where(researchapplication.IDEQ(id)).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrResearchApplicationNotFound, nil)
	}
	return researchApplicationEntityToService(m), nil
}

func (r *researchApplicationRepository) Delete(ctx context.Context, id int64) error {
	return r.client.ResearchApplication.DeleteOneID(id).Exec(ctx)
}

func (r *researchApplicationRepository) ListByUser(ctx context.Context, userID int64) ([]service.ResearchApplication, error) {
	rows, err := r.client.ResearchApplication.Query().
		Where(researchapplication.UserIDEQ(userID)).
		Order(dbent.Desc(researchapplication.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]service.ResearchApplication, 0, len(rows))
	for _, m := range rows {
		result = append(result, *researchApplicationEntityToService(m))
	}
	return result, nil
}

func (r *researchApplicationRepository) ListByStatus(ctx context.Context, params pagination.PaginationParams, status string) ([]service.ResearchApplication, int64, error) {
	query := r.client.ResearchApplication.Query()
	if status != "" {
		query = query.Where(researchapplication.StatusEQ(status))
	}

	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	rows, err := query.
		Order(dbent.Desc(researchapplication.FieldCreatedAt)).
		Offset(params.Offset()).
		Limit(params.Limit()).
		All(ctx)
	if err != nil {
		return nil, 0, err
	}

	result := make([]service.ResearchApplication, 0, len(rows))
	for _, m := range rows {
		result = append(result, *researchApplicationEntityToService(m))
	}
	return result, int64(total), nil
}

// ClaimForApproval 审核判赢：条件更新 WHERE id=? AND status='pending'。
// 命中即写入审核字段并置为 approved（并发/重复 approve 只有一方成功）。
func (r *researchApplicationRepository) ClaimForApproval(ctx context.Context, id int64, reviewerID int64, notes string, rewardAmount float64, reviewedAt time.Time) (bool, error) {
	builder := r.client.ResearchApplication.Update().
		Where(
			researchapplication.IDEQ(id),
			researchapplication.StatusEQ(service.ResearchApplicationStatusPending),
		).
		SetStatus(service.ResearchApplicationStatusApproved).
		SetReviewerID(reviewerID).
		SetRewardAmount(rewardAmount).
		SetReviewedAt(reviewedAt)
	if notes != "" {
		builder = builder.SetReviewNotes(notes)
	}
	affected, err := builder.Save(ctx)
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (r *researchApplicationRepository) SetIssuedRedeemCodeID(ctx context.Context, id int64, redeemCodeID int64) error {
	_, err := r.client.ResearchApplication.Update().
		Where(researchapplication.IDEQ(id)).
		SetIssuedRedeemCodeID(redeemCodeID).
		Save(ctx)
	return err
}

func (r *researchApplicationRepository) RevertToPending(ctx context.Context, id int64) error {
	affected, err := r.client.ResearchApplication.Update().
		Where(
			researchapplication.IDEQ(id),
			researchapplication.StatusEQ(service.ResearchApplicationStatusApproved),
		).
		ClearReviewerID().
		ClearReviewNotes().
		ClearRewardAmount().
		ClearIssuedRedeemCodeID().
		ClearReviewedAt().
		SetStatus(service.ResearchApplicationStatusPending).
		Save(ctx)
	if err != nil {
		return err
	}
	_ = affected
	return nil
}

// MarkRejected 驳回判赢：条件更新 WHERE id=? AND status='pending' → rejected
func (r *researchApplicationRepository) MarkRejected(ctx context.Context, id int64, reviewerID int64, notes string, reviewedAt time.Time) (bool, error) {
	affected, err := r.client.ResearchApplication.Update().
		Where(
			researchapplication.IDEQ(id),
			researchapplication.StatusEQ(service.ResearchApplicationStatusPending),
		).
		SetStatus(service.ResearchApplicationStatusRejected).
		SetReviewerID(reviewerID).
		SetReviewNotes(notes).
		SetReviewedAt(reviewedAt).
		Save(ctx)
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// ─────────────────────────── 附件上传记录仓储 ───────────────────────────

func (r *researchAttachmentUploadRepository) Create(ctx context.Context, up *service.ResearchAttachmentUpload) error {
	created, err := r.client.ResearchAttachmentUpload.Create().
		SetID(up.ID).
		SetUserID(up.UserID).
		SetOriginalName(up.OriginalName).
		SetMime(up.Mime).
		SetSize(up.Size).
		SetStoragePath(up.StoragePath).
		Save(ctx)
	if err != nil {
		return err
	}
	applyResearchAttachmentUploadEntity(up, created)
	return nil
}

func (r *researchAttachmentUploadRepository) GetByID(ctx context.Context, id string) (*service.ResearchAttachmentUpload, error) {
	m, err := r.client.ResearchAttachmentUpload.Query().
		Where(researchattachmentupload.IDEQ(id)).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrResearchAttachmentNotFound, nil)
	}
	return researchAttachmentUploadEntityToService(m), nil
}

// ─────────────────────────── ent ↔ service 映射 ───────────────────────────

func researchAttachmentMetasToEnt(metas []service.ResearchAttachmentMeta) []domain.ResearchAttachmentMeta {
	out := make([]domain.ResearchAttachmentMeta, 0, len(metas))
	for _, m := range metas {
		out = append(out, domain.ResearchAttachmentMeta{ID: m.ID, Name: m.Name, Mime: m.Mime, Size: m.Size})
	}
	return out
}

func researchAttachmentMetasFromEnt(metas []domain.ResearchAttachmentMeta) []service.ResearchAttachmentMeta {
	out := make([]service.ResearchAttachmentMeta, 0, len(metas))
	for _, m := range metas {
		out = append(out, service.ResearchAttachmentMeta{ID: m.ID, Name: m.Name, Mime: m.Mime, Size: m.Size})
	}
	return out
}

func applyResearchApplicationEntity(app *service.ResearchApplication, m *dbent.ResearchApplication) {
	app.ID = m.ID
	app.CreatedAt = m.CreatedAt
	app.UpdatedAt = m.UpdatedAt
	app.UserID = m.UserID
	app.Description = m.Description
	app.Status = m.Status
	app.ReviewerID = m.ReviewerID
	app.ReviewNotes = m.ReviewNotes
	app.RewardAmount = m.RewardAmount
	app.IssuedRedeemCodeID = m.IssuedRedeemCodeID
	app.ReviewedAt = m.ReviewedAt
	app.Attachments = researchAttachmentMetasFromEnt(m.Attachments)
}

func researchApplicationEntityToService(m *dbent.ResearchApplication) *service.ResearchApplication {
	app := &service.ResearchApplication{}
	applyResearchApplicationEntity(app, m)
	return app
}

func applyResearchAttachmentUploadEntity(up *service.ResearchAttachmentUpload, m *dbent.ResearchAttachmentUpload) {
	up.ID = m.ID
	up.UserID = m.UserID
	up.OriginalName = m.OriginalName
	up.Mime = m.Mime
	up.Size = m.Size
	up.StoragePath = m.StoragePath
	up.CreatedAt = m.CreatedAt
	up.ApplicationID = m.ApplicationID
}

func researchAttachmentUploadEntityToService(m *dbent.ResearchAttachmentUpload) *service.ResearchAttachmentUpload {
	up := &service.ResearchAttachmentUpload{}
	applyResearchAttachmentUploadEntity(up, m)
	return up
}

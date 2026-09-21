//go:build unit

package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

// ─────────────────────────── stub 定义（参考 web_chat_service_test.go 风格） ───────────────────────────

// researchAppRepoStub 科研申请仓储桩
type researchAppRepoStub struct {
	apps        map[int64]*ResearchApplication
	nextID      int64
	bindErr     error
	claimLose   bool // 模拟并发判赢失败：pending 但条件更新未命中
	revertCalls int
}

func newResearchAppRepoStub() *researchAppRepoStub {
	return &researchAppRepoStub{apps: map[int64]*ResearchApplication{}, nextID: 100}
}

func (s *researchAppRepoStub) CreateWithAttachmentBinding(ctx context.Context, app *ResearchApplication, attachmentIDs []string) error {
	if s.bindErr != nil {
		return s.bindErr
	}
	s.nextID++
	app.ID = s.nextID
	app.Status = ResearchApplicationStatusPending
	now := time.Now()
	app.CreatedAt = now
	app.UpdatedAt = now
	s.apps[app.ID] = app
	return nil
}

func (s *researchAppRepoStub) GetByID(ctx context.Context, id int64) (*ResearchApplication, error) {
	if app, ok := s.apps[id]; ok {
		return app, nil
	}
	return nil, ErrResearchApplicationNotFound
}

func (s *researchAppRepoStub) Delete(ctx context.Context, id int64) error {
	delete(s.apps, id)
	return nil
}

func (s *researchAppRepoStub) ListByUser(ctx context.Context, userID int64) ([]ResearchApplication, error) {
	return nil, errors.New("unexpected ListByUser call")
}

func (s *researchAppRepoStub) ListByStatus(ctx context.Context, params pagination.PaginationParams, status string) ([]ResearchApplication, int64, error) {
	return nil, 0, errors.New("unexpected ListByStatus call")
}

func (s *researchAppRepoStub) ClaimForApproval(ctx context.Context, id int64, reviewerID int64, notes string, rewardAmount float64, reviewedAt time.Time) (bool, error) {
	if s.claimLose {
		return false, nil
	}
	app, ok := s.apps[id]
	if !ok || app.Status != ResearchApplicationStatusPending {
		return false, nil
	}
	app.Status = ResearchApplicationStatusApproved
	app.ReviewerID = &reviewerID
	app.RewardAmount = &rewardAmount
	app.ReviewedAt = &reviewedAt
	if notes != "" {
		app.ReviewNotes = &notes
	}
	return true, nil
}

func (s *researchAppRepoStub) SetIssuedRedeemCodeID(ctx context.Context, id int64, redeemCodeID int64) error {
	if app, ok := s.apps[id]; ok {
		app.IssuedRedeemCodeID = &redeemCodeID
	}
	return nil
}

func (s *researchAppRepoStub) RevertToPending(ctx context.Context, id int64) error {
	s.revertCalls++
	if app, ok := s.apps[id]; ok {
		app.Status = ResearchApplicationStatusPending
		app.ReviewerID = nil
		app.ReviewNotes = nil
		app.RewardAmount = nil
		app.IssuedRedeemCodeID = nil
		app.ReviewedAt = nil
	}
	return nil
}

func (s *researchAppRepoStub) MarkRejected(ctx context.Context, id int64, reviewerID int64, notes string, reviewedAt time.Time) (bool, error) {
	app, ok := s.apps[id]
	if !ok || app.Status != ResearchApplicationStatusPending {
		return false, nil
	}
	app.Status = ResearchApplicationStatusRejected
	app.ReviewerID = &reviewerID
	app.ReviewNotes = &notes
	app.ReviewedAt = &reviewedAt
	return true, nil
}

// researchUploadRepoStub 附件上传记录仓储桩
type researchUploadRepoStub struct {
	uploads map[string]*ResearchAttachmentUpload
}

func (s *researchUploadRepoStub) Create(ctx context.Context, up *ResearchAttachmentUpload) error {
	s.uploads[up.ID] = up
	return nil
}

func (s *researchUploadRepoStub) GetByID(ctx context.Context, id string) (*ResearchAttachmentUpload, error) {
	if up, ok := s.uploads[id]; ok {
		return up, nil
	}
	return nil, ErrResearchAttachmentNotFound
}

// researchRedeemIssuerStub 兑换发放桩
type researchRedeemIssuerStub struct {
	created       *RedeemCode
	createErr     error
	redeemedCode  *RedeemCode
	redeemErr     error
	redeemUserID  int64
	redeemCodeVal string
	redeemCalls   int
}

func (s *researchRedeemIssuerStub) CreateCode(ctx context.Context, code *RedeemCode) error {
	if s.createErr != nil {
		return s.createErr
	}
	code.ID = 777
	s.created = code
	return nil
}

func (s *researchRedeemIssuerStub) RedeemForAdminFulfillment(ctx context.Context, userID int64, code string) (*RedeemCode, error) {
	s.redeemCalls++
	s.redeemUserID = userID
	s.redeemCodeVal = code
	if s.redeemErr != nil {
		return nil, s.redeemErr
	}
	return s.redeemedCode, nil
}

// researchUserReaderStub 用户读取桩
type researchUserReaderStub struct {
	users map[int64]*User
}

func (s *researchUserReaderStub) GetByID(ctx context.Context, id int64) (*User, error) {
	if u, ok := s.users[id]; ok {
		return u, nil
	}
	return nil, ErrUserNotFound
}

// newResearchTestService 组装被测服务（dataDir 用临时目录）
func newResearchTestService(t *testing.T) (*ResearchApplicationService, *researchAppRepoStub, *researchUploadRepoStub, *researchRedeemIssuerStub) {
	t.Helper()
	appRepo := newResearchAppRepoStub()
	uploadRepo := &researchUploadRepoStub{uploads: map[string]*ResearchAttachmentUpload{}}
	redeem := &researchRedeemIssuerStub{
		redeemedCode: &RedeemCode{ID: 777, Code: "SCI-64-ABCDEF", Type: RedeemTypeBalance, Status: StatusUsed},
	}
	svc := NewResearchApplicationService(appRepo, uploadRepo, redeem, &researchUserReaderStub{}, t.TempDir())
	return svc, appRepo, uploadRepo, redeem
}

func researchPendingApp(userID int64) *ResearchApplication {
	return &ResearchApplication{
		UserID:      userID,
		Description: "某高校在读博士",
		Status:      ResearchApplicationStatusPending,
	}
}

// ─────────────────────────── 提交校验 ───────────────────────────

func TestResearchCreateApplicationValidation(t *testing.T) {
	svc, _, uploadRepo, _ := newResearchTestService(t)
	ctx := context.Background()

	// 描述必填
	_, err := svc.CreateApplication(ctx, 1, ResearchCreateApplicationInput{Description: "   "})
	require.ErrorIs(t, err, ErrResearchDescriptionRequired)

	// 描述 ≤2000 字（按 rune 计）
	_, err = svc.CreateApplication(ctx, 1, ResearchCreateApplicationInput{Description: strings.Repeat("研", 2001)})
	require.ErrorIs(t, err, ErrResearchDescriptionTooLong)

	ok, err := svc.CreateApplication(ctx, 1, ResearchCreateApplicationInput{Description: strings.Repeat("研", 2000)})
	require.NoError(t, err)
	require.Equal(t, ResearchApplicationStatusPending, ok.Status)

	// 附件数 ≤5
	ids := make([]string, 6)
	for i := range ids {
		ids[i] = "att-" + strings.Repeat("a", i+1)
	}
	_, err = svc.CreateApplication(ctx, 1, ResearchCreateApplicationInput{Description: "科研", AttachmentIDs: ids})
	require.Error(t, err)
	require.Contains(t, err.Error(), "RESEARCH_ATTACHMENT_TOO_MANY")

	// 附件必须存在
	_, err = svc.CreateApplication(ctx, 1, ResearchCreateApplicationInput{Description: "科研", AttachmentIDs: []string{"nope"}})
	require.ErrorIs(t, err, ErrResearchAttachmentNotFound)

	// 附件必须属于本人
	uploadRepo.uploads["u1"] = &ResearchAttachmentUpload{ID: "u1", UserID: 2, OriginalName: "a.png", Mime: "image/png", Size: 10, StoragePath: "uploads/research/u1.png"}
	_, err = svc.CreateApplication(ctx, 1, ResearchCreateApplicationInput{Description: "科研", AttachmentIDs: []string{"u1"}})
	require.ErrorIs(t, err, ErrResearchAttachmentInvalid)

	// 附件不能已被其他申请绑定
	bound := int64(42)
	uploadRepo.uploads["u1"].UserID = 1
	uploadRepo.uploads["u1"].ApplicationID = &bound
	_, err = svc.CreateApplication(ctx, 1, ResearchCreateApplicationInput{Description: "科研", AttachmentIDs: []string{"u1"}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "RESEARCH_ATTACHMENT_ALREADY_BOUND")

	// 合法提交：申请创建成功且元数据冗余了附件信息
	uploadRepo.uploads["u1"].ApplicationID = nil
	app, err := svc.CreateApplication(ctx, 1, ResearchCreateApplicationInput{Description: "科研", AttachmentIDs: []string{"u1"}})
	require.NoError(t, err)
	require.Equal(t, ResearchApplicationStatusPending, app.Status)
	require.Len(t, app.Attachments, 1)
	require.Equal(t, "u1", app.Attachments[0].ID)
	require.Equal(t, "image/png", app.Attachments[0].Mime)
}

// ─────────────────────────── approve 状态机 ───────────────────────────

func TestResearchApproveStateAndIssue(t *testing.T) {
	svc, appRepo, _, redeem := newResearchTestService(t)
	ctx := context.Background()

	created := researchPendingApp(9)
	created.ID = 1
	appRepo.apps[1] = created

	// 金额校验
	_, err := svc.Approve(ctx, 5, 1, ResearchApproveInput{Amount: 0})
	require.ErrorIs(t, err, ErrResearchAmountInvalid)
	_, err = svc.Approve(ctx, 5, 1, ResearchApproveInput{Amount: 10001})
	require.ErrorIs(t, err, ErrResearchAmountInvalid)

	// 成功：发放参数与申请字段
	got, err := svc.Approve(ctx, 5, 1, ResearchApproveInput{Amount: 88.5, Notes: "学生证已核验"})
	require.NoError(t, err)

	require.NotNil(t, redeem.created, "CreateCode should be called")
	require.True(t, strings.HasPrefix(redeem.created.Code, "SCI-"), "code value = %s", redeem.created.Code)
	require.Equal(t, RedeemTypeBalance, redeem.created.Type)
	require.Equal(t, 88.5, redeem.created.Value)
	require.Equal(t, "research-discount application #1", redeem.created.Notes)

	require.Equal(t, 1, redeem.redeemCalls, "RedeemForAdminFulfillment should be called once")
	require.Equal(t, int64(9), redeem.redeemUserID, "redeem should target the applicant")
	require.Equal(t, redeem.created.Code, redeem.redeemCodeVal)

	require.Equal(t, ResearchApplicationStatusApproved, got.Status)
	require.NotNil(t, got.ReviewerID)
	require.Equal(t, int64(5), *got.ReviewerID)
	require.NotNil(t, got.RewardAmount)
	require.Equal(t, 88.5, *got.RewardAmount)
	require.NotNil(t, got.IssuedRedeemCodeID)
	require.Equal(t, int64(777), *got.IssuedRedeemCodeID)
	require.NotNil(t, got.ReviewedAt)
	require.NotNil(t, got.ReviewNotes)
	require.Equal(t, "学生证已核验", *got.ReviewNotes)

	// 重复 approve：非 pending → 409/400（NOT PENDING）
	_, err = svc.Approve(ctx, 5, 1, ResearchApproveInput{Amount: 1})
	require.ErrorIs(t, err, ErrResearchApplicationNotPending)
}

func TestResearchApproveIssueFailureKeepsPending(t *testing.T) {
	svc, appRepo, _, redeem := newResearchTestService(t)
	ctx := context.Background()

	created := researchPendingApp(9)
	created.ID = 1
	appRepo.apps[1] = created

	// CreateCode 失败 → 状态回滚 pending，返回错误
	redeem.createErr = errors.New("db down")
	_, err := svc.Approve(ctx, 5, 1, ResearchApproveInput{Amount: 10})
	require.ErrorIs(t, err, ErrResearchRedeemIssueFailed)
	require.Equal(t, ResearchApplicationStatusPending, appRepo.apps[1].Status)
	require.Nil(t, appRepo.apps[1].RewardAmount)
	require.Equal(t, 1, appRepo.revertCalls)
	require.Zero(t, redeem.redeemCalls, "CreateCode 失败后不应再触发兑换")

	// RedeemForAdminFulfillment 失败 → 同样回滚
	redeem.createErr = nil
	redeem.redeemErr = errors.New("redeem failed")
	_, err = svc.Approve(ctx, 5, 1, ResearchApproveInput{Amount: 10})
	require.ErrorIs(t, err, ErrResearchRedeemIssueFailed)
	require.Equal(t, ResearchApplicationStatusPending, appRepo.apps[1].Status)
	require.Equal(t, 2, appRepo.revertCalls)

	// 发放失败后可重试成功
	redeem.redeemErr = nil
	got, err := svc.Approve(ctx, 5, 1, ResearchApproveInput{Amount: 10})
	require.NoError(t, err)
	require.Equal(t, ResearchApplicationStatusApproved, got.Status)
}

func TestResearchApproveConcurrentJudge(t *testing.T) {
	svc, appRepo, _, redeem := newResearchTestService(t)
	ctx := context.Background()

	created := researchPendingApp(9)
	created.ID = 1
	appRepo.apps[1] = created

	// 模拟并发：approve 读取时状态仍 pending，但条件更新（WHERE status='pending'）
	// 未命中（已被其他管理员判赢）→ 服务必须按 NOT PENDING 处理且不发放兑换码
	appRepo.claimLose = true
	_, err := svc.Approve(ctx, 6, 1, ResearchApproveInput{Amount: 10})
	require.ErrorIs(t, err, ErrResearchApplicationNotPending)
	require.Nil(t, redeem.created, "判赢失败不应发放兑换码")
	require.Zero(t, redeem.redeemCalls)
	require.Equal(t, ResearchApplicationStatusPending, appRepo.apps[1].Status)
}

// ─────────────────────────── reject ───────────────────────────

func TestResearchRejectRequiresNotes(t *testing.T) {
	svc, appRepo, _, _ := newResearchTestService(t)
	ctx := context.Background()

	created := researchPendingApp(9)
	created.ID = 1
	appRepo.apps[1] = created

	// 备注必填
	_, err := svc.Reject(ctx, 5, 1, "   ")
	require.ErrorIs(t, err, ErrResearchNotesRequired)

	// 非 pending → 拒绝
	appRepo.apps[1].Status = ResearchApplicationStatusApproved
	_, err = svc.Reject(ctx, 5, 1, "材料不足")
	require.ErrorIs(t, err, ErrResearchApplicationNotPending)

	// 成功驳回
	appRepo.apps[1].Status = ResearchApplicationStatusPending
	got, err := svc.Reject(ctx, 5, 1, "材料不足")
	require.NoError(t, err)
	require.Equal(t, ResearchApplicationStatusRejected, got.Status)
	require.NotNil(t, got.ReviewNotes)
	require.Equal(t, "材料不足", *got.ReviewNotes)
	require.NotNil(t, got.ReviewerID)
	require.Equal(t, int64(5), *got.ReviewerID)
	require.NotNil(t, got.ReviewedAt)

	// 已驳回后再次驳回 → NOT PENDING
	_, err = svc.Reject(ctx, 5, 1, "再驳")
	require.ErrorIs(t, err, ErrResearchApplicationNotPending)
}

// ─────────────────────────── 附件保存与下载权限 ───────────────────────────

func TestResearchSaveAttachmentValidation(t *testing.T) {
	svc, _, uploadRepo, _ := newResearchTestService(t)
	ctx := context.Background()

	// 类型白名单
	_, err := svc.SaveAttachment(ctx, 1, "a.gif", "image/gif", []byte("GIF89a"))
	require.ErrorIs(t, err, ErrResearchAttachmentType)

	// 大小上限（5MB）
	_, err = svc.SaveAttachment(ctx, 1, "a.png", "image/png", make([]byte, 5<<20+1))
	require.ErrorIs(t, err, ErrResearchAttachmentTooLarge)

	// 空文件
	_, err = svc.SaveAttachment(ctx, 1, "a.png", "image/png", nil)
	require.ErrorIs(t, err, ErrResearchAttachmentTooLarge)

	// 内容与声明类型不符（魔数嗅探）
	_, err = svc.SaveAttachment(ctx, 1, "evil.png", "image/png", []byte("<script>alert(1)</script>"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not match")

	// 合法 PNG：落盘 + 记录
	png := append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, make([]byte, 100)...)
	meta, err := svc.SaveAttachment(ctx, 1, "证明.png", "image/png", png)
	require.NoError(t, err)
	require.NotEmpty(t, meta.ID)
	require.Equal(t, "image/png", meta.Mime)
	require.Equal(t, int64(len(png)), meta.Size)
	stored, err := uploadRepo.GetByID(ctx, meta.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), stored.UserID)
	require.True(t, strings.HasPrefix(stored.StoragePath, "uploads/research/"))
	require.True(t, strings.HasSuffix(stored.StoragePath, ".png"))

	// 下载权限：他人申请不可见
	app := researchPendingApp(1)
	app.ID = 7
	appRepo2 := newResearchAppRepoStub()
	appRepo2.apps[7] = app
	uploadRepo.uploads[meta.ID].ApplicationID = &app.ID
	svc2 := NewResearchApplicationService(appRepo2, uploadRepo, nil, &researchUserReaderStub{}, svc.dataDir)

	_, err = svc2.GetAttachmentForUserDownload(ctx, 2, 7, meta.ID)
	require.ErrorIs(t, err, ErrResearchApplicationNotFound)

	dl, err := svc2.GetAttachmentForUserDownload(ctx, 1, 7, meta.ID)
	require.NoError(t, err)
	require.Equal(t, meta.ID, dl.Meta.ID)
	require.True(t, strings.HasPrefix(dl.AbsPath, svc.dataDir))
}

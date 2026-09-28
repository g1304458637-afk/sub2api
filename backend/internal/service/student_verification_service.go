package service

import (
	"context"
	"crypto/subtle"
	"fmt"
	"html"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/campus"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// StudentVerificationConfigReader 学生认证权益配置读取接口（由 *SettingService 实现）。
type StudentVerificationConfigReader interface {
	// IsStudentVerificationEnabled 学生认证功能开关（fail-closed）。
	IsStudentVerificationEnabled(ctx context.Context) bool
	// GetStudentBenefitGroupID 学生权益目标分组（<= 0 视为未配置）。
	GetStudentBenefitGroupID(ctx context.Context) int64
	// GetStudentBenefitPlanID 学生权益展示用套餐身份（0 = 未指定）。
	GetStudentBenefitPlanID(ctx context.Context) int64
	// GetStudentBenefitValidityDays 学生权益时长（天；<= 0 视为未配置）。
	GetStudentBenefitValidityDays(ctx context.Context) int
	// GetStudentBenefitCode 学生权益身份码（空 = 按品牌默认派生）。
	GetStudentBenefitCode(ctx context.Context) string
}

// compile-time 接口满足性检查：*SettingService 必须实现 StudentVerificationConfigReader。
var _ StudentVerificationConfigReader = (*SettingService)(nil)

const (
	// studentVerificationCodeKeyPrefix OTP 的 Redis key 前缀（按用户隔离，
	// 不按邮箱 —— 既防枚举，又保证验证码只属于发起认证的账号）。
	studentVerificationCodeKeyPrefix = "student-verification:user:"

	// StudentBenefitIdentityTypeHubuEmail 权益身份类型（grant.identity_type）。
	StudentBenefitIdentityTypeHubuEmail = "hubu_email"

	// DefaultStudentBenefitCodeFallback 品牌默认权益码无法派生时的兜底。
	DefaultStudentBenefitCodeFallback = "HUBU_STUDENT_WELCOME"
)

var (
	ErrStudentVerificationDisabled = infraerrors.Forbidden("STUDENT_VERIFICATION_DISABLED", "student verification is not enabled")
	ErrStudentEmailInvalid         = infraerrors.BadRequest("STUDENT_EMAIL_INVALID", "use a valid student email address of this campus")
	ErrStudentBenefitMisconfigured = infraerrors.InternalServer("STUDENT_BENEFIT_MISCONFIGURED", "student benefit is not configured, please contact the administrator")
)

// StudentVerificationService 湖北大学学生邮箱认证服务（首版 provider：HUBU_EMAIL）。
//
// 事实边界：本服务只证明「用户控制一个有效的校园学生邮箱」，不证明学籍状态；
// 内部状态为 HUBU_EMAIL_VERIFIED（student_verifications.provider=HUBU_EMAIL,
// status=verified），对外文案为「校园身份认证 / 学生邮箱认证」。
//
// 认证 ≠ 发放：验证通过后统一走 SubscriptionGrantService 发放学生权益
// （StudentVerification → Grant Service → Subscription Service），未来切换
// SSO provider 时只替换验证实现，不重写发放与订阅逻辑。
//
// OTP 安全：不复用明文验证码；复用 EmailCache 的 Redis 存储
// （TTL 15min / 重发冷却 1min / 验证后 GETDEL 单次失效 / 常量时间比较），
// 用户级发送频控复用 notify_code_user_rate（5 次/窗口），IP 级限流由路由层
// RateLimiter 承担。
type StudentVerificationService struct {
	entClient        *dbent.Client
	verificationRepo StudentVerificationRepository
	grantService     *SubscriptionGrantService
	rewardService    *RewardGrantService
	settingReader    StudentVerificationConfigReader
	emailService     *EmailService
	brand            campus.Brand

	now func() time.Time
}

func NewStudentVerificationService(
	entClient *dbent.Client,
	verificationRepo StudentVerificationRepository,
	grantService *SubscriptionGrantService,
	rewardService *RewardGrantService,
	settingReader StudentVerificationConfigReader,
	emailService *EmailService,
) *StudentVerificationService {
	return &StudentVerificationService{
		entClient:        entClient,
		verificationRepo: verificationRepo,
		grantService:     grantService,
		rewardService:    rewardService,
		settingReader:    settingReader,
		emailService:     emailService,
		brand:            campus.Current(),
		now:              time.Now,
	}
}

// SetNowFunc 单测注入时钟。
func (s *StudentVerificationService) SetNowFunc(fn func() time.Time) {
	if s != nil && fn != nil {
		s.now = fn
	}
}

// StudentBenefitCode 返回当前部署的学生权益身份码：
// settings 覆盖优先；否则按品牌派生（HUBU → HUBU_STUDENT_WELCOME）。
func (s *StudentVerificationService) StudentBenefitCode(ctx context.Context) string {
	if s.settingReader != nil {
		if configured := strings.TrimSpace(s.settingReader.GetStudentBenefitCode(ctx)); configured != "" {
			return strings.ToUpper(configured)
		}
	}
	if s.brand.ShortName != "" {
		return strings.ToUpper(s.brand.ShortName) + "_STUDENT_WELCOME"
	}
	return DefaultStudentBenefitCodeFallback
}

// StudentEmailDomain 当前部署的学生邮箱域名（HUBU = stu.hubu.edu.cn）。
func (s *StudentVerificationService) StudentEmailDomain() string {
	return strings.ToLower(strings.TrimSpace(s.brand.EducationDomain))
}

// StudentVerificationProvider 当前部署的认证 provider 标识。
func (s *StudentVerificationService) StudentVerificationProvider() string {
	if s.brand.ShortName != "" {
		return strings.ToUpper(s.brand.ShortName) + "_EMAIL"
	}
	return domain.StudentVerificationProviderOther
}

// SendVerificationCode 发送学生邮箱验证码。
//
// 防枚举：无论邮箱是否存在/是否已认证，调用方看到的错误口径一致
// （仅域名错误与限流错误会显式返回；域名校验失败不消耗任何配额）。
func (s *StudentVerificationService) SendVerificationCode(ctx context.Context, userID int64, email string) error {
	if s == nil || s.settingReader == nil || s.emailService == nil || s.emailService.cache == nil || userID <= 0 {
		return ErrServiceUnavailable
	}
	if !s.settingReader.IsStudentVerificationEnabled(ctx) {
		return ErrStudentVerificationDisabled
	}
	normalized, err := s.normalizeStudentEmail(email)
	if err != nil {
		return err
	}

	cache := s.emailService.cache
	cacheKey := s.codeKey(userID)

	rate, err := cache.GetNotifyCodeUserRate(ctx, userID)
	if err != nil {
		return ErrServiceUnavailable
	}
	if rate >= notifyCodeUserRateLimit {
		return ErrNotifyCodeUserRateLimit
	}

	code, err := s.emailService.GenerateVerifyCode()
	if err != nil {
		return fmt.Errorf("generate student verification code: %w", err)
	}
	// 冷却预留先行：SMTP 失败时释放，避免用户被冷却锁死（与教育邮箱口径一致）。
	reserved, err := cache.ReserveVerificationCodeCooldown(ctx, cacheKey, verifyCodeCooldown)
	if err != nil {
		return ErrServiceUnavailable
	}
	if !reserved {
		return ErrVerifyCodeTooFrequent
	}
	if err := s.sendCodeEmail(ctx, normalized, code); err != nil {
		if releaseErr := cache.ReleaseVerificationCodeCooldown(ctx, cacheKey); releaseErr != nil {
			slog.Error("failed to release student verification cooldown", "user_id", userID, "error", releaseErr)
		}
		return err
	}
	now := s.now().UTC()
	if err := cache.SetVerificationCode(ctx, cacheKey, &VerificationCodeData{
		Code:      code,
		Target:    normalized,
		CreatedAt: now,
		ExpiresAt: now.Add(verifyCodeTTL),
	}, verifyCodeTTL); err != nil {
		return ErrServiceUnavailable
	}

	// 用户级频控只在发送成功后递增一次。
	if _, err := cache.IncrNotifyCodeUserRate(ctx, userID, notifyCodeUserRateWindow); err != nil {
		slog.Error("failed to increment student verification rate", "user_id", userID, "error", err)
	}
	return nil
}

// VerifyEmail 校验 OTP 并完成认证 + 权益发放（原子事务）。
//
// 事务内顺序：
//  1. student_verifications 落库（HUBU_EMAIL_VERIFIED 事实）；
//  2. Grant Service 发放学生权益（claim 幂等 + 台账幂等 + RULE 1 冲突转 pending）；
//  3. 回填 verification.benefit_grant_id；
//  4. （可选）余额奖励 GrantStudentVerificationReward（settings 默认关闭）。
//
// 提交后失效订阅缓存。并发/重放：OTP GETDEL 保证单次消费；benefit_claims
// 双唯一约束保证「一人一号一权益」；benefit_code:identity_key 台账唯一键兜底。
func (s *StudentVerificationService) VerifyEmail(ctx context.Context, userID int64, email, code, ip, userAgent string) (*StudentVerificationResult, error) {
	if s == nil || s.settingReader == nil || s.emailService == nil || s.emailService.cache == nil ||
		s.entClient == nil || s.verificationRepo == nil || s.grantService == nil || userID <= 0 {
		return nil, ErrServiceUnavailable
	}
	if !s.settingReader.IsStudentVerificationEnabled(ctx) {
		return nil, ErrStudentVerificationDisabled
	}
	normalized, err := s.normalizeStudentEmail(email)
	if err != nil {
		return nil, err
	}

	// OTP 单次消费（GETDEL）：过期 / 不匹配 / 重复提交都在这里失败。
	cacheKey := s.codeKey(userID)
	data, err := s.emailService.cache.ConsumeVerificationCode(ctx, cacheKey)
	if err != nil {
		return nil, ErrServiceUnavailable
	}
	code = strings.TrimSpace(code)
	if data == nil || data.ExpiresAt.IsZero() || !s.now().Before(data.ExpiresAt) ||
		!strings.EqualFold(data.Target, normalized) ||
		subtle.ConstantTimeCompare([]byte(data.Code), []byte(code)) != 1 {
		return nil, ErrInvalidVerifyCode
	}

	// 权益配置（发放前校验；配置不完整 → 不落认证、不发放，提示管理员修配置）。
	benefitCmd, err := s.buildBenefitCommand(ctx, userID, normalized)
	if err != nil {
		return nil, err
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin student verification transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)

	verification := &StudentVerification{
		UserID:     userID,
		Provider:   s.StudentVerificationProvider(),
		Email:      normalized,
		Status:     domain.StudentVerificationStatusVerified,
		IP:         truncateForColumn(ip, 64),
		UserAgent:  truncateForColumn(userAgent, 255),
		VerifiedAt: s.now().UTC(),
	}
	verificationID, err := s.verificationRepo.Insert(txCtx, verification)
	if err != nil {
		return nil, fmt.Errorf("insert student verification: %w", err)
	}

	execution, err := s.grantService.CreateGrantInTx(txCtx, benefitCmd)
	if err != nil {
		return nil, err
	}
	if err := s.verificationRepo.LinkBenefitGrant(txCtx, verificationID, execution.Grant.ID); err != nil {
		return nil, fmt.Errorf("link verification benefit grant: %w", err)
	}

	// 可选的余额奖励（student_verification_reward_* settings；默认关闭）。
	if s.rewardService != nil {
		if _, rewardErr := s.rewardService.GrantStudentVerificationReward(txCtx, userID, verificationID, nil); rewardErr != nil {
			return nil, fmt.Errorf("grant student verification reward: %w", rewardErr)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit student verification transaction: %w", err)
	}

	if execution.Outcome != nil && execution.Outcome.Granted() {
		s.grantService.InvalidateGrantCaches(ctx, userID, benefitCmd.GroupID)
	}

	return &StudentVerificationResult{
		Verification: verification,
		Grant:        execution.Grant,
		Outcome:      execution.Outcome,
	}, nil
}

// GetStatus 用户侧认证状态（个人资料卡片）。
func (s *StudentVerificationService) GetStatus(ctx context.Context, userID int64) (*StudentVerificationStatus, error) {
	if s == nil || s.verificationRepo == nil || s.grantService == nil {
		return nil, ErrServiceUnavailable
	}
	status := &StudentVerificationStatus{
		Provider:      s.StudentVerificationProvider(),
		EmailDomain:   s.StudentEmailDomain(),
		BenefitCode:   s.StudentBenefitCode(ctx),
		BenefitDays:   s.settingReader.GetStudentBenefitValidityDays(ctx),
		EmailVerified: false,
	}
	verification, err := s.verificationRepo.LatestVerifiedByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load student verification status: %w", err)
	}
	if verification != nil {
		status.EmailVerified = true
		status.Email = verification.Email
		status.VerifiedAt = &verification.VerifiedAt
	}

	grants, err := s.grantService.ListGrantsByUser(ctx, userID, 20)
	if err != nil {
		return nil, fmt.Errorf("load student benefit grants: %w", err)
	}
	benefitCode := s.StudentBenefitCode(ctx)
	for _, grant := range grants {
		if grant.Source != domain.SubscriptionGrantSourceStudentVerification {
			continue
		}
		if grant.BenefitCode == nil || *grant.BenefitCode != benefitCode {
			continue
		}
		status.GrantStatus = grant.Status
		status.GrantExpiresAt = grant.ContributionEnd
		break
	}
	return status, nil
}

// GetVerification 认证记录详情（管理端）。
func (s *StudentVerificationService) GetVerification(ctx context.Context, id int64) (*StudentVerification, error) {
	if s == nil || s.verificationRepo == nil {
		return nil, ErrServiceUnavailable
	}
	return s.verificationRepo.GetByID(ctx, id)
}

// AdminListVerifications 管理端认证记录分页查询。
func (s *StudentVerificationService) AdminListVerifications(ctx context.Context, filter *StudentVerificationAdminFilter) (*StudentVerificationAdminList, error) {
	if s == nil || s.verificationRepo == nil {
		return nil, ErrServiceUnavailable
	}
	return s.verificationRepo.AdminList(ctx, filter)
}

// AdminRevokeVerification 管理员撤销认证记录（回写 verified → revoked）。
// 不自动撤销权益台账：权益撤销是独立的显式管理动作（见 SubscriptionGrantService.RevokeGrant）。
func (s *StudentVerificationService) AdminRevokeVerification(ctx context.Context, id, operatorID int64, reason string) (*StudentVerification, error) {
	if s == nil || s.entClient == nil || s.verificationRepo == nil {
		return nil, ErrServiceUnavailable
	}
	verification, err := s.verificationRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if verification.Status != domain.StudentVerificationStatusVerified {
		return verification, nil
	}
	if _, err := s.verificationRepo.RevokeByUser(ctx, verification.UserID, operatorID, reason); err != nil {
		return nil, fmt.Errorf("revoke student verification: %w", err)
	}
	return s.verificationRepo.GetByID(ctx, id)
}

// buildBenefitCommand 组装学生权益发放命令。
func (s *StudentVerificationService) buildBenefitCommand(ctx context.Context, userID int64, normalizedEmail string) (*CreateSubscriptionGrantCommand, error) {
	groupID := s.settingReader.GetStudentBenefitGroupID(ctx)
	days := s.settingReader.GetStudentBenefitValidityDays(ctx)
	if groupID <= 0 || days <= 0 {
		return nil, ErrStudentBenefitMisconfigured
	}
	benefitCode := s.StudentBenefitCode(ctx)
	identityType := StudentBenefitIdentityTypeHubuEmail
	if provider := s.StudentVerificationProvider(); provider != "" {
		identityType = strings.ToLower(provider)
	}
	cmd := &CreateSubscriptionGrantCommand{
		UserID:          userID,
		GroupID:         groupID,
		Source:          domain.SubscriptionGrantSourceStudentVerification,
		SourceKey:       fmt.Sprintf("%s:%s", benefitCode, normalizedEmail),
		BenefitCode:     benefitCode,
		IdentityType:    identityType,
		IdentityKey:     normalizedEmail,
		EffectivePolicy: domain.SubscriptionGrantPolicyImmediate,
		DurationDays:    days,
		Reason:          fmt.Sprintf("%s 学生邮箱认证自动发放", s.brand.Name),
		Notes:           "auto granted by student email verification",
		// 安全策略：当前已有其他分组的有效付费订阅 → 转入待生效衔接，
		// 绝不静默覆盖/降级（任务规则 C）。
		PendingFallback: true,
	}
	if planID := s.settingReader.GetStudentBenefitPlanID(ctx); planID > 0 {
		cmd.PlanID = &planID
	}
	return cmd, nil
}

// normalizeStudentEmail 归一化并校验学生邮箱（后端权威校验，前端校验不可信）。
func (s *StudentVerificationService) normalizeStudentEmail(email string) (string, error) {
	domainName := s.StudentEmailDomain()
	if domainName == "" {
		return "", ErrStudentVerificationDisabled
	}
	address := strings.ToLower(strings.TrimSpace(email))
	parsed, err := mail.ParseAddress(address)
	if err != nil || parsed.Address != address {
		return "", ErrStudentEmailInvalid
	}
	separator := strings.LastIndexByte(address, '@')
	if separator <= 0 || address[separator+1:] != domainName {
		return "", ErrStudentEmailInvalid
	}
	return address, nil
}

func (s *StudentVerificationService) codeKey(userID int64) string {
	return fmt.Sprintf("%s%d", studentVerificationCodeKeyPrefix, userID)
}

// sendCodeEmail 品牌化双语验证码邮件（复用 SMTP 基础设施，不落模板系统）。
func (s *StudentVerificationService) sendCodeEmail(ctx context.Context, to, code string) error {
	site := s.brand.SiteName
	if site == "" {
		site = s.brand.Name
	}
	school := s.brand.Name
	if school == "" {
		school = s.brand.ShortName
	}
	subject := fmt.Sprintf("[%s] 学生邮箱认证验证码 / Student Email Verification Code", site)
	body := fmt.Sprintf(`<!doctype html><html><body style="font-family:Arial,sans-serif;color:#1f2937">
<h2>%s 校园身份认证</h2>
<p>您的验证码是：</p>
<p style="font:700 32px monospace;letter-spacing:8px">%s</p>
<p>验证码 15 分钟内有效。若非本人操作，请忽略此邮件并注意账号安全。</p>
<hr>
<h2>%s Student Email Verification</h2>
<p>Your verification code is <strong>%s</strong>. It expires in 15 minutes.</p>
<p>If you did not request this code, you can ignore this email.</p>
</body></html>`, html.EscapeString(school), html.EscapeString(code), html.EscapeString(school), html.EscapeString(code))
	if err := s.emailService.SendEmail(ctx, to, subject, body); err != nil {
		return fmt.Errorf("send student verification email: %w", err)
	}
	return nil
}

// StudentVerificationResult 认证 + 发放的完整结果（前端成功态展示）。
type StudentVerificationResult struct {
	Verification *StudentVerification `json:"verification"`
	Grant        *SubscriptionGrant   `json:"grant"`
	Outcome      *GrantOutcome        `json:"outcome"`
}

// StudentVerificationStatus 用户侧认证状态。
type StudentVerificationStatus struct {
	EmailVerified  bool       `json:"email_verified"`
	Email          string     `json:"email,omitempty"`
	Provider       string     `json:"provider"`
	EmailDomain    string     `json:"email_domain"`
	BenefitCode    string     `json:"benefit_code"`
	BenefitDays    int        `json:"benefit_days"`
	VerifiedAt     *time.Time `json:"verified_at,omitempty"`
	GrantStatus    string     `json:"grant_status,omitempty"`
	GrantExpiresAt *time.Time `json:"grant_expires_at,omitempty"`
}

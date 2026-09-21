package service

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/authidentity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/campus"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	educationEmailProvider      = "education_email"
	educationEmailCodeKeyPrefix = "education-email-identity:user:"

	// EducationEmailProviderType / EducationEmailProviderKey 是上面对应常量的导出别名，
	// 供仓储层与管理端撤销流程定位校园邮箱认证身份，避免重复散落字面量。
	EducationEmailProviderType = educationEmailProvider
)

var (
	educationEmailProviderKey             = campus.Current().EducationDomain
	EducationEmailProviderKey             = educationEmailProviderKey
	ErrEducationEmailInvalid              = infraerrors.BadRequest("EDUCATION_EMAIL_INVALID", "use an exact campus email address")
	ErrEducationEmailAlreadyBound         = infraerrors.Conflict("EDUCATION_EMAIL_ALREADY_BOUND", "this education email is already verified")
	ErrEducationEmailVerificationDisabled = infraerrors.Forbidden("EDUCATION_EMAIL_VERIFICATION_DISABLED", "campus email verification is not enabled")
)

// SendEducationEmailCode sends a short-lived verification code tied to both
// the authenticated user and the exact MUC email address being verified.
func (s *AuthService) SendEducationEmailCode(ctx context.Context, userID int64, email string, locale ...string) error {
	_ = locale
	if s == nil {
		return ErrServiceUnavailable
	}
	if s.settingService == nil || !s.settingService.IsEducationEmailVerificationEnabled(ctx) {
		return ErrEducationEmailVerificationDisabled
	}
	if s.emailService == nil || s.emailService.cache == nil || userID <= 0 {
		return ErrServiceUnavailable
	}
	normalizedEmail, err := normalizeEducationEmail(email)
	if err != nil {
		return err
	}

	cache := s.emailService.cache
	cacheKey := educationEmailCodeKey(userID)
	now := time.Now().UTC()
	rate, err := cache.GetNotifyCodeUserRate(ctx, userID)
	if err != nil {
		return ErrServiceUnavailable
	}
	if rate >= notifyCodeUserRateLimit {
		return ErrNotifyCodeUserRateLimit
	}

	code, err := s.emailService.GenerateVerifyCode()
	if err != nil {
		return fmt.Errorf("generate education email code: %w", err)
	}
	reserved, err := cache.ReserveVerificationCodeCooldown(ctx, cacheKey, verifyCodeCooldown)
	if err != nil {
		return ErrServiceUnavailable
	}
	if !reserved {
		return ErrVerifyCodeTooFrequent
	}
	// 与 SendNotifyEmailCode 相同的口径：SMTP 发送失败时不落验证码、不烧限流
	// 配额，并释放已预定的冷却，避免用户被 1 分钟冷却锁死。
	if err := s.emailService.SendEducationEmailVerification(ctx, normalizedEmail, code); err != nil {
		if releaseErr := cache.ReleaseVerificationCodeCooldown(ctx, cacheKey); releaseErr != nil {
			slog.Error("failed to release education email cooldown", "user_id", userID, "error", releaseErr)
		}
		return err
	}
	data := &VerificationCodeData{
		Code:      code,
		Target:    normalizedEmail,
		CreatedAt: now,
		ExpiresAt: now.Add(verifyCodeTTL),
	}
	if err := cache.SetVerificationCode(ctx, cacheKey, data, verifyCodeTTL); err != nil {
		return ErrServiceUnavailable
	}

	// 用户级限流只在发送成功后递增一次。
	if _, err := cache.IncrNotifyCodeUserRate(ctx, userID, notifyCodeUserRateWindow); err != nil {
		slog.Error("failed to increment education email rate", "user_id", userID, "error", err)
	}
	return nil
}

// VerifyAndBindEducationEmail consumes a code once and persists a verified
// campus identity without changing the user's primary login email.
func (s *AuthService) VerifyAndBindEducationEmail(ctx context.Context, userID int64, email, code string) error {
	if s == nil {
		return ErrServiceUnavailable
	}
	if s.settingService == nil || !s.settingService.IsEducationEmailVerificationEnabled(ctx) {
		return ErrEducationEmailVerificationDisabled
	}
	if s.emailService == nil || s.emailService.cache == nil || s.entClient == nil || userID <= 0 {
		return ErrServiceUnavailable
	}
	normalizedEmail, err := normalizeEducationEmail(email)
	if err != nil {
		return err
	}
	cacheKey := educationEmailCodeKey(userID)
	data, err := s.emailService.cache.ConsumeVerificationCode(ctx, cacheKey)
	if err != nil {
		return ErrServiceUnavailable
	}
	code = strings.TrimSpace(code)
	if data == nil || data.ExpiresAt.IsZero() || !time.Now().Before(data.ExpiresAt) ||
		!strings.EqualFold(data.Target, normalizedEmail) ||
		subtle.ConstantTimeCompare([]byte(data.Code), []byte(code)) != 1 {
		return ErrInvalidVerifyCode
	}

	client := s.entClient
	if tx := dbent.TxFromContext(ctx); tx != nil {
		client = tx.Client()
	}
	if err := client.AuthIdentity.Create().
		SetUserID(userID).
		SetProviderType(educationEmailProvider).
		SetProviderKey(educationEmailProviderKey).
		SetProviderSubject(normalizedEmail).
		SetVerifiedAt(time.Now().UTC()).
		SetMetadata(map[string]any{"source": campus.Current().ID + "_education_email_verification"}).
		OnConflictColumns(
			authidentity.FieldProviderType,
			authidentity.FieldProviderKey,
			authidentity.FieldProviderSubject,
		).
		DoNothing().
		Exec(ctx); err != nil && !isSQLNoRowsError(err) {
		return ErrServiceUnavailable
	}

	identity, err := client.AuthIdentity.Query().Where(
		authidentity.ProviderTypeEQ(educationEmailProvider),
		authidentity.ProviderKeyEQ(educationEmailProviderKey),
		authidentity.ProviderSubjectEQ(normalizedEmail),
	).Only(ctx)
	if err != nil {
		return ErrServiceUnavailable
	}
	if identity.UserID != userID {
		return ErrEmailExists
	}
	return nil
}

func educationEmailCodeKey(userID int64) string {
	return fmt.Sprintf("%s%d", educationEmailCodeKeyPrefix, userID)
}

func normalizeEducationEmail(email string) (string, error) {
	address := strings.ToLower(strings.TrimSpace(email))
	parsed, err := mail.ParseAddress(address)
	if err != nil || parsed.Address != address {
		return "", ErrEducationEmailInvalid
	}
	separator := strings.LastIndexByte(address, '@')
	if separator <= 0 || address[separator+1:] != educationEmailProviderKey {
		return "", ErrEducationEmailInvalid
	}
	return address, nil
}

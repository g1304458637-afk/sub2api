package domain

// Subscription Grant 来源（subscription_grants.source）。
// Grant 回答「为什么、由谁、通过什么活动给了用户一份订阅权益」；
// 新增来源 = 加常量即可，Grant 层不感知来源业务语义。
const (
	SubscriptionGrantSourceAdminGrant          = "admin_grant"
	SubscriptionGrantSourceStudentVerification = "student_verification"
	SubscriptionGrantSourceCampaign            = "campaign"
	SubscriptionGrantSourceInvitation          = "invitation"
	SubscriptionGrantSourceCompensation        = "compensation"
	SubscriptionGrantSourceSchoolBulk          = "school_bulk"
	SubscriptionGrantSourceTeacherVerification = "teacher_verification"
	SubscriptionGrantSourceInternal            = "internal"
	SubscriptionGrantSourceOther               = "other"
)

// SubscriptionGrantStatus 台账状态。
//   - pending:   已登记未生效（等待无冲突激活机会）
//   - fulfilled: 已激活并写入订阅权益（contribution_start/end 记录贡献时段）
//   - expired:   贡献时段已自然结束（由 worker 落库，非用户权益损失）
//   - revoked:   被管理员撤销（受付费地板保护，只回收纯赠送时段）
//   - failed:    激活时配置非法等终态失败
const (
	SubscriptionGrantStatusPending   = "pending"
	SubscriptionGrantStatusFulfilled = "fulfilled"
	SubscriptionGrantStatusExpired   = "expired"
	SubscriptionGrantStatusRevoked   = "revoked"
	SubscriptionGrantStatusFailed    = "failed"
)

// Grant 生效策略（subscription_grants.effective_policy）。
const (
	// SubscriptionGrantPolicyImmediate 立即生效：无冲突则当场激活；同组已有
	// active 订阅则从其 expires_at 顺延；跨组冲突则拒绝（管理员改选 end_of_term）。
	SubscriptionGrantPolicyImmediate = "immediate"
	// SubscriptionGrantPolicyEndOfTerm 当前订阅结束后生效：登记为 pending，
	// 由后台 worker 在用户无冲突 active 订阅时激活。
	SubscriptionGrantPolicyEndOfTerm = "end_of_term"
)

// 合法 source 集合（服务层校验用）。
var ValidSubscriptionGrantSources = map[string]bool{
	SubscriptionGrantSourceAdminGrant:          true,
	SubscriptionGrantSourceStudentVerification: true,
	SubscriptionGrantSourceCampaign:            true,
	SubscriptionGrantSourceInvitation:          true,
	SubscriptionGrantSourceCompensation:        true,
	SubscriptionGrantSourceSchoolBulk:          true,
	SubscriptionGrantSourceTeacherVerification: true,
	SubscriptionGrantSourceInternal:            true,
	SubscriptionGrantSourceOther:               true,
}

// 学生认证 provider（student_verifications.provider）。
// 事实边界：EMAIL 类 provider 只证明「用户控制一个有效学生邮箱」，
// 不证明学籍状态；SSO 类为未来接入学校统一认证预留。
const (
	StudentVerificationProviderHubuEmail = "HUBU_EMAIL"
	StudentVerificationProviderHubuSSO   = "HUBU_SSO"
	StudentVerificationProviderMucEmail  = "MUC_EMAIL"
	StudentVerificationProviderMucSSO    = "MUC_SSO"
	StudentVerificationProviderOther     = "OTHER_UNIVERSITY"
)

// 认证记录状态。
const (
	StudentVerificationStatusVerified = "verified"
	StudentVerificationStatusRevoked  = "revoked"
)

// HUBU 学生邮箱域名（品牌档案可被 HUBU_EDUCATION_EMAIL_DOMAIN 覆盖）。
const HubuStudentEmailDomain = "stu.hubu.edu.cn"

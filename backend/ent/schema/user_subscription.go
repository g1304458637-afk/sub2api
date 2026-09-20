package schema

import (
	"time"

	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/domain"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// UserSubscription holds the schema definition for the UserSubscription entity.
type UserSubscription struct {
	ent.Schema
}

func (UserSubscription) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "user_subscriptions"},
	}
}

func (UserSubscription) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
		mixins.SoftDeleteMixin{},
	}
}

func (UserSubscription) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.Int64("group_id"),

		field.Time("starts_at").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("expires_at").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("status").
			MaxLen(20).
			Default(domain.SubscriptionStatusActive),

		field.Time("daily_window_start").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("weekly_window_start").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("monthly_window_start").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),

		field.Float("daily_usage_usd").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).
			Default(0),
		field.Float("weekly_usage_usd").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).
			Default(0),
		field.Float("monthly_usage_usd").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).
			Default(0),

		field.Int64("assigned_by").
			Optional().
			Nillable(),
		field.Time("assigned_at").
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("notes").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),

		// 用户级 PAYG fallback 开关（个人选择，非分组属性）：
		// false（默认）= 订阅额度耗尽 → 拒绝；
		// true = 订阅额度耗尽 → 后续请求可转余额计费（V1 后续 Phase 实现，仅建字段）。
		// 默认值即现行为：迁移后存量订阅零变化。
		field.Bool("auto_payg_fallback").
			Default(false),

		// Plan 身份（Gate 1）：新购买/续费/升级履约写入；历史行为 NULL =
		// plan_identity_unresolved，Upgrade Preview 拒绝并要求 Admin resolve。
		field.Int64("plan_id").Optional().Nillable(),
		// scheduled downgrade：term 结束后的下一档（Renewal 时生效）
		field.Int64("next_plan_id").Optional().Nillable(),
	}
}

func (UserSubscription) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("subscriptions").
			Field("user_id").
			Unique().
			Required(),
		edge.From("group", Group.Type).
			Ref("subscriptions").
			Field("group_id").
			Unique().
			Required(),
		edge.From("assigned_by_user", User.Type).
			Ref("assigned_subscriptions").
			Field("assigned_by").
			Unique(),
		edge.To("usage_logs", UsageLog.Type),
		// 已付 term 快照（Plan Change proration 的价格真相）
		edge.To("terms", SubscriptionTerm.Type),
		// Reset 应用记录：订阅硬删时随订阅级联清除（事件主记录经 RESTRICT 保留）
		edge.To("reset_applications", SubscriptionResetApplication.Type),
		// 使用本订阅消费的 Reset Card：订阅硬删时 used_subscription_id 置 NULL（卡历史保留）
		edge.To("used_by_reset_cards", SubscriptionResetCard.Type),
	}
}

func (UserSubscription) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("group_id"),
		index.Fields("status"),
		index.Fields("expires_at"),
		// 活跃订阅查询复合索引（线上由 SQL 迁移创建部分索引，schema 仅用于模型可读性对齐）
		index.Fields("user_id", "status", "expires_at"),
		index.Fields("assigned_by"),
		// 唯一约束通过部分索引实现（WHERE deleted_at IS NULL），支持软删除后重新订阅
		// 见迁移文件 016_soft_delete_partial_unique_indexes.sql
		index.Fields("user_id", "group_id"),
		index.Fields("deleted_at"),
	}
}

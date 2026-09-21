package schema

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SubscriptionResetApplication 记录 Reset Event 实际作用到的每一条 UserSubscription。
//
// 审计要求（Phase 1 架构确认）：
//   - 必须能回答「Event X 是否作用到 Subscription Y」→ UNIQUE(reset_event_id, user_subscription_id)
//     同时是 Global Reset worker retry-safe 的基石；
//   - 必须能回答「作用时旧 Anchor / Usage 是什么」→ previous_* 列；
//   - effective_at 冗余自事件，使单行自带「承诺的新锚点」。
//
// 未来 worker 的守卫 invariant（schema 提供数据基础，SQL 由后续 Phase 实现）：
//
//	只允许把锚点从更早的时刻推进到事件 effective_at；
//	current anchor >= event.effective_at 时记 skipped（绝不回拨用户的新锚点）。
//
// 删除语义：事件被引用时 RESTRICT 不可删；订阅硬删时本表随订阅 CASCADE
// （事件主记录保留，用户级数据跟随订阅生命周期，与 user_subscriptions.user_id 惯例一致）。
type SubscriptionResetApplication struct {
	ent.Schema
}

func (SubscriptionResetApplication) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "subscription_reset_applications"},
	}
}

func (SubscriptionResetApplication) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("reset_event_id"),
		field.Int64("user_subscription_id"),
		field.Time("effective_at").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("previous_weekly_window_start").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Float("previous_weekly_usage_usd").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}),
		field.Time("applied_at").
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("status").
			MaxLen(20).
			Default(domain.ResetApplicationStatusApplied),
		field.JSON("metadata", map[string]any{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
	}
}

func (SubscriptionResetApplication) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("reset_event", SubscriptionResetEvent.Type).
			Ref("applications").
			Field("reset_event_id").
			Required().
			Unique().
			Annotations(entsql.OnDelete(entsql.Restrict)),
		edge.From("user_subscription", UserSubscription.Type).
			Ref("reset_applications").
			Field("user_subscription_id").
			Required().
			Unique().
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (SubscriptionResetApplication) Indexes() []ent.Index {
	return []ent.Index{
		// 唯一约束覆盖 (reset_event_id, user_subscription_id) 复合查询；
		// 单订阅侧查询（该订阅的全部 Reset 历史）需要本索引。
		index.Fields("user_subscription_id"),
	}
}

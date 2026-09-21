package schema

import (
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

// SubscriptionResetEvent 定义批量 Reset / 发卡事件（Subscription V1）。
//
// 一次事件 = 一条系统/管理员动作的唯一事实源，同时支撑两类已确认用途：
//   - global_reset：批量重置订阅周周期锚点（effective_at 是权威锚点时间）
//   - reset_card_grant：批量发放 Reset Card
//
// 审计优先：硬删除（跟随 redeem_codes 等审计型实体的策略）；
// 被 applications / grant cards 引用时受 FK RESTRICT 保护无法删除。
// scope 过滤条件存 JSONB（见 migration 239 注释），不做 nullable 列组合。
type SubscriptionResetEvent struct {
	ent.Schema
}

func (SubscriptionResetEvent) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "subscription_reset_events"},
	}
}

func (SubscriptionResetEvent) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (SubscriptionResetEvent) Fields() []ent.Field {
	return []ent.Field{
		field.String("event_type").
			MaxLen(32).
			NotEmpty(),
		field.String("status").
			MaxLen(20).
			Default(domain.ResetEventStatusPending),
		// Global Reset 的权威锚点时间：worker 处理时刻不参与计算，
		// 所有目标订阅的新锚点一律取该值（即使处理跨越数分钟）。
		field.Time("effective_at").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("scope_type").
			MaxLen(32).
			NotEmpty(),
		field.JSON("scope", map[string]any{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.String("reason").
			Default("").
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("campaign").
			MaxLen(64).
			Optional().
			Nillable(),
		field.Int64("created_by").
			Optional().
			Nillable(),
		field.Time("started_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("completed_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.JSON("metadata", map[string]any{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
	}
}

func (SubscriptionResetEvent) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("created_by_user", User.Type).
			Ref("created_reset_events").
			Field("created_by").
			Unique().
			Annotations(entsql.OnDelete(entsql.SetNull)),
		edge.To("applications", SubscriptionResetApplication.Type),
		edge.To("granted_cards", SubscriptionResetCard.Type),
	}
}

func (SubscriptionResetEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("effective_at"),
	}
}

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

// SubscriptionResetCard 定义一次性可消费的周期重置权益（Banked Reset Card）。
//
// 设计要点（Phase 1 架构确认）：
//   - 逐卡一行（禁止 users.reset_card_count 计数列）：可审计、可过期、可撤销、
//     可追溯来源与消费对象；
//   - 卡属于 User，未使用前不绑定订阅——用户可同时持有多分组订阅
//     （Phase 0 已实证），消费时才以 used_subscription_id 记录实际作用对象；
//   - 使用时刻即该订阅的新 Weekly Anchor（Re-Anchoring Reset）；
//   - 永不延长订阅 expires_at、不触碰 balance；
//   - 历史保留：used/expired/revoked 都是 status，不物理删除；
//     软删除仅用于误发放的恢复场景。
type SubscriptionResetCard struct {
	ent.Schema
}

func (SubscriptionResetCard) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "subscription_reset_cards"},
	}
}

func (SubscriptionResetCard) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
		mixins.SoftDeleteMixin{},
	}
}

func (SubscriptionResetCard) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("status").
			MaxLen(20).
			Default(domain.ResetCardStatusAvailable),
		field.String("scope").
			MaxLen(20).
			Default(domain.ResetCardScopeWeekly),
		field.String("source_type").
			MaxLen(32).
			Default(domain.ResetCardSourceAdminGrant),
		field.String("campaign").
			MaxLen(64).
			Optional().
			Nillable(),
		// 活动发卡的事件回链；管理员单发为 NULL
		field.Int64("grant_event_id").
			Optional().
			Nillable(),
		// 活动内序号（0 起）：UNIQUE(grant_event_id, user_id, grant_index)
		// 支持同一活动每人 N 张的批量幂等
		field.Int("grant_index").
			Default(0),
		field.Time("granted_at").
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("expires_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("used_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		// 消费时才绑定的目标订阅
		field.Int64("used_subscription_id").
			Optional().
			Nillable(),
		field.Int64("created_by").
			Optional().
			Nillable(),
		field.String("notes").
			Default("").
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.JSON("metadata", map[string]any{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
	}
}

func (SubscriptionResetCard) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("reset_cards").
			Field("user_id").
			Required().
			Unique().
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.From("created_by_user", User.Type).
			Ref("created_reset_cards").
			Field("created_by").
			Unique().
			Annotations(entsql.OnDelete(entsql.SetNull)),
		edge.From("used_subscription", UserSubscription.Type).
			Ref("used_by_reset_cards").
			Field("used_subscription_id").
			Unique().
			Annotations(entsql.OnDelete(entsql.SetNull)),
		edge.From("grant_event", SubscriptionResetEvent.Type).
			Ref("granted_cards").
			Field("grant_event_id").
			Unique().
			Annotations(entsql.OnDelete(entsql.Restrict)),
	}
}

func (SubscriptionResetCard) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "status"),
		index.Fields("expires_at"),
		index.Fields("grant_event_id"),
		// 批量发卡幂等：同活动同用户同序号至多一张；仅约束活动卡，
		// 软删除的卡不占幂等键（支持误发放删除后重发）。
		index.Fields("grant_event_id", "user_id", "grant_index").
			Unique().
			Annotations(entsql.IndexWhere("grant_event_id IS NOT NULL AND deleted_at IS NULL")),
	}
}

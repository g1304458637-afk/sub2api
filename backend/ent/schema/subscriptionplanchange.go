package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SubscriptionPlanChange Plan Change 审计实体：Quote 冻结快照 + 履约链路。
// 硬删除；取消以 status=cancelled + cancel_reason 表达，保留全程审计。
type SubscriptionPlanChange struct {
	ent.Schema
}

func (SubscriptionPlanChange) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "subscription_plan_changes"}}
}

func (SubscriptionPlanChange) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.TimeMixin{}, mixins.SoftDeleteMixin{}}
}

func (SubscriptionPlanChange) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.Int64("subscription_id"),
		field.String("change_type").MaxLen(20),
		field.Int64("from_plan_id").Optional().Nillable(),
		field.Int64("to_plan_id"),
		field.Int64("from_group_id").Optional().Nillable(),
		field.Int64("to_group_id"),
		field.Int("from_tier").Default(0),
		field.Int("to_tier").Default(0),

		field.Float("old_price_snapshot").
			Optional().Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}),
		field.Float("new_price_snapshot").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}).
			Default(0),
		field.String("currency").MaxLen(8).Default(""),
		field.Time("term_start").Optional().Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("term_end").Optional().Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Int64("remaining_seconds").Default(0),
		field.Float("unused_credit").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}).
			Default(0),
		field.Float("prorated_charge").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}).
			Default(0),
		field.Float("amount_due").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}).
			Default(0),
		field.Time("quote_created_at").Optional().Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("quote_expires_at").Optional().Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),

		field.Time("effective_at").Optional().Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("status").MaxLen(20).Default("quoted"),
		field.String("cancel_reason").MaxLen(64).Optional().Nillable(),
		field.Int64("order_id").Optional().Nillable(),
		field.String("idempotency_key").MaxLen(128).Optional().Nillable(),
		field.JSON("metadata", map[string]any{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),

		field.Time("paid_at").Optional().Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("fulfilled_at").Optional().Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("cancelled_at").Optional().Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (SubscriptionPlanChange) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("subscription_id"),
		index.Fields("user_id"),
		index.Fields("status"),
		index.Fields("idempotency_key").
			Unique().
			Annotations(entsql.IndexWhere("idempotency_key IS NOT NULL AND deleted_at IS NULL")),
	}
}

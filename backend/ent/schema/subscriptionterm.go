package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SubscriptionTerm 保存每段已付订阅事实（价格快照/term 起止/来源订单）。
// Plan Change 的 proration 以此为唯一价格真相（Gate 2/3），绝不使用目录价。
type SubscriptionTerm struct {
	ent.Schema
}

func (SubscriptionTerm) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "subscription_terms"}}
}

func (SubscriptionTerm) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("subscription_id"),
		field.Int64("order_id").Optional().Nillable(),
		field.Int64("plan_id").Optional().Nillable(),
		field.Float("price_paid").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,2)"}).
			Default(0),
		field.String("currency").MaxLen(8).Default(""),
		field.Int("days").Default(0),
		field.Time("term_start").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("term_end").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("source").MaxLen(20).Default("purchase"),
		field.Time("created_at").Default(time.Now).Immutable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (SubscriptionTerm) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("subscription", UserSubscription.Type).
			Ref("terms").Field("subscription_id").Required().Unique(),
	}
}

func (SubscriptionTerm) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("subscription_id"),
		index.Fields("order_id"),
	}
}

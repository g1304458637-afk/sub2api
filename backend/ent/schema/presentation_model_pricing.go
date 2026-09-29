package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// PresentationModelPricing holds the schema definition for the PresentationModelPricing entity.
//
// 展示价（Presentation Pricing）独立层：仅决定用户端「模型与价格」页面看到的
// 标准价格，不参与任何真实计费（Billing / Subscription / Wallet / Usage）。
// 依赖方向：展示层可读取 Billing 价格作为 fallback；Billing 永不读取本表。
//
// 删除策略：硬删除（清除 override 即恢复 manual → official → billing 回退链，
// 删除与展示价语义一致，无历史包袱）。管理员操作由 AuditLogMiddleware 审计，
// 行内保留 updated_by / updated_at。
type PresentationModelPricing struct {
	ent.Schema
}

func (PresentationModelPricing) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "presentation_model_pricing"},
	}
}

func (PresentationModelPricing) Fields() []ent.Field {
	return []ent.Field{
		field.String("model_name").
			MaxLen(255).
			NotEmpty().
			Unique().
			Comment("canonical model key（渠道 SupportedModels 名称）"),
		field.String("billing_mode").
			MaxLen(20).
			Default("token").
			Comment("计价形态: token / image / per_request / video"),
		field.String("currency").
			MaxLen(10).
			Default("USD").
			Comment("价格币种（账本币种；展示层按现有汇率切换）"),
		field.Float("input_price").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "double precision"}).
			Comment("输入价（USD / token）"),
		field.Float("output_price").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "double precision"}).
			Comment("输出价（USD / token）"),
		field.Float("cache_write_price").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "double precision"}).
			Comment("缓存写入价 5m（USD / token）"),
		field.Float("cache_write_1h_price").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "double precision"}).
			Comment("缓存写入价 1h（USD / token）"),
		field.Float("cache_read_price").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "double precision"}).
			Comment("缓存读取价（USD / token）"),
		field.Float("per_request_price").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "double precision"}).
			Comment("按次价（USD / 次；按图、按视频等）"),
		field.Bool("enabled").
			Default(true).
			Comment("false 时本条 override 不生效，走回退链"),
		field.Text("remark").
			Optional().
			Comment("管理员备注"),
		field.Int64("updated_by").
			Optional().
			Nillable().
			Comment("最后修改的管理员用户 ID"),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (PresentationModelPricing) Indexes() []ent.Index {
	return nil
}

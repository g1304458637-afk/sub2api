package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/domain"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// 科研优惠申请状态机
const (
	// ResearchApplicationStatusPending 待审核（创建后默认状态）
	ResearchApplicationStatusPending = "pending"
	// ResearchApplicationStatusApproved 已通过（余额兑换券已静默发放）
	ResearchApplicationStatusApproved = "approved"
	// ResearchApplicationStatusRejected 已驳回（review_notes 必填）
	ResearchApplicationStatusRejected = "rejected"
)

// ResearchAttachmentMeta 科研优惠申请附件元数据类型已迁移至 internal/domain
// （见 domain.ResearchAttachmentMeta）：ent 生成代码引用 JSON 字段类型时，
// 该类型不能定义在 ent/schema 包内，否则形成 import 环。

// ResearchApplication 科研优惠登记申请：用户提交科研身份证明（文字说明 + 附件），
// 管理员审核；通过时后端静默发放等额余额兑换券（RedeemForAdminFulfillment）。
type ResearchApplication struct {
	ent.Schema
}

func (ResearchApplication) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "research_applications"},
	}
}

func (ResearchApplication) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (ResearchApplication) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.Text("description"),
		field.String("status").
			MaxLen(20).
			Default(ResearchApplicationStatusPending),
		field.Int64("reviewer_id").
			Optional().
			Nillable(),
		field.Text("review_notes").
			Optional().
			Nillable(),
		field.Float("reward_amount").
			Optional().
			Nillable(),
		field.Int64("issued_redeem_code_id").
			Optional().
			Nillable(),
		field.Time("reviewed_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.JSON("attachments", []domain.ResearchAttachmentMeta{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
	}
}

func (ResearchApplication) Indexes() []ent.Index {
	return []ent.Index{
		// 用户端"我的申请列表"按 user_id 过滤 + created_at 倒序
		index.Fields("user_id"),
		// 管理端按状态筛选列表
		index.Fields("status"),
	}
}

package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// ResearchAttachmentUpload 科研附件"待绑定"上传记录。
//
// 生命周期：上传接口先落盘（DATA_DIR/uploads/research/<uuid><ext>）并写入本表
// （application_id 为 NULL），返回 {id(uuid), name, mime, size}；用户提交申请时，
// 校验每个附件存在、属于本人、未被其他申请绑定后，将其 application_id 绑定为申请 ID。
// 物理路径 storage_path 只存本表：下载时由 DB 取 path + DATA_DIR 拼绝对路径，
// 禁止用户输入参与路径拼接（防穿越）。
type ResearchAttachmentUpload struct {
	ent.Schema
}

func (ResearchAttachmentUpload) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "research_attachment_uploads"},
	}
}

func (ResearchAttachmentUpload) Mixin() []ent.Mixin {
	return []ent.Mixin{
		// 契约只要求 created_at；这里仍复用 TimeMixin（created_at/updated_at），
		// 与 research_applications 及全库时间戳惯例保持一致。
		mixins.TimeMixin{},
	}
}

func (ResearchAttachmentUpload) Fields() []ent.Field {
	return []ent.Field{
		// 主键：UUID 字符串（36 位）。本仓库默认 idtype 为 int64，这里按
		// 科研附件契约显式声明字符串 UUID 主键，避免可枚举 ID 泄露附件归属。
		field.String("id").
			MaxLen(36).
			DefaultFunc(uuid.NewString),
		field.Int64("user_id"),
		field.String("original_name"),
		field.String("mime").
			MaxLen(100),
		field.Int64("size"),
		field.String("storage_path"),
		field.Int64("application_id").
			Optional().
			Nillable(),
	}
}

func (ResearchAttachmentUpload) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("application_id"),
	}
}

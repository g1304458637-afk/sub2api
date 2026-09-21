// Package domain 科研优惠登记领域类型。
// 独立于 ent schema 包：ent 生成的代码会引用本类型（JSON 字段），放在 schema 包
// 会造成 ent → schema → mixins → ent 的 import 环。
package domain

// ResearchAttachmentMeta 科研优惠申请附件元数据（冗余存于 research_applications.attachments jsonb）。
//
// 设计说明：附件的物理存储路径（storage_path）只保存在 research_attachment_uploads
// 表中，本 jsonb 不含 path——下载时按 uploads 行查 path，避免路径信息多处冗余导致
// 越权/穿越面扩大。ID 为上传阶段生成的 UUID（research_attachment_uploads.id）。
type ResearchAttachmentMeta struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Mime string `json:"mime"`
	Size int64  `json:"size"`
}

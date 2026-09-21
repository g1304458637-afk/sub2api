export default {
  research: {
    title: '科研优惠审核',
    description: '审核用户提交的科研优惠申请',

    filter: {
      all: '全部状态',
      pending: '待审核',
      approved: '已通过',
      rejected: '已驳回'
    },

    columns: {
      applicant: '申请人',
      description: '申请说明',
      attachments: '附件',
      createdAt: '申请时间',
      status: '状态',
      actions: '操作'
    },

    status: {
      pending: '待审核',
      approved: '已通过',
      rejected: '已驳回'
    },

    attachmentCount: '{count} 个',
    viewDetail: '详情',
    applicantFallback: '用户 {id}',

    detail: {
      title: '申请详情',
      applicant: '申请人',
      submittedAt: '申请时间',
      reviewedAt: '审核时间',
      notReviewed: '未审核',
      descriptionLabel: '申请说明',
      attachmentsLabel: '证明材料',
      noAttachments: '无附件',
      preview: '预览',
      download: '下载',
      downloadFailed: '附件下载失败',
      previewFailed: '附件预览失败',
      reviewNotes: '审核备注',
      noReviewNotes: '暂无审核备注',
      rewardAmount: '发放金额',
      approve: '通过',
      reject: '驳回',
      close: '关闭'
    },

    approve: {
      title: '通过申请',
      applicantHint: '通过后将向 {name} 的账户余额发放优惠金额',
      amountLabel: '发放金额（$）',
      amountInvalid: '请输入有效的发放金额（大于 0）',
      notesLabel: '审核备注（可选）',
      notesPlaceholder: '选填，将展示给用户',
      confirm: '确认通过',
      submitting: '提交中...',
      success: '已通过申请并发放余额',
      failed: '通过申请失败'
    },

    reject: {
      title: '驳回申请',
      notesLabel: '驳回原因',
      notesPlaceholder: '请填写驳回原因（必填），将展示给用户',
      notesRequired: '请填写驳回原因',
      confirm: '确认驳回',
      submitting: '提交中...',
      success: '已驳回申请',
      failed: '驳回申请失败'
    },

    list: {
      loadFailed: '加载申请列表失败',
      empty: '暂无申请',
      emptyHint: '当前筛选条件下没有符合条件的申请'
    }
  }
}

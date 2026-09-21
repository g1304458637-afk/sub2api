export default {
  research: {
    title: '科研优惠',
    description: '提交科研身份证明材料，审核通过后自动获得余额优惠',

    intro: {
      title: '关于科研优惠',
      rule1: '科研优惠面向教职工与科研人员，为账户提供余额优惠。',
      rule2: '请在申请说明中简要介绍您的科研身份，并上传证明材料。',
      rule3: '提交后由管理员人工审核，可在下方「我的申请」中查看进度。',
      rule4: '审核通过后，优惠金额会自动发放到您的账户余额。'
    },

    form: {
      title: '提交申请',
      descriptionLabel: '申请说明',
      descriptionPlaceholder:
        '请简要介绍您的身份与研究情况，例如：某高校计算机系教师，因科研课题需要使用大模型 API…',
      descriptionRequired: '请填写申请说明',
      descriptionTooLong: '申请说明不能超过 {max} 字',
      charCount: '{count}/{max}',
      attachmentsLabel: '证明材料（最多 {max} 个）',
      attachmentsHint: '支持 JPG / PNG / WebP / PDF，单个文件不超过 5MB',
      uploadAreaText: '点击选择或拖拽文件到此处',
      uploading: '上传中 {percent}%',
      uploadFailed: '「{name}」上传失败',
      removeAttachment: '移除',
      fileTooLarge: '「{name}」超过 5MB 大小限制',
      fileTypeUnsupported: '「{name}」不是支持的文件类型',
      tooManyFiles: '附件最多 {max} 个',
      submit: '提交申请',
      submitting: '提交中...',
      attachmentsUploading: '附件上传中，请稍候…',
      submitSuccess: '申请已提交，请等待管理员审核',
      submitFailed: '提交申请失败，请稍后重试'
    },

    list: {
      title: '我的申请',
      empty: '还没有申请记录',
      emptyHint: '提交首次申请后，这里会显示审核进度',
      submittedAt: '提交时间：{time}',
      reviewedAt: '审核时间：{time}',
      descriptionLabel: '申请说明',
      reviewNotesLabel: '审核备注',
      noReviewNotes: '暂无审核备注',
      rewardGranted: '已发放余额',
      attachmentsLabel: '附件（{count}）',
      download: '下载',
      downloading: '下载中…',
      downloadFailed: '附件下载失败',
      expand: '展开',
      collapse: '收起'
    },

    status: {
      pending: '待审核',
      approved: '已通过',
      rejected: '已驳回'
    },

    loadFailed: '加载申请列表失败'
  }
}

export default {
  rewardGrants: {
    title: '奖励发放记录',
    description: '系统奖励（学生认证等）的发放流水，只读',

    filters: {
      userId: '用户 ID',
      userIdPlaceholder: '精确用户 ID',
      campaign: '活动',
      campaignPlaceholder: '精确活动标识',
      sourceType: '来源',
      sourceTypeAll: '全部来源'
    },

    sourceType: {
      student_verification: '学生认证'
    },

    columns: {
      time: '发放时间',
      user: '用户',
      amount: '金额',
      sourceType: '来源类型',
      campaign: '活动',
      grantedBy: '发放人',
      idempotencyKey: '幂等键'
    },

    grantedBySystem: '系统自动',
    empty: '暂无发放记录',
    emptyHint: '当前筛选条件下没有发放记录',
    loadFailed: '加载发放记录失败'
  }
}

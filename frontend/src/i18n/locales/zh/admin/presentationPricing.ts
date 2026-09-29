/**
 * 模型展示价格（Presentation Pricing）管理 i18n。
 *
 * 边界：本页只影响用户端「模型与价格」页面的标准价格展示，不影响任何真实计费。
 * 所有文案反复强调这一点，防止管理员误把展示价当真实扣费。
 */
export default {
  presentationPricing: {
    searchPlaceholder: '搜索模型...',
    headerNote: '此处仅影响用户端「模型与价格」页面展示，不影响实际 API 计费',
    warning:
      '修改展示价格不会改变用户实际 Billing。真实扣费仍由现有计费系统（渠道定价、分组倍率、订阅规则）决定。',
    loadFailed: '加载模型展示价格失败',
    plazaUnavailable: '模型广场未启用，无法枚举可配置的模型。请先在系统设置中启用模型广场。',
    empty: '暂无可配置的模型',
    noSearchResult: '没有匹配的模型',
    saved: '展示价格已保存，用户端「模型与价格」页面立即生效',
    cleared: '已恢复默认（清除自定义展示价）',
    saveFailed: '保存展示价格失败',
    disabledBadge: '已停用',
    edit: '编辑',
    billingManagedByBilling: '由现有计费系统决定',
    perImage: '张',
    perRequest: '次',
    table: {
      model: '模型',
      displayPrice: '当前展示价（输入/输出）',
      source: '来源',
      officialPrice: '官方参考价',
      billingNote: '实际计费'
    },
    source: {
      manual: '自定义展示价',
      official: '官方价回退',
      billing: '计费价回退',
      none: '价格暂未公布'
    },
    editor: {
      displaySection: '用户展示价格',
      displaySectionHint: '保存后展示在用户端「模型与价格」页面；清空并停用即回退默认来源',
      enableOverride: '启用自定义展示价',
      inputPrice: '输入',
      outputPrice: '输出',
      cacheWritePrice: '缓存写入 (5m)',
      cacheWrite1hPrice: '缓存写入 (1h)',
      cacheReadPrice: '缓存读取',
      perRequestPrice: '单次价格',
      remark: '备注（仅管理员可见）',
      referenceSection: '参考信息（只读）',
      resolvedDisplay: '用户最终看到',
      restoreDefault: '恢复默认',
      save: '保存展示价格',
      saveHint: '修改展示价格不会改变用户实际 Billing',
      priceRequired: '请至少填写一个价格'
    }
  }
}

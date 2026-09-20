export default {
  chat: {
    title: '网页对话',
    sidebar: {
      newChat: '新对话',
      history: '历史会话',
      empty: '暂无历史会话',
      delete: '删除会话',
      deleteConfirm: '确定删除这个会话吗？',
      collapse: '收起会话列表',
      expand: '展开会话列表',
      localOnly: '对话保存在本浏览器',
    },
    welcome: {
      title: '有什么可以帮忙的？',
      hint: '我是本站 AI 助手，可以回答问题、协助写作与翻译。内容仅供参考，请注意甄别。',
    },
    input: {
      placeholder: '输入消息，Enter 发送，Shift+Enter 换行',
      send: '发送',
      stop: '停止生成',
    },
    modelPicker: {
      title: '选择模型',
      searchPlaceholder: '搜索模型名称或厂商',
      apiOnlyBadge: '仅限 API',
      noResults: '没有匹配的模型',
      modelCount: '{count} 个模型',
    },
    message: {
      copy: '复制',
      copied: '已复制',
      copyFailed: '复制失败',
      regenerate: '重新生成',
      retry: '重试',
    },
    error: {
      responseFailed: '回复生成失败',
      emptyResponse: '模型没有返回内容，请重试',
      stopped: '已停止生成',
    },
    state: {
      loading: '正在加载网页对话…',
      disabledTitle: '网页对话功能暂未开启',
      disabledDescription: '管理员尚未开启网页对话功能。开启后即可在浏览器中直接与可用模型对话。',
      loadFailedTitle: '网页对话加载失败',
      loadFailedDescription: '无法获取网页对话配置，请检查网络连接后重试。',
      loadFailedRetry: '重新加载',
      noModels: '暂无可用模型',
      noModelsHint: '当前没有可在网页中使用的对话模型，请联系管理员。',
    },
  },
}

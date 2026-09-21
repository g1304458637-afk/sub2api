export default {
  music: {
    title: '音乐合成',
    subtitle: '描述你想要的音乐，选择模型，一键生成乐曲。',
    saveHint: '音乐生成后请及时保存，刷新页面后生成结果将不会保留。',
    loadFailed: '音乐合成配置加载失败，请稍后刷新重试',
    generateFailed: '音乐生成失败，请稍后重试',
    timeout: '生成超时，请稍后重试',
    empty: {
      title: '音乐合成功能暂未开放',
      description: '管理员需在系统设置中配置音乐模型，并确保平台已接入音乐生成上游账号。',
    },
    form: {
      model: '模型',
      modelPlaceholder: '选择音乐模型',
      prompt: '提示词',
      promptPlaceholder: '描述你想要的音乐，例如：轻快的钢琴曲，适合清晨骑行…',
      promptRequired: '请先输入提示词',
      generate: '生成音乐',
      generating: '生成中…',
      advanced: '高级选项',
      advancedHide: '收起高级选项',
      lyrics: '歌词',
      lyricsPlaceholder: '可选：输入歌词，留空则由模型自由发挥',
      instrumental: '纯音乐',
    },
    results: {
      title: '生成结果',
      download: '下载音频',
    },
    status: {
      pending: '排队中…',
      running: '生成中…',
      succeeded: '已完成',
      failed: '生成失败',
    },
    elapsed: '已等待 {seconds} 秒',
  },
}

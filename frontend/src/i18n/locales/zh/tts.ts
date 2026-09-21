export default {
  tts: {
    title: '语音合成',
    subtitle: '输入文本，选择语音模型，一键合成语音。',
    saveHint: '音频生成后请及时保存，刷新页面后合成结果将不会保留。',
    loadFailed: '语音合成配置加载失败，请稍后刷新重试',
    generateFailed: '语音合成失败，请稍后重试',
    empty: {
      title: '语音合成功能暂未开放',
      description: '管理员需在系统设置中配置语音模型，并确保平台已接入语音合成上游账号。',
    },
    form: {
      model: '模型',
      modelPlaceholder: '选择语音模型',
      prompt: '文本',
      promptPlaceholder: '输入要合成为语音的文本…',
      promptRequired: '请先输入文本',
      generate: '生成语音',
      generating: '生成中…',
      charCount: '{count} 个字符',
    },
    results: {
      title: '合成结果',
      download: '下载音频',
    },
  },
}

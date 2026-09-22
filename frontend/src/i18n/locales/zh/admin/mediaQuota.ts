export default {
  mediaQuota: {
    title: '媒体额度',
    description: '图片生成额度与音乐/语音按量计费监控',
    refresh: '刷新',
    loading: '加载中…',
    loadFailed: '加载媒体额度数据失败',

    summary: {
      todayTotal: '三媒体今日合计成本',
      image: '图片',
      music: '音乐',
      audio: '语音'
    },

    billing: {
      quota: '独立额度',
      payAsYouGo: '按量计费',
      noQuota: '按量计费，无独立额度'
    },

    image: {
      title: '图片生成'
    },

    music: {
      title: '音乐生成',
      todayTracks: '今日生成首数',
      todayCost: '今日成本'
    },

    audio: {
      title: '语音',
      todayRequests: '今日请求数',
      todayCost: '今日成本'
    },

    quota: {
      used: '已用 / 上限',
      remaining: '剩余',
      resetIn: '窗口重置倒计时',
      resetExpired: '即将重置',
      windowNote: '额度为 {hours} 小时滚动窗口',
      windowDefault: '额度为 3 小时滚动窗口',
      busy: '生成中',
      idle: '空闲',
      auth_ok: '代理鉴权正常',
      auth_bad: '代理鉴权失败',
      auth_unknown: '代理鉴权未知',
      version: '版本',
      latency: '延迟',
      unconfigured: '未配置 MUC_IMAGE_BRIDGE_URL',
      unconfiguredHint: '在服务端环境变量中设置 MUC_IMAGE_BRIDGE_URL 后即可启用图片额度监控',
      unreachable: '图片代理不可达',
      bridgeRecent: '代理最近请求'
    },

    pricing: {
      title: '定价',
      image1k: '1024×1024（1K）',
      image2k: '2048×2048（2K）',
      image4k: '4096×4096（4K）',
      musicPerTrack: '每首音乐',
      audioTts: 'TTS（每百万字符）',
      audioRealtime: 'Realtime（每分钟）',
      audioStt: 'STT（每小时）',
      defaultPrice: '默认价',
      defaultMusicPrice: '默认 $0.50/首',
      unavailable: '暂无 openai 分组定价数据'
    },

    recent: {
      title: '最近记录',
      empty: '暂无记录',
      time: '时间',
      duration: '耗时',
      count: '张数',
      size: '尺寸',
      cost: '成本',
      model: '模型',
      user: '用户',
      status: '状态'
    }
  }
}

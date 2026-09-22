export default {
  mediaQuota: {
    title: 'Media Quota',
    description: 'Image generation quota and pay-as-you-go billing for music/audio',
    refresh: 'Refresh',
    loading: 'Loading…',
    loadFailed: 'Failed to load media quota data',

    summary: {
      todayTotal: "Today's total cost across three media",
      image: 'Image',
      music: 'Music',
      audio: 'Audio'
    },

    billing: {
      quota: 'Dedicated quota',
      payAsYouGo: 'Pay as you go',
      noQuota: 'Billed per use, no dedicated quota'
    },

    image: {
      title: 'Image Generation'
    },

    music: {
      title: 'Music Generation',
      todayTracks: 'Tracks generated today',
      todayCost: "Today's cost"
    },

    audio: {
      title: 'Audio',
      todayRequests: 'Requests today',
      todayCost: "Today's cost"
    },

    quota: {
      used: 'Used / Limit',
      remaining: 'Remaining',
      resetIn: 'Window resets in',
      resetExpired: 'Resetting soon',
      windowNote: 'Quota is a rolling {hours}-hour window',
      windowDefault: 'Quota is a rolling 3-hour window',
      busy: 'Generating',
      idle: 'Idle',
      auth_ok: 'Proxy auth OK',
      auth_bad: 'Proxy auth failed',
      auth_unknown: 'Proxy auth unknown',
      version: 'Version',
      latency: 'Latency',
      unconfigured: 'MUC_IMAGE_BRIDGE_URL is not configured',
      unconfiguredHint: 'Set MUC_IMAGE_BRIDGE_URL in the server environment to enable image quota monitoring',
      unreachable: 'Image proxy unreachable',
      bridgeRecent: 'Recent proxy requests'
    },

    pricing: {
      title: 'Pricing',
      image1k: '1024×1024 (1K)',
      image2k: '2048×2048 (2K)',
      image4k: '4096×4096 (4K)',
      musicPerTrack: 'Per track',
      audioTts: 'TTS (per million chars)',
      audioRealtime: 'Realtime (per minute)',
      audioStt: 'STT (per hour)',
      defaultPrice: 'Default price',
      defaultMusicPrice: 'Default $0.50/track',
      unavailable: 'No openai group pricing available'
    },

    recent: {
      title: 'Recent Records',
      empty: 'No records yet',
      time: 'Time',
      duration: 'Duration',
      count: 'Count',
      size: 'Size',
      cost: 'Cost',
      model: 'Model',
      user: 'User',
      status: 'Status'
    }
  }
}

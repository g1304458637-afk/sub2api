export default {
  music: {
    title: 'Music',
    subtitle: 'Describe the music you want, pick a model, and generate a track in one click.',
    saveHint: 'Save your tracks promptly — generated results are not kept after a page refresh.',
    loadFailed: 'Failed to load music configuration. Please refresh and try again later.',
    generateFailed: 'Music generation failed. Please try again later.',
    timeout: 'Generation timed out. Please try again later.',
    empty: {
      title: 'Music generation is not available yet',
      description:
        'An administrator needs to configure music models in system settings and make sure the platform is connected to a music generation upstream.',
    },
    form: {
      model: 'Model',
      modelPlaceholder: 'Select a music model',
      prompt: 'Prompt',
      promptPlaceholder: 'Describe the music you want, e.g. an upbeat piano piece for a morning ride…',
      promptRequired: 'Please enter a prompt first',
      generate: 'Generate Music',
      generating: 'Generating…',
      advanced: 'Advanced options',
      advancedHide: 'Hide advanced options',
      lyrics: 'Lyrics',
      lyricsPlaceholder: 'Optional: enter lyrics; leave empty to let the model improvise',
      instrumental: 'Instrumental only',
    },
    results: {
      title: 'Results',
      download: 'Download audio',
    },
    status: {
      pending: 'Queued…',
      running: 'Generating…',
      succeeded: 'Done',
      failed: 'Generation failed',
    },
    elapsed: 'waited {seconds}s',
  },
}

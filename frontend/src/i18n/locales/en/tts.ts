export default {
  tts: {
    title: 'Text to Speech',
    subtitle: 'Enter your text, pick a voice model, and generate speech in one click.',
    saveHint: 'Save your audio promptly — generated results are not kept after a page refresh.',
    loadFailed: 'Failed to load text-to-speech configuration. Please refresh and try again later.',
    generateFailed: 'Speech synthesis failed. Please try again later.',
    empty: {
      title: 'Text to speech is not available yet',
      description:
        'An administrator needs to configure TTS models in system settings and make sure the platform is connected to a speech synthesis upstream.',
    },
    form: {
      model: 'Model',
      modelPlaceholder: 'Select a TTS model',
      prompt: 'Text',
      promptPlaceholder: 'Enter the text you want to turn into speech…',
      promptRequired: 'Please enter some text first',
      generate: 'Generate Speech',
      generating: 'Generating…',
      charCount: '{count} characters',
    },
    results: {
      title: 'Results',
      download: 'Download audio',
    },
  },
}

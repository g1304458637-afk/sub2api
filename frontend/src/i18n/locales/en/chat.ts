export default {
  chat: {
    title: 'Web Chat',
    sidebar: {
      newChat: 'New chat',
      history: 'History',
      empty: 'No conversations yet',
      delete: 'Delete conversation',
      deleteConfirm: 'Delete this conversation?',
      collapse: 'Collapse sidebar',
      expand: 'Expand sidebar',
      localOnly: 'Conversations are stored in this browser',
    },
    welcome: {
      motto: 'Minzu University of China',
      title: 'How can I help you today?',
      hint: 'I am this site\'s AI assistant. I can answer questions and help with writing and translation. Responses are for reference only.',
      suggestion1: 'Draft a study plan',
      suggestion2: 'Explain a concept',
      suggestion3: 'Polish my writing',
    },
    input: {
      placeholder: 'Type a message. Enter to send, Shift+Enter for a new line',
      send: 'Send',
      stop: 'Stop generating',
    },
    modelPicker: {
      title: 'Choose a model',
      searchPlaceholder: 'Search by name or vendor',
      apiOnlyBadge: 'API only',
      noResults: 'No matching models',
      modelCount: '{count} models',
    },
    message: {
      copy: 'Copy',
      copied: 'Copied',
      copyFailed: 'Copy failed',
      regenerate: 'Regenerate',
      retry: 'Retry',
    },
    error: {
      responseFailed: 'Failed to generate a response',
      emptyResponse: 'The model returned no content. Please try again',
      stopped: 'Generation stopped',
    },
    state: {
      loading: 'Loading web chat…',
      disabledTitle: 'Web chat is not available yet',
      disabledDescription:
        'The administrator has not enabled web chat. Once enabled, you can chat with available models right in the browser.',
      loadFailedTitle: 'Failed to load web chat',
      loadFailedDescription: 'Could not fetch the web chat configuration. Check your network and try again.',
      loadFailedRetry: 'Reload',
      noModels: 'No models available',
      noModelsHint: 'There are currently no chat models available on the web. Please contact the administrator.',
    },
  },
}

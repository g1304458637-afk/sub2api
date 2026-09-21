/**
 * Web Chat Store
 * Module-level singleton holding the web chat feature config, with in-flight
 * request deduplication and success caching; on failure config stays null so
 * callers decide how to degrade the UI.
 *
 * Deliberately NOT a Pinia store: consumers read the state both as refs
 * (`store.config.value`, e.g. the draw page) and as plain properties
 * (`store.config?.enabled`, e.g. sidebar feature flags). A reactive/Pinia
 * store would auto-unwrap the refs and break the first shape.
 */

import { ref } from 'vue'
import type { Ref } from 'vue'
import { fetchWebChatConfig } from '@/api/webChat'
import type { WebChatConfig } from '@/api/webChat'

/**
 * A ref that also forwards unknown property reads to its current `.value`.
 *
 * Because `ref()` wraps object values in a reactive proxy, forwarded reads
 * still track dependencies and stay reactive.
 */
function createForwardingRef<T extends object>(source: Ref<T | null>): Ref<T | null> {
  return new Proxy(source, {
    get(target, prop, receiver) {
      if (typeof prop === 'symbol' || prop === 'value' || prop in target) {
        return Reflect.get(target, prop, receiver)
      }
      const current: unknown = target.value
      if (current !== null && typeof current === 'object') {
        return Reflect.get(current as object, prop)
      }
      return undefined
    },
  })
}

/**
 * Ref to the config, readable both as `config.value` and `config.enabled`.
 * Structurally assignable to `Ref<WebChatConfig | null>`.
 */
type WebChatConfigRef = Ref<WebChatConfig | null> & Partial<WebChatConfig>

// ==================== Module-level singleton state ====================

const configSource = ref<WebChatConfig | null>(null)
const config: WebChatConfigRef = createForwardingRef(configSource)
const loaded = ref(false)

// In-flight request shared by all callers (dedup)
let configRequest: Promise<void> | null = null

// ==================== Store ====================

export function useWebChatStore(): {
  config: WebChatConfigRef
  loaded: Ref<boolean>
  loadConfig: (force?: boolean) => Promise<void>
} {
  /**
   * Fetch the web chat config (uses cache unless force=true).
   * Never rejects: on failure config stays null and loaded stays false, so
   * the caller decides how to degrade; a later call retries.
   */
  function loadConfig(force = false): Promise<void> {
    // Share the result between concurrent callers (in-flight dedup)
    if (configRequest) {
      return configRequest
    }

    // Already loaded successfully and no forced refresh requested
    if (loaded.value && !force) {
      return Promise.resolve()
    }

    const request = fetchWebChatConfig()
      .then((data) => {
        configSource.value = data
        loaded.value = true
      })
      .catch((error: unknown) => {
        console.error('Failed to load web chat config:', error)
      })
      .finally(() => {
        if (configRequest === request) {
          configRequest = null
        }
      })

    configRequest = request
    return request
  }

  return {
    config,
    loaded,
    loadConfig,
  }
}

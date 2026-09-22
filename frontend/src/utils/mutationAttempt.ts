/** Keep a logical mutation's key through transport errors; clear only on success. */
export function createMutationAttempt(prefix: string) {
  let fingerprint = ''
  let key = ''
  return {
    keyFor(payload: unknown): string {
      const next = JSON.stringify(payload)
      if (!key || next !== fingerprint) {
        fingerprint = next
        key = `${prefix}-${globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`}`
      }
      return key
    },
    clear() { fingerprint = ''; key = '' }
  }
}

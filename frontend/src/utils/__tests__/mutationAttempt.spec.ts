import { describe, it, expect } from 'vitest'
import { createMutationAttempt } from '../mutationAttempt'
describe('mutation retry identity', () => {
  it('reuses a key after a lost response and starts a new operation after success or payload change', () => {
    const attempt = createMutationAttempt('reset')
    const key = attempt.keyFor({ user: 1, quantity: 2 })
    expect(attempt.keyFor({ user: 1, quantity: 2 })).toBe(key)
    expect(attempt.keyFor({ user: 2, quantity: 2 })).not.toBe(key)
    const second = attempt.keyFor({ user: 2, quantity: 2 })
    attempt.clear()
    expect(attempt.keyFor({ user: 2, quantity: 2 })).not.toBe(second)
  })
})

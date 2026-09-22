import { beforeEach, describe, expect, it, vi } from 'vitest'

const { del } = vi.hoisted(() => ({ del: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: { delete: del } }))

import { revokeEducationEmail } from '@/api/admin/users'

describe('revokeEducationEmail', () => {
  beforeEach(() => {
    del.mockReset()
  })

  it('hits the DELETE education-email endpoint and returns the payload', async () => {
    const data = { user_id: 5, revoked_count: 2 }
    del.mockResolvedValue({ data })
    await expect(revokeEducationEmail(5)).resolves.toBe(data)
    expect(del).toHaveBeenCalledWith('/admin/users/5/education-email')
  })

  it('propagates client errors', async () => {
    del.mockRejectedValue(new Error('boom'))
    await expect(revokeEducationEmail(7)).rejects.toThrow('boom')
  })
})

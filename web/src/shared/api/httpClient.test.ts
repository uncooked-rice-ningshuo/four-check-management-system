import { afterEach, describe, expect, it, vi } from 'vitest'
import { apiRequest, setAccessToken } from './httpClient'

afterEach(() => { vi.unstubAllGlobals(); setAccessToken(null) })

describe('API error contract', () => {
  it.each(['current', 'legacy'])('preserves %s field validation errors', async version => {
    const details = [{ path: 'details.studentId', code: 'REQUIRED', message: '请选择学生' }]
    const error = { code: 'VALIDATION_FAILED', message: '请核对检查明细', details }
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify(version === 'legacy' ? { error } : error), { status: 422 })))
    await expect(apiRequest('/inspections')).rejects.toMatchObject({ status: 422, code: 'VALIDATION_FAILED', message: '请核对检查明细', details })
  })

  it.each([null, [], { code: 123, message: { raw: 'internal' } }, { error: 'invalid' }])('handles malformed error bodies safely: %j', async body => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify(body), { status: 500 })))
    await expect(apiRequest('/inspections')).rejects.toMatchObject({ status: 500, code: 'REQUEST_FAILED', message: '请求失败' })
  })

  it('ignores malformed field issues without discarding the actionable error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ code: 'VALIDATION_FAILED', message: '请核对输入', details: [null, { path: 3, code: 'INVALID' }] }), { status: 422 })))
    await expect(apiRequest('/inspections')).rejects.toMatchObject({ code: 'VALIDATION_FAILED', message: '请核对输入', details: [] })
  })
})

import { afterEach, describe, expect, it, vi } from 'vitest'
import { apiDownload, apiRequest, refreshAccessToken, setAccessToken, subscribeSessionExpired } from './httpClient'

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status })
const unauthorized = () => json({ code: 'UNAUTHORIZED', message: '登录状态已失效' }, 401)
function deferred<T>() {
  let resolve: (value: T) => void = () => { throw new Error('Deferred resolver not initialized') }
  const promise = new Promise<T>(done => { resolve = done })
  return { promise, resolve }
}
afterEach(() => { setAccessToken(null); vi.unstubAllGlobals() })

describe('long-lived session renewal', () => {
  it('clears an evicted device without attempting token rotation', async () => {
    setAccessToken('evicted-device')
    const reasons: string[] = []
    const unsubscribe = subscribeSessionExpired(reason => { reasons.push(reason) })
    const fetchMock = vi.fn(async () => json({ code: 'SESSION_EVICTED', message: '当前设备已退出' }, 401))
    vi.stubGlobal('fetch', fetchMock)
    await expect(apiRequest('/work-context')).rejects.toMatchObject({ code: 'SESSION_EVICTED', status: 401 })
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(reasons).toEqual(['evicted'])
    unsubscribe()
  })
  it('renews once and retries the work week without a page reload', async () => {
    setAccessToken('expired')
    const fetchMock = vi.fn(async (url: string, init: RequestInit) => {
      if (url.endsWith('/auth/refresh')) return json({ accessToken: 'renewed' })
      return new Headers(init.headers).get('Authorization') === 'Bearer renewed' ? json({ version: 1 }) : unauthorized()
    })
    vi.stubGlobal('fetch', fetchMock)
    await expect(apiRequest('/work-context')).resolves.toEqual({ version: 1 })
    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual(['/api/v1/work-context', '/api/v1/auth/refresh', '/api/v1/work-context'])
    expect(fetchMock.mock.calls[1][1]).toMatchObject({ method: 'POST', credentials: 'include' })
  })

  it('shares one rotation across concurrent requests and a late old-token 401', async () => {
    setAccessToken('expired')
    const late = deferred<Response>()
    const fetchMock = vi.fn(async (url: string, init: RequestInit) => {
      if (url.endsWith('/auth/refresh')) return json({ accessToken: 'renewed' })
      if (new Headers(init.headers).get('Authorization') === 'Bearer renewed') return json({ ok: true })
      return url.endsWith('/late') ? late.promise : unauthorized()
    })
    vi.stubGlobal('fetch', fetchMock)
    const requests = Promise.all([apiRequest('/work-context'), apiRequest('/dashboard'), apiRequest('/late')]).catch(error => error)
    await vi.waitFor(() => expect(fetchMock.mock.calls.filter(([url, init]) => !url.endsWith('/auth/refresh') && new Headers(init.headers).get('Authorization') === 'Bearer renewed')).toHaveLength(2))
    late.resolve(unauthorized())
    await expect(requests).resolves.toHaveLength(3)
    expect(fetchMock.mock.calls.filter(([url]) => url.endsWith('/auth/refresh'))).toHaveLength(1)
  })

  it('does not loop if the renewed credential is rejected', async () => {
    setAccessToken('expired')
    const fetchMock = vi.fn(async (url: string) => url.endsWith('/auth/refresh') ? json({ accessToken: 'renewed' }) : unauthorized())
    vi.stubGlobal('fetch', fetchMock)
    await expect(apiRequest('/work-context')).rejects.toMatchObject({ status: 401 })
    expect(fetchMock).toHaveBeenCalledTimes(3)
    await expect(apiRequest('/work-context')).rejects.toMatchObject({ status: 401 })
    expect(fetchMock.mock.calls.filter(([url]) => url.endsWith('/auth/refresh'))).toHaveLength(1)
  })

  it.each([401, 500])('does not loop on a refresh %s; temporary failure remains retryable', async status => {
    setAccessToken('expired')
    let recovered = false
    const fetchMock = vi.fn(async (url: string, init: RequestInit) => {
      if (url.endsWith('/auth/refresh')) return recovered ? json({ accessToken: 'renewed' }) : json({ code: status === 401 ? 'INVALID_SESSION' : 'AUTH_SERVICE_UNAVAILABLE' }, status)
      return new Headers(init.headers).get('Authorization') === 'Bearer renewed' ? json({ ok: true }) : unauthorized()
    })
    vi.stubGlobal('fetch', fetchMock)
    await expect(apiRequest('/work-context')).rejects.toMatchObject({ status })
    expect(fetchMock).toHaveBeenCalledTimes(2)
    recovered = true
    if (status === 500) await expect(apiRequest('/work-context')).resolves.toEqual({ ok: true })
    else {
      await expect(apiRequest('/work-context')).rejects.toMatchObject({ status: 401 })
      expect(fetchMock.mock.calls.filter(([url]) => url.endsWith('/auth/refresh'))).toHaveLength(1)
    }
  })

  it.each(['/auth/login', '/auth/register', '/auth/refresh', '/auth/logout'])('never renews recursively for %s', async path => {
    setAccessToken('expired')
    const fetchMock = vi.fn(async () => unauthorized())
    vi.stubGlobal('fetch', fetchMock)
    await expect(apiRequest(path, { method: 'POST' })).rejects.toMatchObject({ status: 401 })
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it.each([403, 500])('does not renew non-authentication failures (%s)', async status => {
    setAccessToken('expired')
    const fetchMock = vi.fn(async () => json({ code: 'FAILED' }, status))
    vi.stubGlobal('fetch', fetchMock)
    await expect(apiRequest('/work-context')).rejects.toMatchObject({ status })
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('preserves upload body and idempotency key when retrying a rejected request', async () => {
    setAccessToken('expired')
    const body = new FormData()
    body.append('file', new Blob(['synthetic']), 'fixture.txt')
    const fetchMock = vi.fn(async (url: string, init: RequestInit) => {
      if (url.endsWith('/auth/refresh')) return json({ accessToken: 'renewed' })
      return new Headers(init.headers).get('Authorization') === 'Bearer renewed' ? json({ ok: true }) : unauthorized()
    })
    vi.stubGlobal('fetch', fetchMock)
    await apiRequest('/imports', { method: 'POST', body, headers: { 'Idempotency-Key': 'same-operation' } })
    const calls = fetchMock.mock.calls.filter(([url]) => url.endsWith('/imports'))
    expect(calls).toHaveLength(2)
    for (const [, init] of calls) {
      expect(init.body).toBe(body)
      expect(new Headers(init.headers).get('Idempotency-Key')).toBe('same-operation')
      expect(new Headers(init.headers).has('Content-Type')).toBe(false)
    }
  })

  it('renews downloads and preserves response headers', async () => {
    setAccessToken('expired')
    vi.stubGlobal('fetch', vi.fn(async (url: string, init: RequestInit) => {
      if (url.endsWith('/auth/refresh')) return json({ accessToken: 'renewed' })
      return new Headers(init.headers).get('Authorization') === 'Bearer renewed' ? new Response('file', { headers: { 'Content-Disposition': 'attachment; filename=test.txt' } }) : unauthorized()
    }))
    expect((await apiDownload('/files/test')).headers.get('Content-Disposition')).toContain('test.txt')
  })

  it.each([null, 'another-account'])('discards late renewal after the session changes to %s', async next => {
    setAccessToken('expired')
    const rotation = deferred<Response>()
    const fetchMock = vi.fn(async (url: string) => url.endsWith('/auth/refresh') ? rotation.promise : unauthorized())
    vi.stubGlobal('fetch', fetchMock)
    const pending = apiRequest('/work-context').catch(error => error)
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2))
    setAccessToken(next)
    rotation.resolve(json({ accessToken: 'stale-renewal' }))
    expect(await pending).toMatchObject({ code: 'SESSION_CHANGED' })
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('does not replay a previous account request after switching accounts', async () => {
    setAccessToken('previous-account')
    const response = deferred<Response>()
    const fetchMock = vi.fn(() => response.promise)
    vi.stubGlobal('fetch', fetchMock)
    const pending = apiRequest('/work-context').catch(error => error)
    setAccessToken('another-account')
    response.resolve(unauthorized())
    expect(await pending).toMatchObject({ code: 'SESSION_CHANGED' })
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('shares the same rotation with startup session recovery', async () => {
    setAccessToken('expired')
    const rotation = deferred<Response>()
    const fetchMock = vi.fn(async (url: string, init: RequestInit) => {
      if (url.endsWith('/auth/refresh')) return rotation.promise
      return new Headers(init.headers).get('Authorization') === 'Bearer renewed' ? json({ ok: true }) : unauthorized()
    })
    vi.stubGlobal('fetch', fetchMock)
    const startup = refreshAccessToken()
    const request = apiRequest('/work-context')
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2))
    rotation.resolve(json({ accessToken: 'renewed' }))
    await startup
    await expect(request).resolves.toEqual({ ok: true })
    expect(fetchMock.mock.calls.filter(([url]) => url.endsWith('/auth/refresh'))).toHaveLength(1)
  })

  it('preserves the session on network failure and can recover on a later retry', async () => {
    setAccessToken('expired')
    let offline = true
    vi.stubGlobal('fetch', vi.fn(async (url: string, init: RequestInit) => {
      if (url.endsWith('/auth/refresh')) {
        if (offline) throw new TypeError('Network failure')
        return json({ accessToken: 'renewed' })
      }
      return new Headers(init.headers).get('Authorization') === 'Bearer renewed' ? json({ ok: true }) : unauthorized()
    }))
    await expect(apiRequest('/work-context')).rejects.toThrow('Network failure')
    offline = false
    await expect(apiRequest('/work-context')).resolves.toEqual({ ok: true })
  })

  it('rejects malformed renewal responses without sending an undefined credential', async () => {
    setAccessToken('expired')
    const fetchMock = vi.fn(async (url: string) => url.endsWith('/auth/refresh') ? json({}) : unauthorized())
    vi.stubGlobal('fetch', fetchMock)
    await expect(apiRequest('/work-context')).rejects.toMatchObject({ code: 'INVALID_AUTH_RESPONSE' })
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('does not retry an aborted request after a shared renewal', async () => {
    setAccessToken('expired')
    const rotation = deferred<Response>()
    const fetchMock = vi.fn(async (url: string) => url.endsWith('/auth/refresh') ? rotation.promise : unauthorized())
    vi.stubGlobal('fetch', fetchMock)
    const controller = new AbortController()
    const pending = apiRequest('/work-context', { signal: controller.signal }).catch(error => error)
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2))
    controller.abort()
    rotation.resolve(json({ accessToken: 'renewed' }))
    expect(await pending).toMatchObject({ name: 'AbortError' })
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })
})

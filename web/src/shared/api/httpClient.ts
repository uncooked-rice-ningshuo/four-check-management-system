// Access Token 只保存在内存中，避免使用 localStorage 扩大脚本注入后的令牌暴露窗口。
let accessToken: string | null = null
// A session boundary prevents an old request from being replayed as a different account.
let sessionGeneration = 0
let refreshInFlight: { generation: number; promise: Promise<void> } | null = null
const sessionExpiredListeners = new Set<(reason: 'expired' | 'evicted') => void>()

// ApiErrorBody 对应 Go API 返回的稳定错误结构。
export type ApiErrorDetail = { path: string; code: string; message: string }
export type ApiErrorBody = { code?: string; message?: string; details?: ApiErrorDetail[] }

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}
function isDetail(value: unknown): value is ApiErrorDetail {
  return isObject(value) && typeof value.path === 'string' && typeof value.code === 'string' && typeof value.message === 'string'
}
function errorBody(value: unknown): ApiErrorBody {
  if (!isObject(value)) return {}
  // 兼容旧服务的嵌套错误，避免前后端滚动更新时丢失字段提示。
  const body = isObject(value.error) ? value.error : value
  return {
    code: typeof body.code === 'string' && body.code.trim() ? body.code : undefined,
    message: typeof body.message === 'string' && body.message.trim() ? body.message : undefined,
    details: Array.isArray(body.details) ? body.details.filter(isDetail) : [],
  }
}

// ApiError 保留机器可读错误码，同时避免页面直接依赖底层传输细节。
export class ApiError extends Error {
  constructor(public readonly status: number, public readonly code: string, message: string, public readonly details: ApiErrorDetail[] = []) {
    super(message)
  }
}

// setAccessToken 在登录、改密或退出后更新凭证，并建立新的会话边界。
export function setAccessToken(token: string | null) {
  accessToken = token
  sessionGeneration += 1
}

// subscribeSessionExpired keeps React state and private query caches in sync with authentication.
export function subscribeSessionExpired(listener: (reason: 'expired' | 'evicted') => void) {
  sessionExpiredListeners.add(listener)
  return () => { sessionExpiredListeners.delete(listener) }
}

function assertSameSession(generation: number) {
  if (generation !== sessionGeneration) throw new ApiError(401, 'SESSION_CHANGED', '登录状态已变化，请重试')
}

function expireSession(generation: number, reason: 'expired' | 'evicted' = 'expired') {
  if (generation !== sessionGeneration) return
  setAccessToken(null)
  for (const listener of sessionExpiredListeners) listener(reason)
}

// apiRequest 是前端唯一的 HTTP 传输入口，负责附加内存令牌，
// 并允许浏览器在认证请求中携带 HttpOnly 刷新 Cookie。
async function send(path: string, init: RequestInit, token: string | null) {
	const headers = new Headers(init.headers)
	if (!(init.body instanceof FormData) && init.body !== undefined && !headers.has('Content-Type')) {
		headers.set('Content-Type', 'application/json')
	}
	if (token) headers.set('Authorization', `Bearer ${token}`)
  const response = await fetch(`/api/v1${path}`, {
    ...init,
    credentials: 'include',
		headers,
  })

  if (!response.ok) {
    const body = errorBody(await response.json().catch(() => ({})))
    throw new ApiError(response.status, body.code ?? 'REQUEST_FAILED', body.message ?? '请求失败', body.details)
  }
	return response
}

// refreshAccessToken is shared by startup recovery and long-lived requests.
// Never retry the rotating Cookie request itself or bind it to one caller's abort signal.
export function refreshAccessToken(): Promise<void> {
  const generation = sessionGeneration
  if (refreshInFlight?.generation === generation) return refreshInFlight.promise
  const promise = (async () => {
    try {
      const response = await send('/auth/refresh', { method: 'POST' }, null)
      const data: unknown = await response.json()
      assertSameSession(generation)
      if (!isObject(data) || typeof data.accessToken !== 'string' || !data.accessToken.trim()) {
        throw new ApiError(502, 'INVALID_AUTH_RESPONSE', '登录续期响应异常，请重试')
      }
      // Renewal belongs to the existing session; it must not create a new account boundary.
      accessToken = data.accessToken
    } catch (error) {
      assertSameSession(generation)
      if (error instanceof ApiError && error.status === 401) expireSession(generation, error.code === 'SESSION_EVICTED' ? 'evicted' : 'expired')
      throw error
    } finally {
      if (refreshInFlight?.generation === generation) refreshInFlight = null
    }
  })()
  refreshInFlight = { generation, promise }
  return promise
}

const sessionEndpoints = new Set(['/auth/login', '/auth/register', '/auth/refresh', '/auth/logout'])

async function request(path: string, init: RequestInit) {
  const generation = sessionGeneration
  const token = accessToken
  try {
    const response = await send(path, init, token)
    assertSameSession(generation)
    return response
  } catch (error) {
    assertSameSession(generation)
	if (error instanceof ApiError && error.code === 'SESSION_EVICTED') {
		expireSession(generation, 'evicted')
		throw error
	}
    if (!(error instanceof ApiError) || error.status !== 401 || !token || sessionEndpoints.has(path)) throw error
    init.signal?.throwIfAborted()
    // A delayed 401 may belong to the credential that another request already renewed.
    if (accessToken === token) await refreshAccessToken()
    assertSameSession(generation)
    init.signal?.throwIfAborted()
    try {
      const response = await send(path, init, accessToken)
      assertSameSession(generation)
      return response
    } catch (retryError) {
      assertSameSession(generation)
      if (retryError instanceof ApiError && retryError.status === 401) expireSession(generation, retryError.code === 'SESSION_EVICTED' ? 'evicted' : 'expired')
      throw retryError
    }
  }
}

export async function apiRequest<T>(path: string, init: RequestInit = {}, responseType: 'json' | 'blob' = 'json'): Promise<T> {
	const response = await request(path, init)
	if (response.status === 204) return undefined as T
	return (responseType === 'blob' ? response.blob() : response.json()) as Promise<T>
}

export async function apiDownload(path: string) {
	const response = await request(path, {})
	return { blob: await response.blob(), headers: response.headers }
}

// Access Token 只保存在内存中，避免使用 localStorage 扩大脚本注入后的令牌暴露窗口。
let accessToken: string | null = null

// ApiErrorBody 对应 Go API 返回的稳定错误结构。
export type ApiErrorBody = { code?: string; message?: string }

// ApiError 保留机器可读错误码，同时避免页面直接依赖底层传输细节。
export class ApiError extends Error {
  constructor(public readonly status: number, public readonly code: string, message: string) {
    super(message)
  }
}

// setAccessToken 在登录、续期或退出后统一更新内存中的访问令牌。
export function setAccessToken(token: string | null) {
  accessToken = token
}

// apiRequest 是前端唯一的 HTTP 传输入口，负责附加内存令牌，
// 并允许浏览器在认证请求中携带 HttpOnly 刷新 Cookie。
async function request(path: string, init: RequestInit) {
	const headers = new Headers(init.headers)
	if (!(init.body instanceof FormData) && init.body !== undefined && !headers.has('Content-Type')) {
		headers.set('Content-Type', 'application/json')
	}
	if (accessToken) headers.set('Authorization', `Bearer ${accessToken}`)
  const response = await fetch(`/api/v1${path}`, {
    ...init,
    credentials: 'include',
		headers,
  })

  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as ApiErrorBody
    throw new ApiError(response.status, body.code ?? 'REQUEST_FAILED', body.message ?? '请求失败')
  }
	return response
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

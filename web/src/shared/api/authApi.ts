import type { User } from '../../entities/user/model/user'
import { apiRequest, setAccessToken } from './httpClient'

let sessionRefreshInFlight: Promise<User> | null = null

// registerAccount 提交待审核个人账号；部门和组织角色申请由服务端在审核通过时生效。
export async function registerAccount(input: { studentNo: string; name: string; password: string; requestedDepartmentId: string; requestedOrgRoleId: string }) {
  return apiRequest<User>('/auth/register', { method: 'POST', body: JSON.stringify(input) })
}

// loginAccount 仅在登录成功后保存短期访问令牌，并重新读取服务端账号信息，
// 避免页面把用户提交的表单字段当作可信身份。
export async function loginAccount(input: { loginName: string; password: string }) {
  const data = await apiRequest<{ accessToken: string }>('/auth/login', { method: 'POST', body: JSON.stringify(input) })
  setAccessToken(data.accessToken)
  return getCurrentUser()
}

export async function changePassword(input: { currentPassword: string; newPassword: string }) {
  await apiRequest<void>('/auth/change-password', { method: 'POST', body: JSON.stringify(input) })
  setAccessToken(null)
}

// refreshSession 在应用启动时使用 HttpOnly Cookie 恢复会话，
// 无需把 Access Token 持久化到浏览器存储。
export function refreshSession() {
  if (sessionRefreshInFlight) return sessionRefreshInFlight

  sessionRefreshInFlight = (async () => {
    const data = await apiRequest<{ accessToken: string }>('/auth/refresh', { method: 'POST' })
    setAccessToken(data.accessToken)
    return getCurrentUser()
  })().finally(() => {
    sessionRefreshInFlight = null
  })

  return sessionRefreshInFlight
}

// getCurrentUser 返回当前访问令牌对应的服务端账号信息。
export function getCurrentUser() {
  return apiRequest<User>('/auth/me')
}

// logoutAccount 先撤销服务端会话，再清除前端内存令牌。
export async function logoutAccount() {
  await apiRequest<void>('/auth/logout', { method: 'POST' })
  setAccessToken(null)
}

import { StrictMode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AuthProvider } from './AuthProvider'
import { useAuth } from './authContext'

const user = {
  id: 'admin-id',
  loginName: 'admin',
  studentNo: '',
  name: '系统管理员',
  status: 'ACTIVE',
  accountKind: 'SYSTEM',
  systemRole: 'SUPER_ADMIN',
  isSuperAdmin: true,
  mustChangePassword: false,
  primaryDepartmentId: '',
  requestedDepartmentId: '',
  requestedOrgRoleId: '',
}

function SessionState() {
  const { user: currentUser, loading } = useAuth()
  if (loading) return <span>正在恢复会话</span>
  return <span>{currentUser?.name ?? '未登录'}</span>
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('AuthProvider', () => {
  it('StrictMode 重复执行 Effect 时只恢复一次会话', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/auth/refresh')) {
        return new Response(JSON.stringify({ accessToken: 'access-token' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      if (url.endsWith('/auth/me')) {
        return new Response(JSON.stringify(user), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    render(<StrictMode><AuthProvider><SessionState /></AuthProvider></StrictMode>)

    await screen.findByText('系统管理员')
    await waitFor(() => {
      const refreshRequests = fetchMock.mock.calls.filter(([input]) => String(input).endsWith('/auth/refresh'))
      expect(refreshRequests).toHaveLength(1)
    })
  })
})

import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { LoginPage } from './LoginPage'

const signIn = vi.fn().mockResolvedValue({
  id: 'admin-id', loginName: 'admin', studentNo: '', name: '系统管理员', status: 'ACTIVE',
  accountKind: 'SYSTEM', isSuperAdmin: true, mustChangePassword: false,
  primaryDepartmentId: '', requestedDepartmentId: '', requestedOrgRoleId: '',
})

vi.mock('../../app/providers/authContext', () => ({
  useAuth: () => ({ user: null, loading: false, signIn, signOut: vi.fn() }),
}))

describe('登录后的去向', () => {
  it('超级管理员直接进入统一工作台', async () => {
    render(<MemoryRouter initialEntries={[{ pathname: '/login', state: { from: '/' } }]}><Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/admin" element={<div>旧管理概览</div>} />
      <Route path="/" element={<div>统一工作台</div>} />
    </Routes></MemoryRouter>)

    fireEvent.change(screen.getByLabelText('账号'), { target: { value: 'admin' } })
    fireEvent.change(screen.getByLabelText('密码'), { target: { value: 'AdminDemo123456' } })
    fireEvent.click(screen.getByRole('button', { name: '登 录' }))

    await waitFor(() => expect(screen.getByText('统一工作台')).toBeInTheDocument())
  })

  it('已完成首次改密后不会按旧来源再次返回改密页', async () => {
    render(<MemoryRouter initialEntries={[{ pathname: '/login', state: { from: '/change-password' } }]}><Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/change-password" element={<div>首次改密</div>} />
      <Route path="/" element={<div>统一工作台</div>} />
    </Routes></MemoryRouter>)

    fireEvent.change(screen.getByLabelText('账号'), { target: { value: 'admin' } })
    fireEvent.change(screen.getByLabelText('密码'), { target: { value: 'ReplacementPassword123' } })
    fireEvent.click(screen.getByRole('button', { name: '登 录' }))

    await waitFor(() => expect(screen.getByText('统一工作台')).toBeInTheDocument())
    expect(screen.queryByText('首次改密')).not.toBeInTheDocument()
  })
})

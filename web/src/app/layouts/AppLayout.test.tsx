import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AppLayout } from './AppLayout'

let currentUser = { id: 'member-1', name: '普通成员', loginName: 'member', systemRole: 'MEMBER', isSuperAdmin: false }
const permissionsQuery = vi.fn()

vi.mock('../providers/authContext', () => ({
  useAuth: () => ({ user: currentUser, signOut: vi.fn() }),
}))
vi.mock('../../features/attendance/hooks/useAttendancePermissions', () => ({
  useAttendancePermissions: () => permissionsQuery(),
}))

function renderLayout(path = '/') {
  return render(<MemoryRouter initialEntries={[path]}><Routes>
    <Route element={<AppLayout />}>
      <Route index element={<div>工作台内容</div>} />
      <Route path="inspections/attendance" element={<div>到课率内容</div>} />
    </Route>
  </Routes></MemoryRouter>)
}

describe('AppLayout', () => {
  beforeEach(() => {
    currentUser = { id: 'member-1', name: '普通成员', loginName: 'member', systemRole: 'MEMBER', isSuperAdmin: false }
    permissionsQuery.mockReset()
    permissionsQuery.mockReturnValue({ data: { permissions: [] }, isLoading: false })
  })

  it('gives every signed-in role the same workbench shell without administrator entries', () => {
    renderLayout()

    expect(screen.getByText('工作台内容')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: '工作台' })).toHaveAttribute('href', '/')
    expect(screen.queryByText('管理概览')).not.toBeInTheDocument()
  })

  it('uses the shared workbench as the only overview entry for administrators', () => {
    currentUser = { id: 'admin-1', name: '系统管理员', loginName: 'admin', systemRole: 'ADMIN', isSuperAdmin: false }
    renderLayout()

    expect(screen.getByRole('link', { name: '工作台' })).toHaveAttribute('href', '/')
    expect(screen.queryByRole('link', { name: '管理概览' })).not.toBeInTheDocument()
  })

  it('adds inspection navigation only when the member has an effective view grant', () => {
    permissionsQuery.mockReturnValue({ data: { permissions: [{ action: 'INSPECTION_VIEW', effect: 'ALLOW', scope: { departmentId: 'department-study' }, sources: [] }] }, isLoading: false })
    renderLayout('/inspections/attendance')

    expect(screen.getByRole('link', { name: '工作台' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: '到课率检查' })).toBeInTheDocument()
    expect(screen.getByText('到课率内容')).toBeInTheDocument()
  })
})

vi.mock('../../features/work-context/WorkContextBar', () => ({ WorkContextBar: () => null }))

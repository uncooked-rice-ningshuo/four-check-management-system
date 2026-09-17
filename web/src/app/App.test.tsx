import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App'

const permissionsQuery = vi.fn()

vi.mock('./providers/authContext', () => ({
  useAuth: () => ({
    user: { id: 'member-1', name: '普通成员', loginName: 'member', systemRole: 'MEMBER', isSuperAdmin: false, mustChangePassword: false },
    loading: false,
    signOut: vi.fn(),
  }),
}))
vi.mock('../features/attendance/hooks/useAttendancePermissions', () => ({
  useAttendancePermissions: () => permissionsQuery(),
}))
vi.mock('../features/phase-two-dashboard/model/usePhaseTwoDashboard', () => ({
	usePhaseTwoDashboard: () => ({
		isLoading: false,
		isError: false,
		data: { context: null, myTodos: { total: 0, draft: 0, returned: 0 }, departmentStatuses: [] },
	}),
}))
vi.mock('../pages/inspection/AttendancePage', () => ({ AttendancePage: () => <div>到课率业务内容</div> }))

describe('application routes', () => {
  beforeEach(() => permissionsQuery.mockReturnValue({ data: { permissions: [] }, isLoading: false }))

  it('renders the member workbench inside the unified application shell', () => {
    render(<MemoryRouter initialEntries={['/']}><App /></MemoryRouter>)

    expect(screen.getByRole('complementary')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: '工作台' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '工作台' })).toBeInTheDocument()
  })

  it('shows 403 for a direct inspection URL without an effective grant', () => {
    render(<MemoryRouter initialEntries={['/inspections/attendance']}><App /></MemoryRouter>)

    expect(screen.getByText('403')).toBeInTheDocument()
    expect(screen.queryByText('到课率业务内容')).not.toBeInTheDocument()
  })
})

vi.mock('../features/work-context/WorkContextBar', () => ({ WorkContextBar: () => null }))

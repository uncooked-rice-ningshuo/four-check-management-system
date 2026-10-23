import type { ReactElement } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render as testingRender, screen } from '@testing-library/react'
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

vi.mock('../features/course-inspection/model/queries', () => ({
 useInspectionContext: (query: {typeCode:string}) => ({data:{inspectionTypeId:query.typeCode,items:[]},isLoading:false}),
}))

it('protects a direct unattended URL from an attendance-only grant',()=>{
 permissionsQuery.mockReturnValue({data:{permissions:[{action:'INSPECTION_VIEW',effect:'ALLOW',scope:{inspectionTypeId:'ATTENDANCE_RATE'},sources:[]}]},isLoading:false})
 render(<MemoryRouter initialEntries={['/inspections/unattended-classroom']}><App/></MemoryRouter>)
 expect(screen.getByText('403')).toBeInTheDocument()
})

it('protects a direct late-return URL from an attendance-only grant',()=>{
 permissionsQuery.mockReturnValue({data:{permissions:[{action:'INSPECTION_VIEW',effect:'ALLOW',scope:{inspectionTypeId:'ATTENDANCE_RATE'},sources:[]}]},isLoading:false})
 render(<MemoryRouter initialEntries={['/inspections/late-return']}><App/></MemoryRouter>)
 expect(screen.getByText('403')).toBeInTheDocument()
})

it('protects a direct morning-attendance URL from an attendance-only grant',()=>{
 permissionsQuery.mockReturnValue({data:{permissions:[{action:'INSPECTION_VIEW',effect:'ALLOW',scope:{inspectionTypeId:'ATTENDANCE_RATE'},sources:[]}]},isLoading:false})
 render(<MemoryRouter initialEntries={['/inspections/morning-attendance']}><App/></MemoryRouter>)
 expect(screen.getByText('403')).toBeInTheDocument()
})

it('protects a direct dormitory-hygiene URL from an attendance-only grant',()=>{
 permissionsQuery.mockReturnValue({data:{permissions:[{action:'INSPECTION_VIEW',effect:'ALLOW',scope:{inspectionTypeId:'ATTENDANCE_RATE'},sources:[]}]},isLoading:false})
 render(<MemoryRouter initialEntries={['/inspections/dormitory-hygiene']}><App/></MemoryRouter>)
 expect(screen.getByText('403')).toBeInTheDocument()
})

vi.mock('../shared/api/hygieneAreaApi', () => ({ hygieneAreaApi: { targets: vi.fn().mockResolvedValue([]) } }))

function render(ui: ReactElement) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return testingRender(<QueryClientProvider client={client}>{ui}</QueryClientProvider>)
}

it('protects a direct public-dormitory-hygiene URL from an attendance-only grant', () => {
  permissionsQuery.mockReturnValue({ data: { permissions: [{ action: 'INSPECTION_VIEW', effect: 'ALLOW', scope: { inspectionTypeId: 'ATTENDANCE_RATE' }, sources: [] }] }, isLoading: false })
  render(<MemoryRouter initialEntries={['/inspections/public-dormitory-hygiene']}><App /></MemoryRouter>)
  expect(screen.getByText('403')).toBeInTheDocument()
})

it('protects a direct classroom-hygiene URL from an attendance-only grant', () => {
  permissionsQuery.mockReturnValue({ data: { permissions: [{ action: 'INSPECTION_VIEW', effect: 'ALLOW', scope: { inspectionTypeId: 'ATTENDANCE_RATE' }, sources: [] }] }, isLoading: false })
  render(<MemoryRouter initialEntries={['/inspections/classroom-hygiene']}><App /></MemoryRouter>)
  expect(screen.getByText('403')).toBeInTheDocument()
})

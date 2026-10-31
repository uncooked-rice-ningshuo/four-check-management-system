import type { ReactElement } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render as testingRender, screen, within } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AppLayout } from '../../app/layouts/AppLayout'
import { buildAcademicYearDefaults, currentAcademicYearStart } from '../../features/academic/model/academicCalendarRules'
import { AcademicCalendarPage } from './AcademicCalendarPage'
import { getAcademicCalendar } from '../../shared/api/adminApi'

vi.mock('../../shared/api/adminApi', () => ({
  getAcademicCalendar: vi.fn().mockResolvedValue({ academicYears: [], semesters: [], weeks: [] }),
  createAcademicYear: vi.fn(),
  createSemester: vi.fn(),
  createWeek: vi.fn(),
}))

const signOut = vi.fn()
let currentUser = { id: 'admin-id', loginName: 'admin', studentNo: '', name: '系统管理员', status: 'ACTIVE', accountKind: 'SYSTEM', systemRole: 'SUPER_ADMIN', isSuperAdmin: true, mustChangePassword: false, primaryDepartmentId: '', requestedDepartmentId: '', requestedOrgRoleId: '' }
const permissionsQuery = vi.fn()
vi.mock('../../app/providers/authContext', () => ({
  useAuth: () => ({
    user: currentUser,
    loading: false,
    signOut,
  }),
}))
vi.mock('../../features/attendance/hooks/useAttendancePermissions', () => ({
  useAttendancePermissions: () => permissionsQuery(),
}))

describe('超级管理员页面', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    currentUser = { id: 'admin-id', loginName: 'admin', studentNo: '', name: '系统管理员', status: 'ACTIVE', accountKind: 'SYSTEM', systemRole: 'SUPER_ADMIN', isSuperAdmin: true, mustChangePassword: false, primaryDepartmentId: '', requestedDepartmentId: '', requestedOrgRoleId: '' }
    permissionsQuery.mockReturnValue({ data: { permissions: [] }, isLoading: false })
    vi.mocked(getAcademicCalendar).mockResolvedValue({ academicYears: [], semesters: [], weeks: [] })
  })

  it('提供注册审核、用户、组织、学期、基础数据和字典入口', () => {
    render(<MemoryRouter initialEntries={['/admin/academic']}><Routes><Route path="/admin" element={<AppLayout />}><Route path="academic" element={<div>内容</div>} /></Route></Routes></MemoryRouter>)
    fireEvent.click(screen.getByText('基础功能'))
    expect(screen.getByText('注册申请')).toBeInTheDocument()
    expect(screen.getByText('用户与角色')).toBeInTheDocument()
    expect(screen.getByText('组织与业务')).toBeInTheDocument()
    expect(screen.getByText('学期与周次')).toBeInTheDocument()
    expect(screen.getByText('基础数据')).toBeInTheDocument()
    expect(screen.getByText('字典管理')).toBeInTheDocument()
  })

  it('只提供统一工作台入口和当前账户菜单', () => {
    render(<MemoryRouter initialEntries={['/admin']}><Routes><Route path="/admin" element={<AppLayout />}><Route index element={<div>概览内容</div>} /></Route></Routes></MemoryRouter>)
    expect(screen.getByRole('link', { name: '工作台' })).toHaveAttribute('href', '/')
    expect(screen.queryByText('管理概览')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '系统管理员账户菜单' })).toBeInTheDocument()
  })

  it('在日常检查路由中仍使用同一侧边栏展示超级管理员的全部可用入口', () => {
    render(<MemoryRouter initialEntries={['/inspections/attendance']}><Routes><Route path="/inspections" element={<AppLayout />}><Route path="attendance" element={<div>到课率内容</div>} /></Route></Routes></MemoryRouter>)

    expect(screen.getByRole('link', { name: '工作台' })).toBeInTheDocument()
    fireEvent.click(screen.getByText('基础功能'))
    fireEvent.click(screen.getByText('系统配置'))
    expect(screen.getByText('注册申请')).toBeInTheDocument()
    expect(screen.getByText('基础数据')).toBeInTheDocument()
    expect(screen.getByText('到课率检查')).toBeInTheDocument()
    expect(screen.queryByText('超级管理员工作台')).not.toBeInTheDocument()
  })

  it('不向缺少检查查看权限的系统管理员展示到课率入口', () => {
    currentUser = { ...currentUser, systemRole: 'ADMIN', isSuperAdmin: false }
    render(<MemoryRouter initialEntries={['/admin']}><Routes><Route path="/admin" element={<AppLayout />}><Route index element={<div>概览内容</div>} /></Route></Routes></MemoryRouter>)

    expect(screen.queryByText('到课率检查')).not.toBeInTheDocument()
  })

  it('向拥有检查查看权限的系统管理员展示到课率入口', () => {
    currentUser = { ...currentUser, systemRole: 'ADMIN', isSuperAdmin: false }
    permissionsQuery.mockReturnValue({ data: { permissions: [{ action: 'INSPECTION_VIEW', effect: 'ALLOW', scope: { departmentId: 'department-study' }, sources: [] }] }, isLoading: false })
    render(<MemoryRouter initialEntries={['/admin']}><Routes><Route path="/admin" element={<AppLayout />}><Route index element={<div>概览内容</div>} /></Route></Routes></MemoryRouter>)

    fireEvent.click(screen.getByText('日常检查'))
    expect(screen.getByText('到课率检查')).toBeInTheDocument()
  })

  it('可以收起和展开侧边菜单栏', () => {
    render(<MemoryRouter initialEntries={['/admin']}><Routes><Route path="/admin" element={<AppLayout />}><Route index element={<div>概览内容</div>} /></Route></Routes></MemoryRouter>)

    const sidebar = screen.getByRole('complementary')
    const header = screen.getByRole('banner')
    const collapseButton = within(sidebar).getByRole('button', { name: '收起侧边栏' })
    expect(collapseButton.closest('.admin-sider-footer')).not.toBeNull()
    expect(collapseButton).not.toHaveTextContent('收起侧边栏')
    expect(within(header).queryByRole('button', { name: '收起侧边栏' })).not.toBeInTheDocument()
    expect(collapseButton).toHaveAttribute('aria-expanded', 'true')

    fireEvent.click(collapseButton)

    const expandButton = within(sidebar).getByRole('button', { name: '展开侧边栏' })
    expect(expandButton).toHaveAttribute('aria-expanded', 'false')
    expect(document.querySelector('.admin-sider')).toHaveClass('ant-layout-sider-collapsed')
  })

  it('没有学年配置时显示可操作的空状态', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(<QueryClientProvider client={client}><MemoryRouter><AcademicCalendarPage /></MemoryRouter></QueryClientProvider>)
    expect(await screen.findByText('尚未配置学年和学期')).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: '新增学年' }).length).toBeGreaterThan(0)
  })

  it('选择新增学年时自动填入当前学年的默认日期', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(<QueryClientProvider client={client}><MemoryRouter><AcademicCalendarPage /></MemoryRouter></QueryClientProvider>)
    const buttons = await screen.findAllByRole('button', { name: '新增学年' })
    fireEvent.click(buttons[0])
    await screen.findByRole('dialog', { name: '新增学年' })
    await new Promise(resolve => setTimeout(resolve, 350))
    const defaults = buildAcademicYearDefaults(currentAcademicYearStart())
    expect(await screen.findByPlaceholderText('开始日期')).toHaveValue(defaults.startDate)
    expect(screen.getByPlaceholderText('结束日期')).toHaveValue(defaults.endDate)
  })

  it('新增学期时只能选择上下学期并填入默认日期', async () => {
    vi.mocked(getAcademicCalendar).mockResolvedValue({
      academicYears: [{ id: 'year-1', name: '2025-2026 学年', startDate: '2025-09-01T00:00:00Z', endDate: '2026-08-31T00:00:00Z', enabled: true }],
      semesters: [],
      weeks: [],
    })
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(<QueryClientProvider client={client}><MemoryRouter><AcademicCalendarPage /></MemoryRouter></QueryClientProvider>)
    await screen.findByRole('cell', { name: '2025-2026 学年' })
    const button = await screen.findByRole('button', { name: '新增学期' })
    fireEvent.click(button)
    await screen.findByRole('dialog', { name: '新增学期' })
    await new Promise(resolve => setTimeout(resolve, 350))
    expect(screen.getByLabelText('学期')).toBeInTheDocument()
    expect(screen.getByText('2025-2026 学年上学期')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('开始日期')).toHaveValue('2025-09-01')
    expect(screen.getByPlaceholderText('结束日期')).toHaveValue('2026-01-31')
  })
})

vi.mock('../../features/work-context/WorkContextBar', () => ({ WorkContextBar: () => null }))

vi.mock('../../features/course-inspection/model/queries', () => ({
  useInspectionContext: (query: { typeCode: string }) => ({ data: { inspectionTypeId: query.typeCode, items: [] }, isLoading: false }),
}))

vi.mock('../../shared/api/hygieneAreaApi', () => ({ hygieneAreaApi: { targets: vi.fn().mockResolvedValue([]) } }))

function render(ui: ReactElement) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return testingRender(<QueryClientProvider client={client}>{ui}</QueryClientProvider>)
}

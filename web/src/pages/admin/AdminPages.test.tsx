import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AdminLayout } from '../../shared/layouts/AdminLayout'
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
vi.mock('../../app/providers/authContext', () => ({
  useAuth: () => ({
    user: { id: 'admin-id', loginName: 'admin', studentNo: '', name: '系统管理员', status: 'ACTIVE', accountKind: 'SYSTEM', isSuperAdmin: true, mustChangePassword: false, primaryDepartmentId: '', requestedDepartmentId: '', requestedOrgRoleId: '' },
    loading: false,
    signOut,
  }),
}))

describe('超级管理员页面', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(getAcademicCalendar).mockResolvedValue({ academicYears: [], semesters: [], weeks: [] })
  })

  it('提供注册审核、用户、组织、学期、基础数据和字典入口', () => {
    render(<MemoryRouter initialEntries={['/admin/academic']}><Routes><Route path="/admin" element={<AdminLayout />}><Route path="academic" element={<div>内容</div>} /></Route></Routes></MemoryRouter>)
    expect(screen.getByText('注册申请')).toBeInTheDocument()
    expect(screen.getByText('用户与角色')).toBeInTheDocument()
    expect(screen.getByText('组织与业务')).toBeInTheDocument()
    expect(screen.getByText('学期与周次')).toBeInTheDocument()
    expect(screen.getByText('基础数据')).toBeInTheDocument()
    expect(screen.getByText('字典管理')).toBeInTheDocument()
  })

  it('提供管理概览和当前账户菜单', () => {
    render(<MemoryRouter initialEntries={['/admin']}><Routes><Route path="/admin" element={<AdminLayout />}><Route index element={<div>概览内容</div>} /></Route></Routes></MemoryRouter>)
    expect(screen.getAllByText('管理概览').length).toBeGreaterThan(0)
    expect(screen.getByRole('button', { name: '系统管理员账户菜单' })).toBeInTheDocument()
  })

  it('可以收起和展开侧边菜单栏', () => {
    render(<MemoryRouter initialEntries={['/admin']}><Routes><Route path="/admin" element={<AdminLayout />}><Route index element={<div>概览内容</div>} /></Route></Routes></MemoryRouter>)

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
    expect(await screen.findByLabelText('开始日期')).toHaveValue(defaults.startDate)
    expect(screen.getByLabelText('结束日期')).toHaveValue(defaults.endDate)
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
    expect(screen.getByLabelText('开始日期')).toHaveValue('2025-09-01')
    expect(screen.getByLabelText('结束日期')).toHaveValue('2026-01-31')
  })
})

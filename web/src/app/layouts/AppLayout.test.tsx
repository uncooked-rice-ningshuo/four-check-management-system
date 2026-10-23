import { hygieneAreaApi } from '../../shared/api/hygieneAreaApi'
import type { ReactElement } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render as testingRender, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AppLayout } from './AppLayout'

let currentUser = { id: 'member-1', name: '普通成员', loginName: 'member', systemRole: 'MEMBER', isSuperAdmin: false }
const permissionsQuery = vi.fn()
const signOut = vi.fn()

vi.mock('../providers/authContext', () => ({
  useAuth: () => ({ user: currentUser, signOut }),
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
    <Route path="/login" element={<div>登录页</div>} />
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

  it('uses expandable parent menus and gives each available inspection a distinct icon', () => {
    currentUser = { id: 'admin-1', name: '超级管理员', loginName: 'admin', systemRole: 'SUPER_ADMIN', isSuperAdmin: true }
    renderLayout()

    expect(screen.getByRole('link', { name: '工作台' })).toHaveAttribute('href', '/')
    for (const heading of ['基础功能', '系统配置', '日常检查']) {
      const parent = screen.getByText(heading).closest('.ant-menu-submenu')
      expect(parent).toBeInTheDocument()
      fireEvent.click(screen.getByText(heading))
      expect(parent).toHaveClass('ant-menu-submenu-open')
      expect(document.querySelectorAll('#admin-navigation .ant-menu-submenu-open')).toHaveLength(1)
    }
    const inspectionNames = ['到课率检查', '无机课堂检查', '早锻炼/早签', '宿舍卫生', '公共宿舍卫生', '晚归', '教室卫生']
    const icons = inspectionNames.map(name => screen.getByRole('link', { name }).closest('.ant-menu-item')?.querySelector('svg[data-icon]')?.getAttribute('data-icon'))
    expect(icons.every(Boolean)).toBe(true)
    expect(new Set(icons).size).toBe(inspectionNames.length)
  })

  it('closes the expanded parent when returning to the workbench', () => {
    currentUser = { id: 'admin-1', name: '超级管理员', loginName: 'admin', systemRole: 'SUPER_ADMIN', isSuperAdmin: true }
    renderLayout('/inspections/attendance')
    expect(screen.getByText('日常检查').closest('.ant-menu-submenu')).toHaveClass('ant-menu-submenu-open')
    fireEvent.click(screen.getByRole('link', { name: '工作台' }))
    expect(document.querySelectorAll('#admin-navigation .ant-menu-submenu-open')).toHaveLength(0)
    fireEvent.click(screen.getByText('日常检查'))
    fireEvent.click(screen.getByRole('link', { name: '到课率检查' }))
    expect(screen.getByText('日常检查').closest('.ant-menu-submenu')).toHaveClass('ant-menu-submenu-open')
    fireEvent.click(screen.getByRole('link', { name: '工作台' }))
    expect(document.querySelectorAll('#admin-navigation .ant-menu-submenu-open')).toHaveLength(0)
  })

  it('shows a single inspection as a top-level link', () => {
    permissionsQuery.mockReturnValue({ data: { permissions: [{ action: 'INSPECTION_VIEW', effect: 'ALLOW', scope: { inspectionTypeId: 'ATTENDANCE_RATE' }, sources: [] }] }, isLoading: false })
    renderLayout()
    const inspection = screen.getByRole('link', { name: '到课率检查' })
    expect(inspection.closest('.ant-menu-submenu')).toBeNull()
    expect(screen.queryByText('日常检查')).not.toBeInTheDocument()
  })

  it('adds inspection navigation only when the member has an effective view grant', () => {
    permissionsQuery.mockReturnValue({ data: { permissions: [{ action: 'INSPECTION_VIEW', effect: 'ALLOW', scope: { departmentId: 'department-study' }, sources: [] }] }, isLoading: false })
    renderLayout('/inspections/attendance')

    expect(screen.getByRole('link', { name: '工作台' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: '到课率检查' })).toBeInTheDocument()
    expect(screen.getByText('到课率内容')).toBeInTheDocument()
  })
})

it('preserves the current page when logout fails and allows an explicit retry', async()=>{
 permissionsQuery.mockReturnValue({data:{permissions:[]},isLoading:false})
 signOut.mockRejectedValueOnce(new Error('private dependency detail')).mockResolvedValueOnce(undefined)
 renderLayout()
 fireEvent.mouseEnter(screen.getByRole('button',{name:/账户菜单$/}))
 fireEvent.click(await screen.findByText('退出登录',{exact:true}))
 await screen.findByText('暂时无法确认退出，请重试。')
 expect(screen.getByText('工作台内容')).toBeInTheDocument()
 expect(screen.queryByText('private dependency detail')).not.toBeInTheDocument()
 fireEvent.click(screen.getByRole('button',{name:'重试退出'}))
 await screen.findByText('登录页')
 expect(signOut).toHaveBeenCalledTimes(2)
})

vi.mock('../../features/work-context/WorkContextBar', () => ({ WorkContextBar: () => null }))

vi.mock('../../features/course-inspection/model/queries', () => ({
 useInspectionContext: (query: {typeCode:string}) => ({data:{inspectionTypeId:query.typeCode,items:[]},isLoading:false}),
}))
it('does not expose other inspection types from an attendance-only grant', () => {
 permissionsQuery.mockReturnValue({data:{permissions:[{action:'INSPECTION_VIEW',effect:'ALLOW',scope:{inspectionTypeId:'ATTENDANCE_RATE'},sources:[]}]},isLoading:false})
 renderLayout()
 expect(screen.getByRole('link',{name:'到课率检查'})).toBeInTheDocument()
 expect(screen.queryByRole('link',{name:'无机课堂检查'})).not.toBeInTheDocument()
})
it('exposes the independent unattended classroom route to its own grant', () => {
 permissionsQuery.mockReturnValue({data:{permissions:[{action:'INSPECTION_VIEW',effect:'ALLOW',scope:{inspectionTypeId:'UNATTENDED_CLASSROOM'},sources:[]}]},isLoading:false})
 renderLayout()
 expect(screen.getByRole('link',{name:'无机课堂检查'})).toHaveAttribute('href','/inspections/unattended-classroom')
 expect(screen.queryByRole('link',{name:'到课率检查'})).not.toBeInTheDocument()
})
it('exposes late-return navigation only for its own view grant', () => {
 permissionsQuery.mockReturnValue({data:{permissions:[{action:'INSPECTION_VIEW',effect:'ALLOW',scope:{inspectionTypeId:'LATE_RETURN'},sources:[]}]},isLoading:false})
 renderLayout()
 expect(screen.getByRole('link',{name:'晚归'})).toHaveAttribute('href','/inspections/late-return')
 expect(screen.queryByRole('link',{name:'无机课堂检查'})).not.toBeInTheDocument()
})

it('exposes morning-attendance navigation only for its own view grant',()=>{
 permissionsQuery.mockReturnValue({data:{permissions:[{action:'INSPECTION_VIEW',effect:'ALLOW',scope:{inspectionTypeId:'MORNING_ATTENDANCE'},sources:[]}]},isLoading:false})
 renderLayout()
 expect(screen.getByRole('link',{name:'早锻炼/早签'})).toHaveAttribute('href','/inspections/morning-attendance')
 expect(screen.queryByRole('link',{name:'到课率检查'})).not.toBeInTheDocument()
})

it('exposes dormitory-hygiene navigation only for its own view grant',()=>{
 permissionsQuery.mockReturnValue({data:{permissions:[{action:'INSPECTION_VIEW',effect:'ALLOW',scope:{inspectionTypeId:'DORMITORY_HYGIENE'},sources:[]}]},isLoading:false})
 renderLayout()
 expect(screen.getByRole('link',{name:'宿舍卫生'})).toHaveAttribute('href','/inspections/dormitory-hygiene')
 expect(screen.queryByRole('link',{name:'到课率检查'})).not.toBeInTheDocument()
})

vi.mock('../../shared/api/hygieneAreaApi', () => ({ hygieneAreaApi: { targets: vi.fn().mockResolvedValue([]) } }))

function render(ui: ReactElement) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return testingRender(<QueryClientProvider client={client}>{ui}</QueryClientProvider>)
}


it('只在服务端返回授权维护范围后展示卫生区域入口', async () => {
  vi.mocked(hygieneAreaApi.targets).mockResolvedValueOnce([{ kind: 'classroom-hygiene-areas', departmentId: 'd', departmentName: '检查部门', inspectionTypeId: 't', inspectionTypeName: '教室卫生' }])
  renderLayout()
  expect(await screen.findByRole('link', { name: '卫生区域维护' })).toHaveAttribute('href', '/admin/hygiene-areas')
  expect(screen.queryByRole('link', { name: '基础数据' })).not.toBeInTheDocument()
})

it('exposes public-dormitory hygiene only for its own view grant', () => {
  permissionsQuery.mockReturnValue({ data: { permissions: [{ action: 'INSPECTION_VIEW', effect: 'ALLOW', scope: { inspectionTypeId: 'PUBLIC_DORMITORY_HYGIENE' }, sources: [] }] }, isLoading: false })
  renderLayout()
  expect(screen.getByRole('link', { name: '公共宿舍卫生' })).toHaveAttribute('href', '/inspections/public-dormitory-hygiene')
  expect(screen.queryByRole('link', { name: '宿舍卫生' })).not.toBeInTheDocument()
})

it('exposes classroom hygiene only for its own view grant', () => {
  permissionsQuery.mockReturnValue({ data: { permissions: [{ action: 'INSPECTION_VIEW', effect: 'ALLOW', scope: { inspectionTypeId: 'CLASSROOM_HYGIENE' }, sources: [] }] }, isLoading: false })
  renderLayout()
  expect(screen.getByRole('link', { name: '教室卫生' })).toHaveAttribute('href', '/inspections/classroom-hygiene')
  expect(screen.queryByRole('link', { name: '宿舍卫生' })).not.toBeInTheDocument()
})

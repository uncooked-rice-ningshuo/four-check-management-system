import { fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '../../shared/api/httpClient'
import { AttendancePage } from './AttendancePage'

const recordsQuery = vi.fn()
const permissionsQuery = vi.fn()
vi.mock('../../features/attendance/hooks/useAttendanceRecords', () => ({ useAttendanceRecords: (...args: unknown[]) => recordsQuery(...args) }))
vi.mock('../../features/attendance/hooks/useAttendancePermissions', () => ({
  useAttendancePermissions: () => permissionsQuery(),
	useCurrentAcademicWeek: () => ({ data: { week: null }, isLoading: false }),
  canInspectionAction: (permissions: { action: string; effect: string }[] | undefined, action: string) => permissions?.some(permission => permission.action === action && permission.effect === 'ALLOW') ?? false,
}))
vi.mock('../../app/providers/authContext', () => ({ useAuth: () => ({ user: { primaryDepartmentId: 'department-study' } }) }))

const entry = '/inspections/attendance?semesterId=semester-1&weekId=week-1&inspectionTypeId=type-attendance&departmentId=department-study'
const permission = (action: string) => ({ action, effect: 'ALLOW', scope: { semesterId: 'semester-1', weekId: 'week-1', inspectionTypeId: 'type-attendance', departmentId: 'department-study' }, sources: [] })

describe('AttendancePage', () => {
  beforeEach(() => {
    recordsQuery.mockReset()
    permissionsQuery.mockReset()
    permissionsQuery.mockReturnValue({ data: { permissions: [permission('INSPECTION_VIEW'), permission('INSPECTION_ENTER')] }, isLoading: false })
  })

  it('shows context, accessible loading and empty states', () => {
    recordsQuery.mockReturnValue({ isLoading: true })
    const { rerender } = render(<MemoryRouter initialEntries={[entry]}><AttendancePage /></MemoryRouter>)
    expect(screen.getByRole('status')).toHaveTextContent('正在加载到课率记录')
    recordsQuery.mockReturnValue({ data: { items: [], total: 0, page: 1, pageSize: 20 }, isLoading: false })
    rerender(<MemoryRouter initialEntries={[entry]}><AttendancePage /></MemoryRouter>)
    expect(screen.getByText('当前范围暂无到课率记录')).toBeInTheDocument()
    expect(screen.getByLabelText('学期')).toHaveValue('semester-1')
    expect(screen.getByLabelText('周次')).toHaveValue('week-1')
  })

  it('renders record status/source, pagination and opens only an in-page import drawer', () => {
    recordsQuery.mockReturnValue({ data: { items: [{ id: 'record-1', studentNo: 'S001', studentName: '测试同学', classId: 'class-1', className: '计科 2401', grade: '2024', courseId: 'course-1', courseName: '软件工程', occurredAt: '2026-08-30T08:00:00Z', absenceHours: 2, period: '1-2', remark: '', source: 'EXCEL', status: 'DRAFT', rowVersion: 1 }], total: 21, page: 1, pageSize: 20 }, isLoading: false })
    render(<MemoryRouter initialEntries={[entry]}><AttendancePage /></MemoryRouter>)
    expect(screen.getByText('测试同学')).toBeInTheDocument()
    expect(screen.getByText('Excel 导入')).toBeInTheDocument()
    expect(screen.getByText('草稿')).toBeInTheDocument()
		fireEvent.click(screen.getByTitle('2'))
		expect(recordsQuery.mock.calls.at(-1)?.[0]).toMatchObject({ page: 2, pageSize: 20 })
    fireEvent.click(screen.getByRole('button', { name: '批量导入到课率' }))
    expect(screen.getByText('批量导入到课率')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: '导入' })).not.toBeInTheDocument()
  })

  it('hides mutation actions without effective permission and reports 403/409 safely', () => {
    permissionsQuery.mockReturnValue({ data: { permissions: [permission('INSPECTION_VIEW')] }, isLoading: false })
    recordsQuery.mockReturnValue({ error: new ApiError(403, 'FORBIDDEN_SCOPE', 'denied'), isError: true, isLoading: false })
    const { rerender } = render(<MemoryRouter initialEntries={[entry]}><AttendancePage /></MemoryRouter>)
    expect(screen.queryByRole('button', { name: '批量导入到课率' })).not.toBeInTheDocument()
    expect(screen.getByRole('alert')).toHaveTextContent('无权访问当前学期、周次或部门的数据')
    recordsQuery.mockReturnValue({ error: new ApiError(409, 'VERSION_CONFLICT', 'conflict'), isError: true, isLoading: false })
    rerender(<MemoryRouter initialEntries={[entry]}><AttendancePage /></MemoryRouter>)
    expect(screen.getByRole('alert')).toHaveTextContent('数据已被其他人更新，请刷新后重试')
  })

	it('reports permission and current-week context failures independently', () => {
		permissionsQuery.mockReturnValue({ isError: true, isLoading: false })
		recordsQuery.mockReturnValue({ data: { items: [], total: 0, page: 1, pageSize: 20 }, isLoading: false })
		render(<MemoryRouter initialEntries={[entry]}><AttendancePage /></MemoryRouter>)
		expect(screen.getByRole('alert')).toHaveTextContent('无法加载权限或当前周次')
	})
})

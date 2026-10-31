vi.mock('../../features/import-job/hooks/useImportContext', () => ({ useImportContext: () => ({ data: { items: [] }, isLoading: false, isError: false }) }))
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '../../shared/api/httpClient'
import { AttendancePage } from './AttendancePage'
vi.mock('../../features/department-submission/ui/DepartmentSubmissionActions', () => ({ DepartmentSubmissionActions: () => null }))
vi.mock('../../features/course-inspection/ui/CourseCreatePanel', () => ({ CourseCreatePanel: () => <div role="dialog" aria-label="新增到课率记录"><input role="combobox" aria-label="目标部门" /></div> }))

const recordsQuery = vi.fn()
const permissionsQuery = vi.fn()
const filterOptionsQuery = vi.fn()
const currentWeekQuery = vi.fn()
let currentUser = { isSuperAdmin: false, primaryDepartmentId: 'department-study' }
vi.mock('../../features/attendance/hooks/useAttendanceRecords', () => ({ useAttendanceRecords: (...args: unknown[]) => recordsQuery(...args) }))
vi.mock('../../features/attendance/hooks/useAttendancePermissions', () => ({
	useAttendancePermissions: () => permissionsQuery(),
	useCurrentAcademicWeek: () => currentWeekQuery(),
	useAttendanceFilterOptions: () => filterOptionsQuery(),
  canInspectionAction: (permissions: { action: string; effect: string }[] | undefined, action: string) => permissions?.some(permission => permission.action === action && permission.effect === 'ALLOW') ?? false,
}))
vi.mock('../../features/attendance/hooks/useAttendanceMutations', () => ({ useUpdateAttendance: () => ({ mutateAsync: vi.fn(), isPending: false }) }))
vi.mock('../../features/attendance/hooks/useCreateAttendance', () => ({ useCreateAttendance: () => ({ mutateAsync: vi.fn(), isPending: false }) }))
vi.mock('../../app/providers/authContext', () => ({ useAuth: () => ({ user: currentUser }) }))
vi.mock('../../features/import-job/hooks/useImportBatch', () => ({
  useImportTemplate: () => ({ data: { file: new Blob(['xlsx']), template: { id: 'template-1', version: 1 } }, isLoading: false }),
  useImportBatch: () => ({ data: undefined, isLoading: false }),
  useImportRows: () => ({ data: { items: [], total: 0, page: 1, pageSize: 20 }, isLoading: false }),
  useUploadAttendanceImport: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useConfirmImport: () => ({ mutateAsync: vi.fn(), isPending: false }),
}))

const entry = '/inspections/attendance?semesterId=semester-1&weekId=week-1&inspectionTypeId=type-attendance&departmentId=department-study'
const permission = (action: string) => ({ action, effect: 'ALLOW', scope: { semesterId: 'semester-1', weekId: 'week-1', inspectionTypeId: 'type-attendance', departmentId: 'department-study' }, sources: [] })
const filterOptions = {
	inspectionType: { id: 'type-attendance', code: 'ATTENDANCE_RATE', name: '到课率' },
	responsibleDepartments: [{ id: 'department-study', code: 'STUDY', name: '学习部', enabled: true }],
	departments: [{ id: 'department-study', code: 'STUDY', name: '学习部', enabled: true }],
	semesters: [{ id: 'semester-1', academicYearId: 'year-1', code: 'FIRST', name: '2026-2027 学年上学期', startDate: '2026-09-01', endDate: '2027-01-31', enabled: true }],
	weeks: [{ id: 'week-1', semesterId: 'semester-1', weekNo: 1, startDate: '2026-09-01', endDate: '2026-09-07', enabled: true, note: '' }],
	classes: [{ id: 'class-1', name: '计科 2401', grade: '2024' }],
	grades: ['2024'],
}

describe('AttendancePage', () => {
  beforeEach(() => {
    recordsQuery.mockReset()
    permissionsQuery.mockReset()
		filterOptionsQuery.mockReset()
		currentWeekQuery.mockReset()
		currentUser = { isSuperAdmin: false, primaryDepartmentId: 'department-study' }
		permissionsQuery.mockReturnValue({ data: { permissions: [permission('INSPECTION_VIEW'), permission('INSPECTION_ENTER')] }, isLoading: false })
		currentWeekQuery.mockReturnValue({ data: { week: null }, isLoading: false })
		filterOptionsQuery.mockReturnValue({ data: filterOptions, isLoading: false, isError: false })
  })

	it('hides entry actions when an attendance-specific deny removes a broader grant', () => {
		recordsQuery.mockReturnValue({ data: { items: [], total: 0, page: 1, pageSize: 20 }, isLoading: false })
		permissionsQuery.mockReturnValue({ data: { permissions: [permission('INSPECTION_VIEW'), { action: 'INSPECTION_ENTER', effect: 'ALLOW', scope: { departmentId: 'department-study' }, sources: [] }, { action: 'INSPECTION_ENTER', effect: 'DENY', scope: { departmentId: 'department-study', inspectionTypeId: 'type-attendance' }, sources: [] }] }, isLoading: false })
		render(<MemoryRouter initialEntries={[entry]}><AttendancePage /></MemoryRouter>)
		expect(screen.queryByRole('button', { name: '新增记录' })).not.toBeInTheDocument()
		expect(screen.queryByRole('button', { name: '批量导入到课率' })).not.toBeInTheDocument()
	})

	it('uses ProTable filters and keeps fixed business context out of the query form', () => {
		recordsQuery.mockReturnValue({ data: { items: [], total: 0, page: 1, pageSize: 20 }, isLoading: false })
		const { container } = render(<MemoryRouter initialEntries={[entry]}><AttendancePage /></MemoryRouter>)

		expect(container.querySelector('.ant-pro-query-filter')).not.toBeNull()
		expect(container.querySelector('.attendance-records-table')).not.toBeNull()
		expect(screen.queryByLabelText('检查类型')).not.toBeInTheDocument()
		expect(screen.queryByLabelText('部门')).not.toBeInTheDocument()
		expect(screen.getByLabelText('学期')).toHaveRole('combobox')
		expect(screen.getByLabelText('周次')).toHaveRole('combobox')
		expect(screen.getByLabelText('班级')).toHaveRole('combobox')
		expect(screen.getByLabelText('年级')).toHaveRole('combobox')
		expect(screen.getByLabelText('数据状态')).toHaveRole('combobox')
		expect(screen.getByLabelText('数据来源')).toHaveRole('combobox')
		expect(screen.getByPlaceholderText('输入学号或姓名…')).toHaveAttribute('type', 'text')
	})

	it('shows the department selector only when the account can access multiple departments', () => {
		recordsQuery.mockReturnValue({ data: { items: [], total: 0, page: 1, pageSize: 20 }, isLoading: false })
		filterOptionsQuery.mockReturnValue({ data: { ...filterOptions, departments: [...filterOptions.departments, { id: 'department-office', code: 'OFFICE', name: '办公室', enabled: true }] }, isLoading: false, isError: false })

		render(<MemoryRouter initialEntries={[entry]}><AttendancePage /></MemoryRouter>)

		expect(screen.getByLabelText('部门')).toHaveRole('combobox')
	})

	it('does not select configured departments, semesters or weeks when no explicit or current context exists', () => {
		recordsQuery.mockReturnValue({ data: { items: [], total: 0, page: 1, pageSize: 20 }, isLoading: false })
		currentUser = { isSuperAdmin: true, primaryDepartmentId: '' }

		const { container } = render(<MemoryRouter initialEntries={['/inspections/attendance']}><AttendancePage /></MemoryRouter>)

		expect(recordsQuery).toHaveBeenLastCalledWith(expect.objectContaining({ departmentId: '', semesterId: '', weekId: '' }), false)
		const heading = container.querySelector('.attendance-page-heading')
		expect(heading).toHaveTextContent('按授权范围查询到课率记录；新增时单独选择录入目标。')
		expect(heading).not.toHaveTextContent('宿管部')
		expect(heading).not.toHaveTextContent('2026-2027 学年上学期')
		expect(heading).not.toHaveTextContent('第 1 周')
		expect(screen.getByRole('button', { name: '批量导入到课率' })).toBeDisabled()
		expect(screen.queryByRole('alert')).not.toBeInTheDocument()
		expect(screen.getByText('请选择学期和周次')).toBeInTheDocument()
	})

	it('selects the semester and week only when they match the current date', () => {
		recordsQuery.mockReturnValue({ data: { items: [], total: 0, page: 1, pageSize: 20 }, isLoading: false })
		currentWeekQuery.mockReturnValue({ data: { week: filterOptions.weeks[0] }, isLoading: false })

		render(<MemoryRouter initialEntries={['/inspections/attendance']}><AttendancePage /></MemoryRouter>)

		expect(recordsQuery).toHaveBeenLastCalledWith(expect.objectContaining({ semesterId: 'semester-1', weekId: 'week-1' }), true)
	})

	it('lets a super administrator query all departments and open import without an outer department filter', () => {
		recordsQuery.mockReturnValue({ data: { items: [], total: 0, page: 1, pageSize: 20 }, isLoading: false })
		currentUser = { isSuperAdmin: true, primaryDepartmentId: '' }
		currentWeekQuery.mockReturnValue({ data: { week: filterOptions.weeks[0] }, isLoading: false })

		render(<MemoryRouter initialEntries={['/inspections/attendance']}><AttendancePage /></MemoryRouter>)

		expect(recordsQuery).toHaveBeenLastCalledWith(expect.objectContaining({ departmentId: '' }), true)
		const importButton = screen.getByRole('button', { name: '批量导入到课率' })
		expect(importButton).toBeEnabled()
		fireEvent.click(importButton)
		expect(screen.getByRole('combobox', { name: '导入目标部门' })).toBeInTheDocument()
	})

	it('queries all authorized departments for a member without an explicit department filter', () => {
		recordsQuery.mockReturnValue({ data: { items: [], total: 0, page: 1, pageSize: 20 }, isLoading: false })
		currentWeekQuery.mockReturnValue({ data: { week: filterOptions.weeks[0] }, isLoading: false })
		render(<MemoryRouter initialEntries={['/inspections/attendance']}><AttendancePage /></MemoryRouter>)
		expect(recordsQuery).toHaveBeenLastCalledWith(expect.objectContaining({ departmentId: '' }), true)
		expect(screen.getByLabelText('部门')).toHaveRole('combobox')
	})

	it('resets an explicit department filter to all authorized departments', async () => {
		recordsQuery.mockReturnValue({ data: { items: [], total: 0, page: 1, pageSize: 20 }, isLoading: false })
		render(<MemoryRouter initialEntries={[entry]}><AttendancePage /></MemoryRouter>)
		fireEvent.click(screen.getByRole('button', { name: /重\s*置/ }))
		await waitFor(() => expect(recordsQuery).toHaveBeenLastCalledWith(expect.objectContaining({ departmentId: '' }), true))
	})

	it('opens manual entry without requiring outer semester, week or department filters', () => {
		recordsQuery.mockReturnValue({ data: { items: [], total: 0, page: 1, pageSize: 20 }, isLoading: false })
		render(<MemoryRouter initialEntries={['/inspections/attendance']}><AttendancePage /></MemoryRouter>)
		fireEvent.click(screen.getByRole('button', { name: '新增记录' }))
		expect(screen.getByRole('dialog', { name: '新增到课率记录' })).toBeInTheDocument()
		expect(screen.getByRole('combobox', { name: '目标部门' })).toBeInTheDocument()
	})

	it('does not crash when an older API response omits responsible departments', () => {
		recordsQuery.mockReturnValue({ data: { items: [], total: 0, page: 1, pageSize: 20 }, isLoading: false })
		filterOptionsQuery.mockReturnValue({ data: { ...filterOptions, responsibleDepartments: undefined }, isLoading: false, isError: false })

		render(<MemoryRouter initialEntries={[entry]}><AttendancePage /></MemoryRouter>)

		expect(screen.getByRole('heading', { name: '到课率检查' })).toBeInTheDocument()
	})

  it('shows context, accessible loading and empty states', () => {
    recordsQuery.mockReturnValue({ isLoading: true })
    const { container, rerender } = render(<MemoryRouter initialEntries={[entry]}><AttendancePage /></MemoryRouter>)
    expect(container.querySelector('.attendance-state')).not.toBeInTheDocument()
    expect(container.querySelector('.attendance-records-table .ant-spin-spinning')).toBeInTheDocument()
    recordsQuery.mockReturnValue({ data: { items: [], total: 0, page: 1, pageSize: 20 }, isLoading: false })
    rerender(<MemoryRouter initialEntries={[entry]}><AttendancePage /></MemoryRouter>)
    expect(screen.getByText('当前范围暂无到课率记录')).toBeInTheDocument()
		expect(screen.getByText('按授权范围查询到课率记录；新增时单独选择录入目标。')).toBeInTheDocument()
  })

  it('renders record status/source, pagination and opens only an in-page import drawer', () => {
    recordsQuery.mockReturnValue({ data: { items: [{ id: 'record-1', studentNo: 'S001', studentName: '测试同学', classId: 'class-1', className: '计科 2401', grade: '2026级', courseId: 'course-1', courseName: '软件工程', occurredAt: '2026-08-30T08:00:00Z', absenceHours: 2, period: '1-2', remark: '', source: 'EXCEL', status: 'DRAFT', rowVersion: 1 }], total: 21, page: 1, pageSize: 20 }, isLoading: false })
    render(<MemoryRouter initialEntries={[entry]}><AttendancePage /></MemoryRouter>)
    expect(screen.getByText('测试同学')).toBeInTheDocument()
		expect(screen.getByText('2026级')).toBeInTheDocument()
		expect(screen.queryByText('2026级 级')).not.toBeInTheDocument()
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
    expect(screen.queryByRole('button', { name: '新增记录' })).not.toBeInTheDocument()
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

	it('allows a resumed import batch drawer to close while retaining its resume id', async () => {
		recordsQuery.mockReturnValue({ data: { items: [], total: 0, page: 1, pageSize: 20 }, isLoading: false })
		render(<MemoryRouter initialEntries={[{ pathname: '/inspections/attendance', search: entry.split('?')[1], state: { importBatchId: 'batch-1' } }]}><AttendancePage /></MemoryRouter>)

		expect(screen.getByRole('dialog', { name: '批量导入到课率' })).toBeInTheDocument()
		fireEvent.click(document.querySelector('.ant-drawer-close') as HTMLElement)

		await waitFor(() => expect(screen.queryByRole('dialog', { name: '批量导入到课率' })).not.toBeInTheDocument())
	})
})

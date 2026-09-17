import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ImportBatch, ImportRow } from '../../../entities/import-job/model/importJob'
import { ApiError } from '../../../shared/api/httpClient'
import { AttendanceImportDrawer } from './AttendanceImportDrawer'

const templateQuery = vi.fn()
const batchQuery = vi.fn()
const rowsQuery = vi.fn()
const uploadMutation = vi.fn()
const confirmMutation = vi.fn()

vi.mock('../hooks/useImportBatch', () => ({
  useImportTemplate: (...args: unknown[]) => templateQuery(...args),
  useImportBatch: () => batchQuery(),
  useImportRows: (...args: unknown[]) => rowsQuery(...args),
  useUploadAttendanceImport: () => uploadMutation(),
  useConfirmImport: () => confirmMutation(),
}))

const context = { semesterId: 'semester-1', weekId: 'week-1', inspectionTypeId: 'type-attendance', departmentId: 'department-study' }
const scopeLabels = { inspectionType: '到课率', semester: '2026-2027 学年上学期', week: '第 1 周' }
const batch = (overrides: Partial<ImportBatch> = {}): ImportBatch => ({
  id: 'batch-1', inspectionTypeId: context.inspectionTypeId, semesterId: context.semesterId, weekId: context.weekId,
  departmentId: context.departmentId, templateId: 'template-1', templateVersion: 3, status: 'PREVIEW_READY',
  totalRows: 2, validRows: 2, invalidRows: 0, importedRows: 0, version: 3, ...overrides,
})
const row = (overrides: Partial<ImportRow> = {}): ImportRow => ({
  id: 'row-1', batchId: 'batch-1', rowNo: 5,
  rawData: { studentNo: 'S001', studentName: '测试同学', className: '计科 2401', courseName: '软件工程' },
  normalizedRecord: { id: '', studentNo: 'S001', studentName: '测试同学', classId: 'class-1', className: '计科 2401', grade: '2024', courseId: 'course-1', courseName: '软件工程', occurredAt: '2026-08-30T08:00:00Z', absenceHours: 2, period: '1-2', remark: '', source: 'EXCEL', status: 'DRAFT', rowVersion: 1 },
  validationStatus: 'VALID', errors: [], ...overrides,
})

function setup(batchId?: string, onBatchIdChange = vi.fn()) {
  return render(<MemoryRouter><AttendanceImportDrawer open context={context} scopeLabels={scopeLabels} departments={[{ id: 'department-study', name: '学习部' }]} batchId={batchId} onBatchIdChange={onBatchIdChange} onClose={vi.fn()} /></MemoryRouter>)
}

describe('AttendanceImportDrawer', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    templateQuery.mockReturnValue({ data: { file: new Blob(['xlsx']), template: { id: 'template-1', version: 3 } }, isLoading: false })
    batchQuery.mockReturnValue({ data: undefined, isLoading: false })
    rowsQuery.mockReturnValue({ data: { items: [], total: 0, page: 1, pageSize: 20 }, isLoading: false })
    uploadMutation.mockReturnValue({ mutateAsync: vi.fn(), isPending: false })
    confirmMutation.mockReturnValue({ mutateAsync: vi.fn(), isPending: false })
  })

  it('rejects non-xlsx files before upload', async () => {
    setup()
    const input = screen.getByLabelText('选择到课率 XLSX 文件')

    fireEvent.change(input, { target: { files: [new File(['csv'], 'attendance.csv', { type: 'text/csv' })] } })
    expect(await screen.findByText('仅支持无宏的 .xlsx 文件')).toBeInTheDocument()
  })

  it('accepts an xlsx file for upload', async () => {
    setup()
    const input = screen.getByLabelText('选择到课率 XLSX 文件')

    fireEvent.change(input, { target: { files: [new File(['xlsx'], 'attendance.xlsx', { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' })] } })
    expect(await screen.findByText('attendance.xlsx')).toBeInTheDocument()
  })

  it('selects the target department inside the drawer when the list filter is empty', async () => {
	const emptyDepartmentContext = { ...context, departmentId: '' }
	render(<MemoryRouter><AttendanceImportDrawer open context={emptyDepartmentContext} scopeLabels={scopeLabels} departments={[{ id: 'department-study', name: '学习部' }, { id: 'department-office', name: '办公室' }]} onBatchIdChange={vi.fn()} onClose={vi.fn()} /></MemoryRouter>)

	expect(screen.getByRole('combobox', { name: '导入目标部门' })).toBeInTheDocument()
	expect(templateQuery).toHaveBeenLastCalledWith(expect.objectContaining({ departmentId: '' }), false)
	fireEvent.mouseDown(screen.getByRole('combobox', { name: '导入目标部门' }))
	fireEvent.click(await screen.findByText('学习部'))
	expect(templateQuery).toHaveBeenLastCalledWith(expect.objectContaining({ departmentId: 'department-study' }), true)
  })

  it('shows the import scope as business names in a structured layout', () => {
	setup()

	const summary = document.querySelector('.import-scope-summary')
	expect(summary).toHaveTextContent('检查类型到课率')
	expect(summary).toHaveTextContent('学期2026-2027 学年上学期')
	expect(summary).toHaveTextContent('周次第 1 周')
	expect(summary).toHaveTextContent('目标部门学习部')
	expect(summary).not.toHaveTextContent('type-attendance')
	expect(summary).not.toHaveTextContent('semester-1')
	expect(summary).not.toHaveTextContent('week-1')
	expect(summary).not.toHaveTextContent('department-study')
  })

  it('disables upload controls while a file is being uploaded', () => {
    uploadMutation.mockReturnValue({ mutateAsync: vi.fn(), isPending: true })
    setup()

    expect(screen.getByRole('button', { name: '正在上传并解析' })).toBeDisabled()
  })

  it('shows row errors and keeps confirmation disabled when preview contains invalid rows', () => {
    batchQuery.mockReturnValue({ data: batch({ validRows: 1, invalidRows: 1 }), isLoading: false })
    rowsQuery.mockReturnValue({ data: { items: [row({ validationStatus: 'INVALID', errors: [{ code: 'STUDENT_NOT_FOUND', field: 'studentNo', message: '未找到对应学生', suggestion: '检查学号后重新上传' }] })], total: 1, page: 1, pageSize: 20 }, isLoading: false })
    setup('batch-1')

    expect(screen.getByText('未找到对应学生')).toBeInTheDocument()
    expect(screen.getByText(/检查学号后重新上传/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '确认导入 1 条记录' })).toBeDisabled()
  })

  it('requests the selected validation status across the whole import batch', async () => {
    batchQuery.mockReturnValue({ data: batch({ validRows: 1, invalidRows: 1 }), isLoading: false })
    setup('batch-1')

    fireEvent.click(screen.getByText('仅错误'))

    await waitFor(() => expect(rowsQuery).toHaveBeenLastCalledWith('batch-1', 1, 20, 'INVALID'))
  })

  it('confirms a clean preview once even when the confirmation button is clicked repeatedly', async () => {
    let resolveConfirm: ((value: unknown) => void) | undefined
    const mutateAsync = vi.fn(() => new Promise(resolve => { resolveConfirm = resolve }))
    batchQuery.mockReturnValue({ data: batch(), isLoading: false })
    rowsQuery.mockReturnValue({ data: { items: [row()], total: 2, page: 1, pageSize: 20 }, isLoading: false })
    confirmMutation.mockReturnValue({ mutateAsync, isPending: false })
    setup('batch-1')

    fireEvent.click(screen.getByRole('button', { name: '确认导入 2 条记录' }))
    const confirmButton = await screen.findByRole('button', { name: '确认写入草稿记录' })
    fireEvent.click(confirmButton)
    fireEvent.click(confirmButton)

    expect(mutateAsync).toHaveBeenCalledTimes(1)
    expect(mutateAsync).toHaveBeenCalledWith(expect.objectContaining({ version: 3, idempotencyKey: expect.any(String) }))
    resolveConfirm?.({ batchId: 'batch-1', status: 'CONFIRMED', importedRows: 2, version: 4 })
    await waitFor(() => expect(screen.getByText('已成功导入 2 条草稿记录')).toBeInTheDocument())
  })

  it('shows an actionable conflict message when the batch version is stale', async () => {
    batchQuery.mockReturnValue({ data: batch(), isLoading: false })
    rowsQuery.mockReturnValue({ data: { items: [row()], total: 2, page: 1, pageSize: 20 }, isLoading: false })
    confirmMutation.mockReturnValue({ mutateAsync: vi.fn().mockRejectedValue(new ApiError(409, 'VERSION_CONFLICT', 'conflict')), isPending: false })
    setup('batch-1')

    fireEvent.click(screen.getByRole('button', { name: '确认导入 2 条记录' }))
    fireEvent.click(await screen.findByRole('button', { name: '确认写入草稿记录' }))

    expect(await screen.findByText('数据已被其他人更新，请刷新后重试')).toBeInTheDocument()
  })
})

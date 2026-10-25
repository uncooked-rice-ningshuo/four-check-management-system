vi.mock('./ImportRollbackPanel', () => ({ ImportRollbackPanel: () => null }))
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ImportBatch, ImportRow } from '../../../entities/import-job/model/importJob'
import { ApiError } from '../../../shared/api/httpClient'
import { AttendanceImportDrawer } from './AttendanceImportDrawer'

const contextQuery = vi.fn()
vi.mock('../hooks/useImportContext', () => ({ useImportContext: (...args: unknown[]) => contextQuery(...args) }))
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
const scopeOptions = { inspectionType: { id: context.inspectionTypeId, name: '到课率' }, semesters: [{ id: context.semesterId, name: '2026-2027 学年上学期' }], weeks: [{ id: context.weekId, semesterId: context.semesterId, weekNo: 1 }] }
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
  return render(<MemoryRouter><AttendanceImportDrawer open context={context} scopeOptions={scopeOptions} departments={[{ id: 'department-study', name: '学习部' }]} batchId={batchId} onBatchIdChange={onBatchIdChange} onClose={vi.fn()} /></MemoryRouter>)
}

describe('AttendanceImportDrawer', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    contextQuery.mockImplementation((query: { kind: string }) => ({ data: { items: query.kind === 'grades' ? [{ id: '2026级', label: '2026级' }] : [{ id: 'class-1', label: '测试班级' }] }, isLoading: false, isError: false }))
    templateQuery.mockReturnValue({ data: { file: new Blob(['xlsx']), template: { id: 'template-1', version: 3 } }, isLoading: false })
    batchQuery.mockReturnValue({ data: undefined, isLoading: false })
    rowsQuery.mockReturnValue({ data: { items: [], total: 0, page: 1, pageSize: 20 }, isLoading: false })
    uploadMutation.mockReturnValue({ mutateAsync: vi.fn(), isPending: false })
    confirmMutation.mockReturnValue({ mutateAsync: vi.fn(), isPending: false })
  })

  it('uses the member department independently of a broader list filter', () => {
    render(<MemoryRouter><AttendanceImportDrawer open context={{...context,departmentId:'other-department'}} memberDepartmentId="department-study" scopeOptions={scopeOptions} departments={[{id:'department-study',name:'学习部'}]} onBatchIdChange={vi.fn()} onClose={vi.fn()}/></MemoryRouter>)
    expect(templateQuery.mock.lastCall?.[0]).toMatchObject({departmentId:'department-study'})
    expect(screen.getByRole('combobox',{name:'导入目标部门'})).toBeDisabled()
    expect(screen.getByText('导入归属部门')).toBeInTheDocument()
  })

  it('blocks a member import when their home department is outside the authorized destination options', () => {
    render(<MemoryRouter><AttendanceImportDrawer open context={context} memberDepartmentId="other-department" scopeOptions={scopeOptions} departments={[{id:'department-study',name:'学习部'}]} onBatchIdChange={vi.fn()} onClose={vi.fn()}/></MemoryRouter>)
    expect(screen.getByText('当前账号所属部门不在可选导入归属范围，请联系管理员。')).toBeInTheDocument()
    expect(screen.getByRole('button',{name:'上传并生成预览'})).toBeDisabled()
  })

  it('ignores list class and grade filters when preparing an import', () => {
    render(<MemoryRouter><AttendanceImportDrawer open context={{ ...context, classId: 'list-class', grade: 'list-grade' }} scopeOptions={scopeOptions} departments={[{ id: context.departmentId, name: '学习部' }]} onBatchIdChange={vi.fn()} onClose={vi.fn()} /></MemoryRouter>)
    expect(templateQuery.mock.lastCall?.[0]).not.toHaveProperty('classId')
    expect(templateQuery.mock.lastCall?.[0]).not.toHaveProperty('grade')
    expect(screen.queryByRole('combobox', { name: '导入目标年级' })).not.toBeInTheDocument()
    expect(screen.queryByRole('combobox', { name: '导入目标班级' })).not.toBeInTheDocument()
  })

  it('does not reuse historical class and grade as targets when restarting an invalid batch', async () => {
    batchQuery.mockReturnValue({ data: batch({ classId: 'class-1', grade: '2026级', invalidRows: 1 }), isLoading: false })
    const onBatchIdChange = vi.fn()
    setup('batch-1', onBatchIdChange)
    expect(screen.getByText(/历史批次保留的导入范围：2026级；测试班级/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '重新上传文件' }))
    fireEvent.click(await screen.findByRole('button', { name: '保留原批次并重新上传' }))
    await waitFor(() => expect(onBatchIdChange).toHaveBeenCalledWith(undefined))
    expect(templateQuery.mock.lastCall?.[0]).not.toHaveProperty('classId')
    expect(templateQuery.mock.lastCall?.[0]).not.toHaveProperty('grade')
  })

  it('keeps the batch calendar when restarting from a different attendance list week', async () => {
    batchQuery.mockReturnValue({ data: batch({ invalidRows: 1 }), isLoading: false })
    render(<MemoryRouter><AttendanceImportDrawer open context={{ ...context, weekId: 'week-2' }} scopeOptions={{ ...scopeOptions, weeks: [...scopeOptions.weeks, { id: 'week-2', semesterId: context.semesterId, weekNo: 2 }] }} departments={[{ id: context.departmentId, name: '学习部' }]} batchId="batch-1" onBatchIdChange={vi.fn()} onClose={vi.fn()} /></MemoryRouter>)
    fireEvent.click(screen.getByRole('button', { name: '重新上传文件' }))
    fireEvent.click(await screen.findByRole('button', { name: '保留原批次并重新上传' }))
    await waitFor(() => expect(templateQuery.mock.lastCall?.[0].weekId).toBe('week-1'))
  })

  it('does not carry a previous target into a new import', () => {
    render(<MemoryRouter><AttendanceImportDrawer open context={context} initialImportScope={{ ...context, classId: 'unavailable-class' }} scopeOptions={scopeOptions} departments={[{ id: context.departmentId, name: '学习部' }]} onBatchIdChange={vi.fn()} onClose={vi.fn()} /></MemoryRouter>)
    expect(screen.queryByRole('combobox', { name: '导入目标年级' })).not.toBeInTheDocument()
    expect(screen.queryByRole('combobox', { name: '导入目标班级' })).not.toBeInTheDocument()
    expect(templateQuery.mock.lastCall?.[0]).not.toHaveProperty('classId')
    expect(templateQuery.mock.lastCall?.[0]).not.toHaveProperty('grade')
  })

  it('shows a retryable lookup failure for a historical batch target', () => {
    const refetch = vi.fn()
    contextQuery.mockReturnValue({ isError: true, isLoading: false, refetch })
    batchQuery.mockReturnValue({ data: batch({ classId: 'class-1' }), isLoading: false })
    setup('batch-1')
    expect(screen.getByText('历史批次班级信息加载失败，请重试')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '重试班级信息' }))
    expect(refetch).toHaveBeenCalledOnce()
  })

  it('clears a completed batch when closing so the next import starts fresh', async () => {
    batchQuery.mockReturnValue({ data: batch({ status: 'CONFIRMED', importedRows: 2 }), isLoading: false })
    const onBatchIdChange = vi.fn()
    setup('batch-1', onBatchIdChange)
    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    expect(onBatchIdChange).toHaveBeenCalledWith(undefined)
  })

  it('shows business names rather than internal IDs in the confirmation dialog', async () => {
    batchQuery.mockReturnValue({ data: batch(), isLoading: false })
    setup('batch-1')
    fireEvent.click(screen.getByRole('button', { name: '确认导入 2 条记录' }))
    const dialog = (await screen.findByRole('button', { name: '确认写入草稿记录' })).closest<HTMLElement>('[role=dialog]')
    if (!dialog) throw new Error('Missing confirmation dialog')
    expect(dialog).toHaveTextContent('到课率')
    expect(dialog).toHaveTextContent('2026-2027 学年上学期')
    expect(dialog).toHaveTextContent('第 1 周')
    expect(dialog).toHaveTextContent('学习部')
    for (const id of Object.values(context)) expect(dialog).not.toHaveTextContent(id)
  })

  it.each(['PREVIEW_READY', 'PARSE_FAILED'] as const)('starts a new upload from %s only after confirmation', async status => {
    batchQuery.mockReturnValue({ data: batch({ status, invalidRows: 1, validRows: 1 }), isLoading: false })
    const onBatchIdChange = vi.fn()
    setup('batch-1', onBatchIdChange)
    fireEvent.click(screen.getByRole('button', { name: '重新上传文件' }))
    const dialog = (await screen.findByRole('button', { name: '保留原批次并重新上传' })).closest<HTMLElement>('[role=dialog]')
    if (!dialog) throw new Error('Missing restart dialog')
    expect(dialog).toHaveTextContent('原批次和审计记录会保留')
    expect(onBatchIdChange).not.toHaveBeenCalled()
    fireEvent.click(within(dialog).getByRole('button', { name: '保留原批次并重新上传' }))
    await waitFor(() => expect(onBatchIdChange).toHaveBeenCalledWith(undefined))
  })

  it('reuses the confirmation key after a temporary failure', async () => {
    const mutateAsync = vi.fn().mockRejectedValueOnce(new ApiError(500, 'INTERNAL_ERROR', 'failure')).mockResolvedValueOnce({ importedRows: 2 })
    batchQuery.mockReturnValue({ data: batch(), isLoading: false })
    confirmMutation.mockReturnValue({ mutateAsync, isPending: false })
    setup('batch-1')
    fireEvent.click(screen.getByRole('button', { name: '确认导入 2 条记录' }))
    fireEvent.click(await screen.findByRole('button', { name: '确认写入草稿记录' }))
    await screen.findByText('操作失败，请稍后重试')
    await waitFor(() => expect(screen.queryByRole('dialog', { name: '确认写入到课率草稿' })).not.toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: '确认导入 2 条记录' }))
    fireEvent.click(await screen.findByRole('button', { name: '确认写入草稿记录' }))
    await screen.findByText('已成功导入 2 条草稿记录')
    expect(mutateAsync.mock.calls[1][0]).toEqual(mutateAsync.mock.calls[0][0])
  })

  it('uses the resumed batch week instead of the current list week', async () => {
    batchQuery.mockReturnValue({ data: batch(), isLoading: false })
    render(<MemoryRouter><AttendanceImportDrawer open context={{ ...context, weekId: 'week-2' }} scopeOptions={{ ...scopeOptions, weeks: [...scopeOptions.weeks, { id: 'week-2', semesterId: context.semesterId, weekNo: 2 }] }} departments={[{ id: context.departmentId, name: '学习部' }]} batchId="batch-1" onBatchIdChange={vi.fn()} onClose={vi.fn()} /></MemoryRouter>)
    fireEvent.click(screen.getByRole('button', { name: '确认导入 2 条记录' }))
    const dialog = (await screen.findByRole('button', { name: '确认写入草稿记录' })).closest<HTMLElement>('[role=dialog]')
    expect(dialog).toHaveTextContent('第 1 周')
    expect(dialog).not.toHaveTextContent('第 2 周')
  })

  it('blocks confirmation if the batch scope cannot be resolved', () => {
    batchQuery.mockReturnValue({ data: batch({ weekId: 'unknown-week' }), isLoading: false })
    setup('batch-1')
    expect(screen.getByText('批次范围信息不可用，请刷新页面后核对')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '确认导入 2 条记录' })).toBeDisabled()
  })

  it.each(['UPLOADED', 'PARSING', 'CONFIRMED'] as const)('does not offer replacement for a %s batch', status => {
    batchQuery.mockReturnValue({ data: batch({ status }), isLoading: false })
    setup('batch-1')
    expect(screen.queryByRole('button', { name: '重新上传文件' })).not.toBeInTheDocument()
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
	render(<MemoryRouter><AttendanceImportDrawer open context={emptyDepartmentContext} scopeOptions={scopeOptions} departments={[{ id: 'department-study', name: '学习部' }, { id: 'department-office', name: '办公室' }]} onBatchIdChange={vi.fn()} onClose={vi.fn()} /></MemoryRouter>)

	expect(screen.getByRole('combobox', { name: '导入目标部门' })).toBeInTheDocument()
	expect(templateQuery).toHaveBeenLastCalledWith(expect.objectContaining({ departmentId: '' }), false, 'ATTENDANCE_RATE')
	fireEvent.mouseDown(screen.getByRole('combobox', { name: '导入目标部门' }))
	fireEvent.click(await screen.findByText('学习部'))
	expect(templateQuery).toHaveBeenLastCalledWith(expect.objectContaining({ departmentId: 'department-study' }), true, 'ATTENDANCE_RATE')
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

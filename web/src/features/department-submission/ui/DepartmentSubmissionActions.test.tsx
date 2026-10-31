import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'
import { DepartmentSubmissionActions } from './DepartmentSubmissionActions'
import { departmentSubmissionApi } from '../../../shared/api/departmentSubmissionApi'
import { submissionCartApi } from '../../../shared/api/submissionCartApi'
import { allOptions } from '../../../shared/api/inspectionOptionsApi'

vi.mock('../../../shared/api/departmentSubmissionApi', () => ({ departmentSubmissionApi: {
  list: vi.fn(), submit: vi.fn(), decide: vi.fn(), resubmit: vi.fn(),
} }))
vi.mock('../../../shared/api/submissionCartApi', () => ({ submissionCartApi: {
  candidates: vi.fn(), batches: vi.fn(), batchRecords: vi.fn(), detail: vi.fn(),
} }))
vi.mock('../../../shared/api/inspectionOptionsApi', () => ({ allOptions: vi.fn() }))
const options = vi.mocked(allOptions)
const api = vi.mocked(departmentSubmissionApi)
const cartApi = vi.mocked(submissionCartApi)
const scope = { semesterId: 'semester', weekId: 'week', departmentId: 'department', inspectionTypeId: 'type' }
const record = { ...scope, id: 'record', rowVersion: 1, status: 'DRAFT' }
const cartRecord = { ...record, typeCode: 'ATTENDANCE_RATE', studentNumber: 'S001', studentName: '测试同学', className: '计科 2401', grade: '2024', source: 'MANUAL', canEdit: true, details: {} }
const batch = { id: 'batch', scope, semesterName: '秋季学期', weekNo: 3, departmentName: '学习部', status: 'PENDING_REVIEW' as const,
  revisionNo: 1, rowVersion: 1, createdBy: 'actor', submittedBy: 'actor', createdAt: '2026-09-01T00:00:00Z',
  submittedAt: '2026-09-01T00:00:00Z', recordIds: ['record'], records: [{ id: 'record', rowVersion: 2, status: 'PENDING_REVIEW', createdBy: 'actor' }] }

beforeEach(() => {
  options.mockImplementation(async (_path, query) => ({ inspectionTypeId: 'type', items: query.kind === 'semesters' ? [{ id: 'semester', label: '秋季学期' }] : query.kind === 'weeks' ? [{ id: 'week', label: '第 3 周' }] : [{ id: 'department', label: '学习部' }] }))
  api.list.mockReset()
  api.submit.mockReset()
  api.decide.mockReset()
  api.resubmit.mockReset()
  cartApi.candidates.mockResolvedValue({ items: [], total: 0, page: 1, pageSize: 10 })
  cartApi.batches.mockResolvedValue({ items: [], total: 0, page: 1, pageSize: 10 })
  cartApi.batchRecords.mockResolvedValue({ items: [], total: 0, page: 1, pageSize: 10 })
  cartApi.detail.mockResolvedValue(cartRecord)
  api.list.mockResolvedValue({ items: [batch], total: 1, page: 1, pageSize: 10 })
  api.submit.mockResolvedValue(batch)
})

function show(selected = [record], canReview = true) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const onChanged = vi.fn()
  render(<QueryClientProvider client={client}><DepartmentSubmissionActions typeCode="ATTENDANCE_RATE" inspectionTypeId="type" selected={selected} canReview={canReview} onChanged={onChanged} /></QueryClientProvider>)
  return onChanged
}

it('selects a candidate inside the drawer and submits after review', async () => {
  cartApi.candidates.mockResolvedValue({ items: [cartRecord], total: 1, page: 1, pageSize: 10 })
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const onChanged = vi.fn()
  render(<QueryClientProvider client={client}><DepartmentSubmissionActions typeCode="ATTENDANCE_RATE" inspectionTypeId="type" listScope={scope} selected={[]} canReview onChanged={onChanged} /></QueryClientProvider>)
  fireEvent.click(screen.getByRole('button', { name: '提交记录' }))
  expect(await screen.findByText('秋季学期')).toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: '下一步：添加记录' }))
  expect(await screen.findByText(/测试同学/)).toBeInTheDocument()
  const checkboxes = screen.getAllByRole('checkbox')
  fireEvent.click(checkboxes[checkboxes.length - 1])
  fireEvent.click(screen.getByRole('button', { name: '下一步：核对清单' }))
  fireEvent.click(screen.getByRole('button', { name: '提交审核' }))
  expect(api.submit).not.toHaveBeenCalled()
  fireEvent.click(await screen.findByRole('button', { name: '确认提交' }))
  await waitFor(() => expect(api.submit).toHaveBeenCalledWith(scope, [{ id: 'record', version: 1 }], expect.any(String)))
  await waitFor(() => expect(onChanged).toHaveBeenCalledTimes(1))
})

it('shows the named week and review actions from the authorized batch list', async () => {
  show()
  fireEvent.click(screen.getByRole('button', { name: '检查提交批次' }))
  expect(await screen.findByText(/秋季学期/)).toBeInTheDocument()
  expect(screen.getByText(/第 3 周/)).toBeInTheDocument()
  expect(screen.getByRole('button', { name: '退回' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: '确认' })).toBeInTheDocument()
})

it('keeps a failed batch load distinct from a successful empty list and allows retry', async () => {
  api.list.mockReset()
  api.list.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce({ items: [], total: 0, page: 1, pageSize: 10 })
  show([], false)
  fireEvent.click(screen.getByRole('button', { name: '检查提交批次' }))
  expect(await screen.findByText('批次加载失败')).toBeInTheDocument()
  expect(screen.queryByText('当前范围暂无提交批次')).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: /重\s*试/ }))
  expect(await screen.findByText('当前范围暂无提交批次')).toBeInTheDocument()
})

it('does not show review controls without a review action hint', async () => {
  show([], false)
  fireEvent.click(screen.getByRole('button', { name: '检查提交批次' }))
  expect(await screen.findByText(/秋季学期/)).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: '退回' })).not.toBeInTheDocument()
  expect(screen.queryByRole('button', { name: '确认' })).not.toBeInTheDocument()
})

it('opens submission without selecting a table row and asks for the submission scope', () => {
  show([], false)
  const button = screen.getByRole('button', { name: '提交记录' })
  expect(button).toBeEnabled()
  fireEvent.click(button)
  expect(screen.getByRole('heading', { name: '确定提交范围' })).toBeInTheDocument()
})

it('shows a recoverable option failure instead of accepting an unresolved scope', async () => {
  options.mockRejectedValue(new Error('offline'))
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(<QueryClientProvider client={client}><DepartmentSubmissionActions typeCode="ATTENDANCE_RATE" inspectionTypeId="type" listScope={scope} selected={[]} canReview={false} onChanged={() => {}} /></QueryClientProvider>)
  fireEvent.click(screen.getByRole('button', { name: '提交记录' }))
  expect(await screen.findByText('范围选项加载失败')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: '下一步：添加记录' })).toBeDisabled()
  expect(screen.queryByText('semester')).not.toBeInTheDocument()
})

it('adds only eligible Excel batch records and shows changed records before submission', async () => {
  cartApi.batches.mockResolvedValue({ items: [{ ...scope, id: 'import-id', originalName: '晚归.xlsx', status: 'CONFIRMED', createdAt: '2026-09-01T00:00:00Z', confirmedAt: '2026-09-01T00:00:00Z', importedRows: 2, selectableRecords: 1, submittedRecords: 1, unavailableRecords: 0 }], total: 1, page: 1, pageSize: 10 })
  cartApi.batchRecords.mockResolvedValue({ items: [{ record: { ...cartRecord, source: 'EXCEL', importBatchId: 'import-id' }, selectable: true }, { record: { ...cartRecord, id: 'already', status: 'PENDING_REVIEW' }, selectable: false, unavailableReason: 'ALREADY_SUBMITTED' }], total: 2, page: 1, pageSize: 100 })
  cartApi.detail.mockResolvedValue({ ...cartRecord, rowVersion: 2 })
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(<QueryClientProvider client={client}><DepartmentSubmissionActions typeCode="ATTENDANCE_RATE" inspectionTypeId="type" listScope={scope} selected={[]} canReview onChanged={() => {}} /></QueryClientProvider>)
  fireEvent.click(screen.getByRole('button', { name: '提交记录' }))
  await waitFor(() => expect(screen.getByRole('button', { name: '下一步：添加记录' })).toBeEnabled())
  fireEvent.click(screen.getByRole('button', { name: '下一步：添加记录' }))
  fireEvent.click(screen.getByRole('button', { name: 'Excel 批次' }))
  fireEvent.click(await screen.findByRole('button', { name: '添加整批可提交记录' }))
  await waitFor(() => expect(screen.getByRole('button', { name: '下一步：核对清单' })).toBeEnabled())
  fireEvent.click(screen.getByRole('button', { name: '下一步：核对清单' }))
  expect(screen.queryByText('import-id')).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: '提交审核' }))
  expect(await screen.findByText(/记录已被修改/)).toBeInTheDocument()
  expect(api.submit).not.toHaveBeenCalled()
})

it('shows course, classroom, date and period when selecting and reviewing a record', async () => {
  const courseRecord = { ...cartRecord, currentCourseName: '离散数学', currentClassroomName: '教学楼 A101', details: { date: '2026-09-10', time: '08:30:00', period: '1-2', absenceHours: 2 } }
  cartApi.candidates.mockResolvedValue({ items: [courseRecord], total: 1, page: 1, pageSize: 10 })
  cartApi.detail.mockResolvedValue(courseRecord)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(<QueryClientProvider client={client}><DepartmentSubmissionActions typeCode="ATTENDANCE_RATE" inspectionTypeId="type" listScope={scope} selected={[]} canReview onChanged={() => {}} /></QueryClientProvider>)
  fireEvent.click(screen.getByRole('button', { name: '提交记录' }))
  await waitFor(() => expect(screen.getByRole('button', { name: '下一步：添加记录' })).toBeEnabled())
  fireEvent.click(screen.getByRole('button', { name: '下一步：添加记录' }))
  expect(await screen.findByText('离散数学')).toBeInTheDocument()
  expect(screen.getByText('教学楼 A101')).toBeInTheDocument()
  expect(screen.getByText('2026-09-10')).toBeInTheDocument()
  expect(screen.getByText('08:30:00')).toBeInTheDocument()
  expect(screen.getByText('1-2')).toBeInTheDocument()
  const checks = screen.getAllByRole('checkbox')
  fireEvent.click(checks[checks.length - 1])
  fireEvent.click(screen.getByRole('button', { name: '下一步：核对清单' }))
  expect(screen.getByText('离散数学')).toBeInTheDocument()
  expect(screen.getByText('2026-09-10')).toBeInTheDocument()
  expect(screen.getByText('08:30:00')).toBeInTheDocument()
})

it('shows the imported record details in batch selection and final confirmation', async () => {
  const imported = { ...cartRecord, source: 'EXCEL', importBatchId: 'import-id', currentCourseName: '线性代数', details: { date: '2026-09-11', time: '10:00:00', period: '3-4' } }
  cartApi.batches.mockResolvedValue({ items: [{ ...scope, id: 'import-id', originalName: '考勤.xlsx', status: 'CONFIRMED', createdAt: '2026-09-01T00:00:00Z', confirmedAt: '2026-09-01T00:00:00Z', importedRows: 1, selectableRecords: 1, submittedRecords: 0, unavailableRecords: 0 }], total: 1, page: 1, pageSize: 10 })
  cartApi.batchRecords.mockResolvedValue({ items: [{ record: imported, selectable: true }], total: 1, page: 1, pageSize: 10 })
  cartApi.detail.mockResolvedValue(imported)
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(<QueryClientProvider client={client}><DepartmentSubmissionActions typeCode="ATTENDANCE_RATE" inspectionTypeId="type" listScope={scope} selected={[]} canReview onChanged={() => {}} /></QueryClientProvider>)
  fireEvent.click(screen.getByRole('button', { name: '提交记录' }))
  await waitFor(() => expect(screen.getByRole('button', { name: '下一步：添加记录' })).toBeEnabled())
  fireEvent.click(screen.getByRole('button', { name: '下一步：添加记录' }))
  fireEvent.click(screen.getByRole('button', { name: 'Excel 批次' }))
  fireEvent.click(await screen.findByRole('button', { name: '查看记录' }))
  expect(await screen.findByText('线性代数')).toBeInTheDocument()
  expect(screen.getByText('10:00:00')).toBeInTheDocument()
  const checks = screen.getAllByRole('checkbox')
  fireEvent.click(checks[checks.length - 1])
  fireEvent.click(screen.getByRole('button', { name: '下一步：核对清单' }))
  fireEvent.click(screen.getByRole('button', { name: '提交审核' }))
  const title = await screen.findByText('最终确认本次提交')
  const dialog = title.closest('[role=dialog]')
  expect(dialog).toHaveTextContent('线性代数')
  expect(dialog).toHaveTextContent('2026-09-11')
  expect(dialog).toHaveTextContent('3-4')
})

it('labels a confirmed college batch as college confirmed', async () => {
  const college = { ...batch, scope: { ...scope, departmentId: '' }, departmentName: undefined }
  api.list.mockResolvedValue({ items: [college], total: 1, page: 1, pageSize: 10 })
  api.decide.mockResolvedValue({ ...college, status: 'CONFIRMED' })
  show()
  fireEvent.click(screen.getByRole('button', { name: '检查提交批次' }))
  fireEvent.click(await screen.findByRole('button', { name: '确认' }))
  fireEvent.click(await screen.findByRole('button', { name: '确认批次' }))
  expect(await screen.findByText('批次状态：校级已确认')).toBeInTheDocument()
})

it('does not describe a department batch with a missing name as college-level', async () => {
  api.list.mockResolvedValue({ items: [{ ...batch, departmentName: undefined }], total: 1, page: 1, pageSize: 10 })
  show()
  fireEvent.click(screen.getByRole('button', { name: '检查提交批次' }))
  expect(await screen.findByRole('cell', { name: /部门名称不可用/ })).toBeInTheDocument()
})

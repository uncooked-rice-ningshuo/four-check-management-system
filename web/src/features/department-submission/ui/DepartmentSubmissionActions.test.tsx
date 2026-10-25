import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'
import { DepartmentSubmissionActions } from './DepartmentSubmissionActions'
import { departmentSubmissionApi } from '../../../shared/api/departmentSubmissionApi'
import { submissionCartApi } from '../../../shared/api/submissionCartApi'

vi.mock('../../../shared/api/departmentSubmissionApi', () => ({ departmentSubmissionApi: {
  list: vi.fn(), submit: vi.fn(), decide: vi.fn(), resubmit: vi.fn(),
} }))
vi.mock('../../../shared/api/submissionCartApi', () => ({ submissionCartApi: {
  candidates: vi.fn(), batches: vi.fn(), batchRecords: vi.fn(), detail: vi.fn(),
} }))
const api = vi.mocked(departmentSubmissionApi)
const cartApi = vi.mocked(submissionCartApi)
const scope = { semesterId: 'semester', weekId: 'week', departmentId: 'department', inspectionTypeId: 'type' }
const record = { ...scope, id: 'record', rowVersion: 1, status: 'DRAFT' }
const cartRecord = { ...record, typeCode: 'ATTENDANCE_RATE', studentNumber: 'S001', studentName: '测试同学', className: '计科 2401', grade: '2024', source: 'MANUAL', canEdit: true, details: {} }
const batch = { id: 'batch', scope, semesterName: '秋季学期', weekNo: 3, departmentName: '学习部', status: 'PENDING_REVIEW' as const,
  revisionNo: 1, rowVersion: 1, createdBy: 'actor', submittedBy: 'actor', createdAt: '2026-09-01T00:00:00Z',
  submittedAt: '2026-09-01T00:00:00Z', recordIds: ['record'], records: [{ id: 'record', rowVersion: 2, status: 'PENDING_REVIEW', createdBy: 'actor' }] }

beforeEach(() => {
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
  render(<QueryClientProvider client={client}><DepartmentSubmissionActions inspectionTypeId="type" selected={selected} canReview={canReview} onChanged={onChanged} /></QueryClientProvider>)
  return onChanged
}

it('submits only a same-scope draft selection after confirmation', async () => {
  const onChanged = show()
  fireEvent.click(screen.getByRole('button', { name: '准备提交（已选 1）' }))
  fireEvent.click(screen.getByRole('button', { name: '核对并提交' }))
  const confirmButton = await screen.findByRole('button', { name: '确认提交' })
  expect(api.submit).not.toHaveBeenCalled()
  fireEvent.click(confirmButton)
  await waitFor(() => expect(api.submit).toHaveBeenCalledWith(scope, [{ id: 'record', version: 1 }], expect.any(String)))
  await waitFor(() => expect(onChanged).toHaveBeenCalledTimes(1))
})

it('does not submit mixed scopes or returned records', async () => {
  show([record, { ...record, id: 'another', weekId: 'other' }])
  expect(screen.getByRole('button', { name: '准备提交（已选 2）' })).toBeDisabled()
  expect(api.submit).not.toHaveBeenCalled()
})

it('keeps the selected record versions when the list refreshes before confirmation', async () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const view = (selected: typeof record[]) => <QueryClientProvider client={client}>
    <DepartmentSubmissionActions inspectionTypeId="type" listScope={scope} selected={selected} canReview onChanged={() => {}} />
  </QueryClientProvider>
  const { rerender } = render(view([record]))
  fireEvent.click(screen.getByRole('button', { name: '准备提交（已选 1）' }))
  rerender(view([]))
  fireEvent.click(screen.getByRole('button', { name: '核对并提交' }))
  fireEvent.click(await screen.findByRole('button', { name: '确认提交' }))
  await waitFor(() => expect(api.submit).toHaveBeenCalledWith(scope, [{ id: 'record', version: 1 }], expect.any(String)))
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

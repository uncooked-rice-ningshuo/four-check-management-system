import { StrictMode } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'
import type { ImportBatch } from '../../../entities/import-job/model/importJob'
import { ApiError } from '../../../shared/api/httpClient'
import { ImportRollbackPanel } from './ImportRollbackPanel'
const mutateAsync = vi.fn()
vi.mock('../hooks/useImportRollback', () => ({ useImportRollback: () => ({ mutateAsync, isPending: false }) }))
const batch: ImportBatch = { id: 'batch', inspectionTypeId: 'type', semesterId: 'term', weekId: 'week', departmentId: 'department', templateId: 'template', templateVersion: 1, status: 'CONFIRMED', totalRows: 2, validRows: 2, invalidRows: 0, importedRows: 2, version: 4 }
beforeEach(() => mutateAsync.mockReset())
it('requires a reason, preserves it on failure, retries and resets after closing in StrictMode', async () => {
  mutateAsync.mockRejectedValueOnce(new ApiError(500, 'IMPORT_SERVICE_UNAVAILABLE', 'unavailable')).mockResolvedValueOnce({})
  const busy = vi.fn()
  render(<StrictMode><ImportRollbackPanel batch={batch} onBusyChange={busy} /></StrictMode>)
  fireEvent.click(screen.getByRole('button', { name: '整批回滚' }))
  expect(screen.getByText(/全部 2 条记录/)).toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: '确认回滚全部记录' }))
  await screen.findByText('请填写回滚原因')
  expect(mutateAsync).not.toHaveBeenCalled()
  fireEvent.change(screen.getByLabelText('回滚原因'), { target: { value: '重复导入核对' } })
  fireEvent.click(screen.getByRole('button', { name: '确认回滚全部记录' }))
  await screen.findByText('回滚失败，请稍后重试。')
  expect(screen.getByLabelText('回滚原因')).toHaveValue('重复导入核对')
  fireEvent.click(screen.getByRole('button', { name: '确认回滚全部记录' }))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(mutateAsync).toHaveBeenLastCalledWith({ version: 4, reason: '重复导入核对' })
  expect(busy).toHaveBeenLastCalledWith(false)
  fireEvent.click(screen.getByRole('button', { name: '整批回滚' }))
  expect(screen.getByLabelText('回滚原因')).toHaveValue('')
  expect(screen.queryByText('回滚失败，请稍后重试。')).not.toBeInTheDocument()
})
it('shows retained provenance after rollback and does not offer rollback for unconfirmed batches', () => {
  const { rerender } = render(<ImportRollbackPanel batch={{ ...batch, status: 'PREVIEW_READY' }} onBusyChange={vi.fn()} />)
  expect(screen.queryByRole('button', { name: '整批回滚' })).not.toBeInTheDocument()
  rerender(<ImportRollbackPanel batch={{ ...batch, status: 'ROLLED_BACK' }} onBusyChange={vi.fn()} />)
  expect(screen.getByText('已整批回滚 2 条记录')).toBeInTheDocument()
  expect(screen.queryByRole('button', { name: '整批回滚' })).not.toBeInTheDocument()
})

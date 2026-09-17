import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'
import { AttendanceMutationDialogs } from './AttendanceMutationDialogs'
import { ApiError } from '../../../shared/api/httpClient'
import type { AttendanceRecord } from '../../../entities/inspection/model/inspection'
const remove = vi.fn()
vi.mock('../hooks/useAttendanceMutations', () => ({ useAttendanceDetail: () => ({ data: undefined }), useDeleteAttendance: () => ({ mutateAsync: remove, isPending: false }) }))
const record: AttendanceRecord = { id: 'r', studentNo: 'S1', studentName: '虚构同学', classId: 'c', className: '测试班', grade: '2026级', courseId: 'course', courseName: '课程', occurredAt: '2026-09-06T00:00:00Z', absenceHours: 2, period: '1-2', remark: '', status: 'DRAFT', source: 'MANUAL', rowVersion: 3 }
beforeEach(() => { remove.mockReset() })
it('requires a reason and sends every selected version on confirmation', async () => {
 const close = vi.fn()
 remove.mockResolvedValue(undefined)
 render(<AttendanceMutationDialogs deleting={[record, { ...record, id: 'r2', rowVersion: 4 }]} onClose={close} />)
 expect(screen.getByText('确认删除 2 条到课率草稿？')).toBeInTheDocument()
 fireEvent.click(screen.getByRole('button', { name: '确认删除' }))
 expect(await screen.findByText('请填写删除原因')).toBeInTheDocument()
 expect(remove).not.toHaveBeenCalled()
 fireEvent.change(screen.getByLabelText('删除原因'), { target: { value: '测试删除' } })
 fireEvent.click(screen.getByRole('button', { name: '确认删除' }))
 await waitFor(() => expect(close).toHaveBeenCalledTimes(1))
 expect(remove).toHaveBeenCalledWith({ items: [{ id: 'r', version: 3 }, { id: 'r2', version: 4 }], reason: '测试删除' })
})
it('keeps the confirmation open on a version conflict', async () => {
 const close = vi.fn()
 remove.mockRejectedValue(new ApiError(409, 'VERSION_CONFLICT', '记录已被修改，请刷新后重试'))
 render(<AttendanceMutationDialogs deleting={[record]} onClose={close} />)
 fireEvent.change(screen.getByLabelText('删除原因'), { target: { value: '测试删除' } })
 fireEvent.click(screen.getByRole('button', { name: '确认删除' }))
 await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('记录已被修改'))
 expect(close).not.toHaveBeenCalled()
 expect(screen.getByLabelText('删除原因')).toHaveValue('测试删除')
})

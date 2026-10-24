import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'
import { ApiError } from '../../../shared/api/httpClient'
import { ManualAttendanceDrawer } from './ManualAttendanceDrawer'
beforeEach(() => { save.mockReset(); update.mockReset() })
const save = vi.fn()
const update = vi.fn()
vi.mock('../hooks/useAttendanceMutations', () => ({ useUpdateAttendance: () => ({ mutateAsync: update, isPending: false }) }))
vi.mock('../hooks/useCreateAttendance', () => ({ useCreateAttendance: () => ({ mutateAsync: save, isPending: false }) }))
vi.mock('../../../entities/inspection/ui/CourseSelect', () => ({ CourseSelect: (props: { value?: string; onChange?: (value?: string) => void }) => <input aria-label="课程" value={props.value ?? ''} onChange={event => props.onChange?.(event.target.value)} /> }))
const options = { inspectionType: { id: 'type', code: 'ATTENDANCE_RATE' as const, name: '到课率' }, responsibleDepartments: [{ id: 'dept', code: 'D', name: '学习部', enabled: true }], departments: [{ id: 'dept', code: 'D', name: '学习部', enabled: true }], semesters: [{ id: 'semester', academicYearId: 'year', code: 'FIRST', name: '第一学期', startDate: '', endDate: '', enabled: true }], weeks: [{ id: 'week', semesterId: 'semester', weekNo: 1, startDate: '', endDate: '', enabled: true, note: '' }], classes: [{ id: 'class', name: '测试班', grade: '2026级' }], grades: ['2026级'] }
it('requires a complete form before creating a draft', async () => {
 render(<ManualAttendanceDrawer options={options} onClose={vi.fn()} />)
 expect(screen.getByRole('dialog', { name: '新增到课率记录' })).toBeInTheDocument()
 fireEvent.click(screen.getByRole('button', { name: '保存草稿' }))
 await waitFor(() => expect(screen.getAllByText('请填写或选择此项').length).toBeGreaterThan(0))
 expect(save).not.toHaveBeenCalled()
})

async function fillForm() {
  for (const [label, text] of [['目标部门', '学习部'], ['学生班级', '测试班']]) {
    fireEvent.mouseDown(screen.getByRole('combobox', { name: label }))
    fireEvent.click(screen.getByText(text, { selector: '.ant-select-item-option-content' }))
  }
  for (const [label, value] of [['学号', 'S001'], ['姓名', '虚构同学'], ['上课教室', '博学楼 101'], ['旷课学时', '2']]) {
    fireEvent.change(screen.getByLabelText(label), { target: { value } })
  }
  for (const [label, value] of [['开始节次', '1'], ['结束节次', '2']]) {
    fireEvent.mouseDown(screen.getByRole('combobox', { name: label }))
    const options = screen.getAllByText(`第 ${value} 节`, { selector: '.ant-select-item-option-content' })
    fireEvent.click(options[options.length - 1])
    if (label === '开始节次') await waitFor(() => expect(screen.getByRole('combobox', { name: '结束节次' })).not.toBeDisabled())
  }
  fireEvent.change(screen.getByLabelText('课程'), { target: { value: 'course-1' } })
  for (const [label, value] of [['检查日期', '2026-09-06'], ['检查时间', '08:00']]) {
    const picker = screen.getByLabelText(label)
    fireEvent.change(picker, { target: { value } })
    fireEvent.keyDown(picker, { key: 'Enter' })
  }
}
it('creates using the form target and server-owned snapshots then closes', async () => {
  const close = vi.fn()
  save.mockResolvedValue({ id: 'created' })
  render(<ManualAttendanceDrawer options={options} initialSemesterId="semester" initialWeekId="week" onClose={close} />)
  await fillForm()
  fireEvent.click(screen.getByRole('button', { name: '保存草稿' }))
  await waitFor(() => expect(close).toHaveBeenCalledTimes(1))
  expect(save).toHaveBeenCalledWith(expect.objectContaining({ departmentId: 'dept', classId: 'class', className: '测试班', grade: '2026级', studentNo: 'S001', courseId: 'course-1', date: '2026-09-06', time: '08:00', period: '1-2', absenceHours: 2 }))
  expect(save.mock.calls[0][0]).not.toHaveProperty('courseName')
  expect(save.mock.calls[0][0]).not.toHaveProperty('status')
})
it.each([['FORBIDDEN_SCOPE', '无权在所选部门'], ['RECORD_ALREADY_EXISTS', '相同记录已存在'], ['VALIDATION_FAILED', '姓名不一致']])('retains form values after %s', async (code, message) => {
  const close = vi.fn()
  save.mockRejectedValue(new ApiError(code === 'RECORD_ALREADY_EXISTS' ? 409 : 403, code, message))
  render(<ManualAttendanceDrawer options={options} initialSemesterId="semester" initialWeekId="week" onClose={close} />)
  await fillForm()
  fireEvent.click(screen.getByRole('button', { name: '保存草稿' }))
  await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent(message))
  expect(screen.getByLabelText('学号')).toHaveValue('S001')
  expect(close).not.toHaveBeenCalled()
})

it('preserves the opened draft version across background refresh and requires a reason', async () => {
 const close = vi.fn()
 update.mockResolvedValue({})
 const editing = { id: 'record', semesterId: 'semester', weekId: 'week', departmentId: 'dept', inspectionTypeId: 'type', studentNo: 'S001', studentName: '虚构同学', classId: 'class', className: '测试班', grade: '2026级', courseId: 'course', courseName: '数据结构', classroomCode: '博学楼 101', occurredAt: '2026-09-06T00:00:00Z', period: '1-2', absenceHours: 2, remark: '', status: 'DRAFT', source: 'EXCEL' as const, rowVersion: 7 }
 const { rerender } = render(<ManualAttendanceDrawer options={options} editing={editing} onClose={close} />)
 rerender(<ManualAttendanceDrawer options={options} editing={{ ...editing, rowVersion: 8 }} onClose={close} />)
 expect(screen.getByLabelText('检查时间')).toHaveValue('08:00')
 fireEvent.click(screen.getByRole('button', { name: '保存修改' }))
 expect(await screen.findByText('请填写或选择此项')).toBeInTheDocument()
 expect(update).not.toHaveBeenCalled()
 fireEvent.change(screen.getByLabelText('修改原因'), { target: { value: '核对原记录' } })
 fireEvent.click(screen.getByRole('button', { name: '保存修改' }))
 await waitFor(() => expect(close).toHaveBeenCalledTimes(1))
 expect(update).toHaveBeenCalledWith(expect.objectContaining({ id: 'record', version: 7, reason: '核对原记录', record: expect.objectContaining({ period: '1-2' }) }))
 expect(save).not.toHaveBeenCalled()
})

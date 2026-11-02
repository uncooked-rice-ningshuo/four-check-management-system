import { render, screen, waitFor } from '@testing-library/react'
import { StrictMode } from 'react'
import { beforeEach, expect, it, vi } from 'vitest'
import { useInspectionEntryOptions } from '../model/queries'
import { ClassroomHygieneDrawer } from './ClassroomHygieneDrawer'

const mutate = vi.fn()
const college = '00000000-0000-0000-0000-000000000000'
let departmentItems: { id: string; label: string }[] = []

vi.mock('../model/queries', () => ({
  useInspectionContext: (query: { kind: string }) => ({
    data: {
      items: query.kind === 'departments'
        ? departmentItems
        : [{ id: query.kind === 'semesters' ? 'semester' : 'week', label: query.kind === 'semesters' ? '测试学期' : '第一周' }],
    },
    isLoading: false,
    isError: false,
  }),
  useInspectionEntryOptions: vi.fn(() => ({ data: [], isLoading: false, isFetching: false })),
  useResponsibleStudentOptions: vi.fn(() => ({ data: [], isLoading: false, isFetching: false })),
  useClassroomHygieneMutation: () => ({ mutateAsync: mutate, isPending: false }),
}))

const entryOptions = vi.mocked(useInspectionEntryOptions)

beforeEach(() => {
  entryOptions.mockClear()
  mutate.mockReset()
})

it('keeps a member on their own department instead of the unavailable college option', async () => {
  departmentItems = [{ id: 'study', label: '学习部' }]
  render(<StrictMode><ClassroomHygieneDrawer inspectionTypeId="type" initialSemesterId="semester" initialWeekId="week" memberDepartmentId="study" onClose={vi.fn()} /></StrictMode>)

  const field = screen.getByRole('combobox', { name: '录入归属部门' })
  expect(field).toBeDisabled()
  await waitFor(() => expect(field.parentElement).toHaveTextContent('学习部'))
  expect(screen.queryByRole('combobox', { name: '业务归属部门' })).not.toBeInTheDocument()
  await waitFor(() => expect(entryOptions).toHaveBeenCalledWith(expect.objectContaining({ kind: 'classes', departmentId: 'study' }), expect.anything()))
  expect(entryOptions.mock.calls.some(call => call[0].departmentId === college)).toBe(false)
})

it('replaces the default college option when the loaded departments do not offer it', async () => {
  departmentItems = [{ id: 'study', label: '学习部' }]
  render(<StrictMode><ClassroomHygieneDrawer inspectionTypeId="type" initialSemesterId="semester" initialWeekId="week" onClose={vi.fn()} /></StrictMode>)

  const field = screen.getByRole('combobox', { name: '业务归属部门' })
  await waitFor(() => expect(field.parentElement).toHaveTextContent('学习部'))
  await waitFor(() => expect(entryOptions).toHaveBeenCalledWith(expect.objectContaining({ kind: 'classes', departmentId: 'study' }), expect.anything()))
  expect(entryOptions.mock.calls.some(call => call[0].departmentId === college)).toBe(false)
})

it('never assigns a department to an administrator-owned record', async () => {
  departmentItems = [{ id: 'study', label: '学习部' }]
  render(<StrictMode><ClassroomHygieneDrawer inspectionTypeId="type" initialSemesterId="semester" initialWeekId="week" isSuperAdmin onClose={vi.fn()} /></StrictMode>)

  expect(screen.queryByRole('combobox', { name: '业务归属部门' })).not.toBeInTheDocument()
  expect(screen.getByText('本次记录归属管理员，不需要选择目标部门')).toBeInTheDocument()
  await waitFor(() => expect(entryOptions).toHaveBeenCalled())
  expect(entryOptions.mock.calls.every(call => !call[0].departmentId)).toBe(true)
})

import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { StrictMode } from 'react'
import { beforeEach, expect, it, vi } from 'vitest'
import type { PublicDormitoryHygieneRecord } from '../../../entities/inspection/model/publicDormitoryHygieneRecord'
import { useInspectionEntryOptions } from '../model/queries'
import { PublicDormitoryHygieneDrawer } from './PublicDormitoryHygieneDrawer'

const mutate = vi.fn()
const college = '00000000-0000-0000-0000-000000000000'
let departmentItems: { id: string; label: string }[] = []
let weekItems: { id: string; label: string }[] = []

vi.mock('../model/queries', () => ({
  useInspectionContext: (query: { kind: string }) => ({
    data: {
      items: query.kind === 'departments'
        ? departmentItems
        : query.kind === 'weeks'
          ? weekItems
          : [{ id: 'semester', label: '测试学期' }],
    },
    isLoading: false,
    isError: false,
  }),
  useInspectionEntryOptions: vi.fn(() => ({ data: [], isLoading: false, isFetching: false })),
  useResponsibleStudentOptions: vi.fn(() => ({ data: [], isLoading: false, isFetching: false })),
  usePublicDormitoryHygieneMutation: () => ({ mutateAsync: mutate, isPending: false }),
}))

const entryOptions = vi.mocked(useInspectionEntryOptions)

beforeEach(() => {
  departmentItems = []
  weekItems = [{ id: 'week', label: '第一周' }]
  entryOptions.mockClear()
  mutate.mockReset()
})

it('keeps a member on their own department instead of the unavailable college option', async () => {
  departmentItems = [{ id: 'dorm', label: '宿管部' }]
  render(<StrictMode><PublicDormitoryHygieneDrawer inspectionTypeId="type" initialSemesterId="semester" initialWeekId="week" memberDepartmentId="dorm" onClose={vi.fn()} /></StrictMode>)

  const field = screen.getByRole('combobox', { name: '录入归属部门' })
  expect(field).toBeDisabled()
  await waitFor(() => expect(field.parentElement).toHaveTextContent('宿管部'))
  await waitFor(() => expect(entryOptions).toHaveBeenCalledWith(expect.objectContaining({ kind: 'classes', departmentId: 'dorm' }), expect.anything()))
  expect(entryOptions.mock.calls.some(call => call[0].departmentId === college && call[1] === true)).toBe(false)
})

it('requires an explicit department when the college option is unavailable', async () => {
  departmentItems = [{ id: 'dorm', label: '宿管部' }]
  render(<StrictMode><PublicDormitoryHygieneDrawer inspectionTypeId="type" initialSemesterId="semester" initialWeekId="week" onClose={vi.fn()} /></StrictMode>)

  const field = screen.getByRole('combobox', { name: '业务归属部门' })
  await waitFor(() => expect(field.parentElement?.querySelector('.ant-select-selection-item')).toBeNull())
  fireEvent.mouseDown(field)
  fireEvent.click(screen.getByText('宿管部', { selector: '.ant-select-item-option-content' }))
  await waitFor(() => expect(field.parentElement).toHaveTextContent('宿管部'))
  await waitFor(() => expect(entryOptions).toHaveBeenCalledWith(expect.objectContaining({ kind: 'classes', departmentId: 'dorm' }), expect.anything()))
  expect(entryOptions.mock.calls.some(call => call[0].departmentId === college && call[1] === true)).toBe(false)
})

it('does not render an unavailable college ID when no department is selectable', async () => {
  departmentItems = []
  render(<StrictMode><PublicDormitoryHygieneDrawer inspectionTypeId="type" initialSemesterId="semester" initialWeekId="week" onClose={vi.fn()} /></StrictMode>)

  const field = screen.getByRole('combobox', { name: '业务归属部门' })
  await waitFor(() => expect(field.parentElement?.querySelector('.ant-select-selection-item')).toBeNull())
  expect(entryOptions.mock.calls.some(call => call[0].departmentId === college && call[1] === true)).toBe(false)
})

it('keeps a member editing record on its department when the week changes', async () => {
  departmentItems = [{ id: 'dorm', label: '宿管部' }]
  weekItems = [{ id: 'week', label: '第一周' }, { id: 'week-2', label: '第二周' }]
  const editing: PublicDormitoryHygieneRecord = {
    id: 'record', typeCode: 'PUBLIC_DORMITORY_HYGIENE', semesterId: 'semester', weekId: 'week', inspectionTypeId: 'type', departmentId: 'dorm',
    details: { responsibleDormitoryIds: ['dormitory'], date: '2026-10-01', result: 'QUALIFIED', responsibleClassIds: ['class'], responsibleStudentIds: ['student'] },
    responsibleClasses: [{ id: 'class', label: '测试班' }], responsibleStudents: [{ id: 'student', label: '测试学生' }], responsibleDormitories: [{ id: 'dormitory', label: '测试寝室' }],
    status: 'DRAFT', source: 'MANUAL', rowVersion: 1, createdBy: 'member', submitterName: '成员', canEdit: true,
  }
  render(<StrictMode><PublicDormitoryHygieneDrawer editing={editing} inspectionTypeId="type" memberDepartmentId="dorm" onClose={vi.fn()} /></StrictMode>)

  const departmentField = screen.getByRole('combobox', { name: '录入归属部门' })
  await waitFor(() => expect(departmentField.parentElement).toHaveTextContent('宿管部'))
  fireEvent.mouseDown(screen.getByRole('combobox', { name: '录入周次' }))
  fireEvent.click(screen.getByText('第二周', { selector: '.ant-select-item-option-content' }))

  await waitFor(() => expect(departmentField.parentElement).toHaveTextContent('宿管部'))
  await waitFor(() => expect(entryOptions).toHaveBeenCalledWith(expect.objectContaining({ kind: 'classes', weekId: 'week-2', departmentId: 'dorm' }), expect.anything()))
})

it('never assigns a department to an administrator-owned record', async () => {
  departmentItems = [{ id: 'dorm', label: '宿管部' }]
  render(<StrictMode><PublicDormitoryHygieneDrawer inspectionTypeId="type" initialSemesterId="semester" initialWeekId="week" isSuperAdmin onClose={vi.fn()} /></StrictMode>)

  expect(screen.queryByRole('combobox', { name: '业务归属部门' })).not.toBeInTheDocument()
  expect(screen.getByText('本次记录归属管理员，不需要选择目标部门')).toBeInTheDocument()
  await waitFor(() => expect(entryOptions).toHaveBeenCalled())
  expect(entryOptions.mock.calls.every(call => !call[0].departmentId)).toBe(true)
})

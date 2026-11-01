import { MemoryRouter } from 'react-router-dom'
import { render, screen } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import { EveningSelfStudyPage } from './EveningSelfStudyPage'

vi.mock('../../features/department-submission/ui/DepartmentSubmissionActions', () => ({ DepartmentSubmissionActions: () => null }))
const records = vi.fn()
vi.mock('../../app/providers/authContext', () => ({ useAuth: () => ({ user: { isSuperAdmin: true } }) }))
vi.mock('../../features/attendance/hooks/useAttendancePermissions', () => ({ useAttendancePermissions: () => ({ data: { permissions: [] } }), useCurrentAcademicWeek: () => ({ data: { week: null }, isLoading: false, isError: false }) }))
vi.mock('../../features/evening-self-study/model/queries', () => ({
  useInspectionContext: () => ({ data: { inspectionTypeId: 'type', items: [{ id: 'semester', label: '可选学期' }] }, isLoading: false }),
  useEveningSelfStudyRecords: () => records(),
  useEveningSelfStudyRecord: () => ({}), useEveningSelfStudyDelete: () => ({}),
}))

it('exposes grade and student keyword filters while keeping an empty semester unselected', () => {
  records.mockReturnValue({ isError: true, error: new Error('offline'), isLoading: false, refetch: vi.fn() })
  render(<MemoryRouter><EveningSelfStudyPage /></MemoryRouter>)
  expect(screen.getByRole('combobox', { name: '学期' })).toHaveValue('')
  expect(screen.getByRole('combobox', { name: '年级' })).toBeInTheDocument()
  expect(screen.getByPlaceholderText('输入学号或姓名')).toBeInTheDocument()
})

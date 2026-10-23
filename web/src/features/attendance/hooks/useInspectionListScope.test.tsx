import type { ReactNode } from 'react'
import { act, renderHook } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, expect, it, vi } from 'vitest'
import { useInspectionListScope } from './useInspectionListScope'

const currentWeek = vi.fn()
vi.mock('./useAttendancePermissions', () => ({ useCurrentAcademicWeek: () => currentWeek() }))

const semesters = [{ id: 'semester-current', label: '当前学期' }, { id: 'semester-old', label: '历史学期' }]
const weeks = [{ id: 'week-current', label: '当前周' }, { id: 'week-old', label: '历史周' }]

beforeEach(() => currentWeek.mockReturnValue({ data: { week: { id: 'week-current', semesterId: 'semester-current' } } }))

it('defaults to the authorized current teaching week and keeps an explicit choice when returning', () => {
  const wrapper = ({ children }: { children: ReactNode }) => <MemoryRouter>{children}</MemoryRouter>
  const { result } = renderHook(() => useInspectionListScope(semesters), { wrapper })
  expect(result.current.resolve(weeks).filters).toMatchObject({ semesterId: 'semester-current', weekId: 'week-current' })
  act(() => result.current.update({ semesterId: 'semester-old', weekId: 'week-old', page: 1, pageSize: 20 }))
  expect(result.current.resolve(weeks).filters).toMatchObject({ semesterId: 'semester-old', weekId: 'week-old' })
  currentWeek.mockReturnValue({ data: { week: { id: 'week-current', semesterId: 'semester-current' } } })
  expect(result.current.resolve(weeks).filters.weekId).toBe('week-old')
})

it('does not substitute a first semester or week when the calendar has no current week', () => {
  currentWeek.mockReturnValue({ data: { week: null } })
  const wrapper = ({ children }: { children: ReactNode }) => <MemoryRouter>{children}</MemoryRouter>
  const { result } = renderHook(() => useInspectionListScope(semesters), { wrapper })
  expect(result.current.resolve(weeks).ready).toBe(false)
  expect(result.current.resolve(weeks).filters.semesterId).toBeUndefined()
  expect(result.current.resolve(weeks).filters.weekId).toBeUndefined()
})

it('rejects a stale URL choice instead of widening the list query', () => {
  const wrapper = ({ children }: { children: ReactNode }) => <MemoryRouter initialEntries={['/?semesterId=semester-old&weekId=missing-week']}>{children}</MemoryRouter>
  const { result } = renderHook(() => useInspectionListScope(semesters), { wrapper })
  expect(result.current.resolve(weeks).invalidSelection).toBe(true)
})

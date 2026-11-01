import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { InspectionImportScopePanel } from './InspectionImportScopePanel'
import type { SupportedImportType } from '../../../shared/api/inspectionImportApi'

const { currentWeek, useImportContext } = vi.hoisted(() => ({ currentWeek: vi.fn(), useImportContext: vi.fn() }))

vi.mock('../../attendance/hooks/useAttendancePermissions', () => ({ useCurrentAcademicWeek: () => currentWeek() }))
vi.mock('../hooks/useImportContext', () => ({ useImportContext: (...args: unknown[]) => useImportContext(...args) }))
vi.mock('../hooks/useImportBatch', () => ({ useImportBatch: () => ({ data: undefined, isLoading: false, isError: false }) }))
vi.mock('./AttendanceImportDrawer', () => ({
  AttendanceImportDrawer: ({ context }: { context: { semesterId: string; weekId: string } }) => <div role="dialog" aria-label="批量导入">{context.semesterId} · {context.weekId}</div>,
}))

const inspectionTypes: SupportedImportType[] = [
  'ATTENDANCE_RATE', 'UNATTENDED_CLASSROOM', 'MORNING_ATTENDANCE', 'LATE_RETURN',
  'DORMITORY_HYGIENE', 'CLASSROOM_HYGIENE', 'PUBLIC_DORMITORY_HYGIENE', 'EVENING_SELF_STUDY',
]

describe('八类检查的统一导入范围', () => {
  beforeEach(() => {
    currentWeek.mockReturnValue({ data: { week: { id: 'week-current', semesterId: 'semester-current' } }, isLoading: false, isError: false })
    useImportContext.mockImplementation((query: { kind: string }) => ({
      data: { items: query.kind === 'semesters'
        ? [{ id: 'semester-current', label: '当前学期' }]
        : query.kind === 'weeks'
          ? [{ id: 'week-current', label: '第 5 周' }]
          : [] },
      isLoading: false,
      isError: false,
    }))
  })

  it.each(inspectionTypes)('%s uses the authorized current semester and week as its import defaults', typeCode => {
    render(<InspectionImportScopePanel typeCode={typeCode} inspectionTypeId="inspection-type" isSuperAdmin onBatchIdChange={vi.fn()} onClose={vi.fn()} />)

    const prepareButton = screen.getByRole('button', { name: '准备导入' })
    expect(prepareButton).toBeEnabled()
    fireEvent.click(prepareButton)
    expect(screen.getByRole('dialog', { name: '批量导入' })).toHaveTextContent('semester-current · week-current')
  })
})

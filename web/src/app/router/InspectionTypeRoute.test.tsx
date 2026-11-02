import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { InspectionTypeRoute } from './InspectionTypeRoute'

const permissionsQuery = vi.fn()
const contextQuery = vi.fn()

vi.mock('../providers/authContext', () => ({ useAuth: () => ({ user: { isSuperAdmin: false } }) }))
vi.mock('../../features/attendance/hooks/useAttendancePermissions', () => ({ useAttendancePermissions: () => permissionsQuery() }))
vi.mock('../../features/course-inspection/model/queries', () => ({ useInspectionContext: () => contextQuery() }))

function renderAt(state?: { fromLogin: boolean }) {
  return render(<MemoryRouter initialEntries={[{ pathname: '/inspections/morning-attendance', state }]}><Routes>
    <Route path="/inspections/morning-attendance" element={<InspectionTypeRoute typeCode="MORNING_ATTENDANCE"><div>早签页面</div></InspectionTypeRoute>} />
    <Route path="/" element={<div>统一工作台</div>} />
  </Routes></MemoryRouter>)
}

describe('InspectionTypeRoute', () => {
  beforeEach(() => {
    permissionsQuery.mockReset()
    contextQuery.mockReset()
    contextQuery.mockReturnValue({ data: { inspectionTypeId: 'type-morning' }, isLoading: false })
  })

  it('renders the page when the type is granted', () => {
    permissionsQuery.mockReturnValue({ data: { permissions: [{ action: 'INSPECTION_VIEW', effect: 'ALLOW', scope: { inspectionTypeId: 'type-morning' }, sources: [] }] }, isLoading: false })
    renderAt()

    expect(screen.getByText('早签页面')).toBeInTheDocument()
  })

  it('shows a 403 page with a way back for a direct URL the account cannot view', () => {
    permissionsQuery.mockReturnValue({ data: { permissions: [{ action: 'INSPECTION_VIEW', effect: 'ALLOW', scope: { inspectionTypeId: 'type-classroom' }, sources: [] }] }, isLoading: false })
    renderAt()

    expect(screen.queryByText('早签页面')).not.toBeInTheDocument()
    expect(screen.getByText('403')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: '返回工作台' })).toHaveAttribute('href', '/')
  })

  it('returns to the workbench instead of a dead end when login bounces back to a forbidden page', () => {
    permissionsQuery.mockReturnValue({ data: { permissions: [{ action: 'INSPECTION_VIEW', effect: 'ALLOW', scope: { inspectionTypeId: 'type-classroom' }, sources: [] }] }, isLoading: false })
    renderAt({ fromLogin: true })

    expect(screen.getByText('统一工作台')).toBeInTheDocument()
    expect(screen.queryByText('403')).not.toBeInTheDocument()
  })
})

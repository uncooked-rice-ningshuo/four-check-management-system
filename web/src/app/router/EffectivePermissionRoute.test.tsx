import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { EffectivePermissionRoute } from './EffectivePermissionRoute'

let currentUser = { isSuperAdmin: false }
const permissionsQuery = vi.fn()

vi.mock('../providers/authContext', () => ({ useAuth: () => ({ user: currentUser }) }))
vi.mock('../../features/attendance/hooks/useAttendancePermissions', () => ({
  useAttendancePermissions: () => permissionsQuery(),
}))

describe('EffectivePermissionRoute', () => {
  beforeEach(() => {
    currentUser = { isSuperAdmin: false }
    permissionsQuery.mockReset()
  })

  it('renders the protected business page for an effective action grant', () => {
    permissionsQuery.mockReturnValue({ data: { permissions: [{ action: 'INSPECTION_VIEW', effect: 'ALLOW', scope: {}, sources: [] }] }, isLoading: false })
    render(<MemoryRouter><EffectivePermissionRoute action="INSPECTION_VIEW"><div>业务页面</div></EffectivePermissionRoute></MemoryRouter>)

    expect(screen.getByText('业务页面')).toBeInTheDocument()
  })

  it('shows an explicit 403 page when the action is not granted', () => {
    permissionsQuery.mockReturnValue({ data: { permissions: [{ action: 'INSPECTION_VIEW', effect: 'DENY', scope: {}, sources: [] }] }, isLoading: false })
    render(<MemoryRouter><EffectivePermissionRoute action="INSPECTION_VIEW"><div>业务页面</div></EffectivePermissionRoute></MemoryRouter>)

    expect(screen.queryByText('业务页面')).not.toBeInTheDocument()
    expect(screen.getByText('403')).toBeInTheDocument()
    expect(screen.getByText('无权访问该业务')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: '返回工作台' })).toHaveAttribute('href', '/')
  })

  it('rejects a grant fully covered by a deny', () => {
    permissionsQuery.mockReturnValue({ data: { permissions: [
      { action: 'INSPECTION_VIEW', effect: 'ALLOW', scope: {}, sources: [] },
      { action: 'INSPECTION_VIEW', effect: 'DENY', scope: {}, sources: [] },
    ] }, isLoading: false })
    render(<MemoryRouter><EffectivePermissionRoute action="INSPECTION_VIEW"><div>业务页面</div></EffectivePermissionRoute></MemoryRouter>)
    expect(screen.getByText('403')).toBeInTheDocument()
  })

  it('waits for permissions before deciding access', () => {
    permissionsQuery.mockReturnValue({ isLoading: true })
    render(<MemoryRouter><EffectivePermissionRoute action="INSPECTION_VIEW"><div>业务页面</div></EffectivePermissionRoute></MemoryRouter>)

    expect(screen.getByRole('status')).toHaveTextContent('正在核对访问权限')
  })

  it('allows the super administrator without depending on scoped grants', () => {
    currentUser = { isSuperAdmin: true }
    permissionsQuery.mockReturnValue({ isError: true, isLoading: false })
    render(<MemoryRouter><EffectivePermissionRoute action="INSPECTION_VIEW"><div>业务页面</div></EffectivePermissionRoute></MemoryRouter>)

    expect(screen.getByText('业务页面')).toBeInTheDocument()
  })
})

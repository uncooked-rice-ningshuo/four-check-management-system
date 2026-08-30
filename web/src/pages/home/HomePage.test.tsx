import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { HomePage } from './HomePage'

vi.mock('../../app/providers/authContext', () => ({ useAuth: () => ({ user: { name: '普通成员', loginName: 'member', isSuperAdmin: false }, signOut: vi.fn() }) }))

describe('HomePage', () => {
  it('offers ordinary users an entry to the inspection workspace without using /admin', () => {
    render(<MemoryRouter><HomePage /></MemoryRouter>)
    expect(screen.getByRole('link', { name: '进入日常检查业务' })).toHaveAttribute('href', '/inspections/attendance')
    expect(screen.queryByRole('link', { name: /导入|审核/ })).not.toBeInTheDocument()
  })
})

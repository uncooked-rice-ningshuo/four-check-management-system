import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { HomePage } from './HomePage'

let currentUser = { name: '普通成员', loginName: 'member', systemRole: 'MEMBER', isSuperAdmin: false }
vi.mock('../../app/providers/authContext', () => ({ useAuth: () => ({ user: currentUser }) }))
vi.mock('../../features/phase-two-dashboard/ui/PhaseTwoDashboard', () => ({ PhaseTwoDashboard: ({ userName }: { userName: string }) => <div>业务工作台：{userName}</div> }))
vi.mock('../admin/AdminDashboardPage', () => ({ AdminDashboardPage: ({ embedded }: { embedded: boolean }) => <div>管理员工作台内容：{String(embedded)}</div> }))

describe('HomePage', () => {
  beforeEach(() => {
    currentUser = { name: '普通成员', loginName: 'member', systemRole: 'MEMBER', isSuperAdmin: false }
	})

	it('shows the shared business workbench and embedded management area to administrators', () => {
		currentUser = { name: '系统管理员', loginName: 'admin', systemRole: 'ADMIN', isSuperAdmin: false }
		render(<HomePage />)

		expect(screen.getByText('业务工作台：系统管理员')).toBeInTheDocument()
		expect(screen.getByText('管理员工作台内容：true')).toBeInTheDocument()
	})

	it('shows the same business workbench without management cards to ordinary members', () => {
		render(<HomePage />)
		expect(screen.getByText('业务工作台：普通成员')).toBeInTheDocument()
		expect(screen.queryByText(/管理员工作台内容/)).not.toBeInTheDocument()
	})
})

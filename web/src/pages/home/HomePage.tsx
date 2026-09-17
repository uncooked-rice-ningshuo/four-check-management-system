import { useAuth } from '../../app/providers/authContext'
import { PhaseTwoDashboard } from '../../features/phase-two-dashboard/ui/PhaseTwoDashboard'
import { AdminDashboardPage } from '../admin/AdminDashboardPage'

export function HomePage() {
	const { user } = useAuth()
	const isAdministrator = Boolean(user?.isSuperAdmin || user?.systemRole === 'ADMIN')
	return <>
		<PhaseTwoDashboard userName={user?.name} />
		{isAdministrator && <AdminDashboardPage embedded />}
	</>
}

import { useQuery } from '@tanstack/react-query'
import { getPhaseTwoDashboard } from '../api/dashboardClient'

export function usePhaseTwoDashboard() {
	return useQuery({ queryKey: ['dashboard', 'workbench'], queryFn: getPhaseTwoDashboard })
}

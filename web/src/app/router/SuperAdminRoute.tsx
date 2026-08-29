import { Navigate } from 'react-router-dom'
import type { ReactNode } from 'react'
import { useAuth } from '../providers/authContext'

export function SuperAdminRoute({ children }: { children: ReactNode }) {
  const { user } = useAuth()
	return user?.isSuperAdmin || user?.systemRole === 'ADMIN' ? children : <Navigate to="/" replace />
}

export function SuperAdminOnly({ children }: { children: ReactNode }) {
  const { user } = useAuth()
  return user?.isSuperAdmin ? children : <Navigate to="/admin" replace />
}

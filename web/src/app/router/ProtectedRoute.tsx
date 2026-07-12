import { Spin } from 'antd'
import { Navigate, useLocation } from 'react-router-dom'
import type { ReactNode } from 'react'
import { useAuth } from '../providers/authContext'

// ProtectedRoute 会等待刷新 Cookie 的会话恢复结束后再决定是否跳转，
// 避免仍有有效会话的用户在加载期间被短暂送回登录页。
export function ProtectedRoute({ children }: { children: ReactNode }) {
  const { user, loading } = useAuth()
  const location = useLocation()
  if (loading) return <div className="loading"><Spin size="large" /></div>
  return user ? children : <Navigate to="/login" state={{ from: location.pathname }} replace />
}

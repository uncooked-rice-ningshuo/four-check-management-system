import { Result } from 'antd'
import type { ReactNode } from 'react'
import { Link, Navigate, useLocation } from 'react-router-dom'
import { useAttendancePermissions } from '../../features/attendance/hooks/useAttendancePermissions'
import { hasNavigableInspectionAction } from '../../features/attendance/hooks/hasNavigableInspectionAction'
import { useAuth } from '../providers/authContext'

export function EffectivePermissionRoute({ action, children }: { action: string; children: ReactNode }) {
  const { user } = useAuth()
  const permissions = useAttendancePermissions()
  const fromLogin = (useLocation().state as { fromLogin?: boolean } | null)?.fromLogin === true

  if (user?.isSuperAdmin) return children
  if (permissions.isLoading) return <div role="status">正在核对访问权限</div>
  if (permissions.isError) {
    return <Result status="warning" title="暂时无法核对访问权限" subTitle="请稍后重试，或返回工作台选择其他功能。" extra={<Link to="/">返回工作台</Link>} />
  }

  const granted = hasNavigableInspectionAction(permissions.data?.permissions, action)
  if (!granted) {
    // 登录后回到的旧地址可能属于上一个账号，直接去工作台而不是让用户面对 403。
    if (fromLogin) return <Navigate to="/" replace />
    return <Result status="403" title="403" subTitle="无权访问该业务" extra={<Link to="/">返回工作台</Link>} />
  }
  return children
}

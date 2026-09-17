import { Result } from 'antd'
import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { useAttendancePermissions } from '../../features/attendance/hooks/useAttendancePermissions'
import { hasNavigableInspectionAction } from '../../features/attendance/hooks/hasNavigableInspectionAction'
import { useAuth } from '../providers/authContext'

export function EffectivePermissionRoute({ action, children }: { action: string; children: ReactNode }) {
  const { user } = useAuth()
  const permissions = useAttendancePermissions()

  if (user?.isSuperAdmin) return children
  if (permissions.isLoading) return <div role="status">正在核对访问权限</div>
  if (permissions.isError) {
    return <Result status="warning" title="暂时无法核对访问权限" subTitle="请稍后重试，或返回工作台选择其他功能。" extra={<Link to="/">返回工作台</Link>} />
  }

  const granted = hasNavigableInspectionAction(permissions.data?.permissions, action)
  if (!granted) {
    return <Result status="403" title="403" subTitle="无权访问该业务" extra={<Link to="/">返回工作台</Link>} />
  }
  return children
}

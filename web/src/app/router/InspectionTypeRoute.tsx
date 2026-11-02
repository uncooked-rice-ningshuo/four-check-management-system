import { Button, Result } from 'antd'
import type { ReactNode } from 'react'
import { Link, Navigate, useLocation } from 'react-router-dom'
import { useInspectionContext } from '../../features/course-inspection/model/queries'
import { useAttendancePermissions } from '../../features/attendance/hooks/useAttendancePermissions'
import { hasNavigableInspectionAction } from '../../features/attendance/hooks/hasNavigableInspectionAction'
import { useAuth } from '../providers/authContext'

export function InspectionTypeRoute({typeCode,children}:{typeCode:string;children:ReactNode}) {
 const {user}=useAuth()
 const permissions=useAttendancePermissions()
 const context=useInspectionContext({typeCode,purpose:'view',kind:'semesters'})
 const fromLogin=(useLocation().state as {fromLogin?:boolean}|null)?.fromLogin===true
 if(user?.isSuperAdmin)return children
 if(context.isLoading||permissions.isLoading)return <div role="status">正在核对访问权限</div>
 if(context.isError||permissions.isError)return <Result status="warning" title="暂时无法核对访问权限" extra={<Button onClick={()=>{void context.refetch();void permissions.refetch()}}>重试</Button>}/>
 if(!context.data?.inspectionTypeId||!hasNavigableInspectionAction(permissions.data?.permissions,'INSPECTION_VIEW',context.data.inspectionTypeId)){
  // 登录后回到的旧地址可能属于上一个账号，直接去工作台而不是让用户面对 403。
  if(fromLogin)return <Navigate to="/" replace />
  return <Result status="403" title="403" subTitle="无权访问该业务" extra={<Link to="/">返回工作台</Link>}/>
 }
 return children
}

import { Alert, Button, Drawer, Spin } from 'antd'
import { useCurrentAcademicWeek } from '../../attendance/hooks/useAttendancePermissions'
import { useInspectionContext } from '../model/queries'
import { DormitoryHygieneDrawer } from './DormitoryHygieneDrawer'

/** Resolve the authorized default before mounting the form; later refreshes never reset entered values. */
export function DormitoryHygieneCreatePanel({inspectionTypeId,onClose,memberDepartmentId}:{inspectionTypeId:string;onClose:()=>void;memberDepartmentId?:string}) {
 const current=useCurrentAcademicWeek()
 const semesters=useInspectionContext({purpose:'entry',kind:'semesters'})
 const candidate=current.data?.week
 const semesterId=semesters.data?.items.some(item=>item.id===candidate?.semesterId)?candidate?.semesterId:undefined
 const weeks=useInspectionContext({purpose:'entry',kind:'weeks',semesterId},!!semesterId)
 const queries=[current,semesters,...(semesterId?[weeks]:[])]
 if(queries.some(query=>query.isError))return <Drawer open title="新增宿舍卫生记录" onClose={onClose}><Alert type="error" title="录入上下文加载失败，请重试。" action={<Button onClick={()=>queries.forEach(query=>{if(query.isError)void query.refetch()})}>重试</Button>}/></Drawer>
 if(queries.some(query=>query.isLoading))return <Drawer open title="新增宿舍卫生记录" onClose={onClose}><Spin/></Drawer>
 const weekId=weeks.data?.items.some(item=>item.id===candidate?.id)?candidate?.id:undefined
 return <DormitoryHygieneDrawer inspectionTypeId={inspectionTypeId} initialSemesterId={semesterId} initialWeekId={weekId} memberDepartmentId={memberDepartmentId} onClose={onClose}/>
}

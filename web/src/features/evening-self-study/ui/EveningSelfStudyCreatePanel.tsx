import { Alert, Button, Drawer, Spin } from 'antd'
import { useCurrentAcademicWeek } from '../../attendance/hooks/useAttendancePermissions'
import { useInspectionContext } from '../model/queries'
import { EveningSelfStudyDrawer } from './EveningSelfStudyDrawer'

export function EveningSelfStudyCreatePanel({ inspectionTypeId, memberDepartmentId, onClose }: { inspectionTypeId: string; memberDepartmentId?: string; onClose: () => void }) {
 const current = useCurrentAcademicWeek()
 const semesters = useInspectionContext({ purpose: 'entry', kind: 'semesters' })
 const candidate = current.data?.week
 const semesterId = semesters.data?.items.some(item => item.id === candidate?.semesterId) ? candidate?.semesterId : undefined
 const weeks = useInspectionContext({ purpose: 'entry', kind: 'weeks', semesterId }, !!semesterId)
 const queries = [current, semesters, ...(semesterId ? [weeks] : [])]
 if (queries.some(query => query.isError)) return <Drawer open title="新增晚自习缺勤记录" onClose={onClose}><Alert type="error" title="录入上下文加载失败，请重试。" action={<Button onClick={() => queries.forEach(query => { if (query.isError) void query.refetch() })}>重试</Button>} /></Drawer>
 if (queries.some(query => query.isLoading)) return <Drawer open title="新增晚自习缺勤记录" onClose={onClose}><Spin /></Drawer>
 const weekId = weeks.data?.items.some(item => item.id === candidate?.id) ? candidate?.id : undefined
 return <EveningSelfStudyDrawer inspectionTypeId={inspectionTypeId} initialSemesterId={semesterId} initialWeekId={weekId} memberDepartmentId={memberDepartmentId} onClose={onClose} />
}

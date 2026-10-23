import { render, screen } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import { MorningAttendanceCreatePanel } from './MorningAttendanceCreatePanel'
const current=vi.fn()
vi.mock('../../attendance/hooks/useAttendancePermissions',()=>({useCurrentAcademicWeek:()=>current()}))
vi.mock('../model/queries',()=>({useInspectionContext:(q:{kind:string})=>({data:{items:q.kind==='semesters'?[{id:'semester',label:'学期'}]:[{id:'week',label:'周次'}]},isLoading:false})}))
vi.mock('./MorningAttendanceDrawer',()=>({MorningAttendanceDrawer:(p:{initialSemesterId?:string;initialWeekId?:string})=><div>{p.initialSemesterId??'未选学期'} / {p.initialWeekId??'未选周次'}</div>}))
it('defaults only to the authorized current week',()=>{
 current.mockReturnValue({data:{week:{id:'week',semesterId:'semester'}},isLoading:false})
 render(<MorningAttendanceCreatePanel inspectionTypeId="type" onClose={vi.fn()}/>)
 expect(screen.getByText('semester / week')).toBeInTheDocument()
})
it('does not substitute the first option when no current week exists',()=>{
 current.mockReturnValue({data:{week:null},isLoading:false})
 render(<MorningAttendanceCreatePanel inspectionTypeId="type" onClose={vi.fn()}/>)
 expect(screen.getByText('未选学期 / 未选周次')).toBeInTheDocument()
})

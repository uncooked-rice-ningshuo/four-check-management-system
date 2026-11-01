import { MemoryRouter } from 'react-router-dom'
import { render, screen } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'
import { MorningAttendancePage } from './MorningAttendancePage'
const pageState=vi.hoisted(()=>({currentWeek:false,plannedDates:[] as string[],isDefault:true}))
vi.mock('../../features/department-submission/ui/DepartmentSubmissionActions', () => ({ DepartmentSubmissionActions: () => null }))
const records=vi.fn()
vi.mock('../../app/providers/authContext',()=>({useAuth:()=>({user:{isSuperAdmin:true}})}))
vi.mock('../../features/attendance/hooks/useAttendancePermissions',()=>({useAttendancePermissions:()=>({data:{permissions:[]}}),useCurrentAcademicWeek:()=>({data:{week:pageState.currentWeek?{id:'week',semesterId:'semester'}:null},isLoading:false,isError:false})}))
vi.mock('../../features/morning-attendance/model/queries',()=>({
 useInspectionContext:(query:{kind:string})=>({data:{inspectionTypeId:'type',items:query.kind==='semesters'?[{id:'semester',label:'可选学期'}]:query.kind==='weeks'?[{id:'week',label:'第 1 周'}]:[{id:'2026',label:'2026级'}]},isLoading:false}),
 useMorningAttendanceRecords:()=>records(),
 useMorningSchedule:()=>({data:pageState.currentWeek?{plannedDates:pageState.plannedDates,isDefault:pageState.isDefault}:undefined,isError:false,isLoading:false,refetch:vi.fn()}),
 useMorningAttendanceRecord:()=>({}),useMorningAttendanceDelete:()=>({}),
}))
beforeEach(()=>{pageState.currentWeek=false;pageState.plannedDates=[];pageState.isDefault=true})
it('distinguishes a failed list from an empty successful list without choosing the first semester',()=>{
 records.mockReturnValue({isError:true,error:new Error('offline'),isLoading:false,refetch:vi.fn()})
 render(<MemoryRouter><MorningAttendancePage/></MemoryRouter>)
 expect(screen.getByText('记录加载失败，请重试。')).toBeInTheDocument()
 expect(screen.queryByText('当前范围暂无早锻炼/早签记录')).not.toBeInTheDocument()
 expect(screen.getByRole('combobox',{name:'学期'})).toHaveValue('')
 expect(screen.getByRole('combobox',{name:'年级'})).toBeInTheDocument()
 expect(screen.getByPlaceholderText('输入学号或姓名')).toBeInTheDocument()
})

it('does not expose persistent week-plan settings from the inspection page',()=>{
 pageState.currentWeek=true
 pageState.plannedDates=['2026-11-02','2026-11-03']
 render(<MemoryRouter><MorningAttendancePage/></MemoryRouter>)
 expect(screen.queryByText(/本周早签计划：/)).not.toBeInTheDocument()
 expect(screen.queryByRole('button',{name:'查看/配置计划'})).not.toBeInTheDocument()
})

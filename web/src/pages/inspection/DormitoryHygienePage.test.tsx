import { MemoryRouter } from 'react-router-dom'
import { render, screen } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import { DormitoryHygienePage } from './DormitoryHygienePage'
vi.mock('../../features/department-submission/ui/DepartmentSubmissionActions', () => ({ DepartmentSubmissionActions: () => null }))
const records=vi.fn()
vi.mock('../../app/providers/authContext',()=>({useAuth:()=>({user:{isSuperAdmin:true}})}))
vi.mock('../../features/attendance/hooks/useAttendancePermissions',()=>({useAttendancePermissions:()=>({data:{permissions:[]}}),useCurrentAcademicWeek:()=>({data:{week:null},isLoading:false,isError:false})}))
vi.mock('../../features/dormitory-hygiene/model/queries',()=>({
 useInspectionContext:()=>({data:{inspectionTypeId:'type',items:[{id:'semester',label:'可选学期'}]},isLoading:false}),
 useDormitoryHygieneRecords:()=>records(),
 useDormitoryHygieneRecord:()=>({}),useDormitoryHygieneDelete:()=>({}),
}))
it('distinguishes a failed list from an empty successful list without choosing the first semester',()=>{
 records.mockReturnValue({isError:true,error:new Error('offline'),isLoading:false,refetch:vi.fn()})
 render(<MemoryRouter><DormitoryHygienePage/></MemoryRouter> )
 expect(screen.getByText('记录加载失败，请重试。')).toBeInTheDocument()
 expect(screen.queryByText('当前范围暂无宿舍卫生记录')).not.toBeInTheDocument()
 expect(screen.getByRole('combobox',{name:'学期'})).toHaveValue('')
})

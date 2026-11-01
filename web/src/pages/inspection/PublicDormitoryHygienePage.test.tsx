import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { expect, it, vi } from 'vitest'
import { PublicDormitoryHygienePage } from './PublicDormitoryHygienePage'
const records=vi.fn()
vi.mock('../../app/providers/authContext',()=>({useAuth:()=>({user:{isSuperAdmin:true}})}))
vi.mock('../../features/attendance/hooks/useAttendancePermissions',()=>({useAttendancePermissions:()=>({data:{permissions:[]}}),useCurrentAcademicWeek:()=>({data:{week:null},isLoading:false,isError:false})}))
vi.mock('../../features/public-dormitory-hygiene/model/queries',()=>({
 useInspectionContext:()=>({data:{inspectionTypeId:'type',items:[{id:'semester',label:'可选学期'}]},isLoading:false}),
 usePublicDormitoryHygieneRecords:()=>records(),
 usePublicDormitoryHygieneRecord:()=>({}),usePublicDormitoryHygieneDelete:()=>({}),
}))
it('distinguishes a failed list from an empty successful list without choosing the first semester',()=>{
 records.mockReturnValue({isError:true,error:new Error('offline'),isLoading:false,refetch:vi.fn()})
 render(<QueryClientProvider client={new QueryClient()}><MemoryRouter><PublicDormitoryHygienePage/></MemoryRouter></QueryClientProvider> )
 expect(screen.getByText('记录加载失败，请重试。')).toBeInTheDocument()
 expect(screen.queryByText('当前范围暂无公共宿舍卫生记录')).not.toBeInTheDocument()
 expect(screen.getByRole('combobox',{name:'学期'})).toHaveValue('')
 expect(screen.getByRole('combobox',{name:'年级'})).toBeInTheDocument()
 expect(screen.getByPlaceholderText('输入学号或姓名')).toBeInTheDocument()
 expect(screen.getByRole('button',{name:'提交记录'})).toBeInTheDocument()
 expect(screen.getByRole('button',{name:'检查提交批次'})).toBeInTheDocument()
})

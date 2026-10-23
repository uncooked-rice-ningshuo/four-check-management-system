import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { StrictMode } from 'react'
import { expect, it, vi } from 'vitest'
import type { MorningAttendanceRecord } from '../../../entities/inspection/model/morningAttendanceRecord'
import { MorningAttendanceDrawer } from './MorningAttendanceDrawer'
const mutate=vi.fn()
vi.mock('../model/queries',()=>({
 useInspectionContext:()=>({data:{items:[],inspectionTypeId:'type'},isLoading:false}),
 useInspectionEntryOptions:()=>({data:[],isLoading:false}),
 useMorningAttendanceMutation:()=>({mutateAsync:mutate,isPending:false}),
}))
const record:MorningAttendanceRecord={id:'one',semesterId:'semester',weekId:'week',inspectionTypeId:'type',departmentId:'department',typeCode:'MORNING_ATTENDANCE',studentNumber:'001',studentName:'学生甲',className:'班级甲',grade:'2026',status:'DRAFT',source:'MANUAL',rowVersion:2,createdBy:'actor',canEdit:true,details:{classId:'class',studentId:'student',date:'2026-09-03',remark:'原备注'}}
it('preserves initial edit values and failed input under StrictMode, then retries the original version',async()=>{
 mutate.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce(record)
 const close=vi.fn()
 render(<StrictMode><MorningAttendanceDrawer editing={record} inspectionTypeId="type" onClose={close}/></StrictMode>)
 expect(screen.getByLabelText('检查日期')).toHaveValue('2026-09-03')
 fireEvent.change(screen.getByLabelText('备注'),{target:{value:'修改内容'}})
 fireEvent.change(screen.getByLabelText('修改原因'),{target:{value:'核对'}})
 fireEvent.click(screen.getByRole('button',{name:'保存修改'}))
 await screen.findByText('保存失败，请稍后重试。')
 expect(screen.getByLabelText('备注')).toHaveValue('修改内容')
 fireEvent.click(screen.getByRole('button',{name:'保存修改'}))
 await waitFor(()=>expect(close).toHaveBeenCalledOnce())
 expect(mutate.mock.calls[1][0]).toMatchObject({id:'one',version:2,reason:'核对',record:{details:{remark:'修改内容'}}})
})
it('opens a new empty form without reading an editing record',()=>{
 render(<StrictMode><MorningAttendanceDrawer inspectionTypeId="type" onClose={vi.fn()}/></StrictMode>)
 expect(screen.getByLabelText('检查日期')).toHaveValue('')
 expect(screen.queryByLabelText('修改原因')).not.toBeInTheDocument()
})

import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { StrictMode } from 'react'
import { expect, it, vi } from 'vitest'
import type { CourseRecord } from '../../../entities/inspection/model/courseRecord'
import { useInspectionEntryOptions } from '../model/queries'
import { CourseRecordDrawer } from './CourseRecordDrawer'
const mutate=vi.fn()
vi.mock('../model/queries',()=>({
 useInspectionContext:(query:{kind:string})=>({data:{items:query.kind==='departments'?[{id:'department',label:'学习部'}]:[{id:'week',label:'测试周',attendancePenaltyMultiplier:2}],inspectionTypeId:'type'},isLoading:false}),
 useInspectionEntryOptions:vi.fn(()=>({data:[],isLoading:false})),
 useCourseMutation:()=>({mutateAsync:mutate,isPending:false}),
}))
vi.mock('../../../entities/inspection/ui/CourseSelect', () => ({ CourseSelect: (props: { value?: string; onChange?: (value?: string) => void }) => <input aria-label="课程" value={props.value ?? ''} onChange={event => props.onChange?.(event.target.value)} /> }))
const record:CourseRecord={id:'one',semesterId:'semester',weekId:'week',inspectionTypeId:'type',departmentId:'department',typeCode:'UNATTENDED_CLASSROOM',studentNumber:'001',studentName:'学生甲',className:'班级甲',grade:'2026',currentCourseName:'课程甲',currentClassroomName:'教室甲',status:'DRAFT',source:'MANUAL',rowVersion:2,createdBy:'actor',canEdit:true,details:{classId:'class',studentId:'student',courseId:'course',classroomId:'room',date:'2026-09-03',time:'08:00',period:'1-2',absenceHours:2,remark:'原备注'}}
async function selectPeriod(label:string, period:number) {
 fireEvent.mouseDown(screen.getByRole('combobox',{name:label}))
 const options = screen.getAllByText(`第 ${period} 节`,{selector:'.ant-select-item-option-content'})
 fireEvent.click(options[options.length - 1])
 if (label === '开始节次') await waitFor(()=>expect(screen.getByRole('combobox',{name:'结束节次'})).not.toBeDisabled())
 else await waitFor(()=>expect(screen.getByRole('combobox',{name:'结束节次'}).parentElement).toHaveTextContent(`第 ${period} 节`))
}
it('preserves initial edit values and failed input under StrictMode, then retries the original version',async()=>{
 mutate.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce(record)
 const close=vi.fn()
 render(<StrictMode><CourseRecordDrawer editing={record} inspectionTypeId="type" onClose={close}/></StrictMode>)
 expect(screen.getByRole('combobox',{name:'开始节次'}).parentElement).toHaveTextContent('第 1 节')
 expect(screen.getByRole('combobox',{name:'结束节次'}).parentElement).toHaveTextContent('第 2 节')
 fireEvent.change(screen.getByLabelText('备注'),{target:{value:'修改内容'}})
 fireEvent.change(screen.getByLabelText('修改原因'),{target:{value:'核对'}})
 fireEvent.click(screen.getByRole('button',{name:'保存修改'}))
 await screen.findByText('保存失败，请稍后重试。')
 expect(screen.getByLabelText('备注')).toHaveValue('修改内容')
 fireEvent.click(screen.getByRole('button',{name:'保存修改'}))
 await waitFor(()=>expect(close).toHaveBeenCalledOnce())
 expect(mutate.mock.calls[1][0]).toMatchObject({id:'one',version:2,reason:'核对',record:{details:{period:'1-2',remark:'修改内容'}}})
})
it('opens a new empty form without reading an editing record',()=>{
 render(<StrictMode><CourseRecordDrawer inspectionTypeId="type" onClose={vi.fn()}/></StrictMode>)
 expect(screen.getByText('选择开始节次')).toBeInTheDocument()
 expect(screen.getByRole('combobox',{name:'结束节次'})).toBeDisabled()
 expect(screen.queryByLabelText('修改原因')).not.toBeInTheDocument()
})
it('preserves the course and selected period range when the inspection date changes',()=>{
 render(<StrictMode><CourseRecordDrawer editing={record} inspectionTypeId="type" onClose={vi.fn()}/></StrictMode>)
 expect(screen.getByRole('combobox',{name:'开始节次'}).parentElement).toHaveTextContent('第 1 节')
 expect(screen.getByRole('combobox',{name:'结束节次'}).parentElement).toHaveTextContent('第 2 节')
 expect(screen.getByLabelText('课程')).toHaveValue('course')
 fireEvent.change(screen.getByLabelText('检查日期'),{target:{value:'2026-09-04'}})
 fireEvent.keyDown(screen.getByLabelText('检查日期'),{key:'Enter',code:'Enter'})
 expect(screen.getByRole('combobox',{name:'开始节次'}).parentElement).toHaveTextContent('第 1 节')
 expect(screen.getByRole('combobox',{name:'结束节次'}).parentElement).toHaveTextContent('第 2 节')
 expect(screen.getByLabelText('课程')).toHaveValue('course')
})
it('submits the selected start and end sections and recalculates attendance hours',async()=>{
 const attendanceRecord:CourseRecord={...record,typeCode:'ATTENDANCE_RATE'}
 mutate.mockResolvedValue(attendanceRecord)
 render(<CourseRecordDrawer editing={attendanceRecord} typeCode="ATTENDANCE_RATE" inspectionTypeId="type" onClose={vi.fn()}/> )
 await selectPeriod('开始节次',3)
 await waitFor(()=>expect(screen.getByText('选择结束节次')).toBeInTheDocument())
 await selectPeriod('结束节次',4)
 expect(screen.getByRole('spinbutton',{name:'实际旷课节次数（按节次自动计算）'})).toHaveValue('2')
 fireEvent.change(screen.getByLabelText('修改原因'),{target:{value:'核对节次范围'}})
 fireEvent.click(screen.getByRole('button',{name:'保存修改'}))
 await waitFor(()=>expect(mutate).toHaveBeenCalledWith(expect.objectContaining({record:expect.objectContaining({details:expect.objectContaining({period:'3-4',absenceHours:2})})})))
})
it('does not request courses or periods when the inspection date changes',async()=>{
 render(<CourseRecordDrawer editing={record} typeCode="ATTENDANCE_RATE" inspectionTypeId="type" onClose={vi.fn()}/>)
 fireEvent.change(screen.getByLabelText('检查日期'),{target:{value:'2026-09-04'}})
 fireEvent.keyDown(screen.getByLabelText('检查日期'),{key:'Enter',code:'Enter'})
 await waitFor(()=>expect(vi.mocked(useInspectionEntryOptions)).toHaveBeenCalledWith(expect.objectContaining({kind:'classrooms'}),expect.any(Boolean)))
 expect(vi.mocked(useInspectionEntryOptions)).not.toHaveBeenCalledWith(expect.objectContaining({kind:'courses'}),expect.any(Boolean))
 expect(vi.mocked(useInspectionEntryOptions)).not.toHaveBeenCalledWith(expect.objectContaining({kind:'periods'}),expect.any(Boolean))
})
it('uses the current multiplier after changing the occurrence date without clearing course or period facts',async()=>{
 const returned:CourseRecord={...record,typeCode:'ATTENDANCE_RATE',status:'RETURNED',details:{...record.details,penaltyMultiplier:1,penaltyHours:2}}
 render(<CourseRecordDrawer editing={returned} typeCode="ATTENDANCE_RATE" inspectionTypeId="type" onClose={vi.fn()}/>)
 await screen.findByText('实际旷课 2 节次；原记录按 1 倍计入违纪，共 2 次。')
 fireEvent.click(screen.getByLabelText('检查日期'))
 fireEvent.click(await screen.findByTitle('2026-09-04'))
 await screen.findByText('实际旷课 2 节次；本周按 2 倍计入违纪，共 4 次。')
 expect(screen.getByLabelText('课程')).toHaveValue('course')
 expect(screen.getByRole('combobox',{name:'开始节次'}).parentElement).toHaveTextContent('第 1 节')
 expect(screen.getByRole('combobox',{name:'结束节次'}).parentElement).toHaveTextContent('第 2 节')
 expect(screen.getByRole('spinbutton',{name:'实际旷课节次数（按节次自动计算）'})).toHaveValue('2')
})
it('does not offer a silent current-week rule for a returned historical record',async()=>{
 const historical:CourseRecord={...record,typeCode:'ATTENDANCE_RATE',status:'RETURNED',details:{...record.details,penaltyMultiplier:null,penaltyHours:null}}
 render(<CourseRecordDrawer editing={historical} typeCode="ATTENDANCE_RATE" inspectionTypeId="type" onClose={vi.fn()}/>)
 await screen.findByText('该历史记录的违纪计入规则未知，不能按当前周次规则直接更正。')
 expect(screen.getByRole('button',{name:'保存修改'})).toBeDisabled()
})
it('shows a member department as read-only instead of requiring manual selection',async()=>{
 render(<CourseRecordDrawer inspectionTypeId="type" initialSemesterId="semester" initialWeekId="week" memberDepartmentId="department" typeCode="ATTENDANCE_RATE" onClose={vi.fn()}/>)
 await screen.findByText('学习部')
 expect(screen.getByRole('combobox',{name:'录入归属部门'})).toBeDisabled()
 expect(screen.queryByRole('combobox',{name:'目标部门'})).not.toBeInTheDocument()
})

import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { StrictMode } from 'react'
import { expect, it, vi } from 'vitest'
import type { DormitoryHygieneRecord } from '../../../entities/inspection/model/dormitoryHygieneRecord'
import { DormitoryHygieneDrawer } from './DormitoryHygieneDrawer'
const mutate=vi.fn()
vi.mock('../model/queries',()=>({
 useInspectionContext:()=>({data:{items:[]},isLoading:false}),
 useInspectionEntryOptions:()=>({data:[],isLoading:false}),
 useResponsibleStudentOptions:()=>({data:[],isLoading:false}),
 useDormitoryHygieneMutation:()=>({mutateAsync:mutate,isPending:false}),
}))
const record:DormitoryHygieneRecord={
 id:'one',typeCode:'DORMITORY_HYGIENE',semesterId:'semester',weekId:'week',inspectionTypeId:'type',departmentId:'department',
 currentDormitoryName:'一区 一栋 101',currentReasonLabel:'地面',responsibleClasses:[{id:'class',label:'班级'}],
 responsibleStudents:[],status:'DRAFT',source:'MANUAL',rowVersion:2,createdBy:'actor',canEdit:true,
 details:{dormitoryId:'room',date:'2026-09-03',round:'首次',result:'UNQUALIFIED',responsibleClassIds:['class'],responsibleStudentIds:[],reasonCode:'FLOOR',description:'地面脏',remark:'原备注'},
}
it('preserves the first edit and failed input under StrictMode, then retries the same version',async()=>{
 mutate.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce(record)
 const close=vi.fn()
 render(<StrictMode><DormitoryHygieneDrawer editing={record} inspectionTypeId="type" onClose={close}/></StrictMode>)
 expect(screen.getByLabelText('检查轮次')).toHaveValue('首次')
 expect(screen.getByLabelText('检查日期')).toHaveValue('2026-09-03')
 expect(screen.queryByLabelText('检查时间')).not.toBeInTheDocument()
 fireEvent.change(screen.getByLabelText('备注'),{target:{value:'修改内容'}})
 fireEvent.change(screen.getByLabelText('修改原因'),{target:{value:'核对'}})
 fireEvent.click(screen.getByRole('button',{name:'保存修改'}))
 await screen.findByText('保存失败，请稍后重试。')
 expect(screen.getByLabelText('备注')).toHaveValue('修改内容')
 fireEvent.click(screen.getByRole('button',{name:'保存修改'}))
 await waitFor(()=>expect(close).toHaveBeenCalledOnce())
 expect(mutate.mock.calls[1][0]).toMatchObject({id:'one',version:2,reason:'核对',record:{details:{round:'首次',result:'UNQUALIFIED',reasonCode:'FLOOR',responsibleClassIds:['class'],remark:'修改内容'}}})
 expect(mutate.mock.calls[1][0].record.details).not.toHaveProperty('time')
 expect(mutate.mock.calls[1][0].record.details).not.toHaveProperty('occurrenceKind')
})
it('opens a new form with one occurrence mode and no stale edit reason',()=>{
 render(<StrictMode><DormitoryHygieneDrawer inspectionTypeId="type" onClose={vi.fn()}/></StrictMode>)
 expect(screen.getByLabelText('检查时间')).toHaveValue('')
 expect(screen.queryByLabelText('检查轮次')).not.toBeInTheDocument()
 expect(screen.queryByLabelText('不合格原因')).not.toBeInTheDocument()
 expect(screen.queryByLabelText('修改原因')).not.toBeInTheDocument()
})
it('requires a concrete description when the reason is other',async()=>{
 mutate.mockClear()
 const otherRecord={...record,currentReasonLabel:'其他',details:{...record.details,reasonCode:'OTHER',description:''}}
 render(<StrictMode><DormitoryHygieneDrawer editing={otherRecord} inspectionTypeId="type" onClose={vi.fn()}/></StrictMode>)
 expect(screen.getByLabelText('其他原因说明')).toHaveValue('')
 fireEvent.change(screen.getByLabelText('修改原因'),{target:{value:'核对'}})
 fireEvent.click(screen.getByRole('button',{name:'保存修改'}))
 expect(await screen.findByText('请填写具体不合格原因')).toBeInTheDocument()
 expect(mutate).not.toHaveBeenCalled()
 fireEvent.change(screen.getByLabelText('其他原因说明'),{target:{value:'门框积灰'}})
 mutate.mockResolvedValueOnce(otherRecord)
 fireEvent.click(screen.getByRole('button',{name:'保存修改'}))
 await waitFor(()=>expect(mutate).toHaveBeenCalledOnce())
 expect(mutate.mock.calls[0][0].record.details).toMatchObject({reasonCode:'OTHER',description:'门框积灰'})
})

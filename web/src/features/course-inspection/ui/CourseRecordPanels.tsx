import { Alert, Button, Descriptions, Drawer, Form, Input, Modal, Spin } from 'antd'
import { useState } from 'react'
import type { CourseRecord } from '../../../entities/inspection/model/courseRecord'
import { ApiError } from '../../../shared/api/httpClient'
import { useCourseDelete, useCourseRecord, useInspectionContext } from '../model/queries'
import { CourseRecordDrawer } from './CourseRecordDrawer'

export function CourseRecordPanel({id,mode,onClose,memberDepartmentId,typeCode='UNATTENDED_CLASSROOM',title='无机课堂'}:{id:string;mode:'detail'|'edit';onClose:()=>void;memberDepartmentId?:string;typeCode?:'UNATTENDED_CLASSROOM'|'ATTENDANCE_RATE';title?:string}) {
 const query=useCourseRecord(id)
 const record=query.data
 const semesters=useInspectionContext({purpose:'view',kind:'semesters',typeCode})
 const departments=useInspectionContext({purpose:'view',kind:'departments',typeCode})
 const weeks=useInspectionContext({purpose:'view',kind:'weeks',semesterId:record?.semesterId,typeCode},!!record?.semesterId)
 const label=(items:{id:string;label:string}[]|undefined,id:string)=>items?.find(item=>item.id===id)?.label??'名称不可用'
 const scopeError=semesters.isError||departments.isError||weeks.isError
 if(query.isError) return <Drawer open title={`${title}记录`} onClose={onClose}><Alert type="error" title="记录加载失败或无权访问。" action={<Button onClick={()=>void query.refetch()}>重试</Button>}/></Drawer>
 if(!record) return <Drawer open title={`${title}记录`} onClose={onClose}><Spin/></Drawer>
 if(record.typeCode!==typeCode)return <Drawer open title={`${title}记录`} onClose={onClose}><Alert type="error" title="记录类型不匹配。"/></Drawer>
 if(mode==='edit')return record.canEdit?<CourseRecordDrawer editing={record} memberDepartmentId={memberDepartmentId} inspectionTypeId={record.inspectionTypeId} typeCode={typeCode} title={title} onClose={onClose}/>:<Drawer open title={`${title}记录`} onClose={onClose}><Alert type="warning" title="记录当前不可编辑，请刷新列表。"/></Drawer>
 return <Drawer open title={`${title}详情`} size={640} onClose={onClose}>{scopeError&&<Alert type="warning" title="范围名称加载失败，请关闭后重试。"/>}<Descriptions column={1} items={[
 {key:'semester',label:'学期',children:label(semesters.data?.items,record.semesterId)},
 {key:'week',label:'周次',children:label(weeks.data?.items,record.weekId)},
 {key:'department',label:'部门',children:label(departments.data?.items,record.departmentId)},
 {key:'status',label:'数据状态',children:({DRAFT:'草稿',PENDING_REVIEW:'待审核',RETURNED:'已退回',CONFIRMED:'已确认',LOCKED:'已锁定'} as Record<string,string>)[record.status]??record.status},
 {key:'source',label:'数据来源',children:({MANUAL:'手工录入',EXCEL:'Excel 导入',REPORT_CORRECTION:'通报修正',MIGRATION:'历史迁移'} as Record<string,string>)[record.source]??record.source},
 {key:'student',label:'学生',children:`${record.studentName}（${record.studentNumber}）`},
 {key:'class',label:'班级',children:record.className},{key:'grade',label:'年级',children:record.grade},
 {key:'course',label:'课程（当前名称）',children:record.currentCourseName||'—'},
 {key:'room',label:'教室（当前名称）',children:record.currentClassroomName||'—'},
 {key:'date',label:'检查时间',children:`${record.details.date} ${record.details.time}`},
 {key:'period',label:'节次',children:record.details.period},{key:'hours',label:'旷课学时',children:record.details.absenceHours},
 {key:'remark',label:'备注',children:record.details.remark||'—'},
 ]}/></Drawer>
}
export function CourseDeleteDialog({records,onClose}:{records:CourseRecord[];onClose:()=>void}) {
 const [form]=Form.useForm<{reason:string}>()
 const [error,setError]=useState('')
 const mutation=useCourseDelete()
 const submit=async({reason}:{reason:string})=>{
  if(mutation.isPending)return
  setError('')
  try{await mutation.mutateAsync({items:records.map(record=>({id:record.id,version:record.rowVersion})),reason});onClose()}
  catch(failure){setError(failure instanceof ApiError && failure.status<500?failure.message:'删除失败，请稍后重试。')}
 }
 return <Modal open title={`确认删除 ${records.length} 条无机课堂记录`} onCancel={()=>{if(!mutation.isPending)onClose()}} onOk={()=>form.submit()} okText="确认删除" okButtonProps={{danger:true}} confirmLoading={mutation.isPending} cancelButtonProps={{disabled:mutation.isPending}} closable={!mutation.isPending} maskClosable={!mutation.isPending}>
 <p>将删除所选草稿：{records.map(record=>record.studentName).join('、')}。操作将保留审计记录。</p>
 {error&&<Alert type="error" title={error}/>}
 <Form form={form} layout="vertical" onFinish={submit}><Form.Item name="reason" label="删除原因" rules={[{required:true,whitespace:true,message:'请填写删除原因'}]}><Input.TextArea maxLength={255} disabled={mutation.isPending}/></Form.Item></Form>
 </Modal>
}

import { ProFormDatePicker, ProFormTimePicker } from '@ant-design/pro-components'
import { Alert, Button, Col, Drawer, Form, Input, Row, Select, Space } from 'antd'
import { useRef, useState } from 'react'
import type { ClassroomHygieneRecord, HygieneResult, InspectionOption } from '../../../entities/inspection/model/classroomHygieneRecord'
import { ApiError } from '../../../shared/api/httpClient'
import { useClassroomHygieneMutation, useInspectionContext, useInspectionEntryOptions, useResponsibleStudentOptions } from '../model/queries'

type Values={
 semesterId:string;weekId:string;departmentId:string;classroomId:string;areaId:string;date:string
 occurrenceKind:'TIME'|'ROUND';time?:string;round?:string;result:HygieneResult
 responsibleClassIds:string[];responsibleStudentIds?:string[];reasonCode?:string;description?:string;remark?:string;reason?:string
}
type Props={editing?:ClassroomHygieneRecord;inspectionTypeId:string;initialSemesterId?:string;initialWeekId?:string;onClose:()=>void}
const required=[{required:true,message:'请填写或选择此项'}]
const choices=(items?:InspectionOption[],fallback:InspectionOption[]=[])=>
 [...fallback.filter(item=>!(items??[]).some(candidate=>candidate.id===item.id)),...(items??[])].map(item=>({value:item.id,label:item.label}))

export function ClassroomHygieneDrawer({editing:initialEditing,inspectionTypeId,initialSemesterId,initialWeekId,onClose}:Props){
 // A mounted editor owns one immutable record/version; cache updates never overwrite its input.
 const [editing]=useState(initialEditing)
 const [form]=Form.useForm<Values>()
 const semesterId=Form.useWatch('semesterId',form)
 const weekId=Form.useWatch('weekId',form)
 const departmentId=Form.useWatch('departmentId',form)
 const classIds=Form.useWatch('responsibleClassIds',form)??editing?.details.responsibleClassIds??[]
 const primaryClassId=classIds[0]
 const result=Form.useWatch('result',form)??editing?.details.result??'QUALIFIED'
 const reasonCode=Form.useWatch('reasonCode',form)??editing?.details.reasonCode
 const occurrenceKind=Form.useWatch('occurrenceKind',form)??(editing?.details.round?'ROUND':'TIME')
 const mutation=useClassroomHygieneMutation(),submitting=useRef(false)
 const [error,setError]=useState('')
 const classroomId=Form.useWatch('classroomId',form)
 const recordId=editing?.id
 const semesters=useInspectionContext({purpose:'entry',recordId,kind:'semesters'})
 const weeks=useInspectionContext({purpose:'entry',recordId,kind:'weeks',semesterId},!!semesterId)
 const departments=useInspectionContext({purpose:'entry',recordId,kind:'departments',semesterId,weekId},!!semesterId&&!!weekId)
 const scope={semesterId,weekId,departmentId,inspectionTypeId,recordId}
 const ready=!!semesterId&&!!weekId&&!!departmentId
 const classes=useInspectionEntryOptions({...scope,kind:'classes'},ready)
 const students=useResponsibleStudentOptions(scope,classIds,ready)
 const classrooms=useInspectionEntryOptions({...scope,kind:'classroom-objects',classId:primaryClassId},ready&&!!primaryClassId)
 const areas=useInspectionEntryOptions({...scope,kind:'classroom-areas',classId:primaryClassId,classroomId},ready&&!!primaryClassId&&!!classroomId)
 const reasons=useInspectionEntryOptions({...scope,kind:'reason-codes',classId:primaryClassId},ready&&!!primaryClassId)
 const queries=[semesters,weeks,departments,classes,students,classrooms,areas,reasons]
 const failed=queries.some(query=>query.isError)
 const clear=(names:(keyof Values)[])=>form.setFields(names.map(name=>({name,value:name==='reasonCode'?'':undefined,errors:[]})))
 const submit=async(values:Values)=>{
  if(submitting.current||failed)return
  submitting.current=true;setError('')
  const details={
   classroomId:values.classroomId,areaId:values.areaId,date:values.date,result:values.result,
   responsibleClassIds:values.responsibleClassIds,responsibleStudentIds:values.responsibleStudentIds??[],
   ...(values.occurrenceKind==='TIME'?{time:values.time}:{round:values.round}),
   ...(values.result==='UNQUALIFIED'?{reasonCode:values.reasonCode}:{}),
   description:values.description??'',remark:values.remark??'',
  }
  try{
   await mutation.mutateAsync({id:editing?.id,version:editing?.rowVersion,reason:values.reason,
    record:{semesterId:values.semesterId,weekId:values.weekId,departmentId:values.departmentId,inspectionTypeId,details}})
   onClose()
  }catch(failure){
   setError(failure instanceof ApiError&&failure.status<500
    ?failure.code==='VERSION_CONFLICT'?'记录已变化，请关闭并重新打开后核对。':failure.message
    :'保存失败，请稍后重试。')
  }finally{submitting.current=false}
 }
 return <Drawer open title={editing?'编辑教室卫生记录':'新增教室卫生记录'} size={760}
  onClose={()=>{if(!submitting.current)onClose()}} closable={!mutation.isPending} maskClosable={!mutation.isPending}
  footer={<Space><Button onClick={onClose} disabled={mutation.isPending}>取消</Button><Button type="primary" loading={mutation.isPending} disabled={failed} onClick={()=>form.submit()}>{editing?'保存修改':'保存草稿'}</Button></Space>}>
  {error&&<Alert type="error" showIcon title={error}/>}
  {failed&&<Alert type="error" showIcon title="录入选项加载失败，请重试。" action={<Button onClick={()=>queries.forEach(query=>{if(query.isError)void query.refetch()})}>重试</Button>}/>}
  <Form form={form} name="classroom-hygiene-record" layout="vertical" onFinish={submit} disabled={mutation.isPending}
   initialValues={editing?{
    semesterId:editing.semesterId,weekId:editing.weekId,departmentId:editing.departmentId,
    ...editing.details,time:editing.details.time?.length===5?`${editing.details.time}:00`:editing.details.time,occurrenceKind:editing.details.time?'TIME':'ROUND',
   }:{semesterId:initialSemesterId,weekId:initialWeekId,occurrenceKind:'TIME',result:'QUALIFIED',responsibleClassIds:[],responsibleStudentIds:[]}}>
   <Row gutter={16}>
    <Col xs={24} sm={12}><Form.Item name="semesterId" label="录入学期" rules={required}><Select loading={semesters.isFetching} options={choices(semesters.data?.items)} onChange={()=>clear(['weekId','departmentId','responsibleClassIds','responsibleStudentIds','classroomId','areaId','reasonCode'])}/></Form.Item></Col>
    <Col xs={24} sm={12}><Form.Item name="weekId" label="录入周次" rules={required}><Select disabled={!semesterId} loading={weeks.isFetching} options={choices(weeks.data?.items)} onChange={()=>clear(['departmentId','responsibleClassIds','responsibleStudentIds','classroomId','areaId','reasonCode'])}/></Form.Item></Col>
    <Col xs={24} sm={12}><Form.Item name="departmentId" label="目标部门" rules={required}><Select disabled={!weekId} loading={departments.isFetching} options={choices(departments.data?.items)} onChange={()=>clear(['responsibleClassIds','responsibleStudentIds','classroomId','areaId','reasonCode'])}/></Form.Item></Col>
    <Col xs={24} sm={12}><Form.Item name="responsibleClassIds" label="责任班级" rules={required}><Select mode="multiple" disabled={!ready} showSearch optionFilterProp="label" loading={classes.isFetching}
     options={choices(classes.data,editing?.responsibleClasses)} onChange={()=>clear(['responsibleStudentIds','classroomId','areaId','reasonCode'])}/></Form.Item></Col>
    <Col xs={24} sm={12}><Form.Item name="classroomId" label="教室" rules={required}><Select disabled={!primaryClassId} showSearch optionFilterProp="label" loading={classrooms.isFetching}
     options={choices(classrooms.data,editing?[{id:editing.details.classroomId,label:editing.currentClassroomName}]:[])} onChange={()=>clear(['areaId'])}/></Form.Item></Col>
    <Col xs={24} sm={12}><Form.Item name="areaId" label="负责区域" rules={required}><Select disabled={!classroomId} showSearch optionFilterProp="label" loading={areas.isFetching}
     options={choices(areas.data,editing&&classroomId===editing.details.classroomId?[{id:editing.details.areaId,label:editing.currentAreaName}]:[])}/></Form.Item></Col>
    <Col xs={24} sm={12}><Form.Item name="responsibleStudentIds" label="责任学生"><Select mode="multiple" disabled={!classIds.length} showSearch optionFilterProp="label" loading={students.isFetching}
     options={choices(students.data,editing?.responsibleStudents)}/></Form.Item></Col>
    <Col xs={24} sm={12}><ProFormDatePicker name="date" label="检查日期" rules={required} getValueFromEvent={(_date: unknown,dateString: string)=>dateString||undefined} fieldProps={{format:'YYYY-MM-DD',style:{width:'100%'}}}/></Col>
    <Col xs={24} sm={12}><Form.Item name="occurrenceKind" label="检查时点方式" rules={required}><Select options={[{value:'TIME',label:'检查时间'},{value:'ROUND',label:'检查轮次'}]} onChange={()=>clear(['time','round'])}/></Form.Item></Col>
    {occurrenceKind==='TIME'
     ?<Col xs={24} sm={12}><ProFormTimePicker name="time" label="检查时间" rules={required} getValueFromEvent={(_time: unknown,timeString: string)=>timeString||undefined} fieldProps={{format:'HH:mm:ss',style:{width:'100%'}}}/></Col>
     :<Col xs={24} sm={12}><Form.Item name="round" label="检查轮次" rules={[...required,{whitespace:true,message:'请填写检查轮次'}]}><Input maxLength={64}/></Form.Item></Col>}
    <Col xs={24} sm={12}><Form.Item name="result" label="检查结果" rules={required}><Select options={[{value:'EXCELLENT',label:'优秀'},{value:'QUALIFIED',label:'合格'},{value:'UNQUALIFIED',label:'不合格'}]} onChange={()=>clear(['reasonCode'])}/></Form.Item></Col>
    {result==='UNQUALIFIED'&&<Col xs={24} sm={12}><Form.Item name="reasonCode" label="不合格原因" rules={required}><Select disabled={!primaryClassId} loading={reasons.isFetching}
     options={choices(reasons.data,editing?.details.reasonCode?[{id:editing.details.reasonCode,label:editing.currentReasonLabel||editing.details.reasonCode}]:[])}/></Form.Item></Col>}
   </Row>
   <Form.Item name="description" label={result==='UNQUALIFIED'&&reasonCode==='OTHER'?'其他原因说明':'情况说明'} rules={result==='UNQUALIFIED'&&reasonCode==='OTHER'?[{required:true,whitespace:true,message:'请填写具体不合格原因'}]:undefined}><Input.TextArea maxLength={500} rows={2}/></Form.Item>
   {editing&&<Form.Item name="reason" label="修改原因" rules={[...required,{whitespace:true,message:'请填写修改原因'}]}><Input.TextArea maxLength={255}/></Form.Item>}
   <Form.Item name="remark" label="备注"><Input.TextArea maxLength={500} rows={3}/></Form.Item>
  </Form>
 </Drawer>
}

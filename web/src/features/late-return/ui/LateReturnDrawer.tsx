import { ProFormDatePicker, ProFormTimePicker } from '@ant-design/pro-components'
import { Alert, Button, Col, Drawer, Form, Input, Row, Select, Space } from 'antd'
import { useEffect, useRef, useState } from 'react'
import type { LateReturnDetails, LateReturnRecord, InspectionOption } from '../../../entities/inspection/model/lateReturnRecord'
import { COLLEGE_DEPARTMENT_OPTION_ID, departmentOptionId } from '../../../entities/inspection/model/inspectionScope'
import { ApiError } from '../../../shared/api/httpClient'
import { useLateReturnMutation, useInspectionContext, useInspectionEntryOptions } from '../model/queries'

type Values = LateReturnDetails & { semesterId: string; weekId: string; departmentId: string; reason?: string }
type Props = { editing?: LateReturnRecord; inspectionTypeId: string; initialSemesterId?: string; initialWeekId?: string; memberDepartmentId?: string; onClose: () => void }
const required = [{ required: true, message: '请填写或选择此项' }]
const options = (items?: InspectionOption[], fallback?: InspectionOption) => {
 const all = items ?? []
 return (fallback && !all.some(item=>item.id===fallback.id) ? [fallback,...all] : all).map(item=>({value:item.id,label:item.label}))
}
export function LateReturnDrawer({ editing: initialEditing, inspectionTypeId, initialSemesterId, initialWeekId, memberDepartmentId, onClose }: Props) {
 // One mounted form owns one immutable record/version; background cache refreshes never replace user input.
 const [editing] = useState(initialEditing)
 const memberEntry = memberDepartmentId !== undefined
 const [form] = Form.useForm<Values>()
 const semesterId = Form.useWatch('semesterId',form)
 const weekId = Form.useWatch('weekId',form)
 const departmentId = Form.useWatch('departmentId',form)
 const classId = Form.useWatch('classId',form)
 const studentId = Form.useWatch('studentId',form)
 const mutation=useLateReturnMutation(), submitting=useRef(false)
 const [error,setError]=useState('')
 const purpose='entry'
 const recordId=editing?.id
 const semesters=useInspectionContext({purpose,recordId,kind:'semesters'})
 const weeks=useInspectionContext({purpose,recordId,kind:'weeks',semesterId},!!semesterId)
 const departments=useInspectionContext({purpose,recordId,kind:'departments',semesterId,weekId},!!semesterId&&!!weekId)
 useEffect(() => { if (!editing && !memberEntry && departments.data?.items.some(item => item.id === COLLEGE_DEPARTMENT_OPTION_ID) && !form.getFieldValue('departmentId')) form.setFieldValue('departmentId', COLLEGE_DEPARTMENT_OPTION_ID) }, [departments.data?.items, editing, form, memberEntry])
 const memberDepartmentAvailable = !memberEntry || !weekId || !!departments.data?.items.some(item=>item.id===memberDepartmentId)
 const scope={semesterId,weekId,departmentId,inspectionTypeId,recordId:editing?.id}
 const ready=!!semesterId&&!!weekId&&!!departmentId
 const classes=useInspectionEntryOptions({...scope,kind:'classes'},ready)
 const students=useInspectionEntryOptions({...scope,kind:'students',classId},ready&&!!classId)
 const dormitories=useInspectionEntryOptions({...scope,kind:'dormitories',classId,studentId},ready&&!!classId&&!!studentId)
 const queries=[semesters,weeks,departments,classes,students,dormitories]
 const failed=queries.some(query=>query.isError)
 const clear=(names:(keyof Values)[])=>form.setFields(names.map(name=>({name,value:undefined,errors:[]})))
 const fallback=(key:keyof LateReturnDetails,label:string)=>editing?{id:String(editing.details[key]),label}:undefined
 const submit=async(values:Values)=>{
  if(submitting.current||failed)return
  submitting.current=true;setError('')
  const {semesterId,weekId,departmentId,reason,...details}=values
  try {await mutation.mutateAsync({id:editing?.id,version:editing?.rowVersion,reason,record:{semesterId,weekId,departmentId,inspectionTypeId,details:{...details,remark:details.remark??''}}});onClose()}
  catch(failure){setError(failure instanceof ApiError && failure.status<500 ? failure.code==='VERSION_CONFLICT'?'记录已变化，请关闭并重新打开后核对。':failure.message:'保存失败，请稍后重试。')}
  finally{submitting.current=false}
 }
 return <Drawer open title={editing?'编辑晚归记录':'新增晚归记录'} size={720} onClose={()=>{if(!submitting.current)onClose()}} closable={!mutation.isPending} maskClosable={!mutation.isPending}
 footer={<Space><Button onClick={onClose} disabled={mutation.isPending}>取消</Button><Button type="primary" loading={mutation.isPending} disabled={failed||!memberDepartmentAvailable} onClick={()=>form.submit()}>{editing?'保存修改':'保存草稿'}</Button></Space>}>
 {error&&<Alert type="error" showIcon title={error} />}
 {failed&&<Alert type="error" showIcon title="录入选项加载失败，请重试。" action={<Button onClick={()=>queries.forEach(query=>{if(query.isError)void query.refetch()})}>重试</Button>}/>}
 {!memberDepartmentAvailable && !departments.isLoading && <Alert type="warning" showIcon title="当前账号所属部门无此范围的录入权限，请联系管理员。" />}
 <Form name="late-return-record" form={form} layout="vertical" onFinish={submit} disabled={mutation.isPending}
 initialValues={editing?{semesterId:editing.semesterId,weekId:editing.weekId,departmentId:departmentOptionId(editing.departmentId),...editing.details,time:editing.details.time.length===5?`${editing.details.time}:00`:editing.details.time}:{semesterId:initialSemesterId,weekId:initialWeekId,departmentId:memberDepartmentId}}>
 <Row gutter={16}>
 <Col xs={24} sm={12}><Form.Item name="semesterId" label="录入学期" rules={required}><Select loading={semesters.isFetching} options={options(semesters.data?.items)} onChange={()=>clear(memberEntry?['weekId','classId','studentId','dormitoryId']:['weekId','departmentId','classId','studentId','dormitoryId'])}/></Form.Item></Col>
 <Col xs={24} sm={12}><Form.Item name="weekId" label="录入周次" rules={required}><Select disabled={!semesterId} loading={weeks.isFetching} options={options(weeks.data?.items)} onChange={()=>clear(memberEntry?['classId','studentId','dormitoryId']:['departmentId','classId','studentId','dormitoryId'])}/></Form.Item></Col>
 <Col xs={24} sm={12}><Form.Item name="departmentId" label={memberEntry?'录入归属部门':'目标部门'} rules={required}><Select disabled={!weekId||memberEntry} loading={departments.isFetching} options={options(departments.data?.items)} onChange={()=>clear(['classId','studentId','dormitoryId'])}/></Form.Item></Col>
 <Col xs={24} sm={12}><Form.Item name="classId" label="学生班级" rules={required}><Select disabled={!ready} showSearch optionFilterProp="label" loading={classes.isFetching} options={options(classes.data,fallback('classId',editing?.className??''))} onChange={()=>clear(['studentId','dormitoryId'])}/></Form.Item></Col>
 <Col xs={24} sm={12}><Form.Item name="studentId" label="学生" rules={required}><Select disabled={!classId} showSearch optionFilterProp="label" loading={students.isFetching} options={options(students.data,editing && editing.details.classId===classId?fallback('studentId',`${editing.studentName}（${editing.studentNumber}）`):undefined)} onChange={()=>clear(['dormitoryId'])}/></Form.Item></Col>
 <Col xs={24} sm={12}><Form.Item name="dormitoryId" label="学生宿舍" rules={required}><Select disabled={!studentId} showSearch optionFilterProp="label" loading={dormitories.isFetching} options={options(dormitories.data,editing && editing.details.studentId===studentId?fallback('dormitoryId',editing.currentDormitoryName):undefined)}/></Form.Item></Col>
 <Col xs={24} sm={12}><ProFormDatePicker name="date" label="晚归日期" rules={required} getValueFromEvent={(_date: unknown,dateString: string)=>dateString||undefined} fieldProps={{format:'YYYY-MM-DD',style:{width:'100%'}}}/></Col>
 <Col xs={24} sm={12}><ProFormTimePicker name="time" label="晚归时间" rules={required} getValueFromEvent={(_time: unknown,timeString: string)=>timeString||undefined} fieldProps={{format:'HH:mm:ss',style:{width:'100%'}}}/></Col>
 </Row>
 {editing&&<Form.Item name="reason" label="修改原因" rules={[...required,{whitespace:true,message:'请填写修改原因'}]}><Input.TextArea maxLength={255}/></Form.Item>}
 <Form.Item name="remark" label="备注"><Input.TextArea maxLength={500} rows={3}/></Form.Item>
 </Form>
 </Drawer>
}

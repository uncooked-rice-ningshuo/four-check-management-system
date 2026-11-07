import { ProFormDatePicker } from '@ant-design/pro-components'
import { Alert, Button, Col, Divider, Drawer, Form, Input, Row, Select, Space } from 'antd'
import { useEffect, useRef, useState } from 'react'
import type { LateReturnDetails, LateReturnRecord, InspectionOption } from '../../../entities/inspection/model/lateReturnRecord'
import { COLLEGE_DEPARTMENT_OPTION_ID, departmentOptionId } from '../../../entities/inspection/model/inspectionScope'
import { InspectionAttributionNote } from '../../../shared/components/InspectionAttributionNote'
import { ApiError } from '../../../shared/api/httpClient'
import { useLateReturnMutation, useInspectionContext, useInspectionEntryOptions } from '../model/queries'

type Values = Omit<LateReturnDetails, 'time'> & { semesterId: string; weekId: string; departmentId: string; dormitoryAreaId?: string; dormitoryBuildingId?: string; reason?: string }
type Props = { editing?: LateReturnRecord; inspectionTypeId: string; initialSemesterId?: string; initialWeekId?: string; memberDepartmentId?: string; isSuperAdmin?: boolean; onClose: () => void }
const required = [{ required: true, message: '请填写或选择此项' }]
const options = (items?: InspectionOption[], fallback?: InspectionOption) => {
 const all = items ?? []
 return (fallback && !all.some(item=>item.id===fallback.id) ? [fallback,...all] : all).map(item=>({value:item.id,label:item.label}))
}
export function LateReturnDrawer({ editing: initialEditing, inspectionTypeId, initialSemesterId, initialWeekId, memberDepartmentId, isSuperAdmin = false, onClose }: Props) {
 // One mounted form owns one immutable record/version; background cache refreshes never replace user input.
 const [editing] = useState(initialEditing)
 const memberEntry = memberDepartmentId !== undefined
 const [form] = Form.useForm<Values>()
 const semesterId = Form.useWatch('semesterId',form)
 const weekId = Form.useWatch('weekId',form)
 const departmentId = Form.useWatch('departmentId',form)
 const dormitoryId = Form.useWatch('dormitoryId',form)
 const dormitoryAreaId = Form.useWatch('dormitoryAreaId',form)
 const dormitoryBuildingId = Form.useWatch('dormitoryBuildingId',form)
 const mutation=useLateReturnMutation(), submitting=useRef(false)
 const [error,setError]=useState('')
 // The class is never typed: it follows the student selected inside the room.
 const [classChoice,setClassChoice]=useState<InspectionOption|undefined>(
  initialEditing?{id:initialEditing.details.classId,label:initialEditing.className}:undefined
 )
 const purpose='entry'
 const recordId=editing?.id
 const semesters=useInspectionContext({purpose,recordId,kind:'semesters'})
 const weeks=useInspectionContext({purpose,recordId,kind:'weeks',semesterId},!!semesterId)
 const departments=useInspectionContext({purpose,recordId,kind:'departments',semesterId,weekId},!!semesterId&&!!weekId)
 useEffect(() => { if (!isSuperAdmin && !editing && !memberEntry && departments.data?.items.some(item => item.id === COLLEGE_DEPARTMENT_OPTION_ID) && !form.getFieldValue('departmentId')) form.setFieldValue('departmentId', COLLEGE_DEPARTMENT_OPTION_ID) }, [departments.data?.items, editing, form, isSuperAdmin, memberEntry])
 const memberDepartmentAvailable = !memberEntry || !weekId || !!departments.data?.items.some(item=>item.id===memberDepartmentId)
 const scope={semesterId,weekId,departmentId,inspectionTypeId,recordId:editing?.id}
 const ready=!!semesterId&&!!weekId&&(isSuperAdmin||!!departmentId)
 // Room first, then only its current members: a late-return record always
 // belongs to the student's own dormitory. Housing is chosen as area → building → room.
 const areas=useInspectionEntryOptions({...scope,kind:'dormitory-areas'},ready)
 const buildings=useInspectionEntryOptions({...scope,kind:'dormitory-buildings',dormitoryAreaId},ready&&!!dormitoryAreaId)
 const dormitories=useInspectionEntryOptions({...scope,kind:'dormitory-objects',dormitoryAreaId,dormitoryBuildingId},ready&&!!dormitoryBuildingId)
 const students=useInspectionEntryOptions({...scope,kind:'students',dormitoryId},ready&&!!dormitoryId)
 const queries=[semesters,weeks,departments,areas,buildings,students,dormitories]
 const failed=queries.some(query=>query.isError)
 const clear=(names:(keyof Values)[])=>form.setFields(names.map(name=>({name,value:undefined,errors:[]})))
 const chooseStudent=(studentIdValue:string)=>{
  const student=students.data?.find(item=>item.id===studentIdValue)
  form.setFieldValue('classId',student?.classId)
  setClassChoice(student?.classId?{id:student.classId,label:student.className??''}:undefined)
 }
 const fallback=(key:keyof LateReturnDetails,label:string)=>editing?{id:String(editing.details[key]),label}:undefined
 const submit=async(values:Values)=>{
  if(submitting.current||failed)return
  submitting.current=true;setError('')
  const {semesterId,weekId,departmentId,reason}=values
  // Housing pickers are entry filters only; the record stores the room itself.
  const details={classId:values.classId,studentId:values.studentId,dormitoryId:values.dormitoryId,date:values.date,remark:values.remark??''}
  try {await mutation.mutateAsync({id:editing?.id,version:editing?.rowVersion,reason,record:{semesterId,weekId,departmentId:isSuperAdmin&&editing?.attributionMode==='ADMINISTRATOR'?'':departmentId,inspectionTypeId,...(!editing&&isSuperAdmin?{attributionMode:'ADMINISTRATOR' as const}:{}),details}});onClose()}
  catch(failure){setError(failure instanceof ApiError && failure.status<500 ? failure.code==='VERSION_CONFLICT'?'记录已变化，请关闭并重新打开后核对。':failure.message:'保存失败，请稍后重试。')}
  finally{submitting.current=false}
 }
 return <Drawer open title={editing?'编辑晚归记录':'新增晚归记录'} size={760} className="inspection-entry-drawer" onClose={()=>{if(!submitting.current)onClose()}} closable={!mutation.isPending} maskClosable={!mutation.isPending}
 footer={<Space><Button onClick={onClose} disabled={mutation.isPending}>取消</Button><Button type="primary" loading={mutation.isPending} disabled={failed||!memberDepartmentAvailable} onClick={()=>form.submit()}>{editing?'保存修改':'保存草稿'}</Button></Space>}>
 {error&&<Alert type="error" showIcon title={error} />}
 {failed&&<Alert type="error" showIcon title="录入选项加载失败，请重试。" action={<Button onClick={()=>queries.forEach(query=>{if(query.isError)void query.refetch()})}>重试</Button>}/>}
 {!memberDepartmentAvailable && !departments.isLoading && <Alert type="warning" showIcon title="当前账号所属部门无此范围的录入权限，请联系管理员。" />}
 <Form name="late-return-record" form={form} layout="vertical" onFinish={submit} disabled={mutation.isPending}
 initialValues={editing?{semesterId:editing.semesterId,weekId:editing.weekId,departmentId:departmentOptionId(editing.departmentId),classId:editing.details.classId,studentId:editing.details.studentId,dormitoryId:editing.details.dormitoryId,dormitoryAreaId:editing.currentDormitoryAreaId,dormitoryBuildingId:editing.currentDormitoryBuildingId,date:editing.details.date,remark:editing.details.remark}:{semesterId:initialSemesterId,weekId:initialWeekId,departmentId:isSuperAdmin?'':memberDepartmentId}}>
 <Divider titlePlacement="start" plain>录入范围</Divider>
 <Row gutter={16}>
 <Col xs={24} sm={12}><Form.Item name="semesterId" label="录入学期" rules={required}><Select loading={semesters.isFetching} options={options(semesters.data?.items)} onChange={()=>{clear(memberEntry?['weekId','dormitoryAreaId','dormitoryBuildingId','dormitoryId','studentId','classId']:['weekId','departmentId','dormitoryAreaId','dormitoryBuildingId','dormitoryId','studentId','classId']);setClassChoice(undefined)}}/></Form.Item></Col>
 <Col xs={24} sm={12}><Form.Item name="weekId" label="录入周次" rules={required}><Select disabled={!semesterId} loading={weeks.isFetching} options={options(weeks.data?.items)} onChange={()=>{clear(memberEntry?['dormitoryAreaId','dormitoryBuildingId','dormitoryId','studentId','classId']:['departmentId','dormitoryAreaId','dormitoryBuildingId','dormitoryId','studentId','classId']);setClassChoice(undefined)}}/></Form.Item></Col>
 {isSuperAdmin?<Col span={24}><Form.Item name="departmentId" hidden><Input/></Form.Item><InspectionAttributionNote editing={!!editing} administratorOwned={editing?.attributionMode === 'ADMINISTRATOR'} /></Col>:<Col xs={24} sm={12}><Form.Item name="departmentId" label={memberEntry?'录入归属部门':'业务归属部门'} rules={required}><Select disabled={!weekId||memberEntry} loading={departments.isFetching} options={options(departments.data?.items)} onChange={()=>{clear(['dormitoryAreaId','dormitoryBuildingId','dormitoryId','studentId','classId']);setClassChoice(undefined)}}/></Form.Item></Col>}
 </Row>
 <Divider titlePlacement="start" plain>晚归信息</Divider>
 <Row gutter={16}>
 <Col xs={24} sm={12}><Form.Item name="dormitoryAreaId" label="宿舍区"><Select disabled={!ready} showSearch optionFilterProp="label" loading={areas.isFetching} options={options(areas.data)} onChange={()=>{clear(['dormitoryBuildingId','dormitoryId','studentId','classId']);setClassChoice(undefined)}}/></Form.Item></Col>
 <Col xs={24} sm={12}><Form.Item name="dormitoryBuildingId" label="宿舍栋"><Select disabled={!dormitoryAreaId} showSearch optionFilterProp="label" loading={buildings.isFetching} options={options(buildings.data)} onChange={()=>{clear(['dormitoryId','studentId','classId']);setClassChoice(undefined)}}/></Form.Item></Col>
 <Col xs={24} sm={12}><Form.Item name="dormitoryId" label="房间号" rules={required}><Select disabled={!dormitoryBuildingId} showSearch optionFilterProp="label" loading={dormitories.isFetching} options={options(dormitories.data,editing?{id:editing.details.dormitoryId,label:editing.currentDormitoryName}:undefined)} onChange={()=>{clear(['studentId','classId']);setClassChoice(undefined)}}/></Form.Item></Col>
 <Col xs={24} sm={12}><Form.Item name="studentId" label="学生" rules={required}><Select disabled={!dormitoryId} showSearch optionFilterProp="label" loading={students.isFetching} options={options(students.data,editing && editing.details.dormitoryId===dormitoryId?fallback('studentId',`${editing.studentName}（${editing.studentNumber}）`):undefined)} onChange={chooseStudent}/></Form.Item></Col>
 <Col xs={24} sm={12}><Form.Item name="classId" label="学生班级" rules={required} extra="按所选学生自动带出"><Select disabled options={classChoice?[{value:classChoice.id,label:classChoice.label}]:[]}/></Form.Item></Col>
 <Col xs={24} sm={12}><ProFormDatePicker name="date" label="晚归日期" rules={required} getValueFromEvent={(_date: unknown,dateString: string)=>dateString||undefined} fieldProps={{format:'YYYY-MM-DD',style:{width:'100%'}}}/></Col>
 </Row>
 <Divider titlePlacement="start" plain>补充信息</Divider>
 {editing&&<Form.Item name="reason" label="修改原因" rules={[...required,{whitespace:true,message:'请填写修改原因'}]}><Input.TextArea maxLength={255}/></Form.Item>}
 <Form.Item name="remark" label="备注"><Input.TextArea maxLength={500} rows={3}/></Form.Item>
 </Form>
 </Drawer>
}

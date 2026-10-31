import { ProFormDatePicker, ProFormTimePicker } from '@ant-design/pro-components'
import { Alert, Button, Col, Drawer, Form, Input, InputNumber, Row, Select, Space } from 'antd'
import { useEffect, useRef, useState } from 'react'
import type { CourseDetails, CourseRecord, InspectionOption } from '../../../entities/inspection/model/courseRecord'
import { coursePeriodHours, formatCoursePeriod, parseCoursePeriod } from '../../../entities/inspection/model/coursePeriod'
import { CourseSelect } from '../../../entities/inspection/ui/CourseSelect'
import { PeriodRangeFields } from '../../../entities/inspection/ui/PeriodRangeFields'
import { COLLEGE_DEPARTMENT_OPTION_ID, departmentOptionId } from '../../../entities/inspection/model/inspectionScope'
import { ApiError } from '../../../shared/api/httpClient'
import { useCourseMutation, useInspectionContext, useInspectionEntryOptions } from '../model/queries'

type Values = Omit<CourseDetails, 'period'> & { periodStart?: number; periodEnd?: number; semesterId: string; weekId: string; departmentId: string; reason?: string }
type Props = { editing?: CourseRecord; inspectionTypeId: string; initialSemesterId?: string; initialWeekId?: string; memberDepartmentId?: string; typeCode?: 'UNATTENDED_CLASSROOM' | 'ATTENDANCE_RATE'; title?: string; onClose: () => void }
const required = [{ required: true, message: '请填写或选择此项' }]
const options = (items?: InspectionOption[], fallback?: InspectionOption) => {
 const all = items ?? []
 return (fallback && !all.some(item=>item.id===fallback.id) ? [fallback,...all] : all).map(item=>({value:item.id,label:item.label}))
}
export function CourseRecordDrawer({ editing: initialEditing, inspectionTypeId, initialSemesterId, initialWeekId, memberDepartmentId, typeCode = 'UNATTENDED_CLASSROOM', title = '无机课堂', onClose }: Props) {
 // One mounted form owns one immutable record/version; background cache refreshes never replace user input.
 const [editing] = useState(initialEditing)
 const memberEntry = memberDepartmentId !== undefined
 const [form] = Form.useForm<Values>()
 const semesterId = Form.useWatch('semesterId',form)
 const weekId = Form.useWatch('weekId',form)
 const departmentId = Form.useWatch('departmentId',form)
 const classId = Form.useWatch('classId',form)
 const date = Form.useWatch('date',{form,preserve:true})
 const absenceHours = Form.useWatch('absenceHours',{form,preserve:true})
 const mutation=useCourseMutation(), submitting=useRef(false)
 const [error,setError]=useState('')
 const purpose='entry'
 const recordId=editing?.id
 const semesters=useInspectionContext({purpose,recordId,kind:'semesters',typeCode})
 const weeks=useInspectionContext({purpose,recordId,kind:'weeks',semesterId,typeCode},!!semesterId)
 const weekMultiplier = weeks.data?.items.find(item=>item.id===weekId)?.attendancePenaltyMultiplier
 const sameSubmittedOccurrence = editing?.status==='RETURNED' && editing.weekId===weekId && editing.details.date===date
 const legacyRuleUnknown = sameSubmittedOccurrence && !editing?.details.penaltyMultiplier
 const effectiveMultiplier = sameSubmittedOccurrence ? editing?.details.penaltyMultiplier : weekMultiplier
 const initialPeriod = parseCoursePeriod(editing?.details.period)
 const initialValues = editing ? {
  semesterId: editing.semesterId,
  weekId: editing.weekId,
  departmentId: departmentOptionId(editing.departmentId),
  ...editing.details,
  periodStart: initialPeriod?.start,
  periodEnd: initialPeriod?.end,
  time: editing.details.time.length===5?`${editing.details.time}:00`:editing.details.time,
 } : {semesterId:initialSemesterId,weekId:initialWeekId,departmentId:memberDepartmentId}
 const departments=useInspectionContext({purpose,recordId,kind:'departments',semesterId,weekId,typeCode},!!semesterId&&!!weekId)
 useEffect(() => { if (!editing && !memberEntry && departments.data?.items.some(item => item.id === COLLEGE_DEPARTMENT_OPTION_ID) && !form.getFieldValue('departmentId')) form.setFieldValue('departmentId', COLLEGE_DEPARTMENT_OPTION_ID) }, [departments.data?.items, editing, form, memberEntry])
 const memberDepartmentAvailable = !memberEntry || !weekId || !!departments.data?.items.some(item=>item.id===memberDepartmentId)
 const scope={semesterId,weekId,departmentId,inspectionTypeId,recordId:editing?.id}
 const ready=!!semesterId&&!!weekId&&!!departmentId
 const classes=useInspectionEntryOptions({...scope,kind:'classes'},ready)
 const students=useInspectionEntryOptions({...scope,kind:'students',classId},ready&&!!classId)
 const classrooms=useInspectionEntryOptions({...scope,kind:'classrooms'},ready)
 const queries=[semesters,weeks,departments,classes,students,classrooms]
 const failed=queries.some(query=>query.isError)
 const clear=(names:(keyof Values)[])=>form.setFields((typeCode==='ATTENDANCE_RATE'&&(names.includes('periodStart')||names.includes('periodEnd'))?[...names,'absenceHours' as const]:names).map(name=>({name,value:undefined,errors:[]})))
 const fallback=(key:keyof CourseDetails,label:string)=>editing?{id:String(editing.details[key]),label}:undefined
 const submit=async(values:Values)=>{
  if(submitting.current||failed)return
  submitting.current=true;setError('')
  const {semesterId,weekId,departmentId,reason,periodStart,periodEnd,penaltyMultiplier:_ignoredMultiplier,penaltyHours:_ignoredHours,...details}=values
  void _ignoredMultiplier; void _ignoredHours
  const period = formatCoursePeriod(periodStart, periodEnd)
  if (!period) { setError('请选择有效的开始和结束节次。'); submitting.current=false; return }
  try {await mutation.mutateAsync({id:editing?.id,version:editing?.rowVersion,reason,record:{semesterId,weekId,departmentId,inspectionTypeId,details:{...details,period,remark:details.remark??''}}});onClose()}
  catch(failure){setError(failure instanceof ApiError && failure.status<500 ? failure.code==='VERSION_CONFLICT'?'记录已变化，请关闭并重新打开后核对。':failure.message:'保存失败，请稍后重试。')}
  finally{submitting.current=false}
 }
 return <Drawer open title={`${editing?'编辑':'新增'}${title}记录`} size={720} onClose={()=>{if(!submitting.current)onClose()}} closable={!mutation.isPending} maskClosable={!mutation.isPending}
 footer={<Space><Button onClick={onClose} disabled={mutation.isPending}>取消</Button><Button type="primary" loading={mutation.isPending} disabled={failed || !memberDepartmentAvailable || (typeCode==='ATTENDANCE_RATE' && !!weekId && !effectiveMultiplier)} onClick={()=>form.submit()}>{editing?'保存修改':'保存草稿'}</Button></Space>}>
 {error&&<Alert type="error" showIcon title={error} />}
 {failed&&<Alert type="error" showIcon title="录入选项加载失败，请重试。" action={<Button onClick={()=>queries.forEach(query=>{if(query.isError)void query.refetch()})}>重试</Button>}/>}
 {!memberDepartmentAvailable && !departments.isLoading && <Alert type="warning" showIcon title="当前账号所属部门无此范围的录入权限，请联系管理员。" />}
 <Form name="course-record" form={form} layout="vertical" onFinish={submit} disabled={mutation.isPending}
 initialValues={initialValues}
 onValuesChange={(changed, values)=>{if(typeCode==='ATTENDANCE_RATE'&&('periodStart' in changed||'periodEnd' in changed))form.setFieldValue('absenceHours',coursePeriodHours(values.periodStart,values.periodEnd))}}>
 {typeCode==='ATTENDANCE_RATE' && weekId && (effectiveMultiplier ? <Alert type="info" showIcon title={typeof absenceHours==='number' && absenceHours>0 ? `实际旷课 ${absenceHours} 节次；${sameSubmittedOccurrence?'原记录':'本周'}按 ${effectiveMultiplier} 倍计入违纪，共 ${Number((absenceHours*effectiveMultiplier).toFixed(2))} 次。` : `${sameSubmittedOccurrence?'原记录':'本周'}违纪计入倍数为 ${effectiveMultiplier}；选择节次后自动显示计入结果。`} /> : <Alert type="warning" showIcon title={legacyRuleUnknown?'该历史记录的违纪计入规则未知，不能按当前周次规则直接更正。':'该周到课率违纪计入规则尚未配置，请联系管理员。'} />)}
 <Row gutter={16}>
 <Col xs={24} sm={12}><Form.Item name="semesterId" label="录入学期" rules={required}><Select loading={semesters.isFetching} options={options(semesters.data?.items)} onChange={()=>clear(memberEntry?['weekId','classId','studentId','classroomId','periodStart','periodEnd']:['weekId','departmentId','classId','studentId','classroomId','periodStart','periodEnd'])}/></Form.Item></Col>
 <Col xs={24} sm={12}><Form.Item name="weekId" label="录入周次" rules={required}><Select disabled={!semesterId} loading={weeks.isFetching} options={options(weeks.data?.items)} onChange={()=>clear(memberEntry?['classId','studentId','classroomId','periodStart','periodEnd']:['departmentId','classId','studentId','classroomId','periodStart','periodEnd'])}/></Form.Item></Col>
 <Col xs={24} sm={12}><Form.Item name="departmentId" label={memberEntry?'录入归属部门':'目标部门'} rules={required}><Select disabled={!weekId||memberEntry} loading={departments.isFetching} options={options(departments.data?.items,memberEntry&&editing?{id:editing.departmentId,label:'原记录部门'}:undefined)} onChange={()=>clear(['classId','studentId','classroomId','periodStart','periodEnd'])}/></Form.Item></Col>
 <Col xs={24} sm={12}><Form.Item name="classId" label="学生班级" rules={required}><Select disabled={!ready} showSearch optionFilterProp="label" loading={classes.isFetching} options={options(classes.data,fallback('classId',editing?.className??''))} onChange={()=>clear(['studentId'])}/></Form.Item></Col>
 <Col xs={24} sm={12}><Form.Item name="studentId" label="学生" rules={required}><Select disabled={!classId} showSearch optionFilterProp="label" loading={students.isFetching} options={options(students.data,editing && editing.details.classId===classId?fallback('studentId',`${editing.studentName}（${editing.studentNumber}）`):undefined)}/></Form.Item></Col>
 <Col xs={24} sm={12}><ProFormDatePicker name="date" label="检查日期" rules={required} getValueFromEvent={(_date: unknown,dateString: string)=>dateString||undefined} fieldProps={{format:'YYYY-MM-DD',style:{width:'100%'}}}/></Col>
 <Col xs={24} sm={12}><Form.Item name="courseId" label="课程" rules={required}><CourseSelect disabled={!ready} scope={scope} selectedLabel={editing?.currentCourseName}/></Form.Item></Col>
 <Col xs={24} sm={12}><Form.Item name="classroomId" label="教室" rules={required}><Select disabled={!ready} showSearch optionFilterProp="label" loading={classrooms.isFetching} options={options(classrooms.data,editing?fallback('classroomId',editing.currentClassroomName):undefined)}/></Form.Item></Col>
 <Col xs={24} sm={12}><ProFormTimePicker name="time" label="检查时间" rules={required} getValueFromEvent={(_time: unknown,timeString: string)=>timeString||undefined} fieldProps={{format:'HH:mm:ss',style:{width:'100%'}}}/></Col>
 <PeriodRangeFields startName="periodStart" endName="periodEnd" />
 <Col xs={24} sm={12}><Form.Item name="absenceHours" label={typeCode==='ATTENDANCE_RATE'?'实际旷课节次数（按节次自动计算）':'旷课学时'} rules={[...required,{type:'number',min:0.01,max:24,message:'学时应大于 0 且不超过 24'}]}><InputNumber disabled={typeCode==='ATTENDANCE_RATE'} min={0.01} max={24} style={{width:'100%'}}/></Form.Item></Col>
 </Row>
 {editing&&<Form.Item name="reason" label="修改原因" rules={[...required,{whitespace:true,message:'请填写修改原因'}]}><Input.TextArea maxLength={255}/></Form.Item>}
 <Form.Item name="remark" label="备注"><Input.TextArea maxLength={500} rows={3}/></Form.Item>
 </Form>
 </Drawer>
}

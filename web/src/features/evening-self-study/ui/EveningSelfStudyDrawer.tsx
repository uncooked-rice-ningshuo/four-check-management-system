import { ProFormDatePicker, ProFormTimePicker } from '@ant-design/pro-components'
import { Alert, Button, Col, Drawer, Form, Input, Row, Select, Space, Tag } from 'antd'
import { useEffect, useRef, useState } from 'react'
import type { EveningSelfStudyRecord, EveningSelfStudyRecordInput, InspectionOption } from '../../../entities/inspection/model/eveningSelfStudyRecord'
import { COLLEGE_DEPARTMENT_OPTION_ID, departmentOptionId } from '../../../entities/inspection/model/inspectionScope'
import { ApiError } from '../../../shared/api/httpClient'
import { useEveningSelfStudyMutation, useInspectionContext, useInspectionEntryOptions } from '../model/queries'

type Values = EveningSelfStudyRecordInput['details'] & { semesterId: string; weekId: string; departmentId: string; reason?: string }
type Props = { editing?: EveningSelfStudyRecord; readOnly?: boolean; inspectionTypeId: string; initialSemesterId?: string; initialWeekId?: string; memberDepartmentId?: string; onClose: () => void }
const required = [{ required: true, message: '请填写或选择此项' }]
const choices = (items?: InspectionOption[], fallback?: InspectionOption) => {
 const all = items ?? []
 return (fallback && !all.some(item => item.id === fallback.id) ? [fallback, ...all] : all).map(item => ({ value: item.id, label: item.label }))
}

export function EveningSelfStudyDrawer({ editing: initialEditing, readOnly = false, inspectionTypeId, initialSemesterId, initialWeekId, memberDepartmentId, onClose }: Props) {
 const [editing] = useState(initialEditing)
 const memberEntry = memberDepartmentId !== undefined
 const [form] = Form.useForm<Values>()
 const semesterId = Form.useWatch('semesterId', form)
 const weekId = Form.useWatch('weekId', form)
 const departmentId = Form.useWatch('departmentId', form)
 const classId = Form.useWatch('classId', form)
 const mutation = useEveningSelfStudyMutation()
 const submitting = useRef(false)
 const [error, setError] = useState('')
 const purpose = readOnly ? 'view' : 'entry'
 const recordId = editing?.id
 const semesters = useInspectionContext({ purpose, recordId, kind: 'semesters' })
 const weeks = useInspectionContext({ purpose, recordId, kind: 'weeks', semesterId }, !!semesterId)
 const departments = useInspectionContext({ purpose, recordId, kind: 'departments', semesterId, weekId }, !!semesterId && !!weekId)
 const weekMultiplier = weeks.data?.items.find(item => item.id === weekId)?.attendancePenaltyMultiplier
 const effectiveMultiplier = editing?.status === 'RETURNED' && editing.weekId === weekId ? editing.details.penaltyMultiplier ?? weekMultiplier : weekMultiplier
 const memberDepartmentAvailable = !memberEntry || !weekId || !!departments.data?.items.some(item => item.id === memberDepartmentId)
 const scope = { semesterId, weekId, departmentId, inspectionTypeId, recordId }
 const ready = !!semesterId && !!weekId && !!departmentId
 const classes = useInspectionEntryOptions({ ...scope, kind: 'classes' }, !readOnly && ready)
 const students = useInspectionEntryOptions({ ...scope, kind: 'students', classId }, !readOnly && ready && !!classId)
 const classrooms = useInspectionEntryOptions({ ...scope, kind: 'classrooms' }, !readOnly && ready)
 const queries = readOnly ? [semesters, weeks, departments] : [semesters, weeks, departments, classes, students, classrooms]
 const failed = queries.some(query => query.isError)
 useEffect(() => {
  if (!editing && !memberEntry && departments.data?.items.some(item => item.id === COLLEGE_DEPARTMENT_OPTION_ID) && !form.getFieldValue('departmentId')) form.setFieldValue('departmentId', COLLEGE_DEPARTMENT_OPTION_ID)
 }, [departments.data?.items, editing, form, memberEntry])
 const clear = (names: (keyof Values)[]) => form.setFields(names.map(name => ({ name, value: undefined, errors: [] })))
 const submit = async (values: Values) => {
  if (submitting.current || failed || readOnly) return
  submitting.current = true
  setError('')
  const { semesterId, weekId, departmentId, reason, ...details } = values
  try {
   await mutation.mutateAsync({ id: editing?.id, version: editing?.rowVersion, reason, record: { semesterId, weekId, departmentId, inspectionTypeId, details: { ...details, remark: details.remark ?? '' } } })
   onClose()
  } catch (failure) {
   setError(failure instanceof ApiError && failure.status < 500
    ? failure.code === 'VERSION_CONFLICT' ? '记录已变化，请关闭并重新打开后核对。' : failure.message
    : '保存失败，请稍后重试。')
  } finally { submitting.current = false }
 }
 const fallback = (id: string, label: string): InspectionOption => ({ id, label })
 const title = editing ? `${readOnly ? '查看' : '编辑'}晚自习缺勤记录` : '新增晚自习缺勤记录'

 return <Drawer open title={title} size={720} onClose={() => { if (!submitting.current) onClose() }} closable={!mutation.isPending} maskClosable={!mutation.isPending}
  footer={readOnly ? null : <Space><Button onClick={onClose} disabled={mutation.isPending}>取消</Button><Button type="primary" loading={mutation.isPending} disabled={failed || !memberDepartmentAvailable || !effectiveMultiplier} onClick={() => form.submit()}>{editing ? '保存修改' : '保存草稿'}</Button></Space>}>
  {error && <Alert type="error" showIcon title={error} style={{ marginBottom: 12 }} />}
  {failed && <Alert type="error" showIcon title="录入选项加载失败，请重试。" action={<Button onClick={() => queries.forEach(query => { if (query.isError) void query.refetch() })}>重试</Button>} style={{ marginBottom: 12 }} />}
  {!memberDepartmentAvailable && !departments.isLoading && <Alert type="warning" showIcon title="当前账号所属部门无此范围的录入权限，请联系管理员。" style={{ marginBottom: 12 }} />}
  {weekId && (effectiveMultiplier ? <Alert type="info" showIcon title={`固定第 9–10 节，每次缺勤计 1 次；按 ${effectiveMultiplier} 倍计入违纪，共 ${effectiveMultiplier} 次。`} style={{ marginBottom: 12 }} /> : <Alert type="warning" showIcon title="该周的违纪计入倍数尚未配置，请联系管理员。" style={{ marginBottom: 12 }} />)}
  <Form name="evening-self-study-record" form={form} layout="vertical" onFinish={submit} disabled={readOnly || mutation.isPending}
   initialValues={editing ? { semesterId: editing.semesterId, weekId: editing.weekId, departmentId: departmentOptionId(editing.departmentId), classId: editing.details.classId, studentId: editing.details.studentId, classroomId: editing.details.classroomId, date: editing.details.date, time: editing.details.time, remark: editing.details.remark } : { semesterId: initialSemesterId, weekId: initialWeekId, departmentId: memberDepartmentId }}>
   <Row gutter={16}>
    <Col xs={24} sm={12}><Form.Item name="semesterId" label="录入学期" rules={required}><Select loading={semesters.isFetching} options={choices(semesters.data?.items)} onChange={() => clear(memberEntry ? ['weekId', 'classId', 'studentId', 'classroomId'] : ['weekId', 'departmentId', 'classId', 'studentId', 'classroomId'])} /></Form.Item></Col>
    <Col xs={24} sm={12}><Form.Item name="weekId" label="录入周次" rules={required}><Select disabled={!semesterId} loading={weeks.isFetching} options={choices(weeks.data?.items)} onChange={() => clear(memberEntry ? ['classId', 'studentId', 'classroomId'] : ['departmentId', 'classId', 'studentId', 'classroomId'])} /></Form.Item></Col>
    <Col xs={24} sm={12}><Form.Item name="departmentId" label={memberEntry ? '录入归属部门' : '目标部门'} rules={required}><Select disabled={readOnly || !weekId || memberEntry} loading={departments.isFetching} options={choices(departments.data?.items, editing ? fallback(editing.departmentId, '原记录部门') : undefined)} onChange={() => clear(['classId', 'studentId', 'classroomId'])} /></Form.Item></Col>
    <Col xs={24} sm={12}><Form.Item name="classId" label="学生班级" rules={required}><Select disabled={!ready} showSearch optionFilterProp="label" loading={classes.isFetching} options={choices(classes.data, editing ? fallback(editing.details.classId, editing.className) : undefined)} onChange={() => clear(['studentId'])} /></Form.Item></Col>
    <Col xs={24} sm={12}><Form.Item name="studentId" label="缺勤学生" rules={required}><Select disabled={!classId} showSearch optionFilterProp="label" loading={students.isFetching} options={choices(students.data, editing && editing.details.classId === classId ? fallback(editing.details.studentId, `${editing.studentName}（${editing.studentNumber}）`) : undefined)} /></Form.Item></Col>
    <Col xs={24} sm={12}><Form.Item name="classroomId" label="晚自习教室" rules={required}><Select disabled={!ready} showSearch optionFilterProp="label" loading={classrooms.isFetching} options={choices(classrooms.data, editing ? fallback(editing.details.classroomId, editing.currentClassroomName) : undefined)} /></Form.Item></Col>
    <Col xs={24} sm={12}><ProFormDatePicker name="date" label="缺勤日期" rules={required} getValueFromEvent={(_date: unknown, dateString: string) => dateString || undefined} fieldProps={{ format: 'YYYY-MM-DD', style: { width: '100%' } }} /></Col>
    <Col xs={24} sm={12}><ProFormTimePicker name="time" label="检查时间" rules={required} getValueFromEvent={(_time: unknown, timeString: string) => timeString || undefined} fieldProps={{ format: 'HH:mm:ss', style: { width: '100%' } }} /></Col>
    <Col xs={24}><Form.Item label="检查节次"><Tag color="blue">第 9–10 节</Tag></Form.Item></Col>
    <Col xs={24}><Form.Item label="基础缺勤次数"><Tag color="blue">1 次</Tag></Form.Item></Col>
   </Row>
   {editing && !readOnly && <Form.Item name="reason" label="修改原因" rules={[...required, { whitespace: true, message: '请填写修改原因' }]}><Input.TextArea maxLength={255} /></Form.Item>}
   <Form.Item name="remark" label="备注"><Input.TextArea maxLength={500} rows={3} /></Form.Item>
  </Form>
 </Drawer>
}

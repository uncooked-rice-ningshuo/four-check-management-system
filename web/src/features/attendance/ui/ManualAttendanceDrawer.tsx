import { Alert, Button, Col, Drawer, Form, Input, InputNumber, Row, Select, Space } from 'antd'
import { useRef, useState } from 'react'
import type { AttendanceFilterOptions, AttendanceRecord, ManualAttendanceInput } from '../../../entities/inspection/model/inspection'
import { ApiError } from '../../../shared/api/httpClient'
import { useUpdateAttendance } from '../hooks/useAttendanceMutations'
import { useCreateAttendance } from '../hooks/useCreateAttendance'

type FormValues = Omit<ManualAttendanceInput, 'inspectionTypeId' | 'className' | 'grade'> & { reason?: string }
type Props = { editing?: AttendanceRecord; options: AttendanceFilterOptions; initialSemesterId?: string; initialWeekId?: string; onClose: () => void }
const required = [{ required: true, message: '请填写或选择此项' }]

export function ManualAttendanceDrawer({ options, initialSemesterId, initialWeekId, editing: initialEditing, onClose }: Props) {
  // Keep the version paired with the original form values during background refetches.
  const [editing] = useState(initialEditing)
  const [form] = Form.useForm<FormValues>()
  const semesterId = Form.useWatch('semesterId', form)
  const create = useCreateAttendance()
  const update = useUpdateAttendance()
  const mutation = editing ? update : create
  const submitting = useRef(false)
  const [error, setError] = useState('')
  const weeks = options.weeks.filter(week => week.semesterId === semesterId)
  const departments = options.responsibleDepartments.filter(item => options.departments.some(allowed => allowed.id === item.id))
  const submit = async (values: FormValues) => {
    if (submitting.current) return
    const selectedClass = options.classes.find(item => item.id === values.classId)
    if (!selectedClass) { setError('班级信息已变化，请刷新页面后重试'); return }
    submitting.current = true
    setError('')
    try {
      const { reason, ...fields } = values
      const record = { ...fields, inspectionTypeId: options.inspectionType.id, className: selectedClass.name, grade: selectedClass.grade }
      if (editing) await update.mutateAsync({ id: editing.id, version: editing.rowVersion, reason: reason ?? '', record })
      else await create.mutateAsync(record)
      onClose()
    } catch (failure) {
      setError(failure instanceof ApiError
        ? failure.code === 'FORBIDDEN_SCOPE' ? '无权在所选部门、班级或周次录入，请调整目标范围或联系管理员。'
          : failure.code === 'RECORD_ALREADY_EXISTS' ? '相同记录已存在，列表已刷新，请核对后再操作。'
            : failure.code === 'VERSION_CONFLICT' ? '记录已被修改，请关闭并重新打开记录，核对最新内容后再保存。'
            : ['VALIDATION_FAILED', 'RECORD_NOT_EDITABLE', 'INSPECTION_NOT_FOUND'].includes(failure.code) ? failure.message : '保存失败，请稍后重试。'
        : '保存失败，请检查网络后重试。')
    } finally { submitting.current = false }
  }
  return <Drawer open title={editing ? "编辑到课率记录" : "新增到课率记录"} size={720} onClose={() => { if (!submitting.current) onClose() }} closable={!mutation.isPending} maskClosable={!mutation.isPending}
    footer={<Space><Button disabled={mutation.isPending} onClick={onClose}>取消</Button><Button type="primary" loading={mutation.isPending} onClick={() => form.submit()}>{editing ? "保存修改" : "保存草稿"}</Button></Space>}>
    {error && <Alert type="error" showIcon title={error} style={{ marginBottom: 16 }} />}
    <Form name="manual-attendance" form={form} layout="vertical" onFinish={submit} disabled={mutation.isPending} initialValues={editing ? { ...editing, classroom: editing.classroomCode, date: new Date(editing.occurredAt).toLocaleDateString('sv-SE', { timeZone: 'Asia/Shanghai' }), time: new Date(editing.occurredAt).toLocaleTimeString('en-GB', { timeZone: 'Asia/Shanghai', hour: '2-digit', minute: '2-digit' }) } : { semesterId: initialSemesterId, weekId: initialWeekId }}>
      <Row gutter={16}>
        <Col xs={24} sm={12}><Form.Item name="semesterId" label="录入学期" rules={required}><Select options={options.semesters.map(item => ({ value: item.id, label: item.name }))} onChange={() => form.setFieldValue('weekId', undefined)} /></Form.Item></Col>
        <Col xs={24} sm={12}><Form.Item name="weekId" label="录入周次" rules={required}><Select disabled={!semesterId} options={weeks.map(item => ({ value: item.id, label: `第 ${item.weekNo} 周` }))} /></Form.Item></Col>
        <Col xs={24} sm={12}><Form.Item name="departmentId" label="目标部门" rules={required}><Select options={departments.map(item => ({ value: item.id, label: item.name }))} /></Form.Item></Col>
        <Col xs={24} sm={12}><Form.Item name="classId" label="学生班级" rules={required}><Select showSearch optionFilterProp="label" options={options.classes.map(item => ({ value: item.id, label: item.name }))} /></Form.Item></Col>
        <Col xs={24} sm={12}><Form.Item name="studentNo" label="学号" rules={[...required, { whitespace: true, message: '学号不能为空' }]}><Input maxLength={64} autoComplete="off" /></Form.Item></Col>
        <Col xs={24} sm={12}><Form.Item name="studentName" label="姓名" rules={[...required, { whitespace: true, message: '姓名不能为空' }]}><Input maxLength={100} autoComplete="off" /></Form.Item></Col>
        <Col xs={24} sm={12}><Form.Item name="courseName" label="课程名称" rules={required}><Input maxLength={200} placeholder="填写该班级在所选学期的课程名称" /></Form.Item></Col>
        <Col xs={24} sm={12}><Form.Item name="classroom" label="上课教室" rules={required}><Input maxLength={200} placeholder="例如：博学楼 101" /></Form.Item></Col>
        <Col xs={24} sm={12}><Form.Item name="date" label="检查日期" rules={required}><Input type="date" /></Form.Item></Col>
        <Col xs={24} sm={12}><Form.Item name="time" label="检查时间" rules={required}><Input type="time" /></Form.Item></Col>
        <Col xs={24} sm={12}><Form.Item name="period" label="节次" rules={required}><Input maxLength={64} placeholder="例如：1-2" /></Form.Item></Col>
        <Col xs={24} sm={12}><Form.Item name="absenceHours" label="旷课学时" rules={[...required, { type: 'number', min: 0.01, max: 24, message: '请输入大于 0 且不超过 24 的学时' }]}><InputNumber min={0.01} max={24} style={{ width: '100%' }} /></Form.Item></Col>
      </Row>
      {editing && <Form.Item name="reason" label="修改原因" rules={[...required, { whitespace: true, message: '请填写修改原因' }]}><Input.TextArea maxLength={255} rows={2} /></Form.Item>}
      <Form.Item name="remark" label="备注"><Input.TextArea maxLength={500} showCount rows={3} /></Form.Item>
    </Form>
  </Drawer>
}

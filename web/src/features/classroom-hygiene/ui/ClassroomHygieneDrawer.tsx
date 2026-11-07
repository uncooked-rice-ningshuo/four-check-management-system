import { ProFormDatePicker } from '@ant-design/pro-components'
import { Alert, Button, Col, Divider, Drawer, Form, Input, Row, Select, Space } from 'antd'
import { useEffect, useRef, useState } from 'react'
import type { ClassroomHygieneRecord, HygieneResult, InspectionOption } from '../../../entities/inspection/model/classroomHygieneRecord'
import { COLLEGE_DEPARTMENT_OPTION_ID, departmentOptionId } from '../../../entities/inspection/model/inspectionScope'
import { InspectionAttributionNote } from '../../../shared/components/InspectionAttributionNote'
import { HygieneSituationSnippets } from '../../../shared/components/HygieneSituationSnippets'
import { ApiError } from '../../../shared/api/httpClient'
import { appendHygieneSituationSnippet } from '../../../shared/utils/hygieneSituation'
import { useClassroomHygieneMutation, useInspectionContext, useInspectionEntryOptions, useResponsibleStudentOptions } from '../model/queries'

type Values = { semesterId: string; weekId: string; departmentId: string; classroomId: string; date: string; result: HygieneResult; responsibleClassIds: string[]; responsibleStudentIds: string[]; description?: string; remark?: string; reason?: string }
type Props = { editing?: ClassroomHygieneRecord; inspectionTypeId: string; initialSemesterId?: string; initialWeekId?: string; memberDepartmentId?: string; isSuperAdmin?: boolean; onClose: () => void }
const required = [{ required: true, message: '请填写或选择此项' }]
const choices = (items?: InspectionOption[], fallback: InspectionOption[] = []) => [...fallback.filter(item => !(items ?? []).some(candidate => candidate.id === item.id)), ...(items ?? [])].map(item => ({ value: item.id, label: item.label }))

export function ClassroomHygieneDrawer({ editing: initialEditing, inspectionTypeId, initialSemesterId, initialWeekId, memberDepartmentId, isSuperAdmin = false, onClose }: Props) {
  const [editing] = useState(initialEditing)
  const memberEntry = memberDepartmentId !== undefined
  const [form] = Form.useForm<Values>()
  const semesterId = Form.useWatch('semesterId', form), weekId = Form.useWatch('weekId', form), departmentId = Form.useWatch('departmentId', form)
  const classIds = Form.useWatch('responsibleClassIds', form) ?? editing?.details.responsibleClassIds.slice(0, 1) ?? []
  const primaryClassId = classIds[0]
  const result = Form.useWatch('result', form) ?? editing?.details.result ?? 'QUALIFIED'
  const mutation = useClassroomHygieneMutation(), submitting = useRef(false)
  const [error, setError] = useState('')
  const recordId = editing?.id
  const semesters = useInspectionContext({ purpose: 'entry', recordId, kind: 'semesters' })
  const weeks = useInspectionContext({ purpose: 'entry', recordId, kind: 'weeks', semesterId }, !!semesterId)
  const departments = useInspectionContext({ purpose: 'entry', recordId, kind: 'departments', semesterId, weekId }, !!semesterId && !!weekId)
  const scope = { semesterId, weekId, departmentId, inspectionTypeId, recordId }
  const departmentReady = isSuperAdmin || (!!departmentId && (!!editing || departments.data?.items.some(item => item.id === departmentId)))
  const ready = !!semesterId && !!weekId && departmentReady
  const classes = useInspectionEntryOptions({ ...scope, kind: 'classes' }, ready)
  const students = useResponsibleStudentOptions(scope, classIds, ready)
  const classrooms = useInspectionEntryOptions({ ...scope, kind: 'classroom-objects', classId: primaryClassId }, ready && !!primaryClassId)
  const snippets = useInspectionEntryOptions({ ...scope, kind: 'situation-snippets' }, ready)
  const coreQueries = [semesters, weeks, departments, classes, students, classrooms]
  const failed = coreQueries.some(query => query.isError)
  const clear = (names: (keyof Values)[]) => form.setFields(names.map(name => ({ name, value: undefined, errors: [] })))
  // Keep the college default only when it is an authorized option. Never infer
  // a different department from list order; a blank required field makes the
  // user choose the intended record attribution explicitly.
  useEffect(() => {
    if (editing || isSuperAdmin || memberEntry) return
    const items = departments.data?.items
    if (!items) return
    const current = form.getFieldValue('departmentId')
    if (current && items.some(item => item.id === current)) return
    if (items.some(item => item.id === COLLEGE_DEPARTMENT_OPTION_ID)) {
      form.setFieldValue('departmentId', COLLEGE_DEPARTMENT_OPTION_ID)
    } else if (current) {
      form.setFieldValue('departmentId', undefined)
    }
  }, [departments.data?.items, editing, form, isSuperAdmin, memberEntry])
  const insertSnippet = (text: string) => {
    form.setFieldValue('description', appendHygieneSituationSnippet(form.getFieldValue('description'), text))
  }
  const submit = async (values: Values) => {
    if (submitting.current || failed) return
    submitting.current = true; setError('')
    const details = { classroomId: values.classroomId, date: values.date, result: values.result, responsibleClassIds: values.responsibleClassIds.slice(0, 1), responsibleStudentIds: values.responsibleStudentIds.slice(0, 1), description: values.description ?? '', remark: values.remark ?? '' }
    try {
      await mutation.mutateAsync({ id: editing?.id, version: editing?.rowVersion, reason: values.reason, record: { semesterId: values.semesterId, weekId: values.weekId, departmentId: isSuperAdmin && editing?.attributionMode === 'ADMINISTRATOR' ? '' : values.departmentId, inspectionTypeId, ...(!editing && isSuperAdmin ? { attributionMode: 'ADMINISTRATOR' as const } : {}), details } })
      onClose()
    } catch (failure) {
      setError(failure instanceof ApiError && failure.status < 500 ? failure.code === 'VERSION_CONFLICT' ? '记录已变化，请关闭并重新打开后核对。' : failure.message : '保存失败，请稍后重试。')
    } finally { submitting.current = false }
  }
  const initialDepartmentId = editing ? departmentOptionId(editing.departmentId) : (isSuperAdmin ? '' : memberDepartmentId ?? COLLEGE_DEPARTMENT_OPTION_ID)
  const semesterClear: (keyof Values)[] = memberEntry ? ['weekId', 'responsibleClassIds', 'responsibleStudentIds', 'classroomId'] : ['weekId', 'departmentId', 'responsibleClassIds', 'responsibleStudentIds', 'classroomId']
  return <Drawer open title={editing ? '编辑教室卫生记录' : '新增教室卫生记录'} size={760} className="inspection-entry-drawer"
    onClose={() => { if (!submitting.current) onClose() }} closable={!mutation.isPending} maskClosable={!mutation.isPending}
    footer={<Space><Button onClick={onClose} disabled={mutation.isPending}>取消</Button><Button type="primary" loading={mutation.isPending} disabled={failed} onClick={() => form.submit()}>{editing ? '保存修改' : '保存草稿'}</Button></Space>}>
    {error && <Alert type="error" showIcon title={error} />}
    {failed && <Alert type="error" showIcon title="录入选项加载失败，请重试。" action={<Button onClick={() => coreQueries.forEach(query => { if (query.isError) void query.refetch() })}>重试</Button>} />}
    <Form form={form} name="classroom-hygiene-record" layout="vertical" onFinish={submit} disabled={mutation.isPending}
      initialValues={editing ? { semesterId: editing.semesterId, weekId: editing.weekId, departmentId: initialDepartmentId, classroomId: editing.details.classroomId, date: editing.details.date, result: editing.details.result, responsibleClassIds: editing.details.responsibleClassIds.slice(0, 1), responsibleStudentIds: editing.details.responsibleStudentIds?.slice(0, 1) ?? [], description: editing.details.description ?? '', remark: editing.details.remark ?? '' } : { semesterId: initialSemesterId, weekId: initialWeekId, departmentId: initialDepartmentId, result: 'QUALIFIED', responsibleClassIds: [], responsibleStudentIds: [] }}>
      <Divider titlePlacement="start" plain>录入范围</Divider>
      <Row gutter={16}>
        <Col xs={24} sm={12}><Form.Item name="semesterId" label="录入学期" rules={required}><Select loading={semesters.isFetching} options={choices(semesters.data?.items)} onChange={() => clear(semesterClear)} /></Form.Item></Col>
        <Col xs={24} sm={12}><Form.Item name="weekId" label="录入周次" rules={required}><Select disabled={!semesterId} loading={weeks.isFetching} options={choices(weeks.data?.items)} onChange={() => clear(['responsibleClassIds', 'responsibleStudentIds', 'classroomId'])} /></Form.Item></Col>
        {isSuperAdmin ? <Col span={24}><Form.Item name="departmentId" hidden><Input /></Form.Item><InspectionAttributionNote editing={!!editing} administratorOwned={editing?.attributionMode === 'ADMINISTRATOR'} /></Col> : <Col xs={24} sm={12}><Form.Item name="departmentId" label={memberEntry ? '录入归属部门' : '业务归属部门'} rules={required}><Select disabled={!weekId || memberEntry} loading={departments.isFetching} options={choices(departments.data?.items)} onChange={() => clear(['responsibleClassIds', 'responsibleStudentIds', 'classroomId'])} /></Form.Item></Col>}
      </Row>
      <Divider titlePlacement="start" plain>卫生检查</Divider>
      <Row gutter={16}>
        <Col xs={24} sm={12}><Form.Item name="responsibleClassIds" label="责任学生班级" rules={required}><Select mode="multiple" maxCount={1} disabled={!ready} showSearch optionFilterProp="label" loading={classes.isFetching} options={choices(classes.data, editing?.responsibleClasses)} onChange={() => clear(['responsibleStudentIds', 'classroomId'])} /></Form.Item></Col>
        <Col xs={24} sm={12}><Form.Item name="classroomId" label="教室" rules={required}><Select disabled={!primaryClassId} showSearch optionFilterProp="label" loading={classrooms.isFetching} options={choices(classrooms.data, editing ? [{ id: editing.details.classroomId, label: editing.currentClassroomName }] : [])} /></Form.Item></Col>
        <Col xs={24} sm={12}><Form.Item name="responsibleStudentIds" label="责任学生" rules={required}><Select mode="multiple" maxCount={1} disabled={!classIds.length} showSearch optionFilterProp="label" loading={students.isFetching} options={choices(students.data, editing?.responsibleStudents)} /></Form.Item></Col>
        <Col xs={24} sm={12}><ProFormDatePicker name="date" label="检查日期" rules={required} getValueFromEvent={(_date: unknown, dateString: string) => dateString || undefined} fieldProps={{ format: 'YYYY-MM-DD', style: { width: '100%' } }} /></Col>
        <Col xs={24} sm={12}><Form.Item name="result" label="检查结果" rules={required}><Select options={[{ value: 'EXCELLENT', label: '优秀' }, { value: 'QUALIFIED', label: '合格' }, { value: 'UNQUALIFIED', label: '不合格' }]} /></Form.Item></Col>
      </Row>
      <Divider titlePlacement="start" plain>现场说明</Divider>
      <Form.Item name="description" label="情况说明" rules={result === 'UNQUALIFIED' ? [{ required: true, whitespace: true, message: '不合格时请填写情况说明' }] : undefined}><Input.TextArea maxLength={1000} rows={3} /></Form.Item>
      <HygieneSituationSnippets items={snippets.data} loading={snippets.isFetching} failed={snippets.isError} onChoose={insertSnippet} />
      {editing && <Form.Item name="reason" label="修改原因" rules={[...required, { whitespace: true, message: '请填写修改原因' }]}><Input.TextArea maxLength={255} /></Form.Item>}
      <Form.Item name="remark" label="备注"><Input.TextArea maxLength={500} rows={2} /></Form.Item>
    </Form>
  </Drawer>
}

import { ProFormDatePicker } from '@ant-design/pro-components'
import { Alert, Button, Col, Drawer, Form, Input, Row, Select, Space, Typography } from 'antd'
import { useEffect, useRef, useState } from 'react'
import type { PublicDormitoryHygieneRecord, HygieneResult, InspectionOption } from '../../../entities/inspection/model/publicDormitoryHygieneRecord'
import { COLLEGE_DEPARTMENT_OPTION_ID, departmentOptionId } from '../../../entities/inspection/model/inspectionScope'
import { HygieneSituationSnippets } from '../../../shared/components/HygieneSituationSnippets'
import { ApiError } from '../../../shared/api/httpClient'
import { appendHygieneSituationSnippet } from '../../../shared/utils/hygieneSituation'
import { usePublicDormitoryHygieneMutation, useInspectionContext, useInspectionEntryOptions, useResponsibleStudentOptions } from '../model/queries'

type Values = { semesterId: string; weekId: string; departmentId: string; responsibleDormitoryIds: string[]; date: string; result: HygieneResult; responsibleClassIds: string[]; responsibleStudentIds: string[]; description?: string; remark?: string; reason?: string }
type Props = { editing?: PublicDormitoryHygieneRecord; inspectionTypeId: string; initialSemesterId?: string; initialWeekId?: string; memberDepartmentId?: string; isSuperAdmin?: boolean; onClose: () => void }
const required = [{ required: true, message: '请填写或选择此项' }]
const choices = (items?: InspectionOption[], fallback: InspectionOption[] = []) => [...fallback.filter(item => !(items ?? []).some(candidate => candidate.id === item.id)), ...(items ?? [])].map(item => ({ value: item.id, label: item.label }))

export function PublicDormitoryHygieneDrawer({ editing: initialEditing, inspectionTypeId, initialSemesterId, initialWeekId, memberDepartmentId, isSuperAdmin = false, onClose }: Props) {
  const [editing] = useState(initialEditing)
  const memberEntry = memberDepartmentId !== undefined
  const [form] = Form.useForm<Values>()
  const semesterId = Form.useWatch('semesterId', form), weekId = Form.useWatch('weekId', form), departmentId = Form.useWatch('departmentId', form)
  const classIds = Form.useWatch('responsibleClassIds', form) ?? editing?.details.responsibleClassIds.slice(0, 1) ?? []
  const primaryClassId = classIds[0]
  const result = Form.useWatch('result', form) ?? editing?.details.result ?? 'QUALIFIED'
  const mutation = usePublicDormitoryHygieneMutation(), submitting = useRef(false)
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
  const dormitories = useInspectionEntryOptions({ ...scope, kind: 'responsible-dormitories', classId: primaryClassId }, ready && !!primaryClassId)
  const snippets = useInspectionEntryOptions({ ...scope, kind: 'situation-snippets' }, ready)
  const coreQueries = [semesters, weeks, departments, classes, students, dormitories]
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
    const details = { responsibleDormitoryIds: values.responsibleDormitoryIds.slice(0, 1), date: values.date, result: values.result, responsibleClassIds: values.responsibleClassIds.slice(0, 1), responsibleStudentIds: values.responsibleStudentIds.slice(0, 1), description: values.description ?? '', remark: values.remark ?? '' }
    try {
      await mutation.mutateAsync({ id: editing?.id, version: editing?.rowVersion, reason: values.reason, record: { semesterId: values.semesterId, weekId: values.weekId, departmentId: isSuperAdmin && editing?.attributionMode === 'ADMINISTRATOR' ? '' : values.departmentId, inspectionTypeId, ...(!editing && isSuperAdmin ? { attributionMode: 'ADMINISTRATOR' as const } : {}), details } })
      onClose()
    } catch (failure) {
      setError(failure instanceof ApiError && failure.status < 500 ? failure.code === 'VERSION_CONFLICT' ? '记录已变化，请关闭并重新打开后核对。' : failure.message : '保存失败，请稍后重试。')
    } finally { submitting.current = false }
  }
  const dormitoryFallback = editing?.responsibleDormitories ?? []
  const initialDepartmentId = editing ? departmentOptionId(editing.departmentId) : (isSuperAdmin ? '' : memberDepartmentId ?? COLLEGE_DEPARTMENT_OPTION_ID)
  const semesterClear: (keyof Values)[] = memberEntry ? ['weekId', 'responsibleClassIds', 'responsibleStudentIds', 'responsibleDormitoryIds'] : ['weekId', 'departmentId', 'responsibleClassIds', 'responsibleStudentIds', 'responsibleDormitoryIds']
  return <Drawer open title={editing ? '编辑公共宿舍卫生记录' : '新增公共宿舍卫生记录'} size={760}
    onClose={() => { if (!submitting.current) onClose() }} closable={!mutation.isPending} maskClosable={!mutation.isPending}
    footer={<Space><Button onClick={onClose} disabled={mutation.isPending}>取消</Button><Button type="primary" loading={mutation.isPending} disabled={failed} onClick={() => form.submit()}>{editing ? '保存修改' : '保存草稿'}</Button></Space>}>
    {error && <Alert type="error" showIcon title={error} />}
    {failed && <Alert type="error" showIcon title="录入选项加载失败，请重试。" action={<Button onClick={() => coreQueries.forEach(query => { if (query.isError) void query.refetch() })}>重试</Button>} />}
    <Form form={form} name="public-dormitory-hygiene-record" layout="vertical" onFinish={submit} disabled={mutation.isPending}
      initialValues={editing ? { semesterId: editing.semesterId, weekId: editing.weekId, departmentId: initialDepartmentId, responsibleDormitoryIds: editing.details.responsibleDormitoryIds?.slice(0, 1) ?? editing.responsibleDormitories.slice(0, 1).map(item => item.id), date: editing.details.date, result: editing.details.result, responsibleClassIds: editing.details.responsibleClassIds.slice(0, 1), responsibleStudentIds: editing.details.responsibleStudentIds?.slice(0, 1) ?? [], description: editing.details.description ?? '', remark: editing.details.remark ?? '' } : { semesterId: initialSemesterId, weekId: initialWeekId, departmentId: initialDepartmentId, result: 'QUALIFIED', responsibleDormitoryIds: [], responsibleClassIds: [], responsibleStudentIds: [] }}>
      <Row gutter={16}>
        <Col xs={24} sm={12}><Form.Item name="semesterId" label="录入学期" rules={required}><Select loading={semesters.isFetching} options={choices(semesters.data?.items)} onChange={() => clear(semesterClear)} /></Form.Item></Col>
        <Col xs={24} sm={12}><Form.Item name="weekId" label="录入周次" rules={required}><Select disabled={!semesterId} loading={weeks.isFetching} options={choices(weeks.data?.items)} onChange={() => clear(['responsibleClassIds', 'responsibleStudentIds', 'responsibleDormitoryIds'])} /></Form.Item></Col>
        {isSuperAdmin ? <Col span={24}><Form.Item name="departmentId" hidden><Input /></Form.Item><Typography.Text type="secondary">{editing ? (editing.attributionMode === 'ADMINISTRATOR' ? '此记录归属管理员独立数据' : '修改将保留原记录归属部门') : '本次记录归属管理员，不需要选择目标部门'}</Typography.Text></Col> : <Col xs={24} sm={12}><Form.Item name="departmentId" label={memberEntry ? '录入归属部门' : '业务归属部门'} rules={required}><Select disabled={!weekId || memberEntry} loading={departments.isFetching} options={choices(departments.data?.items)} onChange={() => clear(['responsibleClassIds', 'responsibleStudentIds', 'responsibleDormitoryIds'])} /></Form.Item></Col>}
        <Col xs={24} sm={12}><Form.Item name="responsibleClassIds" label="责任班级" rules={required}><Select mode="multiple" maxCount={1} disabled={!ready} showSearch optionFilterProp="label" loading={classes.isFetching} options={choices(classes.data, editing?.responsibleClasses)} onChange={() => clear(['responsibleStudentIds', 'responsibleDormitoryIds'])} /></Form.Item></Col>
        <Col xs={24} sm={12}><Form.Item name="responsibleDormitoryIds" label="责任寝室" rules={required}><Select mode="multiple" maxCount={1} disabled={!primaryClassId} showSearch optionFilterProp="label" loading={dormitories.isFetching} options={choices(dormitories.data, dormitoryFallback)} /></Form.Item></Col>
        <Col xs={24} sm={12}><Form.Item name="responsibleStudentIds" label="负责学生" rules={required}><Select mode="multiple" maxCount={1} disabled={!classIds.length} showSearch optionFilterProp="label" loading={students.isFetching} options={choices(students.data, editing?.responsibleStudents)} /></Form.Item></Col>
        <Col xs={24} sm={12}><ProFormDatePicker name="date" label="检查日期" rules={required} getValueFromEvent={(_date: unknown, dateString: string) => dateString || undefined} fieldProps={{ format: 'YYYY-MM-DD', style: { width: '100%' } }} /></Col>
        <Col xs={24} sm={12}><Form.Item name="result" label="检查结果" rules={required}><Select options={[{ value: 'EXCELLENT', label: '优秀' }, { value: 'QUALIFIED', label: '合格' }, { value: 'UNQUALIFIED', label: '不合格' }]} /></Form.Item></Col>
      </Row>
      <Form.Item name="description" label="情况说明" rules={result === 'UNQUALIFIED' ? [{ required: true, whitespace: true, message: '不合格时请填写情况说明' }] : undefined}><Input.TextArea maxLength={1000} rows={3} /></Form.Item>
      <HygieneSituationSnippets items={snippets.data} loading={snippets.isFetching} failed={snippets.isError} onChoose={insertSnippet} />
      {editing && <Form.Item name="reason" label="修改原因" rules={[...required, { whitespace: true, message: '请填写修改原因' }]}><Input.TextArea maxLength={255} /></Form.Item>}
      <Form.Item name="remark" label="备注"><Input.TextArea maxLength={500} rows={2} /></Form.Item>
    </Form>
  </Drawer>
}

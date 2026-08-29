import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ProTable } from '@ant-design/pro-components'
import { Alert, Button, Card, Empty, Form, Input, InputNumber, Modal, Select, Space, Tabs, Tag, Typography, message } from 'antd'
import { useState } from 'react'
import type { AcademicYear, Semester, SemesterWeek } from '../../entities/academic/model/academic'
import { academicYearOptions, buildAcademicYearDefaults, buildSemesterDefaults, currentAcademicYearStart, semesterTypeOptions, type SemesterCode } from '../../features/academic/model/academicCalendarRules'
import { createAcademicYear, createSemester, createWeek, getAcademicCalendar, updateAcademicYear, updateSemester, updateWeek } from '../../shared/api/adminApi'

type ModalKind = 'year' | 'semester' | 'week' | null
const toISO = (value: string) => `${value}T00:00:00Z`
const displayDate = (value: string) => value.slice(0, 10)

export function AcademicCalendarPage() {
  const [modal, setModal] = useState<ModalKind>(null)
  const [editing, setEditing] = useState<AcademicYear | Semester | SemesterWeek | null>(null)
  const [form] = Form.useForm()
  const client = useQueryClient()
  const calendar = useQuery({ queryKey: ['academic-calendar'], queryFn: getAcademicCalendar })
  const save = useMutation({ mutationFn: async (values: Record<string, string | number>) => {
    if (modal === 'year') { const defaults = buildAcademicYearDefaults(Number(values.startYear)); const input = { name: defaults.name, startDate: toISO(String(values.startDate)), endDate: toISO(String(values.endDate)) }; return editing ? updateAcademicYear({ ...input, id: editing.id, enabled: editing.enabled }) : createAcademicYear(input) }
    if (modal === 'semester') { const year = data.academicYears.find(item => item.id === values.academicYearId); const defaults = buildSemesterDefaults(Number(displayDate(year?.startDate ?? '').slice(0, 4)), String(values.semesterCode) as SemesterCode); const input = { academicYearId: String(values.academicYearId), code: defaults.code, name: defaults.name, startDate: toISO(String(values.startDate)), endDate: toISO(String(values.endDate)) }; return editing ? updateSemester({ ...input, id: editing.id, enabled: editing.enabled }) : createSemester(input) }
    const input = { semesterId: String(values.semesterId), weekNo: Number(values.weekNo), startDate: toISO(String(values.startDate)), endDate: toISO(String(values.endDate)), note: String(values.note ?? '') }; return editing ? updateWeek({ ...input, id: editing.id, enabled: editing.enabled }) : createWeek(input)
  }, onSuccess: async () => { await client.invalidateQueries({ queryKey: ['academic-calendar'] }); setModal(null); setEditing(null); form.resetFields(); message.success('配置已保存') } })
  const data = calendar.data ?? { academicYears: [], semesters: [], weeks: [] }
  const yearChoices = academicYearOptions().map(option => ({ ...option, disabled: data.academicYears.some(year => Number(displayDate(year.startDate).slice(0, 4)) === option.value) }))
  const applySemesterDefaults = (academicYearId: string, code: SemesterCode) => {
    const year = data.academicYears.find(item => item.id === academicYearId)
    if (!year) return
    form.setFieldsValue(buildSemesterDefaults(Number(displayDate(year.startDate).slice(0, 4)), code))
  }
  const openCreate = (kind: Exclude<ModalKind, null>) => {
    setEditing(null)
    form.resetFields()
    setModal(kind)
  }
  const initializeCreateForm = (kind: Exclude<ModalKind, null>) => {
    if (kind === 'year') {
      const startYear = currentAcademicYearStart()
      form.setFieldsValue({ startYear, ...buildAcademicYearDefaults(startYear) })
    }
    if (kind === 'semester' && data.academicYears.length) {
      const academicYearId = data.academicYears[0].id
      const code = semesterTypeOptions.find(option => !data.semesters.some(item => item.academicYearId === academicYearId && item.code === option.value))?.value ?? 'FIRST'
      form.setFieldsValue({ academicYearId, semesterCode: code })
      applySemesterDefaults(academicYearId, code)
    }
  }
  const openEdit = (kind: Exclude<ModalKind, null>, record: AcademicYear | Semester | SemesterWeek) => {
    setEditing(record)
    setModal(kind)
    form.setFieldsValue({ ...record, startYear: kind === 'year' ? Number(displayDate(record.startDate).slice(0, 4)) : undefined, semesterCode: kind === 'semester' ? (record as Semester).code : undefined, startDate: displayDate(record.startDate), endDate: displayDate(record.endDate) })
  }
  const onFormValuesChange = (changed: Record<string, unknown>) => {
    if (modal === 'year' && typeof changed.startYear === 'number') form.setFieldsValue(buildAcademicYearDefaults(changed.startYear))
    if (modal === 'semester' && ('academicYearId' in changed || 'semesterCode' in changed)) {
      const academicYearId = String(form.getFieldValue('academicYearId') ?? '')
      let code = form.getFieldValue('semesterCode') as SemesterCode | undefined
      if ('academicYearId' in changed && code && data.semesters.some(item => item.academicYearId === academicYearId && item.code === code)) {
        code = semesterTypeOptions.find(option => !data.semesters.some(item => item.academicYearId === academicYearId && item.code === option.value))?.value
        form.setFieldValue('semesterCode', code)
      }
      if (academicYearId && code) applySemesterDefaults(academicYearId, code)
    }
  }

  return <section className="admin-page">
    <div className="page-heading"><div><Typography.Title level={2}>学期与周次</Typography.Title><Typography.Paragraph>系统初始允许为空；配置后按日期自动识别当前工作周。</Typography.Paragraph></div><Space><Button onClick={() => openCreate('week')} disabled={!data.semesters.length}>新增周次</Button><Button onClick={() => openCreate('semester')} disabled={!data.academicYears.length}>新增学期</Button><Button type="primary" onClick={() => openCreate('year')}>新增学年</Button></Space></div>
    {!calendar.isLoading && data.academicYears.length === 0 && <Card><Empty description="尚未配置学年和学期"><Button type="primary" onClick={() => openCreate('year')}>新增学年</Button></Empty></Card>}
    {calendar.isError && <Alert type="error" showIcon title="学期周次加载失败" />}
    {data.academicYears.length > 0 && <Card><Tabs items={[
      { key: 'years', label: '学年', children: <ProTable<AcademicYear> rowKey="id" search={false} options={false} pagination={false} dataSource={data.academicYears} columns={[{ title: '学年', dataIndex: 'name' }, { title: '开始日期', dataIndex: 'startDate', renderText: displayDate }, { title: '结束日期', dataIndex: 'endDate', renderText: displayDate }, { title: '状态', dataIndex: 'enabled', render: (_, record) => <Tag color={record.enabled ? 'green' : 'default'}>{record.enabled ? '启用' : '停用'}</Tag> }, { title: '操作', valueType: 'option', render: (_, record) => <Button type="link" onClick={() => openEdit('year', record)}>编辑</Button> }]} /> },
      { key: 'semesters', label: '学期', children: <ProTable<Semester> rowKey="id" search={false} options={false} pagination={false} dataSource={data.semesters} columns={[{ title: '名称', dataIndex: 'name' }, { title: '编码', dataIndex: 'code' }, { title: '开始日期', dataIndex: 'startDate', renderText: displayDate }, { title: '结束日期', dataIndex: 'endDate', renderText: displayDate }, { title: '操作', valueType: 'option', render: (_, record) => <Button type="link" onClick={() => openEdit('semester', record)}>编辑</Button> }]} /> },
      { key: 'weeks', label: '周次', children: <ProTable<SemesterWeek> rowKey="id" search={false} options={{ density: true, setting: true }} pagination={{ pageSize: 20 }} dataSource={data.weeks} columns={[{ title: '周次', dataIndex: 'weekNo', renderText: value => `第 ${value} 周` }, { title: '开始日期', dataIndex: 'startDate', renderText: displayDate }, { title: '结束日期', dataIndex: 'endDate', renderText: displayDate }, { title: '备注', dataIndex: 'note' }, { title: '操作', valueType: 'option', render: (_, record) => <Button type="link" onClick={() => openEdit('week', record)}>编辑</Button> }]} /> },
    ]} /></Card>}
    <Modal title={`${editing ? '编辑' : '新增'}${modal === 'year' ? '学年' : modal === 'semester' ? '学期' : '周次'}`} open={modal !== null} afterOpenChange={open => { if (open && !editing && modal) initializeCreateForm(modal) }} onCancel={() => { setModal(null); setEditing(null) }} onOk={() => form.submit()} confirmLoading={save.isPending} destroyOnHidden>
      <Form form={form} layout="vertical" onFinish={values => save.mutate(values)} onValuesChange={onFormValuesChange} preserve={false}>
        {modal === 'year' && <Form.Item name="startYear" label="学年" rules={[{ required: true, message: '请选择学年' }]} extra="结束年份由系统自动按起始年份 +1 生成。"><Select showSearch disabled={Boolean(editing)} options={yearChoices} placeholder="请选择学年…" /></Form.Item>}
        {modal === 'semester' && <Form.Item name="academicYearId" label="所属学年" rules={[{ required: true, message: '请选择所属学年' }]}><Select disabled={Boolean(editing)} options={data.academicYears.map(item => ({ value: item.id, label: item.name }))} /></Form.Item>}
        {modal === 'semester' && <Form.Item noStyle shouldUpdate={(previous, current) => previous.academicYearId !== current.academicYearId}>{({ getFieldValue }) => <Form.Item name="semesterCode" label="学期" rules={[{ required: true, message: '请选择上学期或下学期' }]}><Select disabled={Boolean(editing)} options={semesterTypeOptions.map(option => ({ ...option, disabled: !editing && data.semesters.some(item => item.academicYearId === getFieldValue('academicYearId') && item.code === option.value) }))} /></Form.Item>}</Form.Item>}
        {modal === 'week' && <Form.Item name="semesterId" label="所属学期" rules={[{ required: true }]}><Select options={data.semesters.map(item => ({ value: item.id, label: item.name }))} /></Form.Item>}
        {modal === 'week' && <Form.Item name="weekNo" label="周次" rules={[{ required: true }]}><InputNumber min={1} max={60} style={{ width: '100%' }} /></Form.Item>}
        {(modal === 'year' || modal === 'semester') && <Form.Item noStyle shouldUpdate>{({ getFieldValue }) => {
          const label = modal === 'year' ? buildAcademicYearDefaults(Number(getFieldValue('startYear') || currentAcademicYearStart())).name : (() => { const year = data.academicYears.find(item => item.id === getFieldValue('academicYearId')); const code = getFieldValue('semesterCode') as SemesterCode | undefined; return year && code ? buildSemesterDefaults(Number(displayDate(year.startDate).slice(0, 4)), code).name : '请选择学年和学期' })()
          return <div className="academic-rule-preview"><span>系统名称</span><strong>{label}</strong><small>日期已按默认校历生成，可根据学校实际安排微调。</small></div>
        }}</Form.Item>}
        <Form.Item name="startDate" label="开始日期" rules={[{ required: true }]}><Input type="date" /></Form.Item>
        <Form.Item name="endDate" label="结束日期" rules={[{ required: true }]}><Input type="date" /></Form.Item>
        {modal === 'week' && <Form.Item name="note" label="备注"><Input /></Form.Item>}
      </Form>
    </Modal>
  </section>
}

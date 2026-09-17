import { HistoryOutlined, PlusOutlined } from '@ant-design/icons'
import { ProTable, type ProColumns } from '@ant-design/pro-components'
import { useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import { Alert, Button, Card, Col, Descriptions, Drawer, Empty, Form, Input, InputNumber, Menu, Modal, Popconfirm, Row, Select, Space, Spin, Tag, Typography, message } from 'antd'
import { useState } from 'react'
import { flushSync } from 'react-dom'
import type { CourseSchedule, MasterDataChange, MasterDataKind, MasterDataRecord } from '../../entities/masterdata/model/masterData'
import { createMasterData, getAcademicCalendar, getDictionaries, getMasterData, getMasterDataChanges, updateMasterData, type MasterDataListParams } from '../../shared/api/adminApi'

type KindConfig = { label: string; singular: string; identifierLabel?: string }
const kindConfigs: Record<MasterDataKind, KindConfig> = {
  students: { label: '学生', singular: '学生', identifierLabel: '学号' },
  classes: { label: '班级', singular: '班级' },
  staff: { label: '教职工/班主任', singular: '教职工', identifierLabel: '工号' },
  dormitories: { label: '宿舍', singular: '宿舍' },
  'dormitory-areas': { label: '宿舍区', singular: '宿舍区' },
  'dormitory-buildings': { label: '宿舍楼', singular: '宿舍楼' },
  'teaching-buildings': { label: '教学楼', singular: '教学楼' },
  classrooms: { label: '教室', singular: '教室' },
  courses: { label: '课程', singular: '课程' },
  faculties: { label: '院系', singular: '院系' },
  grades: { label: '年级', singular: '年级' },
  majors: { label: '专业', singular: '专业' },
}
const kindGroups: { label: string; kinds: MasterDataKind[] }[] = [
  { label: '基础信息', kinds: ['grades', 'faculties', 'majors', 'classes', 'staff', 'students'] },
  { label: '宿舍相关', kinds: ['dormitories', 'dormitory-areas', 'dormitory-buildings'] },
  { label: '教学相关', kinds: ['teaching-buildings', 'classrooms', 'courses'] },
]
const dictionaryCodes = ['GENDER', 'ENROLLMENT_STATUS'] as const
const weekdayOptions = ['周一', '周二', '周三', '周四', '周五', '周六', '周日'].map((label, index) => ({ value: index + 1, label }))
const referenceKindsByKind: Record<MasterDataKind, MasterDataKind[]> = {
  students: ['grades', 'faculties', 'majors', 'classes', 'dormitory-areas', 'dormitory-buildings', 'dormitories'],
  classes: ['grades', 'faculties', 'majors', 'staff'],
  staff: ['faculties'],
  majors: ['faculties'],
  courses: ['staff', 'classes', 'classrooms'],
  classrooms: ['teaching-buildings'],
  dormitories: ['dormitory-areas', 'dormitory-buildings'],
  'dormitory-buildings': ['dormitory-areas'],
  faculties: [], grades: [], 'dormitory-areas': [], 'teaching-buildings': [],
}

async function getAllEnabledMasterData(referenceKind: MasterDataKind) {
  const items: MasterDataRecord[] = []
  for (let page = 1; ; page += 1) {
    const result = await getMasterData(referenceKind, { enabled: true, page, pageSize: 100 })
    items.push(...(result.items ?? []))
    if (items.length >= result.total || !(result.items ?? []).length) return { ...result, items }
  }
}

export function MasterDataPage() {
  const [kind, setKind] = useState<MasterDataKind>('students')
  const [filters, setFilters] = useState<MasterDataListParams>({})
  const [page, setPage] = useState(1)
  const [editing, setEditing] = useState<MasterDataRecord | null>(null)
  const [modalOpen, setModalOpen] = useState(false)
  const [historyRecord, setHistoryRecord] = useState<MasterDataRecord | null>(null)
  const [form] = Form.useForm<Partial<MasterDataRecord>>()
  const selectedDormitoryAreaID = Form.useWatch('dormitoryAreaId', form)
  const selectedGradeID = Form.useWatch('gradeId', form)
  const client = useQueryClient()
  const records = useQuery({ queryKey: ['master-data', kind, filters, page], queryFn: () => getMasterData(kind, { ...filters, page, pageSize: 20 }) })
  const referenceKinds = referenceKindsByKind[kind]
  const references = useQueries({ queries: referenceKinds.map(referenceKind => ({
    queryKey: ['master-data-reference', referenceKind],
    queryFn: () => getAllEnabledMasterData(referenceKind),
  })) })
  const referenceItems = (referenceKind: MasterDataKind) => {
    const result = references[referenceKinds.indexOf(referenceKind)]
    return result?.data?.items ?? []
  }
  const referenceName = (referenceKind: MasterDataKind, id?: string) => referenceItems(referenceKind).find(item => item.id === id)?.name ?? ''
  const dormitoryBuildingName = (record: MasterDataRecord) => [referenceName('dormitory-areas', record.dormitoryAreaId), record.name].filter(Boolean).join(' ') || record.name
  const dormitoryName = (record: MasterDataRecord) => [
    referenceName('dormitory-areas', record.dormitoryAreaId),
    referenceName('dormitory-buildings', record.dormitoryBuildingId),
    record.roomNo || record.name,
  ].filter(Boolean).join(' ') || record.name
  const options = (referenceKind: MasterDataKind) => referenceItems(referenceKind).map(item => ({
      value: item.id,
      label: referenceKind === 'dormitory-buildings' ? dormitoryBuildingName(item) : referenceKind === 'dormitories' ? dormitoryName(item) : item.name,
    }))
  const dormitoryBuildingOptions = options('dormitory-buildings').filter(option => {
    if (!selectedDormitoryAreaID) return true
    const result = references[referenceKinds.indexOf('dormitory-buildings')]
    return result?.data?.items.find(item => item.id === option.value)?.dormitoryAreaId === selectedDormitoryAreaID
  })
  const studentClassOptions = options('classes').filter(option => {
    if (!selectedGradeID) return true
    const result = references[referenceKinds.indexOf('classes')]
    return result?.data?.items.find(item => item.id === option.value)?.gradeId === selectedGradeID
  })
  const needsDictionaries = kind === 'students' || kind === 'staff'
  const academicCalendar = useQuery({ queryKey: ['academic-calendar'], queryFn: getAcademicCalendar, enabled: kind === 'courses' })
  const dictionaryTypes = useQuery({ queryKey: ['dictionary-types'], queryFn: () => getDictionaries(), enabled: needsDictionaries })
  const dictionaryTypeIds = dictionaryCodes.map(code => (dictionaryTypes.data?.types ?? []).find(type => type.code === code && type.enabled)?.id)
  const dictionaries = useQueries({ queries: dictionaryCodes.map((code, index) => ({
    queryKey: ['dictionary-items', code, dictionaryTypeIds[index]],
    queryFn: () => dictionaryTypeIds[index] ? getDictionaries(dictionaryTypeIds[index]) : Promise.resolve({ types: [], items: [] }),
    enabled: needsDictionaries && Boolean(dictionaryTypeIds[index]),
  })) })
  const dictionaryOptions = (code: typeof dictionaryCodes[number]) => {
    const result = dictionaries[dictionaryCodes.indexOf(code)]
    return (result.data?.items ?? []).filter(item => item.enabled).map(item => ({ value: item.value, label: item.label }))
  }
  const history = useQuery({ queryKey: ['master-data-history', kind, historyRecord?.id], queryFn: () => getMasterDataChanges(kind, historyRecord!.id), enabled: Boolean(historyRecord) })
  const refresh = async () => {
    await client.invalidateQueries({ queryKey: ['master-data', kind] })
    await client.invalidateQueries({ queryKey: ['master-data-reference', kind] })
  }
  const save = useMutation({
    mutationFn: async (values: Partial<MasterDataRecord>) => {
      const normalized = normalizeRecord(kind, values)
      return editing ? updateMasterData({ ...editing, ...normalized, kind }) : createMasterData(kind, normalized as Omit<MasterDataRecord, 'id' | 'kind' | 'enabled'>)
    },
    onSuccess: async () => { await refresh(); setModalOpen(false); setEditing(null); form.resetFields(); message.success('基础数据已保存') },
    onError: error => message.error(error.message),
  })
  const toggle = useMutation({ mutationFn: (record: MasterDataRecord) => updateMasterData({ ...record, enabled: !record.enabled }), onSuccess: refresh, onError: error => message.error(error.message) })
  const initializeEditorForm = (record: MasterDataRecord | null = editing) => {
    form.resetFields()
    if (record) {
      form.setFieldsValue(kind === 'grades' ? { ...record, name: record.name.replace(/级$/, '') } : record)
      return
    }
    form.setFieldsValue({ gender: 'UNKNOWN', enrollmentStatus: 'ENROLLED', enrolledSize: 0, schedules: kind === 'courses' ? [{} as CourseSchedule] : undefined })
  }
  const openCreate = () => {
    flushSync(() => {
      setEditing(null)
      setModalOpen(true)
    })
    initializeEditorForm(null)
  }
  const openEdit = (record: MasterDataRecord) => {
    flushSync(() => {
      setEditing(record)
      setModalOpen(true)
    })
    initializeEditorForm(record)
  }
  const config = kindConfigs[kind]
  const columns: ProColumns<MasterDataRecord>[] = [
    ...(config.identifierLabel ? [{ title: config.identifierLabel, dataIndex: 'code', width: 150, ellipsis: true }] : []),
    { title: kind === 'grades' ? '年级' : kind === 'dormitories' ? '宿舍位置' : kind === 'dormitory-buildings' ? '宿舍楼' : kind === 'classrooms' ? '教室位置' : '名称', dataIndex: 'name', width: 180, ellipsis: true, renderText: (_: unknown, record: MasterDataRecord) => kind === 'dormitories' ? dormitoryName(record) : kind === 'dormitory-buildings' ? dormitoryBuildingName(record) : record.name },
    ...(kind === 'students' || kind === 'classes' ? [{ title: '年级', dataIndex: 'gradeId', width: 100, valueType: 'select' as const, fieldProps: { showSearch: true, optionFilterProp: 'label', options: options('grades') }, renderText: (_: unknown, record: MasterDataRecord) => record.grade ?? '—' }] : []),
    ...(kind === 'classes' ? [{ title: '在籍人数', dataIndex: 'enrolledSize', width: 100 }] : []),
    ...(kind === 'dormitory-buildings' ? [{ title: '所属宿舍区', dataIndex: 'dormitoryAreaId', hideInTable: true, valueType: 'select' as const, fieldProps: { options: options('dormitory-areas'), showSearch: true, optionFilterProp: 'label' } }] : []),
    ...(kind === 'majors' || kind === 'staff' || kind === 'classes' || kind === 'students' ? [{ title: '所属院系', dataIndex: 'facultyId', width: 180, valueType: 'select' as const, fieldProps: { showSearch: true, optionFilterProp: 'label', options: options('faculties') }, renderText: (value: string) => referenceName('faculties', value) || '—' }] : []),
    ...(kind === 'classes' || kind === 'students' ? [{ title: '专业', dataIndex: 'majorId', width: 150, valueType: 'select' as const, fieldProps: { showSearch: true, optionFilterProp: 'label', options: options('majors') }, renderText: (value: string) => referenceName('majors', value) || '—' }] : []),
    ...(kind === 'classes' ? [{ title: '班主任', dataIndex: 'headTeacherId', width: 130, valueType: 'select' as const, fieldProps: { showSearch: true, optionFilterProp: 'label', options: options('staff') }, renderText: (value: string) => referenceName('staff', value) || '—' }] : []),
    ...(kind === 'students' ? [{ title: '班级', dataIndex: 'classId', width: 160, valueType: 'select' as const, fieldProps: { showSearch: true, optionFilterProp: 'label', options: options('classes') }, renderText: (value: string) => referenceName('classes', value) || '—' }, { title: '宿舍', dataIndex: 'dormitoryId', width: 180, valueType: 'select' as const, fieldProps: { showSearch: true, optionFilterProp: 'label', options: options('dormitories') }, renderText: (value: string) => {
      const dormitory = referenceItems('dormitories').find(item => item.id === value)
      return dormitory ? dormitoryName(dormitory) : '—'
    } }, { title: '性别', dataIndex: 'gender', hideInTable: true, valueType: 'select' as const, fieldProps: { options: dictionaryOptions('GENDER') } }, { title: '学籍状态', dataIndex: 'enrollmentStatus', hideInTable: true, valueType: 'select' as const, fieldProps: { options: dictionaryOptions('ENROLLMENT_STATUS') } }] : []),
    ...(kind === 'staff' ? [{ title: '性别', dataIndex: 'gender', hideInTable: true, valueType: 'select' as const, fieldProps: { options: dictionaryOptions('GENDER') } }] : []),
    ...(kind === 'courses' ? [{ title: '任课教师', dataIndex: 'teacherId', width: 130, valueType: 'select' as const, fieldProps: { options: options('staff') }, renderText: (value: string) => referenceName('staff', value) || '—' }, { title: '授课班级', dataIndex: 'classId', valueType: 'select' as const, fieldProps: { options: options('classes') }, renderText: (_: unknown, record: MasterDataRecord) => (record.classIds ?? []).map(classID => referenceName('classes', classID)).filter(Boolean).join('、') || '—' }, { title: '所属学期', dataIndex: 'semesterId', hideInTable: true, valueType: 'select' as const, fieldProps: { options: (academicCalendar.data?.semesters ?? []).map(semester => ({ value: semester.id, label: semester.name })) } }, { title: '上课教室', dataIndex: 'classroomId', hideInTable: true, valueType: 'select' as const, fieldProps: { options: options('classrooms') } }, { title: '排课安排', search: false, renderText: (_: unknown, record: MasterDataRecord) => (record.schedules ?? []).map(schedule => {
      const semester = academicCalendar.data?.semesters.find(value => value.id === schedule.semesterId)?.name ?? ''
      const weekday = weekdayOptions.find(option => option.value === schedule.weekday)?.label ?? ''
      const classroom = options('classrooms').find(option => option.value === schedule.classroomId)?.label ?? ''
      const minutes = (schedule.endSection - schedule.startSection + 1) * 45
      return `${semester} ${weekday} 第${schedule.startSection}–${schedule.endSection}节（${minutes}分钟） ${classroom}`.trim()
    }).join('；') || '—' }] : []),
    ...(kind === 'dormitories' ? [{ title: '宿舍区', dataIndex: 'dormitoryAreaId', hideInTable: true, valueType: 'select' as const, fieldProps: { options: options('dormitory-areas'), showSearch: true, optionFilterProp: 'label' } }] : []),
    ...(kind === 'dormitories' ? [{ title: '宿舍楼', dataIndex: 'dormitoryBuildingId', hideInTable: true, valueType: 'select' as const, fieldProps: { options: options('dormitory-buildings'), showSearch: true, optionFilterProp: 'label' } }] : []),
    ...(kind === 'classrooms' ? [{ title: '教学楼', dataIndex: 'teachingBuildingId', hideInTable: true, valueType: 'select' as const, fieldProps: { options: options('teaching-buildings'), showSearch: true, optionFilterProp: 'label' } }] : []),
    { title: '状态', dataIndex: 'enabled', width: 90, valueType: 'select', fieldProps: { options: [{ value: true, label: '启用' }, { value: false, label: '停用' }] }, render: (_, record) => <Tag color={record.enabled ? 'green' : 'default'}>{record.enabled ? '启用' : '停用'}</Tag> },
    { title: '操作', valueType: 'option', width: 250, render: (_, record) => <Space size={4}><Button type="link" onClick={() => openEdit(record)}>编辑</Button><Button type="link" icon={<HistoryOutlined aria-hidden />} onClick={() => setHistoryRecord(record)}>修改记录</Button><Popconfirm title={`确认${record.enabled ? '停用' : '启用'}该${config.singular}？`} onConfirm={() => toggle.mutate(record)}><Button type="link" danger={record.enabled}>{record.enabled ? '停用' : '启用'}</Button></Popconfirm></Space> },
  ]

  return <section className="admin-page">
    <div className="page-heading"><div><Typography.Title level={2}>基础数据</Typography.Title><Typography.Paragraph>维护检查、统计与通报依赖的学院基础资料；系统自动维护唯一标识，管理员只需填写业务信息。</Typography.Paragraph></div></div>
	{records.isError && <Alert type="error" showIcon title="基础数据加载失败" description="请检查网络连接后重试。" />}
	{needsDictionaries && (dictionaryTypes.isError || dictionaries.some(query => query.isError)) && <Alert type="error" showIcon title="系统字典加载失败" description="性别和学籍状态暂时无法选择，请刷新页面后重试。" />}
	{kind === 'courses' && academicCalendar.isError && <Alert type="error" showIcon title="学期数据加载失败" description="排课暂时无法编辑，请刷新页面后重试。" />}
    <Card className="master-data-workspace" styles={{ body: { padding: 0 } }}>
      <aside className="master-data-navigation" aria-label="基础数据分类">
        <Menu mode="inline" selectedKeys={[kind]} onClick={({ key }) => { setKind(key as MasterDataKind); setPage(1); setFilters({}) }} items={kindGroups.map(group => ({ type: 'group', label: group.label, children: group.kinds.map(value => ({ key: value, label: kindConfigs[value].label })) }))} />
      </aside>
      <div className="master-data-table-panel">
        <ProTable<MasterDataRecord, MasterDataListParams> key={kind} rowKey="id" loading={records.isFetching} dataSource={records.data?.items ?? []} columns={columns} search={{ labelWidth: 'auto', defaultCollapsed: false }} scroll={{ x: 'max-content' }}
          onSubmit={values => { setFilters(values); setPage(1) }} onReset={() => { setFilters({}); setPage(1) }}
          toolbar={{ title: config.label, actions: [<Button key="create" type="primary" icon={<PlusOutlined aria-hidden />} onClick={openCreate}>新增{config.singular}</Button>] }}
          options={{ reload: () => void records.refetch(), density: true, setting: true }} pagination={{ current: page, pageSize: 20, total: records.data?.total, onChange: setPage, showSizeChanger: false }} locale={{ emptyText: <Empty description={`暂无${config.label}数据`} /> }} />
      </div>
    </Card>
    <Modal title={`${editing ? '编辑' : '新增'}${config.singular}`} open={modalOpen} afterOpenChange={open => { if (open) initializeEditorForm() }} onCancel={() => setModalOpen(false)} onOk={() => form.submit()} confirmLoading={save.isPending} destroyOnHidden width={680}>
      <Form name={`master-data-${kind}-editor`} form={form} layout="vertical" onFinish={values => save.mutate(values)} preserve={false}>
        {config.identifierLabel && <Form.Item name="code" label={config.identifierLabel} rules={[{ required: true, message: `请输入${config.identifierLabel}` }]}><Input disabled={Boolean(editing)} autoComplete="off" /></Form.Item>}
        {kind === 'grades' && <Form.Item name="name" label="年级" rules={[{ required: true, message: '请输入年级' }, { pattern: /^\d{4}$/, message: '请输入4位年份，例如2025' }]}><Input disabled={Boolean(editing)} autoComplete="off" placeholder="例如：2025" maxLength={4} inputMode="numeric" /></Form.Item>}
        {!['grades', 'dormitories', 'classrooms'].includes(kind) && <Form.Item name="name" label="名称" rules={[{ required: true, message: '请输入名称' }]}><Input autoComplete="off" /></Form.Item>}
        {kind === 'majors' && <Form.Item name="facultyId" label="所属院系" rules={[{ required: true, message: '请选择所属院系' }]}><Select showSearch optionFilterProp="label" options={options('faculties')} /></Form.Item>}
        {kind === 'dormitory-buildings' && <Form.Item name="dormitoryAreaId" label="所属宿舍区" rules={[{ required: true, message: '请选择所属宿舍区' }]}><Select showSearch optionFilterProp="label" options={options('dormitory-areas')} /></Form.Item>}
        {(kind === 'staff' || kind === 'classes' || kind === 'students') && <Form.Item name="facultyId" label="所属院系"><Select allowClear showSearch optionFilterProp="label" options={options('faculties')} /></Form.Item>}
        {(kind === 'classes' || kind === 'students') && <Form.Item name="majorId" label="专业"><Select allowClear showSearch optionFilterProp="label" options={options('majors')} /></Form.Item>}
        {(kind === 'classes' || kind === 'students') && <Form.Item name="gradeId" label="年级" rules={[{ required: true, message: '请选择年级' }]}><Select showSearch optionFilterProp="label" options={options('grades')} onChange={() => kind === 'students' && form.setFieldValue('classId', undefined)} /></Form.Item>}
        {kind === 'classes' && <><Form.Item name="headTeacherId" label="班主任"><Select allowClear showSearch optionFilterProp="label" options={options('staff')} /></Form.Item><Form.Item name="enrolledSize" label="实际在籍人数" initialValue={0} rules={[{ required: true, message: '请输入实际在籍人数' }]}><InputNumber min={0} precision={0} style={{ width: '100%' }} /></Form.Item></>}
        {(kind === 'staff' || kind === 'students') && <Form.Item name="gender" label="性别" initialValue="UNKNOWN"><Select loading={dictionaryTypes.isLoading || dictionaries[0].isLoading} options={dictionaryOptions('GENDER')} /></Form.Item>}
        {kind === 'staff' && <Form.Item name="phone" label="联系电话"><Input autoComplete="tel" /></Form.Item>}
        {kind === 'students' && <><Form.Item name="classId" label="班级"><Select allowClear showSearch optionFilterProp="label" options={studentClassOptions} /></Form.Item><Form.Item name="dormitoryId" label="宿舍"><Select allowClear showSearch optionFilterProp="label" options={options('dormitories')} /></Form.Item><Form.Item name="enrollmentStatus" label="学籍状态" initialValue="ENROLLED"><Select loading={dictionaryTypes.isLoading || dictionaries[1].isLoading} options={dictionaryOptions('ENROLLMENT_STATUS')} /></Form.Item></>}
        {kind === 'dormitories' && <><Form.Item name="dormitoryAreaId" label="宿舍区" rules={[{ required: true, message: '请选择宿舍区' }]}><Select showSearch optionFilterProp="label" options={options('dormitory-areas')} onChange={() => form.setFieldValue('dormitoryBuildingId', undefined)} /></Form.Item><Form.Item name="dormitoryBuildingId" label="宿舍楼" rules={[{ required: true, message: '请选择宿舍楼' }]}><Select showSearch optionFilterProp="label" options={dormitoryBuildingOptions} disabled={!selectedDormitoryAreaID} /></Form.Item><Form.Item name="roomNo" label="宿舍号" extra="例如：401、402" rules={[{ required: true }]}><Input autoComplete="off" placeholder="例如：401" /></Form.Item></>}
        {kind === 'classrooms' && <><Form.Item name="teachingBuildingId" label="教学楼" rules={[{ required: true, message: '请选择教学楼' }]}><Select showSearch optionFilterProp="label" options={options('teaching-buildings')} /></Form.Item><Form.Item name="roomNo" label="教室号" rules={[{ required: true, message: '请输入教室号' }]}><Input autoComplete="off" /></Form.Item></>}
        {kind === 'courses' && <>
          <Form.Item name="teacherId" label="任课教师"><Select allowClear showSearch optionFilterProp="label" options={options('staff')} /></Form.Item>
          <Form.Item name="classIds" label="授课班级" rules={[{ required: true, message: '请至少选择一个授课班级' }]}><Select mode="multiple" showSearch optionFilterProp="label" options={options('classes')} /></Form.Item>
          <Form.List name="schedules" rules={[{ validator: async (_, schedules?: CourseSchedule[]) => {
            if (!schedules?.length) throw new Error('请至少添加一条排课安排')
          } }]}>
            {(fields, { add, remove }, { errors }) => <div className="course-schedule-list">
              <div className="course-schedule-heading"><Typography.Text strong>排课安排</Typography.Text><Space><span>每天最多 10 节</span><span>45 分钟/节</span><Button type="dashed" onClick={() => add({})}>添加排课</Button></Space></div>
              {fields.map((field, index) => <Card key={field.key} size="small" title={`排课 ${index + 1}`} extra={fields.length > 1 ? <Button type="link" danger onClick={() => remove(field.name)}>删除</Button> : null}>
                <Row gutter={12}>
                  <Col span={12}><Form.Item name={[field.name, 'semesterId']} label="所属学期" rules={[{ required: true, message: '请选择所属学期' }]}><Select showSearch optionFilterProp="label" loading={academicCalendar.isLoading} options={(academicCalendar.data?.semesters ?? []).filter(semester => semester.enabled).map(semester => ({ value: semester.id, label: semester.name }))} /></Form.Item></Col>
                  <Col span={12}><Form.Item name={[field.name, 'classroomId']} label="上课教室" rules={[{ required: true, message: '请选择上课教室' }]}><Select showSearch optionFilterProp="label" options={options('classrooms')} /></Form.Item></Col>
                  <Col span={8}><Form.Item name={[field.name, 'weekday']} label="星期" rules={[{ required: true, message: '请选择星期' }]}><Select options={weekdayOptions} /></Form.Item></Col>
                  <Col span={8}><Form.Item name={[field.name, 'startSection']} label="开始节次" rules={[{ required: true, message: '请选择开始节次' }]}><InputNumber min={1} max={10} precision={0} style={{ width: '100%' }} /></Form.Item></Col>
                  <Col span={8}><Form.Item name={[field.name, 'endSection']} label="结束节次" dependencies={[[field.name, 'startSection']]} rules={[{ required: true, message: '请选择结束节次' }, ({ getFieldValue }) => ({ validator: async (_, value?: number) => {
                    const start = getFieldValue(['schedules', field.name, 'startSection']) as number | undefined
                    if (value !== undefined && start !== undefined && value < start) throw new Error('结束节次不能早于开始节次')
                  } })]}><InputNumber min={1} max={10} precision={0} style={{ width: '100%' }} /></Form.Item></Col>
                </Row>
              </Card>)}
              <Form.ErrorList errors={errors} />
            </div>}
          </Form.List>
        </>}
      </Form>
    </Modal>
    <Drawer title={`${historyRecord?.name ?? ''} · 修改记录`} open={Boolean(historyRecord)} onClose={() => setHistoryRecord(null)} size="large">
	  {history.isLoading ? <Spin /> : history.isError ? <Alert type="error" showIcon title="修改记录加载失败" /> : history.data?.length ? history.data.map((change: MasterDataChange) => <Descriptions key={change.id} bordered size="small" column={1} className="history-entry" items={[{ key: 'action', label: '操作', children: change.action }, { key: 'actor', label: '操作人', children: change.actorId || '系统' }, { key: 'time', label: '时间', children: new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(change.createdAt)) }]} />) : <Empty description="暂无修改记录" />}
    </Drawer>
  </section>
}

function normalizeRecord(kind: MasterDataKind, values: Partial<MasterDataRecord>) {
  if (kind === 'grades') {
    const year = values.name?.trim() ?? ''
    const normalized = { ...values, name: year }
    delete normalized.code
    return normalized
  }
  return values
}

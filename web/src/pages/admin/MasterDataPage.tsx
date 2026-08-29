import { HistoryOutlined, PlusOutlined } from '@ant-design/icons'
import { ProTable, type ProColumns } from '@ant-design/pro-components'
import { useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import { Alert, Button, Descriptions, Drawer, Empty, Form, Input, InputNumber, Modal, Popconfirm, Select, Space, Spin, Tabs, Tag, Typography, message } from 'antd'
import { useState } from 'react'
import type { MasterDataChange, MasterDataKind, MasterDataRecord } from '../../entities/masterdata/model/masterData'
import { createMasterData, getDictionaries, getMasterData, getMasterDataChanges, updateMasterData } from '../../shared/api/adminApi'

type KindConfig = { label: string; singular: string; codeLabel: string }
const kindConfigs: Record<MasterDataKind, KindConfig> = {
  students: { label: '学生', singular: '学生', codeLabel: '学号' },
  classes: { label: '班级', singular: '班级', codeLabel: '班级编码' },
  staff: { label: '教职工/班主任', singular: '教职工', codeLabel: '工号' },
  dormitories: { label: '宿舍', singular: '宿舍', codeLabel: '宿舍编码' },
  classrooms: { label: '教室', singular: '教室', codeLabel: '教室编号' },
  courses: { label: '课程', singular: '课程', codeLabel: '课程编码' },
  faculties: { label: '院系', singular: '院系', codeLabel: '院系编码' },
  majors: { label: '专业', singular: '专业', codeLabel: '专业编码' },
}
const kinds = Object.keys(kindConfigs) as MasterDataKind[]
const dictionaryCodes = ['GENDER', 'ENROLLMENT_STATUS'] as const

export function MasterDataPage() {
  const [kind, setKind] = useState<MasterDataKind>('students')
  const [keyword, setKeyword] = useState('')
  const [page, setPage] = useState(1)
  const [editing, setEditing] = useState<MasterDataRecord | null>(null)
  const [modalOpen, setModalOpen] = useState(false)
  const [historyRecord, setHistoryRecord] = useState<MasterDataRecord | null>(null)
  const [form] = Form.useForm<Partial<MasterDataRecord>>()
  const client = useQueryClient()
  const records = useQuery({ queryKey: ['master-data', kind, keyword, page], queryFn: () => getMasterData(kind, { keyword, page, pageSize: 20 }) })
  const referenceKinds: MasterDataKind[] = ['faculties', 'majors', 'staff', 'classes', 'dormitories', 'classrooms']
  const references = useQueries({ queries: referenceKinds.map(referenceKind => ({ queryKey: ['master-data-reference', referenceKind], queryFn: () => getMasterData(referenceKind, { pageSize: 100 }) })) })
  const options = (referenceKind: MasterDataKind) => references[referenceKinds.indexOf(referenceKind)].data?.items.map(item => ({ value: item.id, label: `${item.code} · ${item.name}` })) ?? []
  const dictionaryTypes = useQuery({ queryKey: ['dictionary-types'], queryFn: () => getDictionaries() })
  const dictionaryTypeIds = dictionaryCodes.map(code => dictionaryTypes.data?.types.find(type => type.code === code && type.enabled)?.id)
  const dictionaries = useQueries({ queries: dictionaryCodes.map((code, index) => ({
    queryKey: ['dictionary-items', code, dictionaryTypeIds[index]],
    queryFn: () => dictionaryTypeIds[index] ? getDictionaries(dictionaryTypeIds[index]) : Promise.resolve({ types: [], items: [] }),
    enabled: Boolean(dictionaryTypeIds[index]),
  })) })
  const dictionaryOptions = (code: typeof dictionaryCodes[number]) => {
    const result = dictionaries[dictionaryCodes.indexOf(code)]
    return result.data?.items.filter(item => item.enabled).map(item => ({ value: item.value, label: item.label })) ?? []
  }
  const history = useQuery({ queryKey: ['master-data-history', kind, historyRecord?.id], queryFn: () => getMasterDataChanges(kind, historyRecord!.id), enabled: Boolean(historyRecord) })
  const refresh = async () => {
    await client.invalidateQueries({ queryKey: ['master-data'] })
    await client.invalidateQueries({ queryKey: ['master-data-reference'] })
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
  const openCreate = () => { setEditing(null); form.resetFields(); setModalOpen(true) }
  const openEdit = (record: MasterDataRecord) => { setEditing(record); form.setFieldsValue(record); setModalOpen(true) }
  const config = kindConfigs[kind]
  const columns: ProColumns<MasterDataRecord>[] = [
    { title: config.codeLabel, dataIndex: 'code', width: 150, ellipsis: true },
    { title: '名称', dataIndex: 'name', width: 180, ellipsis: true },
    ...(kind === 'students' || kind === 'classes' ? [{ title: '年级', dataIndex: 'grade', width: 100 }] : []),
    ...(kind === 'classes' ? [{ title: '在籍人数', dataIndex: 'enrolledSize', width: 100 }] : []),
    ...(kind === 'dormitories' ? [{ title: '宿舍位置', renderText: (_: unknown, record: MasterDataRecord) => `${record.campusArea ?? ''} ${record.buildingNo ?? ''}-${record.roomNo ?? ''}` }] : []),
    ...(kind === 'classrooms' ? [{ title: '教室位置', renderText: (_: unknown, record: MasterDataRecord) => `${record.building ?? ''} ${record.roomNo ?? ''}` }] : []),
    { title: '状态', dataIndex: 'enabled', width: 90, render: (_, record) => <Tag color={record.enabled ? 'green' : 'default'}>{record.enabled ? '启用' : '停用'}</Tag> },
    { title: '操作', valueType: 'option', width: 250, render: (_, record) => <Space size={4}><Button type="link" onClick={() => openEdit(record)}>编辑</Button><Button type="link" icon={<HistoryOutlined aria-hidden />} onClick={() => setHistoryRecord(record)}>修改记录</Button><Popconfirm title={`确认${record.enabled ? '停用' : '启用'}该${config.singular}？`} onConfirm={() => toggle.mutate(record)}><Button type="link" danger={record.enabled}>{record.enabled ? '停用' : '启用'}</Button></Popconfirm></Space> },
  ]

  return <section className="admin-page">
    <div className="page-heading"><div><Typography.Title level={2}>基础数据</Typography.Title><Typography.Paragraph>维护检查、统计与通报依赖的学院基础资料；唯一编码创建后不可修改。</Typography.Paragraph></div></div>
	{records.isError && <Alert type="error" showIcon title="基础数据加载失败" description="请检查网络连接后重试。" />}
	{(dictionaryTypes.isError || dictionaries.some(query => query.isError)) && <Alert type="error" showIcon title="系统字典加载失败" description="性别和学籍状态暂时无法选择，请刷新页面后重试。" />}
    <Tabs activeKey={kind} onChange={value => { setKind(value as MasterDataKind); setPage(1); setKeyword('') }} items={kinds.map(value => ({ key: value, label: kindConfigs[value].label }))} />
    <ProTable<MasterDataRecord> rowKey="id" loading={records.isLoading} dataSource={records.data?.items} columns={columns} search={false}
      toolbar={{ search: { placeholder: `搜索${config.codeLabel}或名称…`, onSearch: value => { setKeyword(value); setPage(1) } }, actions: [<Button key="create" type="primary" icon={<PlusOutlined aria-hidden />} onClick={openCreate}>新增{config.singular}</Button>] }}
      options={{ reload: () => void records.refetch(), density: true, setting: true }} pagination={{ current: page, pageSize: 20, total: records.data?.total, onChange: setPage, showSizeChanger: false }} locale={{ emptyText: <Empty description={`暂无${config.label}数据`} /> }} />
    <Modal title={`${editing ? '编辑' : '新增'}${config.singular}`} open={modalOpen} onCancel={() => setModalOpen(false)} onOk={() => form.submit()} confirmLoading={save.isPending} destroyOnHidden width={680}>
      <Form form={form} layout="vertical" onFinish={values => save.mutate(values)} preserve={false}>
        <Form.Item name="code" label={config.codeLabel} rules={[{ required: true, message: `请输入${config.codeLabel}` }]}><Input disabled={Boolean(editing)} autoComplete="off" /></Form.Item>
        {!['dormitories', 'classrooms'].includes(kind) && <Form.Item name="name" label="名称" rules={[{ required: true, message: '请输入名称' }]}><Input autoComplete="off" /></Form.Item>}
        {kind === 'majors' && <Form.Item name="facultyId" label="所属院系" rules={[{ required: true, message: '请选择所属院系' }]}><Select showSearch optionFilterProp="label" options={options('faculties')} /></Form.Item>}
        {(kind === 'staff' || kind === 'classes' || kind === 'students') && <Form.Item name="facultyId" label="所属院系"><Select allowClear showSearch optionFilterProp="label" options={options('faculties')} /></Form.Item>}
        {(kind === 'classes' || kind === 'students') && <Form.Item name="majorId" label="专业"><Select allowClear showSearch optionFilterProp="label" options={options('majors')} /></Form.Item>}
        {(kind === 'classes' || kind === 'students') && <Form.Item name="grade" label="年级" rules={[{ required: true, message: '请输入年级' }]}><Input autoComplete="off" placeholder="例如：2025…" /></Form.Item>}
        {kind === 'classes' && <><Form.Item name="headTeacherId" label="班主任"><Select allowClear showSearch optionFilterProp="label" options={options('staff')} /></Form.Item><Form.Item name="enrolledSize" label="实际在籍人数" initialValue={0} rules={[{ required: true, message: '请输入实际在籍人数' }]}><InputNumber min={0} precision={0} style={{ width: '100%' }} /></Form.Item></>}
        {(kind === 'staff' || kind === 'students') && <Form.Item name="gender" label="性别" initialValue="UNKNOWN"><Select loading={dictionaryTypes.isLoading || dictionaries[0].isLoading} options={dictionaryOptions('GENDER')} /></Form.Item>}
        {kind === 'staff' && <Form.Item name="phone" label="联系电话"><Input autoComplete="tel" /></Form.Item>}
        {kind === 'students' && <><Form.Item name="classId" label="班级"><Select allowClear showSearch optionFilterProp="label" options={options('classes')} /></Form.Item><Form.Item name="dormitoryId" label="宿舍"><Select allowClear showSearch optionFilterProp="label" options={options('dormitories')} /></Form.Item><Form.Item name="enrollmentStatus" label="学籍状态" initialValue="ENROLLED"><Select loading={dictionaryTypes.isLoading || dictionaries[1].isLoading} options={dictionaryOptions('ENROLLMENT_STATUS')} /></Form.Item></>}
        {kind === 'dormitories' && <><Form.Item name="campusArea" label="宿舍区" rules={[{ required: true }]}><Input autoComplete="off" /></Form.Item><Form.Item name="buildingNo" label="宿舍楼" rules={[{ required: true }]}><Input autoComplete="off" /></Form.Item><Form.Item name="roomNo" label="宿舍号" rules={[{ required: true }]}><Input autoComplete="off" /></Form.Item></>}
        {kind === 'classrooms' && <><Form.Item name="building" label="教学楼" rules={[{ required: true }]}><Input autoComplete="off" /></Form.Item><Form.Item name="roomNo" label="教室号" rules={[{ required: true }]}><Input autoComplete="off" /></Form.Item><Form.Item name="floor" label="楼层" rules={[{ required: true }]}><InputNumber min={0} precision={0} style={{ width: '100%' }} /></Form.Item></>}
        {kind === 'courses' && <><Form.Item name="teacherId" label="任课教师"><Select allowClear showSearch optionFilterProp="label" options={options('staff')} /></Form.Item><Form.Item name="classId" label="上课班级"><Select allowClear showSearch optionFilterProp="label" options={options('classes')} /></Form.Item><Form.Item name="classroomId" label="上课地点"><Select allowClear showSearch optionFilterProp="label" options={options('classrooms')} /></Form.Item><Form.Item name="scheduleText" label="上课时间"><Input autoComplete="off" placeholder="例如：周一 1–2 节…" /></Form.Item></>}
      </Form>
    </Modal>
    <Drawer title={`${historyRecord?.name ?? ''} · 修改记录`} open={Boolean(historyRecord)} onClose={() => setHistoryRecord(null)} size="large">
	  {history.isLoading ? <Spin /> : history.isError ? <Alert type="error" showIcon title="修改记录加载失败" /> : history.data?.length ? history.data.map((change: MasterDataChange) => <Descriptions key={change.id} bordered size="small" column={1} className="history-entry" items={[{ key: 'action', label: '操作', children: change.action }, { key: 'actor', label: '操作人', children: change.actorId || '系统' }, { key: 'time', label: '时间', children: new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(change.createdAt)) }]} />) : <Empty description="暂无修改记录" />}
    </Drawer>
  </section>
}

function normalizeRecord(kind: MasterDataKind, values: Partial<MasterDataRecord>) {
  if (kind === 'dormitories' || kind === 'classrooms') return { ...values, name: values.code }
  return values
}

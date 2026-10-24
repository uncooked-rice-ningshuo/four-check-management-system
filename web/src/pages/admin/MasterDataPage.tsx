import { DownloadOutlined, FileExcelOutlined, HistoryOutlined, PlusOutlined, UploadOutlined } from '@ant-design/icons'
import { ProTable, type ProColumns } from '@ant-design/pro-components'
import { useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import { Alert, Button, Card, Descriptions, Drawer, Empty, Form, Input, InputNumber, Menu, Modal, Popconfirm, Select, Space, Spin, Table, Tag, Typography, message } from 'antd'
import { useRef, useState } from 'react'
import { flushSync } from 'react-dom'
import type { MasterDataChange, MasterDataImportBatch, MasterDataImportPreviewRow, MasterDataKind, MasterDataRecord } from '../../entities/masterdata/model/masterData'
import { confirmMasterDataImportBatch, createMasterData, downloadMasterDataImportTemplate, getDictionaries, getMasterData, getMasterDataChanges, getMasterDataImportSpec, previewMasterDataImport, updateMasterData, type MasterDataListParams } from '../../shared/api/adminApi'
import { createIdempotencyKey } from '../../shared/utils/idempotencyKey'

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
const referenceKindsByKind: Record<MasterDataKind, MasterDataKind[]> = {
  students: ['grades', 'faculties', 'majors', 'classes', 'dormitory-areas', 'dormitory-buildings', 'dormitories'],
  classes: ['grades', 'faculties', 'majors', 'staff'],
  staff: ['faculties'],
  majors: ['faculties'],
  courses: [],
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
  const [importOpen, setImportOpen] = useState(false)
  const [importFile, setImportFile] = useState<File | null>(null)
  const [importBatch, setImportBatch] = useState<MasterDataImportBatch | null>(null)
  const [confirmIdempotencyKey, setConfirmIdempotencyKey] = useState('')
  const importFileInput = useRef<HTMLInputElement>(null)
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
  const importSpec = useQuery({ queryKey: ['master-data-import-spec', kind], queryFn: () => getMasterDataImportSpec(kind), enabled: importOpen && kind !== 'courses' })
  const refresh = async () => {
    await Promise.all([
      client.invalidateQueries({ queryKey: ['master-data', kind] }),
      client.invalidateQueries({ queryKey: ['master-data-reference', kind] }),
      ...(kind === 'courses' ? [
        client.invalidateQueries({ queryKey: ['inspection-course-options'] }),
        client.invalidateQueries({ queryKey: ['course-inspections'] }),
        client.invalidateQueries({ queryKey: ['attendance'] }),
      ] : []),
    ])
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
  const templateDownload = useMutation({
    mutationFn: () => downloadMasterDataImportTemplate(kind),
    onSuccess: ({ blob }) => {
      const url = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = `${kindConfigs[kind].label}导入模板.xlsx`
      document.body.append(link)
      link.click()
      link.remove()
      window.setTimeout(() => URL.revokeObjectURL(url), 0)
    },
    onError: error => message.error(error.message),
  })
  const uploadImport = useMutation({
    mutationFn: (file: File) => previewMasterDataImport(kind, file),
    onSuccess: batch => setImportBatch(batch),
    onError: error => message.error(error.message),
  })
  const confirmImport = useMutation({
    mutationFn: (idempotencyKey: string) => {
      if (!importBatch) throw new Error('导入预览不存在，请重新上传文件')
      return confirmMasterDataImportBatch(kind, importBatch.id, importBatch.version, idempotencyKey)
    },
    onSuccess: async result => {
      await Promise.all([
        client.invalidateQueries({ queryKey: ['master-data'] }),
        client.invalidateQueries({ queryKey: ['master-data-reference'] }),
      ])
      setImportOpen(false)
      setImportBatch(null)
      setImportFile(null)
      setConfirmIdempotencyKey('')
      message.success(`成功新增 ${result.importedRows} 条${kindConfigs[kind].singular}数据`)
    },
    onError: error => message.error(error.message),
  })
  // Initialize once after mounting; an animation callback would erase early input.
  const initializeEditorForm = (record: MasterDataRecord | null) => {
    form.resetFields()
    if (record) {
      form.setFieldsValue(kind === 'grades' ? { ...record, name: record.name.replace(/级$/, '') } : record)
      return
    }
    form.setFieldsValue({ gender: 'UNKNOWN', enrollmentStatus: 'ENROLLED', enrolledSize: 0 })
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
  const openImport = () => {
    setImportFile(null)
    setImportBatch(null)
    setConfirmIdempotencyKey('')
    uploadImport.reset()
    confirmImport.reset()
    setImportOpen(true)
  }
  const closeImport = () => {
    if (uploadImport.isPending || confirmImport.isPending) return
    setImportOpen(false)
    setImportFile(null)
    setImportBatch(null)
    setConfirmIdempotencyKey('')
    uploadImport.reset()
    confirmImport.reset()
  }
  const startImportConfirmation = () => {
    const key = confirmIdempotencyKey || createIdempotencyKey()
    setConfirmIdempotencyKey(key)
    confirmImport.mutate(key)
  }
  const config = kindConfigs[kind]
  const columns: ProColumns<MasterDataRecord>[] = [
    ...(config.identifierLabel ? [{ title: config.identifierLabel, dataIndex: 'code', width: 150, ellipsis: true }] : []),
    { title: kind === 'courses' ? '课程名' : kind === 'grades' ? '年级' : kind === 'dormitories' ? '宿舍位置' : kind === 'dormitory-buildings' ? '宿舍楼' : kind === 'classrooms' ? '教室位置' : '名称', dataIndex: 'name', width: 180, ellipsis: true, renderText: (_: unknown, record: MasterDataRecord) => kind === 'dormitories' ? dormitoryName(record) : kind === 'dormitory-buildings' ? dormitoryBuildingName(record) : record.name },
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
    <Card className="master-data-workspace" styles={{ body: { padding: 0 } }}>
      <aside className="master-data-navigation" aria-label="基础数据分类">
        <Menu mode="inline" selectedKeys={[kind]} onClick={({ key }) => { setKind(key as MasterDataKind); setPage(1); setFilters({}) }} items={kindGroups.map(group => ({ type: 'group', label: group.label, children: group.kinds.map(value => ({ key: value, label: kindConfigs[value].label })) }))} />
      </aside>
      <div className="master-data-table-panel">
        <ProTable<MasterDataRecord, MasterDataListParams> key={kind} rowKey="id" loading={records.isFetching} dataSource={records.data?.items ?? []} columns={columns} search={{ labelWidth: 'auto', defaultCollapsed: false }} scroll={{ x: 'max-content' }}
          onSubmit={values => { setFilters(values); setPage(1) }} onReset={() => { setFilters({}); setPage(1) }}
          toolbar={{ title: config.label, actions: [
            ...(kind === 'courses' ? [] : [<Button key="import" icon={<FileExcelOutlined aria-hidden />} onClick={openImport}>导入Excel</Button>]),
            <Button key="create" type="primary" icon={<PlusOutlined aria-hidden />} onClick={openCreate}>新增{config.singular}</Button>,
          ] }}
          options={{ reload: () => void records.refetch(), density: true, setting: true }} pagination={{ current: page, pageSize: 20, total: records.data?.total, onChange: setPage, showSizeChanger: false }} locale={{ emptyText: <Empty description={`暂无${config.label}数据`} /> }} />
      </div>
    </Card>
    <Drawer
      title={`导入${config.singular}`}
      open={importOpen}
      onClose={closeImport}
      size="min(1100px, calc(100vw - 16px))"
      closable={!uploadImport.isPending && !confirmImport.isPending}
      maskClosable={!uploadImport.isPending && !confirmImport.isPending}
      keyboard={!uploadImport.isPending && !confirmImport.isPending}
      destroyOnHidden
      footer={importBatch ? <Space>
        <Button onClick={closeImport} disabled={confirmImport.isPending}>关闭</Button>
        {importBatch.invalidRows > 0
          ? <Button onClick={() => { setImportBatch(null); uploadImport.reset() }}>重新选择文件</Button>
          : <Popconfirm title={`确认新增 ${importBatch.validRows} 条${config.singular}数据？`} description="确认后将一次性新增整批数据；系统不会更新已有记录。" okText="确认导入" cancelText="再检查一下" onConfirm={startImportConfirmation}>
            <Button type="primary" loading={confirmImport.isPending}>确认新增</Button>
          </Popconfirm>}
      </Space> : <Space>
        <Button onClick={closeImport} disabled={uploadImport.isPending}>取消</Button>
        <Button type="primary" icon={<UploadOutlined aria-hidden />} loading={uploadImport.isPending} disabled={!importFile} onClick={() => importFile && uploadImport.mutate(importFile)}>上传并预览</Button>
      </Space>}
    >
      {!importBatch ? <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        <Alert type="info" showIcon title="本次只新增数据，不更新已有记录。请先下载当前分类模板；预览中任一行有错误时，整批都不能确认。" />
        {importSpec.isError && <Alert type="error" showIcon title="导入字段说明加载失败" description="可重试加载字段说明；上传时服务端仍会按模板严格校验。" action={<Button size="small" onClick={() => void importSpec.refetch()}>重试</Button>} />}
        {importSpec.data && <div className="master-data-import-field-list" aria-label="导入字段格式要求">
          <Typography.Text strong>字段格式要求</Typography.Text>
          {importSpec.data.fields.map(field => <Typography.Paragraph key={field.code} className="master-data-import-field-rule">
            <Typography.Text strong>{field.header}{field.required ? '（必填）' : '（选填）'}</Typography.Text>：{field.format}
          </Typography.Paragraph>)}
        </div>}
        <Space wrap>
          <Button icon={<DownloadOutlined aria-hidden />} loading={templateDownload.isPending} disabled={importSpec.isLoading} onClick={() => templateDownload.mutate()}>下载当前分类模板</Button>
          <Button icon={<FileExcelOutlined aria-hidden />} disabled={uploadImport.isPending} onClick={() => importFileInput.current?.click()}>选择 XLSX 文件</Button>
          <Typography.Text type="secondary">{importFile?.name ?? '未选择文件'}</Typography.Text>
          <input ref={importFileInput} type="file" accept=".xlsx" aria-label="选择基础数据导入文件" hidden onChange={event => {
            setImportFile(event.currentTarget.files?.[0] ?? null)
            event.currentTarget.value = ''
          }} />
        </Space>
        {uploadImport.isError && <Alert type="error" showIcon title="文件校验失败" description={uploadImport.error.message} />}
      </Space> : <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        <Alert
          type={importBatch.invalidRows > 0 ? 'error' : 'success'}
          showIcon
          title={importBatch.invalidRows > 0 ? '预览发现错误，整批不能导入' : '预览通过，请核对后确认新增'}
          description={`共 ${importBatch.totalRows} 行：${importBatch.validRows} 行通过，${importBatch.invalidRows} 行有错误。${importBatch.invalidRows > 0 ? '请修正文件并重新上传；系统不会导入其中的有效行。' : '确认后所有记录会在同一事务中新增。'}`}
        />
        <Table<MasterDataImportPreviewRow>
          rowKey="rowNo"
          size="small"
          dataSource={importBatch.rows}
          pagination={{ pageSize: 10, showSizeChanger: false }}
          scroll={{ x: 'max-content', y: 420 }}
          columns={[
            { title: 'Excel 行号', dataIndex: 'rowNo', width: 90, fixed: 'left' },
            { title: '校验结果', dataIndex: 'status', width: 100, render: (status: string) => <Tag color={status === 'VALID' ? 'green' : 'red'}>{status === 'VALID' ? '通过' : '有错误'}</Tag> },
            ...importBatch.fields.map(field => ({
              title: `${field.header}${field.required ? ' *' : ''}`,
              key: field.code,
              width: 160,
              render: (_: unknown, row: MasterDataImportPreviewRow) => row.values[field.code] || '—',
            })),
            { title: '错误说明', key: 'errors', width: 260, render: (_: unknown, row: MasterDataImportPreviewRow) => row.errors.length
              ? <Space direction="vertical" size={0}>{row.errors.map((issue, index) => <Typography.Text type="danger" key={`${issue.field}-${index}`}>{importBatch.fields.find(field => field.code === issue.field)?.header ?? issue.field}：{issue.message}</Typography.Text>)}</Space>
              : '—' },
          ]}
        />
        {confirmImport.isError && <Alert type="error" showIcon title="确认导入失败" description={confirmImport.error.message} />}
      </Space>}
    </Drawer>
    <Modal title={`${editing ? '编辑' : '新增'}${config.singular}`} open={modalOpen} onCancel={() => setModalOpen(false)} onOk={() => form.submit()} confirmLoading={save.isPending} destroyOnHidden width={680}>
      <Form name={`master-data-${kind}-editor`} form={form} layout="vertical" onFinish={values => save.mutate(values)} preserve={false}>
        {config.identifierLabel && <Form.Item name="code" label={config.identifierLabel} rules={[{ required: true, message: `请输入${config.identifierLabel}` }]}><Input disabled={Boolean(editing)} autoComplete="off" /></Form.Item>}
        {kind === 'grades' && <Form.Item name="name" label="年级" rules={[{ required: true, message: '请输入年级' }, { pattern: /^\d{4}$/, message: '请输入4位年份，例如2025' }]}><Input disabled={Boolean(editing)} autoComplete="off" placeholder="例如：2025" maxLength={4} inputMode="numeric" /></Form.Item>}
        {!['grades', 'dormitories', 'classrooms'].includes(kind) && <Form.Item name="name" label={kind === 'courses' ? '课程名' : '名称'} rules={[{ required: true, message: kind === 'courses' ? '请输入课程名' : '请输入名称' }]}><Input autoComplete="off" maxLength={kind === 'courses' ? 128 : undefined} /></Form.Item>}
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

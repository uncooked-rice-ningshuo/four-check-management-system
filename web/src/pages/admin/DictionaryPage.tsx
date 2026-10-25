import { PlusOutlined } from '@ant-design/icons'
import { ProTable, type ProColumns } from '@ant-design/pro-components'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { App, Alert, Button, Card, Empty, Form, Input, InputNumber, Modal, Switch, Tag, Typography } from 'antd'
import { useState } from 'react'
import { flushSync } from 'react-dom'
import type { DictionaryItem, DictionaryType } from '../../entities/dictionary/model/dictionary'
import { createDictionaryItem, createDictionaryType, getDictionaries, updateDictionaryItem, updateDictionaryType } from '../../shared/api/adminApi'

const newTypeValues: Partial<DictionaryType> = { code: '', name: '', description: '', enabled: true }
const newItemValues: Partial<DictionaryItem> = { value: '', label: '', description: '', sortOrder: 0, enabled: true }

export function DictionaryPage() {
	const { message } = App.useApp()
	const [modal, modalContextHolder] = Modal.useModal()
  const [selectedTypeIdOverride, setSelectedTypeIdOverride] = useState('')
  const [typeModal, setTypeModal] = useState(false)
  const [itemModal, setItemModal] = useState(false)
  const [editingType, setEditingType] = useState<DictionaryType | null>(null)
  const [editingItem, setEditingItem] = useState<DictionaryItem | null>(null)
  const [typeForm] = Form.useForm<DictionaryType>()
  const [itemForm] = Form.useForm<DictionaryItem>()
  const client = useQueryClient()
  const typesQuery = useQuery({ queryKey: ['dictionaries'], queryFn: () => getDictionaries() })
  const dictionaryTypes = typesQuery.data?.types ?? []
  const selectedTypeId = dictionaryTypes.some(type => type.id === selectedTypeIdOverride)
    ? selectedTypeIdOverride
    : dictionaryTypes[0]?.id ?? ''
  const itemsQuery = useQuery({ queryKey: ['dictionary-items', selectedTypeId], queryFn: () => getDictionaries(selectedTypeId), enabled: Boolean(selectedTypeId) })
  const refresh = async () => { await client.invalidateQueries({ queryKey: ['dictionaries'] }); await client.invalidateQueries({ queryKey: ['dictionary-items'] }) }
  const saveType = useMutation({ mutationFn: (values: DictionaryType) => editingType ? updateDictionaryType({ ...editingType, ...values }) : createDictionaryType(values), onSuccess: async () => { await refresh(); setTypeModal(false); typeForm.resetFields(); message.success('字典类型已保存') }, onError: error => message.error(error.message) })
  const saveItem = useMutation({ mutationFn: (values: DictionaryItem) => editingItem ? updateDictionaryItem({ ...editingItem, ...values }) : createDictionaryItem(selectedTypeId, values), onSuccess: async () => { await refresh(); setItemModal(false); itemForm.resetFields(); message.success('字典项已保存') }, onError: error => message.error(error.message) })
	const toggleType = useMutation({ mutationFn: updateDictionaryType, onSuccess: refresh, onError: error => message.error(error.message) })
	const toggleItem = useMutation({ mutationFn: updateDictionaryItem, onSuccess: refresh, onError: error => message.error(error.message) })
  const openType = (record?: DictionaryType) => {
    flushSync(() => { setEditingType(record ?? null); setTypeModal(true) })
    typeForm.setFieldsValue(record ?? newTypeValues)
  }
  const openItem = (record?: DictionaryItem) => {
    flushSync(() => { setEditingItem(record ?? null); setItemModal(true) })
    itemForm.setFieldsValue(record ?? newItemValues)
  }
  const confirmTypeStatus = (record: DictionaryType, enabled: boolean) => {
	if (enabled) { toggleType.mutate({ ...record, enabled }); return }
	modal.confirm({ title: `停用“${record.name}”字典类型？`, content: '停用后，该类型及其选项不能继续用于新增业务数据。', okText: '确认停用', okButtonProps: { danger: true }, cancelText: '取消', onOk: () => toggleType.mutateAsync({ ...record, enabled }) })
  }
  const confirmItemStatus = (record: DictionaryItem, enabled: boolean) => {
	if (enabled) { toggleItem.mutate({ ...record, enabled }); return }
	modal.confirm({ title: `停用“${record.label}”字典值？`, content: '停用后不能再用于新数据；现有记录仍保留原值。', okText: '确认停用', okButtonProps: { danger: true }, cancelText: '取消', onOk: () => toggleItem.mutateAsync({ ...record, enabled }) })
  }
  const typeColumns: ProColumns<DictionaryType>[] = [
    { title: '类型编码', dataIndex: 'code', width: 150 }, { title: '名称', dataIndex: 'name' }, { title: '属性', render: (_, record) => record.system ? <Tag color="blue">系统字典</Tag> : <Tag>自定义</Tag> },
    { title: '状态', render: (_, record) => <Switch aria-label={`${record.enabled ? '停用' : '启用'}字典类型 ${record.name}`} checked={record.enabled} disabled={record.system} onChange={enabled => confirmTypeStatus(record, enabled)} /> },
    { title: '操作', valueType: 'option', render: (_, record) => <Button type="link" onClick={() => openType(record)}>编辑</Button> },
  ]
  const itemColumns: ProColumns<DictionaryItem>[] = [
    { title: '选项值', dataIndex: 'value', width: 160 }, { title: '显示名称', dataIndex: 'label' }, { title: '排序', dataIndex: 'sortOrder', width: 90 }, { title: '说明', dataIndex: 'description', ellipsis: true },
    { title: '状态', render: (_, record) => <Switch aria-label={`${record.enabled ? '停用' : '启用'}字典值 ${record.label}`} checked={record.enabled} onChange={enabled => confirmItemStatus(record, enabled)} /> },
    { title: '操作', valueType: 'option', render: (_, record) => <Button type="link" onClick={() => openItem(record)}>编辑</Button> },
  ]
	return <section className="admin-page">{modalContextHolder}
    <div className="page-heading"><div><Typography.Title level={2}>字典管理</Typography.Title><Typography.Paragraph>维护仅用于展示和选择的可配置选项；部门、角色和检查类型继续使用独立业务配置。</Typography.Paragraph></div><Button type="primary" icon={<PlusOutlined aria-hidden />} onClick={() => openType()}>新增字典类型</Button></div>
	{typesQuery.isError && <Alert type="error" showIcon title="字典类型加载失败" description="请检查网络连接后重试。" />}
	{itemsQuery.isError && <Alert type="error" showIcon title="字典项加载失败" description="请重新选择字典类型后重试。" />}
    <div className="dictionary-grid">
	  <Card title="字典类型"><ProTable<DictionaryType> rowKey="id" search={false} options={false} pagination={false} scroll={{ x: 'max-content' }} loading={typesQuery.isLoading} dataSource={dictionaryTypes} columns={typeColumns} rowClassName={record => record.id === selectedTypeId ? 'selected-table-row' : ''} rowSelection={{ type: 'radio', selectedRowKeys: selectedTypeId ? [selectedTypeId] : [], onChange: selectedKeys => { if (selectedKeys[0]) setSelectedTypeIdOverride(String(selectedKeys[0])) } }} tableAlertRender={false} locale={{ emptyText: <Empty description="暂无字典类型" /> }} /></Card>
      <Card title={selectedTypeId ? `字典选项 · ${dictionaryTypes.find(item => item.id === selectedTypeId)?.name ?? ''}` : '字典选项'} extra={<Button icon={<PlusOutlined aria-hidden />} disabled={!selectedTypeId} onClick={() => openItem()}>新增选项</Button>}>
        {selectedTypeId ? <ProTable<DictionaryItem> rowKey="id" search={false} options={{ density: true }} pagination={false} loading={itemsQuery.isLoading} dataSource={itemsQuery.data?.items} columns={itemColumns} locale={{ emptyText: <Empty description="暂无字典项" /> }} /> : <Empty description="请先选择一个字典类型" />}
      </Card>
    </div>
    <Modal title={`${editingType ? '编辑' : '新增'}字典类型`} open={typeModal} onCancel={() => setTypeModal(false)} onOk={() => typeForm.submit()} confirmLoading={saveType.isPending} destroyOnHidden>
      <Form form={typeForm} layout="vertical" onFinish={values => saveType.mutate(values)} preserve={false}><Form.Item name="code" label="类型编码" rules={[{ required: true, message: '请输入类型编码' }]}><Input disabled={Boolean(editingType?.system)} autoComplete="off" placeholder="例如：GENDER…" /></Form.Item><Form.Item name="name" label="类型名称" rules={[{ required: true, message: '请输入类型名称' }]}><Input disabled={Boolean(editingType?.system)} autoComplete="off" /></Form.Item><Form.Item name="description" label="说明"><Input.TextArea rows={3} /></Form.Item></Form>
    </Modal>
    <Modal title={`${editingItem ? '编辑' : '新增'}字典项`} open={itemModal} onCancel={() => setItemModal(false)} onOk={() => itemForm.submit()} confirmLoading={saveItem.isPending} destroyOnHidden>
      <Form form={itemForm} layout="vertical" onFinish={values => saveItem.mutate(values)} preserve={false}><Form.Item name="value" label="选项值" rules={[{ required: true, message: '请输入选项值' }]}><Input autoComplete="off" placeholder="例如：MALE…" /></Form.Item><Form.Item name="label" label="显示名称" rules={[{ required: true, message: '请输入显示名称' }]}><Input autoComplete="off" /></Form.Item><Form.Item name="sortOrder" label="排序"><InputNumber precision={0} style={{ width: '100%' }} /></Form.Item><Form.Item name="description" label="说明"><Input.TextArea rows={3} /></Form.Item><Form.Item name="enabled" label="状态" valuePropName="checked"><Switch checkedChildren="启用" unCheckedChildren="停用" /></Form.Item></Form>
    </Modal>
  </section>
}

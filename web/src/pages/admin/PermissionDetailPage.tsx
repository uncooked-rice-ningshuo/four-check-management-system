import { PlusOutlined } from '@ant-design/icons'
import { ProTable, type ProColumns } from '@ant-design/pro-components'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Button, Drawer, Empty, Form, Input, Modal, Popconfirm, Select, Space, Tag, Typography, message } from 'antd'
import { useState } from 'react'
import type { EffectivePermission, PermissionRule } from '../../entities/permission/model/permission'
import { createPermissionRule, deletePermissionRule, getAcademicCalendar, getCatalogs, getEffectivePermissions, getMasterData, getPermissionRules, updatePermissionRule } from '../../shared/api/adminApi'

const actionOptions = [
  ['USER_MANAGE', '用户管理'], ['PERMISSION_MANAGE', '权限管理'], ['MASTER_DATA_MANAGE', '基础数据管理'], ['DICTIONARY_MANAGE', '字典管理'],
  ['INSPECTION_ENTER', '检查录入'], ['INSPECTION_VIEW', '检查查看'], ['INSPECTION_REVIEW', '检查审核'], ['INSPECTION_EXPORT', '检查导出'], ['INSPECTION_SUMMARIZE', '检查汇总'],
  ['REPORT_REVIEW', '通报审核'], ['REPORT_RETURN', '通报退回'], ['REPORT_FINALIZE', '通报定稿'], ['REPORT_PUBLISH', '通报发布'], ['ARCHIVE_VIEW', '历史归档查看'],
].map(([value, label]) => ({ value, label }))
const actionName = (value: string) => actionOptions.find(item => item.value === value)?.label ?? value

export function PermissionDetailDrawer({ accountId, userName, onClose }: { accountId: string; userName: string; onClose: () => void }) {
  const [editing, setEditing] = useState<PermissionRule | null>(null)
  const [modalOpen, setModalOpen] = useState(false)
  const [form] = Form.useForm<PermissionRule>()
  const client = useQueryClient()
  const effective = useQuery({ queryKey: ['effective-permissions', accountId], queryFn: () => getEffectivePermissions(accountId), enabled: Boolean(accountId) })
  const rules = useQuery({ queryKey: ['permission-rules', accountId], queryFn: () => getPermissionRules(accountId), enabled: Boolean(accountId) })
  const catalogs = useQuery({ queryKey: ['admin-catalogs'], queryFn: getCatalogs, enabled: Boolean(accountId) })
  const calendar = useQuery({ queryKey: ['academic-calendar'], queryFn: getAcademicCalendar, enabled: Boolean(accountId) })
  const classes = useQuery({ queryKey: ['master-data-reference', 'classes'], queryFn: () => getMasterData('classes', { pageSize: 100 }), enabled: Boolean(accountId) })
  const refresh = async () => { await client.invalidateQueries({ queryKey: ['effective-permissions', accountId] }); await client.invalidateQueries({ queryKey: ['permission-rules', accountId] }) }
  const save = useMutation({ mutationFn: (values: PermissionRule) => editing ? updatePermissionRule({ ...editing, ...values, scope: values.scope ?? {} }) : createPermissionRule(accountId, { action: values.action, effect: values.effect, note: values.note ?? '', scope: values.scope ?? {} }), onSuccess: async () => { await refresh(); setModalOpen(false); form.resetFields(); message.success('权限规则已保存') }, onError: error => message.error(error.message) })
  const remove = useMutation({ mutationFn: (ruleId: string) => deletePermissionRule(accountId, ruleId), onSuccess: refresh, onError: error => message.error(error.message) })
  const openRule = (rule?: PermissionRule) => { setEditing(rule ?? null); form.setFieldsValue(rule ?? { effect: 'ALLOW', scope: {}, note: '' } as PermissionRule); setModalOpen(true) }
  const permissionColumns: ProColumns<EffectivePermission>[] = [
    { title: '权限动作', dataIndex: 'action', renderText: actionName },
    { title: '结果', dataIndex: 'effect', width: 90, render: (_, record) => <Tag color={record.effect === 'DENY' ? 'red' : 'green'}>{record.effect === 'DENY' ? '拒绝' : '允许'}</Tag> },
    { title: '生效范围', dataIndex: 'scope', render: (_, record) => <Typography.Text>{scopeSummary(record.scope)}</Typography.Text> },
    { title: '权限来源', dataIndex: 'sources', render: (_, record) => <Space wrap>{record.sources.map(source => <Tag key={`${source.type}-${source.label}`} color="blue">{source.label}</Tag>)}</Space> },
  ]
  const ruleColumns: ProColumns<PermissionRule>[] = [
    { title: '动作', dataIndex: 'action', renderText: actionName }, { title: '效果', dataIndex: 'effect', render: (_, record) => <Tag color={record.effect === 'DENY' ? 'red' : 'green'}>{record.effect === 'DENY' ? '拒绝' : '允许'}</Tag> },
    { title: '范围', render: (_, record) => scopeSummary(record.scope) }, { title: '备注', dataIndex: 'note', ellipsis: true },
    { title: '操作', valueType: 'option', render: (_, record) => <Space><Button type="link" onClick={() => openRule(record)}>编辑</Button><Popconfirm title="确认删除这条权限规则？" onConfirm={() => remove.mutate(record.id)}><Button type="link" danger>删除</Button></Popconfirm></Space> },
  ]
  return <Drawer title={`${userName} · 生效权限`} open={Boolean(accountId)} onClose={onClose} size="large" destroyOnHidden>
    <Typography.Paragraph type="secondary">最终权限按系统角色、组织角色、部门业务映射和用户单独授权/限制合并；匹配范围内的拒绝规则优先。</Typography.Paragraph>
    <ProTable<EffectivePermission> rowKey={(record, index) => `${record.action}-${record.effect}-${index}`} headerTitle="当前生效权限" loading={effective.isLoading} search={false} options={false} pagination={{ pageSize: 8 }} dataSource={effective.data?.permissions} columns={permissionColumns} locale={{ emptyText: <Empty description="当前没有生效权限" /> }} />
    <ProTable<PermissionRule> rowKey="id" headerTitle="用户补充规则" loading={rules.isLoading} search={false} options={false} pagination={false} dataSource={rules.data} columns={ruleColumns} toolBarRender={() => [<Button key="add" type="primary" icon={<PlusOutlined aria-hidden />} onClick={() => openRule()}>新增权限规则</Button>]} locale={{ emptyText: <Empty description="暂无用户单独规则" /> }} />
    <Modal title={`${editing ? '编辑' : '新增'}权限规则`} open={modalOpen} onCancel={() => setModalOpen(false)} onOk={() => form.submit()} confirmLoading={save.isPending} width={720} destroyOnHidden>
      <Form form={form} layout="vertical" onFinish={values => save.mutate(values)} preserve={false}>
        <Form.Item name="action" label="权限动作" rules={[{ required: true, message: '请选择权限动作' }]}><Select showSearch optionFilterProp="label" options={actionOptions} /></Form.Item>
        <Form.Item name="effect" label="规则效果" rules={[{ required: true }]}><Select options={[{ value: 'ALLOW', label: '允许（补充授权）' }, { value: 'DENY', label: '拒绝（单独限制）' }]} /></Form.Item>
        <Form.Item name={['scope', 'academicYearId']} label="学年范围"><Select allowClear options={calendar.data?.academicYears.map(item => ({ value: item.id, label: item.name }))} /></Form.Item>
        <Form.Item name={['scope', 'semesterId']} label="学期范围"><Select allowClear options={calendar.data?.semesters.map(item => ({ value: item.id, label: item.name }))} /></Form.Item>
        <Form.Item name={['scope', 'weekId']} label="周次范围"><Select allowClear options={calendar.data?.weeks.map(item => ({ value: item.id, label: `第 ${item.weekNo} 周` }))} /></Form.Item>
        <Form.Item name={['scope', 'inspectionTypeId']} label="检查类型"><Select allowClear options={catalogs.data?.inspectionTypes.map(item => ({ value: item.id, label: item.name }))} /></Form.Item>
        <Form.Item name={['scope', 'departmentId']} label="部门范围"><Select allowClear options={catalogs.data?.departments.map(item => ({ value: item.id, label: item.name }))} /></Form.Item>
        <Form.Item name={['scope', 'classId']} label="班级范围"><Select allowClear showSearch optionFilterProp="label" options={classes.data?.items.map(item => ({ value: item.id, label: item.name }))} /></Form.Item>
        <Form.Item name={['scope', 'grade']} label="年级范围"><Input autoComplete="off" placeholder="留空表示全部年级…" /></Form.Item>
        <Form.Item name="note" label="授权说明" rules={[{ required: true, message: '请说明设置原因' }]}><Input.TextArea rows={3} maxLength={255} showCount /></Form.Item>
      </Form>
    </Modal>
  </Drawer>
}

function scopeSummary(scope: PermissionRule['scope']) {
  const values = Object.values(scope ?? {}).filter(Boolean)
  return values.length ? `已限定 ${values.length} 个范围维度` : '全部范围'
}

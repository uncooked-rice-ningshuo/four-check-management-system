import { PlusOutlined } from '@ant-design/icons'
import { ProTable, type ProColumns } from '@ant-design/pro-components'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { App, Button, Drawer, Empty, Form, Input, Modal, Popconfirm, Select, Space, Tag, Typography } from 'antd'
import { useState } from 'react'
import type { EffectivePermission, PermissionRule } from '../../entities/permission/model/permission'
import { createPermissionRule, deletePermissionRule, getAcademicCalendar, getCatalogs, getEffectivePermissions, getPermissionRules, updatePermissionRule } from '../../shared/api/adminApi'

const actionOptions = [
  ['USER_MANAGE', '用户管理'], ['PERMISSION_MANAGE', '权限管理'], ['MASTER_DATA_MANAGE', '基础数据管理'], ['DICTIONARY_MANAGE', '字典管理'],
  ['CONFIGURATION_VIEW', '系统配置查看'],
  ['INSPECTION_ENTER', '检查录入'], ['INSPECTION_MANAGE', '管理检查记录'], ['INSPECTION_VIEW', '检查查看'], ['INSPECTION_REVIEW', '检查审核'], ['INSPECTION_EXPORT', '检查导出'], ['INSPECTION_SUMMARIZE', '检查汇总'],
  ['REPORT_VIEW', '查看通报汇总'], ['REPORT_OBLIGATION_MANAGE', '管理逐周应交范围'], ['REPORT_FORCE_GENERATE', '强制生成未完成通报'], ['REPORT_REVIEW', '通报审核'], ['REPORT_RETURN', '通报退回'], ['REPORT_FINALIZE', '通报定稿'], ['REPORT_PUBLISH', '通报发布'], ['ARCHIVE_VIEW', '历史归档查看'],
].map(([value, label]) => ({ value, label }))
const actionName = (value: string) => actionOptions.find(item => item.value === value)?.label ?? value

export function PermissionDetailDrawer({ accountId, userName, onClose }: { accountId: string; userName: string; onClose: () => void }) {
  const { message } = App.useApp()
  const [editing, setEditing] = useState<PermissionRule | null>(null)
  const [modalOpen, setModalOpen] = useState(false)
  const [form] = Form.useForm<PermissionRule>()
  const client = useQueryClient()
  const effective = useQuery({ queryKey: ['effective-permissions', accountId], queryFn: () => getEffectivePermissions(accountId), enabled: Boolean(accountId) })
  const rules = useQuery({ queryKey: ['permission-rules', accountId], queryFn: () => getPermissionRules(accountId), enabled: Boolean(accountId) })
  const catalogs = useQuery({ queryKey: ['admin-catalogs'], queryFn: getCatalogs, enabled: Boolean(accountId) })
  const calendar = useQuery({ queryKey: ['academic-calendar'], queryFn: getAcademicCalendar, enabled: Boolean(accountId) })
  const refresh = async () => { await client.invalidateQueries({ queryKey: ['effective-permissions', accountId] }); await client.invalidateQueries({ queryKey: ['permission-rules', accountId] }) }
  const save = useMutation({ mutationFn: (values: PermissionRule) => {
    const scope = permissionRuleScope(values.scope)
    return editing
      ? updatePermissionRule({ ...editing, ...values, scope })
      : createPermissionRule(accountId, { action: values.action, effect: values.effect, note: values.note ?? '', scope })
  }, onSuccess: async () => { await refresh(); setModalOpen(false); form.resetFields(); message.success('权限规则已保存') }, onError: error => message.error(error.message) })
  const remove = useMutation({ mutationFn: (ruleId: string) => deletePermissionRule(accountId, ruleId), onSuccess: refresh, onError: error => message.error(error.message) })
  const openRule = (rule?: PermissionRule) => { form.resetFields(); setEditing(rule ?? null); setModalOpen(true) }
  const scopeText = (scope: PermissionRule['scope']) => scopeSummary(scope, {
    academicYears: calendar.data?.academicYears ?? [], semesters: calendar.data?.semesters ?? [], weeks: calendar.data?.weeks ?? [],
    inspectionTypes: catalogs.data?.inspectionTypes ?? [],
  })
  const permissionColumns: ProColumns<EffectivePermission>[] = [
    { title: '权限动作', dataIndex: 'action', renderText: actionName },
    { title: '结果', dataIndex: 'effect', width: 90, render: (_, record) => <Tag color={record.effect === 'DENY' ? 'red' : 'green'}>{record.effect === 'DENY' ? '拒绝' : '允许'}</Tag> },
    { title: '生效范围', dataIndex: 'scope', render: (_, record) => <Typography.Text>{scopeText(record.scope)}</Typography.Text> },
    { title: '权限来源', dataIndex: 'sources', render: (_, record) => <Space wrap>{(record.sources ?? []).map(source => <Tag key={`${source.type}-${source.label}`} color="blue">{source.label}</Tag>)}</Space> },
  ]
  const ruleColumns: ProColumns<PermissionRule>[] = [
    { title: '动作', dataIndex: 'action', renderText: actionName }, { title: '效果', dataIndex: 'effect', render: (_, record) => <Tag color={record.effect === 'DENY' ? 'red' : 'green'}>{record.effect === 'DENY' ? '拒绝' : '允许'}</Tag> },
    { title: '范围', render: (_, record) => scopeText(record.scope) }, { title: '备注', dataIndex: 'note', ellipsis: true },
    { title: '操作', valueType: 'option', render: (_, record) => <Space><Button type="link" onClick={() => openRule(record)}>编辑</Button><Popconfirm title="确认删除这条权限规则？" onConfirm={() => remove.mutate(record.id)}><Button type="link" danger>删除</Button></Popconfirm></Space> },
  ]
  return <Drawer title={`${userName} · 生效权限`} open={Boolean(accountId)} onClose={onClose} size="large" destroyOnHidden>
    <Typography.Paragraph type="secondary">部门与检查类型映射决定可用的检查类型和操作；获得的操作按检查类型跨部门生效。新增或导入记录仍归属账号主部门。用户单独授权/限制可按学年、学期、周次和检查类型设置，匹配范围内的拒绝优先。</Typography.Paragraph>
    <ProTable<EffectivePermission> rowKey={(record, index) => `${record.action}-${record.effect}-${index}`} headerTitle="当前生效权限" loading={effective.isLoading} search={false} options={false} pagination={{ pageSize: 8 }} dataSource={effective.data?.permissions ?? []} columns={permissionColumns} locale={{ emptyText: <Empty description="当前没有生效权限" /> }} />
    <ProTable<PermissionRule> rowKey="id" headerTitle="用户补充规则" loading={rules.isLoading} search={false} options={false} pagination={false} dataSource={rules.data ?? []} columns={ruleColumns} toolBarRender={() => [<Button key="add" type="primary" icon={<PlusOutlined aria-hidden />} onClick={() => openRule()}>新增权限规则</Button>]} locale={{ emptyText: <Empty description="暂无用户单独规则" /> }} />
    <Modal title={`${editing ? '编辑' : '新增'}权限规则`} open={modalOpen} onCancel={() => setModalOpen(false)} onOk={() => form.submit()} confirmLoading={save.isPending} width={720} destroyOnHidden>
      <Form key={editing?.id ?? 'new'} form={form} layout="vertical" initialValues={editing ? { ...editing, scope: permissionRuleScope(editing.scope) } : { effect: 'ALLOW', scope: {}, note: '' }} onFinish={values => save.mutate(values)} preserve={false}>
        <Form.Item name="action" label="权限动作" rules={[{ required: true, message: '请选择权限动作' }]}><Select showSearch optionFilterProp="label" options={actionOptions} /></Form.Item>
        <Form.Item noStyle shouldUpdate={(previous, current) => previous.action !== current.action}>{({ getFieldValue }) => {
          if (getFieldValue('action') === 'REPORT_VIEW') return <Typography.Paragraph type="secondary">用户规则可按学年、学期、周次和检查类型收窄通报查看范围；不再设置部门、班级或年级范围。</Typography.Paragraph>
          if (getFieldValue('action') === 'REPORT_FORCE_GENERATE') return <Typography.Paragraph type="secondary">仅可单独授予学生会办公室部长或副部长；一般通报权限不会自动包含此权限。此操作作用于整份周通报，只能按学年、学期、周次和检查类型收窄范围。</Typography.Paragraph>
          if (getFieldValue('action') === 'REPORT_OBLIGATION_MANAGE') return <Typography.Paragraph type="secondary">仅可单独授予学生会办公室部长或副部长；此权限允许维护各周应交部门和检查类型，与日常录入范围分开管理，可按学年、学期、周次和检查类型收窄。</Typography.Paragraph>
          return null
        }}</Form.Item>
        <Form.Item name="effect" label="规则效果" rules={[{ required: true }]}><Select options={[{ value: 'ALLOW', label: '允许（补充授权）' }, { value: 'DENY', label: '拒绝（单独限制）' }]} /></Form.Item>
        <Typography.Text strong>适用时间</Typography.Text>
        <Space wrap align="start" style={{ display: 'flex', marginTop: 8 }}>
          <Form.Item name={['scope', 'academicYearId']} label="学年"><Select allowClear placeholder="全部学年" style={{ width: 190 }} options={(calendar.data?.academicYears ?? []).map(item => ({ value: item.id, label: item.name }))} /></Form.Item>
          <Form.Item name={['scope', 'semesterId']} label="学期"><Select allowClear placeholder="全部学期" style={{ width: 190 }} options={(calendar.data?.semesters ?? []).map(item => ({ value: item.id, label: item.name }))} /></Form.Item>
          <Form.Item name={['scope', 'weekId']} label="周次"><Select allowClear placeholder="全部周次" style={{ width: 190 }} options={(calendar.data?.weeks ?? []).map(item => ({ value: item.id, label: `第 ${item.weekNo} 周` }))} /></Form.Item>
        </Space>
        <Form.Item name={['scope', 'inspectionTypeId']} label="检查类型"><Select allowClear options={(catalogs.data?.inspectionTypes ?? []).map(item => ({ value: item.id, label: item.name }))} /></Form.Item>
        <Form.Item noStyle shouldUpdate>{({ getFieldValue }) => <Typography.Paragraph type="secondary">本规则范围：{scopeText(getFieldValue('scope') ?? {})}。未选择的维度表示全部。</Typography.Paragraph>}</Form.Item>
        <Form.Item name="note" label="授权说明" rules={[{ required: true, message: '请说明设置原因' }]}><Input.TextArea rows={3} maxLength={255} showCount /></Form.Item>
      </Form>
    </Modal>
  </Drawer>
}

function permissionRuleScope(scope: PermissionRule['scope'] | undefined): PermissionRule['scope'] {
  return {
    academicYearId: scope?.academicYearId,
    semesterId: scope?.semesterId,
    weekId: scope?.weekId,
    inspectionTypeId: scope?.inspectionTypeId,
  }
}

function scopeSummary(scope: PermissionRule['scope'], names: { academicYears: { id: string; name: string }[]; semesters: { id: string; name: string }[]; weeks: { id: string; weekNo: number }[]; inspectionTypes: { id: string; name: string }[] }) {
  const label = (items: { id: string; name: string }[], id?: string) => id ? items.find(item => item.id === id)?.name ?? '未加载或已停用' : ''
  const parts = [
    scope?.inspectionTypeId && `检查类型：${label(names.inspectionTypes, scope.inspectionTypeId)}`,
    scope?.academicYearId && `学年：${label(names.academicYears, scope.academicYearId)}`,
    scope?.semesterId && `学期：${label(names.semesters, scope.semesterId)}`,
    scope?.weekId && `周次：${names.weeks.find(item => item.id === scope.weekId)?.weekNo ?? '未加载或已停用'}`,
  ].filter(Boolean)
  return parts.length ? parts.join('；') : '全部范围'
}

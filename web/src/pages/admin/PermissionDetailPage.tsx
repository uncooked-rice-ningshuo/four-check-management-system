import { permissionActionName as actionName, permissionActionOptions as actionOptions } from '../../entities/permission/model/actions'
import { PlusOutlined } from '@ant-design/icons'
import { ProTable, type ProColumns } from '@ant-design/pro-components'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { App, Alert, Button, DatePicker, Drawer, Empty, Form, Input, Modal, Popconfirm, Select, Space, Typography } from 'antd'
import { useEffect, useState } from 'react'
import dayjs from 'dayjs'
import zhCN from 'antd/es/date-picker/locale/zh_CN'
import { supportsOperationRange, type EffectivePermission, type PermissionRule } from '../../entities/permission/model/permission'
import { createPermissionRule, deletePermissionRule, getCatalogs, getEffectivePermissions, getPermissionRules, updatePermissionRule } from '../../shared/api/adminApi'
import { useOperationRangeOptions } from '../../features/permissions/model/useOperationRangeOptions'
import { OperationRangeEditor } from '../../features/permissions/ui/OperationRangeEditor'
import { hasConflictingClasses } from '../../features/permissions/model/rangeGroups'
import './permission-detail.css'


const displayDate = (value?: string) => value ? new Intl.DateTimeFormat('zh-CN', { timeZone: 'Asia/Shanghai' }).format(new Date(value)) : '无截止日期'
const dateInput = (value?: string) => value ? dayjs(value).format('YYYY-MM-DD') : undefined
const status = (rule: PermissionRule) => !rule.enabled || rule.mode === 'DISABLE' || rule.effect === 'DENY' ? '已禁用' : rule.startsAt && Date.parse(rule.startsAt) > Date.now() ? '未生效' : rule.endsAt && Date.parse(rule.endsAt) <= Date.now() ? '已到期' : '有效'
type PermissionFormValues = PermissionRule & { validity: [string, string?] }
const isDepartmentAction = (action?: string) => Boolean(action && (action.startsWith('INSPECTION_') || action.startsWith('REPORT_') || action === 'ARCHIVE_VIEW' || action === 'MORNING_SCHEDULE_MANAGE'))

export function PermissionDetailDrawer({ accountId, userName, onClose }: { accountId: string; userName: string; onClose: () => void }) {
  const { message, modal } = App.useApp()
  const [editing, setEditing] = useState<PermissionRule | null>(null)
  const [initial, setInitial] = useState<Partial<PermissionRule>>({})
  const [modalOpen, setModalOpen] = useState(false)
  const [dirty, setDirty] = useState(false)
  const [form] = Form.useForm<PermissionFormValues>()
  const client = useQueryClient()
  const effective = useQuery({ queryKey: ['effective-permissions', accountId], queryFn: async () => { const result = await getEffectivePermissions(accountId); if (!Array.isArray(result.permissions) || !Array.isArray(result.defaultPermissions)) throw new Error('权限响应不完整，请重试'); return result } })
  const rules = useQuery({ queryKey: ['permission-rules', accountId], queryFn: async () => { const result = await getPermissionRules(accountId); if (!Array.isArray(result)) throw new Error('个人权限响应不完整，请重试'); return result } })
  const catalogs = useQuery({ queryKey: ['admin-catalogs'], queryFn: getCatalogs })
  const rangeOptions = useOperationRangeOptions()
  useEffect(() => {
    const boundaries = [...(effective.data?.defaultPermissions ?? []), ...(rules.data ?? [])].flatMap(entry => [entry.startsAt, entry.endsAt]).filter((value): value is string => Boolean(value)).map(Date.parse).filter(value => value > Date.now())
    if (!boundaries.length) return
    const timer = window.setTimeout(() => { void client.invalidateQueries({ queryKey: ['effective-permissions', accountId] }); void client.invalidateQueries({ queryKey: ['permission-rules', accountId] }) }, Math.min(Math.max(0, Math.min(...boundaries) - Date.now()) + 50, 2147483647))
    return () => window.clearTimeout(timer)
  }, [effective.data, rules.data, accountId, client])

  const refresh = async () => { await Promise.all([client.invalidateQueries({ queryKey: ['legacy-stage-grants'] }), client.invalidateQueries({ queryKey: ['effective-permissions', accountId] }), client.invalidateQueries({ queryKey: ['permission-rules', accountId] })]) }
  const close = () => { if (dirty) modal.confirm({ title: '放弃未保存的权限修改？', onOk: () => { setModalOpen(false); setDirty(false) } }); else setModalOpen(false) }
  const save = useMutation({ mutationFn: ({ validity, ...fields }: PermissionFormValues) => {
    const values = { ...fields, startsAt: validity[0], endsAt: validity[1] }
    const input = { ...values, mode: initial.mode === 'DISABLE' ? 'OVERRIDE' : initial.mode ?? 'ADD', scope: values.action.startsWith('INSPECTION_') ? { inspectionTypeId: values.scope?.inspectionTypeId } : {}, ranges: supportsOperationRange(values.action) ? values.ranges ?? [] : [], startsAt: `${values.startsAt}T00:00:00+08:00`, endsAt: values.endsAt ? `${dayjs(values.endsAt).add(1, 'day').format('YYYY-MM-DD')}T00:00:00+08:00` : undefined }
    return editing ? updatePermissionRule({ ...editing, ...input }) : createPermissionRule(accountId, input)
  }, onSuccess: async () => { await refresh(); setModalOpen(false); setDirty(false); message.success('权限已保存') }, onError: error => message.error(error.message) })
  const remove = useMutation({ mutationFn: (id: string) => deletePermissionRule(accountId, id), onSuccess: refresh, onError: error => message.error(error.message) })
  const disable = useMutation({ mutationFn: (entry: EffectivePermission) => {
    const existing = rules.data?.find(rule => rule.sourceDepartmentId === entry.sourceDepartmentId && rule.action === entry.action && rule.scope.inspectionTypeId === entry.scope.inspectionTypeId && rule.mode !== 'ADD')
    const input = { action: entry.action, effect: 'ALLOW' as const, sourceDepartmentId: entry.sourceDepartmentId, mode: 'DISABLE' as const, scope: { inspectionTypeId: entry.scope.inspectionTypeId }, ranges: [], startsAt: new Date().toISOString(), endsAt: undefined, note: '管理员禁用此项默认权限' }
    return existing ? updatePermissionRule({ ...existing, ...input }) : createPermissionRule(accountId, input)
  }, onSuccess: refresh, onError: error => message.error(error.message) })
  const openRule = (rule?: PermissionRule, baseline?: EffectivePermission) => {
    if (!rule && baseline) rule = rules.data?.find(item => item.sourceDepartmentId === baseline.sourceDepartmentId && item.action === baseline.action && item.scope.inspectionTypeId === baseline.scope.inspectionTypeId && (item.mode === 'OVERRIDE' || item.mode === 'DISABLE'))
    setEditing(rule ?? null); setDirty(false)
    setInitial(rule ? { ...rule, startsAt: dateInput(rule.startsAt), endsAt: rule.endsAt ? dateInput(new Date(Date.parse(rule.endsAt) - 1).toISOString()) : undefined } : baseline ? { ...baseline, mode: 'OVERRIDE', startsAt: dateInput(baseline.startsAt) ?? dayjs().format('YYYY-MM-DD'), endsAt: baseline.endsAt ? dateInput(new Date(Date.parse(baseline.endsAt) - 1).toISOString()) : undefined } : { effect: 'ALLOW', mode: 'ADD', scope: {}, startsAt: dayjs().format('YYYY-MM-DD'), note: '' })
    setModalOpen(true)
  }
  const departmentName = (id?: string) => id ? catalogs.data?.departments.find(item => item.id === id)?.name ?? '部门加载中' : '账号全局'
  const scopeText = (entry: EffectivePermission | PermissionRule) => !supportsOperationRange(entry.action) ? '—' : !entry.ranges?.length ? '不限' : entry.ranges.map(range => [range.gradeStage && `待人工调整：${['大一', '大二', '大三', '大四'][range.gradeStage - 1]}`, range.gradeId && (rangeOptions.data?.gradeId.find(item => item.value === range.gradeId)?.label ?? '年级加载中'), range.majorId && (rangeOptions.data?.majorId.find(item => item.value === range.majorId)?.label ?? '专业加载中'), range.classId && (rangeOptions.data?.classId.find(item => item.value === range.classId)?.label ?? '班级加载中')].filter(Boolean).join('、')).join('；或 ')
  const dates = (entry: EffectivePermission | PermissionRule) => `${entry.startsAt ? displayDate(entry.startsAt) : '立即生效'} 至 ${entry.endsAt ? displayDate(new Date(Date.parse(entry.endsAt) - 1).toISOString()) : '无截止日期'}`
  const permissionColumns: ProColumns<EffectivePermission>[] = [
    { title: '功能', dataIndex: 'action', renderText: actionName }, { title: '来源部门', render: (_, entry) => departmentName(entry.sourceDepartmentId) },
    { title: '检查业务', render: (_, entry) => entry.scope.inspectionTypeId ? `检查类型：${catalogs.data?.inspectionTypes.find(item => item.id === entry.scope.inspectionTypeId)?.name ?? '加载中'}` : '—' },
    { title: '操作范围', render: (_, entry) => scopeText(entry.current ?? entry) }, { title: '有效日期', render: (_, entry) => dates(entry.current ?? entry) },
    { title: '状态', render: (_, entry) => entry.status ?? (entry.effect === 'DENY' ? '已禁用' : '有效') },
    { title: '操作', valueType: 'option', render: (_, entry) => entry.sourceDepartmentId && entry.templateVersion ? <Space><Button type="link" onClick={() => openRule(undefined, entry)}>修改</Button><Popconfirm title="禁用此来源的权限？其他来源保持原授权。" onConfirm={() => disable.mutate(entry)}><Button type="link" danger>禁用此项</Button></Popconfirm></Space> : null },
  ]
  const ruleColumns: ProColumns<PermissionRule>[] = [
    { title: '功能', dataIndex: 'action', renderText: actionName }, { title: '来源部门', render: (_, entry) => departmentName(entry.sourceDepartmentId) },
    { title: '检查业务', render: (_, entry) => entry.scope.inspectionTypeId ? `检查类型：${catalogs.data?.inspectionTypes.find(item => item.id === entry.scope.inspectionTypeId)?.name ?? '加载中'}` : '—' },
    { title: '操作范围', render: (_, entry) => scopeText(entry) }, { title: '有效日期', render: (_, entry) => dates(entry) }, { title: '状态', render: (_, entry) => status(entry) },
    { title: '操作', valueType: 'option', render: (_, entry) => <Space><Button type="link" onClick={() => openRule(entry)}>修改</Button><Popconfirm title={entry.mode === 'ADD' ? '删除此个人授权？其他默认来源仍可能授予同一权限。' : '恢复该账号授予时的默认设置，不更新为最新模板，也不重新计算有效期。'} onConfirm={() => remove.mutate(entry.id)}><Button type="link">{entry.mode === 'ADD' ? '删除个人授权' : '恢复默认'}</Button></Popconfirm></Space> },
  ]
  return <Drawer className="permission-detail-drawer" title={`${userName} · 权限详情`} open onClose={onClose} size="large" destroyOnHidden>
    <Typography.Paragraph type="secondary">查看角色权限，或为此账号单独调整权限。</Typography.Paragraph>
    {(effective.isError || rules.isError || catalogs.isError) && <Alert type="error" title="权限信息加载失败" action={<Button onClick={() => { void effective.refetch(); void rules.refetch(); void catalogs.refetch() }}>重试</Button>} />}
    {!effective.isError && <ProTable<EffectivePermission> rowKey={entry => `${entry.sourceDepartmentId ?? 'system'}-${entry.action}-${entry.scope.inspectionTypeId ?? 'all'}`} headerTitle="角色权限" expandable={{ expandedRowRender: entry => <Typography.Paragraph>授予时默认：{scopeText(entry)}；{dates(entry)}。{rules.data?.some(rule => rule.enabled && rule.sourceDepartmentId === entry.sourceDepartmentId && rule.action === entry.action && rule.scope.inspectionTypeId === entry.scope.inspectionTypeId && rule.mode !== 'ADD') ? '当前已由个人设置调整，详见下方。' : ''}</Typography.Paragraph> }} size="small" cardProps={false} pagination={false} search={false} options={false} loading={effective.isLoading} dataSource={effective.data?.defaultPermissions ?? []} columns={permissionColumns} scroll={{ x: 'max-content' }} />}
    {!rules.isError && <ProTable<PermissionRule> rowKey="id" headerTitle="个人权限设置" size="small" cardProps={false} pagination={false} search={false} options={false} loading={rules.isLoading} dataSource={rules.data ?? []} columns={ruleColumns} scroll={{ x: 'max-content' }} toolBarRender={() => [<Button key="add" type="primary" disabled={!effective.data || effective.isError || rules.isError || catalogs.isError} icon={<PlusOutlined aria-hidden />} onClick={() => openRule()}>新增权限</Button>]} locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无个人设置" /> }} />}
    <Modal className="permission-rule-modal" title={`${editing || initial.mode === 'OVERRIDE' ? '修改' : '新增'}权限`} open={modalOpen} onCancel={close} onOk={() => form.submit()} confirmLoading={save.isPending} width={600} destroyOnHidden>
      <Form key={`${editing?.id ?? initial.action ?? 'new'}-${initial.mode}`} form={form} layout="vertical" initialValues={{ ...initial, validity: [initial.startsAt, initial.endsAt] }} onFinish={values => save.mutate(values)} preserve={false} scrollToFirstError validateTrigger="onBlur" onValuesChange={changed => { setDirty(true); if ('action' in changed) { form.setFieldValue('ranges', []); form.setFieldValue('scope', {}); form.setFieldValue('sourceDepartmentId', isDepartmentAction(changed.action) && effective.data?.departmentIds?.length === 1 ? effective.data.departmentIds[0] : undefined) } }}>
        <Form.Item name="action" label="功能权限" rules={[{ required: true, message: '请选择功能' }]}><Select aria-label="功能权限" showSearch optionFilterProp="label" options={actionOptions.map(item => ({ ...item, disabled: item.value.startsWith('REPORT_') && item.value !== 'REPORT_VIEW' && !effective.data?.reportGrantableActions?.includes(item.value) }))} disabled={initial.mode !== 'ADD'} /></Form.Item>
        <Form.Item name="effect" label="权限设置" rules={[{ required: true }]}><Select options={[{ value: 'ALLOW', label: '允许使用' }, { value: 'DENY', label: '禁止使用' }]} /></Form.Item>
        <Form.Item noStyle shouldUpdate={(before, after) => before.action !== after.action || before.effect !== after.effect}>{({ getFieldValue }) => isDepartmentAction(getFieldValue('action')) ? <Form.Item name="sourceDepartmentId" label="适用部门" rules={[{ required: effective.data?.systemRole === 'MEMBER' && getFieldValue('effect') === 'ALLOW', message: '请选择此权限适用的部门' }]} extra={effective.data?.departmentIds?.length ? '这项权限仅用于所选部门的业务，不会改变账号的部门归属。' : '此账号尚无已授予权限的部门，请先保存账号的部门与角色。'}><Select aria-label="适用部门" allowClear={getFieldValue('effect') === 'DENY' || effective.data?.systemRole !== 'MEMBER'} notFoundContent="暂无已授予权限的部门" placeholder={getFieldValue('effect') === 'DENY' ? '全部所属部门' : '请选择部门'} disabled={initial.mode !== 'ADD' || effective.data?.departmentIds?.length === 1 && getFieldValue('effect') === 'ALLOW'} options={(catalogs.data?.departments ?? []).filter(item => effective.data?.departmentIds?.includes(item.id)).map(item => ({ value: item.id, label: item.name }))} /></Form.Item> : null}</Form.Item>
        <Form.Item noStyle shouldUpdate={(before, after) => before.action !== after.action}>{({ getFieldValue }) => <>
          {String(getFieldValue('action') ?? '').startsWith('INSPECTION_') && <Form.Item name={['scope', 'inspectionTypeId']} label="检查业务" rules={[{ required: true, message: '请选择检查业务' }]}><Select aria-label="检查业务" options={catalogs.data?.inspectionTypes.map(item => ({ value: item.id, label: item.name }))} /></Form.Item>}
          {supportsOperationRange(getFieldValue('action')) && <Form.Item name="ranges" label="操作范围" rules={[{ validator: (_, ranges) => ranges?.some((range: { gradeStage?: number }) => range.gradeStage) ? Promise.reject(new Error('请选择具体年级以替换旧阶段')) : ranges?.some((range: { gradeId?: string; majorId?: string; classId?: string; gradeStage?: number }) => !range.gradeId && !range.majorId && !range.classId && !range.gradeStage) ? Promise.reject(new Error('每组至少选择一个条件，或删除空组')) : hasConflictingClasses(ranges ?? [], rangeOptions.data?.classId ?? []) ? Promise.reject(new Error('所选班级与年级或专业不匹配，请重新选择班级')) : Promise.resolve() }]}><OperationRangeEditor /></Form.Item>}
        </>}</Form.Item>
        <Form.Item name="validity" label="有效日期" extra="结束日期可留空，表示长期有效；有结束日期时包含当天。" rules={[{ validator: (_, value: [string, string?] | undefined) => !value?.[0] ? Promise.reject(new Error('请选择开始日期')) : value[1] && value[1] < value[0] ? Promise.reject(new Error('结束日期不能早于开始日期')) : Promise.resolve() }]} getValueProps={value => ({ value: value?.map((date: string | undefined) => date ? dayjs(date) : null) })} getValueFromEvent={(_dates, values) => values}><DatePicker.RangePicker locale={zhCN} aria-label="有效日期" placeholder={['开始日期', '结束日期（可不填）']} allowEmpty={[false, true]} style={{ width: '100%' }} /></Form.Item>
        <Form.Item name="note" label="原因" rules={[{ required: true, message: '请填写原因' }]}><Input.TextArea aria-label="原因" rows={3} maxLength={255} showCount /></Form.Item>
        {save.isError && <Alert type="error" title="保存失败，输入已保留" description={save.error.message} />}
      </Form>
    </Modal>
  </Drawer>
}

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ProTable } from '@ant-design/pro-components'
import { Button, Card, Form, Input, Modal, Switch, Tabs, Typography, message } from 'antd'
import { useState } from 'react'
import type { Department, DepartmentInspectionScope, InspectionType } from '../../entities/organization/model/organization'
import { createDepartment, getCatalogs, getDepartmentInspectionScopes, updateDepartment, updateDepartmentInspectionScope, updateInspectionType } from '../../shared/api/adminApi'

export function OrganizationConfigPage() {
	const [modal, modalContextHolder] = Modal.useModal()
  const [departmentModal, setDepartmentModal] = useState(false)
  const [form] = Form.useForm()
  const client = useQueryClient()
  const catalogs = useQuery({ queryKey: ['admin-catalogs'], queryFn: getCatalogs })
  const scopes = useQuery({ queryKey: ['department-inspection-scopes'], queryFn: getDepartmentInspectionScopes })
  const refresh = async () => { await client.invalidateQueries({ queryKey: ['admin-catalogs'] }); await client.invalidateQueries({ queryKey: ['department-inspection-scopes'] }) }
  const addDepartment = useMutation({ mutationFn: createDepartment, onSuccess: async () => { await refresh(); setDepartmentModal(false); form.resetFields(); message.success('部门已新增') } })
  const toggleDepartment = useMutation({ mutationFn: updateDepartment, onSuccess: refresh })
  const toggleInspection = useMutation({ mutationFn: updateInspectionType, onSuccess: refresh })
  const saveScope = useMutation({ mutationFn: updateDepartmentInspectionScope, onSuccess: async () => { await refresh(); message.success('业务权限已更新') } })
  const scopeRows = (catalogs.data?.departments ?? []).flatMap(department => (catalogs.data?.inspectionTypes ?? []).map(inspection => scopes.data?.find(item => item.departmentId === department.id && item.inspectionTypeId === inspection.id) ?? { departmentId: department.id, inspectionTypeId: inspection.id, canEnter: false, canView: false, canReview: false, canExport: false, canSummarize: false, enabled: true }))
  const nameOf = (values: { id: string; name: string }[] | undefined, id: string) => values?.find(item => item.id === id)?.name ?? id
  const permissionLabels = { canEnter: '录入', canView: '查看', canReview: '审核', canExport: '导出', canSummarize: '汇总' } as const
  const confirmDepartmentStatus = (record: Department, enabled: boolean) => {
    if (enabled) { toggleDepartment.mutate({ ...record, enabled }); return }
	modal.confirm({ title: `停用“${record.name}”部门？`, content: '停用后，相关人员归属和业务权限将不可继续用于新操作。', okText: '确认停用', okButtonProps: { danger: true }, cancelText: '取消', onOk: () => toggleDepartment.mutateAsync({ ...record, enabled }) })
  }
  const confirmInspectionStatus = (record: InspectionType, enabled: boolean) => {
    if (enabled) { toggleInspection.mutate({ ...record, enabled }); return }
	modal.confirm({ title: `停用“${record.name}”检查类型？`, content: '停用后，该类型将不能继续用于录入、审核和通报流程。', okText: '确认停用', okButtonProps: { danger: true }, cancelText: '取消', onOk: () => toggleInspection.mutateAsync({ ...record, enabled }) })
  }
	const confirmReportParticipation = (record: InspectionType, participatesInReport: boolean) => modal.confirm({ title: `修改“${record.name}”通报范围？`, content: participatesInReport ? '启用后，该检查类型的数据将参与后续通报。' : '关闭后，该检查类型的新数据将不再默认参与通报。', okText: '确认修改', cancelText: '取消', onOk: () => toggleInspection.mutateAsync({ ...record, participatesInReport }) })
  const confirmScope = (record: DepartmentInspectionScope, key: keyof typeof permissionLabels, value: boolean) => {
    const departmentName = nameOf(catalogs.data?.departments, record.departmentId)
    const inspectionName = nameOf(catalogs.data?.inspectionTypes, record.inspectionTypeId)
    const permissionName = `${permissionLabels[key]}权限`
	modal.confirm({ title: `修改${departmentName} · ${inspectionName} · ${permissionName}？`, content: '该操作会改变该部门成员可执行的业务范围。', okText: '确认修改', cancelText: '取消', onOk: () => saveScope.mutateAsync({ ...record, [key]: value }) })
  }
	return <section className="admin-page">{modalContextHolder}<div className="page-heading"><div><Typography.Title level={2}>组织与业务</Typography.Title><Typography.Paragraph>部门、检查类型和权限映射均为独立业务配置，不写死在页面权限中。</Typography.Paragraph></div><Button type="primary" onClick={() => setDepartmentModal(true)}>新增部门</Button></div>
    <Card><Tabs items={[
      { key: 'departments', label: '部门', children: <ProTable<Department> rowKey="id" search={false} options={false} pagination={false} dataSource={catalogs.data?.departments} columns={[{ title: '编码', dataIndex: 'code' }, { title: '名称', dataIndex: 'name' }, { title: '状态', render: (_, record) => <Switch aria-label={`${record.enabled ? '停用' : '启用'}部门 ${record.name}`} checked={record.enabled} onChange={enabled => confirmDepartmentStatus(record, enabled)} /> }]} /> },
      { key: 'types', label: '七类检查', children: <ProTable<InspectionType> rowKey="id" search={false} options={false} pagination={false} dataSource={catalogs.data?.inspectionTypes} columns={[{ title: '顺序', dataIndex: 'sortOrder', width: 80 }, { title: '编码', dataIndex: 'code' }, { title: '名称', dataIndex: 'name' }, { title: '参与通报', render: (_, record) => <Switch aria-label={`${record.participatesInReport ? '取消' : '启用'}通报范围 ${record.name}`} checked={record.participatesInReport} onChange={participatesInReport => confirmReportParticipation(record, participatesInReport)} /> }, { title: '状态', render: (_, record) => <Switch aria-label={`${record.enabled ? '停用' : '启用'}检查类型 ${record.name}`} checked={record.enabled} onChange={enabled => confirmInspectionStatus(record, enabled)} /> }]} /> },
      { key: 'scopes', label: '部门业务映射', children: <ProTable<DepartmentInspectionScope> rowKey={record => `${record.departmentId}-${record.inspectionTypeId}`} search={false} options={{ density: true, setting: true }} pagination={{ pageSize: 14 }} dataSource={scopeRows} scroll={{ x: 900 }} columns={[
        { title: '部门', render: (_, record) => nameOf(catalogs.data?.departments, record.departmentId) }, { title: '检查类型', render: (_, record) => nameOf(catalogs.data?.inspectionTypes, record.inspectionTypeId) },
        ...(['canEnter', 'canView', 'canReview', 'canExport', 'canSummarize'] as const).map(key => ({ title: permissionLabels[key], render: (_: unknown, record: DepartmentInspectionScope) => <Switch aria-label={`${nameOf(catalogs.data?.departments, record.departmentId)} ${nameOf(catalogs.data?.inspectionTypes, record.inspectionTypeId)} ${permissionLabels[key]}权限`} size="small" checked={record[key]} onChange={value => confirmScope(record, key, value)} /> })),
      ]} /> },
    ]} /></Card>
    <Modal title="新增部门" open={departmentModal} onCancel={() => setDepartmentModal(false)} onOk={() => form.submit()} confirmLoading={addDepartment.isPending}><Form form={form} layout="vertical" onFinish={values => addDepartment.mutate(values)}><Form.Item name="code" label="部门编码" rules={[{ required: true }]}><Input autoComplete="off" placeholder="例如：PUBLICITY…" /></Form.Item><Form.Item name="name" label="部门名称" rules={[{ required: true }]}><Input autoComplete="off" /></Form.Item></Form></Modal>
  </section>
}

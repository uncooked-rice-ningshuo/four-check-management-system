import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ProTable } from '@ant-design/pro-components'
import { Alert, App, Button, Form, Input, Modal, Switch, Tabs, Typography } from 'antd'
import type { Department, InspectionType } from '../../entities/organization/model/organization'
import { createDepartment, getCatalogs, updateDepartment, updateInspectionType } from '../../shared/api/adminApi'
import { DepartmentDefaultsPanel } from './DepartmentDefaultsPanel'

export function OrganizationConfigPage() {
  const { message } = App.useApp()
  const [modal, modalContextHolder] = Modal.useModal()
  const [departmentModal, setDepartmentModal] = useState(false)
  const [form] = Form.useForm()
  const client = useQueryClient()
  const catalogs = useQuery({ queryKey: ['admin-catalogs'], queryFn: getCatalogs })
  const refresh = () => client.invalidateQueries({ queryKey: ['admin-catalogs'] })
  const addDepartment = useMutation({ mutationFn: createDepartment, onSuccess: async () => { await refresh(); setDepartmentModal(false); form.resetFields(); message.success('部门已新增') }, onError: error => message.error(error.message) })
  const toggleDepartment = useMutation({ mutationFn: updateDepartment, onSuccess: refresh, onError: error => message.error(error.message) })
  const toggleInspection = useMutation({ mutationFn: updateInspectionType, onSuccess: refresh, onError: error => message.error(error.message) })
  const confirmDepartmentStatus = (record: Department, enabled: boolean) => {
    if (enabled) { toggleDepartment.mutate({ ...record, enabled }); return }
    modal.confirm({ title: `停用“${record.name}”部门？`, content: '停用后，相关人员归属和业务权限将不可继续用于新操作。', okText: '确认停用', okButtonProps: { danger: true }, cancelText: '取消', onOk: () => toggleDepartment.mutateAsync({ ...record, enabled }) })
  }
  const confirmInspectionStatus = (record: InspectionType, enabled: boolean) => {
    if (enabled) { toggleInspection.mutate({ ...record, enabled }); return }
    modal.confirm({ title: `停用“${record.name}”检查类型？`, content: '停用后，该类型将不能继续用于录入、审核和通报流程。', okText: '确认停用', okButtonProps: { danger: true }, cancelText: '取消', onOk: () => toggleInspection.mutateAsync({ ...record, enabled }) })
  }
  const confirmReportParticipation = (record: InspectionType, participatesInReport: boolean) => modal.confirm({ title: `修改“${record.name}”通报范围？`, content: participatesInReport ? '启用后，该检查类型的数据将参与后续通报。' : '关闭后，该检查类型的新数据将不再默认参与通报。', okText: '确认修改', cancelText: '取消', onOk: () => toggleInspection.mutateAsync({ ...record, participatesInReport }) })
  return <section className="admin-page organization-config-page">{modalContextHolder}
    <div className="page-heading"><div><Typography.Title level={2}>部门与默认权限</Typography.Title><Typography.Paragraph>按部门配置日常检查、周通报及岗位默认动作；个人授权在“用户与角色”中维护。</Typography.Paragraph></div></div>
    {catalogs.isError && <Alert type="error" showIcon title="部门目录加载失败" action={<Button onClick={() => void catalogs.refetch()}>重试</Button>} />}
    {catalogs.isPending && <div role="status">正在加载部门目录</div>}
    {catalogs.data && <Tabs defaultActiveKey="defaults" items={[
      { key: 'defaults', label: '默认权限', children: <div className="department-defaults-surface"><DepartmentDefaultsPanel catalogs={catalogs.data} /></div> },
      { key: 'departments', label: '部门', children: <ProTable<Department> rowKey="id" headerTitle="部门列表" search={false} options={false} pagination={false} dataSource={catalogs.data.departments}
          toolBarRender={() => [<Button key="create" type="primary" onClick={() => setDepartmentModal(true)}>新增部门</Button>]}
          columns={[{ title: '编码', dataIndex: 'code' }, { title: '名称', dataIndex: 'name' }, { title: '状态', render: (_, record) => <Switch aria-label={`${record.enabled ? '停用' : '启用'}部门 ${record.name}`} checked={record.enabled} onChange={enabled => confirmDepartmentStatus(record, enabled)} /> }]} /> },
      { key: 'types', label: '检查类型', children: <ProTable<InspectionType> rowKey="id" headerTitle="检查类型列表" search={false} options={false} pagination={false} dataSource={catalogs.data.inspectionTypes} columns={[{ title: '顺序', dataIndex: 'sortOrder', width: 80 }, { title: '编码', dataIndex: 'code' }, { title: '名称', dataIndex: 'name' }, { title: '参与通报', render: (_, record) => <Switch aria-label={`${record.participatesInReport ? '取消' : '启用'}通报范围 ${record.name}`} checked={record.participatesInReport} onChange={participatesInReport => confirmReportParticipation(record, participatesInReport)} /> }, { title: '状态', render: (_, record) => <Switch aria-label={`${record.enabled ? '停用' : '启用'}检查类型 ${record.name}`} checked={record.enabled} onChange={enabled => confirmInspectionStatus(record, enabled)} /> }]} /> },
    ]} />}
    <Modal title="新增部门" open={departmentModal} onCancel={() => setDepartmentModal(false)} onOk={() => form.submit()} confirmLoading={addDepartment.isPending}><Form form={form} layout="vertical" onFinish={values => addDepartment.mutate(values)}><Form.Item name="code" label="部门编码" rules={[{ required: true }]}><Input autoComplete="off" placeholder="例如：PUBLICITY…" /></Form.Item><Form.Item name="name" label="部门名称" rules={[{ required: true }]}><Input autoComplete="off" /></Form.Item></Form></Modal>
  </section>
}

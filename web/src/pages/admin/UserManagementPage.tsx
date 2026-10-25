import { PlusOutlined, SafetyCertificateOutlined } from '@ant-design/icons'
import { ProTable, type ProColumns } from '@ant-design/pro-components'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { App, Alert, Button, Form, Input, Modal, Popconfirm, Select, Space, Tag, Tooltip, Typography } from 'antd'
import { useMemo, useState } from 'react'
import type { UserAssignment } from '../../entities/organization/model/organization'
import { assignUserRole, createUser, getCatalogs, getUsers, resetUserPassword, updateUser, updateUserStatus } from '../../shared/api/adminApi'
import { ApiError } from '../../shared/api/httpClient'
import { PermissionDetailDrawer } from './PermissionDetailPage'

type UserFormValues = { studentNo: string; name: string; systemRole: 'ADMIN' | 'MEMBER'; departmentId?: string; organizationRoleId?: string }
type UserFilterValues = {
  keyword?: string
  systemRole?: UserAssignment['systemRole']
  status?: string
  departmentId?: string
  organizationRoleId?: string
}

export function UserManagementPage() {
  const { message } = App.useApp()
  const [filters, setFilters] = useState<UserFilterValues>({})
  const [editing, setEditing] = useState<UserAssignment | null>(null)
  const [editVersionConflict, setEditVersionConflict] = useState(false)
  const [userModal, setUserModal] = useState(false)
  const [resetting, setResetting] = useState<UserAssignment | null>(null)
  const [permissionUser, setPermissionUser] = useState<UserAssignment | null>(null)
  const [userForm] = Form.useForm<UserFormValues>()
  const [issuedPassword, setIssuedPassword] = useState<{ kind: 'created' | 'reset'; name: string; loginName: string; password: string } | null>(null)
  const client = useQueryClient()
  const users = useQuery({ queryKey: ['admin-users'], queryFn: getUsers })
  const catalogs = useQuery({ queryKey: ['admin-catalogs'], queryFn: getCatalogs })
  const refresh = () => client.invalidateQueries({ queryKey: ['admin-users'] })
  const saveUser = useMutation({ mutationFn: async (values: UserFormValues) => {
    if (!editing) return createUser(values)
    if (catalogs.isError || catalogs.isPending) throw new Error('组织目录加载失败，请重试后保存')
    let assignment: UserAssignment | null = null
    if ((values.departmentId ?? '') !== editing.departmentId || (values.organizationRoleId ?? '') !== editing.organizationRoleId) {
      if (!values.departmentId || !values.organizationRoleId) throw new Error('部门和组织角色须同时选择')
      assignment = { ...editing, ...values, departmentId: values.departmentId, organizationRoleId: values.organizationRoleId }
    }
    const updated = await updateUser(editing.accountId, { ...values, version: editing.version })
    if (assignment) {
      setEditing(current => current?.accountId === editing.accountId ? { ...current, version: updated.version } : current)
      try { await assignUserRole({ ...assignment, version: updated.version }) } catch (error) {
        if (error instanceof ApiError && error.code === 'VERSION_CONFLICT') {
          throw new ApiError(error.status, error.code, '基本信息已保存，但部门和组织角色已被其他操作更新。请关闭弹窗并重新打开最新记录。')
        }
        throw new Error(`基本信息已保存，但部门和组织角色更新失败：${error instanceof Error ? error.message : '请重试'}`, { cause: error })
      }
    }
    return null
  }, onSuccess: async result => { await refresh(); setUserModal(false); setEditing(null); setEditVersionConflict(false); userForm.resetFields(); if (result?.temporaryPassword) setIssuedPassword({ kind: 'created', name: result.name, loginName: result.loginName, password: result.temporaryPassword }); else message.success('用户信息已保存') }, onError: async error => { await refresh(); if (error instanceof ApiError && error.code === 'VERSION_CONFLICT') setEditVersionConflict(true); message.error(error.message) } })
  const statusMutation = useMutation({ mutationFn: (record: UserAssignment) => updateUserStatus(record.accountId, record.status === 'ACTIVE' ? 'DISABLED' : 'ACTIVE', record.version), onSuccess: async () => { await refresh(); message.success('账号状态已更新，既有会话已失效') }, onError: async error => { await refresh(); message.error(error instanceof ApiError && error.code === 'VERSION_CONFLICT' ? '账号信息已变化，未执行状态切换；请基于刷新后的状态重新操作。' : error.message) } })
  const resetMutation = useMutation({ mutationFn: () => resetUserPassword(resetting!.accountId, resetting!.version), onSuccess: result => { setIssuedPassword({ kind: 'reset', name: resetting!.name, loginName: resetting!.loginName, password: result.temporaryPassword }); setResetting(null) }, onError: async error => { await refresh(); if (error instanceof ApiError && error.code === 'VERSION_CONFLICT') { setResetting(null); message.error('账号信息已变化，未重置密码；请重新打开记录并确认。'); return }; message.error(error.message) } })
  const closeIssuedPassword = () => { setIssuedPassword(null); saveUser.reset(); resetMutation.reset() }
  const openCreate = () => { userForm.resetFields(); setEditing(null); setEditVersionConflict(false); setUserModal(true) }
  const openEdit = (record: UserAssignment) => { userForm.resetFields(); setEditing(record); setEditVersionConflict(false); setUserModal(true) }
  const departmentOptions = catalogs.data?.departments.filter(item => item.enabled).map(item => ({ value: item.id, label: item.name })) ?? []
  const organizationRoleOptions = catalogs.data?.organizationRoles.filter(item => item.enabled).map(item => ({ value: item.id, label: item.name })) ?? []
  const columns: ProColumns<UserAssignment>[] = [
    { title: '关键字', dataIndex: 'keyword', hideInTable: true, order: 5, fieldProps: { allowClear: true, autoComplete: 'off', placeholder: '输入账号、学号或姓名…', spellCheck: false } },
    { title: '账号', dataIndex: 'loginName', width: 138, search: false, copyable: true, ellipsis: true },
    { title: '学号', dataIndex: 'studentNo', width: 138, search: false, copyable: true, ellipsis: true, renderText: value => value || '—' },
    { title: '姓名', dataIndex: 'name', width: 104, search: false, ellipsis: true },
    { title: '系统角色', dataIndex: 'systemRole', width: 126, order: 4, valueType: 'select', valueEnum: { SUPER_ADMIN: { text: '超级管理员' }, ADMIN: { text: '管理员' }, MEMBER: { text: '成员/普通用户' } }, fieldProps: { placeholder: '全部系统角色', allowClear: true }, render: (_, record) => <Tag color={record.isSuperAdmin ? 'gold' : record.systemRole === 'ADMIN' ? 'blue' : 'default'}>{record.isSuperAdmin ? '超级管理员' : record.systemRole === 'ADMIN' ? '管理员' : '普通用户'}</Tag> },
    { title: '账号状态', dataIndex: 'status', width: 110, order: 3, valueType: 'select', valueEnum: { ACTIVE: { text: '正常', status: 'Success' }, PENDING: { text: '待审核', status: 'Warning' }, DISABLED: { text: '已禁用', status: 'Default' }, REJECTED: { text: '已驳回', status: 'Error' } }, fieldProps: { placeholder: '全部状态', allowClear: true }, render: (_, record) => <Space orientation="vertical" size={2}><Tag color={record.status === 'ACTIVE' ? 'green' : record.status === 'PENDING' ? 'gold' : record.status === 'REJECTED' ? 'red' : 'default'}>{statusLabel(record.status)}</Tag>{record.mustChangePassword && <Typography.Text type="warning" className="password-state">待改密</Typography.Text>}</Space> },
    { title: '所属部门', dataIndex: 'departmentId', width: 166, order: 2, valueType: 'select', fieldProps: { options: departmentOptions, placeholder: '全部部门', allowClear: true, showSearch: true, optionFilterProp: 'label' }, render: (_, record) => <Tag className="user-assignment-tag" color={record.departmentId ? 'blue' : 'default'} title={catalogs.data?.departments.find(item => item.id === record.departmentId)?.name}>{catalogs.data?.departments.find(item => item.id === record.departmentId)?.name ?? (record.departmentId ? '部门信息不可用' : '未分配')}</Tag> },
    { title: '组织角色', dataIndex: 'organizationRoleId', width: 150, order: 1, valueType: 'select', fieldProps: { options: organizationRoleOptions, placeholder: '全部组织角色', allowClear: true, showSearch: true, optionFilterProp: 'label' }, render: (_, record) => <Tag className="user-assignment-tag" color={record.organizationRoleId ? 'purple' : 'default'} title={catalogs.data?.organizationRoles.find(item => item.id === record.organizationRoleId)?.name}>{catalogs.data?.organizationRoles.find(item => item.id === record.organizationRoleId)?.name ?? (record.organizationRoleId ? '角色信息不可用' : '未分配')}</Tag> },
    { title: '操作', valueType: 'option', width: 322, fixed: 'right', search: false, render: (_, record) => record.isSuperAdmin ? <Button type="link" icon={<SafetyCertificateOutlined aria-hidden />} onClick={() => setPermissionUser(record)}>权限详情</Button> : <Space size={2} wrap>
      <Button type="link" disabled={!catalogs.data} onClick={() => openEdit(record)}>编辑</Button>
      <Button type="link" icon={<SafetyCertificateOutlined aria-hidden />} onClick={() => setPermissionUser(record)}>权限详情</Button>
      <Tooltip title="重置成功后显示新初始密码，请复制并告知该用户"><Button type="link" onClick={() => setResetting(record)}>重置密码</Button></Tooltip>
      <Popconfirm title={`确认${record.status === 'ACTIVE' ? '禁用' : '启用'}账号“${record.name}”？`} description="该操作会立即使现有登录会话失效。" onConfirm={() => statusMutation.mutate(record)}><Button type="link" danger={record.status === 'ACTIVE'}>{record.status === 'ACTIVE' ? '禁用' : '启用'}</Button></Popconfirm>
    </Space> },
  ]
  const rows = useMemo(() => filterUsers(users.data ?? [], filters), [users.data, filters])
  return <section className="admin-page user-management-page"><div className="page-heading"><div><Typography.Title level={2}>用户与角色</Typography.Title><Typography.Paragraph>维护账号状态、系统角色、部门归属和最终生效权限；禁用、改密与权限敏感变更会使既有会话失效。</Typography.Paragraph></div></div>
    {users.isError && <Alert type="error" showIcon title="用户列表加载失败" description={users.data ? '当前显示上次加载的结果，请重试获取最新信息。' : '无法读取用户信息，请重试。'} action={<Button loading={users.isFetching} onClick={() => void users.refetch()}>重试用户列表</Button>} />}
    {catalogs.isError && <Alert type="error" showIcon title="组织目录加载失败" description="部门和角色信息暂不可用，请重试后编辑归属。" action={<Button loading={catalogs.isFetching} onClick={() => void catalogs.refetch()}>重试组织目录</Button>} />}
    <ProTable<UserAssignment, UserFilterValues>
      className="user-management-table"
      cardBordered
      rowKey="accountId"
      loading={users.isFetching || catalogs.isFetching}
      dataSource={rows}
      columns={columns}
      headerTitle={<Space size={10}><span>用户列表</span>{users.data && !users.isError && <Tag variant="filled" color="blue">共 {rows.length} 名用户</Tag>}</Space>}
      search={{ labelWidth: 'auto' }}
      onSubmit={values => setFilters(values)}
      onReset={() => setFilters({})}
      options={{ reload: () => { void users.refetch(); void catalogs.refetch() }, density: true, setting: true }}
      toolBarRender={() => [<Button key="create" type="primary" icon={<PlusOutlined aria-hidden />} onClick={openCreate}>新增用户</Button>]}
      pagination={{ defaultPageSize: 10, showSizeChanger: true, showQuickJumper: true, showTotal: total => `共 ${total} 条` }}
      scroll={{ x: 1320 }}
      locale={{ emptyText: users.isError ? '用户列表暂不可用' : users.isPending ? '正在加载用户…' : filters.keyword || filters.systemRole || filters.status || filters.departmentId || filters.organizationRoleId ? '没有符合当前筛选条件的用户' : '暂无用户' }}
    />
    <Modal title={`${editing ? '编辑' : '新增'}用户`} open={userModal} onCancel={() => setUserModal(false)} onOk={() => userForm.submit()} confirmLoading={saveUser.isPending} okButtonProps={{ disabled: Boolean(editing) && (catalogs.isError || catalogs.isPending || editVersionConflict) }} destroyOnHidden>
      {editVersionConflict && <Alert type="warning" showIcon title="账号数据已变化，当前输入已保留" description="请关闭弹窗并重新打开最新记录后继续编辑。若提示基本信息已保存，则只需重新打开记录并更新尚未保存的部门或组织角色。" />}
      {editing && catalogs.isError && <Alert type="error" showIcon title="组织目录加载失败，请重试后保存" action={<Button loading={catalogs.isFetching} onClick={() => void catalogs.refetch()}>重试组织目录</Button>} />}
      <Form name="admin-user-editor" key={editing?.accountId ?? 'create'} form={userForm} layout="vertical" initialValues={editing ? { studentNo: editing.studentNo, name: editing.name, systemRole: editing.systemRole, departmentId: editing.departmentId || undefined, organizationRoleId: editing.organizationRoleId || undefined } : { systemRole: 'MEMBER' }} onFinish={values => saveUser.mutate(values)} preserve={false}><Form.Item name="studentNo" label="学号/登录账号" rules={[{ required: true, message: '请输入学号' }]}><Input autoComplete="off" spellCheck={false} /></Form.Item><Form.Item name="name" label="姓名" rules={[{ required: true, message: '请输入姓名' }]}><Input autoComplete="off" /></Form.Item><Form.Item name="systemRole" label="系统角色" rules={[{ required: true }]}><Select options={[{ value: 'MEMBER', label: '成员/普通用户' }, { value: 'ADMIN', label: '管理员' }]} /></Form.Item>{editing && <><Form.Item name="departmentId" label="所属部门"><Select disabled={catalogs.isError || catalogs.isPending} options={departmentOptions} showSearch optionFilterProp="label" placeholder="选择部门" /></Form.Item><Form.Item name="organizationRoleId" label="组织角色"><Select disabled={catalogs.isError || catalogs.isPending} options={organizationRoleOptions} showSearch optionFilterProp="label" placeholder="选择组织角色" /></Form.Item></>}<Typography.Paragraph type="secondary">{editing ? '修改部门或组织角色会更新该用户的权限。' : '系统将生成一次性初始密码，首次登录必须修改。'}</Typography.Paragraph></Form>
    </Modal>
    <Modal title={`重置 ${resetting?.name ?? ''} 的密码`} open={Boolean(resetting)} onCancel={() => setResetting(null)} onOk={() => resetMutation.mutate()} confirmLoading={resetMutation.isPending}><Typography.Paragraph>确认后系统会生成新初始密码，并在下一弹窗显示，供管理员复制并告知该用户。现有会话将失效；用户下次登录必须修改密码。</Typography.Paragraph></Modal>
    <Modal title={issuedPassword?.kind === 'reset' ? '密码重置成功' : '用户创建成功'} open={Boolean(issuedPassword)} onOk={closeIssuedPassword} onCancel={closeIssuedPassword} okText="我已记录并告知用户" cancelButtonProps={{ style: { display: 'none' } }} maskClosable={false} keyboard={false}><Alert type="success" showIcon title={`${issuedPassword?.name ?? ''}的初始密码如下`} description="请将账号和密码通过安全渠道告知本人。该密码仅在此处显示一次，首次登录须修改。" /><div className="issued-password-details"><Typography.Text type="secondary">登录账号</Typography.Text><Typography.Text copyable={{ text: issuedPassword?.loginName }}>{issuedPassword?.loginName}</Typography.Text><Typography.Text type="secondary">初始密码</Typography.Text><Typography.Text code copyable={{ text: issuedPassword?.password }}>{issuedPassword?.password}</Typography.Text></div></Modal>
    {permissionUser && <PermissionDetailDrawer accountId={permissionUser.accountId} userName={permissionUser.name} departmentId={permissionUser.departmentId} onClose={() => setPermissionUser(null)} />}
  </section>
}

function statusLabel(status: string) {
  return ({ ACTIVE: '正常', PENDING: '待审核', DISABLED: '已禁用', REJECTED: '已驳回' } as Record<string, string>)[status] ?? status
}

function filterUsers(users: UserAssignment[], filters: UserFilterValues) {
  const keyword = filters.keyword?.trim().toLocaleLowerCase() ?? ''
  return users.filter(user => {
    const matchesKeyword = !keyword || [user.loginName, user.studentNo, user.name].some(value => value.toLocaleLowerCase().includes(keyword))
    return matchesKeyword
      && (!filters.systemRole || user.systemRole === filters.systemRole)
      && (!filters.status || user.status === filters.status)
      && (!filters.departmentId || user.departmentId === filters.departmentId)
      && (!filters.organizationRoleId || user.organizationRoleId === filters.organizationRoleId)
  })
}

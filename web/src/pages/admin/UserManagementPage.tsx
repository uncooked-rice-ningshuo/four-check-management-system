import { PlusOutlined, SafetyCertificateOutlined } from '@ant-design/icons'
import { ProTable, type ProColumns } from '@ant-design/pro-components'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Button, Form, Input, Modal, Popconfirm, Select, Space, Tag, Typography, message } from 'antd'
import { useMemo, useState } from 'react'
import type { UserAssignment } from '../../entities/organization/model/organization'
import { assignUserRole, createUser, getCatalogs, getUsers, resetUserPassword, updateUser, updateUserStatus } from '../../shared/api/adminApi'
import { PermissionDetailDrawer } from './PermissionDetailPage'

type UserFormValues = { studentNo: string; name: string; systemRole: 'ADMIN' | 'MEMBER'; temporaryPassword?: string }
type UserFilterValues = {
  keyword?: string
  systemRole?: UserAssignment['systemRole']
  status?: string
  departmentId?: string
  organizationRoleId?: string
}

export function UserManagementPage() {
  const [drafts, setDrafts] = useState<Record<string, Pick<UserAssignment, 'departmentId' | 'organizationRoleId'>>>({})
  const [filters, setFilters] = useState<UserFilterValues>({})
  const [editing, setEditing] = useState<UserAssignment | null>(null)
  const [userModal, setUserModal] = useState(false)
  const [resetting, setResetting] = useState<UserAssignment | null>(null)
  const [permissionUser, setPermissionUser] = useState<UserAssignment | null>(null)
  const [userForm] = Form.useForm<UserFormValues>()
  const [passwordForm] = Form.useForm<{ temporaryPassword: string }>()
  const client = useQueryClient()
  const users = useQuery({ queryKey: ['admin-users'], queryFn: getUsers })
  const catalogs = useQuery({ queryKey: ['admin-catalogs'], queryFn: getCatalogs })
  const refresh = () => client.invalidateQueries({ queryKey: ['admin-users'] })
  const saveAssignment = useMutation({ mutationFn: assignUserRole, onSuccess: async (_, assignment) => { await refresh(); setDrafts(current => { const next = { ...current }; delete next[assignment.accountId]; return next }); message.success('用户归属已更新') }, onError: error => message.error(error.message) })
  const saveUser = useMutation({ mutationFn: (values: UserFormValues) => editing ? updateUser(editing.accountId, values) : createUser({ ...values, temporaryPassword: values.temporaryPassword ?? '' }), onSuccess: async () => { await refresh(); setUserModal(false); setEditing(null); userForm.resetFields(); message.success('用户信息已保存') }, onError: error => message.error(error.message) })
  const statusMutation = useMutation({ mutationFn: (record: UserAssignment) => updateUserStatus(record.accountId, record.status === 'ACTIVE' ? 'DISABLED' : 'ACTIVE'), onSuccess: async () => { await refresh(); message.success('账号状态已更新，既有会话已失效') }, onError: error => message.error(error.message) })
  const resetMutation = useMutation({ mutationFn: (values: { temporaryPassword: string }) => resetUserPassword(resetting!.accountId, values.temporaryPassword), onSuccess: () => { setResetting(null); passwordForm.resetFields(); message.success('临时密码已设置，用户下次登录必须修改密码') }, onError: error => message.error(error.message) })
  const updateDraft = (record: UserAssignment, field: 'departmentId' | 'organizationRoleId', value: string) => setDrafts(current => ({ ...current, [record.accountId]: { departmentId: current[record.accountId]?.departmentId ?? record.departmentId, organizationRoleId: current[record.accountId]?.organizationRoleId ?? record.organizationRoleId, [field]: value } }))
  const openCreate = () => { setEditing(null); userForm.setFieldsValue({ studentNo: '', name: '', systemRole: 'MEMBER', temporaryPassword: '' }); setUserModal(true) }
  const openEdit = (record: UserAssignment) => { setEditing(record); userForm.setFieldsValue({ studentNo: record.studentNo, name: record.name, systemRole: record.systemRole === 'ADMIN' ? 'ADMIN' : 'MEMBER' }); setUserModal(true) }
  const departmentOptions = catalogs.data?.departments.filter(item => item.enabled).map(item => ({ value: item.id, label: item.name })) ?? []
  const organizationRoleOptions = catalogs.data?.organizationRoles.filter(item => item.enabled).map(item => ({ value: item.id, label: item.name })) ?? []
  const columns: ProColumns<UserAssignment>[] = [
    { title: '关键字', dataIndex: 'keyword', hideInTable: true, order: 5, fieldProps: { allowClear: true, autoComplete: 'off', placeholder: '输入账号、学号或姓名…', spellCheck: false } },
    { title: '账号', dataIndex: 'loginName', width: 138, search: false, copyable: true, ellipsis: true },
    { title: '学号', dataIndex: 'studentNo', width: 138, search: false, copyable: true, ellipsis: true, renderText: value => value || '—' },
    { title: '姓名', dataIndex: 'name', width: 104, search: false, ellipsis: true },
    { title: '系统角色', dataIndex: 'systemRole', width: 126, order: 4, valueType: 'select', valueEnum: { SUPER_ADMIN: { text: '超级管理员' }, ADMIN: { text: '管理员' }, MEMBER: { text: '成员/普通用户' } }, fieldProps: { placeholder: '全部系统角色', allowClear: true }, render: (_, record) => <Tag color={record.isSuperAdmin ? 'gold' : record.systemRole === 'ADMIN' ? 'blue' : 'default'}>{record.isSuperAdmin ? '超级管理员' : record.systemRole === 'ADMIN' ? '管理员' : '普通用户'}</Tag> },
    { title: '账号状态', dataIndex: 'status', width: 110, order: 3, valueType: 'select', valueEnum: { ACTIVE: { text: '正常', status: 'Success' }, PENDING: { text: '待审核', status: 'Warning' }, DISABLED: { text: '已禁用', status: 'Default' }, REJECTED: { text: '已驳回', status: 'Error' } }, fieldProps: { placeholder: '全部状态', allowClear: true }, render: (_, record) => <Space orientation="vertical" size={2}><Tag color={record.status === 'ACTIVE' ? 'green' : record.status === 'PENDING' ? 'gold' : record.status === 'REJECTED' ? 'red' : 'default'}>{statusLabel(record.status)}</Tag>{record.mustChangePassword && <Typography.Text type="warning" className="password-state">待改密</Typography.Text>}</Space> },
    { title: '所属部门', dataIndex: 'departmentId', width: 174, order: 2, valueType: 'select', fieldProps: { options: departmentOptions, placeholder: '全部部门', allowClear: true, showSearch: true, optionFilterProp: 'label' }, render: (_, record) => <Select aria-label={`${record.name}所属部门`} disabled={record.isSuperAdmin} value={(drafts[record.accountId]?.departmentId ?? record.departmentId) || undefined} placeholder="未分配" style={{ width: 160 }} options={departmentOptions} onChange={value => updateDraft(record, 'departmentId', value)} /> },
    { title: '组织角色', dataIndex: 'organizationRoleId', width: 174, order: 1, valueType: 'select', fieldProps: { options: organizationRoleOptions, placeholder: '全部组织角色', allowClear: true, showSearch: true, optionFilterProp: 'label' }, render: (_, record) => <Select aria-label={`${record.name}组织角色`} disabled={record.isSuperAdmin} value={(drafts[record.accountId]?.organizationRoleId ?? record.organizationRoleId) || undefined} placeholder="未分配" style={{ width: 160 }} options={organizationRoleOptions} onChange={value => updateDraft(record, 'organizationRoleId', value)} /> },
    { title: '操作', valueType: 'option', width: 360, fixed: 'right', search: false, render: (_, record) => record.isSuperAdmin ? <Button type="link" icon={<SafetyCertificateOutlined aria-hidden />} onClick={() => setPermissionUser(record)}>权限详情</Button> : <Space size={2} wrap>
      <Button type="link" onClick={() => openEdit(record)}>编辑</Button>
      <Button type="link" disabled={!hasAssignmentChanges(record, drafts[record.accountId])} loading={saveAssignment.isPending} onClick={() => saveAssignment.mutate({ ...record, ...drafts[record.accountId] })}>保存归属</Button>
      <Button type="link" icon={<SafetyCertificateOutlined aria-hidden />} onClick={() => setPermissionUser(record)}>权限详情</Button>
      <Button type="link" onClick={() => setResetting(record)}>重置密码</Button>
      <Popconfirm title={`确认${record.status === 'ACTIVE' ? '禁用' : '启用'}账号“${record.name}”？`} description="该操作会立即使现有登录会话失效。" onConfirm={() => statusMutation.mutate(record)}><Button type="link" danger={record.status === 'ACTIVE'}>{record.status === 'ACTIVE' ? '禁用' : '启用'}</Button></Popconfirm>
    </Space> },
  ]
  const rows = useMemo(() => filterUsers(users.data ?? [], filters), [users.data, filters])
  return <section className="admin-page user-management-page"><div className="page-heading"><div><Typography.Title level={2}>用户与角色</Typography.Title><Typography.Paragraph>维护账号状态、系统角色、部门归属和最终生效权限；禁用、改密与权限敏感变更会使既有会话失效。</Typography.Paragraph></div></div>
    <ProTable<UserAssignment, UserFilterValues>
      className="user-management-table"
      cardBordered
      rowKey="accountId"
      loading={users.isLoading || catalogs.isLoading}
      dataSource={rows}
      columns={columns}
      headerTitle={<Space size={10}><span>用户列表</span><Tag variant="filled" color="blue">共 {rows.length} 名用户</Tag></Space>}
      search={{ labelWidth: 'auto' }}
      onSubmit={values => setFilters(values)}
      onReset={() => setFilters({})}
      options={{ reload: () => void users.refetch(), density: true, setting: true }}
      toolBarRender={() => [<Button key="create" type="primary" icon={<PlusOutlined aria-hidden />} onClick={openCreate}>新增用户</Button>]}
      pagination={{ defaultPageSize: 10, showSizeChanger: true, showQuickJumper: true, showTotal: total => `共 ${total} 条` }}
      scroll={{ x: 1450 }}
      locale={{ emptyText: filters.keyword || filters.systemRole || filters.status || filters.departmentId || filters.organizationRoleId ? '没有符合当前筛选条件的用户' : '暂无用户' }}
    />
    <Modal title={`${editing ? '编辑' : '新增'}用户`} open={userModal} onCancel={() => setUserModal(false)} onOk={() => userForm.submit()} confirmLoading={saveUser.isPending} destroyOnHidden>
      <Form form={userForm} layout="vertical" onFinish={values => saveUser.mutate(values)} preserve={false}><Form.Item name="studentNo" label="学号/登录账号" rules={[{ required: true, message: '请输入学号' }]}><Input autoComplete="off" spellCheck={false} /></Form.Item><Form.Item name="name" label="姓名" rules={[{ required: true, message: '请输入姓名' }]}><Input autoComplete="off" /></Form.Item><Form.Item name="systemRole" label="系统角色" rules={[{ required: true }]}><Select options={[{ value: 'MEMBER', label: '成员/普通用户' }, { value: 'ADMIN', label: '管理员' }]} /></Form.Item>{!editing && <Form.Item name="temporaryPassword" label="临时密码" rules={[{ required: true, message: '请输入临时密码' }, { min: 12, message: '临时密码至少 12 位' }]} extra="用户首次登录后必须修改临时密码。"><Input.Password autoComplete="new-password" /></Form.Item>}</Form>
    </Modal>
    <Modal title={`重置 ${resetting?.name ?? ''} 的密码`} open={Boolean(resetting)} onCancel={() => setResetting(null)} onOk={() => passwordForm.submit()} confirmLoading={resetMutation.isPending} destroyOnHidden><Form form={passwordForm} layout="vertical" onFinish={values => resetMutation.mutate(values)} preserve={false}><Form.Item name="temporaryPassword" label="新临时密码" rules={[{ required: true, message: '请输入临时密码' }, { min: 12, message: '临时密码至少 12 位' }]}><Input.Password autoComplete="new-password" /></Form.Item></Form></Modal>
    {permissionUser && <PermissionDetailDrawer accountId={permissionUser.accountId} userName={permissionUser.name} onClose={() => setPermissionUser(null)} />}
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

function hasAssignmentChanges(record: UserAssignment, draft?: Pick<UserAssignment, 'departmentId' | 'organizationRoleId'>) {
  return Boolean(draft?.departmentId && draft.organizationRoleId && (draft.departmentId !== record.departmentId || draft.organizationRoleId !== record.organizationRoleId))
}

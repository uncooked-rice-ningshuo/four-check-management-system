import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ProTable, type ProColumns } from '@ant-design/pro-components'
import { App, Button, Empty, Input, Modal, Space, Tag, Typography } from 'antd'
import { useMemo, useState } from 'react'
import type { User } from '../../entities/user/model/user'
import { getCatalogs, getPendingRegistrations, reviewRegistration } from '../../shared/api/adminApi'

type RegistrationFilters = {
  keyword?: string
  requestedDepartmentId?: string
  requestedOrgRoleId?: string
}

export function RegistrationReviewPage() {
  const { message } = App.useApp()
  const [rejecting, setRejecting] = useState<User | null>(null)
  const [reason, setReason] = useState('')
  const [filters, setFilters] = useState<RegistrationFilters>({})
  const client = useQueryClient()
  const requests = useQuery({ queryKey: ['registration-requests'], queryFn: getPendingRegistrations })
  const catalogs = useQuery({ queryKey: ['admin-catalogs'], queryFn: getCatalogs })
  const review = useMutation({ mutationFn: ({ id, decision, reason = '' }: { id: string; decision: 'APPROVE' | 'REJECT'; reason?: string }) => reviewRegistration(id, decision, reason), onSuccess: async () => { await client.invalidateQueries({ queryKey: ['registration-requests'] }); setRejecting(null); setReason(''); message.success('审核结果已保存') }, onError: error => message.error(error.message) })
  const departmentName = (id: string) => catalogs.data?.departments.find(item => item.id === id)?.name ?? '—'
  const roleName = (id: string) => catalogs.data?.organizationRoles.find(item => item.id === id)?.name ?? '—'
  const departmentOptions = catalogs.data?.departments.filter(item => item.enabled).map(item => ({ value: item.id, label: item.name })) ?? []
  const roleOptions = catalogs.data?.organizationRoles.filter(item => item.enabled).map(item => ({ value: item.id, label: item.name })) ?? []
  const columns: ProColumns<User>[] = [
    { title: '关键字', dataIndex: 'keyword', hideInTable: true, order: 3, fieldProps: { allowClear: true, autoComplete: 'off', placeholder: '输入学号或姓名…', spellCheck: false } },
    { title: '学号', dataIndex: 'studentNo', width: 150, search: false, copyable: true, ellipsis: true },
    { title: '姓名', dataIndex: 'name', width: 120, search: false, ellipsis: true },
    { title: '申请部门', dataIndex: 'requestedDepartmentId', order: 2, valueType: 'select', fieldProps: { options: departmentOptions, allowClear: true, showSearch: true, optionFilterProp: 'label', placeholder: '全部部门' }, renderText: departmentName },
    { title: '申请角色', dataIndex: 'requestedOrgRoleId', order: 1, valueType: 'select', fieldProps: { options: roleOptions, allowClear: true, showSearch: true, optionFilterProp: 'label', placeholder: '全部申请角色' }, renderText: roleName },
    { title: '操作', valueType: 'option', width: 150, search: false, render: (_, record) => <Space><Button type="primary" size="small" loading={review.isPending} onClick={() => review.mutate({ id: record.id, decision: 'APPROVE' })}>通过</Button><Button danger size="small" onClick={() => setRejecting(record)}>驳回</Button></Space> },
  ]
  const rows = useMemo(() => filterRegistrations(requests.data ?? [], filters), [requests.data, filters])
  const hasFilters = Boolean(filters.keyword || filters.requestedDepartmentId || filters.requestedOrgRoleId)
  return <section className="admin-page"><div className="page-heading"><div><Typography.Title level={2}>注册申请</Typography.Title><Typography.Paragraph>审核通过后，账号才能登录并获得申请的部门与组织角色。</Typography.Paragraph></div></div>
    <ProTable<User, RegistrationFilters>
      rowKey="id"
      cardBordered
      loading={requests.isLoading || catalogs.isLoading}
      dataSource={rows}
      columns={columns}
      headerTitle={<Space size={10}><span>申请列表</span><Tag variant="filled" color="blue">待处理 {rows.length}</Tag></Space>}
      search={{ labelWidth: 'auto', defaultCollapsed: false, collapseRender: false }}
      onSubmit={values => setFilters(values)}
      onReset={() => setFilters({})}
      options={{ reload: () => void requests.refetch(), density: true, setting: true }}
      pagination={{ defaultPageSize: 10, showSizeChanger: true, showTotal: total => `共 ${total} 条` }}
      locale={{ emptyText: <Empty description={hasFilters ? '没有符合当前筛选条件的申请' : '暂无待审核申请'} /> }}
    />
    <Modal title={`驳回 ${rejecting?.name ?? ''} 的申请`} open={Boolean(rejecting)} onCancel={() => setRejecting(null)} okButtonProps={{ danger: true, disabled: !reason.trim(), loading: review.isPending }} onOk={() => rejecting && review.mutate({ id: rejecting.id, decision: 'REJECT', reason })}><Input.TextArea value={reason} onChange={event => setReason(event.target.value)} placeholder="请填写驳回原因…" rows={4} /></Modal>
  </section>
}

function filterRegistrations(requests: User[], filters: RegistrationFilters) {
  const keyword = filters.keyword?.trim().toLocaleLowerCase() ?? ''
  return requests.filter(request => {
    const matchesKeyword = !keyword || [request.studentNo, request.name].some(value => value.toLocaleLowerCase().includes(keyword))
    return matchesKeyword
      && (!filters.requestedDepartmentId || request.requestedDepartmentId === filters.requestedDepartmentId)
      && (!filters.requestedOrgRoleId || request.requestedOrgRoleId === filters.requestedOrgRoleId)
  })
}

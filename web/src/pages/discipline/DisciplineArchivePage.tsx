import { useState } from 'react'
import { Alert, Button, Drawer, Descriptions, Table, Typography, Tag, Space, Empty } from 'antd'
import { ProTable } from '@ant-design/pro-components'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useAttendancePermissions } from '../../features/attendance/hooks/useAttendancePermissions'
import { hasNavigableInspectionAction } from '../../features/attendance/hooks/hasNavigableInspectionAction'
import { exportArchivedDecisions, searchArchivedDecisions, type ArchivedDecision, type DisciplineArchiveFilter } from '../../shared/api/disciplineArchiveApi'

const decisionSources: Record<string, string> = {
  TEACHER: '人工确认', AUTHORIZED_USER: '人工确认', SUPER_ADMIN: '超管确认',
  TIMEOUT: '到期自动确认', RULE: '规则直接确定',
}

export function DisciplineArchivePage() {
  const [filter, setFilter] = useState<DisciplineArchiveFilter>({ page: 1, pageSize: 20 })
  const [selected, select] = useState<string>()
  const permissions = useAttendancePermissions()
  const canExport = hasNavigableInspectionAction(permissions.data?.permissions, 'DISCIPLINE_ARCHIVE_EXPORT')
  const download = useMutation({mutationFn: () => exportArchivedDecisions(filter), onSuccess: blob => {
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = '处分归档.xlsx'
    link.click()
    window.setTimeout(() => URL.revokeObjectURL(url),60000)
  }})
  const detail = useQuery({queryKey:['discipline-archive-detail',selected], enabled:!!selected,
    queryFn:() => searchArchivedDecisions({caseId:selected,page:1,pageSize:1})})
  const item = detail.data?.items[0]
  const query = useQuery({
    queryKey: ['discipline-archive', filter],
    queryFn: () => searchArchivedDecisions(filter),
  })
  return <section className="attendance-page" aria-labelledby="discipline-archive-title">
    <div className="attendance-page-heading"><Typography.Title id="discipline-archive-title" level={2}>处分归档</Typography.Title></div>
    {download.isError && <Alert type="error" title="导出失败，请重试" />}
    {query.isError
      ? <Alert type="error" title="处分归档读取失败" action={<Button onClick={() => void query.refetch()}>重试</Button>} />
      : <ProTable<ArchivedDecision, DisciplineArchiveFilter>
          className="attendance-records-table" cardBordered scroll={{ x: 'max-content' }}
          headerTitle={<Space size={10}>归档记录{query.data && <Tag color="blue">共 {query.data.total} 条</Tag>}</Space>}
          locale={{emptyText:<Empty description="当前范围暂无处分归档记录" />}}
          rowKey="id" dataSource={query.data?.items} loading={query.isFetching}
          search={{ labelWidth: 'auto', defaultCollapsed: false }} options={false}
          onSubmit={values => setFilter({ ...values, page: 1, pageSize: filter.pageSize })}
          onReset={() => setFilter({ page: 1, pageSize: filter.pageSize })}
          pagination={{ current: filter.page, pageSize: filter.pageSize, total: query.data?.total,
            onChange: (page, pageSize) => setFilter({ ...filter, page: pageSize === filter.pageSize ? page : 1, pageSize }) }}
          toolBarRender={() => canExport ? [<Button key="export" loading={download.isPending} onClick={() => download.mutate()}>导出当前结果</Button>] : []}
          columns={[
            { title: '学号', dataIndex: 'student', render: (_, row) => row.studentNumber },
            { title: '学期', dataIndex: 'semester', search: false },
            { title: '处分档位', dataIndex: 'tier', search: false },
            { title: '累计值', dataIndex: 'amount', search: false },
            { title: '状态', dataIndex: 'status', valueType: 'select', valueEnum: { APPROVED: '已确定', VOID: '已作废' } },
            { title: '确定方式', search: false, render: (_, row) => decisionSources[row.decisionSource ?? ''] ?? '—' },
            { title: '确定时间', search: false, render: (_, row) => row.decidedAt
              ? new Date(row.decidedAt).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai' }) : '—' },
            { title:'操作',valueType:'option',render:(_,row) => <Button type="link" onClick={() => select(row.id)}>详情</Button> },
          ]}
        />}
    <Drawer open={!!selected} title="处分归档详情" onClose={() => select(undefined)} loading={detail.isLoading}>
      {detail.isError ? <Alert type="error" title="读取详情失败" action={<Button onClick={() => void detail.refetch()}>重试</Button>} />
        : item ? <><Descriptions column={1} items={[
          {key:'student',label:'学号',children:item.studentNumber},{key:'tier',label:'处分档位',children:item.tier},
          {key:'basis',label:'依据条款',children:item.clause || '无'},{key:'reason',label:'延迟原因',children:item.reason || '无'},
        ]} /><Table rowKey="record" size="small" dataSource={item.sources} pagination={{pageSize:8}} columns={[
          {title:'来源周次',dataIndex:'week'},{title:'定稿版本',dataIndex:'version'},{title:'累计贡献',dataIndex:'amount'},
        ]} /></> : null}
    </Drawer>
  </section>
}

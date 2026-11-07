import { useCallback, useEffect, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { ArrowLeftOutlined } from '@ant-design/icons'
import { ProTable, type ProColumns } from '@ant-design/pro-components'
import { Alert, Button, Card, Descriptions, Drawer, Empty, Space, Table, Tag, Timeline, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { useNavigate } from 'react-router-dom'
import type { ReportArchiveEntry, ReportArchiveQuery, ReportArchiveStatus, ReportCorrectionLog, ReportReview, ReportVersion, ReportWorkflow } from '../../entities/reporting/model/reporting'
import { useReportArchive, useReportArchiveWorkflow } from '../../features/reporting/model/queries'
import { reportCorrectionFieldLabel, reportReviewActionLabel, reportStatusLabel } from '../../features/reporting/model/labels'
import { ReportVersionSnapshots } from '../../features/reporting/ui/ReportVersionSnapshots'


function displayTime(value?: string) {
  if (!value) return '—'
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? value : new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(parsed)
}

function displayPeriod(entry: ReportArchiveEntry) {
  const { period } = entry
  return `${period.academicYearName} · ${period.semesterName} · 第 ${period.weekNo} 周`
}

function displayCorrectionValue(value: string | number | null) {
  return value === null || value === '' ? '（空）' : String(value)
}

function ArchiveWorkflowDetail({ workflow, period, drawerWidth, onAccessDenied }: {
  workflow: ReportWorkflow
  period: ReportArchiveEntry['period']
  drawerWidth: number
  onAccessDenied: () => void
}) {
  const [selectedVersion, setSelectedVersion] = useState<ReportVersion | null>(null)
  const current = workflow.report
  const finalizedVersionNumbers = new Set(workflow.reviews.filter(item => item.action === 'FINALIZED').map(item => item.versionNo))
  const showVersionDetails = (versionNo: number) => {
    const version = workflow.versions.find(item => item.versionNo === versionNo)
    if (version) setSelectedVersion(version)
  }
  const versionColumns: ColumnsType<ReportVersion> = [
    { title: '版本', dataIndex: 'versionNo', width: 124, render: (value: number) => <Space size={6}>
      <Typography.Text strong>V{value}</Typography.Text>
      {value === current?.versionNo && <Tag color="blue">当前</Tag>}
    </Space> },
    { title: '确认记录数', key: 'count', align: 'right', render: (_value, item) => item.content.confirmedRecordCount },
    { title: '数据单元', key: 'cells', align: 'right', render: (_value, item) => item.content.rows.length },
    { title: '生成时间', key: 'createdAt', render: (_value, item) => displayTime(item.createdAt) },
    { title: '范围', key: 'scope', render: (_value, item) => item.content.scopeLimited ? <Tag color="orange">授权范围快照</Tag> : <Tag>完整可见范围</Tag> },
    { title: '文件输出', key: 'outputs', width: 210, render: (_value, item) => <Space wrap>
      <Button size="small" disabled title="Word 文件生成功能后续开放">{finalizedVersionNumbers.has(item.versionNo) ? '生成定稿 Word' : '生成草稿 Word'}</Button>
      <Button size="small" disabled title="Excel 文件生成功能后续开放">{finalizedVersionNumbers.has(item.versionNo) ? '生成定稿 Excel' : '生成草稿 Excel'}</Button>
    </Space> },
    { title: '数据明细', key: 'snapshot', width: 120, align: 'right', render: (_value, item) => <Button type="link" size="small"
      aria-expanded={selectedVersion?.id === item.id} onClick={() => showVersionDetails(item.versionNo)}>
      查看明细
    </Button> },
  ]
  const correctionColumns: ColumnsType<ReportCorrectionLog> = [
    { title: '版本', dataIndex: 'versionNo', width: 130, render: value => <Button type="link" size="small"
      aria-label={`查看 V${value} 数据明细`} onClick={() => showVersionDetails(value)}>V{value} · 数据明细</Button> },
    { title: '字段', dataIndex: 'field', width: 180, render: reportCorrectionFieldLabel },
    { title: '原值', dataIndex: 'beforeValue', width: 160, render: displayCorrectionValue },
    { title: '修正后', dataIndex: 'afterValue', width: 160, render: displayCorrectionValue },
    { title: '原因', dataIndex: 'reason' },
    { title: '修正时间', dataIndex: 'correctedAt', width: 170, render: displayTime },
  ]

  return <Space orientation="vertical" size="middle" style={{ display: 'flex' }}>
    {current && <Descriptions size="small" column={{ xs: 1, sm: 2, lg: 4 }}>
      <Descriptions.Item label="当前状态">{reportStatusLabel(current.status)}</Descriptions.Item>
      <Descriptions.Item label="当前版本">V{current.versionNo}</Descriptions.Item>
      <Descriptions.Item label="最后更新">{displayTime(current.updatedAt)}</Descriptions.Item>
    </Descriptions>}
    {workflow.versions.map(version => version.content.scopeLimited && <Alert key={version.id} type="warning" showIcon title={`V${version.versionNo} 只覆盖生成账号当时有权查看的范围`} />)}
    {workflow.reviews.length > 0 && <>
      <Card className="report-archive-review-card" size="small" title="审核与状态轨迹" extra={<Tag>{workflow.reviews.length} 次操作</Tag>}>
        <Timeline items={workflow.reviews.map((item: ReportReview) => ({
          color: item.action === 'RETURNED' ? 'orange' : ['APPROVED', 'FINALIZED', 'PUBLISHED', 'ARCHIVED'].includes(item.action) ? 'green' : 'blue',
          content: <div className="report-archive-review-entry">
            <div className="report-archive-review-entry-heading">
              <Typography.Text strong>{reportReviewActionLabel(item.action)}</Typography.Text>
              <Tag variant="filled">V{item.versionNo}</Tag>
            </div>
            <Typography.Text type="secondary">{displayTime(item.createdAt)}</Typography.Text>
            {item.comment && <p className="report-archive-review-comment">{item.comment}</p>}
          </div>,
        }))} />
      </Card>
    </>}
    <section className="report-archive-version-section" aria-labelledby="report-archive-versions-title">
      <div className="report-archive-section-heading">
        <Typography.Title id="report-archive-versions-title" level={5}>版本快照</Typography.Title>
        <Typography.Text type="secondary">选择版本查看当时保存的记录内容。</Typography.Text>
      </div>
      <Table<ReportVersion> className="report-archive-version-table" rowKey="id" size="small" pagination={false} columns={versionColumns}
        dataSource={workflow.versions} rowClassName={item => item.versionNo === current?.versionNo ? 'report-archive-version-current-row' : ''}
        locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="该周次尚无可查看的版本快照" /> }} scroll={{ x: 900 }} />
    </section>
    {workflow.corrections.length > 0 && <>
      <Typography.Text strong>追加修正记录</Typography.Text>
      <Table<ReportCorrectionLog> rowKey="id" size="small" pagination={false} columns={correctionColumns} dataSource={workflow.corrections} scroll={{ x: 760 }} />
    </>}
    <Alert type="info" showIcon title="归档快照为只读记录" description="Word/Excel 控件已按版本预留，文件生成功能后续开放；归档中的生成操作不会修改历史数据。" />
    <Drawer rootClassName="weekly-report-version-detail-drawer" classNames={{ wrapper: 'weekly-report-version-detail-drawer-wrapper' }}
      styles={{ wrapper: { maxWidth: '100vw' } }}
      title="归档版本明细"
      size={Math.min(960, drawerWidth)} open={Boolean(selectedVersion)} onClose={() => setSelectedVersion(null)}>
      {selectedVersion && <Space className="report-version-detail-layout" orientation="vertical" size="middle" style={{ display: 'flex' }}>
        <Button className="report-version-detail-back" type="link" icon={<ArrowLeftOutlined />} aria-label="返回历史版本" onClick={() => setSelectedVersion(null)}>
          返回历史版本
        </Button>
        <section className="report-version-detail-context" role="group" aria-label="归档版本上下文">
          <div className="report-version-detail-context-heading">
            <div>
              <Typography.Text type="secondary">正在查看归档快照</Typography.Text>
              <Typography.Title level={4}>V{selectedVersion.versionNo} · 数据明细</Typography.Title>
            </div>
            {selectedVersion.versionNo === current?.versionNo && <Tag color="blue">当前版本</Tag>}
          </div>
          <Typography.Text className="report-version-detail-period">
            {period.academicYearName} · {period.semesterName} · 第 {period.weekNo} 周
          </Typography.Text>
          <div className="report-version-detail-metadata">
            <div><Typography.Text type="secondary">确认记录</Typography.Text><Typography.Text strong>{selectedVersion.content.confirmedRecordCount} 条</Typography.Text></div>
            <div><Typography.Text type="secondary">数据单元</Typography.Text><Typography.Text strong>{selectedVersion.content.rows.length} 个</Typography.Text></div>
            <div><Typography.Text type="secondary">生成时间</Typography.Text><Typography.Text strong>{displayTime(selectedVersion.createdAt)}</Typography.Text></div>
            <div><Typography.Text type="secondary">数据范围</Typography.Text><Typography.Text strong>{selectedVersion.content.scopeLimited ? '授权范围快照' : '完整可见范围'}</Typography.Text></div>
          </div>
          <Typography.Text className="report-version-detail-readonly" type="secondary">此处展示版本生成时保存的只读内容；关闭明细或点击“返回历史版本”可回到版本列表。</Typography.Text>
        </section>
        <ReportVersionSnapshots key={selectedVersion.id} period={period} version={selectedVersion} onAccessDenied={onAccessDenied} />
      </Space>}
    </Drawer>
  </Space>
}

export function WeeklyReportArchivePage() {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const [archiveDrawerWidth, setArchiveDrawerWidth] = useState(1040)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [status, setStatus] = useState<ReportArchiveStatus | ''>('')
  const [selected, setSelected] = useState<ReportArchiveEntry | null>(null)
  const [accessChanged, setAccessChanged] = useState(false)
  useEffect(() => {
    const updateArchiveDrawerWidth = () => setArchiveDrawerWidth(Math.min(1040, window.innerWidth))
    updateArchiveDrawerWidth()
    window.addEventListener('resize', updateArchiveDrawerWidth)
    return () => window.removeEventListener('resize', updateArchiveDrawerWidth)
  }, [])
  const query: ReportArchiveQuery = { page, pageSize, ...(status ? { status } : {}) }

  const clearArchiveScope = useCallback(() => {
    queryClient.removeQueries({ queryKey: ['reports', 'archive'] })
    queryClient.removeQueries({ queryKey: ['reports', 'archive-workflow'] })
    setSelected(null)
    setAccessChanged(true)
  }, [queryClient])
  const archiveQuery = useReportArchive(query, clearArchiveScope)
  const workflowQuery = useReportArchiveWorkflow(selected?.period.semesterId ?? '', selected?.period.weekId ?? '', Boolean(selected), clearArchiveScope)

  const columns: ProColumns<ReportArchiveEntry>[] = [
    { title: '通报状态', dataIndex: 'status', hideInTable: true, valueType: 'select', order: 1,
      fieldProps: { options: [
        { value: 'DRAFT', label: '草稿' }, { value: 'PENDING_REVIEW', label: '待审核' },
        { value: 'RETURNED', label: '已退回' }, { value: 'FINALIZING', label: '审核通过，待定稿确认' },
        { value: 'FINALIZED', label: '已定稿' }, { value: 'PUBLISHED', label: '已发布' }, { value: 'ARCHIVED', label: '已归档' },
      ], allowClear: true, placeholder: '全部历史状态' } },
    { title: '学年 / 学期 / 周次', key: 'period', search: false, render: (_value, item) => displayPeriod(item) },
    { title: '状态', dataIndex: 'status', search: false, width: 110, render: (_value, item) => <Tag color={item.status === 'ARCHIVED' ? 'green' : 'blue'}>{reportStatusLabel(item.status)}</Tag> },
    { title: '当前版本', dataIndex: 'versionNo', search: false, width: 96, render: (_value, item) => `V${item.versionNo}` },
    { title: '版本数', dataIndex: 'versionCount', search: false, width: 80, align: 'right' },
    { title: '确认记录', dataIndex: 'confirmedRecordCount', search: false, width: 100, align: 'right' },
    { title: '零记录回执', dataIndex: 'confirmedZeroReceiptCount', search: false, width: 105, align: 'right' },
    { title: '数据单元', dataIndex: 'cellCount', search: false, width: 90, align: 'right' },
    { title: '状态更新时间', dataIndex: 'updatedAt', search: false, width: 170, render: (_value, item) => displayTime(item.updatedAt) },
    { title: '范围', dataIndex: 'scopeLimited', search: false, width: 130, render: (_value, item) => item.scopeLimited ? <Tag color="orange">局部授权范围</Tag> : <Tag>完整可见范围</Tag> },
    { title: '操作', key: 'action', search: false, width: 230, render: (_value, item) => {
      const isFinalized = ['FINALIZED', 'PUBLISHED', 'ARCHIVED'].includes(item.status)
      const stage = ['DRAFT', 'RETURNED'].includes(item.status) || isFinalized ? 1 : 2
      return <Space wrap>
        <Button type="link" onClick={() => setSelected(item)}>查看版本</Button>
        <Button type="link" onClick={() => navigate(`/reports/workbench?semesterId=${encodeURIComponent(item.period.semesterId)}&weekId=${encodeURIComponent(item.period.weekId)}&stage=${stage}`)}>
          {isFinalized ? '基于定稿修正' : '在工作台继续'}
        </Button>
      </Space>
    } },
  ]

  return <section className="attendance-page weekly-report-archive-page" aria-labelledby="weekly-report-archive-title">
    <div className="attendance-page-heading">
      <div>
        <Typography.Title id="weekly-report-archive-title" level={2}>通报归档</Typography.Title>
        <Typography.Text type="secondary">
        这里只展示已生成过草稿的周通报。可查看不同状态和版本的数据快照；草稿修订与定稿操作在“通报工作台”完成。
        </Typography.Text>
      </div>
    </div>

    {accessChanged && <Alert type="warning" showIcon title="你的归档查看权限已变化，页面已清除旧范围数据。请重新进入后重试。" />}
    {archiveQuery.isError && <Alert type="error" showIcon title="历史通报加载失败" description="请检查网络或权限后重试。" action={<Button onClick={() => void archiveQuery.refetch()}>重试</Button>} />}
    <ProTable<ReportArchiveEntry>
      className="weekly-report-archive-table"
      rowKey="id"
      size="middle"
      cardBordered
      columns={columns}
      dataSource={archiveQuery.data?.items ?? []}
      loading={archiveQuery.isPending || archiveQuery.isFetching}
      locale={{ emptyText: archiveQuery.isError ? '通报归档加载失败' : '暂无生成过草稿的周通报' }}
      scroll={{ x: 1100 }}
      headerTitle={<Space size={10}><span>周通报列表</span><Tag variant="filled" color="blue">共 {archiveQuery.data?.total ?? 0} 周通报</Tag></Space>}
      search={{ labelWidth: 'auto', defaultCollapsed: false, collapseRender: false }}
      onSubmit={values => { setStatus((values.status as ReportArchiveStatus | undefined) ?? ''); setPage(1) }}
      onReset={() => { setStatus(''); setPage(1) }}
      options={{ reload: () => void archiveQuery.refetch(), density: false, setting: false, fullScreen: false }}
      pagination={{ current: page, pageSize, total: archiveQuery.data?.total ?? 0, showSizeChanger: true, pageSizeOptions: [10, 20, 50, 100], showTotal: total => `共 ${total} 周通报`, onChange: (nextPage, nextPageSize) => { if (nextPageSize !== pageSize) { setPageSize(nextPageSize); setPage(1) } else setPage(nextPage) } }}
    />

    <Drawer rootClassName="weekly-report-archive-drawer" classNames={{ wrapper: 'weekly-report-archive-drawer-wrapper' }}
      styles={{ wrapper: { maxWidth: '100vw' } }}
      title={selected ? `${displayPeriod(selected)} · 历史版本` : '历史版本'} size={archiveDrawerWidth} open={Boolean(selected)} onClose={() => setSelected(null)}>
      {workflowQuery.isPending && <div role="status">正在加载通报快照</div>}
      {workflowQuery.isError && <Alert type="error" showIcon title="历史版本读取失败" action={<Button onClick={() => void workflowQuery.refetch()}>重试</Button>} />}
      {workflowQuery.data && selected && <ArchiveWorkflowDetail workflow={workflowQuery.data} period={selected.period} drawerWidth={archiveDrawerWidth} onAccessDenied={clearArchiveScope} />}
    </Drawer>
  </section>
}

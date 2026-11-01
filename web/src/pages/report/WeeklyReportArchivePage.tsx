import { useCallback, useEffect, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Alert, Button, Card, Descriptions, Drawer, Select, Space, Table, Tag, Timeline, Typography } from 'antd'
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
    { title: '版本', dataIndex: 'versionNo', width: 74, render: value => `V${value}` },
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
      <Table<ReportVersion> rowKey="id" size="small" pagination={false} columns={versionColumns} dataSource={workflow.versions} scroll={{ x: 860 }} />
    </section>
    {workflow.corrections.length > 0 && <>
      <Typography.Text strong>追加修正记录</Typography.Text>
      <Table<ReportCorrectionLog> rowKey="id" size="small" pagination={false} columns={correctionColumns} dataSource={workflow.corrections} scroll={{ x: 760 }} />
    </>}
    <Alert type="info" showIcon title="归档快照为只读记录" description="Word/Excel 控件已按版本预留，文件生成功能后续开放；归档中的生成操作不会修改历史数据。" />
    <Drawer rootClassName="weekly-report-version-detail-drawer" classNames={{ wrapper: 'weekly-report-version-detail-drawer-wrapper' }}
      styles={{ wrapper: { maxWidth: '100vw' } }}
      title={selectedVersion ? `V${selectedVersion.versionNo} · 数据明细` : '数据明细'}
      size={Math.min(960, drawerWidth)} open={Boolean(selectedVersion)} onClose={() => setSelectedVersion(null)}>
      {selectedVersion && <ReportVersionSnapshots key={selectedVersion.id} period={period} version={selectedVersion} onAccessDenied={onAccessDenied} />}
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

  const columns: ColumnsType<ReportArchiveEntry> = [
    { title: '学年 / 学期 / 周次', key: 'period', render: (_value, item) => displayPeriod(item) },
    { title: '状态', dataIndex: 'status', width: 110, render: (value: ReportArchiveStatus) => <Tag color={value === 'ARCHIVED' ? 'green' : 'blue'}>{reportStatusLabel(value)}</Tag> },
    { title: '当前版本', dataIndex: 'versionNo', width: 96, render: value => `V${value}` },
    { title: '版本数', dataIndex: 'versionCount', width: 80, align: 'right' },
    { title: '确认记录', dataIndex: 'confirmedRecordCount', width: 100, align: 'right' },
    { title: '零记录回执', dataIndex: 'confirmedZeroReceiptCount', width: 105, align: 'right' },
    { title: '数据单元', dataIndex: 'cellCount', width: 90, align: 'right' },
    { title: '状态更新时间', dataIndex: 'updatedAt', width: 170, render: displayTime },
    { title: '范围', dataIndex: 'scopeLimited', width: 130, render: value => value ? <Tag color="orange">局部授权范围</Tag> : <Tag>完整可见范围</Tag> },
    { title: '操作', key: 'action', width: 230, render: (_value, item) => {
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
    <Space wrap>
      <Select aria-label="归档状态筛选" style={{ minWidth: 190 }} value={status || undefined} placeholder="全部历史状态"
        options={[{ value: '', label: '全部状态' }, { value: 'DRAFT', label: '草稿' }, { value: 'PENDING_REVIEW', label: '待审核' }, { value: 'RETURNED', label: '已退回' }, { value: 'FINALIZING', label: '审核通过，待定稿确认' }, { value: 'FINALIZED', label: '已定稿' }, { value: 'PUBLISHED', label: '已发布' }, { value: 'ARCHIVED', label: '已归档' }]}
        onChange={value => { setStatus(value as ReportArchiveStatus | ''); setPage(1) }} />
      <Button onClick={() => void archiveQuery.refetch()} loading={archiveQuery.isFetching}>刷新归档</Button>
    </Space>
    {archiveQuery.isError && <Alert type="error" showIcon title="历史通报加载失败" description="请检查网络或权限后重试。" action={<Button onClick={() => void archiveQuery.refetch()}>重试</Button>} />}
    <Table<ReportArchiveEntry>
      rowKey="id"
      size="middle"
      columns={columns}
      dataSource={archiveQuery.data?.items ?? []}
      loading={archiveQuery.isPending || archiveQuery.isFetching}
      locale={{ emptyText: archiveQuery.isError ? '通报归档加载失败' : '暂无生成过草稿的周通报' }}
      scroll={{ x: 1100 }}
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

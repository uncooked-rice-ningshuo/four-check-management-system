import { useCallback, useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { ProTable, type ProColumns } from '@ant-design/pro-components'
import { Alert, Button, Drawer, Empty, Modal, Space, Table, Tabs, Tag, Timeline, Typography } from 'antd'
import { BarChartOutlined, HistoryOutlined, LeftOutlined, RightOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import type { ReportArchiveEntry, ReportArchiveQuery, ReportArchiveStatus, ReportCorrectionLog, ReportReview, ReportWorkflow } from '../../entities/reporting/model/reporting'
import { useReportArchive, useReportArchiveWorkflow, useStartReportRevision } from '../../features/reporting/model/queries'
import { reportCorrectionFieldLabel, reportReviewActionLabel, reportStatusLabel } from '../../features/reporting/model/labels'
import { ReportVersionSnapshots } from '../../features/reporting/ui/ReportVersionSnapshots'
import { ReportVersionIndicators } from '../../features/reporting/ui/ReportVersionIndicators'
import { ReportOutputPanel } from '../../features/reporting/ui/ReportOutputPanel'
import { permissionCoversReportTypes } from '../../features/reporting/model/reportPermissions'
import { useAttendancePermissions } from '../../features/attendance/hooks/useAttendancePermissions'


function displayTime(value?: string) {
  if (!value) return '—'
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? value : new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(parsed)
}

function displayTimelineTime(value: string) {
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? value : new Intl.DateTimeFormat('zh-CN', { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(parsed)
}

function displayPeriod(entry: ReportArchiveEntry) {
  const { period } = entry
  const semesterName = period.semesterName.startsWith(period.academicYearName)
    ? period.semesterName.slice(period.academicYearName.length).trim() || period.semesterName
    : period.semesterName
  return `${period.academicYearName} · ${semesterName} · 第 ${period.weekNo} 周`
}

function displayCorrectionValue(value: string | number | null) {
  return value === null || value === '' ? '（空）' : String(value)
}

function ArchiveWorkflowDetail({ workflow, onAccessDenied }: { workflow: ReportWorkflow; onAccessDenied: () => void }) {
  const [searchParams, setSearchParams] = useSearchParams()
  const [indicatorsOpen, setIndicatorsOpen] = useState(false)
  const versionListRef = useRef<HTMLDivElement>(null)
  const permissionQuery = useAttendancePermissions()
  const current = workflow.report
  const requestedVersion = Number(searchParams.get('version'))
  const selectedVersion = workflow.versions.find(version => version.versionNo === requestedVersion)
    ?? workflow.versions.find(version => version.versionNo === current?.versionNo)
    ?? workflow.versions.at(-1)
  useEffect(() => {
    const list = versionListRef.current
    const selected = list?.querySelector<HTMLElement>("[aria-current='step']")
    if (list && selected && typeof list.scrollTo === 'function') {
      list.scrollTo({ left: selected.offsetLeft - list.clientWidth / 2 + selected.clientWidth / 2, behavior: 'smooth' })
    }
  }, [selectedVersion?.id])
  const activeSection = searchParams.get('section') === 'history' ? 'history' : 'snapshot'
  const choose = (versionNo: number, section: 'snapshot' | 'history' = activeSection) => {
    const next = new URLSearchParams(searchParams)
    next.set('version', String(versionNo))
    next.set('section', section)
    setSearchParams(next)
  }
  if (!selectedVersion) return <Empty description="该周次尚无可查看的版本快照" />
  const period = selectedVersion.content.period
  const orderedVersions = [...workflow.versions].sort((left, right) => left.versionNo - right.versionNo)
  const selectedIndex = orderedVersions.findIndex(version => version.id === selectedVersion.id)
  const correctionColumns: ColumnsType<ReportCorrectionLog> = [
    { title: '版本', dataIndex: 'versionNo', width: 130, render: value => <Button type="link" size="small" onClick={() => choose(value, 'snapshot')}>V{value} · 查看快照</Button> },
    { title: '字段', dataIndex: 'field', width: 180, render: reportCorrectionFieldLabel },
    { title: '原值', dataIndex: 'beforeValue', width: 160, render: displayCorrectionValue },
    { title: '修正后', dataIndex: 'afterValue', width: 160, render: displayCorrectionValue },
    { title: '原因', dataIndex: 'reason' },
    { title: '修正时间', dataIndex: 'correctedAt', width: 170, render: displayTime },
  ]
  return <div className="report-archive-detail-layout">
    <nav className="report-archive-version-nav" aria-label="通报版本">
      <Button aria-label="上一版本" icon={<LeftOutlined />} disabled={selectedIndex <= 0} onClick={() => choose(orderedVersions[selectedIndex - 1].versionNo, 'snapshot')} />
      <div className="report-archive-version-track"><div className="report-archive-version-list" ref={versionListRef}>{orderedVersions.map(version => <Button key={version.id}
        className="report-archive-version-option" type="text" aria-current={version.id === selectedVersion.id ? 'step' : undefined}
        title={`V${version.versionNo} · ${displayTime(version.createdAt)}`}
        onClick={() => choose(version.versionNo, 'snapshot')}>
        <span className="report-archive-version-time">{displayTimelineTime(version.createdAt)}</span>
        <span className="report-archive-version-dot" aria-hidden="true" />
        <span className="report-archive-version-name">V{version.versionNo}{version.versionNo === current?.versionNo ? ' · 当前' : ''}</span>
      </Button>)}</div></div>
      <Button aria-label="下一版本" icon={<RightOutlined />} disabled={selectedIndex >= orderedVersions.length - 1} onClick={() => choose(orderedVersions[selectedIndex + 1].versionNo, 'snapshot')} />
    </nav>
    <div className="report-archive-detail-main">
      <header className="report-archive-detail-header">
        <div>
          <Typography.Title level={3}>V{selectedVersion.versionNo} · 通报快照</Typography.Title>
        </div>
        <div className="report-archive-header-actions"><Tag color={selectedVersion.content.scopeLimited ? 'orange' : 'default'}>{selectedVersion.content.scopeLimited ? '授权范围快照' : '完整可见范围'}</Tag>
          <Button icon={<BarChartOutlined aria-hidden="true" />} onClick={() => setIndicatorsOpen(true)}>周指标</Button>
          <Button icon={<HistoryOutlined aria-hidden="true" />} onClick={() => choose(selectedVersion.versionNo, 'history')}>流程与修正记录{workflow.reviews.length + workflow.corrections.length > 0 ? ` (${workflow.reviews.length + workflow.corrections.length})` : ''}</Button>
          <ReportOutputPanel key={selectedVersion.id} version={selectedVersion} semesterId={period.semesterId} weekId={period.weekId}
            canGenerate={permissionCoversReportTypes(permissionQuery.data?.permissions ?? [], 'REPORT_GENERATE', period, selectedVersion.content.rows)} /></div>
      </header>
      {selectedVersion.content.scopeLimited && <Alert type="warning" showIcon title="此版本只覆盖生成账号当时有权查看的范围" />}
      <ReportVersionSnapshots key={selectedVersion.id} period={period} version={selectedVersion} onAccessDenied={onAccessDenied} inlineRecordDetail showIndicators={false} showHeading={false} />
      <Drawer title="流程与修正记录" size={Math.min(1040, typeof window === 'undefined' ? 1040 : window.innerWidth)} open={activeSection === 'history'}
        onClose={() => choose(selectedVersion.versionNo, 'snapshot')} destroyOnHidden>
          <Tabs className="report-archive-history-tabs" items={[
            { key: 'reviews', label: `审核与状态轨迹 (${workflow.reviews.length})`, children: <section aria-label="审核与状态轨迹" className="report-archive-history-panel">
              <Alert type="info" showIcon title="审核通过后仍需确认定稿" description="审核通过仅表示进入待定稿确认；确认定稿后才会固定该版数据。" />
              {workflow.reviews.length ? <Timeline items={workflow.reviews.map((item: ReportReview) => ({
                content: <div><Button type="link" onClick={() => choose(item.versionNo, 'snapshot')}>V{item.versionNo} · {reportReviewActionLabel(item.action)}</Button><Typography.Text type="secondary">{displayTime(item.createdAt)}</Typography.Text>{item.comment && <p>{item.comment}</p>}</div>,
              }))} /> : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无审核记录" />}</section> },
            { key: 'corrections', label: `追加修正记录 (${workflow.corrections.length})`, children: <section aria-label="追加修正记录" className="report-archive-history-panel">
              <Table<ReportCorrectionLog> rowKey="id" size="small" pagination={{ pageSize: 10, showSizeChanger: true, pageSizeOptions: [10, 20, 50] }} columns={correctionColumns} dataSource={workflow.corrections} scroll={{ x: 760 }} locale={{ emptyText: '暂无修正记录' }} /></section> },
          ]} />
      </Drawer>
      <Drawer title={`V${selectedVersion.versionNo} · 周指标`} size={Math.min(1040, typeof window === 'undefined' ? 1040 : window.innerWidth)} open={indicatorsOpen}
        onClose={() => setIndicatorsOpen(false)} destroyOnHidden><ReportVersionIndicators version={selectedVersion} archiveTable /></Drawer>
    </div>
  </div>
}

export function WeeklyReportArchiveDetailPage() {
  const { semesterId = '', weekId = '' } = useParams()
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const [confirmRevision, setConfirmRevision] = useState(false)
  const [revisionError, setRevisionError] = useState('')
  const startRevision = useStartReportRevision()
  const permissionQuery = useAttendancePermissions()
  const [accessChanged, setAccessChanged] = useState(false)
  const clearArchiveScope = useCallback(() => {
    queryClient.removeQueries({ queryKey: ['reports', 'archive'] })
    queryClient.removeQueries({ queryKey: ['reports', 'archive-workflow'] })
    setAccessChanged(true)
  }, [queryClient])
  const workflowQuery = useReportArchiveWorkflow(semesterId, weekId, Boolean(semesterId && weekId), clearArchiveScope)
  const version = workflowQuery.data?.versions.find(item => item.versionNo === workflowQuery.data?.report?.versionNo)
  const period = version?.content.period ?? workflowQuery.data?.versions[0]?.content.period
  const isFinalized = ['FINALIZED', 'PUBLISHED', 'ARCHIVED'].includes(workflowQuery.data?.report?.status ?? '')
  const canRevise = Boolean(permissionQuery.data?.canReviseFinalizedReport)
  const canContinueDraft = permissionQuery.data?.permissions.some(item => item.action === 'REPORT_GENERATE' && item.effect === 'ALLOW') ?? false
  const openWorkbench = () => {
    const target = `/reports/workbench?semesterId=${encodeURIComponent(semesterId)}&weekId=${encodeURIComponent(weekId)}&stage=${isFinalized ? 1 : 2}`
    if (!isFinalized) { navigate(target); return }
    setRevisionError('')
    setConfirmRevision(true)
  }
  return <section className="attendance-page weekly-report-archive-detail-page" aria-label="周通报归档详情">
    <Modal title="基于定稿发起更正？" open={confirmRevision} okText="确认发起" cancelText="取消" confirmLoading={startRevision.isPending} onCancel={() => setConfirmRevision(false)} onOk={async () => {
      const report = workflowQuery.data?.report
      if (!report) return
      try {
        await startRevision.mutateAsync({ reportId: report.id, semesterId, weekId, versionNo: report.versionNo })
        setRevisionError(''); setConfirmRevision(false)
        navigate(`/reports/workbench?semesterId=${encodeURIComponent(semesterId)}&weekId=${encodeURIComponent(weekId)}&stage=1`)
      } catch (error) { setRevisionError(error instanceof Error ? error.message : '发起更正失败，请重试') }
    }}>
      第 {period?.weekNo ?? '—'} 周当前定稿 V{workflowQuery.data?.report?.versionNo ?? '—'} 将保留；确认后会记录发起操作，后续更正创建新版本并经过审核。
      {revisionError && <Alert type="error" showIcon title={revisionError} />}
    </Modal>
    <div className="attendance-page-heading report-archive-page-heading"><div>
      <Button type="link" className="report-archive-back" onClick={() => navigate('/reports/archive')}>返回通报归档</Button>
      <Typography.Title level={2}>{period ? displayPeriod({ period } as ReportArchiveEntry) : '周通报归档详情'}</Typography.Title>
      <Typography.Text type="secondary">只读归档</Typography.Text>
    </div>{period && (isFinalized ? canRevise : (['DRAFT', 'RETURNED'].includes(workflowQuery.data?.report?.status ?? '') && canContinueDraft)) && <Button onClick={openWorkbench}>{isFinalized ? '基于定稿修正' : '在工作台继续'}</Button>}</div>
    {accessChanged && <Alert type="warning" showIcon title="你的归档查看权限已变化，请返回归档重新进入" />}
    {workflowQuery.isPending && <div role="status">正在加载通报快照</div>}
    {workflowQuery.isError && <Alert type="error" showIcon title="历史版本读取失败" action={<Button onClick={() => void workflowQuery.refetch()}>重试</Button>} />}
    {workflowQuery.data && !accessChanged && <ArchiveWorkflowDetail workflow={workflowQuery.data} onAccessDenied={clearArchiveScope} />}
  </section>
}

export function WeeklyReportArchivePage() {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const [confirmRevision, setConfirmRevision] = useState<ReportArchiveEntry | null>(null)
  const [revisionError, setRevisionError] = useState('')
  const startRevision = useStartReportRevision()
  const permissionQuery = useAttendancePermissions()
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [status, setStatus] = useState<ReportArchiveStatus | ''>('')
  const [accessChanged, setAccessChanged] = useState(false)
  const query: ReportArchiveQuery = { page, pageSize, ...(status ? { status } : {}) }

  const clearArchiveScope = useCallback(() => {
    queryClient.removeQueries({ queryKey: ['reports', 'archive'] })
    queryClient.removeQueries({ queryKey: ['reports', 'archive-workflow'] })
    setAccessChanged(true)
  }, [queryClient])
  const archiveQuery = useReportArchive(query, clearArchiveScope)

  const columns: ProColumns<ReportArchiveEntry>[] = [
    { title: '通报状态', dataIndex: 'status', hideInTable: true, valueType: 'select', order: 1,
      fieldProps: { options: [
        { value: 'DRAFT', label: '草稿' }, { value: 'PENDING_REVIEW', label: '待审核' },
        { value: 'RETURNED', label: '已退回' }, { value: 'FINALIZING', label: '审核通过，待定稿确认' },
        { value: 'FINALIZED', label: '已定稿' }, { value: 'PUBLISHED', label: '已发布' }, { value: 'ARCHIVED', label: '已归档' },
      ], allowClear: true, placeholder: '全部历史状态' } },
    { title: '学年 / 学期 / 周次', key: 'period', search: false, width: 340, render: (_value, item) => <span className="report-archive-period">{displayPeriod(item)}</span> },
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
      const canRevise = Boolean(permissionQuery.data?.canReviseFinalizedReport)
      const canContinueDraft = permissionQuery.data?.permissions.some(permission => permission.action === 'REPORT_GENERATE' && permission.effect === 'ALLOW') ?? false
      return <Space wrap>
        <Button type="link" onClick={() => navigate(`/reports/archive/${encodeURIComponent(item.period.semesterId)}/${encodeURIComponent(item.period.weekId)}`)}>查看详情</Button>
        {(isFinalized ? canRevise : (['DRAFT', 'RETURNED'].includes(item.status) && canContinueDraft)) && <Button type="link" onClick={() => {
          const target = `/reports/workbench?semesterId=${encodeURIComponent(item.period.semesterId)}&weekId=${encodeURIComponent(item.period.weekId)}&stage=${stage}`
          if (!isFinalized) { navigate(target); return }
          setRevisionError('')
          setConfirmRevision(item)
        }}>
          {isFinalized ? '基于定稿修正' : '在工作台继续'}
        </Button>}
      </Space>
    } },
  ]

  return <section className="attendance-page weekly-report-archive-page" aria-labelledby="weekly-report-archive-title">
    <Modal title="基于定稿发起更正？" open={confirmRevision !== null} okText="确认发起" cancelText="取消" confirmLoading={startRevision.isPending} onCancel={() => setConfirmRevision(null)} onOk={async () => {
      if (!confirmRevision) return
      try {
        await startRevision.mutateAsync({ reportId: confirmRevision.id, semesterId: confirmRevision.period.semesterId,
          weekId: confirmRevision.period.weekId, versionNo: confirmRevision.versionNo })
        navigate(`/reports/workbench?semesterId=${encodeURIComponent(confirmRevision.period.semesterId)}&weekId=${encodeURIComponent(confirmRevision.period.weekId)}&stage=1`)
        setRevisionError(''); setConfirmRevision(null)
      } catch (error) { setRevisionError(error instanceof Error ? error.message : '发起更正失败，请重试') }
    }}>
      {confirmRevision && `${displayPeriod(confirmRevision)} 当前定稿 V${confirmRevision.versionNo} 将保留；确认后会记录发起操作，后续更正创建新版本并经过审核。`}
      {revisionError && <Alert type="error" showIcon title={revisionError} />}
    </Modal>
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
      scroll={{ x: 1500 }}
      headerTitle={<Space size={10}><span>周通报列表</span><Tag variant="filled" color="blue">共 {archiveQuery.data?.total ?? 0} 周通报</Tag></Space>}
      search={{ labelWidth: 'auto', defaultCollapsed: false, collapseRender: false }}
      onSubmit={values => { setStatus((values.status as ReportArchiveStatus | undefined) ?? ''); setPage(1) }}
      onReset={() => { setStatus(''); setPage(1) }}
      options={{ reload: () => void archiveQuery.refetch(), density: false, setting: false, fullScreen: false }}
      pagination={{ current: page, pageSize, total: archiveQuery.data?.total ?? 0, showSizeChanger: true, pageSizeOptions: [10, 20, 50, 100], showTotal: total => `共 ${total} 周通报`, onChange: (nextPage, nextPageSize) => { if (nextPageSize !== pageSize) { setPageSize(nextPageSize); setPage(1) } else setPage(nextPage) } }}
    />

  </section>
}

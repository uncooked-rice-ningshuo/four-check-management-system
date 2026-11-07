import { absenceSummary } from './recordSummary'
import { useCallback, useMemo, useState } from 'react'
import { InfoCircleOutlined, SyncOutlined, TeamOutlined } from '@ant-design/icons'
import { useQueryClient } from '@tanstack/react-query'
import { Alert, Button, Card, Descriptions, Drawer, Empty, Popconfirm, Select, Skeleton, Space, Steps, Table, Tag, Tooltip, Typography, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { useSearchParams } from 'react-router-dom'
import { useCreateReportObligation, useRemoveReportObligation, useReportObligationOptions, useReportObligations, useReportWorkflow, useReportingPeriods, useReportingRecords, useReportingSummary } from '../../features/reporting/model/queries'
import type { ReportObligation, ReportingPeriod, ReportingRecord, ReportingRecordsQuery, ReportingSummaryRow } from '../../entities/reporting/model/reporting'
import { useWorkContext } from '../../features/work-context/useWorkContext'
import { useAttendancePermissions } from '../../features/attendance/hooks/useAttendancePermissions'
import { WeeklyReportWorkflowPanel } from '../../features/reporting/ui/WeeklyReportWorkflowPanel'
import { ReportVersionRecordFilters, type ReportVersionRecordFiltersValue } from '../../features/reporting/ui/ReportVersionRecordFilters'
import { inspectionResultLabel } from '../../features/reporting/model/labels'
import { groupSummaryRows } from '../../features/reporting/model/groupReportRows'

const collegeTarget = '__college__'

function WeeklyObligationManagement({ period }: { period: ReportingPeriod }) {
  const query = { semesterId: period.semesterId, weekId: period.weekId }
  const optionsQuery = useReportObligationOptions(query, true)
  const obligationsQuery = useReportObligations(query, true)
  const create = useCreateReportObligation(query)
  const remove = useRemoveReportObligation(query)
  const [inspectionTypeId, setInspectionTypeId] = useState('')
  const [target, setTarget] = useState('')
  const [notice, noticeContext] = message.useMessage()
  const options = optionsQuery.data
  const eligibleTargets = options?.departmentScopes.filter(item => item.inspectionTypeId === inspectionTypeId) ?? []
  const targetOptions = inspectionTypeId ? [
    { label: '管理员', value: collegeTarget },
    ...eligibleTargets.map(item => ({ label: item.departmentName, value: item.departmentId })),
  ] : []
  const configured = obligationsQuery.data ?? []
  const columns: ColumnsType<ReportObligation> = [
    { title: '范围', key: 'scope', render: (_value, item) => item.collegeLevel ? '管理员' : item.departmentName },
    { title: '检查类型', dataIndex: 'inspectionTypeName' },
    { title: '适格状态', key: 'eligible', render: (_value, item) => item.eligible ? <Tag color="green">有效</Tag> : <Tag color="orange">需清理</Tag> },
    { title: '操作', key: 'action', render: (_value, item) => <Popconfirm title="移除本周应交项？" description="历史提交和零回执不会删除。" okText="移除" cancelText="取消" onConfirm={() => remove.mutate({ id: item.id, version: item.version }, {
      onError: () => void notice.error('应交项未移除，请刷新后重试'),
    })}>
      <Button type="link" danger loading={remove.isPending}>移除</Button>
    </Popconfirm> },
  ]

  return <Card title="该周应交范围配置" extra={<Typography.Text type="secondary">配置应交项，不改变录入或审核权限</Typography.Text>}>
    {noticeContext}
    <Space orientation="vertical" style={{ display: 'flex' }} size="middle">
      {optionsQuery.isError && <Alert type="error" title="应交配置选项加载失败" action={<Button onClick={() => void optionsQuery.refetch()}>重试</Button>} />}
      {obligationsQuery.isError && <Alert type="error" title="现有应交项加载失败" action={<Button onClick={() => void obligationsQuery.refetch()}>重试</Button>} />}
      <Space wrap>
        <Select aria-label="应交检查类型" placeholder="选择检查类型" style={{ minWidth: 190 }} loading={optionsQuery.isFetching}
          value={inspectionTypeId || undefined} options={options?.inspectionTypes.map(item => ({ label: item.name, value: item.id })) ?? []}
          onChange={value => { setInspectionTypeId(value); setTarget('') }} />
        <Select aria-label="应交部门或管理员范围" placeholder="选择部门或管理员范围" style={{ minWidth: 220 }} disabled={!inspectionTypeId}
          value={target || undefined} options={targetOptions} onChange={setTarget} />
        <Button type="primary" disabled={!inspectionTypeId || !target || optionsQuery.isError} loading={create.isPending}
          onClick={() => {
            const departmentId = target === collegeTarget ? '' : target
            create.mutate({ ...query, departmentId, inspectionTypeId }, {
              onSuccess: () => { setTarget(''); void notice.success('已添加本周应交项') },
              onError: () => void notice.error('应交项未保存，请检查权限或刷新后重试'),
            })
          }}>添加应交项</Button>
      </Space>
      <Table<ReportObligation> rowKey="id" size="small" columns={columns} dataSource={configured}
        loading={obligationsQuery.isPending || obligationsQuery.isFetching}
        locale={{ emptyText: obligationsQuery.isError ? '应交项加载失败' : '本周尚未配置应交项' }} pagination={false} />
    </Space>
  </Card>
}

function displayDate(value: string): string {
  if (!value) return '—'
  if (/^\d{4}-\d{2}-\d{2}$/.test(value)) return value
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return value
  return new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: 'long', day: 'numeric', timeZone: 'UTC' }).format(parsed)
}

function periodLabel(period: ReportingPeriod): string {
  const year = period.academicYearName.replace(/\s|学年/g, '')
  const semester = period.semesterName.replace(/\s|学年/g, '')
  const name = year && semester.includes(year) ? period.semesterName : `${period.academicYearName} · ${period.semesterName}`
  return `${name} · 第 ${period.weekNo} 周`
}

function displayRecordDate(record: ReportingRecord): string {
  const date = record.details.date ? displayDate(record.details.date) : '—'
  // 无机课堂 is a date-level record; its stored clock is a fixed midnight placeholder.
  const time = record.typeCode === 'UNATTENDED_CLASSROOM' ? undefined : record.details.time
  return time ? `${date} ${time}` : date
}

function recordSummary(record: ReportingRecord): string {
  const { details } = record
  const parts: string[] = []
  if (details.result) parts.push(inspectionResultLabel(details.result))
  if (record.typeCode !== 'UNATTENDED_CLASSROOM' && details.period) parts.push(`${details.period}节`)
  const absence = absenceSummary(record)
  if (absence) parts.push(absence)
  if (details.absenceCount !== undefined) parts.push(`缺勤 ${details.absenceCount} 次`)
  if (details.penaltyCount !== undefined && details.penaltyCount !== null) parts.push(`违纪计入 ${details.penaltyCount} 次`)
  if (record.currentCourseName) parts.push(record.currentCourseName)
  if (record.currentClassroomName) parts.push(record.currentClassroomName)
  if (record.currentDormitoryName) parts.push(record.currentDormitoryName)
  if (record.currentAreaName) parts.push(record.currentAreaName)
  if (details.description) parts.push(details.description)
  if (record.currentReasonLabel) parts.push(record.currentReasonLabel)
  if (details.remark) parts.push(details.remark)
  return parts.length > 0 ? parts.join(' · ') : '已确认记录'
}

function recordPeople(record: ReportingRecord): string {
  const names = [
    record.studentName,
    ...(record.responsibleStudents ?? []).map(item => item.label),
    ...(record.responsibleClasses ?? []).map(item => item.label),
    ...(record.responsibleDormitories ?? []).map(item => item.label),
  ].filter(Boolean)
  return [...new Set(names)].join('、') || '—'
}

function requiredCellState(row: ReportingSummaryRow) {
  if (!row.isRequired) return <Typography.Text type="secondary">—</Typography.Text>
  const states = [
    row.zeroRecordPendingReviewBatchCount > 0 && <Tag key="pending">零回执待审核 {row.zeroRecordPendingReviewBatchCount}</Tag>,
    row.zeroRecordReturnedBatchCount > 0 && <Tag key="returned" color="red">零回执已退回 {row.zeroRecordReturnedBatchCount}</Tag>,
    row.zeroRecordConfirmedBatchCount > 0 && <Tag key="confirmed" color="green">零回执已确认 {row.zeroRecordConfirmedBatchCount}</Tag>,
  ].filter(Boolean)
  if (states.length > 0) return <Space wrap><Tag color="gold">应交</Tag>{states}</Space>
  if (row.submissionCount === 0) return <Space wrap><Tag color="gold">应交</Tag><Tag>未提交</Tag></Space>
  return <Space wrap><Tag color="gold">应交</Tag><Tag color="blue">已有记录批次</Tag></Space>
}

function RecordDetailsView({ record }: { record: ReportingRecord }) {
  const { details } = record
  const values: Array<[string, string | number | undefined | null]> = [
    ['提交归属（当前名称）', record.sourceDepartmentName],
    ['提交人（当前姓名）', record.submittedByName],
    ['提交时间', record.submittedAt ? new Date(record.submittedAt).toLocaleString('zh-CN') : undefined],
    ['来源修订', record.sourceRevisionNo ? `第 ${record.sourceRevisionNo} 次提交` : undefined],
    ['学号', record.studentNumber],
    ['年级', record.grade],
    ['检查日期', details.date],
    // 无机课堂 has no clock time, period or hours: it counts 1 次 × 周倍数.
    ['发生时间', record.typeCode === 'UNATTENDED_CLASSROOM' ? undefined : details.time],
    ['节次', record.typeCode === 'UNATTENDED_CLASSROOM' ? undefined : details.period],
    ['检查轮次', details.round],
    [record.typeCode === 'ATTENDANCE_RATE' ? '缺课节次数' : '缺课学时', record.typeCode === 'UNATTENDED_CLASSROOM' ? undefined : details.absenceHours],
    [record.typeCode === 'ATTENDANCE_RATE' ? '违纪计入次数' : '计入学时', record.typeCode === 'UNATTENDED_CLASSROOM' ? undefined : details.penaltyHours],
    ['违纪计入次数', details.penaltyCount],
    ['缺勤次数', details.absenceCount],
    ['检查结果', details.result ? inspectionResultLabel(details.result) : undefined],
    ['课程', record.currentCourseName],
    ['教室', record.currentClassroomName],
    ['宿舍', record.currentDormitoryName],
    ['检查区域', record.currentAreaName],
    ['原因', record.currentReasonLabel || (details.reasonCode ? '原因标签不可用' : undefined)],
    ['责任人', record.responsibleStudents?.map(item => item.label).join('、')],
    ['责任班级', record.responsibleClasses?.map(item => item.label).join('、')],
    ['责任宿舍', record.responsibleDormitories?.map(item => item.label).join('、')],
    ['情况说明', details.description],
    ['备注', details.remark],
  ]
  return (
    <Descriptions size="small" column={{ xs: 1, sm: 2 }}>
      {values.filter(([, value]) => value !== undefined && value !== null && value !== '').map(([label, value]) => (
        <Descriptions.Item key={label} label={label}>{value}</Descriptions.Item>
      ))}
    </Descriptions>
  )
}

export function WeeklyReportSummaryPage() {
  const queryClient = useQueryClient()
  const [searchParams, setSearchParams] = useSearchParams()
  const [selectedPeriodKey, setSelectedPeriodKey] = useState('')
  const [manualPeriodSelection, setManualPeriodSelection] = useState(false)
  const [selectedCell, setSelectedCell] = useState<ReportingSummaryRow | null>(null)
  const [selectedTypeId, setSelectedTypeId] = useState('')
  const [selectedTypePeriodKey, setSelectedTypePeriodKey] = useState('')
  const [recordPage, setRecordPage] = useState(1)
  const [recordPageSize, setRecordPageSize] = useState(20)
  const [recordFilters, setRecordFilters] = useState<ReportVersionRecordFiltersValue>({})
  const [recordFilterRevision, setRecordFilterRevision] = useState(0)
  const [reportScopeChanged, setReportScopeChanged] = useState(false)
  const workContextQuery = useWorkContext()
  const permissionQuery = useAttendancePermissions()
  const periodQuery = useReportingPeriods()
  const periods = useMemo(() => periodQuery.data?.items ?? [], [periodQuery.data?.items])
  const requestedSemesterId = searchParams.get('semesterId')
  const requestedWeekId = searchParams.get('weekId')
  const hasRequestedPeriod = Boolean(requestedSemesterId && requestedWeekId)
  const requestedPeriod = hasRequestedPeriod
    ? periods.find(item => item.semesterId === requestedSemesterId && item.weekId === requestedWeekId)
    : undefined
  const currentWeekId = workContextQuery.data?.week?.id
  const currentPeriod = currentWeekId ? periods.find(item => item.weekId === currentWeekId) : undefined
  const defaultPeriod = !manualPeriodSelection ? currentPeriod : undefined
  const effectivePeriodKey = hasRequestedPeriod
    ? requestedPeriod ? `${requestedPeriod.semesterId}:${requestedPeriod.weekId}` : ''
    : manualPeriodSelection ? selectedPeriodKey : defaultPeriod ? `${defaultPeriod.semesterId}:${defaultPeriod.weekId}` : ''
  const selectedPeriod = periods.find(item => `${item.semesterId}:${item.weekId}` === effectivePeriodKey)
  const visibleTypeId = selectedTypePeriodKey === effectivePeriodKey ? selectedTypeId : ''
  const stageValue = searchParams.get('stage')
  const requestedStage = stageValue === null ? Number.NaN : Number(stageValue)
  const selectedStage = hasRequestedPeriod && Number.isInteger(requestedStage) && requestedStage >= 0 && requestedStage <= 2 ? requestedStage : null
  const selectStage = (stage: number) => {
    if (selectedPeriod) setSearchParams({ semesterId: selectedPeriod.semesterId, weekId: selectedPeriod.weekId, stage: String(stage) }, { replace: true })
  }
  const canManageObligations = permissionQuery.data?.permissions.some(item => item.action === 'REPORT_OBLIGATION_MANAGE' && item.effect === 'ALLOW') ?? false
  const clearReportScope = useCallback(() => {
    queryClient.removeQueries({ queryKey: ['reports', 'summary'] })
    queryClient.removeQueries({ queryKey: ['reports', 'records'] })
    queryClient.removeQueries({ queryKey: ['reports', 'workflow'] })
    queryClient.removeQueries({ queryKey: ['reports', 'periods'] })
    setSelectedCell(null)
    setSelectedTypeId('')
    setSelectedPeriodKey('')
    setReportScopeChanged(true)
    setManualPeriodSelection(true)
    setSearchParams({}, { replace: true })
  }, [queryClient, setSearchParams])
  const resetSourceFilters = useCallback(() => {
    setRecordFilters({})
    setRecordFilterRevision(revision => revision + 1)
    setRecordPage(1)
  }, [])
  const summaryQuery = useReportingSummary(
    selectedPeriod?.semesterId ?? '',
    selectedPeriod?.weekId ?? '',
    Boolean(selectedPeriod),
    clearReportScope,
  )
  const workflowQuery = useReportWorkflow(
    selectedPeriod?.semesterId ?? '',
    selectedPeriod?.weekId ?? '',
    Boolean(selectedPeriod),
  )
  const recordsQueryArgs: ReportingRecordsQuery = useMemo(() => ({
    semesterId: selectedPeriod?.semesterId ?? '',
    weekId: selectedPeriod?.weekId ?? '',
    departmentId: selectedCell?.departmentId ?? '',
    collegeLevel: selectedCell?.collegeLevel ?? false,
    inspectionTypeId: selectedCell?.inspectionTypeId ?? '',
    studentNumber: recordFilters.studentNumber,
    studentName: recordFilters.studentName,
    className: recordFilters.className,
    grade: recordFilters.grade,
    page: recordPage,
    pageSize: recordPageSize,
  }), [selectedCell, selectedPeriod, recordFilters, recordPage, recordPageSize])
  const recordsQuery = useReportingRecords(recordsQueryArgs, Boolean(visibleTypeId && selectedCell && selectedPeriod), clearReportScope)
  const typeRows = useMemo(() => groupSummaryRows(summaryQuery.data?.rows ?? []), [summaryQuery.data?.rows])
  const sourceCells = (summaryQuery.data?.rows ?? []).filter(row => row.inspectionTypeId === visibleTypeId)
  const incompleteCells = (summaryQuery.data?.rows ?? []).filter(row => row.isRequired && row.confirmedBatchCount === 0 && row.zeroRecordConfirmedBatchCount === 0)

  const columns: ColumnsType<ReportingSummaryRow> = [
    {
      title: '检查类型', dataIndex: 'inspectionTypeName', width: 170,
    },
    {
      title: '提交情况', key: 'submissions', width: 280,
      render: (_value, row) => {
        const incompleteCount = incompleteCells.filter(item => item.inspectionTypeId === row.inspectionTypeId).length
        return <Space size={[4, 4]} wrap>
          {row.submissionCount > 0 ? <Tag>已提交 {row.submissionCount} 批</Tag> : <Typography.Text type="secondary">暂无提交</Typography.Text>}
          {row.draftBatchCount > 0 && <Tag>草稿 {row.draftBatchCount}</Tag>}
          {row.pendingReviewBatchCount > 0 && <Tag color="gold">待审核 {row.pendingReviewBatchCount}</Tag>}
          {row.returnedBatchCount > 0 && <Tag color="red">已退回 {row.returnedBatchCount}</Tag>}
          {incompleteCount > 0 && <Tag color="orange">未完成应交 {incompleteCount}</Tag>}
        </Space>
      },
    },
    {
      title: '确认数据', key: 'confirmed', width: 220,
      render: (_value, row) => row.confirmedBatchCount > 0 || row.zeroRecordConfirmedBatchCount > 0
        ? <Typography.Text>{row.confirmedBatchCount} 批 · {row.confirmedRecordCount} 条记录 · 零回执 {row.zeroRecordConfirmedBatchCount}</Typography.Text>
        : <Typography.Text type="secondary">尚无确认数据</Typography.Text>,
    },
    {
      title: '来源明细', key: 'action', width: 145, fixed: 'right',
      render: (_value, row) => <Button type="link" size="small" onClick={() => {
        setSelectedTypeId(row.inspectionTypeId)
        setSelectedTypePeriodKey(effectivePeriodKey)
        setSelectedCell(typeRows.find(item => item.inspectionTypeId === row.inspectionTypeId) ?? null)
        resetSourceFilters()
      }}>查看提交与记录</Button>,
    },
  ]

  const recordColumns: ColumnsType<ReportingRecord> = [
    { title: '日期 / 时间', key: 'occurredAt', width: 150, render: (_value, record) => displayRecordDate(record) },
    {
      title: '学生 / 责任对象', key: 'people', width: 190,
      render: (_value, record) => <Typography.Text>{recordPeople(record)}</Typography.Text>,
    },
    {
      title: '班级', key: 'class', width: 140,
      render: (_value, record) => record.className || record.responsibleClasses?.map(item => item.label).join('、') || '—',
    },
    { title: '提交记录内容', key: 'detail', render: (_value, record) => <Typography.Text>{recordSummary(record)}</Typography.Text> },
    { title: '提交归属', key: 'source', width: 130, render: (_value, record) => record.sourceDepartmentName || '归属不可用' },
  ]

  const summary = selectedPeriod ? summaryQuery.data : undefined
  const workflow = workflowQuery.isError ? undefined : workflowQuery.data
  const report = workflow?.report
  const maxStage = !report ? 0 : report.status === 'DRAFT' || report.status === 'RETURNED' ? 1 : 2
  const defaultStage = !report ? 0 : report.status === 'DRAFT' || report.status === 'RETURNED' ? 1 : 2
  const activeStage = selectedStage === null ? defaultStage : Math.min(selectedStage, maxStage)
  const periodOptions = periods.map(period => ({ value: `${period.semesterId}:${period.weekId}`, label: periodLabel(period) }))
  const selectedCellTitle = typeRows.find(row => row.inspectionTypeId === visibleTypeId)?.inspectionTypeName ?? '来源明细'
  const records = recordsQuery.data?.items ?? []
  const sourceConfirmedRecordCount = sourceCells.reduce((total, row) => total + row.confirmedRecordCount, 0)
  const submittedUnitCount = sourceCells.filter(row => row.submissionCount > 0).length
  const sourceConfirmedBatchCount = sourceCells.reduce((total, row) => total + row.confirmedBatchCount + row.zeroRecordConfirmedBatchCount, 0)
  const aggregateSourceCell = typeRows.find(row => row.inspectionTypeId === visibleTypeId) ?? null
  const selectedSourceName = selectedCell?.departmentId || selectedCell?.collegeLevel
    ? selectedCell.collegeLevel ? '管理员' : selectedCell.departmentName
    : '全部来源'

  return (
    <section className="attendance-page weekly-report-summary-page" aria-labelledby="weekly-report-summary-title">
      <div className="attendance-page-heading">
        <div>
          <Typography.Title id="weekly-report-summary-title" level={2}>通报工作台</Typography.Title>
        </div>
        <Space wrap align="center" className="weekly-report-period-controls">
          <Select
            aria-label="选择通报学期和周次"
            placeholder="请选择学期和周次"
            className="weekly-report-period-select"
            value={effectivePeriodKey || undefined}
            options={periodOptions}
            loading={periodQuery.isPending || periodQuery.isFetching}
            disabled={periodQuery.isError}
            onChange={value => { setManualPeriodSelection(true); setSelectedPeriodKey(value); setSelectedCell(null); setReportScopeChanged(false); setSearchParams({}, { replace: true }) }}
          />
          {selectedPeriod && summary && <Typography.Text type="secondary" className="weekly-report-period-range">{displayDate(summary.period.startDate)}—{displayDate(summary.period.endDate)}</Typography.Text>}
        </Space>
      </div>

      {selectedPeriod && <div className="weekly-report-workbench-steps">
        <Steps current={activeStage} onChange={stage => {
          if (stage <= maxStage) {
            selectStage(stage)
            setSelectedCell(null)
          }
        }} items={[
          { title: '确认来源数据', content: '核对部门提交与应交状态' },
          { title: '编制和修订草稿', content: '确认本版数据集' , disabled: maxStage < 1 },
          { title: '确认定稿数据', content: '审核通过并固定数据', disabled: maxStage < 2 },
        ]} />
      </div>}

      {periodQuery.isError && (
        <Alert type="error" showIcon title="可查看的学期周次加载失败。" action={<Button onClick={() => void periodQuery.refetch()}>重试</Button>} />
      )}
      {reportScopeChanged && (
        <Alert type="warning" showIcon title="你的通报查看权限已变化，系统已清除本地汇总数据。请重新选择当前可查看的周期。" />
      )}
      {workContextQuery.isError && !workContextQuery.data && (
        <Alert type="warning" showIcon title="系统当前工作周加载失败，无法自动定位通报周期。" action={<Button onClick={() => void workContextQuery.refetch()}>重试工作周</Button>} />
      )}
      {periodQuery.data && periods.length === 0 && <Empty description="当前授权范围内没有可查看的通报周期" />}
      {!selectedPeriod && !periodQuery.isError && periods.length > 0 && <Empty description={reportScopeChanged ? '请重新选择当前可查看的通报周期' : workContextQuery.isError ? '当前工作周加载失败，请重试或手动选择学期和周次' : workContextQuery.data?.week ? '系统当前工作周不在可查看范围，请手动选择学期和周次' : '系统当前未设置工作周，请手动选择学期和周次'} />}
      {selectedPeriod && summaryQuery.isError && summaryQuery.data && (
        <Alert
          type="warning"
          showIcon
          title="周汇总刷新失败"
          description="本次刷新失败，以下为最近一次成功加载的数据。请重试确认最新状态。"
          action={<Button onClick={() => void summaryQuery.refetch()} loading={summaryQuery.isFetching}>重试周汇总</Button>}
        />
      )}
      {selectedPeriod && summaryQuery.isError && !summaryQuery.data && (
        <Alert type="error" showIcon title="周汇总加载失败" description="请检查网络或权限后重试。" action={<Button onClick={() => void summaryQuery.refetch()}>重试周汇总</Button>} />
      )}
      {selectedPeriod && summaryQuery.isFetching && !summary && <Skeleton active paragraph={{ rows: 4 }} />}
      {summary && activeStage === 0 && (
        <section className="weekly-report-summary-table-section" aria-labelledby="weekly-report-summary-table-title">
          <div className="weekly-report-summary-table-toolbar">
            <div>
              <Space size={6} align="center">
                <Typography.Title id="weekly-report-summary-table-title" level={4}>按检查类型汇总</Typography.Title>
                <Tooltip title="草稿默认只引用已确认来源记录；零记录回执标记完成状态，不计入记录数。">
                  <Button type="text" size="small" aria-label="查看来源与零记录口径" icon={<InfoCircleOutlined aria-hidden="true" />} />
                </Tooltip>
              </Space>
              <Typography.Text type="secondary">已确认批次 {summary.totals.confirmedBatchCount} · 已确认记录 {summary.totals.confirmedRecordCount} · 待审核/退回 {summary.totals.pendingReviewBatchCount + summary.totals.returnedBatchCount} · 草稿 {summary.totals.draftBatchCount}</Typography.Text>
            </div>
            <Space wrap>
              <Typography.Text type="secondary">{typeRows.length} 类检查</Typography.Text>
              {selectedPeriod && <Button size="small" icon={<SyncOutlined aria-hidden="true" />} onClick={() => void summaryQuery.refetch()} loading={summaryQuery.isFetching}>刷新汇总</Button>}
            </Space>
          </div>
          {typeRows.length === 0 ? <Empty description="当前周期暂无可查看的汇总单元" /> : (
            <Table<ReportingSummaryRow>
              rowKey="inspectionTypeId"
              size="middle"
              columns={columns}
              dataSource={typeRows}
              pagination={false}
              scroll={{ x: 815 }}
              loading={summaryQuery.isFetching}
              className="weekly-report-summary-table"
              rowClassName={() => 'weekly-report-inspection-row'}
            />
          )}
          {incompleteCells.length > 0 && <section aria-label="待完成应交项">
            <Typography.Title level={5}>待完成应交项</Typography.Title>
            <Table<ReportingSummaryRow> rowKey={row => `${row.departmentId || 'college'}:${row.inspectionTypeId}`} size="small" pagination={false}
              dataSource={incompleteCells} columns={[
                { title: '提交单位', key: 'unit', render: (_value, row) => row.collegeLevel ? '管理员' : row.departmentName },
                { title: '检查类型', dataIndex: 'inspectionTypeName' },
                { title: '状态', key: 'state', render: (_value, row) => requiredCellState(row) },
              ]} />
          </section>}
        </section>
      )}

      {selectedPeriod && summary && activeStage === 0 && <WeeklyReportWorkflowPanel
        stage="source"
        period={selectedPeriod}
        summaryRows={summary.rows}
        permissions={permissionQuery.data?.permissions ?? []}
        onAccessDenied={clearReportScope}
        onStageChange={selectStage}
      />}
      {selectedPeriod && summary && activeStage === 1 && <WeeklyReportWorkflowPanel
        stage="draft"
        period={selectedPeriod}
        summaryRows={summary.rows}
        permissions={permissionQuery.data?.permissions ?? []}
        onAccessDenied={clearReportScope}
        onStageChange={selectStage}
      />}
      {selectedPeriod && summary && activeStage === 2 && <WeeklyReportWorkflowPanel
        stage="final"
        period={selectedPeriod}
        summaryRows={summary.rows}
        permissions={permissionQuery.data?.permissions ?? []}
        onAccessDenied={clearReportScope}
        onStageChange={selectStage}
      />}
      {selectedPeriod && canManageObligations && activeStage === 0 && <WeeklyObligationManagement period={selectedPeriod} />}
      <Drawer
        className="weekly-report-record-drawer"
        title={<span className="weekly-report-drawer-title">来源明细 · {selectedCellTitle}</span>}
        open={Boolean(visibleTypeId)}
        size={1040}
        styles={{ body: { overflowX: 'hidden' } }}
        onClose={() => { setSelectedCell(null); setSelectedTypeId(''); resetSourceFilters() }}
      >
        <div className="weekly-report-source-drawer">
          <div className="weekly-report-source-overview" aria-label="来源数据概况">
            <div><Typography.Text type="secondary">提交单位</Typography.Text><Typography.Text strong>{sourceCells.length}</Typography.Text></div>
            <div><Typography.Text type="secondary">已提交单位</Typography.Text><Typography.Text strong>{submittedUnitCount}</Typography.Text></div>
            <div><Typography.Text type="secondary">已确认批次</Typography.Text><Typography.Text strong>{sourceConfirmedBatchCount}</Typography.Text></div>
            <div><Typography.Text type="secondary">已确认记录</Typography.Text><Typography.Text strong>{sourceConfirmedRecordCount}</Typography.Text></div>
          </div>
          <div className="weekly-report-source-workspace">
            <section className="weekly-report-source-panel" aria-labelledby="weekly-report-submission-list-title">
              <header className="weekly-report-source-panel-heading">
                <div>
                  <Typography.Text id="weekly-report-submission-list-title" strong>提交情况</Typography.Text>
                  <Typography.Text type="secondary">按单位核对状态</Typography.Text>
                </div>
              </header>
              <Table<ReportingSummaryRow>
                rowKey={row => `${row.departmentId || 'college'}:${row.inspectionTypeId}`}
                className="weekly-report-source-submissions-table"
                size="small"
                pagination={false}
                dataSource={sourceCells}
                rowClassName={row => selectedCell?.departmentId === row.departmentId && selectedCell.collegeLevel === row.collegeLevel ? 'weekly-report-source-selected-row' : ''}
                columns={[
                  { title: '提交单位', key: 'unit', width: 120, render: (_value, row) => row.collegeLevel ? '管理员' : row.departmentName },
                  { title: '提交状态', key: 'status', render: (_value, row) => <Space size={[4, 4]} wrap>
                    {row.submissionCount > 0 ? <Tag color="blue">已提交 {row.submissionCount} 批</Tag> : <Tag>未提交</Tag>}
                    {row.pendingReviewBatchCount > 0 && <Tag color="gold">待审核 {row.pendingReviewBatchCount}</Tag>}
                    {row.returnedBatchCount > 0 && <Tag color="red">已退回 {row.returnedBatchCount}</Tag>}
                  </Space> },
                  { title: '确认记录', key: 'confirmed', width: 100, align: 'right', render: (_value, row) => row.confirmedRecordCount },
                  { title: '操作', key: 'action', width: 90, align: 'right', render: (_value, row) => <Button
                    type="link"
                    size="small"
                    aria-label={`查看${row.collegeLevel ? '管理员' : row.departmentName}记录`}
                    onClick={() => { setSelectedCell(row); resetSourceFilters() }}
                  >查看记录</Button> },
                ]}
                scroll={{ x: 560 }}
              />
            </section>
            <section className="weekly-report-source-panel weekly-report-source-records-panel" aria-labelledby="weekly-report-confirmed-records-title">
              <header className="weekly-report-source-panel-heading weekly-report-source-records-heading">
                <div>
                  <Typography.Text id="weekly-report-confirmed-records-title" strong>已确认记录</Typography.Text>
                  <Typography.Text type="secondary">{selectedSourceName} · 结果仅含已确认记录，筛选项来自当前在籍名册</Typography.Text>
                </div>
                {selectedCell?.departmentId || selectedCell?.collegeLevel ? <Button
                  className="weekly-report-source-all-button"
                  icon={<TeamOutlined aria-hidden="true" />}
                  onClick={() => {
                  setSelectedCell(aggregateSourceCell)
                  resetSourceFilters()
                }}>全部来源</Button> : <Tag color="blue">全部来源</Tag>}
              </header>
              <div className="weekly-report-source-record-filters">
                <ReportVersionRecordFilters
                  key={recordFilterRevision}
                  className="weekly-report-source-record-filter-controls"
                  ariaLabel="筛选已确认记录"
                  filterOptions={recordsQuery.data?.filterOptions}
                  onApply={filters => { setRecordFilters(filters); setRecordPage(1) }}
                />
                <Typography.Text type="secondary" className="weekly-report-source-record-filter-count">
                  {Object.values(recordFilters).some(Boolean) ? '筛选结果' : '全部记录'} · 共 {recordsQuery.data?.total ?? 0} 条
                </Typography.Text>
              </div>
              {recordsQuery.isError && <Alert
                type={records.length > 0 ? 'warning' : 'error'}
                showIcon
                title={records.length > 0 ? '记录刷新失败，仍显示最近一次成功加载的内容。' : '已确认记录加载失败'}
                action={<Button onClick={() => void recordsQuery.refetch()}>重试</Button>}
              />}
              <Table<ReportingRecord>
                rowKey="id"
                size="small"
                columns={recordColumns}
                dataSource={records}
                loading={recordsQuery.isFetching}
                locale={{ emptyText: recordsQuery.isError ? '记录加载失败' : selectedCell?.departmentId || selectedCell?.collegeLevel ? '该单位暂无已确认记录' : '当前检查类型暂无已确认记录' }}
                expandable={{ expandedRowRender: record => <RecordDetailsView record={record} />, expandRowByClick: false }}
                scroll={{ x: 1000, y: records.length > 8 ? 420 : undefined }}
                pagination={{
                  current: recordPage,
                  pageSize: recordPageSize,
                  total: recordsQuery.data?.total ?? 0,
                  showSizeChanger: true,
                  pageSizeOptions: [10, 20, 50, 100],
                  showTotal: total => `共 ${total} 条已确认记录`,
                  onChange: (page, pageSize) => {
                    if (pageSize !== recordPageSize) { setRecordPageSize(pageSize); setRecordPage(1) }
                    else setRecordPage(page)
                  },
                }}
              />
            </section>
          </div>
        </div>
      </Drawer>
    </section>
  )
}

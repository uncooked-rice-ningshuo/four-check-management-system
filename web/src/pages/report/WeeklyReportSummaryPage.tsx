import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Alert, Button, Card, Col, Descriptions, Drawer, Empty, Popconfirm, Row, Select, Space, Statistic, Table, Tag, Typography, message } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { useCreateReportObligation, useRemoveReportObligation, useReportObligationOptions, useReportObligations, useReportingPeriods, useReportingRecords, useReportingSummary } from '../../features/reporting/model/queries'
import type { ReportObligation, ReportingPeriod, ReportingRecord, ReportingRecordsQuery, ReportingSummaryRow } from '../../entities/reporting/model/reporting'
import { useWorkContext } from '../../features/work-context/useWorkContext'
import { useAttendancePermissions } from '../../features/attendance/hooks/useAttendancePermissions'

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
    { label: '校级', value: collegeTarget },
    ...eligibleTargets.map(item => ({ label: item.departmentName, value: item.departmentId })),
  ] : []
  const configured = obligationsQuery.data ?? []
  const columns: ColumnsType<ReportObligation> = [
    { title: '范围', key: 'scope', render: (_value, item) => item.collegeLevel ? '校级' : item.departmentName },
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
    <Space direction="vertical" style={{ display: 'flex' }} size="middle">
      {optionsQuery.isError && <Alert type="error" message="应交配置选项加载失败" action={<Button onClick={() => void optionsQuery.refetch()}>重试</Button>} />}
      {obligationsQuery.isError && <Alert type="error" message="现有应交项加载失败" action={<Button onClick={() => void obligationsQuery.refetch()}>重试</Button>} />}
      <Space wrap>
        <Select aria-label="应交检查类型" placeholder="选择检查类型" style={{ minWidth: 190 }} loading={optionsQuery.isFetching}
          value={inspectionTypeId || undefined} options={options?.inspectionTypes.map(item => ({ label: item.name, value: item.id })) ?? []}
          onChange={value => { setInspectionTypeId(value); setTarget('') }} />
        <Select aria-label="应交部门或校级范围" placeholder="选择部门或校级范围" style={{ minWidth: 220 }} disabled={!inspectionTypeId}
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

type ReportTreeRow = {
  key: string
  kind: 'department' | 'inspection'
  name: string
  inspectionTypeName?: string
  summary: ReportingSummaryRow
  children?: ReportTreeRow[]
}

const emptySummary = (departmentId: string, departmentName: string, collegeLevel: boolean): ReportingSummaryRow => ({
  departmentId,
  departmentName,
  collegeLevel,
  inspectionTypeId: '',
  inspectionTypeName: '',
  isRequired: false,
  submissionCount: 0,
  draftBatchCount: 0,
  pendingReviewBatchCount: 0,
  returnedBatchCount: 0,
  confirmedBatchCount: 0,
  confirmedRecordCount: 0,
  pendingReviewOrReturnedRecordCount: 0,
  zeroRecordPendingReviewBatchCount: 0,
  zeroRecordReturnedBatchCount: 0,
  zeroRecordConfirmedBatchCount: 0,
})

const sumRows = (rows: ReportingSummaryRow[], departmentId: string, departmentName: string, collegeLevel: boolean) => {
  const total = emptySummary(departmentId, departmentName, collegeLevel)
  for (const row of rows) {
    total.submissionCount += row.submissionCount
    total.draftBatchCount += row.draftBatchCount
    total.pendingReviewBatchCount += row.pendingReviewBatchCount
    total.returnedBatchCount += row.returnedBatchCount
    total.confirmedBatchCount += row.confirmedBatchCount
    total.confirmedRecordCount += row.confirmedRecordCount
    total.pendingReviewOrReturnedRecordCount += row.pendingReviewOrReturnedRecordCount
    total.zeroRecordPendingReviewBatchCount += row.zeroRecordPendingReviewBatchCount
    total.zeroRecordReturnedBatchCount += row.zeroRecordReturnedBatchCount
    total.zeroRecordConfirmedBatchCount += row.zeroRecordConfirmedBatchCount
  }
  return total
}

function buildTree(rows: ReportingSummaryRow[]): ReportTreeRow[] {
  const groups = new Map<string, ReportingSummaryRow[]>()
  for (const row of rows) {
    const key = row.collegeLevel ? 'college' : `department:${row.departmentId}`
    groups.set(key, [...(groups.get(key) ?? []), row])
  }
  return [...groups.entries()]
    .map(([key, items]) => {
      const first = items[0]
      const departmentName = first.collegeLevel ? '校级' : first.departmentName
      return {
        key,
        kind: 'department' as const,
        name: departmentName,
        summary: sumRows(items, first.departmentId, departmentName, first.collegeLevel),
        children: items.map(row => ({
          key: `${key}:type:${row.inspectionTypeId}`,
          kind: 'inspection' as const,
          name: row.inspectionTypeName,
          inspectionTypeName: row.inspectionTypeName,
          summary: row,
        })),
      }
    })
    .sort((left, right) => Number(right.summary.collegeLevel) - Number(left.summary.collegeLevel) || left.name.localeCompare(right.name, 'zh-CN'))
}

function displayDate(value: string): string {
  if (!value) return '—'
  if (/^\d{4}-\d{2}-\d{2}$/.test(value)) return value
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return value
  return new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: 'long', day: 'numeric', timeZone: 'UTC' }).format(parsed)
}

function displayPeriod(period: ReportingPeriod): string {
  return `${period.academicYearName} · ${period.semesterName} · 第 ${period.weekNo} 周 · ${displayDate(period.startDate)}—${displayDate(period.endDate)}`
}

function inspectionResult(result: string): string {
  const labels: Record<string, string> = { EXCELLENT: '优秀', QUALIFIED: '合格', UNQUALIFIED: '不合格' }
  return labels[result] ?? result
}

function uniqueLabels(values: Array<string | undefined | null>): string[] {
  return [...new Set(values.map(value => value?.trim()).filter((value): value is string => Boolean(value)))]
}

/** 「测试-计科2401班」与「测试-计科2401班（2024级）」视为同一个班级，只保留信息更全的那条 */
function dedupeClasses(labels: string[]): string[] {
  return labels.filter(label => !labels.some(other => other !== label && other.includes(label)))
}

type RecordCellContent = { main: string; sub?: string }

function recordObjectCell(record: ReportingRecord): RecordCellContent {
  const people = uniqueLabels([record.studentName, ...(record.responsibleStudents ?? []).map(item => item.label)])
  return { main: people.join('、') || '—', sub: record.studentNumber ? `学号 ${record.studentNumber}` : '' }
}

function recordClassCell(record: ReportingRecord): RecordCellContent {
  const classes = dedupeClasses(uniqueLabels([record.className, ...(record.responsibleClasses ?? []).map(item => item.label)]))
  return { main: classes.join('、') || '—', sub: record.grade || '' }
}

function recordPlaceCell(record: ReportingRecord): RecordCellContent {
  const place = uniqueLabels([record.currentAreaName, record.currentDormitoryName]).join(' · ')
  const main = place || record.currentClassroomName || '—'
  const sub = uniqueLabels([
    record.currentCourseName,
    place ? record.currentClassroomName : undefined,
  ]).join(' · ')
  return { main, sub }
}

function recordNoteCell(record: ReportingRecord): RecordCellContent {
  const description = record.details.description?.trim()
  const remark = record.details.remark?.trim()
  // 没有描述时把备注直接作为主内容显示，避免出现「—」加一行灰色小字
  if (!description) return { main: remark || '—' }
  return { main: description, sub: remark && remark !== description ? remark : '' }
}

function RecordCell({ main, sub }: RecordCellContent) {
  return (
    <div className="wr-rec">
      <div className="wr-rec-main">{main}</div>
      {sub ? <div className="wr-rec-sub">{sub}</div> : null}
    </div>
  )
}

const resultTags: Record<string, { color: string; text: string }> = {
  EXCELLENT: { color: 'green', text: '优秀' },
  QUALIFIED: { color: 'blue', text: '合格' },
  UNQUALIFIED: { color: 'red', text: '不合格' },
}

function RecordResultTag({ result }: { result?: string }) {
  if (!result) return <Typography.Text type="secondary">—</Typography.Text>
  const tone = resultTags[result]
  return <Tag color={tone?.color}>{tone?.text ?? result}</Tag>
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

function CountText({ value }: { value: number }) {
  const className = value === 0 ? 'wr-count wr-count--zero' : 'wr-count'
  return <span className={className}>{value}</span>
}

function ConfirmedRecordCell({ value, summaryRow }: { value: number; summaryRow: boolean }) {
  if (value === 0) return <CountText value={0} />
  const className = summaryRow ? 'wr-count-pill wr-count-pill--sum' : 'wr-count-pill'
  return <span className={className}>{value}</span>
}

const countRender = (value: number) => <CountText value={value} />

function RecordDetailsView({ record }: { record: ReportingRecord }) {
  const { details } = record
  const values: Array<[string, string | number | undefined | null]> = [
    ['学号', record.studentNumber],
    ['年级', record.grade],
    ['检查日期', details.date],
    ['发生时间', details.time],
    ['节次', details.period],
    ['检查轮次', details.round],
    ['缺课学时', details.absenceHours],
    ['计入学时', details.penaltyHours],
    ['缺勤次数', details.absenceCount],
    ['检查结果', details.result ? inspectionResult(details.result) : undefined],
    ['原因代码', details.reasonCode && !record.currentReasonLabel ? details.reasonCode : undefined],
    ['课程', record.currentCourseName],
    ['教室', record.currentClassroomName],
    ['宿舍', record.currentDormitoryName],
    ['检查区域', record.currentAreaName],
    ['原因', record.currentReasonLabel],
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
  const [selectedPeriodKey, setSelectedPeriodKey] = useState('')
  const [selectedCell, setSelectedCell] = useState<ReportingSummaryRow | null>(null)
  const [recordPage, setRecordPage] = useState(1)
  const [recordPageSize, setRecordPageSize] = useState(20)
  const [reportScopeChanged, setReportScopeChanged] = useState(false)
  const manualPeriodSelection = useRef(false)
  const workContextQuery = useWorkContext()
  const permissionQuery = useAttendancePermissions()
  const periodQuery = useReportingPeriods()
  const periods = useMemo(() => periodQuery.data?.items ?? [], [periodQuery.data?.items])
  useEffect(() => {
    if (!periodQuery.isSuccess || workContextQuery.isPending || manualPeriodSelection.current) return
    const currentWeekId = workContextQuery.data?.week?.id
    const currentPeriod = currentWeekId ? periods.find(item => item.weekId === currentWeekId) : undefined
    setSelectedPeriodKey(currentPeriod ? `${currentPeriod.semesterId}:${currentPeriod.weekId}` : '')
  }, [periodQuery.isSuccess, periods, workContextQuery.data?.week?.id, workContextQuery.isPending])
  const selectedPeriod = periods.find(item => `${item.semesterId}:${item.weekId}` === selectedPeriodKey)
  const canManageObligations = permissionQuery.data?.permissions.some(item => item.action === 'REPORT_OBLIGATION_MANAGE' && item.effect === 'ALLOW') ?? false
  const clearReportScope = useCallback(() => {
    queryClient.removeQueries({ queryKey: ['reports', 'summary'] })
    queryClient.removeQueries({ queryKey: ['reports', 'records'] })
    queryClient.removeQueries({ queryKey: ['reports', 'periods'] })
    setSelectedCell(null)
    setSelectedPeriodKey('')
    setReportScopeChanged(true)
    manualPeriodSelection.current = true
  }, [queryClient])
  const summaryQuery = useReportingSummary(
    selectedPeriod?.semesterId ?? '',
    selectedPeriod?.weekId ?? '',
    Boolean(selectedPeriod),
    clearReportScope,
  )
  const recordsQueryArgs: ReportingRecordsQuery = useMemo(() => ({
    semesterId: selectedPeriod?.semesterId ?? '',
    weekId: selectedPeriod?.weekId ?? '',
    departmentId: selectedCell?.departmentId ?? '',
    collegeLevel: selectedCell?.collegeLevel ?? false,
    inspectionTypeId: selectedCell?.inspectionTypeId ?? '',
    page: recordPage,
    pageSize: recordPageSize,
  }), [selectedCell, selectedPeriod, recordPage, recordPageSize])
  const recordsQuery = useReportingRecords(recordsQueryArgs, Boolean(selectedCell && selectedPeriod), clearReportScope)
  const treeRows = useMemo(() => buildTree(summaryQuery.data?.rows ?? []), [summaryQuery.data?.rows])

  const columns: ColumnsType<ReportTreeRow> = [
    {
      title: '部门 / 检查类型',
      dataIndex: 'name',
      key: 'name',
      width: 210,
      render: (_name, row) => row.kind === 'department'
        ? <Space size={8}><Typography.Text strong className="wr-name">{row.name}</Typography.Text><span className="wr-kind-count">{row.children?.length ?? 0} 类</span></Space>
        : <Typography.Text className="wr-name">{row.name}</Typography.Text>,
    },
    { title: '应交 / 零回执', key: 'requiredState', width: 220, render: (_value, row) => row.kind === 'inspection' ? requiredCellState(row.summary) : <Typography.Text type="secondary">—</Typography.Text> },
    { title: '提交批次', dataIndex: ['summary', 'submissionCount'], key: 'submissionCount', width: 82, align: 'center', render: countRender },
    { title: '草稿', dataIndex: ['summary', 'draftBatchCount'], key: 'draftBatchCount', width: 70, align: 'center', render: countRender },
    { title: '待审核', dataIndex: ['summary', 'pendingReviewBatchCount'], key: 'pendingReviewBatchCount', width: 78, align: 'center', render: countRender },
    { title: '退回', dataIndex: ['summary', 'returnedBatchCount'], key: 'returnedBatchCount', width: 70, align: 'center', render: countRender },
    { title: '已确认批次', dataIndex: ['summary', 'confirmedBatchCount'], key: 'confirmedBatchCount', width: 102, align: 'center', render: countRender },
    {
      title: '已确认记录', dataIndex: ['summary', 'confirmedRecordCount'], key: 'confirmedRecordCount', width: 102, align: 'center',
      render: (count: number, row) => <ConfirmedRecordCell value={count} summaryRow={row.kind === 'department'} />,
    },
    { title: '待办记录', dataIndex: ['summary', 'pendingReviewOrReturnedRecordCount'], key: 'pendingReviewOrReturnedRecordCount', width: 92, align: 'center', render: countRender },
    {
      title: '记录明细', key: 'action', width: 160, align: 'center',
      render: (_value, row) => {
        if (row.kind === 'department') return <Typography.Text type="secondary" className="wr-hint">—</Typography.Text>
        if (row.summary.confirmedRecordCount > 0) {
          return <Button type="link" size="small" className="wr-detail-link" onClick={() => { setSelectedCell(row.summary); setRecordPage(1) }}>查看已确认记录</Button>
        }
        return <Typography.Text type="secondary" className="wr-hint">暂无已确认记录</Typography.Text>
      },
    },
  ]

  const records = recordsQuery.data?.items ?? []
  // 只有填了检查结果的检查类型（如宿舍卫生、教室卫生）才需要「结果」列，否则整列都是「—」
  const showResultColumn = records.some(record => Boolean(record.details.result))

  const recordColumns: ColumnsType<ReportingRecord> = [
    {
      title: '检查时间', key: 'occurredAt', width: 108,
      render: (_value, record) => (
        <RecordCell main={record.details.date ? displayDate(record.details.date) : '—'} sub={record.details.time} />
      ),
    },
    {
      title: '学生 / 责任对象', key: 'object', width: 190,
      render: (_value, record) => {
        const { main, sub } = recordObjectCell(record)
        return <RecordCell main={main} sub={sub} />
      },
    },
    {
      title: '班级', key: 'class', width: 180,
      render: (_value, record) => {
        const { main, sub } = recordClassCell(record)
        return <RecordCell main={main} sub={sub} />
      },
    },
    ...(showResultColumn ? [{
      title: '结果', key: 'result', width: 84, align: 'center' as const,
      render: (_value: unknown, record: ReportingRecord) => <RecordResultTag result={record.details.result} />,
    }] : []),
    {
      title: '地点 / 课程', key: 'place', width: 210,
      render: (_value, record) => {
        const { main, sub } = recordPlaceCell(record)
        return <RecordCell main={main} sub={sub} />
      },
    },
    {
      title: '情况说明', key: 'note',
      render: (_value, record) => {
        const { main, sub } = recordNoteCell(record)
        return <RecordCell main={main} sub={sub} />
      },
    },
  ]

  const summary = summaryQuery.data
  const periodOptions = periods.map(period => ({ value: `${period.semesterId}:${period.weekId}`, label: `${period.academicYearName} · ${period.semesterName} · 第 ${period.weekNo} 周` }))
  const selectedCellTitle = selectedCell
    ? `${selectedCell.collegeLevel ? '校级' : selectedCell.departmentName} · ${selectedCell.inspectionTypeName} · 已确认记录`
    : '已确认记录'

  return (
    <Space direction="vertical" size="large" style={{ display: 'flex' }}>
      <div>
        <Typography.Title level={2}>数据汇总与统计</Typography.Title>
        <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
          默认定位到系统当前工作周。按部门查看提交和确认情况；展开部门可进入检查类型，再查看已确认提交的实际记录。
        </Typography.Paragraph>
      </div>

      <Card>
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          <Space wrap align="center">
            <Typography.Text strong>通报周期</Typography.Text>
            <Select
              aria-label="选择通报学期和周次"
              placeholder="请选择学期和周次"
              style={{ minWidth: 340 }}
      value={selectedPeriodKey || undefined}
      options={periodOptions}
      loading={periodQuery.isPending || periodQuery.isFetching}
      disabled={periodQuery.isError}
      onChange={value => { manualPeriodSelection.current = true; setSelectedPeriodKey(value); setSelectedCell(null); setReportScopeChanged(false) }}
            />
            {summary && <Typography.Text type="secondary">{displayPeriod(summary.period)}</Typography.Text>}
            {selectedPeriod && summary && (
              <Button onClick={() => void summaryQuery.refetch()} loading={summaryQuery.isFetching}>刷新汇总</Button>
            )}
          </Space>
          <Alert
            type="info"
            showIcon
            message="当前为提交数据预览，统计口径尚待确认"
            description="本页展示提交批次状态和确认后的原始记录数，不计算到课率、出勤率等指标。若授权范围只覆盖部分部门或检查类型，合计仅代表当前授权范围。"
          />
        </Space>
      </Card>

      {periodQuery.isError && (
        <Alert type="error" showIcon message="可查看的学期周次加载失败。" action={<Button onClick={() => void periodQuery.refetch()}>重试</Button>} />
      )}
      {reportScopeChanged && (
        <Alert type="warning" showIcon message="你的通报查看权限已变化，系统已清除本地汇总数据。请重新选择当前可查看的周期。" />
      )}
      {workContextQuery.isError && !workContextQuery.data && (
        <Alert type="warning" showIcon message="系统当前工作周加载失败，无法自动定位通报周期。" action={<Button onClick={() => void workContextQuery.refetch()}>重试工作周</Button>} />
      )}
      {periodQuery.data && periods.length === 0 && <Empty description="当前授权范围内没有可查看的通报周期" />}
      {!selectedPeriod && !periodQuery.isError && periods.length > 0 && <Empty description={reportScopeChanged ? '请重新选择当前可查看的通报周期' : workContextQuery.isError ? '当前工作周加载失败，请重试或手动选择学期和周次' : workContextQuery.data?.week ? '系统当前工作周不在可查看范围，请手动选择学期和周次' : '系统当前未设置工作周，请手动选择学期和周次'} />}
      {selectedPeriod && summaryQuery.isError && summaryQuery.data && (
        <Alert
          type="warning"
          showIcon
          message="周汇总刷新失败"
          description="本次刷新失败，以下为最近一次成功加载的数据。请重试确认最新状态。"
          action={<Button onClick={() => void summaryQuery.refetch()} loading={summaryQuery.isFetching}>重试周汇总</Button>}
        />
      )}
      {selectedPeriod && summaryQuery.isError && !summaryQuery.data && (
        <Alert type="error" showIcon message="周汇总加载失败" description="请检查网络或权限后重试。" action={<Button onClick={() => void summaryQuery.refetch()}>重试周汇总</Button>} />
      )}
      {selectedPeriod && summaryQuery.isFetching && !summary && <Card loading />}
      {summary && (
        <>
          <Row gutter={[16, 16]}>
            <Col xs={12} md={6}><Card><Statistic title="已确认批次" value={summary.totals.confirmedBatchCount} /></Card></Col>
            <Col xs={12} md={6}><Card><Statistic title="已确认记录" value={summary.totals.confirmedRecordCount} /></Card></Col>
            <Col xs={12} md={6}><Card><Statistic title="待审核 / 退回批次" value={summary.totals.pendingReviewBatchCount + summary.totals.returnedBatchCount} /></Card></Col>
            <Col xs={12} md={6}><Card><Statistic title="草稿批次" value={summary.totals.draftBatchCount} /></Card></Col>
          </Row>
          <Card title="部门与校级批次汇总" extra={<Typography.Text type="secondary">展开部门查看检查类型与提交记录</Typography.Text>}>
            {treeRows.length === 0 ? <Empty description="当前周期暂无可查看的汇总单元" /> : (
              <div className="weekly-report-table">
                <Table<ReportTreeRow>
                  rowKey="key"
                  size="middle"
                  columns={columns}
                  dataSource={treeRows}
                  defaultExpandAllRows
                  pagination={false}
                  scroll={{ x: 1190 }}
                  loading={summaryQuery.isFetching}
                  rowClassName={row => row.kind === 'department' ? 'weekly-report-department-row' : 'weekly-report-inspection-row'}
                />
              </div>
            )}
          </Card>
        </>
      )}

      {selectedPeriod && canManageObligations && <WeeklyObligationManagement period={selectedPeriod} />}

      <Drawer
        title={selectedCellTitle}
        open={Boolean(selectedCell)}
        width={1040}
        styles={{ body: { overflowX: 'hidden' } }}
        onClose={() => setSelectedCell(null)}
      >
        <Space direction="vertical" size="middle" style={{ display: 'flex' }}>
          <Alert
            type="info"
            showIcon
            message="以下为当前确认修订中的实际提交记录"
            description="这里不包含草稿、待审核或已退回记录。需要修正数据时，请回到平台的业务记录流程处理后再刷新通报汇总。"
          />
          {recordsQuery.isError && records.length > 0 && (
            <Alert type="warning" showIcon message="记录刷新失败，仍显示最近一次成功加载的内容。" action={<Button onClick={() => void recordsQuery.refetch()}>重试</Button>} />
          )}
          {recordsQuery.isError && records.length === 0 && (
            <Alert type="error" showIcon message="已确认记录加载失败" description="请检查网络或权限后重试。" action={<Button onClick={() => void recordsQuery.refetch()}>重试</Button>} />
          )}
          <div className="weekly-report-records">
            <Table<ReportingRecord>
              rowKey="id"
              size="small"
              columns={recordColumns}
              dataSource={records}
              loading={recordsQuery.isFetching}
              locale={{ emptyText: recordsQuery.isError ? '记录加载失败' : '本项暂无已确认的提交记录' }}
              expandable={{ expandedRowRender: record => <RecordDetailsView record={record} /> }}
              scroll={{ x: showResultColumn ? 880 : 796 }}
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
          </div>
        </Space>
      </Drawer>
    </Space>
  )
}

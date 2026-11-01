import { useEffect, useState } from 'react'
import { inspectionResultLabel, reportCorrectionFieldLabel, reportReviewActionLabel, reportStatusLabel } from '../model/labels'
import { Alert, Button, Drawer, Form, Input, InputNumber, Modal, Popconfirm, Select, Space, Table, Tag, Timeline, Tooltip, Typography } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { ApiError } from '../../../shared/api/httpClient'
import type { EffectivePermission } from '../../../entities/permission/model/permission'
import type { ReportAdjustmentSummary, ReportCorrectionLog, ReportCountRow, ReportReview, ReportTransitionAction, ReportVersion, ReportingPeriod, ReportingRecord, ReportingSummaryRow } from '../../../entities/reporting/model/reporting'
import { useAddReportAdjustment, useAddReportCorrection, useGenerateReportDraft, useReportTransition, useReportVersionRecords, useReportWorkflow } from '../model/queries'
import { reportRecordColumns, reportRecordTableWidth, reportRecordTypeCodeForName, reportRecordUsesPersonFilters } from './ReportRecordTable'
import { ReportRecordSnapshotCard } from './ReportRecordSnapshotCard'
import { ReportVersionRecordFilters, type ReportVersionRecordFiltersValue } from './ReportVersionRecordFilters'
import { ReportVersionSnapshots } from './ReportVersionSnapshots'
import { groupVersionRows } from '../model/groupReportRows'

const correctionFields = [
  { value: 'studentNumber', label: '学号', numeric: false, types: ['ATTENDANCE_RATE', 'UNATTENDED_CLASSROOM', 'EVENING_SELF_STUDY', 'MORNING_ATTENDANCE', 'LATE_RETURN'] },
  { value: 'studentName', label: '姓名', numeric: false, types: ['ATTENDANCE_RATE', 'UNATTENDED_CLASSROOM', 'EVENING_SELF_STUDY', 'MORNING_ATTENDANCE', 'LATE_RETURN'] },
  { value: 'className', label: '班级', numeric: false, types: ['ATTENDANCE_RATE', 'UNATTENDED_CLASSROOM', 'EVENING_SELF_STUDY', 'MORNING_ATTENDANCE', 'LATE_RETURN'] },
  { value: 'grade', label: '年级', numeric: false, types: ['ATTENDANCE_RATE', 'UNATTENDED_CLASSROOM', 'EVENING_SELF_STUDY', 'MORNING_ATTENDANCE', 'LATE_RETURN'] },
  { value: 'currentDormitoryName', label: '宿舍', numeric: false, types: ['LATE_RETURN', 'DORMITORY_HYGIENE'] },
  { value: 'currentCourseName', label: '课程', numeric: false, types: ['ATTENDANCE_RATE', 'UNATTENDED_CLASSROOM'] },
  { value: 'currentClassroomName', label: '教室', numeric: false, types: ['ATTENDANCE_RATE', 'UNATTENDED_CLASSROOM', 'EVENING_SELF_STUDY', 'CLASSROOM_HYGIENE'] },
  { value: 'currentAreaName', label: '检查区域', numeric: false, types: ['CLASSROOM_HYGIENE', 'PUBLIC_DORMITORY_HYGIENE'] },
  { value: 'details.date', label: '检查日期', numeric: false, types: ['ATTENDANCE_RATE', 'UNATTENDED_CLASSROOM', 'EVENING_SELF_STUDY', 'MORNING_ATTENDANCE', 'LATE_RETURN', 'DORMITORY_HYGIENE', 'CLASSROOM_HYGIENE', 'PUBLIC_DORMITORY_HYGIENE'] },
  { value: 'details.time', label: '发生时间', numeric: false, types: ['ATTENDANCE_RATE', 'UNATTENDED_CLASSROOM', 'EVENING_SELF_STUDY', 'LATE_RETURN', 'DORMITORY_HYGIENE', 'CLASSROOM_HYGIENE', 'PUBLIC_DORMITORY_HYGIENE'] },
  { value: 'details.period', label: '节次', numeric: false, types: ['ATTENDANCE_RATE', 'UNATTENDED_CLASSROOM', 'EVENING_SELF_STUDY'] },
  { value: 'details.round', label: '检查轮次', numeric: false, types: ['DORMITORY_HYGIENE', 'CLASSROOM_HYGIENE', 'PUBLIC_DORMITORY_HYGIENE'] },
  { value: 'details.absenceHours', label: '缺课学时', numeric: true, types: ['ATTENDANCE_RATE', 'UNATTENDED_CLASSROOM'] },
  { value: 'details.penaltyHours', label: '违纪计入学时', numeric: true, types: ['UNATTENDED_CLASSROOM'] },
  { value: 'details.penaltyHours', label: '违纪计入次数', numeric: true, types: ['ATTENDANCE_RATE'] },
  { value: 'details.penaltyCount', label: '违纪计入次数', numeric: true, types: ['EVENING_SELF_STUDY'] },
  { value: 'details.absenceCount', label: '缺勤次数', numeric: true, types: ['MORNING_ATTENDANCE'] },
  { value: 'details.result', label: '检查结果', numeric: false, types: ['DORMITORY_HYGIENE', 'CLASSROOM_HYGIENE', 'PUBLIC_DORMITORY_HYGIENE'] },
  { value: 'details.description', label: '情况说明', numeric: false, types: ['DORMITORY_HYGIENE', 'CLASSROOM_HYGIENE', 'PUBLIC_DORMITORY_HYGIENE'] },
  { value: 'details.remark', label: '备注', numeric: false, types: ['ATTENDANCE_RATE', 'UNATTENDED_CLASSROOM', 'EVENING_SELF_STUDY', 'MORNING_ATTENDANCE', 'LATE_RETURN', 'DORMITORY_HYGIENE', 'CLASSROOM_HYGIENE', 'PUBLIC_DORMITORY_HYGIENE'] },
] as const

function correctionFieldsFor(record: ReportingRecord) {
  return correctionFields.filter(field => (field.types as readonly string[]).includes(record.typeCode))
}

function recordFieldValue(record: ReportingRecord, field: string): string {
  const value = field.startsWith('details.')
    ? record.details[field.slice('details.'.length) as keyof ReportingRecord['details']]
    : record[field as keyof ReportingRecord]
  return value === undefined || value === null ? '' : String(value)
}

function correctionChangesFor(record: ReportingRecord, values: Record<string, string>) {
  const changes: { field: string; afterValue: string | number }[] = []
  const invalidFields: string[] = []
  for (const field of correctionFieldsFor(record)) {
    const value = values[field.value] ?? ''
    const originalValue = recordFieldValue(record, field.value)
    if (value === originalValue) continue
    if (field.numeric) {
      const number = Number(value)
      const mustBeInteger = field.value === 'details.penaltyCount' || field.value === 'details.absenceCount'
        || (record.typeCode === 'ATTENDANCE_RATE' && field.value === 'details.penaltyHours')
      if (!value.trim() || !Number.isFinite(number) || number <= 0 || (mustBeInteger && !Number.isInteger(number))) {
        invalidFields.push(field.label)
        continue
      }
      if (number === Number(originalValue)) continue
      changes.push({ field: field.value, afterValue: number })
      continue
    }
    const text = value.trim()
    if (!text && field.value !== 'details.remark') {
      invalidFields.push(field.label)
      continue
    }
    if (text === originalValue) continue
    changes.push({ field: field.value, afterValue: text })
  }
  return { changes, invalidFields }
}

function recordPeople(record: ReportingRecord) {
  return [record.studentName, ...(record.responsibleStudents ?? []).map(item => item.label),
    ...(record.responsibleClasses ?? []).map(item => item.label), ...(record.responsibleDormitories ?? []).map(item => item.label)]
    .filter(Boolean).join('、') || '—'
}

const scopeFields = ['academicYearId', 'semesterId', 'weekId', 'inspectionTypeId'] as const

function permissionCoversRows(permissions: EffectivePermission[], action: string, period: ReportingPeriod, rows: ReportingSummaryRow[]) {
  if (rows.length === 0) return false
  return rows.every(row => {
    const requested = {
      academicYearId: period.academicYearId,
      semesterId: period.semesterId,
      weekId: period.weekId,
      inspectionTypeId: row.inspectionTypeId,
    }
    const rules = permissions.filter(item => item.action === action)
    const matches = (scope: EffectivePermission['scope']) => scopeFields.every(key => !scope[key] || scope[key] === requested[key])
    const allowed = rules.some(item => item.effect === 'ALLOW' && matches(item.scope))
    const denied = rules.some(item => item.effect === 'DENY' && matches(item.scope))
    return allowed && !denied
  })
}

function displayTime(value: string) {
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? value : new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(parsed)
}

function errorMessage(error: unknown) {
  if (error instanceof ApiError) {
    if (error.code === 'INVALID_RESPONSE') return error.message
    if (error.code === 'INVALID_REPORT_CORRECTION') return '修正字段或值无效，请检查后重试。'
    if (error.code === 'REPORT_INCOMPLETE') return '存在未完成的应交单元，当前不能生成通报。'
    if (error.code === 'REPORT_VERSION_CONFLICT') return '通报已被其他操作更新，请刷新后重试。'
    if (error.code === 'REPORT_SOURCE_CHANGED') return '源记录已变化，请重新生成版本后再提交。'
    if (error.code === 'INVALID_REPORT_ADJUSTMENT') return '该记录已调整或不属于当前可操作范围，请刷新后重试。'
    if (error.code === 'INVALID_REPORT_TRANSITION') return '当前通报状态不允许此操作，请刷新状态。'
    if (error.code === 'FORBIDDEN_SCOPE') return '当前账号不能操作通报中的全部数据范围。'
  }
  return '操作未完成，请刷新后重试。'
}

export function WeeklyReportWorkflowPanel({
  period, summaryRows, permissions, onAccessDenied, stage = 'draft', onStageChange,
}: {
  period: ReportingPeriod
  summaryRows: ReportingSummaryRow[]
  permissions: EffectivePermission[]
  onAccessDenied: () => void
  stage?: 'source' | 'draft' | 'final'
  onStageChange?: (stage: number) => void
}) {
  const [workflowDrawerWidth, setWorkflowDrawerWidth] = useState(1040)
  const query = { semesterId: period.semesterId, weekId: period.weekId }
  const workflowQuery = useReportWorkflow(query.semesterId, query.weekId, true)
  const generate = useGenerateReportDraft(query)
  const transition = useReportTransition(query)
  const addCorrection = useAddReportCorrection(query)
  const addAdjustment = useAddReportAdjustment(query)
  const [returnOpen, setReturnOpen] = useState(false)
  const [returnComment, setReturnComment] = useState('')
  const [correctionRecord, setCorrectionRecord] = useState<ReportingRecord | null>(null)
  const [correctionValues, setCorrectionValues] = useState<Record<string, string>>({})
  const [correctionReason, setCorrectionReason] = useState('')
  const [adjustmentRecord, setAdjustmentRecord] = useState<ReportingRecord | null>(null)
  const [adjustmentOperation, setAdjustmentOperation] = useState<'INCLUDE' | 'EXCLUDE'>('EXCLUDE')
  const [adjustmentReason, setAdjustmentReason] = useState('')
  const [selectedReportCell, setSelectedReportCell] = useState<ReportCountRow | null>(null)
  const [recordPreview, setRecordPreview] = useState<{ key: string; record: ReportingRecord } | null>(null)
  const [recordPage, setRecordPage] = useState(1)
  const [recordPageSize, setRecordPageSize] = useState(20)
  const [recordFilters, setRecordFilters] = useState<ReportVersionRecordFiltersValue>({})
  const [expandedVersionKeys, setExpandedVersionKeys] = useState<string[]>([])
  useEffect(() => {
    const updateWorkflowDrawerWidth = () => setWorkflowDrawerWidth(Math.min(1040, window.innerWidth))
    updateWorkflowDrawerWidth()
    window.addEventListener('resize', updateWorkflowDrawerWidth)
    return () => window.removeEventListener('resize', updateWorkflowDrawerWidth)
  }, [])
  const workflow = workflowQuery.isError ? undefined : workflowQuery.data
  const report = workflow?.report ?? null
  const currentVersion = workflow?.versions.find(item => item.versionNo === report?.versionNo)
  const adjustmentHistory = currentVersion?.content.adjustments ?? []
  const latestAdjustmentByRecord = new Map<string, ReportAdjustmentSummary>()
  for (const adjustment of adjustmentHistory) latestAdjustmentByRecord.set(adjustment.recordId, adjustment)
  const excludedRecords = [...latestAdjustmentByRecord.values()].filter(item => item.operation === 'EXCLUDE')
  const reportRows = currentVersion?.content.rows.map(row => ({
    departmentId: row.departmentId,
    departmentName: row.departmentName,
    collegeLevel: row.collegeLevel,
    inspectionTypeId: row.inspectionTypeId,
    inspectionTypeName: row.inspectionType,
    isRequired: false,
    submissionCount: 0,
    draftBatchCount: 0,
    pendingReviewBatchCount: 0,
    returnedBatchCount: 0,
    confirmedBatchCount: 0,
    confirmedRecordCount: row.recordCount,
    pendingReviewOrReturnedRecordCount: 0,
    zeroRecordPendingReviewBatchCount: 0,
    zeroRecordReturnedBatchCount: 0,
    zeroRecordConfirmedBatchCount: row.confirmedZeroReceiptCount,
  })) ?? []
  const displayRows = groupVersionRows(currentVersion?.content.rows ?? [])
  const canGenerate = permissionCoversRows(permissions, 'REPORT_GENERATE', period, summaryRows)
  const canSubmit = permissionCoversRows(permissions, 'REPORT_GENERATE', period, reportRows)
  const canReview = permissionCoversRows(permissions, 'REPORT_REVIEW', period, reportRows)
  const canReturn = permissionCoversRows(permissions, 'REPORT_RETURN', period, reportRows)
  const canCorrect = permissionCoversRows(permissions, 'REPORT_CORRECT', period, reportRows)
  const recordCellKey = selectedReportCell
    ? [period.weekId, currentVersion?.versionNo ?? 0, selectedReportCell.inspectionTypeId].join(':')
    : ''
  const versionRecordsQuery = useReportVersionRecords({
    semesterId: period.semesterId, weekId: period.weekId, versionNo: currentVersion?.versionNo ?? 0,
    ...(selectedReportCell ? {
      inspectionTypeId: selectedReportCell.inspectionTypeId,
    } : {}),
    ...recordFilters,
    page: recordPage, pageSize: recordPageSize,
  }, stage !== 'source' && Boolean(currentVersion && selectedReportCell), onAccessDenied)
  const detailRecordTypeCode = versionRecordsQuery.data?.items[0]?.typeCode
    ?? reportRecordTypeCodeForName(selectedReportCell?.inspectionType)
  const visibleRecordPreview = recordPreview?.key === recordCellKey ? recordPreview.record : null
  const canEditCurrentVersion = ['DRAFT', 'RETURNED', 'FINALIZED', 'PUBLISHED', 'ARCHIVED'].includes(report?.status ?? '')

  const createDraft = () => generate.mutate({
    semesterId: period.semesterId,
    weekId: period.weekId,
    expectedRowVersion: report?.rowVersion ?? 0,
  }, { onSuccess: () => onStageChange?.(1) })
  const runTransition = (action: ReportTransitionAction, comment = '') => {
    if (!report) return
    transition.mutate({ reportId: report.id, action, body: { rowVersion: report.rowVersion, ...(comment ? { comment } : {}) } }, {
      onSuccess: () => onStageChange?.(action === 'return' ? 1 : action === 'submit' || action === 'approve' ? 2 : 2),
    })
  }

  const toggleVersionDetails = (version: ReportVersion) => {
    setExpandedVersionKeys(current => current.includes(version.id)
      ? current.filter(key => key !== version.id)
      : [...current, version.id])
  }

  const showVersionDetails = (versionNo: number) => {
    const version = workflow?.versions.find(item => item.versionNo === versionNo)
    if (version && !expandedVersionKeys.includes(version.id)) {
      setExpandedVersionKeys(current => current.includes(version.id) ? current : [...current, version.id])
    }
  }

  const openCorrection = (record: ReportingRecord) => {
    setCorrectionRecord(record)
    setCorrectionValues(Object.fromEntries(correctionFieldsFor(record).map(field => [field.value, recordFieldValue(record, field.value)])))
    setCorrectionReason('')
  }

  const saveCorrection = () => {
    if (!report || !correctionRecord) return
    const { changes, invalidFields } = correctionChangesFor(correctionRecord, correctionValues)
    if (changes.length === 0 || invalidFields.length > 0 || correctionReason.trim().length < 5) return
    addCorrection.mutate({ reportId: report.id, body: {
      rowVersion: report.rowVersion, recordId: correctionRecord.id, changes, reason: correctionReason.trim(),
    } }, { onSuccess: () => {
      setCorrectionRecord(null)
      setCorrectionReason('')
      setCorrectionValues({})
      // A correction creates a new immutable snapshot; old filters may no longer match the corrected values.
      setRecordFilters({})
      setRecordPage(1)
      onStageChange?.(1)
    } })
  }

  const openAdjustment = (record: ReportingRecord, operation: 'INCLUDE' | 'EXCLUDE') => {
    setAdjustmentRecord(record)
    setAdjustmentOperation(operation)
    setAdjustmentReason('')
  }

  const saveAdjustment = () => {
    if (!report || !adjustmentRecord || adjustmentReason.trim().length < 5) return
    addAdjustment.mutate({ reportId: report.id, body: {
      rowVersion: report.rowVersion,
      recordId: adjustmentRecord.id,
      operation: adjustmentOperation,
      reason: adjustmentReason.trim(),
    } }, { onSuccess: () => {
      setAdjustmentRecord(null)
      setAdjustmentReason('')
      setSelectedReportCell(null)
    } })
  }

  const versionColumns: ColumnsType<ReportVersion> = [
    { title: '版本', dataIndex: 'versionNo', width: 70, render: value => `V${value}` },
    { title: '确认记录数', key: 'count', align: 'right', render: (_value, item) => item.content.confirmedRecordCount },
    { title: '统计单元', key: 'cells', align: 'right', render: (_value, item) => item.content.rows.length },
    { title: '未完成应交项', key: 'incomplete', align: 'right', render: (_value, item) => item.content.incompleteCells.length },
    { title: '生成时间', key: 'createdAt', render: (_value, item) => displayTime(item.createdAt) },
    { title: '范围', key: 'scope', render: (_value, item) => item.content.scopeLimited ? <Tag color="orange">授权范围版本</Tag> : <Tag>完整可见范围</Tag> },
    { title: '数据明细', key: 'snapshot', width: 120, align: 'right', render: (_value, item) => <Button type="link" size="small"
      aria-expanded={expandedVersionKeys.includes(item.id)} onClick={() => toggleVersionDetails(item)}>
      {expandedVersionKeys.includes(item.id) ? '收起明细' : '查看明细'}
    </Button> },
  ]
  const reportRowColumns: ColumnsType<ReportCountRow> = [
    { title: '检查类型', dataIndex: 'inspectionType', width: 115 },
    { title: '本版记录数', dataIndex: 'recordCount', align: 'right' },
    { title: '已确认零记录回执', dataIndex: 'confirmedZeroReceiptCount', align: 'right' },
    { title: '记录', key: 'detail', align: 'right', width: 72, fixed: 'right', render: (_value, row) =>
      <Button type="link" size="small" aria-label="查看明细" onClick={() => {
        setRecordPreview(null)
        setSelectedReportCell(row)
        setRecordFilters({})
        setRecordPage(1)
      }}>明细</Button> },
  ]
  const correctionColumns: ColumnsType<ReportCorrectionLog> = [
    { title: '版本', dataIndex: 'versionNo', width: 130, render: value => <Button type="link" size="small"
      aria-label={`查看 V${value} 数据明细`} onClick={() => showVersionDetails(value)}>V{value} · 数据明细</Button> },
    { title: '修改内容', dataIndex: 'field', width: 190, render: reportCorrectionFieldLabel },
    { title: '原值', dataIndex: 'beforeValue', width: 140, render: value => value === null || value === '' ? '（空）' : String(value) },
    { title: '修正后', dataIndex: 'afterValue', width: 140, render: value => String(value) },
    { title: '处理说明', dataIndex: 'reason' },
    { title: '处理时间', dataIndex: 'correctedAt', width: 165, render: value => displayTime(value) },
  ]
  const detailColumns: ColumnsType<ReportingRecord> = [
    ...reportRecordColumns(detailRecordTypeCode),
    { title: '提交归属', key: 'sourceUnit', width: 130, render: (_value, record) => record.sourceDepartmentName || '历史归属不可用' },
    { title: '记录明细', key: 'view', width: 96, fixed: 'right', render: (_value, record) =>
      <Button type="link" size="small" onClick={() => setRecordPreview({ key: recordCellKey, record })}>查看</Button> },
    { title: '本版处理', key: 'actions', width: 150, fixed: 'right', render: (_value, record) => stage === 'draft' && canEditCurrentVersion && canCorrect
      ? <Space size={4}>
        <Button type="link" onClick={() => openCorrection(record)}>修正</Button>
        <Button type="link" danger onClick={() => openAdjustment(record, 'EXCLUDE')}>排除</Button>
      </Space>
      : <Typography.Text type="secondary">只读</Typography.Text> },
  ]
  const reviews: ReportReview[] = workflow?.reviews ?? []
  const latestReturn = report?.status === 'RETURNED' ? [...reviews].reverse().find(item => item.action === 'RETURNED' && item.comment) : undefined

  const finalized = report !== null && ['FINALIZED', 'PUBLISHED', 'ARCHIVED'].includes(report.status)
  const correctionValidation = correctionRecord ? correctionChangesFor(correctionRecord, correctionValues) : { changes: [], invalidFields: [] }
  const outputControls = (kind: 'draft' | 'final') => <Space wrap>
    <Tooltip title="后续开放；生成结果将关联当前版本。"><span><Button disabled>{kind === 'final' ? '生成定稿 Word' : '生成草稿 Word'}</Button></span></Tooltip>
    <Tooltip title="后续开放；生成结果将关联当前版本。"><span><Button disabled>{kind === 'final' ? '生成定稿 Excel' : '生成草稿 Excel'}</Button></span></Tooltip>
  </Space>

  return <section className="weekly-report-workflow-panel" aria-label={stage === 'source' ? '来源核对操作' : undefined} aria-labelledby={stage === 'source' ? undefined : 'weekly-report-workflow-title'}>
    {stage !== 'source' && <div className="weekly-report-workflow-heading">
      <Space size="middle" wrap>
        <Typography.Title id="weekly-report-workflow-title" level={4}>本版数据</Typography.Title>
        {currentVersion && <Typography.Text type="secondary">已确认记录 {currentVersion.content.confirmedRecordCount} 条</Typography.Text>}
      </Space>
      {report && <Tag color={report.status === 'RETURNED' ? 'red' : 'blue'}>{reportStatusLabel(report.status)} · V{report.versionNo}</Tag>}
    </div>}
    {workflowQuery.isError && <Alert
      type={workflowQuery.error instanceof ApiError && workflowQuery.error.status === 403 ? 'warning' : 'error'}
      showIcon
      title={workflowQuery.error instanceof ApiError && workflowQuery.error.status === 403 ? '当前账号无权查看完整通报版本' : '通报版本状态加载失败'}
      description={workflowQuery.error instanceof ApiError
        ? workflowQuery.error.status === 403
          ? '你仍可查看已授权的来源汇总；草稿和定稿阶段需要覆盖整版数据的权限。'
          : workflowQuery.error.message
        : '网络或服务异常，请检查后重试。'}
      action={<Button onClick={() => void workflowQuery.refetch()}>重试</Button>}
    />}
    {generate.isError && <Alert type="error" showIcon title={errorMessage(generate.error)} />}
    {workflowQuery.isPending && <Typography.Text type="secondary">正在加载通报版本…</Typography.Text>}
    {stage === 'source' && workflow && !report && <Space orientation="vertical" style={{ width: '100%' }}>
      <Typography.Text type="secondary">草稿将纳入已确认的来源数据。</Typography.Text>
      {!canGenerate && <Alert type="warning" showIcon title="当前账号无权生成草稿" />}
    </Space>}
    {stage === 'source' && report && <Typography.Text type="secondary">该周已有 {reportStatusLabel(report.status)} 数据集 V{report.versionNo}。</Typography.Text>}
    {stage !== 'source' && report && workflow && <Space orientation="vertical" style={{ width: '100%' }}>
      {currentVersion?.content.scopeLimited && <Tooltip title="该版本只覆盖生成账号当时有权查看的范围，不代表完整学院数据。"><Tag color="orange">授权范围版本 ⓘ</Tag></Tooltip>}
      {transition.isError && <Alert type="error" showIcon title={errorMessage(transition.error)} />}
      {addAdjustment.isError && <Alert type="error" showIcon title={errorMessage(addAdjustment.error)} />}
      {latestReturn && <Alert type="warning" showIcon title={`退回原因：${latestReturn.comment}`} />}
      {currentVersion && <section aria-label="按检查类型分类">
        <Table<ReportCountRow> rowKey="inspectionTypeId" size="small" pagination={false} columns={reportRowColumns} dataSource={displayRows} scroll={{ x: 760 }} />
      </section>}
      {stage === 'draft' && <>
        {excludedRecords.length > 0 && <>
          <Typography.Text strong>已从本版排除</Typography.Text>
          <Table<ReportAdjustmentSummary> rowKey="id" size="small" pagination={false} dataSource={excludedRecords} scroll={{ x: 640 }} columns={[
            { title: '对象', key: 'person', render: (_value, item) => `${item.record.studentName || '未命名'} · ${item.record.className || '未标注班级'}` },
            { title: '排除原因', dataIndex: 'reason' },
            { title: '操作', key: 'actions', align: 'right', width: 140, render: (_value, item) => canEditCurrentVersion && canCorrect
              ? <Button type="link" onClick={() => openAdjustment(item.record, 'INCLUDE')}>恢复纳入</Button>
              : <Typography.Text type="secondary">只读</Typography.Text> },
          ]} />
        </>}
        {addCorrection.isError && <Alert type="error" showIcon title={errorMessage(addCorrection.error)} />}
      </>}
      <details className="weekly-report-history">
        <summary>版本与操作记录</summary>
        <Space orientation="vertical" size="middle" style={{ display: 'flex' }}>
          <Table<ReportVersion> rowKey="id" size="small" pagination={false} columns={versionColumns} dataSource={workflow.versions} scroll={{ x: 850 }}
            expandable={{ showExpandColumn: false, expandedRowKeys: expandedVersionKeys, expandedRowRender: version => <ReportVersionSnapshots
              period={period}
              version={version}
              onAccessDenied={onAccessDenied}
              editable={stage === 'draft' && canEditCurrentVersion && version.versionNo === currentVersion?.versionNo && canCorrect}
              onCorrect={openCorrection}
              onExclude={record => openAdjustment(record, 'EXCLUDE')}
            /> }} />
          {reviews.length > 0 && <>
            <Typography.Text strong>审核与状态记录</Typography.Text>
            <Timeline items={reviews.map(item => ({ content: <span>{reportReviewActionLabel(item.action)} · V{item.versionNo} · {displayTime(item.createdAt)}{item.comment ? ` · ${item.comment}` : ''}</span> }))} />
          </>}
          {workflow.corrections.length > 0 && <>
            <Typography.Text strong>追加修正审计</Typography.Text>
            <Table<ReportCorrectionLog> rowKey="id" size="small" pagination={false} columns={correctionColumns} dataSource={workflow.corrections} scroll={{ x: 760 }} />
          </>}
        </Space>
      </details>
    </Space>}
    <Drawer rootClassName="weekly-report-record-drawer" classNames={{ wrapper: 'weekly-report-record-drawer-wrapper' }}
      title={selectedReportCell ? `${selectedReportCell.inspectionType} · V${currentVersion?.versionNo ?? ''}` : '本版记录明细'} open={Boolean(selectedReportCell)}
      size={workflowDrawerWidth} onClose={() => { setSelectedReportCell(null); setRecordPreview(null) }} destroyOnHidden>
      {selectedReportCell && <Space orientation="vertical" size="middle" style={{ display: 'flex' }}>
        {versionRecordsQuery.isError && <Alert type="error" showIcon title="记录明细加载失败" action={<Button onClick={() => void versionRecordsQuery.refetch()}>重试</Button>} />}
        {stage === 'draft' && <Typography.Text type="secondary">明细来自当前版本冻结快照；修正、排除和恢复纳入都会生成新版本。</Typography.Text>}
        {!canCorrect && stage === 'draft' && <Typography.Text type="secondary">当前账号没有通报修正权限，记录仅供查看。</Typography.Text>}
        {reportRecordUsesPersonFilters(detailRecordTypeCode) && <ReportVersionRecordFilters key={`${currentVersion?.versionNo ?? 0}:${selectedReportCell.inspectionTypeId}`}
          filterOptions={versionRecordsQuery.data?.filterOptions}
          onApply={filters => { setRecordFilters(filters); setRecordPage(1) }} />}
        <Table<ReportingRecord> className="report-record-table" rowKey="id" size="middle" columns={detailColumns} dataSource={versionRecordsQuery.data?.items ?? []}
          loading={versionRecordsQuery.isPending || versionRecordsQuery.isFetching}
          locale={{ emptyText: versionRecordsQuery.isError ? '记录明细加载失败' : Object.values(recordFilters).some(Boolean) ? '没有符合筛选条件的本版记录' : '该分类下没有本版记录' }}
          scroll={{ x: reportRecordTableWidth(detailColumns) }} pagination={{ current: recordPage, pageSize: recordPageSize,
            total: versionRecordsQuery.data?.total ?? 0, showSizeChanger: true, pageSizeOptions: [10, 20, 50, 100],
            showTotal: total => `共 ${total} 条本版记录`, onChange: (page, pageSize) => {
              if (pageSize !== recordPageSize) { setRecordPageSize(pageSize); setRecordPage(1) } else setRecordPage(page)
            } }} />
      </Space>}
    </Drawer>
    <Modal className="report-record-detail-modal" title="记录明细" width={960} open={Boolean(visibleRecordPreview)} footer={null}
      onCancel={() => setRecordPreview(null)} destroyOnHidden>
      {visibleRecordPreview && <ReportRecordSnapshotCard record={visibleRecordPreview} versionNo={currentVersion?.versionNo ?? 1} title={`V${currentVersion?.versionNo ?? 1} · 记录快照`} />}
    </Modal>
    <div className="weekly-report-stage-actions" role="region" aria-label="当前阶段操作">
      <div className="weekly-report-stage-actions-inner">
        {stage === 'source' && workflow && !report && canGenerate && <Button type="primary" loading={generate.isPending} onClick={createDraft}>确认数据并生成草稿</Button>}
        {stage === 'source' && report && <Button type="primary" onClick={() => onStageChange?.(report.status === 'DRAFT' || report.status === 'RETURNED' ? 1 : 2)}>
          {report.status === 'DRAFT' || report.status === 'RETURNED' ? '继续编制草稿' : '查看定稿确认'}
        </Button>}
        {stage === 'draft' && report && <Space wrap>
          {(report.status === 'DRAFT' || report.status === 'RETURNED') && canGenerate && <Popconfirm title="读取最新已确认来源并生成新版本？" description="本版手工修正和排除项不会自动沿用，旧版本仍保留。" okText="生成新版本" cancelText="取消" onConfirm={createDraft}>
            <Button loading={generate.isPending}>读取最新数据</Button>
          </Popconfirm>}
          {(report.status === 'DRAFT' || report.status === 'RETURNED') && canSubmit && <Button type="primary" loading={transition.isPending} onClick={() => runTransition('submit')}>草稿无误，进入定稿确认</Button>}
          {outputControls('draft')}
        </Space>}
        {stage === 'final' && report && <Space wrap>
          {report.status === 'DRAFT' || report.status === 'RETURNED' ? <Button onClick={() => onStageChange?.(1)}>返回草稿修订</Button> : null}
          {report.status === 'PENDING_REVIEW' && canReview && <Button type="primary" loading={transition.isPending} onClick={() => runTransition('approve')}>审核通过</Button>}
          {report.status === 'PENDING_REVIEW' && canReturn && <Button danger loading={transition.isPending} onClick={() => { setReturnComment(''); setReturnOpen(true) }}>退回草稿</Button>}
          {report.status === 'FINALIZING' && permissionCoversRows(permissions, 'REPORT_FINALIZE', period, reportRows) && <Button type="primary" loading={transition.isPending} onClick={() => runTransition('finalize')}>确认定稿数据</Button>}
          {finalized && outputControls('final')}
        </Space>}
      </div>
    </div>
    <Modal className="report-correction-modal" width={960} title={correctionRecord ? `编辑 V${currentVersion?.versionNo ?? ''} 通报记录` : '编辑通报记录'} open={Boolean(correctionRecord)}
      okText="保存修改并生成新版本" cancelText="取消" confirmLoading={addCorrection.isPending}
      okButtonProps={{ disabled: !correctionRecord || correctionReason.trim().length < 5 || correctionValidation.changes.length === 0 || correctionValidation.invalidFields.length > 0 }}
      onCancel={() => setCorrectionRecord(null)} onOk={saveCorrection} destroyOnHidden>
      {correctionRecord && <div className="report-correction-form">
        <section className="report-correction-snapshot" aria-label="当前版本记录快照">
          <ReportRecordSnapshotCard record={correctionRecord} versionNo={currentVersion?.versionNo ?? 1} />
        </section>
        <Form layout="vertical" className="report-correction-fields-form" colon={false}>
          <section aria-labelledby="report-correction-fields-title">
            <Typography.Text id="report-correction-fields-title" strong>修正字段</Typography.Text>
            <div className="report-correction-field-grid">
              {correctionFieldsFor(correctionRecord).map(field => {
                const fieldLabel = correctionRecord.typeCode === 'ATTENDANCE_RATE' && field.value === 'details.absenceHours' ? '缺课节次数' : field.label
                const multiline = field.value === 'details.description' || field.value === 'details.remark'
                const value = correctionValues[field.value] ?? ''
                const integerValue = field.value === 'details.penaltyCount' || field.value === 'details.absenceCount'
                  || (correctionRecord.typeCode === 'ATTENDANCE_RATE' && field.value === 'details.penaltyHours')
                const numericValue = field.numeric && value !== '' ? Number(value) : null
                const updateValue = (nextValue: string) => setCorrectionValues(current => ({ ...current, [field.value]: nextValue }))
                return <Form.Item key={field.value} label={fieldLabel}
                  className={`report-correction-field${multiline ? ' report-correction-field--wide' : ''}`}>
                  {field.value === 'details.result' ? <Select aria-label={fieldLabel} value={value || undefined}
                    options={['EXCELLENT', 'QUALIFIED', 'UNQUALIFIED'].map(option => ({ value: option, label: inspectionResultLabel(option) }))}
                    onChange={updateValue} /> : multiline
                    ? <Input.TextArea aria-label={fieldLabel} rows={2} value={value}
                      onChange={event => updateValue(event.target.value)} /> : field.numeric
                      ? <InputNumber aria-label={fieldLabel} min={0} precision={integerValue ? 0 : undefined} step={integerValue ? 1 : 0.1}
                        style={{ width: '100%' }} value={Number.isFinite(numericValue) ? numericValue : null}
                        onChange={nextValue => updateValue(nextValue === null ? '' : String(nextValue))} /> : field.value === 'details.date'
                        ? <Input aria-label={fieldLabel} inputMode="numeric" placeholder="YYYY-MM-DD" value={value}
                          onChange={event => updateValue(event.target.value)} />
                        : field.value === 'details.time'
                          ? <Input aria-label={fieldLabel} inputMode="numeric" placeholder="HH:mm:ss" value={value}
                            onChange={event => updateValue(event.target.value)} />
                          : <Input aria-label={fieldLabel} value={value} onChange={event => updateValue(event.target.value)} />}
                </Form.Item>
              })}
            </div>
          </section>
          {correctionValidation.invalidFields.length > 0 && <Alert type="warning" showIcon title={`请修正无效字段：${correctionValidation.invalidFields.join('、')}`} />}
          <Form.Item label="处理说明（审计留痕）" className="report-correction-reason">
            <Input.TextArea aria-label="处理说明" rows={3} maxLength={500} showCount value={correctionReason} onChange={event => setCorrectionReason(event.target.value)} />
          </Form.Item>
          <Typography.Text type="secondary">处理说明至少填写 5 个字；每个修改值会保存在新版本和修正审计中。</Typography.Text>
          {addCorrection.isError && <Alert type="error" showIcon title="修正未保存" description={errorMessage(addCorrection.error)} />}
        </Form>
      </div>}
    </Modal>
    <Modal title={adjustmentOperation === 'EXCLUDE' ? '从本版排除记录' : '恢复纳入本版'} open={Boolean(adjustmentRecord)}
      okText={adjustmentOperation === 'EXCLUDE' ? '排除并生成新版本' : '恢复并生成新版本'} cancelText="取消" confirmLoading={addAdjustment.isPending}
      okButtonProps={{ disabled: adjustmentReason.trim().length < 5 }} onCancel={() => setAdjustmentRecord(null)} onOk={saveAdjustment} destroyOnHidden>
      {adjustmentRecord && <Space orientation="vertical" size="middle" style={{ display: 'flex' }}>
        <Typography.Text>{recordPeople(adjustmentRecord)} · {adjustmentRecord.className || '未标注班级'}</Typography.Text>
        <label>调整原因<Input.TextArea aria-label="调整原因" rows={3} maxLength={500} showCount value={adjustmentReason} onChange={event => setAdjustmentReason(event.target.value)} /></label>
        <Typography.Text type="secondary">只调整通报版本，部门已确认的来源记录不会被删除或改写。</Typography.Text>
      </Space>}
    </Modal>
    <Modal title="退回通报修改" open={returnOpen} okText="确认退回" cancelText="取消" confirmLoading={transition.isPending}
      okButtonProps={{ danger: true, disabled: !returnComment.trim() }} onCancel={() => setReturnOpen(false)}
      onOk={() => {
        if (!returnComment.trim()) return
        runTransition('return', returnComment.trim())
        setReturnOpen(false)
      }}>
      <Input.TextArea aria-label="退回原因" maxLength={1000} showCount rows={4} value={returnComment} onChange={event => setReturnComment(event.target.value)} />
    </Modal>
  </section>
}

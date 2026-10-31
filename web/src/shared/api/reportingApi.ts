import type { AddReportAdjustmentInput, AddReportCorrectionInput, CreateReportObligationInput, GenerateReportDraftInput, ReportAdjustmentSummary, ReportAdjustmentResult, ReportArchiveEntry, ReportArchivePage, ReportArchiveQuery, ReportCorrectionLog, ReportCorrectionResult, ReportDraft, ReportObligation, ReportObligationDepartmentScope, ReportObligationInspectionType, ReportObligationOptions, ReportReview, ReportState, ReportTransitionAction, ReportTransitionInput, ReportVersion, ReportWorkflow, ReportVersionContent, ReportVersionRecordsQuery, ReportingPeriod, ReportingPeriodPage, ReportingRecord, ReportingRecordPage, ReportingRecordsQuery, ReportingSummary, ReportingSummaryRow, ReportingSummaryTotals } from '../../entities/reporting/model/reporting'
import { apiRequest } from './httpClient'
import { badResponse, object } from './inspectionOptionsApi'

function isValidDateRange(startDate: unknown, endDate: unknown): boolean {
  if (typeof startDate !== 'string' || typeof endDate !== 'string') return false
  const startTimestamp = Date.parse(startDate)
  const endTimestamp = Date.parse(endDate)
  return Number.isFinite(startTimestamp) && Number.isFinite(endTimestamp) && startTimestamp <= endTimestamp
}

function isPeriod(value: unknown): value is ReportingPeriod {
  return object(value)
    && typeof value.academicYearId === 'string'
    && typeof value.academicYearName === 'string'
    && typeof value.semesterId === 'string'
    && typeof value.semesterName === 'string'
    && typeof value.weekId === 'string'
    && Number.isInteger(value.weekNo)
    && Number(value.weekNo) >= 1
    && Number(value.weekNo) <= 60
    && isValidDateRange(value.startDate, value.endDate)
}

function isSummaryRow(value: unknown): value is ReportingSummaryRow {
  return object(value)
    && typeof value.departmentId === 'string'
    && typeof value.departmentName === 'string'
    && typeof value.collegeLevel === 'boolean'
    && typeof value.inspectionTypeId === 'string'
    && typeof value.inspectionTypeName === 'string'
    && typeof value.isRequired === 'boolean'
    && ['submissionCount', 'draftBatchCount', 'pendingReviewBatchCount', 'returnedBatchCount', 'confirmedBatchCount', 'confirmedRecordCount', 'pendingReviewOrReturnedRecordCount', 'zeroRecordPendingReviewBatchCount', 'zeroRecordReturnedBatchCount', 'zeroRecordConfirmedBatchCount']
      .every(key => Number.isInteger(value[key]) && Number(value[key]) >= 0)
}

function isTotals(value: unknown): value is ReportingSummaryTotals {
  return object(value)
    && ['submissionCount', 'draftBatchCount', 'pendingReviewBatchCount', 'returnedBatchCount', 'confirmedBatchCount', 'confirmedRecordCount', 'pendingReviewOrReturnedRecordCount', 'zeroRecordPendingReviewBatchCount', 'zeroRecordReturnedBatchCount', 'zeroRecordConfirmedBatchCount']
      .every(key => Number.isInteger(value[key]) && Number(value[key]) >= 0)
}

function isRecordReference(value: unknown): boolean {
  return object(value) && typeof value.label === 'string'
    && (value.grade === undefined || typeof value.grade === 'string')
}

function isRecordDetails(value: unknown): boolean {
  if (!object(value)) return false
  const stringFields = ['date', 'time', 'period', 'round', 'result', 'reasonCode', 'description', 'remark']
  if (!stringFields.every(field => value[field] === undefined || typeof value[field] === 'string')) return false
  const numberFields = ['absenceHours', 'absenceCount']
  if (!numberFields.every(field => value[field] === undefined || (typeof value[field] === 'number' && Number.isFinite(value[field])))) return false
  return value.penaltyHours === undefined || value.penaltyHours === null
    || (typeof value.penaltyHours === 'number' && Number.isFinite(value.penaltyHours))
}

function isReportingRecord(value: unknown): value is ReportingRecord {
  return object(value)
    && typeof value.id === 'string'
    && typeof value.studentNumber === 'string'
    && typeof value.studentName === 'string'
    && typeof value.className === 'string'
    && typeof value.grade === 'string'
    && typeof value.typeCode === 'string'
    && isRecordDetails(value.details)
    && typeof value.currentDormitoryName === 'string'
    && typeof value.currentCourseName === 'string'
    && typeof value.currentClassroomName === 'string'
    && (value.currentAreaName === undefined || typeof value.currentAreaName === 'string')
    && (value.currentReasonLabel === undefined || typeof value.currentReasonLabel === 'string')
    && (value.responsibleClasses === undefined || (Array.isArray(value.responsibleClasses) && value.responsibleClasses.every(isRecordReference)))
    && (value.responsibleStudents === undefined || (Array.isArray(value.responsibleStudents) && value.responsibleStudents.every(isRecordReference)))
    && (value.responsibleDormitories === undefined || (Array.isArray(value.responsibleDormitories) && value.responsibleDormitories.every(isRecordReference)))
}

function isReportObligation(value: unknown): value is ReportObligation {
  return object(value)
    && typeof value.id === 'string'
    && typeof value.semesterId === 'string'
    && typeof value.weekId === 'string'
    && typeof value.academicYearId === 'string'
    && typeof value.departmentId === 'string'
    && typeof value.departmentName === 'string'
    && typeof value.collegeLevel === 'boolean'
    && typeof value.inspectionTypeId === 'string'
    && typeof value.inspectionTypeName === 'string'
    && typeof value.eligible === 'boolean'
    && Number.isInteger(value.version) && Number(value.version) > 0
    && typeof value.createdAt === 'string' && Number.isFinite(Date.parse(value.createdAt))
    && typeof value.updatedAt === 'string' && Number.isFinite(Date.parse(value.updatedAt))
    && (value.removedAt === undefined || (typeof value.removedAt === 'string' && Number.isFinite(Date.parse(value.removedAt))))
}

function isReportObligationInspectionType(value: unknown): value is ReportObligationInspectionType {
  return object(value) && typeof value.id === 'string' && typeof value.name === 'string'
    && Number.isInteger(value.sortOrder)
}

function isReportObligationDepartmentScope(value: unknown): value is ReportObligationDepartmentScope {
  return object(value) && typeof value.departmentId === 'string' && typeof value.departmentName === 'string'
    && typeof value.inspectionTypeId === 'string' && typeof value.inspectionTypeName === 'string'
}

function isReportState(value: unknown): value is ReportState {
  return object(value) && typeof value.id === 'string' && typeof value.semesterId === 'string'
    && typeof value.weekId === 'string' && ['DRAFT', 'PENDING_REVIEW', 'RETURNED', 'FINALIZING', 'FINALIZED', 'PUBLISHED', 'ARCHIVED'].includes(String(value.status))
    && Number.isInteger(value.versionNo) && Number(value.versionNo) >= 0
    && Number.isInteger(value.rowVersion) && Number(value.rowVersion) > 0
    && typeof value.updatedAt === 'string' && Number.isFinite(Date.parse(value.updatedAt))
}

function isReportVersionContent(value: unknown): value is ReportVersionContent {
  if (!object(value) || !isPeriod(value.period) || typeof value.scopeLimited !== 'boolean'
    || typeof value.statisticsRuleVersion !== 'string' || !Number.isInteger(value.confirmedRecordCount)
    || !Array.isArray(value.rows) || !Array.isArray(value.incompleteCells)
    || !Number.isInteger(value.pendingDepartmentReconfirmationCount) || !Array.isArray(value.corrections)
    || (value.adjustments !== undefined && value.adjustments !== null && (!Array.isArray(value.adjustments) || !value.adjustments.every(isReportAdjustmentSummary)))
    || typeof value.generatedAt !== 'string' || !Number.isFinite(Date.parse(value.generatedAt))) return false
  return value.rows.every(row => object(row) && typeof row.departmentId === 'string' && typeof row.departmentName === 'string'
    && typeof row.collegeLevel === 'boolean' && typeof row.inspectionTypeId === 'string' && typeof row.inspectionType === 'string'
    && Number.isInteger(row.recordCount) && Number(row.recordCount) >= 0
    && Number.isInteger(row.confirmedZeroReceiptCount) && Number(row.confirmedZeroReceiptCount) >= 0
    && typeof row.dataStatus === 'string')
}

function isReportAdjustmentSummary(value: unknown): value is ReportAdjustmentSummary {
  return object(value) && typeof value.id === 'string' && typeof value.recordId === 'string'
    && (value.operation === 'INCLUDE' || value.operation === 'EXCLUDE')
    && typeof value.reason === 'string' && isReportingRecord(value.record)
    && typeof value.adjustedBy === 'string' && typeof value.adjustedAt === 'string'
    && Number.isFinite(Date.parse(value.adjustedAt))
}

function isReportVersion(value: unknown): value is ReportVersion {
  return object(value) && typeof value.id === 'string' && Number.isInteger(value.versionNo) && Number(value.versionNo) >= 1
    && typeof value.createdBy === 'string' && typeof value.createdAt === 'string' && Number.isFinite(Date.parse(value.createdAt))
    && isReportVersionContent(value.content)
}

function isReportDraft(value: unknown): value is ReportDraft {
  return object(value) && typeof value.id === 'string' && value.status === 'DRAFT'
    && Number.isInteger(value.rowVersion) && Number(value.rowVersion) > 0
    && Number.isInteger(value.versionNo) && Number(value.versionNo) > 0
    && typeof value.createdAt === 'string' && Number.isFinite(Date.parse(value.createdAt))
    && isReportVersionContent(value.content)
}

function isReportReview(value: unknown): value is ReportReview {
  return object(value) && typeof value.id === 'string' && Number.isInteger(value.versionNo)
    && typeof value.action === 'string' && typeof value.comment === 'string' && typeof value.actorId === 'string'
    && typeof value.createdAt === 'string' && Number.isFinite(Date.parse(value.createdAt))
}

function isReportCorrection(value: unknown): value is ReportCorrectionLog {
  return object(value) && typeof value.id === 'string' && Number.isInteger(value.versionNo)
    && typeof value.recordId === 'string' && typeof value.field === 'string' && typeof value.reason === 'string'
    && typeof value.factChanged === 'boolean' && typeof value.reconfirmationStatus === 'string'
    && typeof value.correctedBy === 'string' && typeof value.correctedAt === 'string' && Number.isFinite(Date.parse(value.correctedAt))
    && (value.decision === undefined || typeof value.decision === 'string')
    && (value.decisionComment === undefined || typeof value.decisionComment === 'string')
    && (value.decisionActorId === undefined || typeof value.decisionActorId === 'string')
    && (value.decisionCreatedAt === undefined || (typeof value.decisionCreatedAt === 'string' && Number.isFinite(Date.parse(value.decisionCreatedAt))))
}

function isReportWorkflow(value: unknown): value is ReportWorkflow {
  return object(value) && (value.report === null || isReportState(value.report))
    && Array.isArray(value.versions) && value.versions.every(isReportVersion)
    && Array.isArray(value.reviews) && value.reviews.every(isReportReview)
    && Array.isArray(value.corrections) && value.corrections.every(isReportCorrection)
}

function isReportArchiveEntry(value: unknown): value is ReportArchiveEntry {
  return object(value) && typeof value.id === 'string' && isPeriod(value.period)
    && ['DRAFT', 'PENDING_REVIEW', 'RETURNED', 'FINALIZING', 'FINALIZED', 'PUBLISHED', 'ARCHIVED'].includes(String(value.status))
    && Number.isInteger(value.versionNo) && Number(value.versionNo) > 0
    && Number.isInteger(value.versionCount) && Number(value.versionCount) >= Number(value.versionNo)
    && Number.isInteger(value.confirmedRecordCount) && Number(value.confirmedRecordCount) >= 0
    && Number.isInteger(value.confirmedZeroReceiptCount) && Number(value.confirmedZeroReceiptCount) >= 0
    && Number.isInteger(value.cellCount) && Number(value.cellCount) >= 0
    && typeof value.scopeLimited === 'boolean'
    && typeof value.updatedAt === 'string' && Number.isFinite(Date.parse(value.updatedAt))
    && (value.finalizedAt === undefined || (typeof value.finalizedAt === 'string' && Number.isFinite(Date.parse(value.finalizedAt))))
    && (value.publishedAt === undefined || (typeof value.publishedAt === 'string' && Number.isFinite(Date.parse(value.publishedAt))))
}

const summaryCountFields = [
  'submissionCount',
  'draftBatchCount',
  'pendingReviewBatchCount',
  'returnedBatchCount',
  'confirmedBatchCount',
  'confirmedRecordCount',
  'pendingReviewOrReturnedRecordCount',
  'zeroRecordPendingReviewBatchCount',
  'zeroRecordReturnedBatchCount',
  'zeroRecordConfirmedBatchCount',
] as const satisfies readonly (keyof ReportingSummaryTotals)[]

function totalsMatchRows(rows: ReportingSummaryRow[], totals: ReportingSummaryTotals): boolean {
  return summaryCountFields.every(field => rows.reduce((sum, row) => sum + row[field], 0) === totals[field])
}

export const reportingApi = {
  async periods(): Promise<ReportingPeriodPage> {
    const value = await apiRequest<unknown>('/reports/periods')
    if (!object(value) || !Array.isArray(value.items) || !value.items.every(isPeriod)) throw badResponse()
    return { items: value.items }
  },
  async summary(semesterId: string, weekId: string): Promise<ReportingSummary> {
    const params = new URLSearchParams({ semesterId, weekId })
    const value = await apiRequest<unknown>(`/reports/summary?${params.toString()}`)
    if (!object(value) || !isPeriod(value.period) || !Array.isArray(value.rows) || !value.rows.every(isSummaryRow) || !isTotals(value.totals)) throw badResponse()
    if (!totalsMatchRows(value.rows.filter(isSummaryRow), value.totals)) throw badResponse()
    return value as ReportingSummary
  },
  async workflow(semesterId: string, weekId: string): Promise<ReportWorkflow> {
    const params = new URLSearchParams({ semesterId, weekId })
    const value = await apiRequest<unknown>(`/reports/workflow?${params.toString()}`)
    if (!isReportWorkflow(value)) throw badResponse()
    return value
  },
  async archive(query: ReportArchiveQuery): Promise<ReportArchivePage> {
    const params = new URLSearchParams({ page: String(query.page), pageSize: String(query.pageSize) })
    if (query.status) params.set('status', query.status)
    const value = await apiRequest<unknown>(`/reports/archive?${params.toString()}`)
    if (!object(value) || !Array.isArray(value.items) || !value.items.every(isReportArchiveEntry)
      || !Number.isInteger(value.total) || Number(value.total) < 0
      || value.page !== query.page || value.pageSize !== query.pageSize) throw badResponse()
    return value as ReportArchivePage
  },
  async archiveWorkflow(semesterId: string, weekId: string): Promise<ReportWorkflow> {
    const value = await apiRequest<unknown>(`/reports/archive/${encodeURIComponent(semesterId)}/${encodeURIComponent(weekId)}`)
    if (!isReportWorkflow(value)) throw badResponse()
    return value
  },
  async versionRecords(query: ReportVersionRecordsQuery): Promise<ReportingRecordPage> {
    const params = new URLSearchParams({ page: String(query.page), pageSize: String(query.pageSize) })
    if (query.inspectionTypeId) {
      params.set('inspectionTypeId', query.inspectionTypeId)
      params.set('departmentId', query.departmentId ?? '')
      params.set('collegeLevel', String(query.collegeLevel === true))
    }
    const value = await apiRequest<unknown>(`/reports/workflow/${encodeURIComponent(query.semesterId)}/${encodeURIComponent(query.weekId)}/versions/${query.versionNo}/records?${params.toString()}`)
    if (!object(value) || !Array.isArray(value.items) || !value.items.every(isReportingRecord)
      || !Number.isInteger(value.total) || Number(value.total) < 0
      || value.page !== query.page || value.pageSize !== query.pageSize) throw badResponse()
    return value as ReportingRecordPage
  },
  async generateDraft(input: GenerateReportDraftInput): Promise<ReportDraft> {
    const value = await apiRequest<unknown>('/reports/drafts', { method: 'POST', body: JSON.stringify(input) })
    if (!isReportDraft(value)) throw badResponse()
    return value
  },
  async transition(reportId: string, action: ReportTransitionAction, input: ReportTransitionInput): Promise<ReportState> {
    const value = await apiRequest<unknown>(`/reports/${encodeURIComponent(reportId)}/${action}`, {
      method: 'POST', body: JSON.stringify(input),
    })
    if (!isReportState(value)) throw badResponse()
    return value
  },
  async addCorrection(reportId: string, input: AddReportCorrectionInput): Promise<ReportCorrectionResult> {
    const value = await apiRequest<unknown>(`/reports/${encodeURIComponent(reportId)}/corrections`, {
      method: 'POST', body: JSON.stringify(input),
    })
    if (!object(value) || typeof value.correctionId !== 'string' || !isReportState(value.report)) throw badResponse()
    return value as ReportCorrectionResult
  },
  async addAdjustment(reportId: string, input: AddReportAdjustmentInput): Promise<ReportAdjustmentResult> {
    const value = await apiRequest<unknown>(`/reports/${encodeURIComponent(reportId)}/adjustments`, {
      method: 'POST', body: JSON.stringify(input),
    })
    if (!object(value) || typeof value.adjustmentId !== 'string' || !isReportState(value.report)) throw badResponse()
    return value as ReportAdjustmentResult
  },
  async records(query: ReportingRecordsQuery): Promise<ReportingRecordPage> {
    const params = new URLSearchParams({
      semesterId: query.semesterId,
      weekId: query.weekId,
      departmentId: query.departmentId,
      collegeLevel: String(query.collegeLevel),
      inspectionTypeId: query.inspectionTypeId,
      page: String(query.page),
      pageSize: String(query.pageSize),
    })
    const value = await apiRequest<unknown>(`/reports/records?${params.toString()}`)
    if (!object(value) || !Array.isArray(value.items) || !value.items.every(isReportingRecord)
      || !Number.isInteger(value.total) || Number(value.total) < 0
      || value.page !== query.page || value.pageSize !== query.pageSize) throw badResponse()
    return value as ReportingRecordPage
  },
  async obligationOptions(semesterId: string, weekId: string): Promise<ReportObligationOptions> {
    const params = new URLSearchParams({ semesterId, weekId })
    const value = await apiRequest<unknown>(`/reports/obligation-options?${params.toString()}`)
    if (!object(value) || !isPeriod(value.period) || !Array.isArray(value.inspectionTypes)
      || !value.inspectionTypes.every(isReportObligationInspectionType) || !Array.isArray(value.departmentScopes)
      || !value.departmentScopes.every(isReportObligationDepartmentScope)) throw badResponse()
    return value as ReportObligationOptions
  },
  async obligations(semesterId: string, weekId: string): Promise<ReportObligation[]> {
    const params = new URLSearchParams({ semesterId, weekId })
    const value = await apiRequest<unknown>(`/reports/obligations?${params.toString()}`)
    if (!object(value) || !Array.isArray(value.items) || !value.items.every(isReportObligation)) throw badResponse()
    return value.items
  },
  async createObligation(input: CreateReportObligationInput): Promise<ReportObligation> {
    const value = await apiRequest<unknown>('/reports/obligations', { method: 'POST', body: JSON.stringify(input) })
    if (!isReportObligation(value)) throw badResponse()
    return value
  },
  async removeObligation(input: { id: string; version: number }): Promise<void> {
    await apiRequest<void>(`/reports/obligations/${encodeURIComponent(input.id)}`, {
      method: 'DELETE', body: JSON.stringify({ version: input.version }),
    })
  },
}

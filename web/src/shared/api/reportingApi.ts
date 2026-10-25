import type { CreateReportObligationInput, ReportObligation, ReportObligationDepartmentScope, ReportObligationInspectionType, ReportObligationOptions, ReportingPeriod, ReportingPeriodPage, ReportingRecord, ReportingRecordPage, ReportingRecordsQuery, ReportingSummary, ReportingSummaryRow, ReportingSummaryTotals } from '../../entities/reporting/model/reporting'
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

export type ReportingPeriod = {
  academicYearId: string
  academicYearName: string
  semesterId: string
  semesterName: string
  weekId: string
  weekNo: number
  startDate: string
  endDate: string
}

export type ReportingPeriodPage = { items: ReportingPeriod[] }

export type ReportingSummaryRow = {
  departmentId: string
  departmentName: string
  collegeLevel: boolean
  inspectionTypeId: string
  inspectionTypeName: string
  isRequired: boolean
  submissionCount: number
  draftBatchCount: number
  pendingReviewBatchCount: number
  returnedBatchCount: number
  confirmedBatchCount: number
  confirmedRecordCount: number
  pendingReviewOrReturnedRecordCount: number
  zeroRecordPendingReviewBatchCount: number
  zeroRecordReturnedBatchCount: number
  zeroRecordConfirmedBatchCount: number
}

export type ReportingSummaryTotals = Omit<ReportingSummaryRow, 'departmentId' | 'departmentName' | 'collegeLevel' | 'inspectionTypeId' | 'inspectionTypeName' | 'isRequired'>

export type ReportingSummary = {
  period: ReportingPeriod
  rows: ReportingSummaryRow[]
  totals: ReportingSummaryTotals
}

export type ReportCountRow = {
  departmentId: string
  departmentName: string
  collegeLevel: boolean
  inspectionTypeId: string
  inspectionType: string
  recordCount: number
  confirmedZeroReceiptCount: number
  dataStatus: string
}

export type ReportVersionContent = {
  period: ReportingPeriod
  scopeLimited: boolean
  statisticsRuleVersion: string
  confirmedRecordCount: number
  rows: ReportCountRow[]
  incompleteCells: { departmentId: string; departmentName: string; collegeLevel: boolean; inspectionTypeId: string; inspectionType: string }[]
  pendingDepartmentReconfirmationCount: number
  corrections: { id: string; recordId: string; field: string; factChanged: boolean; status: string }[]
  adjustments?: ReportAdjustmentSummary[] | null
  generatedAt: string
}

export type ReportAdjustmentSummary = {
  id: string
  recordId: string
  operation: 'INCLUDE' | 'EXCLUDE'
  reason: string
  record: ReportingRecord
  adjustedBy: string
  adjustedAt: string
}

export type ReportStatus = 'DRAFT' | 'PENDING_REVIEW' | 'RETURNED' | 'FINALIZING' | 'FINALIZED' | 'PUBLISHED' | 'ARCHIVED'

export type ReportState = {
  id: string
  semesterId: string
  weekId: string
  status: ReportStatus
  versionNo: number
  rowVersion: number
  updatedAt: string
}

export type ReportVersion = {
  id: string
  versionNo: number
  content: ReportVersionContent
  createdBy: string
  createdAt: string
}

export type ReportDraft = {
  id: string
  status: 'DRAFT'
  rowVersion: number
  versionNo: number
  content: ReportVersionContent
  createdAt: string
}

export type ReportReview = {
  id: string
  versionNo: number
  action: string
  comment: string
  actorId: string
  createdAt: string
}

export type ReportCorrectionLog = {
  id: string
  versionNo: number
  recordId: string
  field: string
  reason: string
  factChanged: boolean
  reconfirmationStatus: string
  correctedBy: string
  correctedAt: string
  decision?: string
  decisionComment?: string
  decisionActorId?: string
  decisionCreatedAt?: string
}

export type ReportWorkflow = {
  report: ReportState | null
  versions: ReportVersion[]
  reviews: ReportReview[]
  corrections: ReportCorrectionLog[]
}

export type ReportArchiveStatus = ReportStatus
export type ReportArchiveQuery = { page: number; pageSize: number; status?: ReportArchiveStatus }
export type ReportArchiveEntry = {
  id: string
  period: ReportingPeriod
  status: ReportArchiveStatus
  versionNo: number
  versionCount: number
  confirmedRecordCount: number
  confirmedZeroReceiptCount: number
  cellCount: number
  scopeLimited: boolean
  finalizedAt?: string
  publishedAt?: string
  updatedAt: string
}
export type ReportArchivePage = { items: ReportArchiveEntry[]; total: number; page: number; pageSize: number }

export type ReportVersionRecordsQuery = {
  semesterId: string
  weekId: string
  versionNo: number
  departmentId?: string
  collegeLevel?: boolean
  inspectionTypeId?: string
  page: number
  pageSize: number
}
export type AddReportCorrectionInput = { rowVersion: number; recordId: string; field: string; afterValue: string | number | null; reason: string; factChanged: boolean }
export type ReportCorrectionResult = { correctionId: string; report: ReportState }
export type AddReportAdjustmentInput = { rowVersion: number; recordId: string; operation: 'INCLUDE' | 'EXCLUDE'; reason: string }
export type ReportAdjustmentResult = { adjustmentId: string; report: ReportState }

export type GenerateReportDraftInput = { semesterId: string; weekId: string; expectedRowVersion: number }
export type ReportTransitionAction = 'submit' | 'return' | 'approve' | 'finalize'
export type ReportTransitionInput = { rowVersion: number; comment?: string }

export type ReportingRecordDetails = {
  date?: string
  time?: string
  period?: string
  round?: string
  absenceHours?: number
  penaltyHours?: number | null
  absenceCount?: number
  result?: string
  reasonCode?: string
  description?: string
  remark?: string
}

export type ReportingRecordReference = { label: string; grade?: string }

export type ReportingRecord = {
  id: string
  studentNumber: string
  studentName: string
  className: string
  grade: string
  typeCode: string
  details: ReportingRecordDetails
  currentDormitoryName: string
  currentCourseName: string
  currentClassroomName: string
  currentAreaName?: string
  currentReasonLabel?: string
  responsibleClasses?: ReportingRecordReference[]
  responsibleStudents?: ReportingRecordReference[]
  responsibleDormitories?: ReportingRecordReference[]
}

export type ReportingRecordsQuery = {
  semesterId: string
  weekId: string
  departmentId: string
  collegeLevel: boolean
  inspectionTypeId: string
  page: number
  pageSize: number
}

export type ReportingRecordPage = {
  items: ReportingRecord[]
  total: number
  page: number
  pageSize: number
}

export type ReportObligationPeriodQuery = { semesterId: string; weekId: string }

export type ReportObligation = {
  id: string
  semesterId: string
  weekId: string
  academicYearId: string
  departmentId: string
  departmentName: string
  collegeLevel: boolean
  inspectionTypeId: string
  inspectionTypeName: string
  eligible: boolean
  version: number
  createdAt: string
  updatedAt: string
  removedAt?: string
}

export type ReportObligationInspectionType = { id: string; name: string; sortOrder: number }
export type ReportObligationDepartmentScope = {
  departmentId: string
  departmentName: string
  inspectionTypeId: string
  inspectionTypeName: string
}

export type ReportObligationOptions = {
  period: ReportingPeriod
  inspectionTypes: ReportObligationInspectionType[]
  departmentScopes: ReportObligationDepartmentScope[]
}

export type CreateReportObligationInput = ReportObligationPeriodQuery & {
  departmentId: string
  inspectionTypeId: string
}

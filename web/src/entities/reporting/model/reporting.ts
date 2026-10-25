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

/** One batch belongs to one academic week and check type; college batches have no department. */
export type DepartmentSubmissionScope = {
  semesterId: string
  weekId: string
  departmentId: string
  inspectionTypeId: string
}

export type VersionedInspectionRecord = { id: string; version: number }
export type DepartmentSubmissionKind = 'RECORDS' | 'ZERO_RECORD'
export type SubmissionRecord = { id: string; rowVersion: number; status: string; createdBy: string }
export type DepartmentSubmission = {
  id: string
  scope: DepartmentSubmissionScope
  semesterName?: string
  weekNo?: number
  departmentName?: string
  status: 'PENDING_REVIEW' | 'RETURNED' | 'CONFIRMED'
  kind?: DepartmentSubmissionKind
  revisionNo: number
  rowVersion: number
  createdBy: string
  submittedBy: string
  reviewedBy?: string
  returnReason?: string
  createdAt: string
  submittedAt: string
  reviewedAt?: string
  recordIds: string[]
  records: SubmissionRecord[]
}

export type DepartmentSubmissionPage = { items: DepartmentSubmission[]; total: number; page: number; pageSize: number }

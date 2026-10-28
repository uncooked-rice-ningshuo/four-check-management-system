export type SubmissionCartRecord = {
  id: string
  semesterId: string
  weekId: string
  inspectionTypeId: string
  departmentId: string
  typeCode: string
  studentNumber: string
  studentName: string
  className: string
  grade: string
  status: string
  source: string
  importBatchId?: string
  importRowNo?: number
  rowVersion: number
  canEdit: boolean
  details: Record<string, unknown>
  /** Live reference labels resolved by the API for object-level inspections; empty for student records. */
  currentDormitoryName?: string
  currentClassroomName?: string
  currentAreaName?: string
  currentCourseName?: string
}

export type SubmissionCartBatch = {
  id: string
  semesterId: string
  weekId: string
  inspectionTypeId: string
  departmentId: string
  originalName: string
  status: string
  createdAt: string
  confirmedAt: string
  importedRows: number
  selectableRecords: number
  submittedRecords: number
  unavailableRecords: number
}

export type SubmissionCartBatchRecord = { record: SubmissionCartRecord; selectable: boolean; unavailableReason?: string }
export type SubmissionCartPage<T> = { items: T[]; total: number; page: number; pageSize: number }

export type MasterDataKind = 'faculties' | 'grades' | 'majors' | 'staff' | 'classes' | 'dormitory-areas' | 'dormitory-buildings' | 'dormitories' | 'teaching-buildings' | 'classrooms' | 'courses' | 'students'

export type MasterDataRecord = {
  id: string
  kind: MasterDataKind
  code?: string
  name: string
  enabled: boolean
  facultyId?: string
  majorId?: string
  headTeacherId?: string
  classId?: string
  dormitoryId?: string
  classroomId?: string
  teachingBuildingId?: string
  dormitoryAreaId?: string
  dormitoryBuildingId?: string
  gender?: string
  phone?: string
  grade?: string
  gradeId?: string
  enrolledSize?: number
  campusArea?: string
  buildingNo?: string
  roomNo?: string
  enrollmentStatus?: string
}

export type MasterDataPage = { items: MasterDataRecord[]; total: number; page: number; pageSize: number }
export type MasterDataChange = { id: string; entityType: string; entityId: string; action: string; beforeJson?: unknown; afterJson?: unknown; actorId: string; createdAt: string }
export type MasterDataImportField = { code: string; header: string; required: boolean; format: string; maxRunes?: number }
export type MasterDataImportSpec = { kind: MasterDataKind; templateCode: string; templateVersion: number; sheetName: string; label: string; fields: MasterDataImportField[] }
export type MasterDataImportRowError = { field: string; message: string }
export type MasterDataImportPreviewRow = { rowNo: number; values: Record<string, string>; status: 'VALID' | 'INVALID'; errors: MasterDataImportRowError[]; record?: MasterDataRecord }
export type MasterDataImportBatch = {
  id: string
  kind: MasterDataKind
  status: 'PREVIEW_READY' | 'PREVIEW_HAS_ERRORS' | 'CONFIRMED'
  templateVersion: number
  fields: MasterDataImportField[]
  sourceFileId?: string
  totalRows: number
  validRows: number
  invalidRows: number
  version: number
  rows: MasterDataImportPreviewRow[]
  createdAt: string
  confirmedAt?: string
}
export type MasterDataImportConfirmResult = { batchId: string; kind: MasterDataKind; status: 'CONFIRMED'; importedRows: number; version: number }

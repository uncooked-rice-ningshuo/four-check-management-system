import type { SemesterWeek } from './academic'

export type AcademicWeekImportField = {
  code: string
  header: string
  required: boolean
  format: string
}

export type AcademicWeekImportSpec = {
  templateCode: string
  templateVersion: number
  sheetName: string
  maxRows: number
  fields: AcademicWeekImportField[]
}

export type AcademicWeekImportRowError = { field: string; message: string }
export type AcademicWeekImportPreviewRow = {
  rowNo: number
  values: Record<string, string>
  status: 'VALID' | 'INVALID'
  errors: AcademicWeekImportRowError[]
  week?: SemesterWeek
}

export type AcademicWeekImportBatch = {
  batchId: string
  status: 'PREVIEW_READY' | 'PREVIEW_HAS_ERRORS' | 'CONFIRMED'
  version: number
  templateVersion: number
  semesterId: string
  semesterName: string
  academicYearName: string
  totalRows: number
  validRows: number
  invalidRows: number
  affectsToday: boolean
  rows: AcademicWeekImportPreviewRow[]
  createdAt: string
  confirmedAt?: string
}

export type AcademicWeekImportConfirmResult = {
  batchId: string
  status: 'CONFIRMED'
  importedRows: number
  version: number
}

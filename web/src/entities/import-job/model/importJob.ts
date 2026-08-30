import type { AttendanceRecord, Page } from '../../inspection/model/inspection'

export type ImportBatchStatus = 'UPLOADED' | 'PARSING' | 'PREVIEW_READY' | 'CONFIRMED' | 'PARSE_FAILED' | 'CONFIRM_FAILED' | 'CANCELLED'

export type ImportBatch = {
  id: string
  inspectionTypeId: string
  semesterId: string
  weekId: string
  departmentId: string
  templateId: string
  templateVersion: number
  status: ImportBatchStatus
  totalRows: number
  validRows: number
  invalidRows: number
  importedRows: number
  version: number
  failureCode?: string
  failureMessage?: string
}

export type ImportValidationError = { code: string; field: string; message: string; suggestion: string }
export type ImportRow = { id: string; batchId: string; rowNo: number; rawData: Record<string, string>; normalizedRecord: AttendanceRecord; validationStatus: 'VALID' | 'INVALID'; errors: ImportValidationError[]; recordId?: string }
export type ImportRowPage = Page<ImportRow>
export type ImportTemplate = { id: string; code?: string; name?: string; inspectionTypeId: string; version: number; fileId?: string; status?: string }
export type ConfirmImportResult = { batchId: string; status: string; importedRows: number; version: number }

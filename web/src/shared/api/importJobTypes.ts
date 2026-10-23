// Shared import transport contract; domain modules re-export these transport shapes.
export type ImportBatchStatus = 'UPLOADED' | 'PARSING' | 'PREVIEW_READY' | 'CONFIRMED' | 'PARSE_FAILED' | 'CONFIRM_FAILED' | 'CANCELLED' | 'ROLLED_BACK'

export type ImportBatch = {
  id: string
  inspectionTypeId: string
  semesterId: string
  weekId: string
  departmentId: string
  classId?: string
  grade?: string
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


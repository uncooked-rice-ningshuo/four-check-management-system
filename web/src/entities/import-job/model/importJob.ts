import type { AttendanceRecord, Page } from '../../inspection/model/inspection'

export type { ImportBatch, ImportBatchStatus } from '../../../shared/api/importJobTypes'

export type ImportValidationError = { code: string; field: string; message: string; suggestion: string }
export type ImportRowValidationFilter = 'ALL' | 'VALID' | 'INVALID'
export type ImportRow = { id: string; batchId: string; rowNo: number; rawData: Record<string, string>; normalizedRecord?: AttendanceRecord; validationStatus: 'VALID' | 'INVALID'; errors: ImportValidationError[]; recordId?: string }
export type ImportRowPage = Page<ImportRow>
export type ImportTemplate = { id: string; code?: string; name?: string; inspectionTypeId: string; version: number; fileId?: string; status?: string }
export type ConfirmImportResult = { batchId: string; status: string; importedRows: number; version: number }

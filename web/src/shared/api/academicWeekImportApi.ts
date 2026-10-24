import type { AcademicWeekImportBatch, AcademicWeekImportConfirmResult, AcademicWeekImportSpec } from '../../entities/academic/model/academicWeekImport'
import { apiDownload, apiRequest } from './httpClient'

const root = '/admin/academic/weeks'

export const academicWeekImportApi = {
  spec: () => apiRequest<AcademicWeekImportSpec>(`${root}/import-spec`),
  template: () => apiDownload(`${root}/import-template`),
  preview: (semesterId: string, file: File) => {
    const body = new FormData()
    body.set('semesterId', semesterId)
    body.set('file', file)
    return apiRequest<AcademicWeekImportBatch>(`${root}/imports`, { method: 'POST', body })
  },
  batch: (batchId: string) => apiRequest<AcademicWeekImportBatch>(`${root}/imports/${encodeURIComponent(batchId)}`),
  confirm: (batchId: string, version: number, idempotencyKey: string) => apiRequest<AcademicWeekImportConfirmResult>(`${root}/imports/${encodeURIComponent(batchId)}/confirm`, {
    method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, body: JSON.stringify({ version }),
  }),
}

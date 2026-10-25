import type { SubmissionCartBatch, SubmissionCartBatchRecord, SubmissionCartPage, SubmissionCartRecord } from '../../entities/inspection/model/submissionCart'
import { apiRequest } from './httpClient'
import { badResponse, object, queryString, type InspectionQuery } from './inspectionOptionsApi'

function record(value: unknown): SubmissionCartRecord {
  if (!object(value) || !object(value.details)) throw badResponse()
  for (const key of ['id', 'semesterId', 'weekId', 'inspectionTypeId', 'departmentId', 'typeCode', 'studentNumber', 'studentName', 'className', 'grade', 'status', 'source']) {
    if (typeof value[key] !== 'string') throw badResponse()
  }
  if (value.importBatchId !== undefined && typeof value.importBatchId !== 'string') throw badResponse()
  if (value.importRowNo !== undefined && !Number.isInteger(value.importRowNo)) throw badResponse()
  if (!Number.isInteger(value.rowVersion) || typeof value.canEdit !== 'boolean') throw badResponse()
  return value as SubmissionCartRecord
}

function page<T>(value: unknown, parse: (item: unknown) => T): SubmissionCartPage<T> {
  if (!object(value) || !Array.isArray(value.items) || !Number.isInteger(value.total) || !Number.isInteger(value.page) || !Number.isInteger(value.pageSize)) throw badResponse()
  return { items: value.items.map(parse), total: value.total as number, page: value.page as number, pageSize: value.pageSize as number }
}

function batch(value: unknown): SubmissionCartBatch {
  if (!object(value)) throw badResponse()
  for (const key of ['id', 'semesterId', 'weekId', 'inspectionTypeId', 'departmentId', 'originalName', 'status', 'createdAt', 'confirmedAt']) if (typeof value[key] !== 'string') throw badResponse()
  for (const key of ['importedRows', 'selectableRecords', 'submittedRecords', 'unavailableRecords']) if (!Number.isInteger(value[key])) throw badResponse()
  return value as SubmissionCartBatch
}

function batchRecord(value: unknown): SubmissionCartBatchRecord {
  if (!object(value) || typeof value.selectable !== 'boolean' || (value.unavailableReason !== undefined && typeof value.unavailableReason !== 'string')) throw badResponse()
  return { record: record(value.record), selectable: value.selectable, unavailableReason: value.unavailableReason as string | undefined }
}

export const submissionCartApi = {
  async candidates(query: InspectionQuery) {
    return page(await apiRequest<unknown>(`/inspections/submission-candidates?${queryString(query)}`), record)
  },
  async batches(query: InspectionQuery) {
    return page(await apiRequest<unknown>(`/imports?${queryString(query)}`), batch)
  },
  async batchRecords(batchId: string, query: InspectionQuery) {
    return page(await apiRequest<unknown>(`/imports/${encodeURIComponent(batchId)}/submission-records?${queryString(query)}`), batchRecord)
  },
  async detail(id: string) { return record(await apiRequest<unknown>(`/inspections/${encodeURIComponent(id)}`)) },
}

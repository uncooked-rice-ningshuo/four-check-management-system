import type { DepartmentSubmission, DepartmentSubmissionKind, DepartmentSubmissionPage, DepartmentSubmissionScope, VersionedInspectionRecord } from '../../entities/inspection/model/departmentSubmission'
import { apiRequest } from './httpClient'
import { badResponse, object, queryString, type InspectionQuery } from './inspectionOptionsApi'

function submission(value: unknown): DepartmentSubmission {
  if (!object(value) || !object(value.scope) || !Array.isArray(value.recordIds) || !value.recordIds.every(id => typeof id === 'string') || !Array.isArray(value.records)) throw badResponse()
  for (const key of ['semesterId', 'weekId', 'departmentId', 'inspectionTypeId']) if (typeof value.scope[key] !== 'string') throw badResponse()
  for (const key of ['id', 'status', 'createdBy', 'submittedBy', 'createdAt', 'submittedAt']) if (typeof value[key] !== 'string') throw badResponse()
  if (!['PENDING_REVIEW', 'RETURNED', 'CONFIRMED'].includes(value.status as string) || !Number.isInteger(value.revisionNo) || !Number.isInteger(value.rowVersion)) throw badResponse()
  const kind = value.kind ?? 'RECORDS'
  if (kind !== 'RECORDS' && kind !== 'ZERO_RECORD') throw badResponse()
  for (const key of ['reviewedBy', 'returnReason', 'reviewedAt']) if (value[key] !== undefined && typeof value[key] !== 'string') throw badResponse()
  for (const key of ['semesterName', 'departmentName']) if (value[key] !== undefined && typeof value[key] !== 'string') throw badResponse()
  if (value.weekNo !== undefined && !Number.isInteger(value.weekNo)) throw badResponse()
  if (!value.records.every(record => object(record) && typeof record.id === 'string' && Number.isInteger(record.rowVersion) && typeof record.status === 'string' && typeof record.createdBy === 'string')) throw badResponse()
  return { ...value, kind } as DepartmentSubmission
}

export const departmentSubmissionApi = {
  async list(query: InspectionQuery): Promise<DepartmentSubmissionPage> {
    const value = await apiRequest<unknown>(`/department-submissions?${queryString(query)}`)
    if (!object(value) || !Array.isArray(value.items) || !Number.isInteger(value.total) || !Number.isInteger(value.page) || !Number.isInteger(value.pageSize)) throw badResponse()
    return { items: value.items.map(submission), total: value.total as number, page: value.page as number, pageSize: value.pageSize as number }
  },
  async detail(id: string) { return submission(await apiRequest<unknown>(`/department-submissions/${encodeURIComponent(id)}`)) },
  async submit(scope: DepartmentSubmissionScope, records: VersionedInspectionRecord[], idempotencyKey: string,
    kind: DepartmentSubmissionKind = 'RECORDS', explanation = '') {
    return submission(await apiRequest<unknown>('/department-submissions', { method: 'POST', body: JSON.stringify({ scope, kind, records, idempotencyKey, explanation }) }))
  },
  async decide(id: string, action: 'return' | 'confirm', version: number, reason = '') {
    return submission(await apiRequest<unknown>(`/department-submissions/${encodeURIComponent(id)}/${action}`, { method: 'POST', body: JSON.stringify({ version, reason }) }))
  },
  async resubmit(id: string, version: number, records: VersionedInspectionRecord[],
    kind: DepartmentSubmissionKind = 'RECORDS', explanation = '') {
    return submission(await apiRequest<unknown>(`/department-submissions/${encodeURIComponent(id)}/resubmit`, { method: 'POST', body: JSON.stringify({ version, kind, records, explanation }) }))
  },
}

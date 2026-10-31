import { apiRequest } from './httpClient'
import { allOptions, badResponse, object, queryString, type ContextOptions, type InspectionOption, type InspectionQuery } from './inspectionOptionsApi'
export type { ContextOptions, InspectionOption, InspectionQuery } from './inspectionOptionsApi'

export type EveningSelfStudyDetails = {
 classId: string; studentId: string; classroomId: string; date: string; time: string
 period: '9-10'; penaltyMultiplier?: 1 | 2 | null; penaltyCount: number; remark: string
}
export type EveningSelfStudyRecordInput = {
 semesterId: string; weekId: string; inspectionTypeId: string; departmentId: string
 details: Pick<EveningSelfStudyDetails, 'classId' | 'studentId' | 'classroomId' | 'date' | 'time' | 'remark'>
}
export type EveningSelfStudyRecordDTO = EveningSelfStudyRecordInput & {
 id: string; typeCode: 'EVENING_SELF_STUDY'; studentNumber: string; studentName: string; className: string; grade: string
 currentClassroomName: string; status: string; source: string; rowVersion: number; createdBy: string; importBatchId?: string; canEdit: boolean
 details: EveningSelfStudyDetails
}

function record(value: unknown): EveningSelfStudyRecordDTO {
 if (!object(value) || !object(value.details)) throw badResponse()
 for (const key of ['id','semesterId','weekId','inspectionTypeId','departmentId','studentNumber','studentName','className','grade','currentClassroomName','status','source','createdBy']) if (typeof value[key] !== 'string') throw badResponse()
 for (const key of ['classId','studentId','classroomId','date','time']) if (typeof value.details[key] !== 'string') throw badResponse()
 if (value.typeCode !== 'EVENING_SELF_STUDY' || value.details.period !== '9-10' || (value.details.penaltyCount !== 1 && value.details.penaltyCount !== 2) || value.details.penaltyCount !== value.details.penaltyMultiplier) throw badResponse()
 if (value.details.remark !== undefined && typeof value.details.remark !== 'string') throw badResponse()
 for (const key of ['penaltyMultiplier']) if (value.details[key] !== undefined && value.details[key] !== null && typeof value.details[key] !== 'number') throw badResponse()
 if (value.importBatchId !== undefined && typeof value.importBatchId !== 'string') throw badResponse()
 if (!Number.isInteger(value.rowVersion) || typeof value.canEdit !== 'boolean') throw badResponse()
 return { ...value, details: { ...value.details, remark: value.details.remark ?? '' } } as EveningSelfStudyRecordDTO
}

export const eveningSelfStudyApi = {
 async contextOptions(query: InspectionQuery): Promise<ContextOptions> { return allOptions('/inspections/context-options', { typeCode: 'EVENING_SELF_STUDY', ...query }, true) },
 async entryOptions(query: InspectionQuery): Promise<InspectionOption[]> { return (await allOptions('/inspections/entry-options', query, false)).items },
 async list(query: InspectionQuery) {
  const value = await apiRequest<unknown>(`/inspections?${queryString(query)}`)
  if (!object(value) || !Array.isArray(value.items) || typeof value.total !== 'number' || typeof value.page !== 'number' || typeof value.pageSize !== 'number') throw badResponse()
  return { items: value.items.map(record), total: value.total, page: value.page, pageSize: value.pageSize }
 },
 async detail(id: string) { return record(await apiRequest<unknown>(`/inspections/${encodeURIComponent(id)}`)) },
 async create(input: EveningSelfStudyRecordInput) { return record(await apiRequest<unknown>('/inspections', { method: 'POST', body: JSON.stringify(input) })) },
 async update(input: { id: string; version: number; reason: string; record: EveningSelfStudyRecordInput }) { const { id, ...body } = input; return record(await apiRequest<unknown>(`/inspections/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(body) })) },
 async delete(items: { id: string; version: number }[], reason: string) { await apiRequest<void>('/inspections/batch', { method: 'POST', body: JSON.stringify({ action: 'DELETE', items, reason }) }) },
}

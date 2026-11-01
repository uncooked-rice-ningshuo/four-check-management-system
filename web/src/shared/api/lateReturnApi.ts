import { apiRequest } from './httpClient'
import { allOptions, badResponse, object, queryString, type ContextOptions, type InspectionOption, type InspectionQuery } from './inspectionOptionsApi'
export type { ContextOptions, InspectionOption, InspectionQuery } from './inspectionOptionsApi'

/** Transport contracts for late-return inspection endpoints. */
export type LateReturnDetails = { classId: string; studentId: string; dormitoryId: string; date: string; time?: string; remark: string }
export type LateReturnRecordInput = { semesterId: string; weekId: string; inspectionTypeId: string; departmentId: string; attributionMode?: 'DEPARTMENT'|'COLLEGE'|'ADMINISTRATOR'; details: Omit<LateReturnDetails, 'time'> }
export type LateReturnRecordDTO = Omit<LateReturnRecordInput, 'details'> & { details: LateReturnDetails } & {
 importBatchId?: string
 id: string; typeCode: string; studentNumber: string; studentName: string; className: string; grade: string
 currentDormitoryName: string; status: string; source: string; rowVersion: number; createdBy: string; submitterName: string; canEdit: boolean
}
function record(value: unknown): LateReturnRecordDTO {
 if (!object(value) || !object(value.details)) throw badResponse()
 if (value.attributionMode !== undefined && !['DEPARTMENT','COLLEGE','ADMINISTRATOR'].includes(String(value.attributionMode))) throw badResponse()
 for (const key of ['id','typeCode','semesterId','weekId','inspectionTypeId','departmentId','studentNumber','studentName','className','grade','currentDormitoryName','status','source','createdBy','submitterName']) if (typeof value[key] !== 'string') throw badResponse()
 for (const key of ['classId','studentId','dormitoryId','date']) if (typeof value.details[key] !== 'string') throw badResponse()
 if(value.details.time!==undefined&&typeof value.details.time!=='string')throw badResponse()
 if (value.details.remark !== undefined && typeof value.details.remark !== 'string') throw badResponse()
 if (!Number.isInteger(value.rowVersion) || typeof value.canEdit !== 'boolean') throw badResponse()
 if (value.typeCode !== 'LATE_RETURN' || (value.importBatchId !== undefined && typeof value.importBatchId !== 'string')) throw badResponse()
 // Fields are checked above because older servers must fail visibly, not masquerade as empty records.
 return { ...value, details: { ...value.details, remark: value.details.remark ?? '' } } as LateReturnRecordDTO
}
export const lateReturnApi = {
 async contextOptions(query: InspectionQuery): Promise<ContextOptions> { return allOptions('/inspections/context-options',{typeCode:'LATE_RETURN',...query},true) },
 async entryOptions(query: InspectionQuery): Promise<InspectionOption[]> { return (await allOptions('/inspections/entry-options',query,false)).items },
 async list(query: InspectionQuery) {
  const value=await apiRequest<unknown>(`/inspections?${queryString(query)}`)
  if(!object(value)||!Array.isArray(value.items)||typeof value.total!=='number'||typeof value.page!=='number'||typeof value.pageSize!=='number')throw badResponse()
  return {items:value.items.map(record),total:value.total,page:value.page,pageSize:value.pageSize}
 },
 async detail(id:string) { return record(await apiRequest<unknown>(`/inspections/${encodeURIComponent(id)}`)) },
 async create(input:LateReturnRecordInput) { return record(await apiRequest<unknown>('/inspections',{method:'POST',body:JSON.stringify(input)})) },
 async update(input:{id:string;version:number;reason:string;record:LateReturnRecordInput}) { const {id,...body}=input;return record(await apiRequest<unknown>(`/inspections/${encodeURIComponent(id)}`,{method:'PATCH',body:JSON.stringify(body)})) },
 async delete(items:{id:string;version:number}[],reason:string) { await apiRequest<void>('/inspections/batch',{method:'POST',body:JSON.stringify({action:'DELETE',items,reason})}) },
}

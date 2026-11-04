import { apiRequest } from './httpClient'
import { allOptions, badResponse, object, queryString, type ContextOptions, type InspectionOption, type InspectionQuery } from './inspectionOptionsApi'
import { validInspectionProvenance, type InspectionProvenance } from './inspectionProvenance'
export type { ContextOptions, InspectionOption, InspectionQuery } from './inspectionOptionsApi'

/** The server owns absenceCount; each record represents one student on one day. */
export type MorningAttendanceDetails = { classId: string; studentId: string; date: string; remark: string }
export type MorningAttendanceRecordInput = { semesterId: string; weekId: string; inspectionTypeId: string; departmentId: string; attributionMode?: 'DEPARTMENT'|'COLLEGE'|'ADMINISTRATOR'; details: MorningAttendanceDetails }
export type MorningAttendanceRecordDTO = MorningAttendanceRecordInput & InspectionProvenance & {
 importBatchId?: string
 id: string; typeCode: string; studentNumber: string; studentName: string; className: string; grade: string
 status: string; source: string; rowVersion: number; createdBy: string; submitterName: string; canEdit: boolean
}
function record(value: unknown): MorningAttendanceRecordDTO {
	if (!object(value) || !object(value.details) || !validInspectionProvenance(value)) throw badResponse()
 if (value.attributionMode !== undefined && !['DEPARTMENT','COLLEGE','ADMINISTRATOR'].includes(String(value.attributionMode))) throw badResponse()
 for (const key of ['id','typeCode','semesterId','weekId','inspectionTypeId','departmentId','studentNumber','studentName','className','grade','status','source','createdBy','submitterName']) if (typeof value[key] !== 'string') throw badResponse()
 if (value.importBatchId !== undefined && typeof value.importBatchId !== 'string') throw badResponse()
 if (value.typeCode !== 'MORNING_ATTENDANCE') throw badResponse()
 for (const key of ['classId','studentId','date']) if (typeof value.details[key] !== 'string') throw badResponse()
 if (value.details.remark !== undefined && typeof value.details.remark !== 'string') throw badResponse()
 if (value.details.absenceCount !== 1 || !Number.isInteger(value.rowVersion) || typeof value.canEdit !== 'boolean') throw badResponse()
 return { ...value, details: { classId:value.details.classId,studentId:value.details.studentId,date:value.details.date,remark:value.details.remark ?? '' } } as MorningAttendanceRecordDTO
}
export const morningAttendanceApi = {
 async contextOptions(query: InspectionQuery): Promise<ContextOptions> { return allOptions('/inspections/context-options',{typeCode:'MORNING_ATTENDANCE',...query},true) },
 async entryOptions(query: InspectionQuery): Promise<InspectionOption[]> { return (await allOptions('/inspections/entry-options',query,false)).items },
 async list(query: InspectionQuery) {
  const value=await apiRequest<unknown>(`/inspections?${queryString(query)}`)
  if(!object(value)||!Array.isArray(value.items)||typeof value.total!=='number'||typeof value.page!=='number'||typeof value.pageSize!=='number')throw badResponse()
  return {items:value.items.map(record),total:value.total,page:value.page,pageSize:value.pageSize}
 },
 async detail(id:string) { return record(await apiRequest<unknown>(`/inspections/${encodeURIComponent(id)}`)) },
 async create(input:MorningAttendanceRecordInput) { return record(await apiRequest<unknown>('/inspections',{method:'POST',body:JSON.stringify(input)})) },
 async update(input:{id:string;version:number;reason:string;record:MorningAttendanceRecordInput}) { const {id,...body}=input;return record(await apiRequest<unknown>(`/inspections/${encodeURIComponent(id)}`,{method:'PATCH',body:JSON.stringify(body)})) },
 async delete(items:{id:string;version:number}[],reason:string) { await apiRequest<void>('/inspections/batch',{method:'POST',body:JSON.stringify({action:'DELETE',items,reason})}) },
}

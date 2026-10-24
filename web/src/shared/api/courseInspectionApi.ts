import { apiRequest } from './httpClient'
import { allOptions, badResponse, object, queryString, type ContextOptions, type InspectionOption, type InspectionQuery } from './inspectionOptionsApi'
export type { ContextOptions, InspectionOption, InspectionQuery } from './inspectionOptionsApi'

/** Transport contracts for course-based inspection endpoints. */
export type CourseDetails = { classId: string; studentId: string; courseId: string; classroomId: string; date: string; time: string; period: string; absenceHours: number; remark: string; penaltyMultiplier?: 1 | 2 | null; penaltyHours?: number | null }
export type CourseRecordInput = { semesterId: string; weekId: string; inspectionTypeId: string; departmentId: string; details: Omit<CourseDetails, 'penaltyMultiplier' | 'penaltyHours'> }
export type CourseRecordDTO = Omit<CourseRecordInput, 'details'> & { details: CourseDetails } & {
 id: string; typeCode: string; studentNumber: string; studentName: string; className: string; grade: string
 currentCourseName: string; currentClassroomName: string; status: string; source: string; rowVersion: number; createdBy: string; importBatchId?: string; canEdit: boolean
}
export type InspectionOptionPage = { items: InspectionOption[]; total: number; page: number; pageSize: number }
function record(value: unknown): CourseRecordDTO {
 if (!object(value) || !object(value.details)) throw badResponse()
 for (const key of ['id','typeCode','semesterId','weekId','inspectionTypeId','departmentId','studentNumber','studentName','className','grade','currentCourseName','currentClassroomName','status','source','createdBy']) if (typeof value[key] !== 'string') throw badResponse()
 for (const key of ['classId','studentId','courseId','classroomId','date','time','period']) if (typeof value.details[key] !== 'string') throw badResponse()
 if (value.details.remark !== undefined && typeof value.details.remark !== 'string') throw badResponse()
 for (const key of ['penaltyMultiplier','penaltyHours']) if (value.details[key] !== undefined && value.details[key] !== null && typeof value.details[key] !== 'number') throw badResponse()
 if (value.importBatchId !== undefined && typeof value.importBatchId !== 'string') throw badResponse()
 if (!Number.isInteger(value.rowVersion) || typeof value.details.absenceHours !== 'number' || typeof value.canEdit !== 'boolean') throw badResponse()
 // Fields are checked above because older servers must fail visibly, not masquerade as empty records.
 return { ...value, details: { ...value.details, remark: value.details.remark ?? '' } } as CourseRecordDTO
}
export const courseInspectionApi = {
 async contextOptions(query: InspectionQuery): Promise<ContextOptions> { return allOptions('/inspections/context-options',{typeCode:'UNATTENDED_CLASSROOM',...query},true) },
 async entryOptions(query: InspectionQuery): Promise<InspectionOption[]> { return (await allOptions('/inspections/entry-options',query,false)).items },
 async courseOptions(query: InspectionQuery): Promise<InspectionOptionPage> {
  const value=await apiRequest<unknown>(`/inspections/entry-options?${queryString({...query,kind:'courses'})}`)
  if(!object(value)||!Array.isArray(value.items)||!value.items.every(item=>object(item)&&typeof item.id==='string'&&typeof item.label==='string')||!Number.isInteger(value.total)||typeof value.total!=='number'||!Number.isInteger(value.page)||typeof value.page!=='number'||!Number.isInteger(value.pageSize)||typeof value.pageSize!=='number')throw badResponse()
  return {items:value.items as InspectionOption[],total:value.total,page:value.page,pageSize:value.pageSize}
 },
 async list(query: InspectionQuery) {
  const value=await apiRequest<unknown>(`/inspections?${queryString(query)}`)
  if(!object(value)||!Array.isArray(value.items)||typeof value.total!=='number'||typeof value.page!=='number'||typeof value.pageSize!=='number')throw badResponse()
  return {items:value.items.map(record),total:value.total,page:value.page,pageSize:value.pageSize}
 },
 async detail(id:string) { return record(await apiRequest<unknown>(`/inspections/${encodeURIComponent(id)}`)) },
 async create(input:CourseRecordInput) { return record(await apiRequest<unknown>('/inspections',{method:'POST',body:JSON.stringify(input)})) },
 async update(input:{id:string;version:number;reason:string;record:CourseRecordInput}) { const {id,...body}=input;return record(await apiRequest<unknown>(`/inspections/${encodeURIComponent(id)}`,{method:'PATCH',body:JSON.stringify(body)})) },
 async delete(items:{id:string;version:number}[],reason:string) { await apiRequest<void>('/inspections/batch',{method:'POST',body:JSON.stringify({action:'DELETE',items,reason})}) },
}

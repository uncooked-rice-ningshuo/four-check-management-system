import { apiRequest } from './httpClient'
import { allOptions, badResponse, object, queryString, type ContextOptions, type InspectionOption, type InspectionQuery } from './inspectionOptionsApi'
export type { ContextOptions, InspectionOption, InspectionQuery } from './inspectionOptionsApi'

export type HygieneResult = 'EXCELLENT' | 'QUALIFIED' | 'UNQUALIFIED'
/** One inspected classroom area/event with authorized responsibility associations. */
export type ClassroomHygieneDetails = {
 classroomId:string;areaId:string;date:string;time?:string;round?:string;result:HygieneResult
 responsibleClassIds:string[];responsibleStudentIds?:string[];reasonCode?:string;description?:string;remark?:string
}
export type ClassroomHygieneInput = {semesterId:string;weekId:string;inspectionTypeId:string;departmentId:string;details:ClassroomHygieneDetails}
export type ClassroomHygieneRecordDTO = ClassroomHygieneInput & {
 id:string;typeCode:'CLASSROOM_HYGIENE';currentClassroomName:string;currentAreaName:string;currentReasonLabel?:string
 responsibleClasses:InspectionOption[];responsibleStudents:InspectionOption[]
 importBatchId?:string;status:string;source:string;rowVersion:number;createdBy:string;canEdit:boolean
}
function option(value:unknown):value is InspectionOption {return object(value)&&typeof value.id==='string'&&typeof value.label==='string'}
function record(value:unknown):ClassroomHygieneRecordDTO {
 if(!object(value)||!object(value.details)||value.typeCode!=='CLASSROOM_HYGIENE')throw badResponse()
 for(const key of ['id','semesterId','weekId','inspectionTypeId','departmentId','currentClassroomName','currentAreaName','status','source','createdBy'])if(typeof value[key]!=='string')throw badResponse()
 for(const key of ['classroomId','areaId','date'])if(typeof value.details[key]!=='string')throw badResponse()
 for(const key of ['time','round','reasonCode','description','remark'])if(value.details[key]!==undefined&&typeof value.details[key]!=='string')throw badResponse()
 if(!['EXCELLENT','QUALIFIED','UNQUALIFIED'].includes(String(value.details.result))||!Array.isArray(value.details.responsibleClassIds)||!value.details.responsibleClassIds.length||!value.details.responsibleClassIds.every(item=>typeof item==='string'))throw badResponse()
 if(value.details.responsibleStudentIds!==undefined&&(!Array.isArray(value.details.responsibleStudentIds)||!value.details.responsibleStudentIds.every(item=>typeof item==='string')))throw badResponse()
 if(!Array.isArray(value.responsibleClasses)||!value.responsibleClasses.length||!value.responsibleClasses.every(option))throw badResponse()
 if(value.responsibleStudents!==undefined&&(!Array.isArray(value.responsibleStudents)||!value.responsibleStudents.every(option)))throw badResponse()
 if(value.importBatchId!==undefined&&typeof value.importBatchId!=='string')throw badResponse()
 if(value.currentReasonLabel!==undefined&&typeof value.currentReasonLabel!=='string')throw badResponse()
 if(!Number.isInteger(value.rowVersion)||typeof value.canEdit!=='boolean')throw badResponse()
 // All required fields and optional arrays have been checked above; normalize legacy omissions only.
 return {...value,responsibleStudents:value.responsibleStudents??[],details:{...value.details,responsibleStudentIds:value.details.responsibleStudentIds??[],reasonCode:value.details.reasonCode??'',description:value.details.description??'',remark:value.details.remark??''}} as ClassroomHygieneRecordDTO
}
export const classroomHygieneApi={
 async contextOptions(query:InspectionQuery):Promise<ContextOptions>{return allOptions('/inspections/context-options',{typeCode:'CLASSROOM_HYGIENE',...query},true)},
 async entryOptions(query:InspectionQuery):Promise<InspectionOption[]>{return (await allOptions('/inspections/entry-options',query,false)).items},
 async list(query:InspectionQuery){
  const value=await apiRequest<unknown>(`/inspections?${queryString(query)}`)
  if(!object(value)||!Array.isArray(value.items)||typeof value.total!=='number'||typeof value.page!=='number'||typeof value.pageSize!=='number')throw badResponse()
  return {items:value.items.map(record),total:value.total,page:value.page,pageSize:value.pageSize}
 },
 async detail(id:string){return record(await apiRequest<unknown>(`/inspections/${encodeURIComponent(id)}`))},
 async create(input:ClassroomHygieneInput){return record(await apiRequest<unknown>('/inspections',{method:'POST',body:JSON.stringify(input)}))},
 async update(input:{id:string;version:number;reason:string;record:ClassroomHygieneInput}){const {id,...body}=input;return record(await apiRequest<unknown>(`/inspections/${encodeURIComponent(id)}`,{method:'PATCH',body:JSON.stringify(body)}))},
 async delete(items:{id:string;version:number}[],reason:string){await apiRequest<void>('/inspections/batch',{method:'POST',body:JSON.stringify({action:'DELETE',items,reason})})},
}

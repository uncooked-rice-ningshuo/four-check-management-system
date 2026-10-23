import { apiRequest } from './httpClient'
import { allOptions, badResponse, object, queryString, type ContextOptions, type InspectionOption, type InspectionQuery } from './inspectionOptionsApi'
export type { ContextOptions, InspectionOption, InspectionQuery } from './inspectionOptionsApi'

export type HygieneResult = 'EXCELLENT' | 'QUALIFIED' | 'UNQUALIFIED'
/** One inspected public area/event with authorized responsibility associations. */
export type PublicDormitoryHygieneDetails = {
 areaId:string;responsibleDormitoryIds?:string[];date:string;time?:string;round?:string;result:HygieneResult
 responsibleClassIds:string[];responsibleStudentIds?:string[];reasonCode?:string;description?:string;remark?:string
}
export type PublicDormitoryHygieneInput = {semesterId:string;weekId:string;inspectionTypeId:string;departmentId:string;details:PublicDormitoryHygieneDetails}
export type PublicDormitoryHygieneRecordDTO = PublicDormitoryHygieneInput & {
 id:string;typeCode:'PUBLIC_DORMITORY_HYGIENE';currentAreaName:string;currentReasonLabel?:string
 responsibleClasses:InspectionOption[];responsibleStudents:InspectionOption[];responsibleDormitories:InspectionOption[]
 status:string;source:string;rowVersion:number;createdBy:string;canEdit:boolean
}
function option(value:unknown):value is InspectionOption {return object(value)&&typeof value.id==='string'&&typeof value.label==='string'}
function record(value:unknown):PublicDormitoryHygieneRecordDTO {
 if(!object(value)||!object(value.details)||value.typeCode!=='PUBLIC_DORMITORY_HYGIENE')throw badResponse()
 for(const key of ['id','semesterId','weekId','inspectionTypeId','departmentId','currentAreaName','status','source','createdBy'])if(typeof value[key]!=='string')throw badResponse()
 for(const key of ['areaId','date'])if(typeof value.details[key]!=='string')throw badResponse()
 for(const key of ['time','round','reasonCode','description','remark'])if(value.details[key]!==undefined&&typeof value.details[key]!=='string')throw badResponse()
 if(!['EXCELLENT','QUALIFIED','UNQUALIFIED'].includes(String(value.details.result))||!Array.isArray(value.details.responsibleClassIds)||!value.details.responsibleClassIds.length||!value.details.responsibleClassIds.every(item=>typeof item==='string'))throw badResponse()
 if(value.details.responsibleStudentIds!==undefined&&(!Array.isArray(value.details.responsibleStudentIds)||!value.details.responsibleStudentIds.every(item=>typeof item==='string')))throw badResponse()
 if(!Array.isArray(value.responsibleClasses)||!value.responsibleClasses.length||!value.responsibleClasses.every(option))throw badResponse()
 if(value.responsibleStudents!==undefined&&(!Array.isArray(value.responsibleStudents)||!value.responsibleStudents.every(option)))throw badResponse()
 if(value.details.responsibleDormitoryIds!==undefined&&(!Array.isArray(value.details.responsibleDormitoryIds)||!value.details.responsibleDormitoryIds.every(item=>typeof item==='string')))throw badResponse()
 if(value.responsibleDormitories!==undefined&&(!Array.isArray(value.responsibleDormitories)||!value.responsibleDormitories.every(option)))throw badResponse()
 if(value.currentReasonLabel!==undefined&&typeof value.currentReasonLabel!=='string')throw badResponse()
 if(!Number.isInteger(value.rowVersion)||typeof value.canEdit!=='boolean')throw badResponse()
 // All required fields and optional arrays have been checked above; normalize legacy omissions only.
 return {...value,responsibleDormitories:value.responsibleDormitories??[],responsibleStudents:value.responsibleStudents??[],details:{...value.details,responsibleDormitoryIds:value.details.responsibleDormitoryIds??[],responsibleStudentIds:value.details.responsibleStudentIds??[],reasonCode:value.details.reasonCode??'',description:value.details.description??'',remark:value.details.remark??''}} as PublicDormitoryHygieneRecordDTO
}
export const publicDormitoryHygieneApi={
 async contextOptions(query:InspectionQuery):Promise<ContextOptions>{return allOptions('/inspections/context-options',{typeCode:'PUBLIC_DORMITORY_HYGIENE',...query},true)},
 async entryOptions(query:InspectionQuery):Promise<InspectionOption[]>{return (await allOptions('/inspections/entry-options',query,false)).items},
 async list(query:InspectionQuery){
  const value=await apiRequest<unknown>(`/inspections?${queryString(query)}`)
  if(!object(value)||!Array.isArray(value.items)||typeof value.total!=='number'||typeof value.page!=='number'||typeof value.pageSize!=='number')throw badResponse()
  return {items:value.items.map(record),total:value.total,page:value.page,pageSize:value.pageSize}
 },
 async detail(id:string){return record(await apiRequest<unknown>(`/inspections/${encodeURIComponent(id)}`))},
 async create(input:PublicDormitoryHygieneInput){return record(await apiRequest<unknown>('/inspections',{method:'POST',body:JSON.stringify(input)}))},
 async update(input:{id:string;version:number;reason:string;record:PublicDormitoryHygieneInput}){const {id,...body}=input;return record(await apiRequest<unknown>(`/inspections/${encodeURIComponent(id)}`,{method:'PATCH',body:JSON.stringify(body)}))},
 async delete(items:{id:string;version:number}[],reason:string){await apiRequest<void>('/inspections/batch',{method:'POST',body:JSON.stringify({action:'DELETE',items,reason})})},
}

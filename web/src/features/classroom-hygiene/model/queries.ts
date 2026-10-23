import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { classroomHygieneApi, type ClassroomHygieneInput, type InspectionQuery } from '../../../shared/api/classroomHygieneApi'

/** Read authorized context options separately for viewing and mutation targets. */
export function useInspectionContext(query: InspectionQuery, enabled=true) {
 return useQuery({queryKey:['classroom-hygienes','context',query],queryFn:()=>classroomHygieneApi.contextOptions(query),enabled})
}
/** Resolve complete reference choices for an explicit destination scope. */
export function useInspectionEntryOptions(query: InspectionQuery, enabled=true) {
 return useQuery({queryKey:['classroom-hygienes','entry-options',query],queryFn:()=>classroomHygieneApi.entryOptions(query),enabled})
}
/** Merge scoped student choices across every selected responsibility class. */
export function useResponsibleStudentOptions(query: InspectionQuery, classIds: string[], enabled=true) {
 return useQuery({queryKey:['classroom-hygienes','responsible-students',query,classIds],
  queryFn:async()=>{
   const pages=await Promise.all(classIds.map(classId=>classroomHygieneApi.entryOptions({...query,kind:'students',classId})))
   return pages.flat()
  },enabled:enabled&&classIds.length>0})
}
/** Cache record lists by their full structured filter. */
export function useClassroomHygieneRecords(query: InspectionQuery, enabled=true) {
 return useQuery({queryKey:['classroom-hygienes','records',query],queryFn:()=>classroomHygieneApi.list(query),enabled})
}
/** Retrieve the authoritative detail and mutation hint for one record. */
export function useClassroomHygieneRecord(id?:string) {
 return useQuery({queryKey:['classroom-hygienes','record',id],queryFn:()=>classroomHygieneApi.detail(id ?? ''),enabled:!!id})
}
/** Refresh lists, details and personal counts after all mutations, including conflicts. */
export function useClassroomHygieneMutation() {
 const client=useQueryClient()
 return useMutation({mutationFn:async(input:{record:ClassroomHygieneInput;id?:string;version?:number;reason?:string})=>input.id
  ?classroomHygieneApi.update({id:input.id,version:input.version??0,reason:input.reason??'',record:input.record})
  :classroomHygieneApi.create(input.record),
  onSettled:async()=>{await Promise.all([client.invalidateQueries({queryKey:['classroom-hygienes']}),client.invalidateQueries({queryKey:['dashboard', 'workbench']})])},
 })
}
/** Delete versioned drafts and refresh records and personal counts. */
export function useClassroomHygieneDelete() {
 const client=useQueryClient()
 return useMutation({mutationFn:(input:{items:{id:string;version:number}[];reason:string})=>classroomHygieneApi.delete(input.items,input.reason),
 onSettled:async()=>{await Promise.all([client.invalidateQueries({queryKey:['classroom-hygienes']}),client.invalidateQueries({queryKey:['dashboard', 'workbench']})])}})
}

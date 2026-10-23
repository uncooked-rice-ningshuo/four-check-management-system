import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { publicDormitoryHygieneApi, type PublicDormitoryHygieneInput, type InspectionQuery } from '../../../shared/api/publicDormitoryHygieneApi'

/** Read authorized context options separately for viewing and mutation targets. */
export function useInspectionContext(query: InspectionQuery, enabled=true) {
 return useQuery({queryKey:['public-dormitory-hygienes','context',query],queryFn:()=>publicDormitoryHygieneApi.contextOptions(query),enabled})
}
/** Resolve complete reference choices for an explicit destination scope. */
export function useInspectionEntryOptions(query: InspectionQuery, enabled=true) {
 return useQuery({queryKey:['public-dormitory-hygienes','entry-options',query],queryFn:()=>publicDormitoryHygieneApi.entryOptions(query),enabled})
}
/** Merge scoped student choices across every selected responsibility class. */
export function useResponsibleStudentOptions(query: InspectionQuery, classIds: string[], enabled=true) {
 return useQuery({queryKey:['public-dormitory-hygienes','responsible-students',query,classIds],
  queryFn:async()=>{
   const pages=await Promise.all(classIds.map(classId=>publicDormitoryHygieneApi.entryOptions({...query,kind:'students',classId})))
   return pages.flat()
  },enabled:enabled&&classIds.length>0})
}
/** Cache record lists by their full structured filter. */
export function usePublicDormitoryHygieneRecords(query: InspectionQuery, enabled=true) {
 return useQuery({queryKey:['public-dormitory-hygienes','records',query],queryFn:()=>publicDormitoryHygieneApi.list(query),enabled})
}
/** Retrieve the authoritative detail and mutation hint for one record. */
export function usePublicDormitoryHygieneRecord(id?:string) {
 return useQuery({queryKey:['public-dormitory-hygienes','record',id],queryFn:()=>publicDormitoryHygieneApi.detail(id ?? ''),enabled:!!id})
}
/** Refresh lists, details and personal counts after all mutations, including conflicts. */
export function usePublicDormitoryHygieneMutation() {
 const client=useQueryClient()
 return useMutation({mutationFn:async(input:{record:PublicDormitoryHygieneInput;id?:string;version?:number;reason?:string})=>input.id
  ?publicDormitoryHygieneApi.update({id:input.id,version:input.version??0,reason:input.reason??'',record:input.record})
  :publicDormitoryHygieneApi.create(input.record),
  onSettled:async()=>{await Promise.all([client.invalidateQueries({queryKey:['public-dormitory-hygienes']}),client.invalidateQueries({queryKey:['dashboard', 'workbench']})])},
 })
}
/** Delete versioned drafts and refresh records and personal counts. */
export function usePublicDormitoryHygieneDelete() {
 const client=useQueryClient()
 return useMutation({mutationFn:(input:{items:{id:string;version:number}[];reason:string})=>publicDormitoryHygieneApi.delete(input.items,input.reason),
 onSettled:async()=>{await Promise.all([client.invalidateQueries({queryKey:['public-dormitory-hygienes']}),client.invalidateQueries({queryKey:['dashboard', 'workbench']})])}})
}

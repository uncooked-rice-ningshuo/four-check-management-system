import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { dormitoryHygieneApi, type DormitoryHygieneInput, type InspectionQuery } from '../../../shared/api/dormitoryHygieneApi'

/** Read authorized context options separately for viewing and mutation targets. */
export function useInspectionContext(query: InspectionQuery, enabled=true) {
 return useQuery({queryKey:['dormitory-hygienes','context',query],queryFn:()=>dormitoryHygieneApi.contextOptions(query),enabled})
}
/** Resolve complete reference choices for an explicit destination scope. */
export function useInspectionEntryOptions(query: InspectionQuery, enabled=true) {
 return useQuery({queryKey:['dormitory-hygienes','entry-options',query],queryFn:()=>dormitoryHygieneApi.entryOptions(query),enabled})
}
/** Merge scoped student choices across every selected responsibility class. */
export function useResponsibleStudentOptions(query: InspectionQuery, classIds: string[], enabled=true) {
 return useQuery({queryKey:['dormitory-hygienes','responsible-students',query,classIds],
  queryFn:async()=>{
   const pages=await Promise.all(classIds.map(classId=>dormitoryHygieneApi.entryOptions({...query,kind:'students',classId})))
   return pages.flat()
  },enabled:enabled&&classIds.length>0})
}
/** Cache record lists by their full structured filter. */
export function useDormitoryHygieneRecords(query: InspectionQuery, enabled=true) {
 return useQuery({queryKey:['dormitory-hygienes','records',query],queryFn:()=>dormitoryHygieneApi.list(query),enabled})
}
/** Retrieve the authoritative detail and mutation hint for one record. */
export function useDormitoryHygieneRecord(id?:string) {
 return useQuery({queryKey:['dormitory-hygienes','record',id],queryFn:()=>dormitoryHygieneApi.detail(id ?? ''),enabled:!!id})
}
/** Refresh lists, details and personal counts after all mutations, including conflicts. */
export function useDormitoryHygieneMutation() {
 const client=useQueryClient()
 return useMutation({mutationFn:async(input:{record:DormitoryHygieneInput;id?:string;version?:number;reason?:string})=>input.id
  ?dormitoryHygieneApi.update({id:input.id,version:input.version??0,reason:input.reason??'',record:input.record})
  :dormitoryHygieneApi.create(input.record),
  onSettled:async()=>{await Promise.all([client.invalidateQueries({queryKey:['dormitory-hygienes']}),client.invalidateQueries({queryKey:['dashboard', 'workbench']})])},
 })
}
/** Delete versioned drafts and refresh records and personal counts. */
export function useDormitoryHygieneDelete() {
 const client=useQueryClient()
 return useMutation({mutationFn:(input:{items:{id:string;version:number}[];reason:string})=>dormitoryHygieneApi.delete(input.items,input.reason),
 onSettled:async()=>{await Promise.all([client.invalidateQueries({queryKey:['dormitory-hygienes']}),client.invalidateQueries({queryKey:['dashboard', 'workbench']})])}})
}

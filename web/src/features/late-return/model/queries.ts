import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { lateReturnApi, type LateReturnRecordInput, type InspectionQuery } from '../../../shared/api/lateReturnApi'

/** Read authorized context options separately for viewing and mutation targets. */
export function useInspectionContext(query: InspectionQuery, enabled=true) {
 return useQuery({queryKey:['late-returns','context',query],queryFn:()=>lateReturnApi.contextOptions(query),enabled})
}
/** Resolve complete reference choices for an explicit destination scope. */
export function useInspectionEntryOptions(query: InspectionQuery, enabled=true) {
 return useQuery({queryKey:['late-returns','entry-options',query],queryFn:()=>lateReturnApi.entryOptions(query),enabled})
}
/** Cache record lists by their full structured filter. */
export function useLateReturnRecords(query: InspectionQuery, enabled=true) {
 return useQuery({queryKey:['late-returns','records',query],queryFn:()=>lateReturnApi.list(query),enabled})
}
/** Retrieve the authoritative detail and mutation hint for one record. */
export function useLateReturnRecord(id?:string) {
 return useQuery({queryKey:['late-returns','record',id],queryFn:()=>lateReturnApi.detail(id ?? ''),enabled:!!id})
}
/** Refresh lists, details and personal counts after all mutations, including conflicts. */
export function useLateReturnMutation() {
 const client=useQueryClient()
 return useMutation({mutationFn:async(input:{record:LateReturnRecordInput;id?:string;version?:number;reason?:string})=>input.id
  ?lateReturnApi.update({id:input.id,version:input.version??0,reason:input.reason??'',record:input.record})
  :lateReturnApi.create(input.record),
  onSettled:async()=>{await Promise.all([client.invalidateQueries({queryKey:['late-returns']}),client.invalidateQueries({queryKey:['dashboard', 'workbench']})])},
 })
}
/** Delete versioned drafts and refresh records and personal counts. */
export function useLateReturnDelete() {
 const client=useQueryClient()
 return useMutation({mutationFn:(input:{items:{id:string;version:number}[];reason:string})=>lateReturnApi.delete(input.items,input.reason),
 onSettled:async()=>{await Promise.all([client.invalidateQueries({queryKey:['late-returns']}),client.invalidateQueries({queryKey:['dashboard', 'workbench']})])}})
}

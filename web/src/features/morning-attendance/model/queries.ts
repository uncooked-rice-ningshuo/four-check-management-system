import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { morningAttendanceApi, type MorningAttendanceRecordInput, type InspectionQuery } from '../../../shared/api/morningAttendanceApi'

/** Read authorized context options separately for viewing and mutation targets. */
export function useInspectionContext(query: InspectionQuery, enabled=true) {
 return useQuery({queryKey:['morning-attendances','context',query],queryFn:()=>morningAttendanceApi.contextOptions(query),enabled})
}
/** Resolve complete reference choices for an explicit destination scope. */
export function useInspectionEntryOptions(query: InspectionQuery, enabled=true) {
 return useQuery({queryKey:['morning-attendances','entry-options',query],queryFn:()=>morningAttendanceApi.entryOptions(query),enabled})
}
/** Cache record lists by their full structured filter. */
export function useMorningAttendanceRecords(query: InspectionQuery, enabled=true) {
 return useQuery({queryKey:['morning-attendances','records',query],queryFn:()=>morningAttendanceApi.list(query),enabled})
}
/** Retrieve the authoritative detail and mutation hint for one record. */
export function useMorningAttendanceRecord(id?:string) {
 return useQuery({queryKey:['morning-attendances','record',id],queryFn:()=>morningAttendanceApi.detail(id ?? ''),enabled:!!id})
}
/** Refresh lists, details and personal counts after all mutations, including conflicts. */
export function useMorningAttendanceMutation() {
 const client=useQueryClient()
 return useMutation({mutationFn:async(input:{record:MorningAttendanceRecordInput;id?:string;version?:number;reason?:string})=>input.id
  ?morningAttendanceApi.update({id:input.id,version:input.version??0,reason:input.reason??'',record:input.record})
  :morningAttendanceApi.create(input.record),
  onSettled:async()=>{await Promise.all([client.invalidateQueries({queryKey:['morning-attendances']}),client.invalidateQueries({queryKey:['dashboard', 'workbench']})])},
 })
}
/** Delete versioned drafts and refresh records and personal counts. */
export function useMorningAttendanceDelete() {
 const client=useQueryClient()
 return useMutation({mutationFn:(input:{items:{id:string;version:number}[];reason:string})=>morningAttendanceApi.delete(input.items,input.reason),
 onSettled:async()=>{await Promise.all([client.invalidateQueries({queryKey:['morning-attendances']}),client.invalidateQueries({queryKey:['dashboard', 'workbench']})])}})
}

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { courseInspectionApi, type CourseRecordInput, type InspectionQuery } from '../../../shared/api/courseInspectionApi'

/** Read authorized context options separately for viewing and mutation targets. */
export function useInspectionContext(query: InspectionQuery, enabled=true) {
 return useQuery({queryKey:['course-inspections','context',query],queryFn:()=>courseInspectionApi.contextOptions(query),enabled})
}
/** Resolve complete reference choices for an explicit destination scope. */
export function useInspectionEntryOptions(query: InspectionQuery, enabled=true) {
 return useQuery({queryKey:['course-inspections','entry-options',query],queryFn:()=>courseInspectionApi.entryOptions(query),enabled})
}
/** Cache record lists by their full structured filter. */
export function useCourseRecords(query: InspectionQuery, enabled=true) {
 return useQuery({queryKey:['course-inspections','records',query],queryFn:()=>courseInspectionApi.list(query),enabled})
}
/** Retrieve the authoritative detail and mutation hint for one record. */
export function useCourseRecord(id?:string) {
 return useQuery({queryKey:['course-inspections','record',id],queryFn:()=>courseInspectionApi.detail(id ?? ''),enabled:!!id})
}
/** Refresh lists, details and personal counts after all mutations, including conflicts. */
export function useCourseMutation() {
 const client=useQueryClient()
 return useMutation({mutationFn:async(input:{record:CourseRecordInput;id?:string;version?:number;reason?:string})=>input.id
  ?courseInspectionApi.update({id:input.id,version:input.version??0,reason:input.reason??'',record:input.record})
  :courseInspectionApi.create(input.record),
  onSettled:async()=>{await Promise.all([client.invalidateQueries({queryKey:['course-inspections']}),client.invalidateQueries({queryKey:['attendance']}),client.invalidateQueries({queryKey:['dashboard', 'workbench']})])},
 })
}
/** Delete versioned drafts and refresh records and personal counts. */
export function useCourseDelete() {
 const client=useQueryClient()
 return useMutation({mutationFn:(input:{items:{id:string;version:number}[];reason:string})=>courseInspectionApi.delete(input.items,input.reason),
 onSettled:async()=>{await Promise.all([client.invalidateQueries({queryKey:['course-inspections']}),client.invalidateQueries({queryKey:['dashboard', 'workbench']})])}})
}

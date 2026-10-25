import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { eveningSelfStudyApi, type EveningSelfStudyRecordInput, type InspectionQuery } from '../../../shared/api/eveningSelfStudyApi'

const root = ['evening-self-studies'] as const
export function useInspectionContext(query: InspectionQuery, enabled = true) { return useQuery({ queryKey: [...root, 'context', query], queryFn: () => eveningSelfStudyApi.contextOptions(query), enabled }) }
export function useInspectionEntryOptions(query: InspectionQuery, enabled = true) { return useQuery({ queryKey: [...root, 'entry-options', query], queryFn: () => eveningSelfStudyApi.entryOptions(query), enabled }) }
export function useEveningSelfStudyRecords(query: InspectionQuery, enabled = true) { return useQuery({ queryKey: [...root, 'records', query], queryFn: () => eveningSelfStudyApi.list(query), enabled }) }
export function useEveningSelfStudyMutation() {
 const client = useQueryClient()
 return useMutation({
  mutationFn: (input: { record: EveningSelfStudyRecordInput; id?: string; version?: number; reason?: string }) => input.id
   ? eveningSelfStudyApi.update({ id: input.id, version: input.version ?? 0, reason: input.reason ?? '', record: input.record })
   : eveningSelfStudyApi.create(input.record),
  onSettled: async () => Promise.all([client.invalidateQueries({ queryKey: root }), client.invalidateQueries({ queryKey: ['dashboard', 'workbench'] })]),
 })
}
export function useEveningSelfStudyDelete() {
 const client = useQueryClient()
 return useMutation({
  mutationFn: (input: { items: { id: string; version: number }[]; reason: string }) => eveningSelfStudyApi.delete(input.items, input.reason),
  onSettled: async () => Promise.all([client.invalidateQueries({ queryKey: root }), client.invalidateQueries({ queryKey: ['dashboard', 'workbench'] })]),
 })
}

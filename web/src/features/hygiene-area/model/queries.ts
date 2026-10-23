import { skipToken, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { hygieneAreaApi, type HygieneArea, type HygieneAreaTarget } from '../../../shared/api/hygieneAreaApi'

/** Live department/type grants shared by navigation and configuration. */
export function useHygieneAreaTargets(enabled = true) {
  return useQuery({ queryKey: ['hygiene-area-targets'], queryFn: hygieneAreaApi.targets, enabled })
}
/** Lists only the explicitly selected configuration target. */
export function useHygieneAreas(target: HygieneAreaTarget | undefined, page: number) {
  return useQuery({ queryKey: ['hygiene-areas', target?.kind, target?.departmentId, page], queryFn: target ? () => hygieneAreaApi.list(target.kind, target.departmentId, page) : skipToken })
}
/** Loads every page of authorized parent choices. */
export function useHygieneAreaReferences(target?: HygieneAreaTarget) {
  return useQuery({ queryKey: ['hygiene-area-references', target?.kind, target?.departmentId], queryFn: target ? () => hygieneAreaApi.references(target.kind, target.departmentId) : skipToken })
}
/** Reads immutable changes for the selected area. */
export function useHygieneAreaChanges(target?: HygieneAreaTarget, id?: string) {
  return useQuery({ queryKey: ['hygiene-area-changes', target?.kind, target?.departmentId, id], queryFn: target && id ? () => hygieneAreaApi.changes(target.kind, target.departmentId, id) : skipToken })
}
/** Invalidates current labels, entry choices and history after configuration changes. */
export function useHygieneAreaSave() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: ({ target, value, id }: { target: HygieneAreaTarget; value: Omit<HygieneArea, 'id' | 'kind'>; id?: string }) => id
      ? hygieneAreaApi.update(target.kind, target.departmentId, { ...value, id, kind: target.kind })
      : hygieneAreaApi.create(target.kind, target.departmentId, value),
    onSuccess: async () => {
      await Promise.all([
        client.invalidateQueries({ queryKey: ['hygiene-areas'] }),
        client.invalidateQueries({ queryKey: ['hygiene-area-changes'] }),
        client.invalidateQueries({ queryKey: ['course-inspections'] }),
        client.invalidateQueries({ queryKey: ['public-dormitory-hygienes'] }),
        client.invalidateQueries({ queryKey: ['classroom-hygienes'] }),
      ])
    },
  })
}

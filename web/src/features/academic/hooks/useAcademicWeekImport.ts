import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { academicWeekImportApi } from '../../../shared/api/academicWeekImportApi'

export const academicWeekImportKeys = {
  spec: ['academic-week-import', 'spec'] as const,
  template: ['academic-week-import', 'template'] as const,
}

export function useAcademicWeekImportSpec(enabled: boolean) {
  return useQuery({ queryKey: academicWeekImportKeys.spec, queryFn: academicWeekImportApi.spec, enabled })
}

export function useAcademicWeekImportTemplate(enabled: boolean) {
  return useQuery({ queryKey: academicWeekImportKeys.template, queryFn: academicWeekImportApi.template, enabled })
}

export function usePreviewAcademicWeekImport() {
  return useMutation({ mutationFn: ({ semesterId, file }: { semesterId: string; file: File }) => academicWeekImportApi.preview(semesterId, file) })
}

export function useConfirmAcademicWeekImport(batchId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: ({ version, idempotencyKey }: { version: number; idempotencyKey: string }) => academicWeekImportApi.confirm(batchId, version, idempotencyKey),
    onSuccess: async () => {
      await Promise.all([
        client.invalidateQueries({ queryKey: ['academic-calendar'] }),
        client.invalidateQueries({ queryKey: ['work-context'] }),
        client.invalidateQueries({ queryKey: ['academic', 'current-week'] }),
        client.invalidateQueries({ queryKey: ['course-inspections', 'context'] }),
        client.invalidateQueries({ queryKey: ['academic', 'calendar', 'override'] }),
        client.invalidateQueries({ queryKey: ['dashboard', 'workbench'] }),
        client.invalidateQueries({ queryKey: ['attendance', 'filter-options'] }),
      ])
    },
  })
}

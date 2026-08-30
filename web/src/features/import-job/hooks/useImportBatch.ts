import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { InspectionContext } from '../../../entities/inspection/model/inspection'
import type { ImportTemplate } from '../../../entities/import-job/model/importJob'
import { attendanceKeys } from '../../attendance/hooks/useAttendanceRecords'
import { importJobApi } from '../api/importJobApi'

export const importJobKeys = {
  all: ['import-jobs'] as const,
  batch: (id: string) => [...importJobKeys.all, 'batch', id] as const,
  rows: (id: string, page: number, pageSize: number) => [...importJobKeys.batch(id), 'rows', page, pageSize] as const,
}

export function useImportBatch(id?: string) {
  return useQuery({ queryKey: importJobKeys.batch(id ?? ''), queryFn: () => importJobApi.batch(id!), enabled: Boolean(id) })
}

export function useImportRows(id: string | undefined, page: number, pageSize: number) {
  return useQuery({ queryKey: importJobKeys.rows(id ?? '', page, pageSize), queryFn: () => importJobApi.rows(id!, page, pageSize), enabled: Boolean(id) })
}

export function useUploadAttendanceImport() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: ({ context, file, template }: { context: InspectionContext; file: File; template: Pick<ImportTemplate, 'id' | 'version'> }) => importJobApi.uploadAttendance(context, file, template),
    onSuccess: (batch) => client.invalidateQueries({ queryKey: importJobKeys.batch(batch.id) }),
  })
}

export function useConfirmImport(context: InspectionContext, batchId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: ({ version, idempotencyKey }: { version: number; idempotencyKey: string }) => importJobApi.confirm(batchId, version, idempotencyKey),
    onSuccess: async () => {
      await Promise.all([
        client.invalidateQueries({ queryKey: importJobKeys.batch(batchId) }),
		client.invalidateQueries({
			queryKey: attendanceKeys.all,
			predicate: query => {
				const filters = query.queryKey[2]
				return typeof filters === 'object' && filters !== null &&
					Object.entries(context).every(([key, value]) => (filters as Record<string, unknown>)[key] === value)
			},
		}),
      ])
    },
  })
}

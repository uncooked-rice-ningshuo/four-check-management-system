import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { InspectionContext } from '../../../entities/inspection/model/inspection'
import type { ImportRowValidationFilter, ImportTemplate } from '../../../entities/import-job/model/importJob'
import { ApiError } from '../../../shared/api/httpClient'
import { attendanceKeys } from '../../attendance/hooks/useAttendanceRecords'
import { inspectionImportApi, type SupportedImportType } from '../../../shared/api/inspectionImportApi'
import { courseImportApi } from '../../../shared/api/courseImportApi'
import { importJobApi } from '../api/importJobApi'

export const importJobKeys = {
  all: ['import-jobs'] as const,
  template: (context: InspectionContext) => [...importJobKeys.all, 'template', context] as const,
  batch: (id: string) => [...importJobKeys.all, 'batch', id] as const,
  rows: (id: string, page: number, pageSize: number, filter: ImportRowValidationFilter) => [...importJobKeys.batch(id), 'rows', page, pageSize, filter] as const,
}

export function useImportTemplate(context: InspectionContext, enabled: boolean, typeCode: SupportedImportType = 'ATTENDANCE_RATE') {
  return useQuery({ queryKey: importJobKeys.template(context), queryFn: () => ((typeCode === 'CLASSROOM_HYGIENE' || typeCode === 'DORMITORY_HYGIENE' || typeCode === 'MORNING_ATTENDANCE' || typeCode === 'LATE_RETURN') ? inspectionImportApi : typeCode === 'UNATTENDED_CLASSROOM' ? courseImportApi : importJobApi).template(context, true), enabled })
}

export function useImportBatch(id?: string) {
  return useQuery({
    queryKey: importJobKeys.batch(id ?? ''),
    queryFn: () => importJobApi.batch(id!),
    enabled: Boolean(id),
    refetchInterval: query => ['UPLOADED', 'PARSING'].includes(query.state.data?.status ?? '') ? 1000 : false,
  })
}

export function useImportRows(id: string | undefined, page: number, pageSize: number, filter: ImportRowValidationFilter) {
  return useQuery({
    queryKey: importJobKeys.rows(id ?? '', page, pageSize, filter),
    queryFn: () => importJobApi.rows(id!, page, pageSize, filter === 'ALL' ? undefined : filter),
    enabled: Boolean(id),
  })
}

export function useUploadAttendanceImport() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: ({ context, file, template, typeCode }: { typeCode?: SupportedImportType; context: InspectionContext; file: File; template: Pick<ImportTemplate, 'id' | 'version'> }) => (typeCode === 'CLASSROOM_HYGIENE' || typeCode === 'DORMITORY_HYGIENE' || typeCode === 'MORNING_ATTENDANCE' || typeCode === 'LATE_RETURN') ? inspectionImportApi.upload(context, file, template) : (typeCode === 'UNATTENDED_CLASSROOM' ? courseImportApi : importJobApi).uploadAttendance(context, file, template),
    onSuccess: (batch) => client.invalidateQueries({ queryKey: importJobKeys.batch(batch.id) }),
  })
}

export function useConfirmImport(batchId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: ({ version, idempotencyKey }: { version: number; idempotencyKey: string }) => importJobApi.confirm(batchId, version, idempotencyKey),
    onError: async error => {
      if (error instanceof ApiError && error.status === 409) {
        await client.invalidateQueries({ queryKey: importJobKeys.batch(batchId) })
      }
    },
    onSuccess: async () => {
      await Promise.all([
        client.invalidateQueries({ queryKey: importJobKeys.batch(batchId) }),
        // An import also changes unfiltered lists and the personal workbench.
        client.invalidateQueries({ queryKey: attendanceKeys.all }),
        client.invalidateQueries({ queryKey: ['course-inspections'] }),
        client.invalidateQueries({ queryKey: ['morning-attendances'] }),
        client.invalidateQueries({ queryKey: ['late-returns'] }),
        client.invalidateQueries({ queryKey: ['dormitory-hygienes'] }),
        client.invalidateQueries({ queryKey: ['classroom-hygienes'] }),
        client.invalidateQueries({ queryKey: ['dashboard', 'workbench'] }),
      ])
    },
  })
}

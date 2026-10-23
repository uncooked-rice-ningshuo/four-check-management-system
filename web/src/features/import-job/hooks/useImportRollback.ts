import { useMutation, useQueryClient } from '@tanstack/react-query'
import type { ImportBatch } from '../../../entities/import-job/model/importJob'
import { rollbackImportBatch } from '../../../shared/api/importRollbackApi'
import { importJobKeys } from './useImportBatch'

/** A rollback changes batch status, all inspection lists and personal counts. */
export function useImportRollback(batchId: string) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: ({ version, reason }: { version: number; reason: string }) => rollbackImportBatch(batchId, version, reason),
    onSuccess: result => {
      client.setQueryData<ImportBatch>(importJobKeys.batch(batchId), current => current ? { ...current, status: result.status, version: result.version } : current)
    },
    onSettled: async () => {
      await Promise.all([
        client.invalidateQueries({ queryKey: importJobKeys.batch(batchId) }),
        ...['attendance', 'course-inspections', 'late-returns', 'morning-attendances', 'dormitory-hygienes', 'public-dormitory-hygienes', 'classroom-hygienes'].map(key => client.invalidateQueries({ queryKey: [key] })),
        client.invalidateQueries({ queryKey: ['dashboard', 'workbench'] }),
      ])
    },
  })
}

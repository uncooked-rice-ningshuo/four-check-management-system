import { expect, it, vi } from 'vitest'
import { apiRequest } from './httpClient'
import { rollbackImportBatch } from './importRollbackApi'
vi.mock('./httpClient', async original => ({ ...await original<typeof import('./httpClient')>(), apiRequest: vi.fn() }))
it('sends version and reason and rejects an incomplete or wrong batch response', async () => {
  vi.mocked(apiRequest).mockResolvedValueOnce({ batchId: 'batch', status: 'ROLLED_BACK', rolledBackRows: 2, version: 4 })
  await expect(rollbackImportBatch('batch', 3, '整批误导入')).resolves.toMatchObject({ version: 4 })
  expect(apiRequest).toHaveBeenCalledWith('/imports/batch/rollback', { method: 'POST', body: JSON.stringify({ version: 3, reason: '整批误导入' }) })
  for (const value of [null, {}, { batchId: 'other', status: 'ROLLED_BACK', rolledBackRows: 2, version: 4 }]) {
    vi.mocked(apiRequest).mockResolvedValueOnce(value)
    await expect(rollbackImportBatch('batch', 3, '整批误导入')).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  }
})

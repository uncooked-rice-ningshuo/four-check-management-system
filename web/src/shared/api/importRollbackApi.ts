import { apiRequest } from './httpClient'
import { badResponse, object } from './inspectionOptionsApi'

export type ImportRollbackResult = { batchId: string; status: 'ROLLED_BACK'; rolledBackRows: number; version: number }

/** Undo one complete confirmed batch with an explicit version and audit reason. */
export async function rollbackImportBatch(id: string, version: number, reason: string): Promise<ImportRollbackResult> {
  const value = await apiRequest<unknown>(`/imports/${encodeURIComponent(id)}/rollback`, { method: 'POST', body: JSON.stringify({ version, reason }) })
  if (!object(value) || value.batchId !== id || value.status !== 'ROLLED_BACK' || typeof value.rolledBackRows !== 'number' || !Number.isInteger(value.rolledBackRows) || value.rolledBackRows < 1 || typeof value.version !== 'number' || !Number.isInteger(value.version) || value.version < 1) throw badResponse()
  return value as ImportRollbackResult
}

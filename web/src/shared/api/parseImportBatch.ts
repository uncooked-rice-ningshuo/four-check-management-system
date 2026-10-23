import type { ImportBatch } from './importJobTypes'
import { departmentOptionId } from '../../entities/inspection/model/inspectionScope'
import { badResponse, object } from './inspectionOptionsApi'

const statuses = new Set(['UPLOADED','PARSING','PREVIEW_READY','CONFIRMED','PARSE_FAILED','CONFIRM_FAILED','CANCELLED','ROLLED_BACK'])
/** Validate batch identity, counters and an optional authoritative upload destination. */
export function parseImportBatch(value: unknown, expected?: Pick<ImportBatch, 'inspectionTypeId' | 'semesterId' | 'weekId' | 'departmentId' | 'classId' | 'grade'>): ImportBatch {
  if (!object(value) || !['id','inspectionTypeId','semesterId','weekId','templateId'].every(key => typeof value[key] === 'string' && value[key].trim()) || typeof value.departmentId !== 'string' || !['templateVersion','version'].every(key => typeof value[key] === 'number' && Number.isInteger(value[key]) && value[key] > 0) || !['totalRows','validRows','invalidRows','importedRows'].every(key => typeof value[key] === 'number' && Number.isInteger(value[key]) && value[key] >= 0) || typeof value.status !== 'string' || !statuses.has(value.status)) throw badResponse()
  if (!object(value) || ['classId', 'grade'].some(key => value[key] !== undefined && typeof value[key] !== 'string')) throw badResponse()
  if (expected && (['inspectionTypeId', 'semesterId', 'weekId', 'classId', 'grade'] as const).some(key => (value[key] || undefined) !== (expected[key] || undefined))) throw badResponse()
  if (expected && departmentOptionId(value.departmentId) !== departmentOptionId(expected.departmentId)) throw badResponse()
  // The required transport fields have been validated above.
  return value as ImportBatch
}

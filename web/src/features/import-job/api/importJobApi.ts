import type { InspectionContext } from '../../../entities/inspection/model/inspection'
import type { ConfirmImportResult, ImportRowPage, ImportTemplate } from '../../../entities/import-job/model/importJob'
import { ApiError, apiDownload, apiRequest } from '../../../shared/api/httpClient'
import { attendanceQuery } from '../../attendance/api/attendanceApi'

import { parseImportBatch } from '../../../shared/api/parseImportBatch'

export const importJobApi = {
  async template(context: InspectionContext, withExample = false) {
		const scope = importScope(context)
		const download = await apiDownload(`/imports/attendance/template?${attendanceQuery(scope)}${withExample ? '&example=1' : ''}`)
		const id = download.headers.get('X-Import-Template-ID') ?? ''
		const version = Number(download.headers.get('X-Import-Template-Version'))
		if (!id || !Number.isInteger(version) || version < 1) throw new ApiError(502, 'INVALID_TEMPLATE_RESPONSE', '导入模板响应无效')
		return { file: download.blob, template: { id, version } }
  },
  uploadAttendance(context: InspectionContext, file: File, template: Pick<ImportTemplate, 'id' | 'version'>) {
    const body = new FormData()
    Object.entries(importScope(context)).forEach(([key, value]) => { if (value) body.set(key, value) })
    body.set('templateId', template.id)
    body.set('templateVersion', String(template.version))
    body.set('file', file)
    return apiRequest<unknown>('/imports/attendance', { method: 'POST', body }).then(value => parseImportBatch(value, importScope(context)))
  },
  batch(id: string) { return apiRequest<unknown>(`/imports/${encodeURIComponent(id)}`).then(parseImportBatch) },
  rows(id: string, page: number, pageSize: number, validationStatus?: 'VALID' | 'INVALID') {
    const filterQuery = validationStatus ? `&validationStatus=${validationStatus}` : ''
    return apiRequest<ImportRowPage>(`/imports/${encodeURIComponent(id)}/rows?page=${page}&pageSize=${pageSize}${filterQuery}`)
  },
  confirm(id: string, version: number, idempotencyKey: string) {
    return apiRequest<ConfirmImportResult>(`/imports/${encodeURIComponent(id)}/confirm`, { method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, body: JSON.stringify({ version }) })
  },
  cancel(id: string, version: number) { return apiRequest<unknown>(`/imports/${encodeURIComponent(id)}/cancel`, { method: 'POST', body: JSON.stringify({ version }) }).then(parseImportBatch) },
}

function importScope(context: InspectionContext) {
  return {
    semesterId: context.semesterId,
    weekId: context.weekId,
    inspectionTypeId: context.inspectionTypeId,
    departmentId: context.departmentId,
  }
}

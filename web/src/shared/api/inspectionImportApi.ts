import { parseImportBatch } from './parseImportBatch'
import { ApiError, apiDownload, apiRequest } from './httpClient'
import { queryString } from './inspectionOptionsApi'

export type SupportedImportType = 'ATTENDANCE_RATE' | 'UNATTENDED_CLASSROOM' | 'MORNING_ATTENDANCE' | 'LATE_RETURN' | 'DORMITORY_HYGIENE' | 'CLASSROOM_HYGIENE'
export type InspectionImportScope = { semesterId: string; weekId: string; inspectionTypeId: string; departmentId: string; classId?: string; grade?: string }
/** Share multipart/template contracts while retaining legacy endpoint boundaries. */
export function createInspectionImportClient(root: '/imports/course-records' | '/imports/inspections') {
  return {
    async template(context: InspectionImportScope) {
      const download = await apiDownload(`${root}/template?${queryString({ ...context })}`)
      const id = download.headers.get('X-Import-Template-ID') ?? ''
      const version = Number(download.headers.get('X-Import-Template-Version'))
      if (!id.trim() || !Number.isInteger(version) || version < 1) throw new ApiError(502, 'INVALID_TEMPLATE_RESPONSE', '导入模板响应无效')
      return { file: download.blob, template: { id, version } }
    },
    async upload(context: InspectionImportScope, file: File, template: { id: string; version: number }) {
      const body = new FormData()
      Object.entries(context).forEach(([key, value]) => { if (value) body.set(key, value) })
      body.set('templateId', template.id)
      body.set('templateVersion', String(template.version))
      body.set('file', file)
      const batch = parseImportBatch(await apiRequest<unknown>(root, { method: 'POST', body }), context)
      return batch
    },
  }
}
export const inspectionImportApi = createInspectionImportClient('/imports/inspections')

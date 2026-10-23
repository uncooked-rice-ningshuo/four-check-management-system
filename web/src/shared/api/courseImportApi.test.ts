import { beforeEach, expect, it, vi } from 'vitest'
import { apiDownload, apiRequest } from './httpClient'
import { courseImportApi } from './courseImportApi'
vi.mock('./httpClient', async original => ({ ...await original<typeof import('./httpClient')>(), apiDownload: vi.fn(), apiRequest: vi.fn() }))
const scope = { semesterId: 'semester', weekId: 'week', inspectionTypeId: 'unattended', departmentId: 'department' }
beforeEach(() => vi.clearAllMocks())
it('requires published template identity and a positive integer version', async () => {
  for (const headers of [new Headers(), new Headers({ 'X-Import-Template-ID': 'template', 'X-Import-Template-Version': '0' }), new Headers({ 'X-Import-Template-ID': 'template', 'X-Import-Template-Version': '1.5' })]) {
    vi.mocked(apiDownload).mockResolvedValueOnce({ blob: new Blob(), headers })
    await expect(courseImportApi.template(scope)).rejects.toMatchObject({ code: 'INVALID_TEMPLATE_RESPONSE' })
  }
  vi.mocked(apiDownload).mockResolvedValueOnce({ blob: new Blob(), headers: new Headers({ 'X-Import-Template-ID': 'template', 'X-Import-Template-Version': '1' }) })
  await expect(courseImportApi.template(scope)).resolves.toMatchObject({ template: { id: 'template', version: 1 } })
  expect(apiDownload).toHaveBeenLastCalledWith(expect.stringContaining('/imports/course-records/template?'))
})
it('uploads the independently chosen scope and template through the common HTTP client', async () => {
  const file = new File(['fixture'], 'unattended.xlsx')
  vi.mocked(apiRequest).mockResolvedValueOnce({ ...scope, id: 'batch', templateId: 'template', templateVersion: 1, version: 3, status: 'PREVIEW_READY', totalRows: 2, validRows: 2, invalidRows: 0, importedRows: 0 })
  await courseImportApi.uploadAttendance(scope, file, { id: 'template', version: 1 })
  const [path, options] = vi.mocked(apiRequest).mock.calls[0]
  expect(path).toBe('/imports/course-records')
  expect(options?.method).toBe('POST')
  const body = options?.body as FormData
  expect(body.get('inspectionTypeId')).toBe('unattended')
  expect(body.get('departmentId')).toBe('department')
  expect(body.get('templateVersion')).toBe('1')
  expect(body.get('file')).toBe(file)
})

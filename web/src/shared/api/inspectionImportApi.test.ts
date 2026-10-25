import { beforeEach, expect, it, vi } from 'vitest'
import { apiDownload, apiRequest } from './httpClient'
import { inspectionImportApi } from './inspectionImportApi'
vi.mock('./httpClient', async original => ({ ...await original<typeof import('./httpClient')>(), apiDownload: vi.fn(), apiRequest: vi.fn() }))
const scope = { semesterId:'semester', weekId:'week', inspectionTypeId:'morning', departmentId:'department' }
const batch = { ...scope, id:'batch', templateId:'template', templateVersion:1, version:3, status:'PREVIEW_READY', totalRows:2, validRows:2, invalidRows:0, importedRows:0 }
beforeEach(() => vi.clearAllMocks())
it('uses the generic entry and validates the returned destination', async () => {
 vi.mocked(apiRequest).mockResolvedValueOnce(batch)
 await expect(inspectionImportApi.upload(scope,new File(['fixture'],'morning.xlsx'),{id:'template',version:1})).resolves.toEqual(batch)
 const [path,options] = vi.mocked(apiRequest).mock.calls[0]
 expect(path).toBe('/imports/inspections')
 expect((options?.body as FormData).get('inspectionTypeId')).toBe('morning')
})
it.each([undefined,{}, {...batch,weekId:'other'},{...batch,inspectionTypeId:'course'},{...batch,version:0},{...batch,totalRows:undefined},{...batch,status:'UNKNOWN'}])('rejects an incomplete or mismatched upload response %#', async value => {
 vi.mocked(apiRequest).mockResolvedValueOnce(value)
 await expect(inspectionImportApi.upload(scope,new File(['fixture'],'morning.xlsx'),{id:'template',version:1})).rejects.toThrow('服务返回的数据不完整')
})
it('requires template identity and version from the generic download',async()=>{
 vi.mocked(apiDownload).mockResolvedValueOnce({blob:new Blob(),headers:new Headers()})
 await expect(inspectionImportApi.template(scope)).rejects.toMatchObject({code:'INVALID_TEMPLATE_RESPONSE'})
 vi.mocked(apiDownload).mockResolvedValueOnce({blob:new Blob(),headers:new Headers({'X-Import-Template-ID':'template','X-Import-Template-Version':'1'})})
 await expect(inspectionImportApi.template(scope)).resolves.toMatchObject({template:{id:'template',version:1}})
 expect(apiDownload).toHaveBeenLastCalledWith(expect.stringContaining('/imports/inspections/template?'))
})

it('does not submit legacy class or grade targets for new import batches', async () => {
 vi.mocked(apiRequest).mockResolvedValueOnce(batch)
 await inspectionImportApi.upload(scope, new File(['fixture'], 'scope.xlsx'), { id: 'template', version: 1 })
 const form = vi.mocked(apiRequest).mock.calls[0][1]?.body as FormData
 expect(form.has('classId')).toBe(false)
 expect(form.has('grade')).toBe(false)
})

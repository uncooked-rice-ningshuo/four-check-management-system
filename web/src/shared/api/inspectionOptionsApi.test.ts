import { expect, it, vi } from 'vitest'
import { apiRequest } from './httpClient'
import { allOptions } from './inspectionOptionsApi'
vi.mock('./httpClient', async original => ({ ...await original<typeof import('./httpClient')>(), apiRequest: vi.fn() }))
it('preserves optional class grade metadata and accepts legacy options without it', async () => {
 vi.mocked(apiRequest).mockResolvedValueOnce({items:[{id:'class',label:'Class',grade:'2026'},{id:'legacy',label:'Legacy'}],total:2,page:1,inspectionTypeId:'type'})
 const result=await allOptions('/inspections/context-options',{kind:'classes'},true)
 expect(result.items).toEqual([{id:'class',label:'Class',grade:'2026'},{id:'legacy',label:'Legacy'}])
})
it.each([null,2026,[]])('rejects malformed optional grade metadata %#',async grade=>{
 vi.mocked(apiRequest).mockResolvedValueOnce({items:[{id:'class',label:'Class',grade}],total:1,page:1,inspectionTypeId:'type'})
 await expect(allOptions('/inspections/context-options',{kind:'classes'},true)).rejects.toMatchObject({code:'INVALID_RESPONSE'})
})

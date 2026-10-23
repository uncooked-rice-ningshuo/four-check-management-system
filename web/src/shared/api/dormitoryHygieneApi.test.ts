import { beforeEach, expect, it, vi } from 'vitest'
import { dormitoryHygieneApi } from './dormitoryHygieneApi'
import { apiRequest } from './httpClient'
vi.mock('./httpClient',async original=>({...await original<typeof import('./httpClient')>(),apiRequest:vi.fn()}))
const request=vi.mocked(apiRequest)
beforeEach(()=>request.mockReset())

const record={
 id:'record',typeCode:'DORMITORY_HYGIENE',semesterId:'semester',weekId:'week',inspectionTypeId:'type',departmentId:'department',
 currentDormitoryName:'一区 一栋 101',currentReasonLabel:'地面',responsibleClasses:[{id:'class',label:'班级'}],
 responsibleStudents:[{id:'student',label:'学生'}],status:'DRAFT',source:'MANUAL',createdBy:'actor',rowVersion:1,canEdit:true,
 details:{dormitoryId:'room',date:'2026-09-03',round:'首次',result:'UNQUALIFIED',responsibleClassIds:['class'],responsibleStudentIds:['student'],reasonCode:'FLOOR'},
}
it('loads all object and reason pages in the explicit destination scope',async()=>{
 request.mockResolvedValueOnce({items:[{id:'room-1',label:'一区 一栋 101'}],total:2,page:1,pageSize:200})
 request.mockResolvedValueOnce({items:[{id:'room-2',label:'一区 一栋 102'}],total:2,page:2,pageSize:200})
 const items=await dormitoryHygieneApi.entryOptions({kind:'dormitory-objects',classId:'class',departmentId:'department'})
 expect(items.map(item=>item.id)).toEqual(['room-1','room-2'])
 expect(request.mock.calls[0][0]).toContain('classId=class')
 expect(request.mock.calls[1][0]).toContain('page=2')
 request.mockResolvedValueOnce({items:[],total:0,page:1,pageSize:200,inspectionTypeId:'type'})
 await dormitoryHygieneApi.contextOptions({purpose:'view',kind:'semesters'})
 expect(request.mock.calls[2][0]).toContain('typeCode=DORMITORY_HYGIENE')
})
it('requires responsibility labels and a complete object response',async()=>{
 request.mockResolvedValueOnce(record)
 expect((await dormitoryHygieneApi.detail('record')).responsibleStudents[0].label).toBe('学生')
 request.mockResolvedValueOnce({...record,responsibleClasses:undefined})
 await expect(dormitoryHygieneApi.detail('record')).rejects.toThrow('服务返回的数据不完整')
 request.mockResolvedValueOnce({...record,currentDormitoryName:undefined})
 await expect(dormitoryHygieneApi.detail('record')).rejects.toThrow('服务返回的数据不完整')
 request.mockResolvedValueOnce({...record,details:{...record.details,responsibleStudentIds:undefined},responsibleStudents:undefined})
 expect((await dormitoryHygieneApi.detail('record')).responsibleStudents).toEqual([])
})
it('rejects malformed pages instead of presenting an empty result',async()=>{
 request.mockResolvedValueOnce({items:[],total:1,page:1,pageSize:200,inspectionTypeId:'type'})
 await expect(dormitoryHygieneApi.contextOptions({purpose:'view',kind:'semesters'})).rejects.toThrow()
})

it('preserves optional import provenance and rejects malformed batch identifiers',async()=>{
 request.mockResolvedValueOnce({...record,source:'EXCEL',importBatchId:'batch'})
 expect((await dormitoryHygieneApi.detail('record')).importBatchId).toBe('batch')
 request.mockResolvedValueOnce(record)
 expect((await dormitoryHygieneApi.detail('record')).importBatchId).toBeUndefined()
 request.mockResolvedValueOnce({...record,importBatchId:123})
 await expect(dormitoryHygieneApi.detail('record')).rejects.toThrow('服务返回的数据不完整')
})

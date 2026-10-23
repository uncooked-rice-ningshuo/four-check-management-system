import { beforeEach, expect, it, vi } from 'vitest'
import { lateReturnApi } from './lateReturnApi'
import { apiRequest } from './httpClient'
vi.mock('./httpClient', async original => ({ ...await original<typeof import('./httpClient')>(), apiRequest: vi.fn() }))
const request = vi.mocked(apiRequest)
beforeEach(() => request.mockReset())
it('loads every reference page and preserves the explicit destination scope', async () => {
 request.mockResolvedValueOnce({ items:[{id:'one',label:'学生一'}],total:2,page:1,pageSize:200 }).mockResolvedValueOnce({items:[{id:'two',label:'学生二'}],total:2,page:2,pageSize:200})
 const items=await lateReturnApi.entryOptions({semesterId:'semester',weekId:'week',inspectionTypeId:'type',departmentId:'department',classId:'class',kind:'students'})
 expect(items.map(item=>item.id)).toEqual(['one','two'])
 expect(request.mock.calls[1][0]).toContain('page=2')
 expect(request.mock.calls[1][0]).toContain('departmentId=department')
})
it('rejects malformed or stalled pages instead of displaying a successful empty state', async () => {
 request.mockResolvedValueOnce({total:0,page:1,pageSize:200})
 await expect(lateReturnApi.contextOptions({purpose:'view',kind:'semesters'})).rejects.toThrow()
 request.mockResolvedValueOnce({items:[],total:2,page:1,pageSize:200,inspectionTypeId:'type'})
 await expect(lateReturnApi.contextOptions({purpose:'view',kind:'semesters'})).rejects.toThrow()
})
it('accepts an omitted optional remark but rejects missing required details', async () => {
 const value={id:'record',typeCode:'LATE_RETURN',semesterId:'semester',weekId:'week',inspectionTypeId:'type',departmentId:'department',studentNumber:'001',studentName:'学生',className:'班级',grade:'2026',currentDormitoryName:'宿舍',status:'DRAFT',source:'MANUAL',createdBy:'actor',rowVersion:1,canEdit:true,details:{classId:'class',studentId:'student',dormitoryId:'room',date:'2026-09-03',time:'08:00:00'}}
 request.mockResolvedValueOnce(value)
 expect((await lateReturnApi.detail('record')).details.remark).toBe('')
 request.mockResolvedValueOnce({...value,currentDormitoryName:undefined})
 await expect(lateReturnApi.detail('record')).rejects.toThrow('服务返回的数据不完整')
 request.mockResolvedValueOnce({...value,details:{...value.details,studentId:undefined}})
 await expect(lateReturnApi.detail('record')).rejects.toThrow('服务返回的数据不完整')
})

it('uses late-return context rather than course context', async () => {
 request.mockResolvedValueOnce({items:[],total:0,page:1,pageSize:200,inspectionTypeId:'late'})
 await lateReturnApi.contextOptions({purpose:'entry',kind:'semesters'})
 expect(request.mock.calls[0][0]).toContain('typeCode=LATE_RETURN')
})

it('rejects a response belonging to another inspection type',async()=>{
 request.mockResolvedValueOnce({id:'record',typeCode:'MORNING_ATTENDANCE',semesterId:'semester',weekId:'week',inspectionTypeId:'type',departmentId:'department',studentNumber:'001',studentName:'学生',className:'班级',grade:'2026',currentDormitoryName:'区域 楼栋 101',status:'DRAFT',source:'EXCEL',createdBy:'actor',rowVersion:1,canEdit:true,details:{classId:'class',studentId:'student',dormitoryId:'dorm',date:'2026-09-03',time:'23:05'}})
 await expect(lateReturnApi.detail('record')).rejects.toThrow('服务返回的数据不完整')
})

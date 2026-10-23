import { beforeEach, expect, it, vi } from 'vitest'
import { morningAttendanceApi } from './morningAttendanceApi'
import { apiRequest } from './httpClient'

vi.mock('./httpClient', async original => ({ ...await original<typeof import('./httpClient')>(), apiRequest: vi.fn() }))
const request=vi.mocked(apiRequest)
beforeEach(()=>request.mockReset())

it('requests the dedicated type and loads all authorized reference pages',async()=>{
 request.mockResolvedValueOnce({items:[],total:0,page:1,pageSize:200,inspectionTypeId:'morning'})
 expect((await morningAttendanceApi.contextOptions({purpose:'view',kind:'semesters'})).inspectionTypeId).toBe('morning')
 expect(request.mock.calls[0][0]).toContain('typeCode=MORNING_ATTENDANCE')
 request.mockResolvedValueOnce({items:[{id:'a',label:'甲'}],total:2,page:1,pageSize:200})
 request.mockResolvedValueOnce({items:[{id:'b',label:'乙'}],total:2,page:2,pageSize:200})
 expect((await morningAttendanceApi.entryOptions({kind:'students',departmentId:'department'})).map(item=>item.id)).toEqual(['a','b'])
 expect(request.mock.calls[2][0]).toContain('page=2')
})

it('requires the server-owned daily count and all required display fields',async()=>{
 const value={id:'record',typeCode:'MORNING_ATTENDANCE',semesterId:'semester',weekId:'week',inspectionTypeId:'type',departmentId:'department',studentNumber:'001',studentName:'学生',className:'班级',grade:'2026',status:'DRAFT',source:'MANUAL',createdBy:'actor',rowVersion:1,canEdit:true,details:{classId:'class',studentId:'student',date:'2026-09-03',absenceCount:1}}
 request.mockResolvedValueOnce(value)
 expect((await morningAttendanceApi.detail('record')).details.remark).toBe('')
 request.mockResolvedValueOnce({...value,details:{...value.details,absenceCount:2}})
 await expect(morningAttendanceApi.detail('record')).rejects.toThrow('服务返回的数据不完整')
 request.mockResolvedValueOnce({...value,studentName:undefined})
 await expect(morningAttendanceApi.detail('record')).rejects.toThrow('服务返回的数据不完整')
})

it('omits cumulative count and subtype when creating a daily event',async()=>{
 const input={semesterId:'semester',weekId:'week',inspectionTypeId:'type',departmentId:'department',details:{classId:'class',studentId:'student',date:'2026-09-03',remark:''}}
 request.mockResolvedValueOnce({id:'record',typeCode:'MORNING_ATTENDANCE',...input,studentNumber:'001',studentName:'学生',className:'班级',grade:'2026',status:'DRAFT',source:'MANUAL',createdBy:'actor',rowVersion:1,canEdit:true,details:{...input.details,absenceCount:1}})
 await morningAttendanceApi.create(input)
 expect(JSON.parse(String(request.mock.calls[0][1]?.body)).details).toEqual(input.details)
})

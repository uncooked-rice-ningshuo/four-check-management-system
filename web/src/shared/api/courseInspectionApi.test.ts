import { beforeEach, expect, it, vi } from 'vitest'
import { courseInspectionApi } from './courseInspectionApi'
import { apiRequest } from './httpClient'
vi.mock('./httpClient', async original => ({ ...await original<typeof import('./httpClient')>(), apiRequest: vi.fn() }))
const request = vi.mocked(apiRequest)
beforeEach(() => request.mockReset())
it('loads every reference page and preserves the explicit destination scope', async () => {
 request.mockResolvedValueOnce({ items:[{id:'one',label:'学生一'}],total:2,page:1,pageSize:200 }).mockResolvedValueOnce({items:[{id:'two',label:'学生二'}],total:2,page:2,pageSize:200})
 const items=await courseInspectionApi.entryOptions({semesterId:'semester',weekId:'week',inspectionTypeId:'type',departmentId:'department',classId:'class',kind:'students'})
 expect(items.map(item=>item.id)).toEqual(['one','two'])
 expect(request.mock.calls[1][0]).toContain('page=2')
 expect(request.mock.calls[1][0]).toContain('departmentId=department')
})
it('loads one fuzzy course search page with the bounded server page size', async () => {
 request.mockResolvedValueOnce({ items: [{ id: 'course-1', label: '软件工程' }], total: 21, page: 2, pageSize: 20 })
 const page = await courseInspectionApi.courseOptions({ semesterId: 'semester', weekId: 'week', inspectionTypeId: 'type', departmentId: 'department', keyword: '软件', page: 2, pageSize: 20 })
 expect(page.items).toEqual([{ id: 'course-1', label: '软件工程' }])
 expect(request.mock.calls[0][0]).toContain('kind=courses')
 expect(request.mock.calls[0][0]).toContain('keyword=%E8%BD%AF%E4%BB%B6')
 expect(request.mock.calls[0][0]).toContain('page=2')
 expect(request.mock.calls[0][0]).toContain('pageSize=20')
})
it('rejects malformed or stalled pages instead of displaying a successful empty state', async () => {
 request.mockResolvedValueOnce({total:0,page:1,pageSize:200})
 await expect(courseInspectionApi.contextOptions({purpose:'view',kind:'semesters'})).rejects.toThrow()
 request.mockResolvedValueOnce({items:[],total:2,page:1,pageSize:200,inspectionTypeId:'type'})
 await expect(courseInspectionApi.contextOptions({purpose:'view',kind:'semesters'})).rejects.toThrow()
})
it('accepts an omitted optional remark but rejects missing required details', async () => {
 const value={id:'record',typeCode:'UNATTENDED_CLASSROOM',semesterId:'semester',weekId:'week',inspectionTypeId:'type',departmentId:'department',studentNumber:'001',studentName:'学生',className:'班级',grade:'2026',currentCourseName:'课程',currentClassroomName:'教室',status:'DRAFT',source:'MANUAL',createdBy:'actor',rowVersion:1,canEdit:true,details:{classId:'class',studentId:'student',courseId:'course',classroomId:'room',date:'2026-09-03',time:'08:00:00',period:'1-2',absenceHours:2}}
 request.mockResolvedValueOnce(value)
 expect((await courseInspectionApi.detail('record')).details.remark).toBe('')
 request.mockResolvedValueOnce({...value,details:{...value.details,studentId:undefined}})
 await expect(courseInspectionApi.detail('record')).rejects.toThrow('服务返回的数据不完整')
})

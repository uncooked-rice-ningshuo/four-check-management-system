import { beforeEach, expect, it, vi } from 'vitest'
import { eveningSelfStudyApi } from './eveningSelfStudyApi'
import { apiRequest } from './httpClient'
vi.mock('./httpClient', async original => ({ ...await original<typeof import('./httpClient')>(), apiRequest: vi.fn() }))
const request = vi.mocked(apiRequest)
beforeEach(() => request.mockReset())
const value = { id:'record', typeCode:'EVENING_SELF_STUDY', semesterId:'semester', weekId:'week', inspectionTypeId:'type', departmentId:'department', studentNumber:'001', studentName:'学生', className:'班级', grade:'2026', currentClassroomName:'教室', status:'DRAFT', source:'MANUAL', createdBy:'actor', rowVersion:1, canEdit:true, details:{classId:'class', studentId:'student', classroomId:'room', date:'2026-09-03', time:'20:00:00', period:'9-10', penaltyMultiplier:2, penaltyCount:2} }
it('loads a counted evening violation and rejects a missing or malformed count', async () => {
 request.mockResolvedValueOnce(value)
 expect((await eveningSelfStudyApi.detail('record')).details.penaltyCount).toBe(2)
 request.mockResolvedValueOnce({...value, details:{...value.details, penaltyCount:undefined}})
 await expect(eveningSelfStudyApi.detail('record')).rejects.toThrow('服务返回的数据不完整')
 request.mockResolvedValueOnce({...value, details:{...value.details, penaltyCount:1}})
 await expect(eveningSelfStudyApi.detail('record')).rejects.toThrow('服务返回的数据不完整')
})

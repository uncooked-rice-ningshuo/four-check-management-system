import { beforeEach, expect, it, vi } from 'vitest'
import { classroomHygieneApi } from './classroomHygieneApi'
import { apiRequest } from './httpClient'
vi.mock('./httpClient', async original => ({ ...await original<typeof import('./httpClient')>(), apiRequest: vi.fn() }))
const request = vi.mocked(apiRequest)
beforeEach(() => request.mockReset())
const record = {
  id: 'record', typeCode: 'CLASSROOM_HYGIENE', semesterId: 'semester', weekId: 'week', inspectionTypeId: 'type', departmentId: 'department',
  currentClassroomName: '教学楼 101', currentAreaName: '教学楼 101 / 地面', currentReasonLabel: '地面', responsibleClasses: [{ id: 'class', label: '班级' }],
  responsibleStudents: [{ id: 'student', label: '学生' }],
  status: 'DRAFT', source: 'MANUAL', createdBy: 'actor', rowVersion: 1, canEdit: true,
  details: { classroomId: 'room', areaId: 'area', date: '2026-10-04', round: '首次', result: 'UNQUALIFIED', responsibleClassIds: ['class'], responsibleStudentIds: ['student'], reasonCode: 'FLOOR' },
}
it('uses the classroom hygiene context and loads every scoped classroom area option page', async () => {
  request.mockResolvedValueOnce({ items: [{ id: 'room-1', label: '一区 一栋 101' }], total: 2, page: 1, pageSize: 200 })
  request.mockResolvedValueOnce({ items: [{ id: 'room-2', label: '一区 一栋 102' }], total: 2, page: 2, pageSize: 200 })
  expect((await classroomHygieneApi.entryOptions({ kind: 'classroom-areas', classroomId: 'room', classId: 'class', departmentId: 'department' })).map(item => item.id)).toEqual(['room-1', 'room-2'])
  expect(request.mock.calls[1][0]).toContain('classroomId=room')
  expect(request.mock.calls[1][0]).toContain('page=2')
  request.mockResolvedValueOnce({ items: [], total: 0, page: 1, pageSize: 200, inspectionTypeId: 'type' })
  await classroomHygieneApi.contextOptions({ purpose: 'view', kind: 'semesters' })
  expect(request.mock.calls[2][0]).toContain('typeCode=CLASSROOM_HYGIENE')
})
it('requires area and class labels and preserves optional responsibility names', async () => {
  request.mockResolvedValueOnce(record)
  const loaded = await classroomHygieneApi.detail('record')
  expect(loaded.currentAreaName).toBe(record.currentAreaName)
  expect(loaded.currentClassroomName).toBe('教学楼 101')
  for (const field of ['responsibleClasses', 'currentAreaName', 'currentClassroomName']) {
    request.mockResolvedValueOnce({ ...record, [field]: undefined })
    await expect(classroomHygieneApi.detail('record')).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  }
  request.mockResolvedValueOnce({ ...record, responsibleStudents: undefined, details: { ...record.details, responsibleStudentIds: undefined } })
  expect((await classroomHygieneApi.detail('record')).responsibleStudents).toEqual([])
})
it('rejects cross-type and incomplete list responses instead of an empty result', async () => {
  request.mockResolvedValueOnce({ ...record, typeCode: 'DORMITORY_HYGIENE' })
  await expect(classroomHygieneApi.detail('record')).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  request.mockResolvedValueOnce({ total: 0, page: 1, pageSize: 20 })
  await expect(classroomHygieneApi.list({})).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
})
it('preserves optimistic version, reason and responsibility identifiers on update', async () => {
  request.mockResolvedValueOnce(record)
  const input = { semesterId: record.semesterId, weekId: record.weekId, departmentId: record.departmentId, inspectionTypeId: record.inspectionTypeId, details: { ...record.details, result: 'UNQUALIFIED' as const } }
  await classroomHygieneApi.update({ id: 'record', version: 4, reason: '核对责任', record: input })
  expect(request).toHaveBeenCalledWith('/inspections/record', { method: 'PATCH', body: JSON.stringify({ version: 4, reason: '核对责任', record: input }) })
})

it('preserves optional import provenance and rejects malformed batch identifiers',async()=>{
 request.mockResolvedValueOnce({...record,source:'EXCEL',importBatchId:'batch'})
 expect((await classroomHygieneApi.detail('record')).importBatchId).toBe('batch')
 request.mockResolvedValueOnce(record)
 expect((await classroomHygieneApi.detail('record')).importBatchId).toBeUndefined()
 request.mockResolvedValueOnce({...record,importBatchId:123})
 await expect(classroomHygieneApi.detail('record')).rejects.toThrow('服务返回的数据不完整')
})

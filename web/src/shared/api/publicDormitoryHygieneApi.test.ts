import { beforeEach, expect, it, vi } from 'vitest'
import { publicDormitoryHygieneApi } from './publicDormitoryHygieneApi'
import { apiRequest } from './httpClient'
vi.mock('./httpClient', async original => ({ ...await original<typeof import('./httpClient')>(), apiRequest: vi.fn() }))
const request = vi.mocked(apiRequest)
beforeEach(() => request.mockReset())
const record = {
  id: 'record', typeCode: 'PUBLIC_DORMITORY_HYGIENE', semesterId: 'semester', weekId: 'week', inspectionTypeId: 'type', departmentId: 'department',
  currentAreaName: '一区 一栋 / 一层 / 走廊', currentReasonLabel: '地面', responsibleClasses: [{ id: 'class', label: '班级' }],
  responsibleStudents: [{ id: 'student', label: '学生' }], responsibleDormitories: [{ id: 'room', label: '一区 一栋 101' }],
  status: 'DRAFT', source: 'MANUAL', createdBy: 'actor', rowVersion: 1, canEdit: true,
  details: { areaId: 'area', date: '2026-10-04', round: '首次', result: 'UNQUALIFIED', responsibleClassIds: ['class'], responsibleStudentIds: ['student'], responsibleDormitoryIds: ['room'], reasonCode: 'FLOOR' },
}
it('uses the public hygiene context and loads every scoped area/room option page', async () => {
  request.mockResolvedValueOnce({ items: [{ id: 'room-1', label: '一区 一栋 101' }], total: 2, page: 1, pageSize: 200 })
  request.mockResolvedValueOnce({ items: [{ id: 'room-2', label: '一区 一栋 102' }], total: 2, page: 2, pageSize: 200 })
  expect((await publicDormitoryHygieneApi.entryOptions({ kind: 'responsible-dormitories', areaId: 'area', classId: 'class', departmentId: 'department' })).map(item => item.id)).toEqual(['room-1', 'room-2'])
  expect(request.mock.calls[1][0]).toContain('areaId=area')
  expect(request.mock.calls[1][0]).toContain('page=2')
  request.mockResolvedValueOnce({ items: [], total: 0, page: 1, pageSize: 200, inspectionTypeId: 'type' })
  await publicDormitoryHygieneApi.contextOptions({ purpose: 'view', kind: 'semesters' })
  expect(request.mock.calls[2][0]).toContain('typeCode=PUBLIC_DORMITORY_HYGIENE')
})
it('requires area and class labels and preserves optional responsibility names', async () => {
  request.mockResolvedValueOnce(record)
  const loaded = await publicDormitoryHygieneApi.detail('record')
  expect(loaded.currentAreaName).toBe(record.currentAreaName)
  expect(loaded.responsibleDormitories[0].label).toBe('一区 一栋 101')
  for (const field of ['responsibleClasses', 'currentAreaName']) {
    request.mockResolvedValueOnce({ ...record, [field]: undefined })
    await expect(publicDormitoryHygieneApi.detail('record')).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  }
  request.mockResolvedValueOnce({ ...record, responsibleDormitories: undefined, responsibleStudents: undefined, details: { ...record.details, responsibleDormitoryIds: undefined, responsibleStudentIds: undefined } })
  expect((await publicDormitoryHygieneApi.detail('record')).responsibleDormitories).toEqual([])
})
it('rejects cross-type and incomplete list responses instead of an empty result', async () => {
  request.mockResolvedValueOnce({ ...record, typeCode: 'DORMITORY_HYGIENE' })
  await expect(publicDormitoryHygieneApi.detail('record')).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  request.mockResolvedValueOnce({ total: 0, page: 1, pageSize: 20 })
  await expect(publicDormitoryHygieneApi.list({})).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
})
it('preserves optimistic version, reason and responsibility identifiers on update', async () => {
  request.mockResolvedValueOnce(record)
  const input = { semesterId: record.semesterId, weekId: record.weekId, departmentId: record.departmentId, inspectionTypeId: record.inspectionTypeId, details: { ...record.details, result: 'UNQUALIFIED' as const } }
  await publicDormitoryHygieneApi.update({ id: 'record', version: 4, reason: '核对责任', record: input })
  expect(request).toHaveBeenCalledWith('/inspections/record', { method: 'PATCH', body: JSON.stringify({ version: 4, reason: '核对责任', record: input }) })
})

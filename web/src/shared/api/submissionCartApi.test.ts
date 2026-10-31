import { beforeEach, expect, it, vi } from 'vitest'
import { submissionCartApi } from './submissionCartApi'
import { apiRequest } from './httpClient'

vi.mock('./httpClient', async original => ({ ...await original<typeof import('./httpClient')>(), apiRequest: vi.fn() }))
const request = vi.mocked(apiRequest)
const record = { id: 'r', semesterId: 's', weekId: 'w', inspectionTypeId: 't', departmentId: 'd', typeCode: 'ATTENDANCE_RATE', studentNumber: '1', studentName: '学生', className: '班级', grade: '2026', status: 'DRAFT', source: 'MANUAL', rowVersion: 1, canEdit: true, details: { date: '2026-09-10', time: '08:30:00', period: '1-2' } }
beforeEach(() => request.mockReset())

it('preserves optional display labels and remains compatible with older responses', async () => {
  request.mockResolvedValueOnce({ items: [{ ...record, currentCourseName: '离散数学', currentClassroomName: 'A101' }, record], total: 2, page: 1, pageSize: 10 })
  const result = await submissionCartApi.candidates({})
  expect(result.items[0]).toMatchObject({ currentCourseName: '离散数学', currentClassroomName: 'A101', details: { period: '1-2' } })
  expect(result.items[1].currentCourseName).toBeUndefined()
})

it('rejects malformed optional display fields instead of treating them as empty', async () => {
  request.mockResolvedValueOnce({ items: [{ ...record, currentCourseName: 12 }], total: 1, page: 1, pageSize: 10 })
  await expect(submissionCartApi.candidates({})).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
})

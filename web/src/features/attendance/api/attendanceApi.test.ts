import { afterEach, describe, expect, it, vi } from 'vitest'
import { attendanceApi } from './attendanceApi'

describe('attendanceApi', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('serializes context, filters and pagination through the shared transport', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ items: [], total: 0, page: 2, pageSize: 25 }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    vi.stubGlobal('fetch', fetchMock)

    await attendanceApi.list({ semesterId: 'semester-1', weekId: 'week-1', inspectionTypeId: 'type-attendance', departmentId: 'department-study', keyword: 'S01', page: 2, pageSize: 25 })

    expect(fetchMock).toHaveBeenCalledWith(expect.stringContaining('/api/v1/inspections/attendance?'), expect.any(Object))
    const url = new URL(fetchMock.mock.calls[0][0], 'http://localhost')
    expect(Object.fromEntries(url.searchParams)).toMatchObject({ semesterId: 'semester-1', weekId: 'week-1', departmentId: 'department-study', keyword: 'S01', page: '2', pageSize: '25' })
  })

  it('posts manual business fields using the shared transport', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ id: 'record', source: 'MANUAL' }), { status: 201 }))
    vi.stubGlobal('fetch', fetchMock)
    const input = { semesterId: 'semester', weekId: 'week', inspectionTypeId: 'type', departmentId: 'department', classId: 'class', className: '测试班', grade: '2026级', studentNo: 'S001', studentName: '虚构同学', courseName: '数据结构', classroom: '博学楼 101', date: '2026-09-06', time: '08:00', period: '1-2', absenceHours: 2 }
    await attendanceApi.create(input)
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/inspections/attendance', expect.objectContaining({ method: 'POST', body: JSON.stringify(input) }))
  })

	it('loads business-scoped filter options from the attendance endpoint', async () => {
		const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ inspectionType: { id: 'type-attendance', code: 'ATTENDANCE_RATE', name: '到课率' }, departments: [], semesters: [], weeks: [], classes: [], grades: [] }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
		vi.stubGlobal('fetch', fetchMock)

		await attendanceApi.filterOptions()

		expect(fetchMock).toHaveBeenCalledWith(expect.stringContaining('/api/v1/inspections/attendance/filter-options'), expect.any(Object))
	})
})

describe('attendance mutation routes', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('keeps flat attendance details and mutations on dedicated endpoints', async () => {
    const fetchMock = vi.fn().mockImplementation(() => Promise.resolve(new Response('{}', { status: 200 })))
    vi.stubGlobal('fetch', fetchMock)
    await attendanceApi.detail('record-1')
    expect(fetchMock).toHaveBeenLastCalledWith('/api/v1/inspections/attendance/record-1', expect.any(Object))
    await attendanceApi.remove({ items: [{ id: 'record-1', version: 1 }], reason: '核对' })
    expect(fetchMock).toHaveBeenLastCalledWith('/api/v1/inspections/attendance/record-1', expect.objectContaining({ method: 'DELETE' }))
    await attendanceApi.remove({ items: [{ id: 'record-1', version: 1 }, { id: 'record-2', version: 2 }], reason: '核对' })
    expect(fetchMock).toHaveBeenLastCalledWith('/api/v1/inspections/attendance/batch', expect.objectContaining({ method: 'POST' }))
  })
})

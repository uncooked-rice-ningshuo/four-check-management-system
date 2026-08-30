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
})

import { beforeEach, expect, it, vi } from 'vitest'
import { reportingApi } from './reportingApi'
import { apiRequest } from './httpClient'

vi.mock('./httpClient', async original => ({ ...await original<typeof import('./httpClient')>(), apiRequest: vi.fn() }))
const request = vi.mocked(apiRequest)

beforeEach(() => request.mockReset())

const period = {
  academicYearId: 'year-1',
  academicYearName: '2025-2026学年',
  semesterId: 'semester-1',
  semesterName: '2025-2026下学期',
  weekId: 'week-2',
  weekNo: 2,
  startDate: '2026-03-09T00:00:00Z',
  endDate: '2026-03-15T00:00:00Z',
}

it('rejects unparseable reporting period dates before the page formats them', async () => {
  request.mockResolvedValueOnce({ items: [{ ...period, startDate: 'not-a-date' }] })

  await expect(reportingApi.periods()).rejects.toMatchObject({ status: 502, code: 'INVALID_RESPONSE' })
})

it('rejects reporting periods outside the academic calendar constraints', async () => {
  request.mockResolvedValueOnce({ items: [{ ...period, weekNo: 0 }] })
  await expect(reportingApi.periods()).rejects.toMatchObject({ status: 502, code: 'INVALID_RESPONSE' })

  request.mockResolvedValueOnce({ items: [{ ...period, startDate: '2026-03-16T00:00:00Z' }] })
  await expect(reportingApi.periods()).rejects.toMatchObject({ status: 502, code: 'INVALID_RESPONSE' })
})

it('accepts valid reporting periods and sends selected IDs as query parameters', async () => {
  request.mockResolvedValueOnce({ items: [period] })
  await expect(reportingApi.periods()).resolves.toEqual({ items: [period] })

  request.mockResolvedValueOnce({ period, rows: [], totals: {
    submissionCount: 0,
    draftBatchCount: 0,
    pendingReviewBatchCount: 0,
    returnedBatchCount: 0,
    confirmedBatchCount: 0,
    confirmedRecordCount: 0,
    pendingReviewOrReturnedRecordCount: 0,
    zeroRecordPendingReviewBatchCount: 0,
    zeroRecordReturnedBatchCount: 0,
    zeroRecordConfirmedBatchCount: 0,
  } })
  await reportingApi.summary('semester-1', 'week-2')
  expect(request.mock.calls[1][0]).toBe('/reports/summary?semesterId=semester-1&weekId=week-2')
})

it('rejects a summary whose totals disagree with its visible rows', async () => {
  request.mockResolvedValueOnce({
    period,
    rows: [{
      departmentId: 'department-1',
      departmentName: '学习部',
      collegeLevel: false,
      inspectionTypeId: 'type-1',
      inspectionTypeName: '到课率',
      isRequired: false,
      submissionCount: 1,
      draftBatchCount: 0,
      pendingReviewBatchCount: 0,
      returnedBatchCount: 0,
      confirmedBatchCount: 1,
      confirmedRecordCount: 2,
      pendingReviewOrReturnedRecordCount: 0,
      zeroRecordPendingReviewBatchCount: 0,
      zeroRecordReturnedBatchCount: 0,
      zeroRecordConfirmedBatchCount: 0,
    }],
    totals: {
      submissionCount: 0,
      draftBatchCount: 0,
      pendingReviewBatchCount: 0,
      returnedBatchCount: 0,
      confirmedBatchCount: 0,
      confirmedRecordCount: 0,
      pendingReviewOrReturnedRecordCount: 0,
      zeroRecordPendingReviewBatchCount: 0,
      zeroRecordReturnedBatchCount: 0,
      zeroRecordConfirmedBatchCount: 0,
    },
  })

  await expect(reportingApi.summary('semester-1', 'week-2')).rejects.toMatchObject({ status: 502, code: 'INVALID_RESPONSE' })
})

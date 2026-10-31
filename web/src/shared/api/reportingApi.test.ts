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

it('parses archive pages and reads a selected finalized workflow through the archive route', async () => {
  const archiveEntry = {
    id: 'report-1', period, status: 'FINALIZED', versionNo: 2, versionCount: 2,
    confirmedRecordCount: 7, confirmedZeroReceiptCount: 1, cellCount: 3, scopeLimited: false,
    finalizedAt: '2026-03-16T08:00:00Z', updatedAt: '2026-03-16T08:00:00Z',
  }
  request.mockResolvedValueOnce({ items: [archiveEntry], total: 1, page: 2, pageSize: 5 })
  await expect(reportingApi.archive({ page: 2, pageSize: 5, status: 'FINALIZED' })).resolves.toEqual({
    items: [archiveEntry], total: 1, page: 2, pageSize: 5,
  })
  expect(request.mock.calls[0][0]).toBe('/reports/archive?page=2&pageSize=5&status=FINALIZED')

  request.mockResolvedValueOnce({ report: null, versions: [], reviews: [], corrections: [] })
  await reportingApi.archiveWorkflow('semester-1', 'week-2')
  expect(request.mock.calls[1][0]).toBe('/reports/archive/semester-1/week-2')
})

it('accepts a generated draft archive entry even when it has no record cells', async () => {
  const draftEntry = {
    id: 'report-draft', period, status: 'DRAFT', versionNo: 1, versionCount: 1,
    confirmedRecordCount: 0, confirmedZeroReceiptCount: 1, cellCount: 0, scopeLimited: false,
    updatedAt: '2026-03-16T08:00:00Z',
  }
  request.mockResolvedValueOnce({ items: [draftEntry], total: 1, page: 1, pageSize: 20 })
  await expect(reportingApi.archive({ page: 1, pageSize: 20 })).resolves.toMatchObject({ items: [draftEntry], total: 1 })
})

it('parses the report workflow and draft response contracts independently', async () => {
  const content = {
    period,
    scopeLimited: false,
    statisticsRuleVersion: 'COUNT_DETAILS_V1',
    confirmedRecordCount: 0,
    rows: [{
      departmentId: '',
      departmentName: '校级',
      collegeLevel: true,
      inspectionTypeId: 'type-1',
      inspectionType: '到课率',
      recordCount: 0,
      confirmedZeroReceiptCount: 0,
      dataStatus: 'NO_CONFIRMED_DATA',
    }],
    incompleteCells: [],
    pendingDepartmentReconfirmationCount: 0,
    corrections: [],
    generatedAt: '2026-03-10T00:00:00Z',
  }
  request.mockResolvedValueOnce({ report: null, versions: [], reviews: [], corrections: [] })
  await expect(reportingApi.workflow('semester-1', 'week-2')).resolves.toEqual({ report: null, versions: [], reviews: [], corrections: [] })
  expect(request.mock.calls[0][0]).toBe('/reports/workflow?semesterId=semester-1&weekId=week-2')

  const legacyWorkflow = {
    report: {
      id: 'report-1', semesterId: 'semester-1', weekId: 'week-2', status: 'DRAFT',
      versionNo: 1, rowVersion: 1, updatedAt: '2026-03-10T00:00:00Z',
    },
    versions: [{ id: 'version-1', versionNo: 1, content: { ...content, adjustments: null }, createdBy: 'admin-1', createdAt: content.generatedAt }],
    reviews: [],
    corrections: [],
  }
  request.mockResolvedValueOnce(legacyWorkflow)
  await expect(reportingApi.workflow('semester-1', 'week-2')).resolves.toEqual(legacyWorkflow)

  request.mockResolvedValueOnce({ id: 'report-id', status: 'DRAFT', rowVersion: 1, versionNo: 1, content, createdAt: content.generatedAt })
  await expect(reportingApi.generateDraft({ semesterId: 'semester-1', weekId: 'week-2', expectedRowVersion: 0 }))
    .resolves.toMatchObject({ id: 'report-id', status: 'DRAFT', versionNo: 1 })
  expect(request.mock.calls[2][0]).toBe('/reports/drafts')
})

it('reads paginated record values from the selected immutable department/type report version', async () => {
  const record = {
    id: 'record-1', studentNumber: '20260001', studentName: '测试学生', className: '计科一班', grade: '2026级',
    typeCode: 'ATTENDANCE_RATE', details: { remark: '已冻结的备注' }, currentDormitoryName: '',
    currentCourseName: '离散数学', currentClassroomName: 'A101',
  }
  request.mockResolvedValueOnce({ items: [record], total: 1, page: 2, pageSize: 10 })
  await expect(reportingApi.versionRecords({
    semesterId: 'semester-1', weekId: 'week-2', versionNo: 3,
    departmentId: 'department-1', collegeLevel: false, inspectionTypeId: 'type-1', page: 2, pageSize: 10,
  }))
    .resolves.toEqual({ items: [record], total: 1, page: 2, pageSize: 10 })
  expect(request.mock.calls[0][0]).toBe('/reports/workflow/semester-1/week-2/versions/3/records?page=2&pageSize=10&inspectionTypeId=type-1&departmentId=department-1&collegeLevel=false')
})

it('appends a version adjustment without overwriting the original report snapshot', async () => {
  request.mockResolvedValueOnce({
    adjustmentId: 'adjustment-1',
    report: {
      id: 'report-1', semesterId: 'semester-1', weekId: 'week-2', status: 'DRAFT',
      versionNo: 2, rowVersion: 3, updatedAt: '2026-03-10T00:00:00Z',
    },
  })

  await expect(reportingApi.addAdjustment('report-1', {
    rowVersion: 2, recordId: 'record-1', operation: 'EXCLUDE', reason: '重复记录，经核实不纳入',
  })).resolves.toMatchObject({ adjustmentId: 'adjustment-1', report: { versionNo: 2, rowVersion: 3 } })
  expect(request.mock.calls[0][0]).toBe('/reports/report-1/adjustments')
  expect(request.mock.calls[0][1]).toMatchObject({
    method: 'POST',
    body: JSON.stringify({ rowVersion: 2, recordId: 'record-1', operation: 'EXCLUDE', reason: '重复记录，经核实不纳入' }),
  })
})

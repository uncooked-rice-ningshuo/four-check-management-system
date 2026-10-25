import { beforeEach, expect, it, vi } from 'vitest'
import { departmentSubmissionApi } from './departmentSubmissionApi'
import { apiRequest } from './httpClient'

vi.mock('./httpClient', async original => ({ ...await original<typeof import('./httpClient')>(), apiRequest: vi.fn() }))
const request = vi.mocked(apiRequest)
beforeEach(() => request.mockReset())

const scope = { semesterId: 's', weekId: 'w', departmentId: 'd', inspectionTypeId: 't' }
const batch = { id: 'batch', scope, status: 'PENDING_REVIEW', revisionNo: 1, rowVersion: 1,
  createdBy: 'actor', submittedBy: 'actor', createdAt: '2026-09-01T00:00:00Z', submittedAt: '2026-09-01T00:00:00Z',
  recordIds: ['r'], records: [{ id: 'r', rowVersion: 2, status: 'PENDING_REVIEW', createdBy: 'actor' }] }

it('sends a versioned batch and parses its paginated read response', async () => {
  request.mockResolvedValueOnce(batch)
  expect((await departmentSubmissionApi.submit(scope, [{ id: 'r', version: 1 }], 'retry-key')).id).toBe('batch')
  expect(request.mock.calls[0][0]).toBe('/department-submissions')
  expect(JSON.parse(String(request.mock.calls[0][1]?.body))).toEqual({ scope, kind: 'RECORDS', records: [{ id: 'r', version: 1 }], idempotencyKey: 'retry-key', explanation: '' })
  request.mockResolvedValueOnce({ items: [batch], total: 1, page: 1, pageSize: 10 })
  expect((await departmentSubmissionApi.list({ inspectionTypeId: 't', page: 1, pageSize: 10 })).items[0].records[0].rowVersion).toBe(2)
  expect(request.mock.calls[1][0]).toContain('inspectionTypeId=t')
})

it('rejects missing or malformed server-owned batch fields', async () => {
  request.mockResolvedValueOnce({ ...batch, records: undefined })
  await expect(departmentSubmissionApi.detail('batch')).rejects.toThrow('服务返回的数据不完整')
  request.mockResolvedValueOnce({ items: [{ ...batch, rowVersion: '1' }], total: 1, page: 1, pageSize: 10 })
  await expect(departmentSubmissionApi.list({})).rejects.toThrow('服务返回的数据不完整')
})

it('uses the current aggregate version for decisions and complete record versions for resubmission', async () => {
  request.mockResolvedValueOnce(batch)
  await departmentSubmissionApi.decide('batch', 'return', 1, '请核对')
  expect(JSON.parse(String(request.mock.calls[0][1]?.body))).toEqual({ version: 1, reason: '请核对' })
  request.mockResolvedValueOnce(batch)
  await departmentSubmissionApi.resubmit('batch', 2, [{ id: 'r', version: 3 }])
  expect(JSON.parse(String(request.mock.calls[1][1]?.body))).toEqual({ version: 2, kind: 'RECORDS', records: [{ id: 'r', version: 3 }], explanation: '' })
})

it('sends an explicit zero-record receipt with its optional explanation', async () => {
  request.mockResolvedValueOnce({ ...batch, kind: 'ZERO_RECORD', recordIds: [], records: [] })
  await departmentSubmissionApi.submit(scope, [], 'zero-key', 'ZERO_RECORD', '本周已完成检查，没有异常')
  expect(JSON.parse(String(request.mock.calls[0][1]?.body))).toEqual({
    scope, kind: 'ZERO_RECORD', records: [], idempotencyKey: 'zero-key', explanation: '本周已完成检查，没有异常',
  })
})

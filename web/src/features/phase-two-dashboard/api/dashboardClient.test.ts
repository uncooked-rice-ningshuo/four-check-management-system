import { beforeEach, expect, it, vi } from 'vitest'
import { getPhaseTwoDashboard } from './dashboardClient'
import { apiRequest } from '../../../shared/api/httpClient'

vi.mock('../../../shared/api/httpClient', async original => ({ ...await original<typeof import('../../../shared/api/httpClient')>(), apiRequest: vi.fn() }))
const request = vi.mocked(apiRequest)
beforeEach(() => request.mockReset())

const status = { departmentId: 'd', departmentName: '学习部', inspectionTypeId: 't', inspectionTypeCode: 'ATTENDANCE_RATE',
  inspectionTypeName: '到课率', status: 'PENDING_REVIEW', total: 2, draft: 0, pendingReview: 2, returned: 0,
  confirmed: 0, locked: 0, batchTotal: 2, batchPending: 2, batchReturned: 0, batchConfirmed: 0,
  canEnter: true, canReview: true }

it('uses the server batch counts for the current weekly department status', async () => {
  request.mockResolvedValueOnce({ context: null, myTodos: { total: 0, draft: 0, returned: 0 }, departmentStatuses: [status] })
  expect((await getPhaseTwoDashboard()).departmentStatuses[0].batchTotal).toBe(2)
})

it('fails explicitly when an older response omits required batch fields', async () => {
	const oldStatus: Record<string, unknown> = { ...status }
	delete oldStatus.batchTotal
  request.mockResolvedValueOnce({ context: null, myTodos: { total: 0, draft: 0, returned: 0 }, departmentStatuses: [oldStatus] })
  await expect(getPhaseTwoDashboard()).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
})

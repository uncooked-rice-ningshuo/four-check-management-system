import { afterEach, describe, expect, it, vi } from 'vitest'
import { importJobApi } from './importJobApi'

const context = { semesterId: 'semester-1', weekId: 'week-1', inspectionTypeId: 'type-attendance', departmentId: 'department-study' }

describe('importJobApi', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('lets the browser set the multipart boundary', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ id: 'batch-1', version: 1 }), { status: 201, headers: { 'Content-Type': 'application/json' } }))
    vi.stubGlobal('fetch', fetchMock)
    await importJobApi.uploadAttendance(context, new File(['xlsx'], 'attendance.xlsx'), { id: 'template-1', version: 1 })
    const init = fetchMock.mock.calls[0][1] as RequestInit
    expect(init.body).toBeInstanceOf(FormData)
    expect(new Headers(init.headers).has('Content-Type')).toBe(false)
  })

  it('sends pagination, expected version and idempotency key', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ items: [], total: 0, page: 3, pageSize: 10 }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ batchId: 'batch-1', status: 'CONFIRMED', importedRows: 2, version: 4 }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    vi.stubGlobal('fetch', fetchMock)
    await importJobApi.rows('batch-1', 3, 10)
    await importJobApi.confirm('batch-1', 3, 'confirm-key')
    expect(fetchMock.mock.calls[0][0]).toContain('/imports/batch-1/rows?page=3&pageSize=10')
    const confirmInit = fetchMock.mock.calls[1][1] as RequestInit
    expect(new Headers(confirmInit.headers).get('Idempotency-Key')).toBe('confirm-key')
    expect(JSON.parse(confirmInit.body as string)).toEqual({ version: 3 })
  })

	it('returns the downloaded file together with authoritative template identity', async () => {
		const fetchMock = vi.fn().mockResolvedValue(new Response('xlsx', { status: 200, headers: { 'X-Import-Template-ID': 'template-1', 'X-Import-Template-Version': '3' } }))
		vi.stubGlobal('fetch', fetchMock)
		const result = await importJobApi.template(context)
		expect(result.template).toEqual({ id: 'template-1', version: 3 })
		expect(result.file.size).toBe(4)
	})
})

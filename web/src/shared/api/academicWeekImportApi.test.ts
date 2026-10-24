import { beforeEach, expect, it, vi } from 'vitest'
import { apiDownload, apiRequest } from './httpClient'
import { academicWeekImportApi } from './academicWeekImportApi'

vi.mock('./httpClient', async original => ({ ...await original<typeof import('./httpClient')>(), apiDownload: vi.fn(), apiRequest: vi.fn() }))

beforeEach(() => vi.clearAllMocks())

it('downloads the dedicated academic week template through the authenticated client', async () => {
  const result = { blob: new Blob(['xlsx']), headers: new Headers() }
  vi.mocked(apiDownload).mockResolvedValueOnce(result)
  await expect(academicWeekImportApi.template()).resolves.toEqual(result)
  expect(apiDownload).toHaveBeenCalledWith('/admin/academic/weeks/import-template')
})

it('sends the explicit semester and selected workbook in multipart preview', async () => {
  const file = new File(['fixture'], 'weeks.xlsx')
  const batch = { batchId: 'batch-1', semesterId: 'semester-1', status: 'PREVIEW_READY', version: 1 }
  vi.mocked(apiRequest).mockResolvedValueOnce(batch)
  await expect(academicWeekImportApi.preview('semester-1', file)).resolves.toEqual(batch)
  const [path, options] = vi.mocked(apiRequest).mock.calls[0]
  expect(path).toBe('/admin/academic/weeks/imports')
  expect(options?.method).toBe('POST')
  expect(options?.body).toBeInstanceOf(FormData)
  expect((options?.body as FormData).get('semesterId')).toBe('semester-1')
  expect((options?.body as FormData).get('file')).toBe(file)
})

it('sends version and a stable idempotency key for confirmation', async () => {
  vi.mocked(apiRequest).mockResolvedValueOnce({ batchId: 'batch-1', version: 2 })
  await academicWeekImportApi.confirm('batch-1', 1, 'retry-key-0123456789')
  const [path, options] = vi.mocked(apiRequest).mock.calls[0]
  expect(path).toBe('/admin/academic/weeks/imports/batch-1/confirm')
  expect(options?.headers).toEqual({ 'Idempotency-Key': 'retry-key-0123456789' })
  expect(options?.body).toBe(JSON.stringify({ version: 1 }))
})

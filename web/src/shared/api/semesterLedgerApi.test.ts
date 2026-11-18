import { expect, it, vi } from 'vitest'
import { apiRequest } from './httpClient'
import { emptyLedgerFilter, readSemesterLedger } from './semesterLedgerApi'

vi.mock('./httpClient', async importOriginal => ({
  ...await importOriginal<typeof import('./httpClient')>(), apiRequest: vi.fn(),
}))

const response = { semesters: [], organizations: [], columns: [], total: 1, unassigned: 0, ruleVersion: null, weeks: [1], page: 1, pageSize: 50 }
const student = { number: '001', name: '示例', grade: '26级', className: '一班', sources: [], values: { attendance: null } }

it('保留未能计算的原始量与前导零学号', async () => {
  vi.mocked(apiRequest).mockResolvedValue({ ...response, items: [student] })
  const result = await readSemesterLedger('semester', emptyLedgerFilter, 1, 50)
  expect(result.items[0]).toEqual(student)
})

it('缺少来源或完整定稿周次时拒绝显示正常空态', async () => {
  for (const invalid of [{ ...response, items: [{ ...student, sources: undefined }] }, { ...response, items: [], weeks: undefined }]) {
    vi.mocked(apiRequest).mockResolvedValue(invalid)
    await expect(readSemesterLedger('semester', emptyLedgerFilter, 1, 50)).rejects.toMatchObject({ code: 'INVALID_RESPONSE' })
  }
})

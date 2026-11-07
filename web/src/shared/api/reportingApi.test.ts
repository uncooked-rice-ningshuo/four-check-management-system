import { beforeEach, describe, expect, it, vi } from 'vitest'
import { reportingApi } from './reportingApi'

const mocks = vi.hoisted(() => ({ apiRequest: vi.fn() }))

vi.mock('./httpClient', () => ({ apiRequest: mocks.apiRequest }))

describe('通报记录筛选请求', () => {
  beforeEach(() => {
    mocks.apiRequest.mockReset().mockResolvedValue({ items: [], total: 0, page: 2, pageSize: 50,
      filterOptions: { studentNumbers: [], studentNames: [], classNames: [], grades: [] } })
  })

  it('将学号、姓名、班级、年级和分页参数分别发送给服务端', async () => {
    await reportingApi.records({
      semesterId: 'semester-1', weekId: 'week-2', departmentId: 'department-1',
      collegeLevel: false, inspectionTypeId: 'type-1', studentNumber: '20260001', studentName: '李同学',
      className: '计算机一班', grade: '2026级', page: 2, pageSize: 50,
    })

    const requestUrl = mocks.apiRequest.mock.calls[0]?.[0]
    expect(typeof requestUrl).toBe('string')
    const params = new URL(String(requestUrl), 'http://localhost').searchParams
    expect(params.get('studentNumber')).toBe('20260001')
    expect(params.get('studentName')).toBe('李同学')
    expect(params.get('className')).toBe('计算机一班')
    expect(params.get('grade')).toBe('2026级')
    expect(params.get('page')).toBe('2')
    expect(params.get('pageSize')).toBe('50')
  })
})

import { describe, expect, it } from 'vitest'
import { academicYearOption, buildAcademicYearDefaults, buildSemesterDefaults } from './academicCalendarRules'

describe('校历默认规则', () => {
  it('学年结束年份固定为开始年份加一', () => {
    expect(academicYearOption(2025)).toEqual({ label: '2025-2026 学年', value: 2025 })
    expect(buildAcademicYearDefaults(2025)).toEqual({
      name: '2025-2026 学年',
      startDate: '2025-09-01',
      endDate: '2026-08-31',
    })
  })

  it('上学期使用九月至次年一月的默认区间', () => {
    expect(buildSemesterDefaults(2025, 'FIRST')).toEqual({
      code: 'FIRST',
      name: '2025-2026 学年上学期',
      startDate: '2025-09-01',
      endDate: '2026-01-31',
    })
  })

  it('下学期使用次年二月至八月的默认区间', () => {
    expect(buildSemesterDefaults(2025, 'SECOND')).toEqual({
      code: 'SECOND',
      name: '2025-2026 学年下学期',
      startDate: '2026-02-01',
      endDate: '2026-08-31',
    })
  })
})

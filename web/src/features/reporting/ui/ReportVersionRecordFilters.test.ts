import { describe, expect, it } from 'vitest'
import { nextReportFilterOptionCount, reportFilterOptionCount, reportFilterOptionPage } from '../model/reportRecordFilters'

describe('通报记录筛选下拉加载', () => {
  it('每次滚动加载一批选项并停在总数', () => {
    expect(nextReportFilterOptionCount(50, 180)).toBe(100)
    expect(nextReportFilterOptionCount(150, 180)).toBe(180)
    expect(nextReportFilterOptionCount(180, 180)).toBe(180)
  })

  it('从完整名册中搜索，并对匹配项分批显示', () => {
    const roster = [...Array.from({ length: 70 }, (_, index) => `学生${index + 1}`), '目标学生']
    expect(reportFilterOptionPage(roster, '', 50)).toHaveLength(50)
    expect(reportFilterOptionCount(roster, '目标')).toBe(1)
    expect(reportFilterOptionPage(roster, '目标', 50)).toEqual(['目标学生'])
  })
})

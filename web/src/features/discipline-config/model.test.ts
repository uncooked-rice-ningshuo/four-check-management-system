import { describe, expect, it } from 'vitest'
import { calculateRuleExample, orderGroupsByInitialConfiguration, type DisciplineGroup, type DisciplineRuleVersion } from './model'

describe('calculateRuleExample', () => {
  it('applies conversion factors and operators, then matches the half-open tier', () => {
    const group: DisciplineGroup = {
      key: 'combined', name: '合并组', unit: '学时',
      terms: [
        { inspectionTypeId: 'hours', inspectionTypeName: '旷课', sourceUnit: '学时', operator: '+', factor: 1 },
        { inspectionTypeId: 'absence', inspectionTypeName: '晚自习缺勤', sourceUnit: '次', operator: '+', factor: 2 },
      ],
      tiers: [{ code: 'NOTICE', name: '通报批评', lower: 3, upper: 5, authorityNumber: '示例1号', clauseReference: '条款', teacherConfirmationRequired: false }],
    }

    expect(calculateRuleExample(group, { hours: 1, absence: 1 })).toEqual({ total: 3, tier: group.tiers[0] })
    expect(calculateRuleExample(group, { hours: 3, absence: 1 }).tier).toBeUndefined()
  })
})

it('keeps the first published group order when a later version returns groups in a different order', () => {
  const group = (key: string): DisciplineGroup => ({ key, name: key, unit: '次', terms: [], tiers: [] })
  const version = (id: string, number: number, status: DisciplineRuleVersion['status'], groups: DisciplineGroup[]): DisciplineRuleVersion => ({
    id, version: number, status, rowVersion: 1, createdBy: 'admin', createdAt: '2026-11-16T00:00:00Z', groups,
  })
  const first = version('v1', 1, 'SUPERSEDED', [group('first'), group('second')])
  const current = version('v2', 2, 'PUBLISHED', [group('second'), group('new'), group('first')])
  expect(orderGroupsByInitialConfiguration(current.groups, [current, first]).map((item) => item.key)).toEqual(['first', 'second', 'new'])
  expect(current.groups.map((item) => item.key)).toEqual(['second', 'new', 'first'])
})

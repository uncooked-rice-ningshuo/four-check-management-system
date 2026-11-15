import { describe, expect, it } from 'vitest'
import { classMatchesRange, expandRangeGroups, groupRanges, hasConflictingClasses, updateRangeConditions } from './rangeGroups'

describe('class range selection', () => {
  it('saves all selected classes and restores one multi-select without losing other conditions', () => {
    const groups = [{ gradeStage: 1, majorId: 'm1', classIds: ['c1', 'c2'] }, { gradeId: 'g2', classIds: ['c3'] }]
    expect(groupRanges(expandRangeGroups(groups))).toEqual(groups)
    expect(expandRangeGroups(groups)).toEqual([{ gradeStage: 1, majorId: 'm1', classId: 'c1' }, { gradeStage: 1, majorId: 'm1', classId: 'c2' }, { gradeId: 'g2', classId: 'c3' }])
  })
  it('does not merge an unrestricted class clause with restricted classes or drop unfinished groups', () => {
    const ranges = [{ majorId: 'm1' }, { majorId: 'm1', classId: 'c1' }, {}, {}]
    expect(expandRangeGroups(groupRanges(ranges))).toEqual(ranges)
  })
  it('removes only the selected class', () => {
    const groups = groupRanges([{ classId: 'c1' }, { classId: 'c2' }])
    groups[0].classIds = ['c2']
    expect(expandRangeGroups(groups)).toEqual([{ classId: 'c2' }])
  })
})

describe('dependent class choices', () => {
  const classes = [{ value: 'a', gradeId: 'g24', majorId: 'm1' }, { value: 'b', gradeId: 'g26', majorId: 'm1' }, { value: 'c', gradeId: 'g24', majorId: 'm2' }]
  it('filters by both grade and major', () => {
    expect(classes.filter(item => classMatchesRange(item, { gradeId: 'g24', majorId: 'm1' })).map(item => item.value)).toEqual(['a'])
  })
  it('clears incompatible selections when a parent changes and retains matching selections', () => {
    expect(updateRangeConditions({ classIds: ['a', 'b', 'c'] }, { gradeId: 'g24' }, classes).classIds).toEqual(['a', 'c'])
    expect(updateRangeConditions({ gradeId: 'g24', classIds: ['a', 'c'] }, { majorId: 'm2' }, classes).classIds).toEqual(['c'])
  })
  it('rejects contradictory saved combinations without broadening the range', () => {
    expect(hasConflictingClasses([{ gradeId: 'g24', classId: 'b' }], classes)).toBe(true)
    expect(hasConflictingClasses([{ majorId: 'm2', classId: 'a' }], classes)).toBe(true)
    expect(hasConflictingClasses([{ gradeId: 'g24', majorId: 'm1', classId: 'a' }], classes)).toBe(false)
  })
})

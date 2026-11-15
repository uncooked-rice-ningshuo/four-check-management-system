import type { OperationRange } from '../../../entities/permission/model/permission'

export type RangeGroup = Omit<OperationRange, 'classId'> & { classIds: string[] }

// Several class choices are equivalent OR clauses; the API keeps its existing range contract.
export function groupRanges(ranges: OperationRange[]): RangeGroup[] {
  const groups: RangeGroup[] = []
  for (const { classId, ...conditions } of ranges) {
    const existing = classId && groups.find(group => group.classIds.length > 0 && group.gradeId === conditions.gradeId && group.gradeStage === conditions.gradeStage && group.majorId === conditions.majorId)
    if (existing) {
      if (!existing.classIds.includes(classId)) existing.classIds.push(classId)
    } else groups.push({ ...conditions, classIds: classId ? [classId] : [] })
  }
  return groups
}

export function expandRangeGroups(groups: RangeGroup[]): OperationRange[] {
  return groups.flatMap(({ classIds, ...conditions }) => classIds.length ? classIds.map(classId => ({ ...conditions, classId })) : [conditions])
}

export type RangeClassOption = { value: string; gradeId?: string; majorId?: string }
export function classMatchesRange(option: RangeClassOption, range: Pick<OperationRange, 'gradeId' | 'majorId'>): boolean {
  return (!range.gradeId || option.gradeId === range.gradeId) && (!range.majorId || option.majorId === range.majorId)
}

export function updateRangeConditions(group: RangeGroup, change: Partial<RangeGroup>, classes: RangeClassOption[]): RangeGroup {
  const next = { ...group, ...change }
  return { ...next, classIds: next.classIds.filter(id => classes.some(option => option.value === id && classMatchesRange(option, next))) }
}

export function hasConflictingClasses(ranges: OperationRange[], classes: RangeClassOption[]): boolean {
  return ranges.some(range => range.classId && !classes.some(option => option.value === range.classId && classMatchesRange(option, range)))
}

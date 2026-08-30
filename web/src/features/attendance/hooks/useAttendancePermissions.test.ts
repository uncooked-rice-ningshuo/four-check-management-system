import { describe, expect, it } from 'vitest'
import type { EffectivePermission } from '../../../entities/permission/model/permission'
import { canInspectionAction } from './useAttendancePermissions'

describe('canInspectionAction', () => {
  it('does not expose a broad action that would encompass a narrower deny', () => {
    const permissions: EffectivePermission[] = [
      { action: 'INSPECTION_ENTER', effect: 'ALLOW', scope: { departmentId: 'department-1' }, sources: [] },
      { action: 'INSPECTION_ENTER', effect: 'DENY', scope: { departmentId: 'department-1', classId: 'blocked-class' }, sources: [] },
    ]
    expect(canInspectionAction(permissions, 'INSPECTION_ENTER', { semesterId: 'semester-1', weekId: 'week-1', inspectionTypeId: 'type-1', departmentId: 'department-1' })).toBe(false)
    expect(canInspectionAction(permissions, 'INSPECTION_ENTER', { semesterId: 'semester-1', weekId: 'week-1', inspectionTypeId: 'type-1', departmentId: 'department-1', classId: 'allowed-class' })).toBe(true)
  })
})

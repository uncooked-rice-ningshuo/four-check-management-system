import { useQuery } from '@tanstack/react-query'
import type { InspectionContext } from '../../../entities/inspection/model/inspection'
import type { EffectivePermission, PermissionScope } from '../../../entities/permission/model/permission'
import { allOptions } from '../../../shared/api/inspectionOptionsApi'
import { attendanceApi } from '../api/attendanceApi'

const permissionScopeFields: (keyof PermissionScope)[] = ['inspectionTypeId']

export function useAttendancePermissions() {
  return useQuery({ queryKey: ['effective-permissions'], queryFn: attendanceApi.effectivePermissions })
}

export function useCurrentAcademicWeek() {
  return useQuery({ queryKey: ['academic', 'current-week'], queryFn: attendanceApi.currentWeek })
}

export function useAttendanceFilterOptions() {
	return useQuery({ queryKey: ['attendance', 'filter-options'], queryFn: attendanceApi.filterOptions })
}

function scopeMatches(granted: PermissionScope, requested: PermissionScope) {
	return permissionScopeFields.every(key => !granted[key] || granted[key] === requested[key])
}

export function canInspectionAction(permissions: EffectivePermission[] | undefined, action: string, context: InspectionContext) {
  const rules = (permissions ?? []).filter(rule => rule.action === action)
  return rules.some(allow => allow.effect === 'ALLOW' && scopeMatches(allow.scope, context) && !rules.some(deny =>
    deny.effect === 'DENY' && (!deny.sourceDepartmentId || deny.sourceDepartmentId === allow.sourceDepartmentId) &&
    !deny.ranges?.length && scopesOverlap(deny.scope, context)))
}

function scopesOverlap(left: PermissionScope, right: PermissionScope) {
	return permissionScopeFields.every(key => !left[key] || !right[key] || left[key] === right[key])
}

export function useAttendanceSuggestedFilters(semesterId: string, enabled: boolean) {
 return useQuery({ queryKey: ['attendance', 'suggested-filters', semesterId], queryFn: () => allOptions('/inspections/context-options', { purpose: 'view', kind: 'weeks', typeCode: 'ATTENDANCE_RATE', semesterId }, true), enabled: enabled && Boolean(semesterId) })
}

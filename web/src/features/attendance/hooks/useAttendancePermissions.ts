import { useQuery } from '@tanstack/react-query'
import type { InspectionContext } from '../../../entities/inspection/model/inspection'
import type { EffectivePermission, PermissionScope } from '../../../entities/permission/model/permission'
import { attendanceApi } from '../api/attendanceApi'

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
  return (Object.keys(granted) as (keyof PermissionScope)[]).every(key => !granted[key] || granted[key] === requested[key])
}

export function canInspectionAction(permissions: EffectivePermission[] | undefined, action: string, context: InspectionContext) {
  let allowed = false
  for (const permission of permissions ?? []) {
		if (permission.action !== action) continue
		if (permission.effect === 'DENY' && scopesOverlap(permission.scope, context)) return false
		if (permission.effect === 'ALLOW' && scopeMatches(permission.scope, context)) allowed = true
  }
  return allowed
}

function scopesOverlap(left: PermissionScope, right: PermissionScope) {
	return (Object.keys({ ...left, ...right }) as (keyof PermissionScope)[]).every(key => !left[key] || !right[key] || left[key] === right[key])
}

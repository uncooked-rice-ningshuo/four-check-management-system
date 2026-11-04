import type { AttendanceRecord } from '../../../entities/inspection/model/inspection'
import type { EffectivePermission } from '../../../entities/permission/model/permission'
import { canInspectionAction } from './useAttendancePermissions'

// This is only an interaction hint; mutation APIs enforce the authoritative scope.
export function canMutateAttendance(record: AttendanceRecord, actor: { id: string; isSuperAdmin: boolean }, permissions?: EffectivePermission[], academicYearId?: string) {
  if (record.status !== 'DRAFT' && record.status !== 'RETURNED') return false
  if (actor.isSuperAdmin) return true
  if (!record.semesterId || !record.weekId || !record.inspectionTypeId) return false
  const scope = { academicYearId, semesterId: record.semesterId, weekId: record.weekId, departmentId: record.departmentId ?? '', inspectionTypeId: record.inspectionTypeId, classId: record.classId, grade: record.grade }
  const canManage = canInspectionAction(permissions, 'INSPECTION_MANAGE', scope)
  if (record.attributionMode === 'ADMINISTRATOR' && actor.id !== record.createdBy && !canManage) return false
  return (actor.id === record.createdBy && canInspectionAction(permissions, 'INSPECTION_ENTER', scope)) || canManage
}

/** Delete is narrower than edit: only unlocked, unsubmitted drafts are eligible. */
export function canDeleteAttendance(record: AttendanceRecord, actor: { id: string; isSuperAdmin: boolean }, permissions?: EffectivePermission[], academicYearId?: string) {
  return record.canDelete === true && canMutateAttendance(record, actor, permissions, academicYearId)
}

import type { EffectivePermission, PermissionScope } from '../../../entities/permission/model/permission'

const permissionScopeFields: (keyof PermissionScope)[] = ['academicYearId', 'semesterId', 'weekId', 'inspectionTypeId']

// A partially denied grant still provides navigation to its remaining scope.
// Only a DENY that contains the entire grant removes that entry; APIs enforce each concrete request.
export function hasNavigableInspectionAction(permissions: EffectivePermission[] | undefined, action: string, inspectionTypeId?: string) {
  const rules = (permissions ?? []).filter(rule => rule.action === action && (!inspectionTypeId || !rule.scope.inspectionTypeId || rule.scope.inspectionTypeId === inspectionTypeId))
    .map(rule => inspectionTypeId ? { ...rule, scope: { ...rule.scope, inspectionTypeId } } : rule)
  return rules.some(allow => allow.effect === 'ALLOW' && !rules.some(deny => deny.effect === 'DENY'
    && permissionScopeFields.every(key => !deny.scope[key] || deny.scope[key] === allow.scope[key])))
}

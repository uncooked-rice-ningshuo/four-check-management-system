import type { EffectivePermission, PermissionScope } from '../../../entities/permission/model/permission'

// A partially denied grant still provides navigation to its remaining scope.
// Only a DENY that contains the entire grant removes that entry; APIs enforce each concrete request.
export function hasNavigableInspectionAction(permissions: EffectivePermission[] | undefined, action: string, inspectionTypeId?: string, departmentId?: string) {
  const rules = (permissions ?? []).filter(rule => rule.action === action && (!inspectionTypeId || !rule.scope.inspectionTypeId || rule.scope.inspectionTypeId === inspectionTypeId) && (!departmentId || !rule.scope.departmentId || rule.scope.departmentId === departmentId))
    .map(rule => (inspectionTypeId || departmentId) ? { ...rule, scope: { ...rule.scope, ...(inspectionTypeId ? { inspectionTypeId } : {}), ...(departmentId ? { departmentId } : {}) } } : rule)
  return rules.some(allow => allow.effect === 'ALLOW' && !rules.some(deny => deny.effect === 'DENY'
    && (Object.keys(deny.scope) as (keyof PermissionScope)[]).every(key => !deny.scope[key] || deny.scope[key] === allow.scope[key])))
}

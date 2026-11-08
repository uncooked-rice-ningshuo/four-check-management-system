import type { EffectivePermission } from '../../../entities/permission/model/permission'
import type { ReportingPeriod } from '../../../entities/reporting/model/reporting'

const scopeFields = ['academicYearId', 'semesterId', 'weekId', 'inspectionTypeId'] as const

// Report actions require every included inspection type, including explicit DENY checks.
export function permissionCoversReportTypes(
  permissions: EffectivePermission[],
  action: string,
  period: Pick<ReportingPeriod, 'academicYearId' | 'semesterId' | 'weekId'>,
  rows: { inspectionTypeId: string }[],
) {
  if (rows.length === 0) return false
  return rows.every(row => {
    const requested = {
      academicYearId: period.academicYearId,
      semesterId: period.semesterId,
      weekId: period.weekId,
      inspectionTypeId: row.inspectionTypeId,
    }
    const rules = permissions.filter(item => item.action === action)
    const matches = (scope: EffectivePermission['scope']) => scopeFields.every(key => !scope[key] || scope[key] === requested[key])
    return rules.some(item => item.effect === 'ALLOW' && matches(item.scope)) &&
      !rules.some(item => item.effect === 'DENY' && matches(item.scope))
  })
}

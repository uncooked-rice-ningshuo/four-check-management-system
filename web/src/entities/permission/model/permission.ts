export type OperationRange = { gradeId?: string; majorId?: string; classId?: string; gradeStage?: number }
export const supportsOperationRange = (action: string) => ['INSPECTION_ENTER', 'INSPECTION_MANAGE', 'INSPECTION_REVIEW'].includes(action)
export type GrantDates = { startsAt?: string; endsAt?: string; sourceDepartmentId?: string; ranges?: OperationRange[] }
export type PermissionScope = {
  academicYearId?: string
  semesterId?: string
  weekId?: string
  inspectionTypeId?: string
}

export type PermissionRule = GrantDates & {
 mode?: 'ADD' | 'OVERRIDE' | 'DISABLE'
  id: string
  accountId: string
  action: string
  effect: 'ALLOW' | 'DENY'
  sourceType: string
  scope: PermissionScope
  note: string
  enabled: boolean
}

export type EffectivePermission = GrantDates & {
 status?: string
 current?: EffectivePermission
 templateVersion?: number
  action: string
  effect: 'ALLOW' | 'DENY'
  scope: PermissionScope
  sources: { type: string; label: string }[]
}

export type EffectivePermissions = {
 pendingDepartmentIds?: string[]
 defaultPermissions?: EffectivePermission[]
 departmentIds?: string[]
  reportGrantableActions?: string[]
  confirmedInspectionsOnly?: boolean
  canReviseFinalizedReport?: boolean
  accountId: string
  systemRole: 'SUPER_ADMIN' | 'ADMIN' | 'MEMBER'
  permissions: EffectivePermission[]
}

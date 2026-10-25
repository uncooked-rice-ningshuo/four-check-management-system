export type PermissionScope = {
  academicYearId?: string
  semesterId?: string
  weekId?: string
  inspectionTypeId?: string
}

export type PermissionRule = {
  id: string
  accountId: string
  action: string
  effect: 'ALLOW' | 'DENY'
  sourceType: string
  scope: PermissionScope
  note: string
  enabled: boolean
}

export type EffectivePermission = {
  action: string
  effect: 'ALLOW' | 'DENY'
  scope: PermissionScope
  sources: { type: string; label: string }[]
}

export type EffectivePermissions = {
  accountId: string
  systemRole: 'SUPER_ADMIN' | 'ADMIN' | 'MEMBER'
  permissions: EffectivePermission[]
}

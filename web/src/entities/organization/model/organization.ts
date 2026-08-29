export type Department = { id: string; code: string; name: string; enabled: boolean }
export type OrganizationRole = { id: string; code: string; name: string; enabled: boolean; sortOrder: number }
export type InspectionType = { id: string; code: string; name: string; extensionKind: string; enabled: boolean; participatesInReport: boolean; sortOrder: number }
export type DepartmentInspectionScope = {
  departmentId: string
  inspectionTypeId: string
  canEnter: boolean
  canView: boolean
  canReview: boolean
  canExport: boolean
  canSummarize: boolean
  enabled: boolean
}
export type OrganizationCatalogs = {
  departments: Department[]
  organizationRoles: OrganizationRole[]
  inspectionTypes: InspectionType[]
}
export type UserAssignment = {
  accountId: string
  loginName: string
  studentNo: string
  name: string
  status: string
  systemRole: 'SUPER_ADMIN' | 'ADMIN' | 'MEMBER'
  isSuperAdmin: boolean
  mustChangePassword: boolean
  departmentId: string
  organizationRoleId: string
}

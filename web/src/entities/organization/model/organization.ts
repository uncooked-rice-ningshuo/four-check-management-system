export type Department = { id: string; code: string; name: string; enabled: boolean; permissionVersion?: number }
export type OrganizationRole = { id: string; code: string; name: string; enabled: boolean; sortOrder: number }
export type InspectionType = { id: string; code: string; name: string; extensionKind: string; enabled: boolean; participatesInReport: boolean; sortOrder: number }
export type DepartmentInspectionScope = {
  departmentId: string
  inspectionTypeId: string
  canEnter: boolean
  canView: boolean
  canManage: boolean
  canReview: boolean
  canExport: boolean
  enabled: boolean
}
export type InspectionRoleDefaults = { inspectionTypeId: string; roleCode: 'MEMBER' | 'LEADERSHIP' | 'PRESIDIUM'; canView: boolean; canEnter: boolean; canManage: boolean; canReview: boolean; canExport: boolean }
export type ReportRoleDefaults = { roleCode: 'MEMBER' | 'MINISTER' | 'VICE_MINISTER'; canView: boolean; canGenerate: boolean; canCorrect: boolean; canReview: boolean; canReturn: boolean; canFinalize: boolean; canPublish: boolean; canOutputSettings: boolean; canArchive: boolean }
export type DepartmentReportScope = Omit<ReportRoleDefaults, 'roleCode'> & { enabled: boolean }
export type DepartmentDefaultPermissions = { reportMaintenance: { state: 'assigned' | 'unassigned'; departmentId: string; departmentName: string }; departmentId: string; version: number; scopes: DepartmentInspectionScope[]; inspectionRoles: InspectionRoleDefaults[]; reportScope: DepartmentReportScope; reportRoles: ReportRoleDefaults[] }
export type OrganizationCatalogs = {
  departments: Department[]
  organizationRoles: OrganizationRole[]
  inspectionTypes: InspectionType[]
}
export type UserAssignment = {
  version: number
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

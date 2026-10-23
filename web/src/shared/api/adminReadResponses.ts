import type { Department, InspectionType, OrganizationCatalogs, OrganizationRole, UserAssignment } from '../../entities/organization/model/organization'
import { ApiError } from './httpClient'

function object(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}
function strings(value: Record<string, unknown>, fields: string[]) {
  return fields.every(field => typeof value[field] === 'string')
}
function namedReference(value: unknown): value is Record<string, unknown> {
  return object(value) && strings(value, ['id', 'code', 'name']) && typeof value.id === 'string' && value.id.trim() !== '' && typeof value.enabled === 'boolean'
}
function department(value: unknown): value is Department { return namedReference(value) }
function role(value: unknown): value is OrganizationRole {
  return namedReference(value) && Number.isSafeInteger(value.sortOrder)
}
function inspection(value: unknown): value is InspectionType {
  return namedReference(value) && typeof value.extensionKind === 'string' && typeof value.participatesInReport === 'boolean' && Number.isSafeInteger(value.sortOrder)
}
function user(value: unknown): value is UserAssignment {
  return object(value) && strings(value, ['accountId', 'loginName', 'studentNo', 'name', 'status', 'departmentId', 'organizationRoleId'])
    && typeof value.accountId === 'string' && value.accountId.trim() !== ''
    && (value.systemRole === 'SUPER_ADMIN' || value.systemRole === 'ADMIN' || value.systemRole === 'MEMBER')
    && typeof value.isSuperAdmin === 'boolean' && typeof value.mustChangePassword === 'boolean'
}
function invalidResponse(): never {
  throw new ApiError(502, 'INVALID_RESPONSE', '服务返回的数据不完整，请重试或联系管理员。')
}
function unique<T>(values: T[], key: (value: T) => string) {
  return new Set(values.map(key)).size === values.length
}

// Validate required fields before the page uses identity, permission, and assignment values.
export function parseUserAssignments(value: unknown): UserAssignment[] {
  if (!Array.isArray(value) || !value.every(user) || !unique(value, item => item.accountId)) return invalidResponse()
  return value
}

// Missing collections are failures; only explicit empty arrays represent an empty catalog.
export function parseOrganizationCatalogs(value: unknown): OrganizationCatalogs {
  if (!object(value)) return invalidResponse()
  const { departments, organizationRoles, inspectionTypes } = value
  if (!Array.isArray(departments) || !departments.every(department) || !unique(departments, item => item.id)
    || !Array.isArray(organizationRoles) || !organizationRoles.every(role) || !unique(organizationRoles, item => item.id)
    || !Array.isArray(inspectionTypes) || !inspectionTypes.every(inspection) || !unique(inspectionTypes, item => item.id)) return invalidResponse()
  return { departments, organizationRoles, inspectionTypes }
}

import type { User } from '../../entities/user/model/user'
import type { AcademicCalendar, AcademicYear, Semester, SemesterWeek } from '../../entities/academic/model/academic'
import type { Department, DepartmentInspectionScope, InspectionType, OrganizationCatalogs, UserAssignment } from '../../entities/organization/model/organization'
import type { EffectivePermissions, PermissionRule } from '../../entities/permission/model/permission'
import type { MasterDataChange, MasterDataKind, MasterDataPage, MasterDataRecord } from '../../entities/masterdata/model/masterData'
import type { DictionaryCatalog, DictionaryItem, DictionaryType } from '../../entities/dictionary/model/dictionary'
import { apiRequest } from './httpClient'
import { parseOrganizationCatalogs, parseUserAssignments } from './adminReadResponses'

export const getRegistrationOptions = () => apiRequest<OrganizationCatalogs>('/public/registration-options')
export const getPendingRegistrations = () => apiRequest<User[]>('/admin/registration-requests')
export const reviewRegistration = (id: string, decision: 'APPROVE' | 'REJECT', reason = '') => apiRequest<User>(`/admin/registration-requests/${id}/review`, { method: 'POST', body: JSON.stringify({ decision, reason }) })
export const getCatalogs = async () => parseOrganizationCatalogs(await apiRequest<unknown>('/admin/catalogs'))
export const createDepartment = (input: Pick<Department, 'code' | 'name'>) => apiRequest<Department>('/admin/departments', { method: 'POST', body: JSON.stringify(input) })
export const updateDepartment = (input: Department) => apiRequest<Department>(`/admin/departments/${input.id}`, { method: 'PUT', body: JSON.stringify(input) })
export const updateInspectionType = (input: InspectionType) => apiRequest<InspectionType>(`/admin/inspection-types/${input.id}`, { method: 'PUT', body: JSON.stringify(input) })
export const getDepartmentInspectionScopes = () => apiRequest<DepartmentInspectionScope[]>('/admin/department-inspection-scopes')
export const updateDepartmentInspectionScope = (input: DepartmentInspectionScope) => apiRequest<void>('/admin/department-inspection-scopes', { method: 'PUT', body: JSON.stringify(input) })
export const getUsers = async () => parseUserAssignments(await apiRequest<unknown>('/admin/users'))
export const createUser = (input: { studentNo: string; name: string; systemRole: 'ADMIN' | 'MEMBER' }) => apiRequest<User & { temporaryPassword: string }>('/admin/users', { method: 'POST', body: JSON.stringify(input) })
export const updateUser = (id: string, input: { studentNo: string; name: string; systemRole: 'ADMIN' | 'MEMBER' }) => apiRequest<User>(`/admin/users/${id}`, { method: 'PUT', body: JSON.stringify(input) })
export const updateUserStatus = (id: string, status: 'ACTIVE' | 'DISABLED') => apiRequest<User>(`/admin/users/${id}/status`, { method: 'PUT', body: JSON.stringify({ status }) })
export const resetUserPassword = (id: string) => apiRequest<{ temporaryPassword: string }>(`/admin/users/${id}/reset-password`, { method: 'POST' })
export const assignUserRole = (input: UserAssignment) => apiRequest<void>(`/admin/users/${input.accountId}/assignment`, { method: 'PUT', body: JSON.stringify(input) })
export const getEffectivePermissions = (accountId: string) => apiRequest<EffectivePermissions>(`/admin/users/${accountId}/permissions`)
export const getPermissionRules = (accountId: string) => apiRequest<PermissionRule[]>(`/admin/users/${accountId}/permission-rules`)
export const createPermissionRule = (accountId: string, input: Omit<PermissionRule, 'id' | 'accountId' | 'sourceType' | 'enabled'>) => apiRequest<PermissionRule>(`/admin/users/${accountId}/permission-rules`, { method: 'POST', body: JSON.stringify(input) })
export const updatePermissionRule = (input: PermissionRule) => apiRequest<PermissionRule>(`/admin/users/${input.accountId}/permission-rules/${input.id}`, { method: 'PUT', body: JSON.stringify(input) })
export const deletePermissionRule = (accountId: string, ruleId: string) => apiRequest<void>(`/admin/users/${accountId}/permission-rules/${ruleId}`, { method: 'DELETE' })
export type MasterDataListParams = Partial<Pick<MasterDataRecord,
  'facultyId' | 'majorId' | 'headTeacherId' | 'teacherId' | 'classId' | 'dormitoryId' | 'classroomId' |
  'gradeId' | 'gender' | 'enrollmentStatus' | 'campusArea' | 'buildingNo' | 'dormitoryAreaId' | 'dormitoryBuildingId' | 'teachingBuildingId'>> & {
  keyword?: string
  code?: string
  name?: string
  enabled?: boolean
  semesterId?: string
  page?: number
  pageSize?: number
}

export const getMasterData = (kind: MasterDataKind, params: MasterDataListParams = {}) => {
  const query = new URLSearchParams()
  Object.entries(params).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== '') query.set(key, String(value))
  })
  query.set('page', String(params.page ?? 1))
  query.set('pageSize', String(params.pageSize ?? 20))
  return apiRequest<MasterDataPage>(`/admin/master-data/${kind}?${query}`)
}
export const createMasterData = (kind: MasterDataKind, input: Omit<MasterDataRecord, 'id' | 'kind' | 'enabled'>) => apiRequest<MasterDataRecord>(`/admin/master-data/${kind}`, { method: 'POST', body: JSON.stringify(input) })
export const updateMasterData = (input: MasterDataRecord) => apiRequest<MasterDataRecord>(`/admin/master-data/${input.kind}/${input.id}`, { method: 'PUT', body: JSON.stringify(input) })
export const getMasterDataChanges = (kind: MasterDataKind, id: string) => apiRequest<MasterDataChange[]>(`/admin/master-data/${kind}/${id}/changes`)
export const getDictionaries = (typeId = '') => apiRequest<DictionaryCatalog>(`/admin/dictionaries${typeId ? `?typeId=${encodeURIComponent(typeId)}` : ''}`)
export const createDictionaryType = (input: Omit<DictionaryType, 'id' | 'system' | 'enabled'>) => apiRequest<DictionaryType>('/admin/dictionaries/types', { method: 'POST', body: JSON.stringify(input) })
export const updateDictionaryType = (input: DictionaryType) => apiRequest<DictionaryType>(`/admin/dictionaries/types/${input.id}`, { method: 'PUT', body: JSON.stringify(input) })
export const createDictionaryItem = (typeId: string, input: Omit<DictionaryItem, 'id' | 'typeId' | 'enabled'>) => apiRequest<DictionaryItem>(`/admin/dictionaries/types/${typeId}/items`, { method: 'POST', body: JSON.stringify(input) })
export const updateDictionaryItem = (input: DictionaryItem) => apiRequest<DictionaryItem>(`/admin/dictionaries/types/${input.typeId}/items/${input.id}`, { method: 'PUT', body: JSON.stringify(input) })
export const getAcademicCalendar = () => apiRequest<AcademicCalendar>('/admin/academic/calendar')
export const createAcademicYear = (input: Omit<AcademicYear, 'id' | 'enabled'>) => apiRequest<AcademicYear>('/admin/academic/years', { method: 'POST', body: JSON.stringify(input) })
export const updateAcademicYear = (input: AcademicYear) => apiRequest<AcademicYear>(`/admin/academic/years/${input.id}`, { method: 'PUT', body: JSON.stringify(input) })
export const createSemester = (input: Omit<Semester, 'id' | 'enabled'>) => apiRequest<Semester>('/admin/academic/semesters', { method: 'POST', body: JSON.stringify(input) })
export const updateSemester = (input: Semester) => apiRequest<Semester>(`/admin/academic/semesters/${input.id}`, { method: 'PUT', body: JSON.stringify(input) })
export const createWeek = (input: Omit<SemesterWeek, 'id' | 'enabled'>) => apiRequest<SemesterWeek>('/admin/academic/weeks', { method: 'POST', body: JSON.stringify(input) })
export const updateWeek = (input: SemesterWeek) => apiRequest<SemesterWeek>(`/admin/academic/weeks/${input.id}`, { method: 'PUT', body: JSON.stringify(input) })

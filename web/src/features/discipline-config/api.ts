import { apiRequest } from '../../shared/api/httpClient'
import type { DisciplineRuleVersion } from './model'

export const getCurrentDisciplineRule = () => apiRequest<{ current: DisciplineRuleVersion | null }>('/admin/discipline/rule-versions/current')
export const getDisciplineRuleVersions = () => apiRequest<{ items: DisciplineRuleVersion[] }>('/admin/discipline/rule-versions')
export const getDisciplineRuleVersion = (id: string) => apiRequest<DisciplineRuleVersion>(`/admin/discipline/rule-versions/${id}`)
export const createDisciplineRuleDraft = () => apiRequest<DisciplineRuleVersion>('/admin/discipline/rule-versions', { method: 'POST' })
export const saveDisciplineRuleDraft = (version: DisciplineRuleVersion) => apiRequest<DisciplineRuleVersion>(`/admin/discipline/rule-versions/${version.id}`, { method: 'PUT', body: JSON.stringify(version) })
export const validateDisciplineRuleDraft = (id: string) => apiRequest<{ valid: boolean }>(`/admin/discipline/rule-versions/${id}/validate`, { method: 'POST' })
export const publishDisciplineRuleDraft = (id: string, rowVersion: number) => apiRequest<DisciplineRuleVersion>(`/admin/discipline/rule-versions/${id}/publish`, { method: 'POST', body: JSON.stringify({ rowVersion }) })

import { apiRequest } from './httpClient'

export type DisciplineArchiveFilter = {
  caseId?: string
  student?: string
  status?: 'APPROVED' | 'VOID'
  page: number
  pageSize: number
}

export type ArchivedDecision = {
  id: string
  studentNumber: string
  semester: string
  group: string
  tier: string
  status: 'APPROVED' | 'VOID'
  amount: number
  decidedAt: string | null
  clauseReference?: string
  authorityNumber?: string
  sources?: {record:string;week:number;version:number;amount:number}[]
  voidReason?: string | null
  voidedAt?: string | null
  reason?: string
  decisionSource: string | null
}

export function searchArchivedDecisions(filter: DisciplineArchiveFilter) {
  return apiRequest<{ items: ArchivedDecision[]; total: number }>('/discipline/archive/search', {
    method: 'POST',
    body: JSON.stringify(filter),
  })
}

export function exportArchivedDecisions(filter: DisciplineArchiveFilter) {
  return apiRequest<Blob>('/discipline/archive/export', { method:'POST', body:JSON.stringify(filter) }, 'blob')
}

import { apiRequest } from './httpClient'

export type ReviewItem = {
  canProcess?: boolean; id: string; studentNumber: string; className: string; tier: string; amount: number
  studentName?: string | null; gradeName?: string | null; majorName?: string | null
  groupName?: string | null; unit?: string | null
  status: 'PENDING' | 'DEFERRED'; version: number; reason: string | null; dueAt: string
}
export type ReviewFilter = { student?: string; status?: string; page: number; pageSize: number }
const post = (body: unknown) => ({ method: 'POST', body: JSON.stringify(body) })
export const searchReviews = (filter: ReviewFilter) =>
  apiRequest<{ items: ReviewItem[]; total: number }>('/discipline/cases/search', post(filter))
export const submitReview = (item: ReviewItem, decision: 'APPROVED' | 'DEFERRED', reason: string) =>
  apiRequest<void>('/discipline/cases/' + encodeURIComponent(item.id) + '/review', post({ decision, reason, version: item.version }))

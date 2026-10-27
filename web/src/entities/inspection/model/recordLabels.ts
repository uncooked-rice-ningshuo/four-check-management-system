// Display-only labels for record status and source codes returned by the API.
// Unknown values are shown unchanged so a contract drift stays visible instead of silently hidden.
export const RECORD_STATUS_LABELS: Record<string, string> = {
  DRAFT: '草稿', PENDING_REVIEW: '待审核', RETURNED: '已退回', CONFIRMED: '已确认', LOCKED: '已锁定',
}

export const RECORD_SOURCE_LABELS: Record<string, string> = {
  MANUAL: '手工录入', EXCEL: 'Excel 导入', REPORT_CORRECTION: '通报修正', MIGRATION: '历史迁移',
}

export function recordStatusLabel(status?: string): string {
  if (!status) return '—'
  return RECORD_STATUS_LABELS[status] ?? status
}

export function recordSourceLabel(source?: string): string {
  if (!source) return '—'
  return RECORD_SOURCE_LABELS[source] ?? source
}

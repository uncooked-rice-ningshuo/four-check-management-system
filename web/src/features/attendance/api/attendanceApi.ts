import type { ManualAttendanceInput, AttendanceFilterOptions, AttendanceFilters, AttendanceRecord, CurrentAcademicWeek, Page } from '../../../entities/inspection/model/inspection'
import { apiRequest } from '../../../shared/api/httpClient'
import type { EffectivePermissions } from '../../../entities/permission/model/permission'

function query(input: Record<string, string | number | undefined>) {
  const search = new URLSearchParams()
  Object.entries(input).forEach(([key, value]) => { if (value !== undefined && value !== '') search.set(key, String(value)) })
  return search.toString()
}

export const attendanceApi = {
  detail(id: string) { return apiRequest<AttendanceRecord>(`/inspections/attendance/${encodeURIComponent(id)}`) },
  update(input: { id: string; version: number; reason: string; record: ManualAttendanceInput }) {
    const { id, ...body } = input
    return apiRequest<AttendanceRecord>(`/inspections/attendance/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(body) })
  },
  remove(input: { items: { id: string; version: number }[]; reason: string }) {
    if (input.items.length === 1) return apiRequest<void>(`/inspections/attendance/${encodeURIComponent(input.items[0].id)}`, { method: 'DELETE', body: JSON.stringify({ version: input.items[0].version, reason: input.reason }) })
    return apiRequest<void>('/inspections/attendance/batch', { method: 'POST', body: JSON.stringify({ ...input, action: 'DELETE' }) })
  },
  create(input: ManualAttendanceInput) {
    return apiRequest<AttendanceRecord>('/inspections/attendance', { method: 'POST', body: JSON.stringify(input) })
  },
  list(filters: AttendanceFilters) {
    return apiRequest<Page<AttendanceRecord>>(`/inspections/attendance?${query(filters)}`)
  },
	effectivePermissions() { return apiRequest<EffectivePermissions>('/permissions/effective') },
	currentWeek() { return apiRequest<{ week: CurrentAcademicWeek | null }>('/academic/current-week') },
	filterOptions() { return apiRequest<AttendanceFilterOptions>('/inspections/attendance/filter-options') },
}

export { query as attendanceQuery }

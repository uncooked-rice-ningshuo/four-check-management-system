import type { AttendanceFilters, AttendanceRecord, CurrentAcademicWeek, Page } from '../../../entities/inspection/model/inspection'
import { apiRequest } from '../../../shared/api/httpClient'
import type { EffectivePermissions } from '../../../entities/permission/model/permission'

function query(input: Record<string, string | number | undefined>) {
  const search = new URLSearchParams()
  Object.entries(input).forEach(([key, value]) => { if (value !== undefined && value !== '') search.set(key, String(value)) })
  return search.toString()
}

export const attendanceApi = {
  list(filters: AttendanceFilters) {
    return apiRequest<Page<AttendanceRecord>>(`/inspections/attendance?${query(filters)}`)
  },
	effectivePermissions() { return apiRequest<EffectivePermissions>('/permissions/effective') },
	currentWeek() { return apiRequest<{ week: CurrentAcademicWeek | null }>('/academic/current-week') },
}

export { query as attendanceQuery }

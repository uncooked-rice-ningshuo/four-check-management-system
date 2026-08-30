import { useQuery } from '@tanstack/react-query'
import type { AttendanceFilters } from '../../../entities/inspection/model/inspection'
import { attendanceApi } from '../api/attendanceApi'

export const attendanceKeys = {
  all: ['attendance'] as const,
  list: (filters: AttendanceFilters) => [...attendanceKeys.all, 'list', filters] as const,
}

export function useAttendanceRecords(filters: AttendanceFilters, enabled = true) {
  return useQuery({ queryKey: attendanceKeys.list(filters), queryFn: () => attendanceApi.list(filters), enabled })
}

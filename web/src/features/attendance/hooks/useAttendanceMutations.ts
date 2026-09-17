import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { attendanceApi } from '../api/attendanceApi'
import { ApiError } from '../../../shared/api/httpClient'

export function useAttendanceDetail(id?: string) {
  return useQuery({ queryKey: ['attendance', 'detail', id], queryFn: () => attendanceApi.detail(id ?? ''), enabled: Boolean(id) })
}
function useMutationRefresh() {
  const client = useQueryClient()
  const refresh = () => Promise.all([client.invalidateQueries({ queryKey: ['attendance'] }), client.invalidateQueries({ queryKey: ['dashboard', 'workbench'] })])
  return { onSuccess: refresh, onError: (error: Error) => { if (error instanceof ApiError && [403, 404, 409].includes(error.status)) return refresh() } }
}
export function useUpdateAttendance() { return useMutation({ mutationFn: attendanceApi.update, ...useMutationRefresh() }) }
export function useDeleteAttendance() { return useMutation({ mutationFn: attendanceApi.remove, ...useMutationRefresh() }) }

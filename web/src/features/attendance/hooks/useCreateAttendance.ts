import { useMutation, useQueryClient } from '@tanstack/react-query'
import { attendanceApi } from '../api/attendanceApi'
import { ApiError } from '../../../shared/api/httpClient'

// Refresh both the list and personal todos after creation or duplicate conflict.
export function useCreateAttendance() {
  const client = useQueryClient()
  const refresh = () => Promise.all([
    client.invalidateQueries({ queryKey: ['attendance'] }),
    client.invalidateQueries({ queryKey: ['dashboard', 'workbench'] }),
  ])
  return useMutation({ mutationFn: attendanceApi.create, onSuccess: refresh, onError: error => {
    if (error instanceof ApiError && error.status === 409) return refresh()
  } })
}

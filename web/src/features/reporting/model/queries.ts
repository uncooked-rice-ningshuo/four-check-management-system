import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { reportingApi } from '../../../shared/api/reportingApi'
import { ApiError } from '../../../shared/api/httpClient'
import type { CreateReportObligationInput, ReportObligationPeriodQuery, ReportingRecordsQuery } from '../../../entities/reporting/model/reporting'

export function useReportingPeriods() {
  return useQuery({ queryKey: ['reports', 'periods'], queryFn: reportingApi.periods })
}

export function useReportingRecords(query: ReportingRecordsQuery, enabled: boolean, onAccessDenied: () => void) {
  return useQuery({
    queryKey: ['reports', 'records', query],
    queryFn: async () => {
      try {
        return await reportingApi.records(query)
      } catch (error) {
        if (error instanceof ApiError && error.status === 403 && error.code === 'FORBIDDEN_SCOPE') onAccessDenied()
        throw error
      }
    },
    enabled: enabled && Boolean(query.semesterId && query.weekId && query.inspectionTypeId),
  })
}

export function useReportingSummary(semesterId: string, weekId: string, enabled: boolean, onAccessDenied: () => void) {
  return useQuery({
    queryKey: ['reports', 'summary', semesterId, weekId],
    queryFn: async () => {
      try {
        return await reportingApi.summary(semesterId, weekId)
      } catch (error) {
        if (error instanceof ApiError && error.status === 403 && error.code === 'FORBIDDEN_SCOPE') onAccessDenied()
        throw error
      }
    },
    enabled: enabled && Boolean(semesterId && weekId),
  })
}

export function useReportObligationOptions(query: ReportObligationPeriodQuery, enabled: boolean) {
  return useQuery({
    queryKey: ['reports', 'obligation-options', query.semesterId, query.weekId],
    queryFn: () => reportingApi.obligationOptions(query.semesterId, query.weekId),
    enabled: enabled && Boolean(query.semesterId && query.weekId),
  })
}

export function useReportObligations(query: ReportObligationPeriodQuery, enabled: boolean) {
  return useQuery({
    queryKey: ['reports', 'obligations', query.semesterId, query.weekId],
    queryFn: () => reportingApi.obligations(query.semesterId, query.weekId),
    enabled: enabled && Boolean(query.semesterId && query.weekId),
  })
}

export function useCreateReportObligation(query: ReportObligationPeriodQuery) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateReportObligationInput) => reportingApi.createObligation(input),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['reports', 'obligations', query.semesterId, query.weekId] }),
        queryClient.invalidateQueries({ queryKey: ['reports', 'summary', query.semesterId, query.weekId] }),
      ])
    },
  })
}

export function useRemoveReportObligation(query: ReportObligationPeriodQuery) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: { id: string; version: number }) => reportingApi.removeObligation(input),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['reports', 'obligations', query.semesterId, query.weekId] }),
        queryClient.invalidateQueries({ queryKey: ['reports', 'summary', query.semesterId, query.weekId] }),
      ])
    },
  })
}

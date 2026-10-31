import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { reportingApi } from '../../../shared/api/reportingApi'
import { ApiError } from '../../../shared/api/httpClient'
import type { AddReportAdjustmentInput, AddReportCorrectionInput, CreateReportObligationInput, GenerateReportDraftInput, ReportArchiveQuery, ReportObligationPeriodQuery, ReportTransitionAction, ReportTransitionInput, ReportVersionRecordsQuery, ReportingRecordsQuery } from '../../../entities/reporting/model/reporting'

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

export function useReportWorkflow(semesterId: string, weekId: string, enabled: boolean, onAccessDenied?: () => void) {
  return useQuery({
    queryKey: ['reports', 'workflow', semesterId, weekId],
    queryFn: async () => {
      try {
        return await reportingApi.workflow(semesterId, weekId)
      } catch (error) {
        if (error instanceof ApiError && error.status === 403 && error.code === 'FORBIDDEN_SCOPE') onAccessDenied?.()
        throw error
      }
    },
    enabled: enabled && Boolean(semesterId && weekId),
  })
}

export function useReportVersionRecords(query: ReportVersionRecordsQuery, enabled: boolean, onAccessDenied: () => void) {
  return useQuery({
    queryKey: ['reports', 'version-records', query],
    queryFn: async () => {
      try {
        return await reportingApi.versionRecords(query)
      } catch (error) {
        if (error instanceof ApiError && error.status === 403 && error.code === 'FORBIDDEN_SCOPE') onAccessDenied()
        throw error
      }
    },
    enabled: enabled && Boolean(query.semesterId && query.weekId && query.versionNo > 0),
  })
}

export function useAddReportCorrection(query: ReportObligationPeriodQuery) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: { reportId: string; body: AddReportCorrectionInput }) => reportingApi.addCorrection(input.reportId, input.body),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['reports', 'workflow', query.semesterId, query.weekId] }),
        queryClient.invalidateQueries({ queryKey: ['reports', 'archive'] }),
        queryClient.invalidateQueries({ queryKey: ['reports', 'version-records'] }),
      ])
    },
  })
}

export function useAddReportAdjustment(query: ReportObligationPeriodQuery) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: { reportId: string; body: AddReportAdjustmentInput }) => reportingApi.addAdjustment(input.reportId, input.body),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['reports', 'workflow', query.semesterId, query.weekId] }),
        queryClient.invalidateQueries({ queryKey: ['reports', 'archive'] }),
        queryClient.invalidateQueries({ queryKey: ['reports', 'version-records'] }),
      ])
    },
  })
}

export function useReportArchive(query: ReportArchiveQuery, onAccessDenied: () => void) {
  return useQuery({
    queryKey: ['reports', 'archive', query],
    queryFn: async () => {
      try {
        return await reportingApi.archive(query)
      } catch (error) {
        if (error instanceof ApiError && error.status === 403 && error.code === 'FORBIDDEN_SCOPE') onAccessDenied()
        throw error
      }
    },
  })
}

export function useReportArchiveWorkflow(semesterId: string, weekId: string, enabled: boolean, onAccessDenied: () => void) {
  return useQuery({
    queryKey: ['reports', 'archive-workflow', semesterId, weekId],
    queryFn: async () => {
      try {
        return await reportingApi.archiveWorkflow(semesterId, weekId)
      } catch (error) {
        if (error instanceof ApiError && error.status === 403 && error.code === 'FORBIDDEN_SCOPE') onAccessDenied()
        throw error
      }
    },
    enabled: enabled && Boolean(semesterId && weekId),
  })
}

export function useGenerateReportDraft(query: ReportObligationPeriodQuery) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: GenerateReportDraftInput) => reportingApi.generateDraft(input),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['reports', 'workflow', query.semesterId, query.weekId] }),
        queryClient.invalidateQueries({ queryKey: ['reports', 'summary', query.semesterId, query.weekId] }),
      ])
    },
  })
}

export function useReportTransition(query: ReportObligationPeriodQuery) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: { reportId: string; action: ReportTransitionAction; body: ReportTransitionInput }) =>
      reportingApi.transition(input.reportId, input.action, input.body),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['reports', 'workflow', query.semesterId, query.weekId] })
      await queryClient.invalidateQueries({ queryKey: ['reports', 'archive'] })
    },
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

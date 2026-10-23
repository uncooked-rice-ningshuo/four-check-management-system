import { useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import type { InspectionQuery, InspectionOption } from '../../../shared/api/inspectionOptionsApi'
import { useCurrentAcademicWeek } from './useAttendancePermissions'

const filterKeys = ['departmentId', 'classId', 'status', 'source'] as const

/** Keep an explicit list choice in the URL; use the work week only when no choice exists. */
export function useInspectionListScope(semesters?: InspectionOption[]) {
  const currentWeek = useCurrentAcademicWeek()
  const [search, setSearch] = useSearchParams()
  const [draftSemester, setDraftSemester] = useState<string | undefined>()
  const requestedSemester = search.get('semesterId')
  const candidateSemester = requestedSemester ?? currentWeek.data?.week?.semesterId
  const semesterId = semesters?.some(item => item.id === candidateSemester) ? candidateSemester : undefined
  const page = Number(search.get('page'))
  const pageSize = Number(search.get('pageSize'))
  const resolve = (weeks?: InspectionOption[]) => {
    const weekForSemester = currentWeek.data?.week && currentWeek.data.week.semesterId === semesterId ? currentWeek.data.week.id : undefined
    const candidateWeek = search.get('weekId') ?? weekForSemester
    const weekId = weeks?.some(item => item.id === candidateWeek) ? candidateWeek : undefined
    const invalidSelection = !!semesters && ((!!candidateSemester && !semesterId) || (!!weeks && !!candidateWeek && !weekId))
    const filters: InspectionQuery = {
      semesterId, weekId,
      page: Number.isInteger(page) && page > 0 ? page : 1,
      pageSize: Number.isInteger(pageSize) && pageSize > 0 ? pageSize : 20,
    }
    for (const key of filterKeys) filters[key] = search.get(key) ?? undefined
    return { weekId, filters, ready: !!semesterId && !!weekId, invalidSelection }
  }
  const update = (values: InspectionQuery) => {
    const next = new URLSearchParams()
    for (const [key, value] of Object.entries(values)) if (value !== undefined && value !== '') next.set(key, String(value))
    setSearch(next)
    setDraftSemester(undefined)
  }
  return {
    currentWeek, semesterId, resolve, update,
    optionSemesterId: draftSemester === undefined ? semesterId : draftSemester,
    setDraftSemester,
  }
}

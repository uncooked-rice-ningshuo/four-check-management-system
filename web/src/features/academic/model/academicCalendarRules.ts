export type SemesterCode = 'FIRST' | 'SECOND'

export const semesterTypeOptions = [
  { label: '上学期', value: 'FIRST' as const },
  { label: '下学期', value: 'SECOND' as const },
]

export function academicYearOption(startYear: number) {
  return { label: `${startYear}-${startYear + 1} 学年`, value: startYear }
}

export function academicYearOptions(anchorYear = new Date().getFullYear()) {
  return Array.from({ length: 11 }, (_, index) => academicYearOption(anchorYear - 5 + index))
}

export function currentAcademicYearStart(now = new Date()) {
  return now.getMonth() >= 8 ? now.getFullYear() : now.getFullYear() - 1
}

export function buildAcademicYearDefaults(startYear: number) {
  return {
    name: academicYearOption(startYear).label,
    startDate: `${startYear}-09-01`,
    endDate: `${startYear + 1}-08-31`,
  }
}

export function buildSemesterDefaults(startYear: number, code: SemesterCode) {
  const suffix = code === 'FIRST' ? '上学期' : '下学期'
  return {
    code,
    name: `${academicYearOption(startYear).label}${suffix}`,
    startDate: code === 'FIRST' ? `${startYear}-09-01` : `${startYear + 1}-02-01`,
    endDate: code === 'FIRST' ? `${startYear + 1}-01-31` : `${startYear + 1}-08-31`,
  }
}

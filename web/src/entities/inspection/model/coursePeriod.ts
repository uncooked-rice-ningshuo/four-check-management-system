export type CoursePeriodRange = { start: number; end: number }

export const COURSE_PERIOD_NUMBERS = Array.from({ length: 10 }, (_, index) => index + 1)

const coursePeriodPattern = /^(?:第)?(10|[1-9])(?:[-—－](10|[1-9]))?(?:节)?$/

export function parseCoursePeriod(value?: string | null): CoursePeriodRange | undefined {
  const match = coursePeriodPattern.exec((value ?? '').replace(/\s/g, ''))
  if (!match) return undefined

  const start = Number(match[1])
  const end = match[2] ? Number(match[2]) : start
  return start <= end ? { start, end } : undefined
}

export function formatCoursePeriod(start?: number, end?: number): string | undefined {
  if (start === undefined || end === undefined || !Number.isInteger(start) || !Number.isInteger(end) || start < 1 || end > 10 || start > end) return undefined
  return `${start}-${end}`
}

export function coursePeriodHours(start?: number, end?: number): number | undefined {
  if (formatCoursePeriod(start, end) === undefined || start === undefined || end === undefined) return undefined
  return end - start + 1
}

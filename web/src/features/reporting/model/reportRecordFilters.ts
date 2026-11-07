const optionBatchSize = 50

export function nextReportFilterOptionCount(currentCount: number, totalCount: number): number {
  return Math.min(totalCount, currentCount + optionBatchSize)
}

export function reportFilterOptionCount(values: readonly string[], searchQuery: string): number {
  const normalizedQuery = searchQuery.trim().toLocaleLowerCase()
  return normalizedQuery
    ? values.filter(value => value.toLocaleLowerCase().includes(normalizedQuery)).length
    : values.length
}

export function reportFilterOptionPage(values: readonly string[], searchQuery: string, visibleCount: number): string[] {
  const normalizedQuery = searchQuery.trim().toLocaleLowerCase()
  const matches = normalizedQuery
    ? values.filter(value => value.toLocaleLowerCase().includes(normalizedQuery))
    : values
  return matches.slice(0, visibleCount)
}

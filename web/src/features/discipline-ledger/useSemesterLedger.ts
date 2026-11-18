import { useMutation, useQuery } from '@tanstack/react-query'
import { downloadSemesterLedger, readSemesterLedger, type LedgerFilter } from '../../shared/api/semesterLedgerApi'

export function useSemesterLedger(semester: string, filter: LedgerFilter, page: number, size: number) {
  return useQuery({ queryKey: ['semester-ledger', semester, filter, page, size], queryFn: () => readSemesterLedger(semester, filter, page, size) })
}

export function useLedgerExport(semester: string, filter: LedgerFilter) {
  return useMutation({ mutationFn: () => downloadSemesterLedger(semester, filter), onSuccess: blob => {
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = '学期违纪台账.xlsx'
    link.click()
    window.setTimeout(() => URL.revokeObjectURL(url), 60000)
  } })
}

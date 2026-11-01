import type { ReportCountRow, ReportingSummaryRow } from '../../../entities/reporting/model/reporting'

export function groupVersionRows(rows: ReportCountRow[]): ReportCountRow[] {
  const grouped = new Map<string, ReportCountRow>()
  for (const row of rows) {
    const current = grouped.get(row.inspectionTypeId)
    if (current) {
      current.recordCount += row.recordCount
      current.confirmedZeroReceiptCount += row.confirmedZeroReceiptCount
      current.dataStatus = current.recordCount > 0 ? 'CONFIRMED_RECORDS'
        : current.confirmedZeroReceiptCount > 0 ? 'CONFIRMED_ZERO_RECORD_RECEIPT' : 'NO_CONFIRMED_DATA'
    } else grouped.set(row.inspectionTypeId, { ...row, departmentId: '', departmentName: '', collegeLevel: false })
  }
  return [...grouped.values()]
}

export function groupSummaryRows(rows: ReportingSummaryRow[]): ReportingSummaryRow[] {
  const grouped = new Map<string, ReportingSummaryRow>()
  for (const row of rows) {
    const current = grouped.get(row.inspectionTypeId)
    if (current) {
      current.submissionCount += row.submissionCount
      current.draftBatchCount += row.draftBatchCount
      current.pendingReviewBatchCount += row.pendingReviewBatchCount
      current.returnedBatchCount += row.returnedBatchCount
      current.confirmedBatchCount += row.confirmedBatchCount
      current.confirmedRecordCount += row.confirmedRecordCount
      current.pendingReviewOrReturnedRecordCount += row.pendingReviewOrReturnedRecordCount
      current.zeroRecordPendingReviewBatchCount += row.zeroRecordPendingReviewBatchCount
      current.zeroRecordReturnedBatchCount += row.zeroRecordReturnedBatchCount
      current.zeroRecordConfirmedBatchCount += row.zeroRecordConfirmedBatchCount
      current.isRequired ||= row.isRequired
    } else grouped.set(row.inspectionTypeId, { ...row, departmentId: '', departmentName: '', collegeLevel: false })
  }
  return [...grouped.values()]
}

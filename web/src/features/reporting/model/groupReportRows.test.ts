import { describe, expect, it } from 'vitest'
import type { ReportCountRow, ReportingSummaryRow } from '../../../entities/reporting/model/reporting'
import { groupSummaryRows, groupVersionRows } from './groupReportRows'

describe('按检查类型汇总', () => {
  it('合并不同来源的批次和记录，仍把零回执与记录分开', () => {
    const first: ReportingSummaryRow = { departmentId: 'a', departmentName: '学习部', collegeLevel: false,
      inspectionTypeId: 'type-1', inspectionTypeName: '到课率', isRequired: true, submissionCount: 2, draftBatchCount: 0,
      pendingReviewBatchCount: 1, returnedBatchCount: 0, confirmedBatchCount: 1, confirmedRecordCount: 3,
      pendingReviewOrReturnedRecordCount: 1, zeroRecordPendingReviewBatchCount: 0,
      zeroRecordReturnedBatchCount: 0, zeroRecordConfirmedBatchCount: 0 }
    const result = groupSummaryRows([first, { ...first, departmentId: '', departmentName: '管理员', collegeLevel: true,
      submissionCount: 1, pendingReviewBatchCount: 0, confirmedBatchCount: 1, confirmedRecordCount: 0,
      pendingReviewOrReturnedRecordCount: 0, zeroRecordConfirmedBatchCount: 1 }])
    expect(result).toHaveLength(1)
    expect(result[0]).toMatchObject({ submissionCount: 3, confirmedRecordCount: 3, zeroRecordConfirmedBatchCount: 1 })
  })

  it('同一版本跨来源合并，保留记录与零回执的不同含义', () => {
    const rows: ReportCountRow[] = [
      { departmentId: 'a', departmentName: '学习部', collegeLevel: false, inspectionTypeId: 'type-1', inspectionType: '到课率', recordCount: 2, confirmedZeroReceiptCount: 0, dataStatus: 'CONFIRMED_RECORDS' },
      { departmentId: '', departmentName: '管理员', collegeLevel: true, inspectionTypeId: 'type-1', inspectionType: '到课率', recordCount: 0, confirmedZeroReceiptCount: 1, dataStatus: 'CONFIRMED_ZERO_RECORD_RECEIPT' },
    ]
    expect(groupVersionRows(rows)).toMatchObject([{ recordCount: 2, confirmedZeroReceiptCount: 1 }])
  })
})

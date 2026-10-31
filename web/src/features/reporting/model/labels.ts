/** Human-readable names for report snapshots and audit entries. */
const sourceStatuses: Record<string, string> = {
  NO_CONFIRMED_DATA: '暂无已确认数据',
  CONFIRMED_RECORDS: '已确认记录',
  CONFIRMED_ZERO_RECORD_RECEIPT: '已确认零记录',
  CONFIRMED_RECORDS_AND_ZERO_RECEIPT: '已确认记录及零记录回执',
}

const reportStatuses: Record<string, string> = {
  DRAFT: '草稿', PENDING_REVIEW: '待审核', RETURNED: '已退回', FINALIZING: '审核通过，待定稿确认',
  FINALIZED: '已定稿', PUBLISHED: '已发布', ARCHIVED: '已归档',
}

const reviewActions: Record<string, string> = {
  SUBMITTED: '提交审核', APPROVED: '审核通过', RETURNED: '退回修改', FINALIZATION_STARTED: '进入定稿',
  FINALIZED: '确认定稿', PUBLISHED: '已发布', ARCHIVED: '已归档',
}

const reconfirmationStatuses: Record<string, string> = {
  NOT_REQUIRED: '无需部门重确认', REQUIRED: '待部门重确认', CONFIRMED: '部门已确认',
}

const correctionFieldLabels: Record<string, string> = {
  studentNumber: '学号', studentName: '姓名', className: '班级', grade: '年级',
  currentDormitoryName: '宿舍', currentCourseName: '课程', currentClassroomName: '教室',
  currentAreaName: '检查区域', currentReasonLabel: '原因类别', 'details.date': '检查日期',
  'details.time': '发生时间', 'details.period': '节次', 'details.round': '检查轮次',
  'details.absenceHours': '缺课数量', 'details.penaltyHours': '违纪计入数量', 'details.absenceCount': '缺勤次数',
  'details.result': '检查结果', 'details.reasonCode': '原因类别', 'details.description': '情况说明', 'details.remark': '备注',
}

const inspectionResults: Record<string, string> = { EXCELLENT: '优秀', QUALIFIED: '合格', UNQUALIFIED: '不合格' }
const labelFor = (labels: Record<string, string>, value: string, fallback: string) => Object.hasOwn(labels, value) ? labels[value] : fallback

export const reportSourceStatusLabel = (value: string) => labelFor(sourceStatuses, value, '未知来源状态')
export const reportStatusLabel = (value: string) => labelFor(reportStatuses, value, '未知通报状态')
export const reportReviewActionLabel = (value: string) => labelFor(reviewActions, value, '未知操作')
export const reportReconfirmationLabel = (value: string) => labelFor(reconfirmationStatuses, value, '未知确认状态')
export const reportCorrectionFieldLabel = (value: string) => labelFor(correctionFieldLabels, value, '未知修正字段')
export const inspectionResultLabel = (value: string) => labelFor(inspectionResults, value, '未知检查结果')

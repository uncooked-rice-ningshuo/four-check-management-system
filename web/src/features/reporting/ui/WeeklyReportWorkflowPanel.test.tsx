import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { StrictMode } from 'react'
import { describe, expect, it, vi } from 'vitest'
import type { ReportWorkflow, ReportingPeriod, ReportingRecord, ReportingSummaryRow } from '../../../entities/reporting/model/reporting'
import { WeeklyReportWorkflowPanel } from './WeeklyReportWorkflowPanel'

const mocks = vi.hoisted(() => ({ workflow: vi.fn(), records: vi.fn(), correct: vi.fn() }))
vi.mock('../model/queries', () => ({
  useReportWorkflow: mocks.workflow,
  useReportVersionRecords: mocks.records,
  useGenerateReportDraft: () => ({ mutate: vi.fn(), isPending: false }),
  useReportTransition: () => ({ mutate: vi.fn(), isPending: false }),
  useAddReportAdjustment: () => ({ mutate: vi.fn(), isPending: false }),
  useAddReportCorrection: () => ({ mutate: mocks.correct, isPending: false }),
}))

const period: ReportingPeriod = { academicYearId: 'year-1', academicYearName: '学年', semesterId: 'semester-1', semesterName: '学期', weekId: 'week-1', weekNo: 1, startDate: '2026-03-01', endDate: '2026-03-07' }
const summaryRow: ReportingSummaryRow = { departmentId: 'department-1', departmentName: '宿管部', collegeLevel: false, inspectionTypeId: 'type-1', inspectionTypeName: '宿舍卫生', isRequired: false, submissionCount: 1, draftBatchCount: 0, pendingReviewBatchCount: 0, returnedBatchCount: 0, confirmedBatchCount: 1, confirmedRecordCount: 1, pendingReviewOrReturnedRecordCount: 0, zeroRecordPendingReviewBatchCount: 0, zeroRecordReturnedBatchCount: 0, zeroRecordConfirmedBatchCount: 0 }
const record: ReportingRecord = { id: 'record-1', studentNumber: '', studentName: '测试学生', className: '测试班级', grade: '', typeCode: 'DORMITORY_HYGIENE', details: { result: 'UNQUALIFIED' }, currentDormitoryName: '', currentCourseName: '', currentClassroomName: '' }
const filterOptions = { studentNumbers: ['20260001'], studentNames: ['测试学生'], classNames: ['测试班级'], grades: ['2026级'] }
const workflow: ReportWorkflow = {
  report: { id: 'report-1', semesterId: period.semesterId, weekId: period.weekId, status: 'DRAFT', versionNo: 1, rowVersion: 1, updatedAt: '2026-03-01' },
  versions: [{ id: 'version-1', versionNo: 1, createdBy: 'actor', createdAt: '2026-03-01', content: {
    period, scopeLimited: false, statisticsRuleVersion: 'v1', confirmedRecordCount: 1,
    rows: [{ departmentId: 'department-1', departmentName: '宿管部', collegeLevel: false, inspectionTypeId: 'type-1', inspectionType: '宿舍卫生', recordCount: 1, confirmedZeroReceiptCount: 0, dataStatus: 'CONFIRMED_RECORDS' }],
    incompleteCells: [], corrections: [], generatedAt: '2026-03-01',
  } }], reviews: [], corrections: [],
}

describe('通报记录修正', () => {
  it('草稿明细按基本信息列展示，并合并时间、课程地点和检查情况', async () => {
    const attendanceWorkflow = structuredClone(workflow)
    attendanceWorkflow.versions[0].content.rows[0].inspectionType = '到课率'
    mocks.workflow.mockReturnValue({ data: attendanceWorkflow, isError: false, isPending: false })
    mocks.records.mockReturnValue({ data: { items: [{ ...record, studentNumber: '20260001', typeCode: 'ATTENDANCE_RATE', currentCourseName: '高等数学', currentClassroomName: '教学楼 101', details: { date: '2026-03-02', time: '08:00', period: '1-2', absenceHours: 2, penaltyHours: 2 } }], total: 1, filterOptions }, isError: false, isPending: false, isFetching: false })
    render(<WeeklyReportWorkflowPanel period={period} summaryRows={[summaryRow]} permissions={[]} onAccessDenied={vi.fn()} stage="draft" />)
    fireEvent.click(screen.getAllByRole('button', { name: '查看明细' })[0])
    const detailDrawer = document.querySelector<HTMLElement>('.weekly-report-record-drawer')
    expect(detailDrawer).toBeInTheDocument()
    const detailTable = within(detailDrawer as HTMLElement)
    expect(detailTable.getByRole('columnheader', { name: '基本信息' })).toBeVisible()
    for (const column of ['姓名', '学号', '班级', '年级', '检查时间', '课程与地点', '到课情况']) {
      expect(detailTable.getByRole('columnheader', { name: column })).toBeVisible()
    }
    const row = detailTable.getAllByRole('row').find(candidate => candidate.textContent?.includes('测试学生'))
    expect(row).toBeDefined()
    if (!row) throw new Error('表格中没有测试学生记录')
    for (const value of ['测试学生', '20260001', '测试班级', '2026-03-02 08:00', '课程', '高等数学', '教室', '教学楼 101', '发生节次', '1-2', '缺课节次数', '2 节次', '违纪计入次数', '2 次']) {
      expect(within(row).getByText(value, { exact: true })).toBeVisible()
    }
    expect(detailTable.queryByRole('button', { name: 'Expand row' })).not.toBeInTheDocument()
    expect(detailTable.queryByText('记录方式')).not.toBeInTheDocument()
    expect(detailTable.queryByText('其他核验信息')).not.toBeInTheDocument()
  })

  it('在草稿明细中按学号、姓名、班级和年级查询服务器分页快照', async () => {
    const attendanceWorkflow = structuredClone(workflow)
    attendanceWorkflow.versions[0].content.rows[0].inspectionType = '到课率'
    mocks.workflow.mockReturnValue({ data: attendanceWorkflow, isError: false, isPending: false })
    mocks.records.mockReturnValue({ data: { items: [{ ...record, typeCode: 'ATTENDANCE_RATE' }], total: 1, filterOptions }, isError: false, isPending: false, isFetching: false })
    render(<WeeklyReportWorkflowPanel period={period} summaryRows={[summaryRow]} permissions={[]} onAccessDenied={vi.fn()} stage="draft" />)
    fireEvent.click(screen.getAllByRole('button', { name: '查看明细' })[0])

    const filterForm = await screen.findByRole('form', { name: '筛选本版记录' })
    for (const [label, value] of [['学号', '20260001'], ['姓名', '测试学生'], ['班级', '测试班级'], ['年级', '2026级']]) {
      fireEvent.mouseDown(within(filterForm).getByRole('combobox', { name: label }))
      fireEvent.click(screen.getByText(value, { selector: '.ant-select-item-option-content' }))
    }
    fireEvent.click(within(filterForm).getByRole('button', { name: /筛\s*选/ }))

    await waitFor(() => expect(mocks.records).toHaveBeenLastCalledWith(expect.objectContaining({
      semesterId: period.semesterId, weekId: period.weekId, versionNo: 1,
      inspectionTypeId: 'type-1',
      studentNumber: '20260001', studentName: '测试学生', className: '测试班级', grade: '2026级', page: 1,
    }), true, expect.any(Function)))
  })

  it('修正生成新版本后清除旧版本筛选，仍能看到修正后的记录', async () => {
    const lateReturnWorkflow = structuredClone(workflow)
    lateReturnWorkflow.versions[0].content.rows[0].inspectionType = '晚归'
    let activeWorkflow = lateReturnWorkflow
    mocks.workflow.mockImplementation(() => ({ data: activeWorkflow, isError: false, isPending: false }))
    mocks.records.mockReturnValue({ data: { items: [{ ...record, typeCode: 'LATE_RETURN' }], total: 1, filterOptions }, isError: false, isFetching: false, isPending: false })
    mocks.correct.mockImplementation((_input: unknown, options: { onSuccess: () => void }) => {
      activeWorkflow = {
        ...lateReturnWorkflow,
        report: { id: 'report-1', semesterId: period.semesterId, weekId: period.weekId, status: 'DRAFT', versionNo: 2, rowVersion: 2, updatedAt: '2026-03-02' },
        versions: [...lateReturnWorkflow.versions, { ...lateReturnWorkflow.versions[0], id: 'version-2', versionNo: 2, createdAt: '2026-03-02' }],
      }
      options.onSuccess()
    })
    render(<WeeklyReportWorkflowPanel period={period} summaryRows={[summaryRow]} permissions={[{ action: 'REPORT_CORRECT', effect: 'ALLOW', scope: {}, sources: [] }]} onAccessDenied={vi.fn()} stage="draft" />)
    fireEvent.click(screen.getAllByRole('button', { name: '查看明细' })[0])

    const filterForm = await screen.findByRole('form', { name: '筛选本版记录' })
    fireEvent.mouseDown(within(filterForm).getByRole('combobox', { name: '姓名' }))
    fireEvent.click(screen.getByText('测试学生', { selector: '.ant-select-item-option-content' }))
    fireEvent.click(within(filterForm).getByRole('button', { name: /筛\s*选/ }))
    await waitFor(() => expect(mocks.records).toHaveBeenLastCalledWith(expect.objectContaining({ studentName: '测试学生', versionNo: 1 }), true, expect.any(Function)))

    fireEvent.click(await screen.findByRole('button', { name: '修正' }))
    fireEvent.change(screen.getByRole('textbox', { name: '宿舍' }), { target: { value: '2舍 301' } })
    fireEvent.change(screen.getByRole('textbox', { name: '处理说明' }), { target: { value: '现场复核后修正记录' } })
    fireEvent.click(screen.getByRole('button', { name: '保存修改并生成新版本' }))

    await waitFor(() => expect(mocks.records).toHaveBeenLastCalledWith(expect.objectContaining({ versionNo: 2, page: 1 }), true, expect.any(Function)))
    const latestQuery = mocks.records.mock.lastCall?.[0]
    for (const filter of ['studentName', 'studentNumber', 'className', 'grade']) expect(latestQuery).not.toHaveProperty(filter)
  })

  it('未生成草稿时只在底栏提供生成入口', () => {
    mocks.workflow.mockReturnValue({ data: { report: null, versions: [], reviews: [], corrections: [] }, isError: false, isPending: false })
    mocks.records.mockReturnValue({ isError: false, isPending: false, isFetching: false })
    render(<WeeklyReportWorkflowPanel period={period} summaryRows={[summaryRow]} permissions={[{ action: 'REPORT_GENERATE', effect: 'ALLOW', scope: {}, sources: [] }]} onAccessDenied={vi.fn()} stage="source" />)

    const actions = screen.getByRole('region', { name: '当前阶段操作' })
    expect(within(actions).getByRole('button', { name: '确认数据并生成草稿' })).toBeVisible()
    expect(screen.getAllByRole('button', { name: '确认数据并生成草稿' })).toHaveLength(1)
  })

  it('来源阶段只在底栏提供继续编制入口', () => {
    mocks.workflow.mockReturnValue({ data: workflow, isError: false, isPending: false })
    mocks.records.mockReturnValue({ isError: false, isPending: false, isFetching: false })
    const onStageChange = vi.fn()
    render(<WeeklyReportWorkflowPanel period={period} summaryRows={[summaryRow]} permissions={[]} onAccessDenied={vi.fn()} stage="source" onStageChange={onStageChange} />)

    const actions = screen.getByRole('region', { name: '当前阶段操作' })
    expect(screen.getAllByRole('button', { name: '继续编制草稿' })).toHaveLength(1)
    fireEvent.click(within(actions).getByRole('button', { name: '继续编制草稿' }))
    expect(onStageChange).toHaveBeenCalledWith(1)
  })

  it('草稿阶段只显示一次当前版本，版本历史按需展开', () => {
    mocks.workflow.mockReturnValue({ data: workflow, isError: false, isPending: false })
    mocks.records.mockReturnValue({ isError: false, isPending: false, isFetching: false })
    render(<WeeklyReportWorkflowPanel period={period} summaryRows={[summaryRow]} permissions={[]} onAccessDenied={vi.fn()} stage="draft" />)

    expect(screen.getByText('草稿 · V1')).toBeVisible()
    expect(screen.getByText('版本与操作记录').closest('details')).not.toHaveAttribute('open')
    expect(screen.getByRole('region', { name: '按检查类型分类' })).toBeVisible()
    fireEvent.click(screen.getByText('版本与操作记录'))
    expect(screen.getByText('版本与操作记录').closest('details')).toHaveAttribute('open')
    expect(screen.getByRole('columnheader', { name: '生成时间' })).toBeVisible()
    expect(screen.getByText('V1')).toBeVisible()
  })

  it('操作记录中的版本链接可查看当时保存的表格快照', async () => {
    const historicalRecord: ReportingRecord = {
      ...record,
      currentDormitoryName: '1舍 203',
      currentAreaName: '宿舍楼',
      details: { date: '2026-03-02', round: '第一轮', result: 'UNQUALIFIED', description: '版本一保存的检查说明', remark: '版本一保存的备注' },
    }
    mocks.workflow.mockReturnValue({ data: { ...workflow, corrections: [{
      id: 'correction-1', versionNo: 1, recordId: historicalRecord.id, field: 'details.description',
      beforeValue: '旧说明', afterValue: '新说明', reason: '现场记录复核后修正', correctedBy: 'office-user', correctedAt: '2026-03-02',
    }] }, isError: false, isPending: false })
    mocks.records.mockReturnValue({ data: { items: [historicalRecord], total: 1, filterOptions }, isError: false, isPending: false, isFetching: false })
    render(<WeeklyReportWorkflowPanel period={period} summaryRows={[summaryRow]} permissions={[]} onAccessDenied={vi.fn()} stage="draft" />)

    fireEvent.click(screen.getByText('版本与操作记录'))
    fireEvent.click(screen.getByRole('button', { name: '查看 V1 数据明细' }))
    const snapshotRegion = screen.getByRole('region', { name: 'V1 数据明细' })
    const snapshotRow = within(snapshotRegion).getAllByRole('row').find(candidate => candidate.textContent?.includes('1舍 203'))
    expect(snapshotRow).toBeDefined()
    if (!snapshotRow) throw new Error('V1 快照中没有宿舍卫生记录')
    expect(await within(snapshotRow).findByText('2026-03-02')).toBeVisible()
    expect(within(snapshotRegion).getByRole('columnheader', { name: '寝室' })).toBeVisible()
    for (const value of ['1舍 203', '轮次', '第一轮', '不合格']) {
      expect(within(snapshotRow).getByText(value, { exact: true })).toBeVisible()
    }
    for (const irrelevantValue of ['课程', '教室', '区域', '宿舍楼']) {
      expect(within(snapshotRow).queryByText(irrelevantValue, { exact: true })).not.toBeInTheDocument()
    }
    expect(within(snapshotRegion).queryByRole('button', { name: 'Expand row' })).not.toBeInTheDocument()
    expect(within(snapshotRegion).getByText(/版本一保存的检查说明/)).toBeVisible()
    expect(within(snapshotRegion).getByText('版本一保存的备注')).toBeVisible()
  })

  it('定稿核对保留分类明细，审核和退回集中在底栏', () => {
    mocks.workflow.mockReturnValue({ data: { ...workflow, report: { ...workflow.report, status: 'PENDING_REVIEW' } }, isError: false, isPending: false })
    mocks.records.mockReturnValue({ isError: false, isPending: false, isFetching: false })
    render(<WeeklyReportWorkflowPanel period={period} summaryRows={[summaryRow]} permissions={['REPORT_REVIEW', 'REPORT_RETURN'].map(action => ({ action, effect: 'ALLOW', scope: {}, sources: [] }))} onAccessDenied={vi.fn()} stage="final" />)

    expect(screen.getByRole('region', { name: '按检查类型分类' })).toBeVisible()
    const actions = screen.getByRole('region', { name: '当前阶段操作' })
    expect(within(actions).getByRole('button', { name: '审核通过' })).toBeVisible()
    expect(within(actions).getByRole('button', { name: '退回草稿' })).toBeVisible()
    expect(screen.getAllByRole('button', { name: '审核通过' })).toHaveLength(1)
  })

  it('审核通过后只在底栏提供确认定稿入口', () => {
    mocks.workflow.mockReturnValue({ data: { ...workflow, report: { ...workflow.report, status: 'FINALIZING' } }, isError: false, isPending: false })
    mocks.records.mockReturnValue({ isError: false, isPending: false, isFetching: false })
    render(<WeeklyReportWorkflowPanel period={period} summaryRows={[summaryRow]} permissions={[{ action: 'REPORT_FINALIZE', effect: 'ALLOW', scope: {}, sources: [] }]} onAccessDenied={vi.fn()} stage="final" />)

    const actions = screen.getByRole('region', { name: '当前阶段操作' })
    expect(within(actions).getByRole('button', { name: '确认定稿数据' })).toBeVisible()
    expect(screen.getAllByRole('button', { name: '确认定稿数据' })).toHaveLength(1)
  })

  it('已定稿的状态只显示一次，底栏保留输出入口', () => {
    mocks.workflow.mockReturnValue({ data: { ...workflow, report: { ...workflow.report, status: 'FINALIZED' } }, isError: false, isPending: false })
    mocks.records.mockReturnValue({ isError: false, isPending: false, isFetching: false })
    render(<WeeklyReportWorkflowPanel period={period} summaryRows={[summaryRow]} permissions={[]} onAccessDenied={vi.fn()} stage="final" />)

    expect(screen.getByText('已定稿 · V1')).toBeVisible()
    const actions = screen.getByRole('region', { name: '当前阶段操作' })
    expect(within(actions).queryByText('已定稿 · 只读')).not.toBeInTheDocument()
    expect(within(actions).getByRole('button', { name: /Word/ })).toBeDisabled()
    expect(within(actions).getByRole('button', { name: /Excel/ })).toBeDisabled()
    expect(screen.getByRole('region', { name: '按检查类型分类' })).toBeVisible()
  })

  it('退回草稿时直接显示最近退回原因，保留展开的历史记录', () => {
    mocks.workflow.mockReturnValue({ data: { ...workflow, report: { ...workflow.report, status: 'RETURNED' }, reviews: [
      { id: 'review-1', versionNo: 1, action: 'RETURNED', comment: '请核对旧数据', actorId: 'actor', createdAt: '2026-03-01' },
      { id: 'review-2', versionNo: 1, action: 'RETURNED', comment: '请补充本周记录', actorId: 'actor', createdAt: '2026-03-02' },
    ] }, isError: false, isPending: false })
    mocks.records.mockReturnValue({ isError: false, isPending: false, isFetching: false })
    render(<WeeklyReportWorkflowPanel period={period} summaryRows={[summaryRow]} permissions={[]} onAccessDenied={vi.fn()} stage="draft" />)

    expect(screen.getByText('退回原因：请补充本周记录')).toBeVisible()
    expect(screen.getByText('版本与操作记录').closest('details')).not.toHaveAttribute('open')
    fireEvent.click(screen.getByText('版本与操作记录'))
    expect(screen.getByText(/请核对旧数据/)).toBeVisible()
  })

  it('按类型展示整条记录并一次提交多个直接编辑字段', async () => {
    mocks.workflow.mockReturnValue({ data: workflow, isError: false, isPending: false })
    mocks.records.mockReturnValue({ data: { items: [record], total: 1 }, isError: false, isPending: false, isFetching: false })
    mocks.correct.mockReset()
    render(<StrictMode><WeeklyReportWorkflowPanel period={period} summaryRows={[summaryRow]} permissions={[{ action: 'REPORT_CORRECT', effect: 'ALLOW', scope: {}, sources: [] }]} onAccessDenied={vi.fn()} /></StrictMode>)

    fireEvent.click(screen.getAllByRole('button', { name: '查看明细' })[0])
    fireEvent.click(await screen.findByRole('button', { name: '修正' }))
    const correctionDialog = document.querySelector<HTMLElement>('.report-correction-modal')
    expect(correctionDialog).toBeInTheDocument()
    const snapshotCard = correctionDialog?.querySelector('.report-correction-snapshot-card')
    expect(snapshotCard).toHaveClass('ant-card')
    expect(snapshotCard?.querySelector('.ant-descriptions')).toBeInTheDocument()
    expect(snapshotCard?.querySelector('.ant-table')).toBeNull()
    expect(snapshotCard).toHaveTextContent('当前版本记录')
    expect(correctionDialog?.querySelector('.ant-form')).toBeInTheDocument()
    expect(correctionDialog?.querySelectorAll('.ant-input').length).toBeGreaterThan(0)
    expect(correctionDialog?.querySelector('.ant-select')).toBeInTheDocument()
    expect(correctionDialog?.querySelector('input[type="date"], input[type="time"], input[type="number"], select')).toBeNull()
    fireEvent.mouseDown(screen.getByRole('combobox', { name: '检查结果' }))
    fireEvent.click(screen.getByText('合格', { selector: '.ant-select-item-option-content' }))
    fireEvent.change(screen.getByRole('textbox', { name: '情况说明' }), { target: { value: '已根据现场照片复核' } })
    fireEvent.change(screen.getByRole('textbox', { name: '处理说明' }), { target: { value: '根据现场照片核对后修正检查信息' } })
    expect(screen.getByRole('button', { name: '保存修改并生成新版本' })).toBeEnabled()
    fireEvent.click(screen.getByRole('button', { name: '保存修改并生成新版本' }))

    await waitFor(() => expect(mocks.correct).toHaveBeenCalledWith(expect.objectContaining({
      reportId: 'report-1', body: expect.objectContaining({ changes: expect.arrayContaining([
        { field: 'details.result', afterValue: 'QUALIFIED' },
        { field: 'details.description', afterValue: '已根据现场照片复核' },
      ]) }),
    }), expect.any(Object)))
  })

  it('办公室草稿无需来源部门确认即可提交', () => {
    mocks.workflow.mockReturnValue({ data: workflow, isError: false, isPending: false })
    mocks.records.mockReturnValue({ isError: false, isPending: false, isFetching: false })
    render(<WeeklyReportWorkflowPanel period={period} summaryRows={[summaryRow]} permissions={[{ action: 'REPORT_GENERATE', effect: 'ALLOW', scope: {}, sources: [] }]} onAccessDenied={vi.fn()} stage="draft" />)

    expect(screen.queryByText(/等待部门重新确认/)).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '草稿无误，进入定稿确认' })).toBeEnabled()
  })
})

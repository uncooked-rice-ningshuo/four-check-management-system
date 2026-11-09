import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ReportVersion } from '../../../entities/reporting/model/reporting'
import { reportOutputApi } from '../../../shared/api/reportOutputApi'
import { ReportOutputPanel } from './ReportOutputPanel'

vi.mock('../../../shared/api/reportOutputApi', () => ({ reportOutputApi: {
  settings: vi.fn().mockResolvedValue({ introduction: '本周检查情况说明', defaultTypeCodes: ['ATTENDANCE_RATE'], version: 1 }),
  generate: vi.fn().mockResolvedValue(new Blob(['test'])),
} }))

const version: ReportVersion = { id: 'v1', versionNo: 1, createdBy: 'actor', createdAt: '2026-01-01', content: {
  period: { academicYearId: 'year', academicYearName: '学年', semesterId: 'semester', semesterName: '学期', weekId: 'week', weekNo: 1, startDate: '2026-01-01', endDate: '2026-01-07' },
  scopeLimited: false, statisticsRuleVersion: 'WEEKLY_INDICATORS_V2', confirmedRecordCount: 0,
  rows: [{ departmentId: 'department', departmentName: '学习部', collegeLevel: false, inspectionTypeId: 'type', inspectionTypeCode: 'ATTENDANCE_RATE', inspectionType: '到课率', recordCount: 0, confirmedZeroReceiptCount: 1, dataStatus: 'CONFIRMED_ZERO_RECORD_RECEIPT' }],
  incompleteCells: [], corrections: [], generatedAt: '2026-01-08',
} }

function renderActions(canGenerate = true, reportVersion = version) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(<QueryClientProvider client={client}><ReportOutputPanel version={reportVersion} semesterId="semester" weekId="week" canGenerate={canGenerate} /></QueryClientProvider>)
}

describe('ReportOutputPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    Object.defineProperty(URL, 'createObjectURL', { value: vi.fn(() => 'blob:report-output'), configurable: true })
    Object.defineProperty(URL, 'revokeObjectURL', { value: vi.fn(), configurable: true })
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
  })

  it('generates and downloads Word and Excel without showing stored file history', async () => {
    renderActions()
    expect(screen.queryByText('V1 · Word/Excel 文件')).not.toBeInTheDocument()
    expect(reportOutputApi.settings).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: '生成 Word' }))
    const wordDialog = await screen.findByRole('dialog', { name: 'V1 · Word 文件' })
    await waitFor(() => expect(reportOutputApi.settings).toHaveBeenCalled())
    expect(within(wordDialog).queryByText('本周检查情况说明')).not.toBeInTheDocument()
    expect(within(wordDialog).queryByText('已生成文件')).not.toBeInTheDocument()
    fireEvent.click(within(wordDialog).getByRole('button', { name: '生成并下载 Word' }))
    await waitFor(() => expect(reportOutputApi.generate).toHaveBeenCalledWith('semester', 'week', 1, { format: 'WORD', typeCodes: ['ATTENDANCE_RATE'], combined: true, settingsVersion: 1 }))
    await waitFor(() => expect(HTMLAnchorElement.prototype.click).toHaveBeenCalled())
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'V1 · Word 文件' })).not.toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: '生成 Excel' }))
    const excelDialog = await screen.findByRole('dialog', { name: 'V1 · Excel 文件' })
    expect(within(excelDialog).queryByText('本周检查情况说明')).not.toBeInTheDocument()
    fireEvent.click(within(excelDialog).getByRole('button', { name: '生成并下载 Excel' }))
    await waitFor(() => expect(reportOutputApi.generate).toHaveBeenCalledWith('semester', 'week', 1, { format: 'EXCEL', typeCodes: ['ATTENDANCE_RATE'], combined: true, settingsVersion: 1 }))
  })

  it('does not offer a stored-file entry to read only viewers', () => {
    renderActions(false)
    expect(screen.queryByRole('button', { name: /Word|Excel/ })).not.toBeInTheDocument()
  })

  it('allows Word and Excel generation for an unsubmitted empty dimension', async () => {
    const incomplete: ReportVersion = { ...version, content: { ...version.content, rows: version.content.rows.map(row => ({ ...row, confirmedZeroReceiptCount: 0, dataStatus: 'NO_CONFIRMED_DATA' })) } }
    renderActions(true, incomplete)
    for (const format of ['Word', 'Excel']) {
      fireEvent.click(screen.getByRole('button', { name: `生成 ${format}` }))
      const dialog = await screen.findByRole('dialog', { name: `V1 · ${format} 文件` })
      await waitFor(() => expect(within(dialog).getByRole('button', { name: `生成并下载 ${format}` })).toBeEnabled())
      fireEvent.click(within(dialog).getByRole('button', { name: `生成并下载 ${format}` }))
      await waitFor(() => expect(reportOutputApi.generate).toHaveBeenCalledWith('semester', 'week', 1, { format: format.toUpperCase(), typeCodes: ['ATTENDANCE_RATE'], combined: true, settingsVersion: 1 }))
      await waitFor(() => expect(screen.queryByRole('dialog', { name: `V1 · ${format} 文件` })).not.toBeInTheDocument())
    }
  })

  it('uses the configured weekly date without a generation-time input', async () => {
    vi.mocked(reportOutputApi.settings).mockResolvedValueOnce({
      introduction: '', wordTypeCodes: ['ATTENDANCE_RATE'], excelTypeCodes: ['ATTENDANCE_RATE'], version: 1,
      introductionDocument: { type: 'doc', content: [{ type: 'paragraph', attrs: { style: 'date', textAlign: 'right' }, content: [{ type: 'text', text: '{reportDate}' }] }] },
    })
    renderActions()
    fireEvent.click(screen.getByRole('button', { name: '生成 Word' }))
    const dialog = await screen.findByRole('dialog', { name: 'V1 · Word 文件' })
    expect(within(dialog).queryByLabelText('文件日期')).not.toBeInTheDocument()
    await waitFor(() => expect(within(dialog).getByRole('button', { name: '生成并下载 Word' })).toBeEnabled())
    fireEvent.click(within(dialog).getByRole('button', { name: '生成并下载 Word' }))
    await waitFor(() => expect(reportOutputApi.generate).toHaveBeenCalledWith('semester', 'week', 1, { format: 'WORD', typeCodes: ['ATTENDANCE_RATE'], combined: true, settingsVersion: 1 }))
  })

  it('keeps the selection open when generation fails so it can be retried', async () => {
    vi.mocked(reportOutputApi.generate).mockRejectedValueOnce(new Error('生成失败，请重试'))
    renderActions()
    fireEvent.click(screen.getByRole('button', { name: '生成 Excel' }))
    const dialog = await screen.findByRole('dialog', { name: 'V1 · Excel 文件' })
    await waitFor(() => expect(within(dialog).getByRole('button', { name: '生成并下载 Excel' })).toBeEnabled())
    fireEvent.click(within(dialog).getByRole('button', { name: '生成并下载 Excel' }))
    await waitFor(() => expect(reportOutputApi.generate).toHaveBeenCalledOnce())
    expect(within(dialog).getByRole('button', { name: '生成并下载 Excel' })).toBeVisible()
    expect(HTMLAnchorElement.prototype.click).not.toHaveBeenCalled()
  })
})

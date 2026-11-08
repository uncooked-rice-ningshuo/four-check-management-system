import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ReportVersion } from '../../../entities/reporting/model/reporting'
import { reportOutputApi } from '../../../shared/api/reportOutputApi'
import { ReportOutputPanel } from './ReportOutputPanel'

vi.mock('../../../shared/api/reportOutputApi', () => ({ reportOutputApi: {
  settings: vi.fn().mockResolvedValue({ introduction: '本周检查情况说明', defaultTypeCodes: ['ATTENDANCE_RATE'], version: 1 }),
  list: vi.fn().mockResolvedValue({ items: [{ id: 'failed', reportVersionId: 'v1', format: 'WORD', typeCodes: ['ATTENDANCE_RATE'], combined: false, status: 'FAILED', createdAt: '2026-01-01' }] }),
  generate: vi.fn().mockResolvedValue({ id: 'new', reportVersionId: 'v1', format: 'WORD', typeCodes: ['ATTENDANCE_RATE'], combined: true, status: 'AVAILABLE', createdAt: '2026-01-01' }),
  download: vi.fn(),
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
  beforeEach(() => { vi.clearAllMocks() })

  it('opens separate Word and Excel dialogs with the configured selection and hides failed downloads', async () => {
    renderActions()
    expect(screen.queryByText('V1 · Word/Excel 文件')).not.toBeInTheDocument()
    expect(reportOutputApi.settings).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: '生成 Word' }))
    const wordDialog = await screen.findByRole('dialog', { name: 'V1 · Word 文件' })
    await waitFor(() => expect(reportOutputApi.settings).toHaveBeenCalled())
    expect(within(wordDialog).queryByText('本周检查情况说明')).not.toBeInTheDocument()
    await waitFor(() => expect(within(wordDialog).getByText('生成失败')).toBeVisible())
    expect(within(wordDialog).queryByRole('button', { name: '下载 Word 文件' })).not.toBeInTheDocument()
    fireEvent.click(within(wordDialog).getByRole('button', { name: '生成 Word' }))
    await waitFor(() => expect(reportOutputApi.generate).toHaveBeenCalledWith('semester', 'week', 1, { format: 'WORD', typeCodes: ['ATTENDANCE_RATE'], combined: true, settingsVersion: 1 }))
    fireEvent.click(within(wordDialog).getByRole('button', { name: /关\s*闭/ }))
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'V1 · Word 文件' })).not.toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: '生成 Excel' }))
    const excelDialog = await screen.findByRole('dialog', { name: 'V1 · Excel 文件' })
    expect(within(excelDialog).queryByText('本周检查情况说明')).not.toBeInTheDocument()
    fireEvent.click(within(excelDialog).getByRole('button', { name: '生成 Excel' }))
    await waitFor(() => expect(reportOutputApi.generate).toHaveBeenCalledWith('semester', 'week', 1, { format: 'EXCEL', typeCodes: ['ATTENDANCE_RATE'], combined: true, settingsVersion: 1 }))
  })

  it('downloads available files from a format dialog without offering generation to read only viewers', async () => {
    vi.mocked(reportOutputApi.list).mockResolvedValueOnce({ items: [{ id: 'available', reportVersionId: 'v1', format: 'EXCEL', typeCodes: ['ATTENDANCE_RATE'], combined: false, status: 'AVAILABLE', createdAt: '2026-01-01' }] })
    vi.mocked(reportOutputApi.download).mockResolvedValueOnce({ blob: new Blob(['test']), headers: new Headers() })
    Object.defineProperty(URL, 'createObjectURL', { value: vi.fn(() => 'blob:report-output'), configurable: true })
    Object.defineProperty(URL, 'revokeObjectURL', { value: vi.fn(), configurable: true })
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    renderActions(false)
    fireEvent.click(screen.getByRole('button', { name: '查看 Excel 文件' }))
    const dialog = await screen.findByRole('dialog', { name: 'V1 · Excel 文件' })
    fireEvent.click(await within(dialog).findByRole('button', { name: '下载 Excel 文件' }))
    await waitFor(() => expect(reportOutputApi.download).toHaveBeenCalledWith('semester', 'week', 1, 'available'))
    await waitFor(() => expect(click).toHaveBeenCalledOnce())
    expect(within(dialog).queryByRole('button', { name: '生成 Excel' })).not.toBeInTheDocument()
    click.mockRestore()
  })

  it('allows Word and Excel generation for an unsubmitted empty dimension', async () => {
    const incomplete: ReportVersion = { ...version, content: { ...version.content, rows: version.content.rows.map(row => ({ ...row, confirmedZeroReceiptCount: 0, dataStatus: 'NO_CONFIRMED_DATA' })) } }
    renderActions(true, incomplete)
    for (const format of ['Word', 'Excel']) {
      fireEvent.click(screen.getByRole('button', { name: `生成 ${format}` }))
      const dialog = await screen.findByRole('dialog', { name: `V1 · ${format} 文件` })
      await waitFor(() => expect(within(dialog).getByRole('button', { name: `生成 ${format}` })).toBeEnabled())
      fireEvent.click(within(dialog).getByRole('button', { name: `生成 ${format}` }))
      await waitFor(() => expect(reportOutputApi.generate).toHaveBeenCalledWith('semester', 'week', 1, { format: format.toUpperCase(), typeCodes: ['ATTENDANCE_RATE'], combined: true, settingsVersion: 1 }))
      fireEvent.click(within(dialog).getByRole('button', { name: /关\s*闭/ }))
      await waitFor(() => expect(screen.queryByRole('dialog', { name: `V1 · ${format} 文件` })).not.toBeInTheDocument())
    }
  })
})

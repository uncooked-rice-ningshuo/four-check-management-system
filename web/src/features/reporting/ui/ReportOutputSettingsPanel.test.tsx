import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { reportOutputApi } from '../../../shared/api/reportOutputApi'
import { ReportOutputSettingsPanel } from './ReportOutputSettingsPanel'

const savedSettings = {
  introduction: '原说明',
  introductionDocument: { type: 'doc' as const, content: [{ type: 'paragraph' as const, attrs: { style: 'body', textAlign: 'justify' }, content: [{ type: 'text' as const, text: '原说明' }] }] },
  wordTypeCodes: ['LATE_RETURN'],
  excelTypeCodes: ['ATTENDANCE_RATE'],
  version: 3,
  inspectionTypes: [{ code: 'LATE_RETURN', name: '晚归', title: '晚归检查', columns: [], excelColumns: ['时间'] }, { code: 'ATTENDANCE_RATE', name: '到课率', title: '到课率', columns: ['应到'], excelColumns: ['班级', '本周旷课学时'] }],
}
vi.mock('../../../shared/api/reportOutputApi', () => ({ reportOutputApi: {
  editorSettings: vi.fn(), updateSettings: vi.fn(), previewWord: vi.fn(), previewExcel: vi.fn(),
} }))
function renderSettings() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}><ReportOutputSettingsPanel /></QueryClientProvider>)
}

describe('ReportOutputSettingsPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(reportOutputApi.editorSettings).mockResolvedValue(savedSettings)
    vi.mocked(reportOutputApi.updateSettings).mockResolvedValue({ ...savedSettings, version: 4 })
    Object.defineProperty(Range.prototype, 'getClientRects', { value: () => ({ 0: new DOMRect(), length: 1, item: () => new DOMRect() }), configurable: true })
    Object.defineProperty(Range.prototype, 'getBoundingClientRect', { value: () => new DOMRect(), configurable: true })
  })

  it('keeps Word and Excel selections independent and saves both', async () => {
    renderSettings()
    expect(await screen.findByRole('menuitem', { name: /Word 配置/ })).toBeVisible()
    fireEvent.click(screen.getByText('到课率'))
    fireEvent.click(screen.getByRole('menuitem', { name: /Excel 配置/ }))
    expect(screen.getByText('1. 到课率')).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: '保存配置' }))
    await waitFor(() => expect(reportOutputApi.updateSettings).toHaveBeenCalledWith(expect.objectContaining({
      wordTypeCodes: ['LATE_RETURN', 'ATTENDANCE_RATE'], excelTypeCodes: ['ATTENDANCE_RATE'], version: 3,
    })))
  })

  it('opens the Word introduction editor', async () => {
    renderSettings()
    expect(await screen.findByText('通报首页')).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: '编辑首页' }))
    expect((await screen.findAllByText('原说明'))[0]).toBeVisible()
    expect(document.querySelector('.report-intro-content .tiptap')).toHaveTextContent('第9周日常管理检查通报')
    expect(document.querySelector('.report-intro-content .tiptap')).not.toHaveTextContent('{semesterName}')
    expect(document.querySelector('.report-intro-content .tiptap')).toHaveTextContent('附件1：晚归检查')
    expect(document.querySelector('.report-intro-content .tiptap')).not.toHaveTextContent('附件：1.')
    expect(document.querySelector('.report-intro-content .tiptap p[data-style="body"]')).toHaveAttribute('data-indent', '0')
    expect(document.querySelector('.report-intro-content .tiptap p[data-style="body"]')).toHaveAttribute('data-line-spacing', '25')
    expect(screen.getByRole('button', { name: '插入附件文本' })).toHaveClass('report-intro-tool')
    expect(screen.getByRole('button', { name: '插入变量' })).toHaveClass('report-intro-tool')
    fireEvent.click(screen.getByRole('button', { name: '段落间距' }))
    expect(await screen.findByRole('spinbutton', { name: '段前间距' })).toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: '行距方式' })).toBeInTheDocument()
    fireEvent.change(screen.getByRole('textbox', { name: '编辑页首' }), { target: { value: '学院页首' } })
    expect(screen.getByRole('textbox', { name: '编辑页首' })).toHaveValue('学院页首')
    fireEvent.click(screen.getByRole('button', { name: '撤销' }))
    expect(screen.getByRole('textbox', { name: '编辑页首' })).toHaveValue('')
    fireEvent.click(screen.getByRole('button', { name: '重做' }))
    expect(screen.getByRole('textbox', { name: '编辑页首' })).toHaveValue('学院页首')
    expect(screen.getAllByText('计算机科学与工程学院').length).toBeGreaterThan(0)
    fireEvent.click(screen.getByRole('button', { name: '页码靠右' }))
    fireEvent.click(screen.getByRole('button', { name: '完成编辑' }))
    fireEvent.click(screen.getByRole('button', { name: '保存配置' }))
    await waitFor(() => expect(reportOutputApi.updateSettings).toHaveBeenCalledWith(expect.objectContaining({
      introductionDocument: expect.objectContaining({ headerText: '学院页首', footerText: '\t{page}', footerLayout: 'custom', footerAlign: 'left', content: expect.arrayContaining([expect.objectContaining({ attrs: expect.objectContaining({ style: 'title' }) })]) }),
    })))
  })

  it('previews the selected Word attachments', async () => {
    const view = renderSettings()
    expect(await screen.findByRole('menuitem', { name: /Word 配置/ })).toBeVisible()
    const preview = view.container.querySelector<HTMLButtonElement>('.report-output-settings-header button:first-child')
    expect(preview).not.toBeNull()
    fireEvent.click(preview!)
    expect(await screen.findByLabelText('晚归附件预览')).toBeInTheDocument()
  })

  it('updates only an unchanged legacy attachment list to the reference format', async () => {
    vi.mocked(reportOutputApi.editorSettings).mockResolvedValue({ ...savedSettings, introductionDocument: {
      type: 'doc', manualAttachmentList: true, content: [
        { type: 'paragraph', attrs: { style: 'body', textAlign: 'left' }, content: [{ type: 'text', text: '附件：' }] },
        { type: 'paragraph', attrs: { style: 'body', textAlign: 'left' }, content: [{ type: 'text', text: '1. 晚归检查' }] },
      ],
    } })
    renderSettings()
    fireEvent.click(await screen.findByRole('button', { name: '编辑首页' }))
    expect(document.querySelector('.report-intro-content .tiptap')).toHaveTextContent('附件1：晚归检查')
    expect(document.querySelector('.report-intro-content .tiptap')).not.toHaveTextContent('附件：1.')
  })

  it('shows Excel settings without a Word introduction editor', async () => {
    renderSettings()
    expect(await screen.findByRole('menuitem', { name: /Excel 配置/ })).toBeVisible()
    fireEvent.click(screen.getByRole('menuitem', { name: /Excel 配置/ }))
    expect(screen.getByText('工作表顺序')).toBeVisible()
    expect(screen.queryByRole('button', { name: '编辑首页' })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /预\s*览/ }))
    expect(screen.getByRole('region', { name: '到课率工作表' })).toHaveTextContent('本周旷课学时')
  })

  it('shows selected types as tabs in one Excel workbook', async () => {
    vi.mocked(reportOutputApi.editorSettings).mockResolvedValue({ ...savedSettings, excelTypeCodes: ['ATTENDANCE_RATE', 'LATE_RETURN'] })
    renderSettings()
    fireEvent.click(await screen.findByRole('menuitem', { name: /Excel 配置/ }))
    fireEvent.click(screen.getByRole('button', { name: /预\s*览/ }))
    expect(screen.getByRole('tablist', { name: '工作表' })).toBeVisible()
    expect(screen.getAllByRole('region', { name: /工作表$/ })).toHaveLength(1)
    expect(screen.getByRole('tab', { name: '到课率' })).toHaveAttribute('aria-selected', 'true')
    fireEvent.click(screen.getByRole('tab', { name: '晚归' }))
    expect(screen.getByRole('region', { name: '晚归工作表' })).toHaveTextContent('时间')
    expect(screen.getByRole('tab', { name: '晚归' })).toHaveAttribute('aria-selected', 'true')
  })
})

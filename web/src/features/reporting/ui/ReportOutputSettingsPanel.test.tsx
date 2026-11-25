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
  editorSettings: vi.fn(), updateSettings: vi.fn(), previewWord: vi.fn(), previewExcel: vi.fn(), weekDates: vi.fn(), saveWeekDate: vi.fn(),
} }))
vi.mock('../../work-context/useWorkContext', () => ({ useWorkContext: () => ({ data: { week: { id: 'week-1' } } }) }))
function renderSettings() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}><ReportOutputSettingsPanel /></QueryClientProvider>)
}

describe('ReportOutputSettingsPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(reportOutputApi.editorSettings).mockResolvedValue(savedSettings)
    vi.mocked(reportOutputApi.updateSettings).mockResolvedValue({ ...savedSettings, version: 4 })
    vi.mocked(reportOutputApi.weekDates).mockResolvedValue({ items: [{ semesterId: 'semester-1', semesterName: '2025—2026 学年第二学期', weekId: 'week-1', weekNo: 8, issueDate: '', version: 0 }, { semesterId: 'semester-1', semesterName: '2025—2026 学年第二学期', weekId: 'week-2', weekNo: 9, issueDate: '2026-05-01', version: 2 }] })
    vi.mocked(reportOutputApi.saveWeekDate).mockImplementation(async input => ({ ...input, version: input.version + 1 }))
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

  it('converts the old layout and saves its table, title and section as separate blocks', async () => {
    renderSettings()
    fireEvent.click(await screen.findByRole('button', {name:'编辑模板'}))
    expect(await screen.findByRole('toolbar',{name:'模板编辑工具'})).toBeVisible()
    fireEvent.click(screen.getByRole('button',{name:'保存并完成'}))
    await waitFor(()=>expect(reportOutputApi.updateSettings).toHaveBeenCalledWith(expect.objectContaining({
      introductionDocument:expect.objectContaining({schemaVersion:2,content:expect.arrayContaining([
        {type:'sectionBreak',attrs:{orientation:'landscape'}},
        {type:'reportTable',attrs:{key:'LATE_RETURN'}},
        expect.objectContaining({type:'paragraph',content:[{type:'text',text:'晚归检查'}]}),
      ])}),
    })))
  })

  it('opens the Word introduction editor', async () => {
    renderSettings()
    expect(await screen.findByText('通报模板')).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: '编辑模板' }))
    expect((await screen.findAllByText('原说明'))[0]).toBeVisible()
    const dialog = screen.getByRole('dialog')
    expect(dialog.querySelector('.report-intro-content .tiptap')).toHaveTextContent('第9周日常管理检查通报')
    expect(dialog.querySelector('.report-intro-content .tiptap')).not.toHaveTextContent('{semesterName}')
    expect(dialog.querySelector('.report-intro-content .tiptap')).toHaveTextContent('附件1：晚归检查')
    expect(dialog.querySelector('.report-intro-content .tiptap')).not.toHaveTextContent('附件：1.')
    expect(dialog.querySelector('.report-intro-content .tiptap p[data-style="body"]')).toHaveAttribute('data-indent', '0')
    expect(dialog.querySelector('.report-intro-content .tiptap p[data-style="body"]')).toHaveAttribute('data-line-spacing', '25')
    expect(screen.getByLabelText('插入通报表')).toHaveClass('report-intro-tool')
    expect(screen.getByLabelText('插入字段')).toHaveClass('report-intro-tool')
    fireEvent.click(screen.getByLabelText('段落间距'))
    expect(await screen.findByLabelText('段前间距')).toBeInTheDocument()
    expect(screen.getByLabelText('行距方式')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button',{name:'确定段落设置'}))
    fireEvent.change(screen.getByLabelText('编辑页首'), { target: { value: '学院页首' } })
    expect(screen.getByLabelText('编辑页首')).toHaveValue('学院页首')
    expect(screen.getAllByText('计算机科学与工程学院').length).toBeGreaterThan(0)
    fireEvent.focus(dialog.querySelector('.report-intro-footer-content .tiptap')!)
    expect(screen.getByRole('toolbar', { name: '页脚快捷工具' })).toBeVisible()
    fireEvent.click(screen.getByText('参考页脚版式', {exact:true}))
    fireEvent.click(screen.getByText('保存并完成', {exact:true}))
    await waitFor(() => expect(reportOutputApi.updateSettings).toHaveBeenCalledWith(expect.objectContaining({
      introductionDocument: expect.objectContaining({ headerText: '学院页首', footerText: '{semesterName} 第{weekNo}周\t第 {page} 页 共 {totalPages} 页\t', footerLayout: 'custom', footerAlign: 'left', content: expect.arrayContaining([expect.objectContaining({ attrs: expect.objectContaining({ style: 'title' }) })]) }),
    })))
  })

  it('previews the selected Word attachments', async () => {
    vi.mocked(reportOutputApi.editorSettings).mockResolvedValue({ ...savedSettings, introductionDocument: { type: 'doc', content: [{ type: 'paragraph', attrs: { style: 'date', textAlign: 'right' }, content: [{ type: 'text', text: '{reportDate}' }] }] } })
    renderSettings()
    expect((await screen.findAllByLabelText('模板内容预览')).length).toBeGreaterThan(0)
    expect(screen.queryByRole('combobox', { name: '预览通报周次' })).toBeNull()
    await waitFor(() => expect(screen.getAllByText('2025年6月1日').length).toBeGreaterThan(0))
  })

  it('downloads the Word preview without configured dates', async () => {
    vi.mocked(reportOutputApi.weekDates).mockResolvedValue({ items: [] })
    vi.mocked(reportOutputApi.previewWord).mockResolvedValue(new Blob(['preview']))
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => undefined)
    const createObjectURL = vi.fn(() => 'blob:preview')
    Object.defineProperty(URL, 'createObjectURL', { value: createObjectURL, configurable: true })
    renderSettings()
    fireEvent.click(await screen.findByRole('button', { name: '下载 Word 模板' }))
    await waitFor(() => expect(reportOutputApi.previewWord).toHaveBeenCalledWith(expect.any(Object), ['LATE_RETURN']))
    click.mockRestore()
  })

  it('saves the selected week date separately from template settings', async () => {
    renderSettings()
    expect(await screen.findByRole('menuitem', { name: '文件日期' })).toBeVisible()
    expect(screen.queryByLabelText('文件日期')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('menuitem', { name: '文件日期' }))
    const input = await screen.findByLabelText('文件日期')
    expect(input).toHaveValue('')
    expect(screen.queryByRole('button', { name: '保存配置' })).not.toBeInTheDocument()
    fireEvent.change(input, { target: { value: '2026-05-08' } })
    fireEvent.keyDown(input, { key: 'Enter', code: 'Enter', keyCode: 13 })
    await waitFor(() => expect(screen.getByRole('button', { name: '保存日期' })).toBeEnabled())
    fireEvent.click(screen.getByRole('button', { name: '保存日期' }))
    await waitFor(() => expect(reportOutputApi.saveWeekDate).toHaveBeenCalledWith(expect.objectContaining({ weekId: 'week-1', issueDate: '2026-05-08', version: 0 })))
    expect(reportOutputApi.updateSettings).not.toHaveBeenCalled()
  })

  it('renders a saved file date variable with its own label', async () => {
    vi.mocked(reportOutputApi.editorSettings).mockResolvedValue({ ...savedSettings, introductionDocument: {
      type: 'doc', content: [
        { type: 'paragraph', attrs: { style: 'date', textAlign: 'right' }, content: [{ type: 'text', text: '{reportDate}' }] },
      ],
    } })
    renderSettings()
    fireEvent.click(await screen.findByRole('button', { name: '编辑模板' }))
    const dateVariable = screen.getByRole('dialog').querySelector('.report-intro-content .report-template-variable[data-word-value="report.reportDate"]')
    expect(dateVariable).toHaveTextContent('通报日期')
    expect(dateVariable).toHaveAttribute('title', '通报日期')
  })

  it('saves an inserted weekly file date token in the Word introduction', async () => {
    renderSettings()
    fireEvent.click(await screen.findByRole('button', { name: '编辑模板' }))
    fireEvent.click(screen.getByLabelText('插入字段'))
    fireEvent.click(await screen.findByRole('menuitem', { name: '通报日期' }))
    expect(screen.getByRole('dialog').querySelector('.report-intro-content .report-template-variable[data-word-value="report.reportDate"]')).toBeInTheDocument()
    fireEvent.click(screen.getByText('保存并完成', {exact:true}))
    await waitFor(() => expect(reportOutputApi.updateSettings).toHaveBeenCalledWith(expect.objectContaining({
      introductionDocument: expect.objectContaining({ content: expect.arrayContaining([expect.objectContaining({
        content: expect.arrayContaining([expect.objectContaining({ type:'templateField',attrs:{key:'report.reportDate'} })]),
      })]) }),
    })))
  })

  it('saves Chinese week and page number formats', async () => {
    renderSettings()
    fireEvent.click(await screen.findByRole('button', { name: '编辑模板' }))
    fireEvent.click(screen.getByLabelText('插入字段'))
    fireEvent.click(await screen.findByRole('menuitem', { name: '周次汉字' }))
    expect(screen.getByRole('dialog').querySelector('.report-intro-content .report-template-variable[data-word-value="report.weekNoChinese"]')).toBeInTheDocument()
    fireEvent.focus(screen.getByRole('dialog').querySelector('.report-intro-footer-content .tiptap')!)
    fireEvent.click(screen.getByText('页码样式', {exact:true}))
    fireEvent.click(await screen.findByRole('menuitem', { name: '第一页，共N页（汉字）' }))
    expect(screen.getByRole('dialog').querySelector('.report-intro-footer-content')).toHaveTextContent('总页数（汉字）')
    fireEvent.click(screen.getByText('保存并完成', {exact:true}))
    await waitFor(() => expect(reportOutputApi.updateSettings).toHaveBeenCalledWith(expect.objectContaining({ introductionDocument: expect.objectContaining({ footerText: '第{pageChinese}页，共{totalPagesChinese}页' }) })))
  })

  it('updates only an unchanged legacy attachment list to the reference format', async () => {
    vi.mocked(reportOutputApi.editorSettings).mockResolvedValue({ ...savedSettings, introductionDocument: {
      type: 'doc', manualAttachmentList: true, content: [
        { type: 'paragraph', attrs: { style: 'body', textAlign: 'left' }, content: [{ type: 'text', text: '附件：' }] },
        { type: 'paragraph', attrs: { style: 'body', textAlign: 'left' }, content: [{ type: 'text', text: '1. 晚归检查' }] },
      ],
    } })
    renderSettings()
    fireEvent.click(await screen.findByRole('button', { name: '编辑模板' }))
    expect(screen.getByRole('dialog').querySelector('.report-intro-content .tiptap')).toHaveTextContent('附件1：晚归检查')
    expect(screen.getByRole('dialog').querySelector('.report-intro-content .tiptap')).not.toHaveTextContent('附件：1.')
  })

  it('shows Excel settings without a Word introduction editor', async () => {
    renderSettings()
    expect(await screen.findByRole('menuitem', { name: /Excel 配置/ })).toBeVisible()
    fireEvent.click(screen.getByRole('menuitem', { name: /Excel 配置/ }))
    expect(screen.getByText('工作表顺序')).toBeVisible()
    expect(screen.queryByRole('button', { name: '编辑模板' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /^预\s*览$/ })).toBeNull()
    expect(screen.getByRole('region', { name: '到课率工作表' })).toHaveTextContent('本周旷课学时')
  })

  it('shows selected types as tabs in one Excel workbook', async () => {
    vi.mocked(reportOutputApi.editorSettings).mockResolvedValue({ ...savedSettings, excelTypeCodes: ['ATTENDANCE_RATE', 'LATE_RETURN'] })
    renderSettings()
    fireEvent.click(await screen.findByRole('menuitem', { name: /Excel 配置/ }))
    expect(screen.queryByRole('button', { name: /^预\s*览$/ })).toBeNull()
    expect(screen.getByRole('tablist', { name: '工作表' })).toBeVisible()
    expect(screen.getAllByRole('region', { name: /工作表$/ })).toHaveLength(1)
    expect(screen.getByRole('tab', { name: '到课率' })).toHaveAttribute('aria-selected', 'true')
    fireEvent.click(screen.getByRole('button', { name: '上移 晚归' }))
    expect(screen.getAllByRole('tab')[0]).toHaveTextContent('晚归')
    fireEvent.click(screen.getByRole('tab', { name: '晚归' }))
    expect(screen.getByRole('region', { name: '晚归工作表' })).toHaveTextContent('时间')
    expect(screen.getByRole('tab', { name: '晚归' })).toHaveAttribute('aria-selected', 'true')
  })
})

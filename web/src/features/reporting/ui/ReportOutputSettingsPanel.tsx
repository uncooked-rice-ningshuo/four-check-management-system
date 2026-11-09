import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowDownOutlined, ArrowUpOutlined } from '@ant-design/icons'
import { Alert, Button, Card, Checkbox, DatePicker, Drawer, Menu, message, Select, Space, Typography } from 'antd'
import dayjs from 'dayjs'
import { useState } from 'react'
import { reportOutputApi, type IntroductionDocument, type ReportOutputEditorSettings, type ReportOutputSettings, type ReportOutputWeekDate } from '../../../shared/api/reportOutputApi'
import { useWorkContext } from '../../work-context/useWorkContext'
import { ReportIntroductionEditor } from './ReportIntroductionEditor'
import { ReportDocumentPreview } from './ReportDocumentPreview'

function attachmentLines(settings: ReportOutputEditorSettings, codes: string[]) {
  const names = codes.map(code => settings.inspectionTypes.find(type => type.code === code)?.title || settings.inspectionTypes.find(type => type.code === code)?.name || code)
  return names.map((name, index) => `附件${index + 1}：${name}`)
}

function oldAttachmentLines(settings: ReportOutputEditorSettings, codes: string[]) {
  const names = codes.map(code => settings.inspectionTypes.find(type => type.code === code)?.title || settings.inspectionTypes.find(type => type.code === code)?.name || code)
  return ['附件：', ...names.map((name, index) => `${index + 1}. ${name}`)]
}

function attachmentParagraphs(lines: string[]): IntroductionDocument['content'] {
  return lines.map(text => ({ type: 'paragraph', attrs: { style: 'body', textAlign: 'left', indent: 2, lineSpacing: 27 }, content: [{ type: 'text', text }] }))
}

function syncGeneratedAttachments(document: IntroductionDocument, previous: string[], next: string[]): IntroductionDocument {
  const content = [...document.content]
  const text = (index: number) => content[index]?.content?.map(run => run.text).join('')
  const start = content.findIndex((_, index) => previous.every((line, offset) => text(index + offset) === line))
  if (start < 0) return document
  content.splice(start, previous.length, ...attachmentParagraphs(next))
  return { ...document, content }
}

function initialDocument(settings: ReportOutputEditorSettings): IntroductionDocument {
  let document: IntroductionDocument = settings.introductionDocument ?? { type: 'doc', content: settings.introduction.split('\n').map(line => ({
    type: 'paragraph', attrs: { style: 'body', textAlign: 'left', indent: 2, lineSpacing: 27 },
    content: line ? [{ type: 'text', text: line }] : [],
  })) }
  if (!document.content.some(paragraph => paragraph.attrs.style === 'title' || paragraph.attrs.style === 'subtitle')) {
    const title = document.titleTemplate || '第{weekNo}周日常管理检查通报'
    document = { ...document, content: [
      { type: 'paragraph', attrs: { style: 'title', textAlign: 'center' }, content: [{ type: 'text', text: '计算机科学与工程学院' }] },
      { type: 'paragraph', attrs: { style: 'subtitle', textAlign: 'center', spaceBefore: 8 }, content: [{ type: 'text', text: title }] },
      ...document.content,
    ], attachmentPosition: document.attachmentPosition === undefined ? undefined : document.attachmentPosition + 2 }
  }
  let firstBody = true
  document = { ...document, content: document.content.map(paragraph => {
    if (paragraph.attrs.style !== 'body') return paragraph
    const attrs = { ...paragraph.attrs, lineSpacing: paragraph.attrs.lineSpacing ?? (firstBody ? 25 : 27), spaceBefore: paragraph.attrs.spaceBefore ?? (firstBody ? 16 : 0) }
    firstBody = false
    return { ...paragraph, attrs }
  }) }
  if (document.manualAttachmentList) return syncGeneratedAttachments(document, oldAttachmentLines(settings, settings.wordTypeCodes ?? []), attachmentLines(settings, settings.wordTypeCodes ?? []))
  const attachmentText = attachmentParagraphs(attachmentLines(settings, settings.wordTypeCodes ?? []))
  const content = [...document.content]
  content.splice(document.attachmentPosition ?? content.length, 0, ...attachmentText)
  return { ...document, content, attachmentPosition: undefined, manualAttachmentList: true }
}

type Format = 'WORD' | 'EXCEL' | 'DATE'

function WeekDateEditor({ week }: { week: ReportOutputWeekDate }) {
  const client = useQueryClient()
  const [messageApi, contextHolder] = message.useMessage()
  const [issueDate, setIssueDate] = useState(week.issueDate)
  const save = useMutation({
    mutationFn: () => reportOutputApi.saveWeekDate({ ...week, issueDate }),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ['report-output-week-dates'] })
      void messageApi.success('本周文件日期已保存')
    },
    onError: error => { void messageApi.error(error.message) },
  })
  return <div className="report-output-week-date-form">{contextHolder}
    <DatePicker aria-label="文件日期" format="YYYY-MM-DD" placeholder="选择文件日期" value={issueDate ? dayjs(issueDate) : null} onChange={date => setIssueDate(date?.format('YYYY-MM-DD') ?? '')} />
    <Button onClick={() => save.mutate()} loading={save.isPending} disabled={!issueDate || issueDate === week.issueDate}>保存日期</Button>
  </div>
}

function WeekDateSettings() {
  const dates = useQuery({ queryKey: ['report-output-week-dates'], queryFn: reportOutputApi.weekDates })
  const workContext = useWorkContext()
  const [selectedWeekId, setSelectedWeekId] = useState<string>()
  const currentWeekId = workContext.data?.week?.id
  const effectiveWeekId = selectedWeekId ?? (dates.data?.items.some(item => item.weekId === currentWeekId) ? currentWeekId : undefined)
  const selected = dates.data?.items.find(item => item.weekId === effectiveWeekId)
  return <section className="report-output-config-section" aria-label="每周文件日期">
    <div className="report-output-config-section-title"><strong>通报周次</strong></div>
    <Typography.Paragraph type="secondary">选择通报周次并设置文件落款日期。Word 生成时使用这里保存的日期。</Typography.Paragraph>
    {dates.isError && <Alert type="error" showIcon title="周次日期加载失败" action={<Button onClick={() => void dates.refetch()}>重试</Button>} />}
    <Select aria-label="通报周次" placeholder="选择学期和周次" loading={dates.isLoading} value={effectiveWeekId}
      style={{ width: '100%' }} onChange={setSelectedWeekId}
      options={(dates.data?.items ?? []).map(item => ({ value: item.weekId, label: `${item.semesterName} · 第 ${item.weekNo} 周` }))} />
    {selected && <WeekDateEditor key={`${selected.weekId}:${selected.version}`} week={selected} />}
  </section>
}

function WorkbookPreview({ types }: { types: ReportOutputEditorSettings['inspectionTypes'] }) {
  const [activeCode, setActiveCode] = useState(types[0]?.code)
  const active = types.find(type => type.code === activeCode) ?? types[0]
  if (!active) return <div className="report-workbook-empty">请先选择要预览的检查类型</div>
  return <div className="report-workbook-preview" aria-label="Excel 工作簿预览">
    <section className="report-workbook-sheet" aria-label={active.name + '工作表'}>
      <div className="report-workbook-formula"><span>fx</span><span>示例学期 第9周 {active.title || active.name}</span></div>
      <div className="report-workbook-grid"><table><thead><tr><th className="report-workbook-axis" />{active.excelColumns.map((_, index) => <th className="report-workbook-letter" key={index}>{String.fromCharCode(65 + index)}</th>)}</tr></thead><tbody>
        <tr><th className="report-workbook-axis">1</th><td colSpan={active.excelColumns.length} className="report-workbook-title">示例学期 第9周 {active.title || active.name}</td></tr>
        <tr><th className="report-workbook-axis">2</th>{active.excelColumns.map(column => <td key={column}>{column}</td>)}</tr>
        <tr><th className="report-workbook-axis">3</th><td colSpan={active.excelColumns.length || 1}>本周无已确认违纪记录</td></tr>
      </tbody></table></div>
      <div className="report-workbook-tabs" role="tablist" aria-label="工作表">{types.map(type => <button key={type.code} type="button" role="tab" aria-selected={type.code === active.code} className={type.code === active.code ? 'active' : ''} onClick={() => setActiveCode(type.code)}>{type.name}</button>)}</div>
    </section>
  </div>
}

function SettingsForm({ settings, save, pending }: { settings: ReportOutputEditorSettings; save: (value: ReportOutputSettings) => Promise<boolean>; pending: boolean }) {
  const dates = useQuery({ queryKey: ['report-output-week-dates'], queryFn: reportOutputApi.weekDates })
  const workContext = useWorkContext()
  const [previewWeekId, setPreviewWeekId] = useState<string>()
  const [format, setFormat] = useState<Format>('WORD')
  const [document, setDocument] = useState<IntroductionDocument>(() => initialDocument(settings))
  const [wordCodes, setWordCodes] = useState(() => settings.wordTypeCodes ?? [])
  const [excelCodes, setExcelCodes] = useState(() => settings.excelTypeCodes ?? [])
  const [editorOpen, setEditorOpen] = useState(false)
  const [previewOpen, setPreviewOpen] = useState(false)
  const [previewing, setPreviewing] = useState(false)
  const [previewError, setPreviewError] = useState('')
  const configuredWeeks = dates.data?.items.filter(item => item.issueDate) ?? []
  const selectedWeek = configuredWeeks.find(item => item.weekId === previewWeekId)
    ?? configuredWeeks.find(item => item.weekId === workContext.data?.week?.id)
    ?? configuredWeeks[0]
  const previewPeriod = selectedWeek ? { semesterName: selectedWeek.semesterName, weekNo: selectedWeek.weekNo, issueDate: selectedWeek.issueDate } : undefined
  const codes = format === 'WORD' ? wordCodes : format === 'EXCEL' ? excelCodes : []
  const updateCodes = (next: string[]) => {
    if (format === 'WORD') {
      setWordCodes(next)
      setDocument(current => syncGeneratedAttachments(current, attachmentLines(settings, wordCodes), attachmentLines(settings, next)))
    } else setExcelCodes(next)
  }
  const names = codes.map(code => settings.inspectionTypes.find(type => type.code === code)?.name ?? code)
  const wordTypes = wordCodes.map(code => settings.inspectionTypes.find(type => type.code === code)).filter((type): type is NonNullable<typeof type> => Boolean(type))
  const selectedTypes = codes.map(code => settings.inspectionTypes.find(type => type.code === code)).filter((type): type is NonNullable<typeof type> => Boolean(type))
  const toggle = (code: string, checked: boolean) => updateCodes(checked ? [...codes, code] : codes.filter(value => value !== code))
  const move = (index: number, offset: number) => {
    const next = index + offset
    if (next < 0 || next >= codes.length) return
    const reordered = [...codes]
    ;[reordered[index], reordered[next]] = [reordered[next], reordered[index]]
    updateCodes(reordered)
  }
  const saveSettings = () => save({ introduction: settings.introduction, introductionDocument: document, wordTypeCodes: wordCodes, excelTypeCodes: excelCodes, version: settings.version })
  const downloadPreview = async () => {
    setPreviewing(true)
    setPreviewError('')
    try {
      let blob: Blob
      if (format === 'WORD') {
        if (!selectedWeek) throw new Error('请先在“文件日期”中设置通报周次的日期')
        blob = await reportOutputApi.previewWord(document, wordCodes, { semesterId: selectedWeek.semesterId, weekId: selectedWeek.weekId })
      } else {
        blob = await reportOutputApi.previewExcel(excelCodes)
      }
      const url = URL.createObjectURL(blob)
      const anchor = window.document.createElement('a')
      anchor.href = url
      anchor.download = format === 'WORD' ? '通报版式预览.docx' : '通报工作簿预览.xlsx'
      window.document.body.append(anchor)
      anchor.click()
      anchor.remove()
      window.setTimeout(() => URL.revokeObjectURL(url), 60_000)
    } catch (error) {
      setPreviewError(error instanceof Error ? error.message : '预览生成失败，请重试')
    } finally {
      setPreviewing(false)
    }
  }
  return <div className="report-output-settings-workspace">
    <header className="report-output-settings-header">
      <div><Typography.Title level={3}>通报文件设置</Typography.Title><Typography.Text type="secondary">维护 Word、Excel 与每周文件日期</Typography.Text></div>
      {format !== 'DATE' && <Space><Button onClick={() => setPreviewOpen(true)}>预览</Button><Button type="primary" loading={pending} onClick={() => void saveSettings()}>保存配置</Button></Space>}
    </header>
    <div className="report-output-config-layout">
      <nav className="report-output-config-nav" aria-label="文件配置类型">
        <Menu mode="inline" selectedKeys={[format]} onClick={({ key }) => setFormat(key as Format)} items={[
          { key: 'WORD', label: <span>Word 配置 <Typography.Text type="secondary">{wordCodes.length} 项</Typography.Text></span> },
          { key: 'EXCEL', label: <span>Excel 配置 <Typography.Text type="secondary">{excelCodes.length} 项</Typography.Text></span> },
          { key: 'DATE', label: '文件日期' },
        ]} />
      </nav>
      <Card className="report-output-config-detail" variant="borderless">
        <div className="report-output-config-heading"><div><Typography.Title level={4}>{format === 'WORD' ? 'Word 配置' : format === 'EXCEL' ? 'Excel 配置' : '每周文件日期'}</Typography.Title><Typography.Text type="secondary">{format === 'WORD' ? '通报前文与检查附件' : format === 'EXCEL' ? '每项检查生成一张工作表' : '按通报周次设置文件落款日期'}</Typography.Text></div>{format !== 'DATE' && <span>配置版本 {settings.version}</span>}</div>
        {format === 'DATE' && <WeekDateSettings />}
        {format === 'WORD' && <section className="report-output-config-section" aria-label="Word 前文"><div className="report-output-config-section-title"><strong>通报首页</strong><Button onClick={() => setEditorOpen(true)}>编辑首页</Button></div><ReportDocumentPreview document={document} types={wordTypes} period={previewPeriod ?? { semesterName: '示例学期', weekNo: 9, issueDate: '未设置文件日期' }} compact /></section>}
        {format !== 'DATE' && <div className="report-output-config-columns">
          <section className="report-output-config-section" aria-label="检查类型"><div className="report-output-config-section-title"><strong>检查类型</strong><span>已选 {codes.length} 项</span></div><div className="report-output-type-grid">{settings.inspectionTypes.map(type => <label className="report-output-type-option" key={type.code}><Checkbox checked={codes.includes(type.code)} onChange={event => toggle(type.code, event.target.checked)} /> <span>{type.name}</span></label>)}</div></section>
          <section className="report-output-config-section" aria-label="文件顺序"><div className="report-output-config-section-title"><strong>{format === 'WORD' ? '附件顺序' : '工作表顺序'}</strong></div><div className="report-output-default-order">{codes.length === 0 && <p className="report-output-config-empty">选择检查类型后，在此调整顺序</p>}{codes.map((code, index) => <div className="report-output-order-row" key={code}><span>{index + 1}. {names[index]}</span><Space size={0}><Button type="text" size="small" icon={<ArrowUpOutlined />} aria-label={'上移 ' + names[index]} disabled={index === 0} onClick={() => move(index, -1)} /><Button type="text" size="small" icon={<ArrowDownOutlined />} aria-label={'下移 ' + names[index]} disabled={index === codes.length - 1} onClick={() => move(index, 1)} /></Space></div>)}</div></section>
        </div>}
      </Card>
    </div>
    <Drawer title="编辑通报首页" className="report-intro-drawer" open={editorOpen} size={1100} extra={<Button type="primary" loading={pending} onClick={() => { void saveSettings().then(saved => { if (saved) setEditorOpen(false) }) }}>保存并完成</Button>} onClose={() => setEditorOpen(false)} destroyOnHidden><ReportIntroductionEditor value={document} onChange={setDocument} attachmentNames={wordTypes.map(type => type.title || type.name)} /></Drawer>
    <Drawer title={format === 'WORD' ? 'Word 文件预览' : 'Excel 工作簿预览'} open={previewOpen} size={1100} onClose={() => setPreviewOpen(false)} extra={<Button loading={previewing} disabled={!codes.length || (format === 'WORD' && !selectedWeek)} onClick={() => void downloadPreview()}>下载预览 {format === 'WORD' ? 'Word' : 'Excel'}</Button>}>
      <Typography.Text type="secondary">使用当前未保存的配置展示文件结构；以下载的文件为准</Typography.Text>
      {format === 'WORD' && <div style={{ margin: '16px 0' }}><Select aria-label="预览通报周次" placeholder="选择已设置文件日期的周次" loading={dates.isLoading} value={selectedWeek?.weekId} onChange={setPreviewWeekId} style={{ width: 320 }} options={configuredWeeks.map(item => ({ value: item.weekId, label: `${item.semesterName} · 第 ${item.weekNo} 周 · ${item.issueDate}` }))} />{dates.isError && <Alert type="error" showIcon title="文件日期加载失败" action={<Button onClick={() => void dates.refetch()}>重试</Button>} />}{!dates.isLoading && !dates.isError && !configuredWeeks.length && <Alert type="warning" showIcon title="请先在“文件日期”中设置通报周次的日期" />}</div>}
      {format === 'WORD' ? <ReportDocumentPreview document={document} types={wordTypes} period={previewPeriod ?? { semesterName: '示例学期', weekNo: 9, issueDate: '未设置文件日期' }} /> : <WorkbookPreview types={selectedTypes} />}
      {previewError && <Alert type="error" showIcon title={previewError} />}
    </Drawer>
  </div>
}

export function ReportOutputSettingsPanel() {
  const client = useQueryClient()
  const [messageApi, contextHolder] = message.useMessage()
  const settings = useQuery({ queryKey: ['report-output-editor-settings'], queryFn: reportOutputApi.editorSettings })
  const save = useMutation({
    mutationFn: (input: ReportOutputSettings) => reportOutputApi.updateSettings(input),
    onSuccess: async () => {
      await Promise.all([client.invalidateQueries({ queryKey: ['report-output-editor-settings'] }), client.invalidateQueries({ queryKey: ['report-output-settings'] })])
      void messageApi.success('通报文件设置已保存')
    },
    onError: error => { void messageApi.error(error.message) },
  })
  return <section aria-label="通报文件设置">{contextHolder}
    {settings.isError && <Alert type="error" showIcon title="通报文件设置加载失败" action={<Button onClick={() => void settings.refetch()}>重试</Button>} />}
    {settings.isLoading && <div role="status">正在加载通报文件设置…</div>}
    {settings.data && <SettingsForm key={settings.data.version} settings={settings.data} save={async value => {
      try { await save.mutateAsync(value); return true } catch { return false }
    }} pending={save.isPending} />}
  </section>
}

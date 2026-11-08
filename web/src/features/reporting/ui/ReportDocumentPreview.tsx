import type { IntroductionDocument, ReportOutputType } from '../../../shared/api/reportOutputApi'
import { footerTemplate } from '../model/reportFooter'

const fontStacks: Record<string, string> = { 仿宋: "FangSong, '仿宋', serif", 宋体: "SimSun, '宋体', serif", 黑体: "SimHei, '黑体', sans-serif", 楷体: "KaiTi, '楷体', serif", 微软雅黑: "'Microsoft YaHei', '微软雅黑', sans-serif", 华文新魏: "STXinwei, '华文新魏', serif" }

type PreviewProps = { document: IntroductionDocument; types: ReportOutputType[]; compact?: boolean }

function documentText(document: IntroductionDocument) {
  return document.content.some(paragraph => paragraph.attrs.style !== 'title' && paragraph.attrs.style !== 'subtitle' && paragraph.content?.some(run => run.text.trim()))
}

function PageFooter({ document, page }: { document: IntroductionDocument; page: number }) {
  if (document.footerLayout === 'custom') return <div className="report-document-page-footer custom" style={{ textAlign: document.footerAlign ?? 'center' }}>
    {footerTemplate(document).split('\n').map((line, index) => line.includes('\t') ? <div className="report-document-footer-sides" key={index}><span>{line.split('\t')[0].replaceAll('{page}', String(page))}</span><span>{line.split('\t').slice(1).join('\t').replaceAll('{page}', String(page))}</span></div> : <div key={index}>{line.replaceAll('{page}', String(page)) || '\u00a0'}</div>)}
  </div>
  const layout = document.footerLayout || 'stacked'
  return <div className={`report-document-page-footer layout-${layout}`}>
    {layout !== 'numberOnly' && <span className="report-document-footer-text">{document.footerText || '\u00a0'}</span>}
    {layout !== 'textOnly' && <span className="report-document-page-number">{page}</span>}
  </div>
}

/** Visual preview of the same cover structure and ordered attachment columns used by the DOCX renderer. */
export function ReportDocumentPreview({ document, types, compact = false }: PreviewProps) {
  const title = (document.titleTemplate || '{semesterName} 第{weekNo}周日常管理通报').replaceAll('{semesterName}', '示例学期').replaceAll('{weekNo}', '9')
  const hasStyledTitle = document.content.some(paragraph => paragraph.attrs.style === 'title' || paragraph.attrs.style === 'subtitle')
  const attachmentPosition = document.attachmentPosition ?? document.content.length
  const attachments = <div className="report-document-attachments">{types.map((type, index) => <p key={type.code}>{`附件${index + 1}：${type.title || type.name}`}</p>)}</div>
  return <div className={compact ? 'report-document-preview compact' : 'report-document-preview'}>
    <article className="report-document-cover" aria-label="Word 通报首页预览">
      <div className="report-document-page-header">{document.headerText || '　'}</div>
      {!hasStyledTitle && <h2>{title}</h2>}
      {!documentText(document) && <p className="report-document-empty">通报前文待填写</p>}
      {document.content.map((paragraph, index) => <div key={index}>{!document.manualAttachmentList && attachmentPosition === index && attachments}<p className={'report-document-paragraph style-' + paragraph.attrs.style} style={{ textAlign: paragraph.attrs.textAlign as 'left' | 'center' | 'right' | 'justify', textIndent: paragraph.attrs.indent ? `${paragraph.attrs.indent}em` : undefined, marginTop: paragraph.attrs.spaceBefore !== undefined ? `${paragraph.attrs.spaceBefore}pt` : undefined, marginBottom: paragraph.attrs.spaceAfter !== undefined ? `${paragraph.attrs.spaceAfter}pt` : undefined, lineHeight: paragraph.attrs.lineSpacingMode === 'single' ? 1 : paragraph.attrs.lineSpacingMode === 'oneHalf' ? 1.5 : paragraph.attrs.lineSpacingMode === 'double' ? 2 : paragraph.attrs.lineSpacing ? `${paragraph.attrs.lineSpacing}pt` : undefined }}>
        {paragraph.content?.map((run, runIndex) => {
          const font = run.marks?.find(mark => mark.type === 'textStyle')
          const style = font?.type === 'textStyle' ? font.attrs : undefined
          return <span key={runIndex} style={{ fontFamily: style?.fontFamily ? fontStacks[style.fontFamily] : undefined, fontSize: compact ? undefined : style?.fontSize, fontWeight: run.marks?.some(mark => mark.type === 'bold') ? 700 : undefined, fontStyle: run.marks?.some(mark => mark.type === 'italic') ? 'italic' : undefined, textDecoration: [run.marks?.some(mark => mark.type === 'underline') ? 'underline' : '', run.marks?.some(mark => mark.type === 'strike') ? 'line-through' : ''].filter(Boolean).join(' ') || undefined }}>{run.text.replaceAll('{semesterName}', '示例学期').replaceAll('{weekNo}', '9')}</span>
        })}
      </p></div>)}
      {!document.manualAttachmentList && attachmentPosition >= document.content.length && attachments}
      <PageFooter document={document} page={1} />
    </article>
    {!compact && types.map((type, index) => <article className="report-document-attachment" key={type.code} aria-label={type.name + '附件预览'}>
      <div className="report-document-page-header">{document.headerText || '　'}</div>
      <h2>{type.title || type.name}</h2>
      <table><thead><tr>{type.columns.map(column => <th key={column}>{column}</th>)}</tr></thead><tbody><tr><td colSpan={type.columns.length || 1}>本周无已确认违纪记录</td></tr></tbody></table>
      <PageFooter document={document} page={index + 2} />
    </article>)}
  </div>
}

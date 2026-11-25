import { Icon } from '@iconify/react'
import alignLeft from '@iconify-icons/lucide/align-left'
import alignCenter from '@iconify-icons/lucide/align-center'
import alignRight from '@iconify-icons/lucide/align-right'
import alignJustify from '@iconify-icons/lucide/align-justify'
import growFont from '@iconify-icons/lucide/a-arrow-up'
import shrinkFont from '@iconify-icons/lucide/a-arrow-down'
import bold from '@iconify-icons/lucide/bold'
import italic from '@iconify-icons/lucide/italic'
import underline from '@iconify-icons/lucide/underline'
import strike from '@iconify-icons/lucide/strikethrough'
import clearFormat from '@iconify-icons/lucide/remove-formatting'
import minus from '@iconify-icons/lucide/minus'
import plus from '@iconify-icons/lucide/plus'
import undoIcon from '@iconify-icons/lucide/undo-2'
import redoIcon from '@iconify-icons/lucide/redo-2'
import listTree from '@iconify-icons/lucide/list-tree'
import bracesIcon from '@iconify-icons/lucide/braces'
import indentIncrease from '@iconify-icons/lucide/indent-increase'
import indentDecrease from '@iconify-icons/lucide/indent-decrease'
import spacingIcon from '@iconify-icons/lucide/rows-3'
import { Node, type Editor, type JSONContent } from '@tiptap/core'
import { Paragraph } from '@tiptap/extension-paragraph'
import TextAlign from '@tiptap/extension-text-align'
import { FontFamily, FontSize, TextStyle } from '@tiptap/extension-text-style'
import { EditorContent, useEditor, useEditorState } from '@tiptap/react'
import StarterKit from '@tiptap/starter-kit'
import { Button, Dropdown, InputNumber, Popover, Select, Tooltip } from 'antd'
import { useEffect, useRef, useState, type ReactNode } from 'react'
import type { IntroductionDocument, IntroductionLineSpacingMode } from '../../../shared/api/reportOutputApi'
import { applyWordFontSize, wordFontSizes as sizes } from '../model/wordTable'
import { ParagraphSettings } from './ParagraphSettings'
import { ReportFooterEditor, type ReportFooterEditorHandle } from './ReportFooterEditor'

const chineseFontSizes:Record<string,number> = {初号:42,小初:36,一号:26,小一:24,二号:22,小二:18,三号:16,小三:15,四号:14,小四:12,五号:10.5,小五:9,六号:7.5,小六:6.5,七号:5.5,八号:5}
const fontSizeOptions = [
  {label:'中文字号',options:Object.entries(chineseFontSizes).map(([name,points])=>({value:name,label:name+' · '+points+' 磅'}))},
  {label:'数字字号（磅）',options:sizes.map(size=>({value:size,label:size.replace('pt','')}))},
]
const fonts = ['仿宋', '宋体', '黑体', '楷体', '微软雅黑', '华文新魏']
const fontStacks: Record<string, string> = { 仿宋: "FangSong, '仿宋', serif", 宋体: "SimSun, '宋体', serif", 黑体: "SimHei, '黑体', sans-serif", 楷体: "KaiTi, '楷体', serif", 微软雅黑: "'Microsoft YaHei', '微软雅黑', sans-serif", 华文新魏: "STXinwei, '华文新魏', serif" }
const defaultFont = (style?: string) => style === 'title' ? '华文新魏' : ['subtitle', 'heading', 'heading1', 'heading2', 'heading3'].includes(style ?? '') ? '黑体' : '仿宋'
const defaultSize = (style?: string) => style === 'title' ? '30pt' : style === 'subtitle' ? '20pt' : '15pt'

const paragraphStyles = [
  { value: 'body', label: '正文' },
  { value: 'title', label: '通报主标题' },
  { value: 'subtitle', label: '通报副标题' },
  { value: 'heading', label: '小标题' },
  { value: 'heading1', label: '一级标题' },
  { value: 'heading2', label: '二级标题' },
  { value: 'heading3', label: '三级标题' },
  { value: 'signature', label: '落款' },
  { value: 'date', label: '日期' },
]
const alignments = [
  { value: 'left', label: '左对齐', icon: <Icon icon={alignLeft} /> },
  { value: 'center', label: '居中', icon: <Icon icon={alignCenter} /> },
  { value: 'right', label: '右对齐', icon: <Icon icon={alignRight} /> },
  { value: 'justify', label: '两端对齐', icon: <Icon icon={alignJustify} /> },
] as const

const IntroductionParagraph = Paragraph.extend({
  addAttributes() {
    return {
      ...this.parent?.(),
      style: {
        default: 'body',
        parseHTML: element => {
          const style = element.getAttribute('data-style')
          return paragraphStyles.some(option => option.value === style) ? style : 'body'
        },
        renderHTML: attributes => ({ 'data-style': attributes.style }),
      },
      fontSize: { default:null, parseHTML:el=>sizes.includes(el.style.fontSize)?el.style.fontSize:null,
        renderHTML:attrs=>sizes.includes(attrs.fontSize)?{style:`font-size:${attrs.fontSize}`}:{} },
      border: { default: null, parseHTML: element => element.dataset.paragraphBorder, renderHTML: attrs => attrs.border ? { 'data-paragraph-border': attrs.border } : {} },
      indent: {
        default: 0,
        parseHTML: element => Number(element.getAttribute('data-indent') || 0),
        renderHTML: attributes => ({ 'data-indent': Math.max(0, Math.min(4, Number(attributes.indent) || 0)) }),
      },
      ...Object.fromEntries(['tabStop','leftChars','rightChars','hanging','outline','beforeLines','afterLines','rtl','gridIndent','paragraphGrid','keepNext','keepLines','pageBefore','widow'].map(key=>{
        const attribute='data-'+key.replace(/[A-Z]/g,c=>'-'+c.toLowerCase())
        return [key,{default:null,parseHTML:(el:HTMLElement)=>{
          const value=el.getAttribute(attribute)
          return value===null?null:value==='true'?true:value==='false'?false:Number(value)
        },renderHTML:(attrs:Record<string,unknown>)=>attrs[key]==null?{}:{[attribute]:attrs[key]}}]
      })),
      spaceBefore: { default: 0, parseHTML: element => Number(element.getAttribute('data-space-before') || 0), renderHTML: attributes => ({ 'data-space-before': attributes.spaceBefore }) },
      spaceAfter: { default: 0, parseHTML: element => Number(element.getAttribute('data-space-after') || 0), renderHTML: attributes => ({ 'data-space-after': attributes.spaceAfter }) },
      lineSpacing: { default: 0, parseHTML: element => Number(element.getAttribute('data-line-spacing') || 0), renderHTML: attributes => ({ 'data-line-spacing': attributes.lineSpacing }) },
      lineSpacingMode: { default: 'fixed', parseHTML: element => element.getAttribute('data-line-spacing-mode') || 'fixed', renderHTML: attributes => ({ 'data-line-spacing-mode': attributes.lineSpacingMode }) },
    }
  },
  addKeyboardShortcuts(){
    return {Enter:()=>{
      const {selection}=this.editor.state
      const paragraph=selection.$from.parent
      const size=this.editor.getAttributes('textStyle').fontSize
      if(!selection.empty || paragraph.type.name!=='paragraph' || selection.$from.parentOffset!==paragraph.content.size || !sizes.includes(size))return false
      return this.editor.chain().splitBlock().updateAttributes('paragraph',{fontSize:size}).setFontSize(size).run()
    },Tab:()=>this.editor.state.selection.$from.parent.type.name==='paragraph' && !!this.editor.state.selection.$from.parent.attrs.tabStop && this.editor.commands.insertContent('\t')}
  },
  renderHTML({ HTMLAttributes, node }) {
    const defaultLine = node.attrs.style === 'title' ? 40 : node.attrs.style === 'subtitle' ? 28 : 27
    const lineHeight = node.attrs.lineSpacingMode === 'single' ? 'normal' : node.attrs.lineSpacingMode === 'oneHalf' ? '1.5' : node.attrs.lineSpacingMode === 'double' ? '2' : `${node.attrs.lineSpacing || defaultLine}pt`
    const styles = [HTMLAttributes.style, node.attrs.tabStop && `tab-size:${node.attrs.tabStop/20}pt`, node.attrs.rtl && 'direction:rtl', node.attrs.leftChars && `padding-inline-start:${node.attrs.leftChars}em`, node.attrs.rightChars && `padding-inline-end:${node.attrs.rightChars}em`, node.attrs.hanging && `text-indent:-${node.attrs.hanging}em`, `margin-top:${node.attrs.spaceBefore || 0}pt`, `margin-bottom:${node.attrs.spaceAfter || 0}pt`, `line-height:${lineHeight}`].filter(Boolean).join(';')
    return ['p', { ...HTMLAttributes, style: styles }, 0]
  },
})

const TemplateVariable = Node.create({
  name: 'templateVariable', group: 'inline', inline: true, atom: true,
  addAttributes() { return { key: { default: 'semesterName', parseHTML: element => element.getAttribute('data-template-variable'), renderHTML: attributes => ({ 'data-template-variable': attributes.key }) } } },
  parseHTML() { return [{ tag: 'span[data-template-variable]' }] },
  renderHTML({ node }) { return ['span', { class: 'report-template-variable', 'data-template-variable': node.attrs.key, title: node.attrs.key === 'reportDate' ? '通报文件设置中的每周文件日期' : node.attrs.key === 'weekNo' || node.attrs.key === 'weekNoChinese' ? '周次变量' : '学期变量' }, node.attrs.key === 'reportDate' ? '文件日期' : node.attrs.key === 'weekNoChinese' ? '九' : node.attrs.key === 'weekNo' ? '9' : '示例学期'] },
})

function editorContent(document: IntroductionDocument): JSONContent {
  return { type: 'doc', content: document.content.map(paragraph => ({ ...paragraph, content: paragraph.content?.flatMap(run => run.text.split(/(\{semesterName\}|\{weekNo\}|\{weekNoChinese\}|\{reportDate\})/g).filter(Boolean).map(part => ['{semesterName}', '{weekNo}', '{weekNoChinese}', '{reportDate}'].includes(part) ? { type: 'templateVariable', attrs: { key: part.slice(1, -1) } } : { ...run, text: part })) })) }
}

const RestrictedFontFamily = FontFamily.extend({
  addGlobalAttributes() {
    return [{ types: ['textStyle'], attributes: { fontFamily: {
      default: null,
      parseHTML: element => fonts.find(font => element.style.fontFamily === font || element.style.fontFamily === fontStacks[font]) ?? null,
      renderHTML: attributes => fonts.includes(attributes.fontFamily) ? { style: `font-family: ${fontStacks[attributes.fontFamily]}` } : {},
    } } }]
  },
})

const RestrictedFontSize = FontSize.extend({
  addGlobalAttributes() {
    return [{ types: ['textStyle'], attributes: { fontSize: {
      default: null,
      parseHTML: element => sizes.includes(element.style.fontSize) ? element.style.fontSize : null,
      renderHTML: attributes => sizes.includes(attributes.fontSize) ? { style: `font-size: ${attributes.fontSize}` } : {},
    } } }]
  },
})

export const introductionExtensions = [
  StarterKit.configure({
    paragraph: false, blockquote: false, bulletList: false, code: false,
    codeBlock: false, dropcursor: false, gapcursor: false, hardBreak: false,
    heading: false, horizontalRule: false, link: false, listItem: false,
    listKeymap: false, orderedList: false, trailingNode: false,
  }),
  IntroductionParagraph, TemplateVariable,
  TextAlign.configure({ types: ['paragraph'], defaultAlignment: 'left' }),
  TextStyle, RestrictedFontFamily, RestrictedFontSize,
]

function introductionFromEditor(document: JSONContent): IntroductionDocument {
  const content: IntroductionDocument['content'] = (document.content ?? [])
    .filter(paragraph => paragraph.type === 'paragraph')
    .map(paragraph => {
      const style = paragraph.attrs?.style
      const textAlign = paragraph.attrs?.textAlign
      const indent = Math.max(0, Math.min(4, Number(paragraph.attrs?.indent) || 0))
      const runs: NonNullable<IntroductionDocument['content'][number]['content']> = (paragraph.content ?? [])
        .filter(run => (run.type === 'text' && typeof run.text === 'string' && run.text.length > 0) || run.type === 'templateVariable')
        .map(run => {
          if (run.type === 'templateVariable') return { type: 'text' as const, text: run.attrs?.key === 'reportDate' ? '{reportDate}' : run.attrs?.key === 'weekNoChinese' ? '{weekNoChinese}' : run.attrs?.key === 'weekNo' ? '{weekNo}' : '{semesterName}' }
          const marks: NonNullable<NonNullable<IntroductionDocument['content'][number]['content']>[number]['marks']> = []
          for (const mark of run.marks ?? []) {
            if (mark.type === 'bold' || mark.type === 'italic' || mark.type === 'strike' || mark.type === 'underline') marks.push({ type: mark.type })
            if (mark.type === 'textStyle') {
              const fontFamily = fonts.includes(mark.attrs?.fontFamily) ? mark.attrs?.fontFamily as string : undefined
              const fontSize = sizes.includes(mark.attrs?.fontSize) ? mark.attrs?.fontSize as string : undefined
              if (fontFamily || fontSize) marks.push({ type: 'textStyle', attrs: { fontFamily, fontSize } })
            }
          }
          return { type: 'text' as const, text: run.text ?? '', ...(marks.length ? { marks } : {}) }
        })
      return {
        type: 'paragraph' as const,
        attrs: {
          style: paragraphStyles.some(option => option.value === style) ? style as string : 'body',
          textAlign: alignments.some(option => option.value === textAlign) ? textAlign as string : 'left',
          ...(sizes.includes(paragraph.attrs?.fontSize) ? {fontSize:paragraph.attrs?.fontSize} : {}),
          ...(paragraph.attrs?.border ? {border:paragraph.attrs.border} : {}),
          indent,
          spaceBefore: Number(paragraph.attrs?.spaceBefore) || 0,
          spaceAfter: Number(paragraph.attrs?.spaceAfter) || 0,
          lineSpacing: Number(paragraph.attrs?.lineSpacing) || 0,
          lineSpacingMode: (['single', 'oneHalf', 'double', 'fixed'].includes(paragraph.attrs?.lineSpacingMode) ? paragraph.attrs?.lineSpacingMode : 'fixed') as IntroductionLineSpacingMode,
        },
        ...(runs.length ? { content: runs } : {}),
      }
    })
  return { type: 'doc', content: content.length ? content : [{ type: 'paragraph', attrs: { style: 'body', textAlign: 'left' } }] }
}

type ToolButtonProps = {
  label: string
  icon: ReactNode
  active?: boolean
  disabled?: boolean
  onClick: () => void
}

export function ToolButton({ label, icon, active, disabled = false, onClick }: ToolButtonProps) {
  return <Tooltip title={label} trigger={['hover', 'focus']}><Button
    type="text"
    className="report-intro-tool"
    icon={icon}
    aria-label={label}
    aria-pressed={active}
    disabled={disabled}
    data-active={active || undefined}
    onMouseDown={event => event.preventDefault()}
    onClick={onClick}
  /></Tooltip>
}

function selectionFormatting(editor: Editor) {
  return {
    border: editor.getAttributes('paragraph').border as string | undefined,
    paragraphStyle: editor.getAttributes('paragraph').style as string | undefined,
    indent: Number(editor.getAttributes('paragraph').indent) || 0,
    spaceBefore: Number(editor.getAttributes('paragraph').spaceBefore) || 0,
    spaceAfter: Number(editor.getAttributes('paragraph').spaceAfter) || 0,
    lineSpacing: Number(editor.getAttributes('paragraph').lineSpacing) || 0,
    lineSpacingMode: (editor.getAttributes('paragraph').lineSpacingMode || 'fixed') as IntroductionLineSpacingMode,
    textAlign: editor.getAttributes('paragraph').textAlign as string | undefined,
    fontFamily: editor.getAttributes('textStyle').fontFamily as string | undefined,
    fontSize: (editor.getAttributes('textStyle').fontSize ?? editor.state.selection.$from.parent.attrs.fontSize) as string | undefined,
    bold: editor.isActive('bold'),
    italic: editor.isActive('italic'),
    underline: editor.isActive('underline'),
    strike: editor.isActive('strike'),
  }
}

/** Edits the cover title, body, attachment directory, and page text accepted by the Word renderer. */
export function ReportIntroductionEditor({ value, onChange, attachmentNames = [], toolbarAction }: { value: IntroductionDocument; onChange: (value: IntroductionDocument) => void; attachmentNames?: string[]; toolbarAction?: ReactNode }) {
  const footerRef = useRef<ReportFooterEditorHandle>(null)
  const [footerActive, setFooterActive] = useState(false)
  const onChangeRef = useRef(onChange)
  const documentRef = useRef(value)
  const history = useRef<{ past: IntroductionDocument[]; future: IntroductionDocument[] }>({ past: [], future: [] })
  const [historyState, setHistoryState] = useState({ canUndo: false, canRedo: false })
  const [zoom, setZoom] = useState(100)
  const [pageCount, setPageCount] = useState(1)
  const contentRef = useRef<HTMLDivElement>(null)
  useEffect(() => { onChangeRef.current = onChange; documentRef.current = value }, [onChange, value])
  const updateDocument = (next: IntroductionDocument) => {
    if (JSON.stringify(next) === JSON.stringify(documentRef.current)) return
    history.current.past.push(documentRef.current)
    if (history.current.past.length > 100) history.current.past.shift()
    history.current.future = []
    documentRef.current = next
    onChangeRef.current(next)
    setHistoryState({ canUndo: true, canRedo: false })
  }

  const editor = useEditor({
    extensions: introductionExtensions,
    content: editorContent(value),
    immediatelyRender: false,
    onUpdate: ({ editor: updatedEditor }) => {
      const json = updatedEditor.getJSON()
      const content = introductionFromEditor(json).content
      updateDocument({ ...documentRef.current, content })
    },
  })
  useEffect(() => {
    if (!editor) return
    let frame = 0
    const measure = () => {
      cancelAnimationFrame(frame)
      frame = requestAnimationFrame(() => {
        const content = contentRef.current?.querySelector<HTMLElement>('.tiptap')
        if (!content) return
        const paragraphs = Array.from(content.children).filter((node): node is HTMLElement => node instanceof HTMLElement)
        // Keep pagination visual only; page spacing never enters the saved document.
        for (const paragraph of paragraphs) paragraph.style.marginTop = `${Number(paragraph.dataset.spaceBefore) || 0}pt`
        let page = 1
        let used = 0
        for (const paragraph of paragraphs) {
          const before = (Number(paragraph.dataset.spaceBefore) || 0) * 4 / 3
          const after = (Number(paragraph.dataset.spaceAfter) || 0) * 4 / 3
          const height = paragraph.getBoundingClientRect().height
          if (used > 0 && used + before + height + after > 887) {
            paragraph.style.marginTop = `calc(${before}px + ${887 - used + 260}px)`
            used = before + height + after
            page += 1
          } else {
            used += before + height + after
          }
        }
        setPageCount(page)
      })
    }
    const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(measure)
    observer?.observe(editor.view.dom)
    editor.on('transaction', measure)
    measure()
    return () => { observer?.disconnect(); editor.off('transaction', measure); cancelAnimationFrame(frame) }
  }, [editor])
  const restore = (direction: 'undo' | 'redo') => {
    const source = direction === 'undo' ? history.current.past : history.current.future
    const target = direction === 'undo' ? history.current.future : history.current.past
    const next = source.pop()
    if (!next) return
    target.push(documentRef.current)
    documentRef.current = next
    editor?.commands.setContent(editorContent(next), { emitUpdate: false })
    onChangeRef.current(next)
    setHistoryState({ canUndo: history.current.past.length > 0, canRedo: history.current.future.length > 0 })
  }
  const insertDirectory = () => {
    if (!editor || attachmentNames.length === 0) return
    const selection = editor.state.selection.$from
    const position = Math.min(selection.index(0) + 1, documentRef.current.content.length)
    const inserted = attachmentNames.map((name, index) => `附件${index + 1}：${name}`).map(text => ({ type: 'paragraph' as const, attrs: { style: 'body', textAlign: 'left', indent: 2, lineSpacing: 27 }, content: [{ type: 'text' as const, text }] }))
    const content = [...documentRef.current.content]
    content.splice(position, 0, ...inserted)
    const next = { ...documentRef.current, content, attachmentPosition: undefined, manualAttachmentList: true }
    updateDocument(next)
    editor.commands.setContent(editorContent(next), { emitUpdate: false })
  }
  const insertTitleVariable = (token: string) => { editor?.chain().focus().insertContent({ type: 'templateVariable', attrs: { key: token.slice(1, -1) } }).run() }

  return <div className="report-intro-editor" onKeyDownCapture={event => {
    if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'z') {
      event.preventDefault()
      restore(event.shiftKey ? 'redo' : 'undo')
    }
  }}>
    <div className="report-intro-toolbar" role="toolbar" aria-label="通报正文排版工具">
      <div className="report-intro-zoom" aria-label="页面缩放">
        <ToolButton label="缩小" icon={<Icon icon={minus} />} disabled={zoom <= 75} onClick={() => setZoom(current => Math.max(75, current - 25))} />
        <span aria-live="polite">{zoom}%</span>
        <ToolButton label="放大" icon={<Icon icon={plus} />} disabled={zoom >= 125} onClick={() => setZoom(current => Math.min(125, current + 25))} />
      </div>
      <span className="report-intro-page-count" role="status">共 {pageCount} 页{pageCount > 1 ? '，正文已续到下一页' : ''}</span>
      <span className="report-intro-toolbar-divider" aria-hidden="true" />
      <WordFormattingControls editor={editor}/>
      <span className="report-intro-toolbar-divider" aria-hidden="true" />
      <ToolButton label="撤销" icon={<Icon icon={undoIcon} />} disabled={!historyState.canUndo} onClick={() => restore('undo')} />
      <ToolButton label="重做" icon={<Icon icon={redoIcon} />} disabled={!historyState.canRedo} onClick={() => restore('redo')} />
      <span className="report-intro-toolbar-divider" aria-hidden="true" />
      {!footerActive && <div className="report-intro-inserts"><ToolButton label="插入附件文本" icon={<Icon icon={listTree} />} disabled={!editor || attachmentNames.length === 0} onClick={insertDirectory} />
        <Dropdown classNames={{root:"word-control-popup"}} menu={{ items: [{ key: 'semester', label: '学期名称' }, { key: 'week', label: '周次数字' }, { key: 'weekChinese', label: '周次汉字' }, { key: 'date', label: '每周文件日期' }], onClick: ({ key }) => insertTitleVariable(key === 'semester' ? '{semesterName}' : key === 'week' ? '{weekNo}' : key === 'weekChinese' ? '{weekNoChinese}' : '{reportDate}') }} trigger={['click']}><Button type="text" className="report-intro-tool" icon={<Icon icon={bracesIcon} />} aria-label="插入变量" title="插入变量" disabled={!editor} /></Dropdown></div>
      }
      {footerActive && <div className="report-intro-footer-tools" role="toolbar" aria-label="页脚工具">
        <span>页脚</span>
        <ToolButton label="页脚左对齐" icon={<Icon icon={alignLeft} />} onClick={() => updateDocument({ ...documentRef.current, footerLayout: 'custom', footerAlign: 'left' })} />
        <ToolButton label="页脚居中对齐" icon={<Icon icon={alignCenter} />} onClick={() => updateDocument({ ...documentRef.current, footerLayout: 'custom', footerAlign: 'center' })} />
        <ToolButton label="页脚右对齐" icon={<Icon icon={alignRight} />} onClick={() => updateDocument({ ...documentRef.current, footerLayout: 'custom', footerAlign: 'right' })} />
        <Button type="text" size="small" onClick={() => footerRef.current?.insertTab()}>插入分区</Button>
        <Dropdown classNames={{root:"word-control-popup"}} menu={{ items: [
          { key: 'page', label: '当前页码' }, { key: 'totalPages', label: '总页数' },
          { key: 'pageChinese', label: '当前页码（汉字）' }, { key: 'totalPagesChinese', label: '总页数（汉字）' },
          { key: 'semesterName', label: '学期名称' }, { key: 'weekNo', label: '周次数字' }, { key: 'weekNoChinese', label: '周次汉字' },
        ], onClick: ({ key }) => footerRef.current?.insertField(key as 'page' | 'totalPages' | 'pageChinese' | 'totalPagesChinese' | 'semesterName' | 'weekNo' | 'weekNoChinese') }} trigger={['click']}>
          <Button type="text" size="small">插入字段</Button>
        </Dropdown>
        <Button type="text" size="small" onClick={() => updateDocument({ ...documentRef.current, footerLayout: 'custom', footerAlign: 'left', footerText: '{semesterName} 第{weekNo}周\t第 {page} 页 共 {totalPages} 页\t' })}>参考页脚版式</Button>
      </div>}
      {toolbarAction && <div className="report-intro-toolbar-action">{toolbarAction}</div>}
    </div>
    <div className="report-intro-stage" aria-label="通报正文纸张工作区">
      <div className="report-intro-pages" style={{ zoom: `${zoom}%`, height: 1123 * pageCount + 24 * (pageCount - 1) }} aria-label="A4 分页排版参考">
        {Array.from({ length: pageCount }, (_, index) => <div key={index} className="report-intro-page" style={{ top: index * 1147 }} aria-label={`第 ${index + 1} 页`}>
          <div className="report-intro-page-corners" aria-hidden="true"><i /><i /><i /><i /></div>
          {index === 0 ? <>
            <input className="report-intro-page-header" aria-label="编辑页首" maxLength={120} placeholder="点击编辑页首" value={value.headerText ?? ''} onFocus={() => setFooterActive(false)} onChange={event => updateDocument({ ...documentRef.current, headerText: event.target.value })} />
            <ReportFooterEditor ref={footerRef} value={value} onChange={updateDocument} onFocus={() => setFooterActive(true)} />
          </> : <>
            <div className="report-intro-page-header" aria-hidden="true">{value.headerText}</div>
            <div className="report-intro-page-footer" aria-hidden="true">第 {index + 1} 页 / 共 {pageCount} 页</div>
          </>}
        </div>)}
        <div ref={contentRef} onFocusCapture={() => setFooterActive(false)}><EditorContent editor={editor} className="report-intro-content" aria-label="通报正文编辑区" /></div>
      </div>
    </div>
  </div>
}

export function WordFormattingControls({editor,characterTools,discipline=false}:{discipline?:boolean;editor:Editor | null;characterTools?:React.ReactNode}) {
  const state = useEditorState({
    editor,
    selector: ({ editor: currentEditor }) => currentEditor ? selectionFormatting(currentEditor) : null,
  })
  const active = state ?? (editor ? selectionFormatting(editor) : null)
  const [namedSize,setNamedSize]=useState('')
  const currentSize=active?.fontSize || defaultSize(active?.paragraphStyle)
  const index=sizes.indexOf(currentSize)
  const chooseSize=(value:string)=>{
    const points=chineseFontSizes[value]
    setNamedSize(points ? value : '')
    if(editor)applyWordFontSize(editor,points ? points+'pt' : value)
  }
  const stepSize=(offset:number)=>{const next=sizes[index+offset];if(next)chooseSize(next)}
  return <>
      <Select classNames={{popup:{root:"word-control-popup"}}} popupMatchSelectWidth={false} size="small" aria-label="段落样式" variant="borderless" className="report-intro-select report-intro-style-select" value={active?.paragraphStyle || 'body'} options={paragraphStyles} disabled={!editor} onChange={style => editor?.chain().focus().updateAttributes('paragraph', { style }).setTextAlign(style === 'signature' || style === 'date' ? 'right' : style === 'title' || style === 'subtitle' ? 'center' : 'left').run()} />
      <Select classNames={{popup:{root:"word-control-popup"}}} popupMatchSelectWidth={false} size="small" aria-label="段落边框" className="report-intro-select word-border-select" variant="borderless" value={active?.border || ''} options={[{value:'',label:'无边框'},{value:'topBottom',label:'上下单线'},{value:'bottomDouble',label:'下方双线'},...(discipline?[{value:'redRule',label:'粗红分隔线'}]:[])]} onChange={border=>editor?.chain().focus().updateAttributes('paragraph',{border:border||null}).run()}/>
      <Select classNames={{popup:{root:"word-control-popup"}}} popupMatchSelectWidth={false} size="small" aria-label="字体" variant="borderless" className="report-intro-select report-intro-font-select" value={active?.fontFamily || defaultFont(active?.paragraphStyle)} options={fonts.map(font => ({ value: font, label: font }))} disabled={!editor} onChange={font => editor?.chain().focus().setFontFamily(font).run()} />
      <Select classNames={{popup:{root:"word-control-popup"}}} popupMatchSelectWidth={142} virtual={false} showSearch={{optionFilterProp:"label"}} size="small" aria-label="字号" variant="borderless" className="report-intro-select report-intro-size-select" value={chineseFontSizes[namedSize]+'pt'===currentSize?namedSize:currentSize} labelRender={({value})=>String(value).replace('pt','')} options={fontSizeOptions} disabled={!editor} onSelect={chooseSize} />
      <ToolButton label="增大字号" icon={<Icon icon={growFont}/>} disabled={!editor || index>=sizes.length-1} onClick={()=>stepSize(1)}/>
      <ToolButton label="减小字号" icon={<Icon icon={shrinkFont}/>} disabled={!editor || index<=0} onClick={()=>stepSize(-1)}/>
      <span className="report-intro-toolbar-divider" aria-hidden="true" />
      <ToolButton label="加粗" icon={<Icon icon={bold} />} active={active?.bold} disabled={!editor} onClick={() => editor?.chain().focus().toggleBold().run()} />
      <ToolButton label="斜体" icon={<Icon icon={italic} />} active={active?.italic} disabled={!editor} onClick={() => editor?.chain().focus().toggleItalic().run()} />
      <ToolButton label="下划线" icon={<Icon icon={underline} />} active={active?.underline} disabled={!editor} onClick={() => editor?.chain().focus().toggleUnderline().run()} />
      <ToolButton label="删除线" icon={<Icon icon={strike} />} active={active?.strike} disabled={!editor} onClick={() => editor?.chain().focus().toggleStrike().run()} />
      <ToolButton label="清除文字格式" icon={<Icon icon={clearFormat} />} disabled={!editor} onClick={() => editor?.chain().focus().unsetAllMarks().run()} />
      <span className="report-intro-toolbar-divider" aria-hidden="true" />
      {characterTools && <>{characterTools}<span className="word-toolbar-newline" aria-hidden="true"/></>}
      {alignments.map(item => <ToolButton key={item.value} label={item.label} icon={item.icon} active={active?.textAlign === item.value} disabled={!editor} onClick={() => editor?.chain().focus().setTextAlign(item.value).run()} />)}
      <ToolButton label="减少缩进" icon={<Icon icon={indentDecrease} />} disabled={!editor || !active?.indent} onClick={() => editor?.chain().focus().updateAttributes('paragraph', { indent: Math.max(0, (active?.indent ?? 0) - 1), hanging:null }).run()} />
      <ToolButton label="增加缩进" icon={<Icon icon={indentIncrease} />} disabled={!editor || (active?.indent ?? 0) >= 4} onClick={() => editor?.chain().focus().updateAttributes('paragraph', { indent: Math.min(4, (active?.indent ?? 0) + 1), hanging:null }).run()} />
      {characterTools ? <ParagraphSettings editor={editor}/> : <Popover trigger="click" content={<div className="report-intro-spacing">
        <label>段前（磅） <InputNumber aria-label="段前间距" min={0} max={72} value={active?.spaceBefore ?? 0} onChange={spaceBefore => { if (spaceBefore !== null) editor?.chain().focus().updateAttributes('paragraph', { spaceBefore }).run() }} /></label>
        <label>段后（磅） <InputNumber aria-label="段后间距" min={0} max={72} value={active?.spaceAfter ?? 0} onChange={spaceAfter => { if (spaceAfter !== null) editor?.chain().focus().updateAttributes('paragraph', { spaceAfter }).run() }} /></label>
        <label>行距 <Select classNames={{popup:{root:"word-control-popup"}}} popupMatchSelectWidth={false} aria-label="行距方式" value={active?.lineSpacingMode || 'fixed'} options={[{ value: 'single', label: '单倍行距' }, { value: 'oneHalf', label: '1.5 倍行距' }, { value: 'double', label: '双倍行距' }, { value: 'fixed', label: '固定值' }]} onChange={lineSpacingMode => editor?.chain().focus().updateAttributes('paragraph', lineSpacingMode === 'fixed' ? { lineSpacingMode, lineSpacing:active?.lineSpacing || 27 } : { lineSpacingMode }).run()} /></label>
        {(active?.lineSpacingMode || 'fixed') === 'fixed' && <label>固定值（磅） <InputNumber aria-label="固定行距" min={18} max={48} value={active?.lineSpacing || 27} onChange={lineSpacing => { if (lineSpacing !== null) editor?.chain().focus().updateAttributes('paragraph', { lineSpacing }).run() }} /></label>}
      </div>}><Button type="text" className="report-intro-tool" icon={<Icon icon={spacingIcon} />} aria-label="段落间距" title="段落间距" /></Popover>}
  </>
}

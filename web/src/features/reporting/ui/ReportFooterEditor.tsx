import { Node, type JSONContent } from '@tiptap/core'
import { EditorContent, useEditor } from '@tiptap/react'
import StarterKit from '@tiptap/starter-kit'
import { Button, Dropdown } from 'antd'
import { forwardRef, useEffect, useImperativeHandle, useLayoutEffect, useRef } from 'react'
import type { IntroductionDocument } from '../../../shared/api/reportOutputApi'
import { footerTemplate } from '../model/reportFooter'

type FooterFieldKey = 'page' | 'totalPages' | 'pageChinese' | 'totalPagesChinese' | 'semesterName' | 'weekNo' | 'weekNoChinese'
const fieldLabels: Record<FooterFieldKey, string> = { page: '1', totalPages: '总页数', pageChinese: '一', totalPagesChinese: '总页数（汉字）', semesterName: '学期名称', weekNo: '周次', weekNoChinese: '周次（汉字）' }

const FooterField = Node.create({
  name: 'reportFooterField', group: 'inline', inline: true, atom: true,
  addAttributes: () => ({ key: { default: 'page', parseHTML: element => element.getAttribute('data-field-key'), renderHTML: attributes => ({ 'data-field-key': attributes.key }) } }),
  parseHTML: () => [{ tag: 'span[data-report-footer-field]' }],
  renderHTML: ({ node }) => ['span', { 'data-report-footer-field': '', 'data-field-key': node.attrs.key, class: 'report-intro-page-field', contenteditable: 'false', title: '生成文件时自动填充' }, fieldLabels[node.attrs.key as FooterFieldKey]],
})
const FooterTab = Node.create({
  name: 'reportFooterTab', group: 'inline', inline: true, atom: true,
  parseHTML: () => [{ tag: 'span[data-report-footer-tab]' }],
  renderHTML: () => ['span', { 'data-report-footer-tab': '', class: 'report-intro-footer-tab', contenteditable: 'false' }],
})

function footerContent(template: string): JSONContent {
  return { type: 'doc', content: template.split('\n').map(line => ({ type: 'paragraph', content: line.split(/(\{(?:page|pageChinese|totalPages|totalPagesChinese|semesterName|weekNo|weekNoChinese)\}|\t)/g).filter(Boolean).map(part => part === '\t' ? { type: 'reportFooterTab' } : /^\{(?:page|pageChinese|totalPages|totalPagesChinese|semesterName|weekNo|weekNoChinese)\}$/.test(part) ? { type: 'reportFooterField', attrs: { key: part.slice(1, -1) } } : { type: 'text', text: part }) })) }
}

function footerValue(json: JSONContent): string {
  return (json.content ?? []).map(paragraph => (paragraph.content ?? []).map(node => node.type === 'reportFooterField' ? `{${node.attrs?.key}}` : node.type === 'reportFooterTab' ? '\t' : node.type === 'text' ? node.text ?? '' : '').join('')).join('\n')
}

export interface ReportFooterEditorHandle {
  insertField: (key: FooterFieldKey) => void
  insertTab: () => void
  focus: () => void
}

export const ReportFooterEditor = forwardRef<ReportFooterEditorHandle, { value: IntroductionDocument; onChange: (next: IntroductionDocument) => void; onFocus?: () => void }>(({ value, onChange, onFocus }, ref) => {
  const containerRef = useRef<HTMLDivElement>(null)
  const valueRef = useRef(value)
  const changeRef = useRef(onChange)
  useEffect(() => { valueRef.current = value; changeRef.current = onChange }, [value, onChange])
  const editor = useEditor({
    extensions: [StarterKit.configure({ heading: false, blockquote: false, bulletList: false, orderedList: false, code: false, codeBlock: false, horizontalRule: false, hardBreak: false }), FooterField, FooterTab],
    content: footerContent(footerTemplate(value)), immediatelyRender: false,
    onUpdate: ({ editor: updated }) => changeRef.current({ ...valueRef.current, footerText: footerValue(updated.getJSON()), footerLayout: 'custom', footerAlign: valueRef.current.footerAlign ?? 'left' }),
  })
  const setTemplate = (template: string) => {
    const next = { ...valueRef.current, footerLayout: 'custom' as const, footerAlign: 'center' as const, footerText: template }
    valueRef.current = next
    editor?.commands.setContent(footerContent(template), { emitUpdate: false })
    changeRef.current(next)
  }
  const centerFooterField = () => {
    if (typeof document.createRange !== 'function') return
    for (const paragraph of containerRef.current?.querySelectorAll('.tiptap p') ?? []) {
      const tabs = paragraph.querySelectorAll<HTMLElement>('.report-intro-footer-tab')
      if (tabs.length !== 2) continue
      const left = document.createRange()
      left.setStart(paragraph, 0)
      left.setEndBefore(tabs[0])
      const center = document.createRange()
      center.setStartAfter(tabs[0])
      center.setEndBefore(tabs[1])
      if (typeof left.getBoundingClientRect !== 'function') continue
      const gap = Math.max(0, paragraph.getBoundingClientRect().width / 2 - left.getBoundingClientRect().width - center.getBoundingClientRect().width / 2)
      tabs[0].style.flex = `0 0 ${gap}px`
      tabs[1].style.flex = '1 1 auto'
    }
  }
  useLayoutEffect(() => {
    if (!editor || !containerRef.current) return
    centerFooterField()
    const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(centerFooterField)
    observer?.observe(containerRef.current)
    editor.on('update', centerFooterField)
    return () => { observer?.disconnect(); editor.off('update', centerFooterField) }
  }, [editor, value])
  useEffect(() => {
    if (editor && footerValue(editor.getJSON()) !== footerTemplate(value)) editor.commands.setContent(footerContent(footerTemplate(value)), { emitUpdate: false })
  }, [editor, value])
  useImperativeHandle(ref, () => ({
    insertField: key => {
      if ((key === 'page' || key === 'totalPages' || key === 'pageChinese' || key === 'totalPagesChinese') && footerTemplate(valueRef.current).includes(`{${key}}`)) return
      editor?.chain().focus().insertContent({ type: 'reportFooterField', attrs: { key } }).run()
    },
    insertTab: () => editor?.chain().focus().insertContent({ type: 'reportFooterTab' }).run(),
    focus: () => editor?.commands.focus(),
  }), [editor])
  return <div ref={containerRef} className="report-intro-page-footer" aria-label="页尾与页码" onFocusCapture={onFocus}>
    <div className="report-intro-footer-local-tools" role="toolbar" aria-label="页脚快捷工具" onMouseDown={event => event.preventDefault()}>
      <span>页脚</span>
      <Button type="text" size="small" onClick={() => editor?.chain().focus().insertContent({ type: 'reportFooterTab' }).run()}>分区</Button>
      <Dropdown menu={{ items: [{ key: 'page', label: '当前页码' }, { key: 'totalPages', label: '总页数' }, { key: 'pageChinese', label: '当前页码（汉字）' }, { key: 'totalPagesChinese', label: '总页数（汉字）' }, { key: 'semesterName', label: '学期名称' }, { key: 'weekNo', label: '周次数字' }, { key: 'weekNoChinese', label: '周次汉字' }], onClick: ({ key }) => {
        if ((key === 'page' || key === 'totalPages' || key === 'pageChinese' || key === 'totalPagesChinese') && footerTemplate(valueRef.current).includes(`{${key}}`)) return
        editor?.chain().focus().insertContent({ type: 'reportFooterField', attrs: { key } }).run()
      } }} trigger={['click']}><Button type="text" size="small">插入字段</Button></Dropdown>
      <Dropdown menu={{ items: [
        { key: 'number', label: '1' }, { key: 'numberPage', label: '第1页' }, { key: 'numberTotal', label: '第1页，共N页' },
        { key: 'chinesePage', label: '第一页' }, { key: 'chineseTotal', label: '第一页，共N页（汉字）' },
      ], onClick: ({ key }) => {
        const templates: Record<string, string> = { number: '{page}', numberPage: '第{page}页', numberTotal: '第{page}页，共{totalPages}页', chinesePage: '第{pageChinese}页', chineseTotal: '第{pageChinese}页，共{totalPagesChinese}页' }
        setTemplate(templates[key])
      } }} trigger={['click']}><Button type="text" size="small">页码样式</Button></Dropdown>
    </div>
    <EditorContent editor={editor} className="report-intro-footer-content" aria-label="编辑页脚" />
  </div>
})
ReportFooterEditor.displayName = 'ReportFooterEditor'

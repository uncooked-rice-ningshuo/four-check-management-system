import { Icon } from '@iconify/react'
import alignLeft from '@iconify-icons/lucide/align-left'
import alignCenter from '@iconify-icons/lucide/align-center'
import alignRight from '@iconify-icons/lucide/align-right'
import { Node, type JSONContent } from '@tiptap/core'
import { EditorContent, useEditor } from '@tiptap/react'
import StarterKit from '@tiptap/starter-kit'
import { Button } from 'antd'
import { useEffect, useRef } from 'react'
import type { IntroductionDocument } from '../../../shared/api/reportOutputApi'
import { footerTemplate } from '../model/reportFooter'

const PageField = Node.create({
  name: 'reportPageField', group: 'inline', inline: true, atom: true,
  parseHTML: () => [{ tag: 'span[data-report-page-field]' }],
  renderHTML: () => ['span', { 'data-report-page-field': '', class: 'report-intro-page-field', contenteditable: 'false', title: '自动页码' }, '1'],
})
const RightTab = Node.create({ name: 'reportRightTab', group: 'inline', inline: true, atom: true,
  parseHTML: () => [{ tag: 'span[data-report-right-tab]' }],
  renderHTML: () => ['span', { 'data-report-right-tab': '', class: 'report-intro-footer-right-tab', contenteditable: 'false' }],
})

function footerContent(template: string): JSONContent {
  return { type: 'doc', content: template.split('\n').map(line => ({ type: 'paragraph', content: line.split(/(\{page\}|\t)/g).filter(Boolean).map(part => part === '{page}' ? { type: 'reportPageField' } : part === '\t' ? { type: 'reportRightTab' } : { type: 'text', text: part }) })) }
}

function footerValue(json: JSONContent): string {
  return (json.content ?? []).map(paragraph => (paragraph.content ?? []).map(node => node.type === 'reportPageField' ? '{page}' : node.type === 'reportRightTab' ? '\t' : node.type === 'text' ? node.text ?? '' : '').join('')).join('\n')
}

export function ReportFooterEditor({ value, onChange }: { value: IntroductionDocument; onChange: (next: IntroductionDocument) => void }) {
  const valueRef = useRef(value)
  const changeRef = useRef(onChange)
  useEffect(() => { valueRef.current = value; changeRef.current = onChange }, [value, onChange])
  const editor = useEditor({
    extensions: [StarterKit.configure({ heading: false, blockquote: false, bulletList: false, orderedList: false, code: false, codeBlock: false, horizontalRule: false, hardBreak: false }), PageField, RightTab], content: footerContent(footerTemplate(value)), immediatelyRender: false,
    onUpdate: ({ editor: updated }) => changeRef.current({ ...valueRef.current, footerText: footerValue(updated.getJSON()), footerLayout: 'custom', footerAlign: valueRef.current.footerAlign ?? 'center' }),
  })
  useEffect(() => {
    if (editor && footerValue(editor.getJSON()) !== footerTemplate(value)) editor.commands.setContent(footerContent(footerTemplate(value)), { emitUpdate: false })
  }, [editor, value])
  const align = value.footerAlign ?? 'center'
  const setAlign = (footerAlign: 'left' | 'center' | 'right') => onChange({ ...value, footerText: footerTemplate(value), footerLayout: 'custom', footerAlign })
  const pageRight = () => {
    const text = footerTemplate(value).replaceAll('{page}', '').trimEnd().replace(/\t/g, '')
    onChange({ ...value, footerText: `${text}\t{page}`, footerLayout: 'custom', footerAlign: 'left' })
  }
  return <div className="report-intro-page-footer" aria-label="页尾与页码">
    <div className="report-intro-footer-controls" role="toolbar" aria-label="页脚工具">
      <span>页脚</span>
      {(['left', 'center', 'right'] as const).map(direction => <Button key={direction} type="text" size="small" aria-label={`页脚${direction === 'left' ? '左' : direction === 'center' ? '居中' : '右'}对齐`} aria-pressed={align === direction} className={align === direction ? 'active' : ''} icon={<Icon icon={direction === 'left' ? alignLeft : direction === 'center' ? alignCenter : alignRight} />} onClick={() => setAlign(direction)} />)}
      <Button type="text" size="small" disabled={footerTemplate(value).includes('{page}')} onClick={() => editor?.chain().focus().insertContent({ type: 'reportPageField' }).run()}>插入页码</Button>
      <Button type="text" size="small" onClick={pageRight}>页码靠右</Button>
    </div>
    <EditorContent editor={editor} className="report-intro-footer-content" aria-label="编辑页脚" style={{ textAlign: align }} />
  </div>
}

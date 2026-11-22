import { Button, Tooltip } from 'antd'
import { Icon } from '@iconify/react'
import minus from '@iconify-icons/lucide/minus'
import plus from '@iconify-icons/lucide/plus'
import fitWidth from '@iconify-icons/lucide/move-horizontal'
import { EditorContent } from '@tiptap/react'
import type { Transaction } from '@tiptap/pm/state'
import type { Editor } from '@tiptap/core'
import { useId, useLayoutEffect, useRef, useState } from 'react'
import type { WordTemplateDocument } from '../../../shared/api/reportOutputApi'
import { ReportFooterEditor } from './ReportFooterEditor'
import { chineseReportNumber } from '../model/reportNumber'
import { footerTemplate } from '../model/reportFooter'

const pageSize = { portrait: [794, 1123], landscape: [1123, 794] } as const
type Sheet = { orientation: WordTemplateDocument['orientation']; top: number }

// The editor stays in one DOM tree so selecting, dragging and undoing across pages
// retains the same ProseMirror positions as the saved document.
export function TemplatePaper({editor,value,onChange,zoom,discipline,readOnly=false}:{editor:Editor|null;value:WordTemplateDocument;onChange:(doc:WordTemplateDocument)=>void;zoom:number;discipline:boolean;readOnly?:boolean}) {
  // Stored v2 documents may omit orientation. The DOCX renderer defaults to portrait.
  const firstOrientation = value.orientation ?? 'portrait'
  const bodyId=useId()
  const [layout,setLayout]=useState('')
  const revealBreak = useRef(false)
  const host=useRef<HTMLDivElement>(null)
  const [sheets,setSheets]=useState<Sheet[]>([{orientation:firstOrientation,top:0}])
  useLayoutEffect(()=>{
    if(!editor || !host.current)return
    const body=editor.view.dom
    let frame=0
    const measure=()=>{
      const rules:string[]=[]
      const pages:Sheet[]=[{orientation:firstOrientation,top:0}]
      let page=pages[0], used=0, next:Sheet['orientation']|undefined
      for(const block of Array.from(body.children)){
        if(!(block instanceof HTMLElement))continue
        const advance=()=>{page={orientation:next??page.orientation,top:page.top+pageSize[page.orientation][1]+24};pages.push(page);next=undefined;used=0}
        const previousEnd=page.top+used
        if(next)advance()

        const before=Number(block.dataset.spaceBefore||0)*4/3
        const height=block.offsetHeight
        if(used>0 && used+before+height>pageSize[page.orientation][1]-236)advance()
        const margin=page.top+used-previousEnd+before
        const after=Number(block.dataset.spaceAfter||0)*4/3
        const width=pageSize[page.orientation][0]
        const leftInset=(width-144)*Number(block.dataset.insetLeft || 0)/10000
        const rightInset=(width-144)*Number(block.dataset.insetRight || 0)/10000
        rules.push(`[id="${bodyId}"] .tiptap > :nth-child(${rules.length+1}){width:${width-144-leftInset-rightInset}px;margin-top:${margin}px!important;margin-bottom:${after}px!important;margin-left:calc(max(0px,(100cqw - ${width}px)/2) + ${leftInset}px)}`)
        used+=before+height+after
        if(block.dataset.wordNode==='pageBreak')next=page.orientation
        if(block.dataset.wordNode==='sectionBreak')next=block.dataset.wordValue==='landscape'?'landscape':'portrait'
      }
      setLayout(rules.join('\n'))
      setSheets(old=>JSON.stringify(old)===JSON.stringify(pages)?old:pages)
    }
    const schedule=()=>{cancelAnimationFrame(frame);frame=requestAnimationFrame(measure)}
    measure()
    const onTransaction = ({transaction}:{transaction:Transaction}) => {
      if (transaction.getMeta('revealTemplateBreak')) revealBreak.current = true
      if (transaction.docChanged) schedule()
    }
    editor.on('transaction',onTransaction)
    const observer=new ResizeObserver(schedule);observer.observe(body)
    return ()=>{editor.off('transaction',onTransaction);observer.disconnect();cancelAnimationFrame(frame)}
  },[editor,firstOrientation,bodyId])
  // ProseMirror's initial scroll precedes the page margins applied by React.
  useLayoutEffect(() => {
    if (!editor || !revealBreak.current) return
    revealBreak.current = false
    const cursor = editor.state.selection.$from
    if (cursor.depth && editor.view.hasFocus()) {
      const paragraph = editor.view.nodeDOM(cursor.before())
      if (paragraph instanceof HTMLElement) paragraph.scrollIntoView({block:'nearest', inline:'nearest'})
    }
  }, [editor,layout,sheets])
  const [availableWidth,setAvailableWidth] = useState(0)
  const [manualZoom,setManualZoom] = useState<number>()
  useLayoutEffect(() => {
    const stage = host.current
    if (!readOnly || !stage) return
    const observer = new ResizeObserver(([entry]) => setAvailableWidth(entry.contentRect.width))
    observer.observe(stage)
    return () => observer.disconnect()
  }, [readOnly])
  const paperWidth = Math.max(...sheets.map(sheet=>pageSize[sheet.orientation][0]))
  const fit = availableWidth ? Math.max(1, Math.floor(100 * availableWidth / paperWidth)) : 100
  const scale = readOnly ? manualZoom ?? fit : zoom
  const last=sheets[sheets.length-1]
  const paper = <div className="report-intro-stage" ref={host} aria-label="模板纸张工作区">
    <style>{layout}</style>
    <div className="report-intro-pages template-pages" style={{zoom:scale/100,width:paperWidth,height:last.top+pageSize[last.orientation][1]}}>
      {sheets.map(({orientation,top},i)=><div key={i} className="report-intro-page" aria-label={`第 ${i+1} 页，${orientation==='portrait'?'纵向':'横向'}`} style={{top,left:`max(0px,calc((100cqw - ${pageSize[orientation][0]}px)/2))`,width:pageSize[orientation][0],height:pageSize[orientation][1]}}>
        <div className="report-intro-page-corners" aria-hidden="true"><i/><i/><i/><i/></div>
        {i===0 && !readOnly ? <>
          <input className="report-intro-page-header" aria-label="编辑页首" placeholder="点击编辑页首" maxLength={120} value={value.headerText??''} onChange={e=>onChange({...value,headerText:e.target.value})}/>
          <ReportFooterEditor reportFields={!discipline} value={{...value,content:[]}} onChange={footer=>onChange({...value,footerText:footer.footerText,footerLayout:footer.footerLayout,footerAlign:footer.footerAlign})}/>
        </>:<>
          <div className="report-intro-page-header">{value.headerText}</div>
          <div className="report-intro-page-footer" aria-label="页尾与页码" style={{whiteSpace:'pre-wrap',textAlign:value.footerAlign ?? 'center'}}>
            {footerTemplate(value).replaceAll('{page}',String(i+1)).replaceAll('{totalPages}',String(sheets.length))
              .replaceAll('{pageChinese}',chineseReportNumber(i+1)).replaceAll('{totalPagesChinese}',chineseReportNumber(sheets.length))}
          </div>
        </>}
      </div>)}
      <EditorContent id={bodyId} editor={editor} className="report-intro-content template-body" data-context={discipline?'discipline':'report'}/>
    </div>
  </div>
  if (!readOnly) return paper
  return <div className="template-preview-viewport">
    {paper}
    <div className="template-preview-controls" role="group" aria-label="预览缩放">
      <Tooltip title="缩小预览" trigger={['hover','focus']}><Button type="text" aria-label="缩小预览" disabled={scale<=25}
        icon={<Icon icon={minus}/>} onClick={()=>setManualZoom(Math.max(25,scale-10))}/></Tooltip>
      <span aria-live="polite">{scale}%</span>
      <Tooltip title="放大预览" trigger={['hover','focus']}><Button type="text" aria-label="放大预览" disabled={scale>=200}
        icon={<Icon icon={plus}/>} onClick={()=>setManualZoom(Math.min(200,scale+10))}/></Tooltip>
      <Tooltip title="适应宽度" trigger={['hover','focus']}><Button type="text" aria-label="适应宽度" aria-pressed={manualZoom===undefined}
        icon={<Icon icon={fitWidth}/>} onClick={()=>setManualZoom(undefined)}/></Tooltip>
    </div>
  </div>
}

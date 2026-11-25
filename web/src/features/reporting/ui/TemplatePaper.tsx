import { Button, Tooltip } from 'antd'
import { Icon } from '@iconify/react'
import minus from '@iconify-icons/lucide/minus'
import plus from '@iconify-icons/lucide/plus'
import fitWidth from '@iconify-icons/lucide/move-horizontal'
import { EditorContent } from '@tiptap/react'
import { Plugin, PluginKey, type Transaction } from '@tiptap/pm/state'
import { Decoration, DecorationSet } from '@tiptap/pm/view'
import type { Node as ProseMirrorNode } from '@tiptap/pm/model'
import type { Editor } from '@tiptap/core'
import { type CSSProperties, useId, useLayoutEffect, useRef, useState } from 'react'
import type { WordTemplateDocument } from '../../../shared/api/reportOutputApi'
import { ReportFooterEditor } from './ReportFooterEditor'
import { chineseReportNumber } from '../model/reportNumber'
import { footerAlignment, footerTemplate } from '../model/reportFooter'

const pageSize = { portrait: [794, 1123], landscape: [1123, 794] } as const
type Sheet = { orientation: WordTemplateDocument['orientation']; top: number }

const glyphLayoutKey = new PluginKey<DecorationSet>('wordGlyphLayout')
const words = new Intl.Segmenter('zh', {granularity:'word'})
const graphemes = new Intl.Segmenter(undefined, {granularity:'grapheme'})

function scaledGlyphs(doc: ProseMirrorNode) {
  const pieces: Decoration[] = []
  doc.descendants((node, pos) => {
    if (!node.isText || !node.text) return
    const attrs = node.marks.find(mark => mark.type.name === 'textStyle')?.attrs
    const scale = Number(attrs?.scale)
    if (!(scale > 0) || scale === 100) return
    // Keep joined scripts and Latin words shaped together, and allow CJK
    // wrapping at grapheme boundaries. Word adds tracking after width scaling.
    const style = `--word-glyph-scale:${scale/100};--word-glyph-inverse:${100/scale};letter-spacing:${Number(attrs?.spacing || 0)*5/scale}pt`
    for (const word of words.segment(node.text)) {
      const start = pos + word.index
      const cjk = /[\p{Script=Han}\p{Script=Hiragana}\p{Script=Katakana}\p{Script=Hangul}]/u.test(word.segment)
      const segments = cjk ? graphemes.segment(word.segment) : [{index:0,segment:word.segment}]
      for (const {index,segment} of segments) {
        pieces.push(Decoration.inline(start+index,start+index+segment.length,{class:'word-scaled-glyph',style}))
      }
    }
  })
  return DecorationSet.create(doc,pieces)
}

// In Word's wrapped 12 pt FangSong sample the baseline interval is 15.6 pt.
function paragraphLineMetrics(block:HTMLElement):string {
  if(block.tagName!=='P')return ''
  const mode=block.dataset.lineSpacingMode
  const automatic=mode==='single' || mode==='oneHalf' || mode==='double' || (block.dataset.style==='body' && mode==='fixed' && block.dataset.lineSpacing==='0' && block.closest('[data-context="discipline"]'))
  if(!automatic)return ''
  const calibratedFont=(el:HTMLElement)=>/^(FangSong|仿宋|SimSun|宋体)/i.test(getComputedStyle(el).fontFamily)
  // Do not let the larger paragraph strut determine every wrapped line when
  // all text runs have an explicit size. Unmarked text keeps its body size.
  const sizes=Array.from(block.childNodes,run=>run instanceof HTMLElement && run.style.fontSize ? parseFloat(getComputedStyle(run).fontSize) : 0)
  const base=sizes.length && sizes.every(size=>size>0) ? 'font-size:'+Math.min(...sizes)+'px!important;' : ''
  const runs=Array.from(block.childNodes)
  const families=runs.map(run=>run instanceof HTMLElement ? run.style.fontFamily : '')
  const uniformFamily=families[0] && families.every(family=>family===families[0]) ? `font-family:${families[0]};` : ''
  const entirelyBold=runs.length>0 && runs.every(run=>run instanceof HTMLElement && (run.tagName==='STRONG' ||
    (run.childNodes.length===1 && run.firstElementChild?.tagName==='STRONG')))
  const effectiveFont=uniformFamily && runs[0] instanceof HTMLElement ? runs[0] : block
  if(!calibratedFont(effectiveFont) || !Array.from(block.querySelectorAll<HTMLElement>('[style*="font-family"]')).every(calibratedFont))return base+uniformFamily
  const multiple = mode==='oneHalf' ? 1.5 : mode==='double' ? 2 : 1
  return base+uniformFamily+(entirelyBold?'font-weight:bold;':'')+`--word-single-line:1.3;line-height:${1.3*multiple}!important;`
}

// Transformed text can paint beyond its line box. Reserve only the part
// outside the paragraph, using the active browser's font and preview zoom.
function scaledTextInsets(block: HTMLElement, style: CSSStyleDeclaration): string {
  if (block.tagName !== 'P' || !block.querySelector('.word-scaled-glyph')) return ''
  if (block.dataset.lineSpacingMode === 'fixed' && Number(block.dataset.lineSpacing) > 0) return ''
  const bounds = block.getBoundingClientRect()
  const zoom = bounds.width / parseFloat(style.width)
  if (!(zoom > 0)) return ''
  const range = document.createRange()
  range.selectNodeContents(block)
  const text = range.getBoundingClientRect()
  const top = Math.max(0, Math.ceil(parseFloat(style.paddingTop) + (bounds.top - text.top) / zoom))
  const bottom = Math.max(0, Math.ceil(parseFloat(style.paddingBottom) + (text.bottom - bounds.bottom) / zoom))
  return `padding-top:${top}px;padding-bottom:${bottom}px;`
}

// The editor stays in one DOM tree so selecting, dragging and undoing across pages
// retains the same ProseMirror positions as the saved document.
export function TemplatePaper({editor,value,onChange,zoom,discipline,readOnly=false}:{editor:Editor|null;value:WordTemplateDocument;onChange:(doc:WordTemplateDocument)=>void;zoom:number;discipline:boolean;readOnly?:boolean}) {
  // Stored v2 documents may omit orientation. The DOCX renderer defaults to portrait.
  const firstOrientation = value.orientation ?? 'portrait'
  const marginPx = value.marginTwips ? value.marginTwips / 15 : undefined
  const top = value.margins ? value.margins.top/15 : marginPx ?? 96
  const bottom = value.margins ? value.margins.bottom/15 : marginPx ?? 140
  const left = value.margins ? value.margins.left/15 : marginPx ?? 72
  const horizontal = left + (value.margins ? value.margins.right/15 : marginPx ?? 72)
  const bodyId=useId()
  const [layout,setLayout]=useState('')
  const revealBreak = useRef(false)
  const host=useRef<HTMLDivElement>(null)
  const [sheets,setSheets]=useState<Sheet[]>([{orientation:firstOrientation,top:0}])
  useLayoutEffect(()=>{
    if(!editor || !host.current)return
    editor.registerPlugin(new Plugin({
      key:glyphLayoutKey,
      state:{init:(_,state)=>scaledGlyphs(state.doc),apply:(tr,previous)=>tr.docChanged?scaledGlyphs(tr.doc):previous},
      props:{decorations:state=>glyphLayoutKey.getState(state)},
    }))
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
        if(next || (used>0 && block.dataset.pageBefore==='true'))advance()

        const style=getComputedStyle(block)
        const lineHeight=parseFloat(style.lineHeight)||parseFloat(style.fontSize)*1.3
        const before=block.dataset.beforeLines && Number(block.dataset.beforeLines)>0 ? Number(block.dataset.beforeLines)*lineHeight/100 : Number(block.dataset.spaceBefore||0)*4/3
        const height=block.offsetHeight
        const nextBlock=block.nextElementSibling
        const followingHeight=block.dataset.keepNext==='true' && nextBlock instanceof HTMLElement ? nextBlock.offsetHeight : 0
        if(used>0 && used+before+height+followingHeight>pageSize[page.orientation][1]-(top+bottom))advance()
        const margin=page.top+used-previousEnd+before
        const after=Number(block.dataset.afterLines||0)>0 ? Number(block.dataset.afterLines)*lineHeight/100 : Number(block.dataset.spaceAfter||0)*4/3
        const width=pageSize[page.orientation][0]
        const leftInset=(width-horizontal)*Number(block.dataset.insetLeft || 0)/10000
        const rightInset=(width-horizontal)*Number(block.dataset.insetRight || 0)/10000
        rules.push(`[id="${bodyId}"] .tiptap > :nth-child(${rules.length+1}){${paragraphLineMetrics(block)}${scaledTextInsets(block,style)}width:${width-horizontal-leftInset-rightInset}px;margin-top:${margin}px!important;margin-bottom:${after}px!important;margin-left:calc(max(0px,(100cqw - ${width}px)/2) + ${leftInset}px)}`)
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
    return ()=>{editor.unregisterPlugin(glyphLayoutKey);editor.off('transaction',onTransaction);observer.disconnect();cancelAnimationFrame(frame)}
  },[editor,firstOrientation,bodyId,horizontal,top,bottom])
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
    <div className="report-intro-pages template-pages" style={{zoom:scale/100,width:paperWidth,height:last.top+pageSize[last.orientation][1],
      '--word-margin-top':top+'px','--word-margin-bottom':bottom+'px','--word-margin-left':left+'px','--word-margin-right':(horizontal-left)+'px',
      '--word-header-offset':Math.floor(top*15/2)/15+'px','--word-footer-offset':Math.floor(bottom*15/2)/15+'px',
    } as CSSProperties}>
      {sheets.map(({orientation,top},i)=><div key={i} className="report-intro-page" aria-label={`第 ${i+1} 页，${orientation==='portrait'?'纵向':'横向'}`} style={{top,left:`max(0px,calc((100cqw - ${pageSize[orientation][0]}px)/2))`,width:pageSize[orientation][0],height:pageSize[orientation][1]}}>
        <div className="report-intro-page-corners" aria-hidden="true"><i/><i/><i/><i/></div>
        {i===0 && !readOnly ? <>
          <input className="report-intro-page-header" aria-label="编辑页首" placeholder="点击编辑页首" maxLength={120} value={value.headerText??''} onChange={e=>onChange({...value,headerText:e.target.value})}/>
          <ReportFooterEditor reportFields={!discipline} value={{...value,content:[]}} onChange={footer=>onChange({...value,footerText:footer.footerText,footerLayout:footer.footerLayout,footerAlign:footer.footerAlign})}/>
        </>:<>
          <div className="report-intro-page-header">{value.headerText}</div>
          <div className="report-intro-page-footer" aria-label="页尾与页码" style={{whiteSpace:'pre-wrap',textAlign:footerAlignment(value)}}>
            {footerTemplate(value).replaceAll('{page}',String(i+1)).replaceAll('{totalPages}',String(sheets.length))
              .replaceAll('{pageChinese}',chineseReportNumber(i+1)).replaceAll('{totalPagesChinese}',chineseReportNumber(sheets.length))}
          </div>
        </>}
      </div>)}
      <EditorContent style={{top,left,width:paperWidth-horizontal}} id={bodyId} editor={editor} className="report-intro-content template-body" data-context={discipline?'discipline':'report'}/>
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

import { Button, Checkbox, InputNumber, Modal, Radio, Select, Tabs } from 'antd'
import { Icon } from '@iconify/react'
import rows from '@iconify-icons/lucide/rows-3'
import type { Editor } from '@tiptap/core'
import { useState, type CSSProperties } from 'react'

const defaults = {
  tabStop:0, textAlign:'left', outline:0, rtl:false, leftChars:0, rightChars:0, indent:0, hanging:0,
  spaceBefore:0, spaceAfter:0, beforeLines:0, afterLines:0, lineSpacingMode:'fixed', lineSpacing:27,
  gridIndent:true, paragraphGrid:true, keepNext:false, keepLines:false, pageBefore:false, widow:true,
}
type Settings=typeof defaults
const options=(entries: Record<string,string>)=>Object.entries(entries).map(([value,label])=>({value,label}))
const alignment=options({left:'左对齐',center:'居中',right:'右对齐',justify:'两端对齐'})
const lineModes=options({single:'单倍行距',oneHalf:'1.5 倍行距',double:'双倍行距',fixed:'固定值'})

export function ParagraphSettings({editor}:{editor:Editor|null}){
  const [draft,setDraft]=useState<Settings>()
  const [lineUnits,setLineUnits]=useState({before:false,after:false})
  const [special,setSpecial]=useState('none')
  const [showTabs,setShowTabs]=useState(false)
  const [changed,setChanged]=useState<Partial<Settings>>({})
  const update=(patch:Partial<Settings>)=>{
    setDraft(previous=>previous && {...previous,...patch})
    setChanged(previous=>({...previous,...patch}))
  }
  const open=()=>{
    if(!editor)return
    const attrs=editor.getAttributes('paragraph')
    const current={...defaults}
    for(const key of Object.keys(defaults) as (keyof Settings)[]){
      if(attrs[key]!=null)Object.assign(current,{[key]:attrs[key]})
    }
    current.lineSpacing ||= attrs.style==='title'?40:attrs.style==='subtitle'?28:27
    setDraft(current);setChanged({});setShowTabs(false)
    setSpecial(current.hanging?'hanging':current.indent?'first':'none')
    setLineUnits({before:!!current.beforeLines,after:!!current.afterLines})
  }
  const trigger=<Button type="text" className="report-intro-tool" icon={<Icon icon={rows}/>} aria-label="段落间距" title="段落设置" disabled={!editor?.isActive('paragraph')} onClick={open}/>
  if(!draft)return trigger
  const check=(key:'gridIndent'|'paragraphGrid'|'keepNext'|'keepLines'|'pageBefore'|'widow',label:string)=><Checkbox checked={draft[key]} onChange={e=>update({[key]:e.target.checked})}>{label}</Checkbox>
  const number=(name:string,value:number,max:number,onChange:(n:number)=>void,step=1,min=0,disabled=false)=><InputNumber size="small" aria-label={name} value={value} min={min} max={max} step={step} precision={step===1?0:2} disabled={disabled} onChange={n=>n!==null && onChange(n)}/>
  const spacing=(before:boolean)=>{
    const side=before?'before':'after', key=before?'spaceBefore':'spaceAfter', lines=before?'beforeLines':'afterLines'
    const label=before?'段前':'段后', inLines=lineUnits[side]
    return <label>{label}{number(label+'间距',inLines?draft[lines]/100:draft[key],inLines?10:72,n=>update(inLines?{[lines]:Math.round(n*100),[key]:0}:{[key]:n,[lines]:0}),inLines?0.1:1)}
      <Select size="small" aria-label={label+'单位'} value={inLines?'行':'磅'} options={options({'行':'行','磅':'磅'})} onChange={v=>{
        setLineUnits({...lineUnits,[side]:v==='行'})
        update(v==='行'?{[lines]:Math.min(1000,Math.round(draft[key]/draft.lineSpacing*100)),[key]:0}:{[key]:Math.min(72,Math.round(draft[lines]/100*draft.lineSpacing)),[lines]:0})
      }}/></label>
  }
  const preview:CSSProperties={textAlign:draft.textAlign as CSSProperties['textAlign'],direction:draft.rtl?'rtl':'ltr',
    paddingInlineStart:draft.leftChars+'em',paddingInlineEnd:draft.rightChars+'em',textIndent:(draft.indent-draft.hanging)+'em',
    marginBlockStart:draft.beforeLines?draft.beforeLines/100+'lh':draft.spaceBefore+'pt',marginBlockEnd:draft.afterLines?draft.afterLines/100+'lh':draft.spaceAfter+'pt',
    lineHeight:draft.lineSpacingMode==='fixed'?draft.lineSpacing+'pt':draft.lineSpacingMode==='single'?1.3:draft.lineSpacingMode==='double'?2.6:1.95}
  return <>{trigger}<Modal open title="段落" width={520} className="word-settings-dialog word-paragraph-dialog" cancelText="取消" okText="确定" okButtonProps={{'aria-label':'确定段落设置'}}
    onCancel={()=>setDraft(undefined)} onOk={()=>{if(Object.keys(changed).length)editor?.chain().focus().updateAttributes('paragraph',changed).run();setDraft(undefined)}}>
    <Tabs size="small" items={[
      {key:'spacing',label:'缩进和间距',children:<div className="word-paragraph-form">
        <fieldset><legend>常规</legend><label>对齐方式<Select size="small" aria-label="对齐方式" value={draft.textAlign} options={alignment} onChange={textAlign=>update({textAlign})}/></label>
        <label>大纲级别<Select size="small" aria-label="大纲级别" value={draft.outline} options={Array.from({length:10},(_,value)=>({value,label:value?value+' 级':'正文文本'}))} onChange={outline=>update({outline})}/></label>
        <div className="wide">方向 <Radio.Group size="small" value={draft.rtl} options={[{value:false,label:'从左向右'},{value:true,label:'从右向左'}]} onChange={e=>update({rtl:e.target.value})}/></div></fieldset>
        <fieldset><legend>缩进</legend><label>文本之前{number('文本之前',draft.leftChars,16,leftChars=>update({leftChars}))}字符</label>
        <label>特殊格式<Select size="small" aria-label="特殊格式" value={special} options={options({none:'无',first:'首行缩进',hanging:'悬挂缩进'})} onChange={v=>{setSpecial(v);update({indent:v==='first'?draft.indent||2:0,hanging:v==='hanging'?draft.hanging||2:0})}}/></label>
        <label>文本之后{number('文本之后',draft.rightChars,16,rightChars=>update({rightChars}))}字符</label>
        <label>度量值{number('缩进度量值',draft.indent||draft.hanging,special==='first'?4:16,n=>update(special==='first'?{indent:n}:{hanging:n}),1,0,special==='none')}字符</label>
        <div className="wide">{check('gridIndent','定义文档网格时，自动调整右缩进')}</div></fieldset>
        <fieldset><legend>间距</legend>{spacing(true)}<label>行距<Select size="small" aria-label="行距方式" options={lineModes} value={draft.lineSpacingMode} onChange={v=>update(v==='fixed'?{lineSpacingMode:v,lineSpacing:draft.lineSpacing}:{lineSpacingMode:v})}/></label>
        {spacing(false)}<label>设置值{number('固定行距',draft.lineSpacing,48,lineSpacing=>update({lineSpacing}),1,18,draft.lineSpacingMode!=='fixed')}磅</label>
        <div className="wide">{check('paragraphGrid','定义文档网格时，与网格对齐')}</div></fieldset></div>},
      {key:'page',label:'换行和分页',children:<div className="word-pagination">{check('widow','孤行控制')}{check('keepNext','与下段同页')}{check('keepLines','段中不分页')}{check('pageBefore','段前分页')}</div>},
    ]}/>
    <button type="button" className="word-tabstop-toggle" aria-expanded={showTabs} onClick={()=>setShowTabs(!showTabs)}>制表位 {showTabs?'▴':'▾'}</button>
    {showTabs && <label className="word-tabstop-field">左对齐制表位 {number('左对齐制表位',draft.tabStop/20,420,n=>update({tabStop:Math.round(n*20)}),0.5)} 磅 <Button type="text" size="small" onClick={()=>update({tabStop:0})}>清除</Button></label>}
    <figure className="word-font-preview"><figcaption>预览</figcaption><section className="word-paragraph-preview"><p style={preview}>{editor?.state.selection.$from.parent.textContent.slice(0,100)||'本段文字展示所选的对齐、缩进和行距效果。调整设置后，可在这里查看段落排版。'}</p></section></figure>
  </Modal></>
}

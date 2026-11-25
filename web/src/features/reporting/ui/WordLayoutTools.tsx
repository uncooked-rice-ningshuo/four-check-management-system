import { Icon } from '@iconify/react'
import baseline from '@iconify-icons/lucide/baseline'
import chevronDown from '@iconify-icons/lucide/chevron-down'
import frame from '@iconify-icons/lucide/frame'
import moveHorizontal from '@iconify-icons/lucide/move-horizontal'
import { Button, Checkbox, ColorPicker, Dropdown, InputNumber, Modal, Select } from 'antd'
import { useState } from 'react'
import type { Editor } from '@tiptap/core'
import type { WordTemplateDocument } from '../../../shared/api/reportOutputApi'

type Margins = NonNullable<WordTemplateDocument['margins']>
const edges = { top: '上', bottom: '下', left: '左', right: '右' } as const
const presets = [
  {name:'普通', top:1440, bottom:1440, left:1800, right:1800},
  {name:'窄', top:720, bottom:720, left:720, right:720},
  {name:'适中', top:1440, bottom:1440, left:1080, right:1080},
  {name:'宽', top:1440, bottom:1440, left:2880, right:2880},
]
const cm = (twips:number) => Number((twips*2.54/1440).toFixed(2))

export function PageMarginTool({value,onChange}:{value:WordTemplateDocument;onChange:(doc:WordTemplateDocument)=>void}) {
  const [draft,setDraft] = useState<Margins>()
  const apply = (margins:Margins) => {onChange({...value,marginTwips:undefined,margins});setDraft(undefined)}
  const edge = value.marginTwips ?? 900
  return <>
    <Dropdown trigger={['click']} menu={{items:[
      ...presets.map(({name,top,bottom,left,right})=>({key:name,label:<div className="word-margin-preset"><strong>{name}</strong><small>上下 {cm(top)} · 左右 {cm(left)} 厘米</small></div>,onClick:()=>apply({top,bottom,left,right})})),
      {key:'custom',label:'自定义页边距…',onClick:()=>setDraft(value.margins ?? {top:edge,bottom:edge,left:edge,right:edge})},
    ]}}><Button type="text" className="report-intro-tool" aria-label="页边距" title="页边距" icon={<Icon icon={frame}/>}/></Dropdown>
    {draft && <Modal open title="自定义页边距" className="word-settings-dialog word-margins-dialog" width={320} cancelText="取消" okText="应用" okButtonProps={{"aria-label":"应用"}} onCancel={()=>setDraft(undefined)} onOk={()=>apply(draft)}>
      <div className="word-setting-grid">{(Object.keys(edges) as (keyof Margins)[]).map(side=><label key={side}>{edges[side]}
        <InputNumber size="small" aria-label={edges[side]+'边距'} min={0} max={7.93} precision={2} step={0.1} suffix="厘米" value={cm(draft[side])} onChange={n=>n!==null && setDraft({...draft,[side]:Math.round(n*1440/2.54)})}/>
      </label>)}</div></Modal>}
  </>
}

const palette = [
  ['深红','#c00000'],['红色','#ff0000'],['橙色','#ffc000'],['黄色','#ffff00'],['浅绿','#92d050'],
  ['绿色','#00b050'],['浅蓝','#00b0f0'],['蓝色','#0070c0'],['深蓝','#002060'],['紫色','#7030a0'],
]
type CharacterSettings = {scale:number; spacing:number; position:number; kern:number; snap:boolean}

export function CharacterTools({editor,color}:{editor:Editor|null;color?:string}) {
  const [draft,setDraft] = useState<CharacterSettings>()
  const [units,setUnits] = useState({spacing:"磅",position:"磅"})
  const [colorOpen,setColorOpen] = useState(false)
  const applyColor = (next:string|null) => {
    editor?.chain().focus().setMark('textStyle',{color:next}).removeEmptyTextStyle().run()
    setColorOpen(false)
  }
  const openSettings = () => {
    const attrs=editor?.getAttributes('textStyle') ?? {}
    setDraft({scale:attrs.scale??100,spacing:attrs.spacing??0,position:attrs.position??0,kern:attrs.kern??0,snap:attrs.snap??true})
  }
  return <>
    <ColorPicker open={colorOpen} onOpenChange={setColorOpen} value={color || '#000000'} disabledAlpha format="hex"
      presets={[]} onChangeComplete={c=>applyColor(c.toHexString())}
      panelRender={panel=><div className="word-color-panel"><strong>标准颜色</strong><div className="word-palette">{palette.map(([name,hex])=><button type="button" title={name} aria-label={name} aria-pressed={color?.toLowerCase()===hex} key={hex} style={{background:hex}} onClick={()=>applyColor(hex)}/>)}</div><details className="word-color-disclosure"><summary>自定义颜色<Icon icon={chevronDown} aria-hidden="true"/></summary>{panel}</details><Button type="text" block onClick={()=>applyColor(null)}>自动颜色</Button></div>}>
      <Button type="text" className="report-intro-tool" aria-label="文字颜色" title="文字颜色" icon={<span className="word-color-icon"><Icon icon={baseline}/><i style={{backgroundColor:color || '#000000'}}/></span>}/>
    </ColorPicker>
    <Button type="text" className="report-intro-tool" aria-label="字符间距" title="字符间距" icon={<Icon icon={moveHorizontal}/>} onClick={openSettings}/>
    {draft && <Modal open title="字符间距" className="word-settings-dialog word-character-dialog" width={356} cancelText="取消" okText="应用" okButtonProps={{"aria-label":"应用"}} onCancel={()=>setDraft(undefined)} onOk={()=>{
      editor?.chain().focus().setMark('textStyle',{scale:draft.scale===100?null:draft.scale,spacing:draft.spacing||null,position:draft.position||null,kern:draft.kern||null,snap:draft.snap}).run()
      setDraft(undefined)
    }}><div className="word-character-panel">
      <label>缩放<InputNumber size="small" aria-label="字符缩放" min={1} max={600} precision={0} suffix="%" value={draft.scale} onChange={n=>n && setDraft({...draft,scale:n})}/></label>
      {(['spacing','position'] as const).map(key=>{
        const spacing = key==='spacing', title=spacing?'间距':'位置', baseUnit=spacing?20:2, unit=baseUnit*(units[key]==='厘米'?72/2.54:1)
        return <label key={key}>{title}<Select size="small" aria-label={title} value={Math.sign(draft[key])}
          options={[{value:0,label:'标准'},{value:1,label:spacing?'加宽':'提升'},{value:-1,label:spacing?'紧缩':'降低'}]}
          onChange={sign=>setDraft({...draft,[key]:sign*Math.min(Math.abs(draft[key])||baseUnit,spacing && sign<0?100:200)})}/>
          <InputNumber size="small" aria-label={title+'值'} disabled={!draft[key]} min={!draft[key]?0:units[key]==="厘米"?0.01:1/baseUnit} max={(spacing?(draft[key]<0?5:10):100)*baseUnit/unit} precision={2} step={units[key]==="厘米"?0.01:1/baseUnit} value={Math.abs(draft[key])/unit}
            onChange={n=>n && setDraft({...draft,[key]:Math.sign(draft[key])*Math.max(1,Math.round(n*unit))})}/><Select size="small" aria-label={title+"单位"} value={units[key]} options={["磅","厘米"].map(value=>({value,label:value}))} onChange={value=>setUnits({...units,[key]:value})}/></label>
      })}
      <div className="word-character-options"><Checkbox checked={draft.kern>0} onChange={e=>setDraft({...draft,kern:e.target.checked?2:0})}>为字体调整字间距</Checkbox>
        <InputNumber size="small" aria-label="字偶距起始字号" min={draft.kern?0.5:0} max={163} step={0.5} disabled={!draft.kern} value={draft.kern/2} onChange={n=>n && setDraft({...draft,kern:Math.round(n*2)})}/> 磅或更大</div>
      <Checkbox checked={draft.snap} onChange={e=>setDraft({...draft,snap:e.target.checked})}>如果定义了文档网格，则对齐网格</Checkbox>
      <figure className="word-font-preview"><figcaption>效果预览</figcaption><div><span style={{transform:`scaleX(${draft.scale/100}) translateY(${-draft.position/2}pt)`,letterSpacing:draft.spacing/20+'pt',fontKerning:draft.kern>0 && draft.kern<=32?'normal':'none',color}}>{editor?.state.doc.textBetween(editor.state.selection.from,editor.state.selection.to,' ').slice(0,22)||'计算机学院 · Word'}</span></div></figure>
    </div></Modal>}
  </>
}

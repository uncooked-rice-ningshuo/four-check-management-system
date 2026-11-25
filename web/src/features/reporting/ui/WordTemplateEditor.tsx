import rowsIcon from '@iconify-icons/lucide/rows-2'
import columnsIcon from '@iconify-icons/lucide/columns-2'
import trashIcon from '@iconify-icons/lucide/trash-2'
import alignLeft from '@iconify-icons/lucide/align-left'
import alignCenter from '@iconify-icons/lucide/align-center'
import alignRight from '@iconify-icons/lucide/align-right'
import alignTop from '@iconify-icons/lucide/align-vertical-justify-start'
import alignMiddle from '@iconify-icons/lucide/align-vertical-justify-center'
import alignBottom from '@iconify-icons/lucide/align-vertical-justify-end'
import ruler from '@iconify-icons/lucide/ruler'
import { applyCellLayout, readCellLayout, type CellLayout, newTable, tableForEditor, tableForStorage, wordCell, wordRow, wordTable } from '../model/wordTable'
import {addColumnAfter,addRowAfter,deleteColumn,deleteRow,deleteTable,setCellAttr} from '@tiptap/pm/tables'
import { disciplineFields } from '../model/disciplineFields'
import { Icon } from '@iconify/react'
import minus from '@iconify-icons/lucide/minus'
import pageBreak from '@iconify-icons/lucide/separator-horizontal'
import sectionBreak from '@iconify-icons/lucide/panels-top-left'
import { chineseReportNumber } from '../model/reportNumber'
import plus from '@iconify-icons/lucide/plus'
import undo from '@iconify-icons/lucide/undo-2'
import redo from '@iconify-icons/lucide/redo-2'
import braces from '@iconify-icons/lucide/braces'
import table from '@iconify-icons/lucide/table'
import { TemplatePaper } from './TemplatePaper'
import { TextSelection } from '@tiptap/pm/state'
import { closeHistory } from '@tiptap/pm/history'
import type { DOMOutputSpec } from '@tiptap/pm/model'
import { Extension, Node, type JSONContent } from '@tiptap/core'
import { useEditor, useEditorState } from '@tiptap/react'
import { Button, Dropdown, Select, Checkbox, InputNumber, Modal, Space } from 'antd'
import { useLayoutEffect, useRef, useState } from 'react'
import type { IntroductionDocument, ReportOutputType, WordTemplateDocument } from '../../../shared/api/reportOutputApi'
import { introductionExtensions, ToolButton, WordFormattingControls } from './ReportIntroductionEditor'

import { CharacterTools, PageMarginTool } from './WordLayoutTools'

const disciplineRows = Extension.create({
  name: 'disciplineRows',
  addGlobalAttributes: () => [{types:['paragraph'],attributes:{letterhead:{
    default:false,keepOnSplit:false,
    parseHTML: element => element.hasAttribute('data-letterhead'),
    renderHTML: attrs => attrs.letterhead ? {'data-letterhead':'true'} : {},
  }}}],
})

const characterFormatting = Extension.create({
  name: 'wordCharacterFormatting',
  addGlobalAttributes: () => [{ types: ['textStyle'], attributes: {
    scale: {default:null, parseHTML:el=>Number(el.dataset.wordScale)||null,renderHTML:a=>a.scale?{'data-word-scale':a.scale,style:`--word-glyph-scale:${a.scale/100};--word-glyph-inverse:${100/a.scale}`}:{}},
    position: {default:null,parseHTML:el=>Number(el.dataset.wordPosition)||null,renderHTML:a=>a.position?{'data-word-position':a.position,style:`vertical-align:${a.position/2}pt`}:{}},
    kern: {default:null,parseHTML:el=>Number(el.dataset.wordKern)||null,renderHTML:a=>a.kern?{'data-word-kern':a.kern,style:`font-kerning:${parseFloat(a.fontSize||'14')>=a.kern/2?'normal':'none'}`}:{}},
    snap: {default:null,parseHTML:el=>el.hasAttribute('data-word-grid')?el.dataset.wordGrid==='true':null,renderHTML:a=>a.snap==null?{}:{'data-word-grid':String(a.snap)}},
    color: { default: null, parseHTML: el => el.dataset.wordColor?.match(/^#[0-9a-f]{6}$/i)?.[0] ?? null,
      renderHTML: attrs => attrs.color ? { 'data-word-color': attrs.color, style: `color:${attrs.color}` } : {} },
    spacing: { default: null, parseHTML: el => {
      const spacing = Number(el.dataset.wordSpacing)
      return Number.isInteger(spacing) && spacing >= -100 && spacing <= 200 && spacing !== 0 ? spacing : null
    }, renderHTML: attrs => attrs.spacing ? { 'data-word-spacing': attrs.spacing, style: `--word-spacing:${attrs.spacing / 20}pt;letter-spacing:var(--word-spacing)` } : {} },
  } }],
})

const actionIcons:Record<string,typeof table>={addRow:rowsIcon,addColumn:columnsIcon,deleteRow:trashIcon,deleteColumn:trashIcon,deleteTable:trashIcon,'align-left':alignLeft,'align-center':alignCenter,'align-right':alignRight,'vertical-top':alignTop,'vertical-center':alignMiddle,'vertical-bottom':alignBottom}
const tableCommands:Record<string,typeof addRowAfter>={addRow:addRowAfter,addColumn:addColumnAfter,deleteRow,deleteColumn,deleteTable}
const tableActions={addRow:'在下方插入行',addColumn:'在右侧插入列',deleteRow:'删除当前行',deleteColumn:'删除当前列',deleteTable:'删除表格','border-all':'全部边框','border-horizontalEdges':'仅上下边框','border-none':'无边框','align-left':'单元格左对齐','align-center':'单元格居中','align-right':'单元格右对齐','vertical-top':'顶端对齐','vertical-center':'垂直居中','vertical-bottom':'底端对齐'}
const reportFields = {
  'common.generatedDate': '生成日期', 'report.reportDate': '通报日期',
  'report.semesterName': '学期名称', 'report.weekNo': '周次数字', 'report.weekNoChinese': '周次汉字',
}

export function convertReportTemplate(value: IntroductionDocument, types: ReportOutputType[]): WordTemplateDocument {
  const content: JSONContent[] = value.content.map(paragraph => ({
    ...paragraph, content: paragraph.content?.flatMap(run => run.text.split(/(\{(?:semesterName|weekNo|weekNoChinese|reportDate)\})/g).filter(Boolean).map(text =>
      /^\{(?:semesterName|weekNo|weekNoChinese|reportDate)\}$/.test(text) ? { type: 'templateField', attrs: { key: 'report.' + text.slice(1,-1) } } : { ...run, text })),
  }))
  if (types.length) content.push({ type: 'sectionBreak', attrs: { orientation: 'landscape' } })
  for (const type of types) content.push(
    { type: 'paragraph', attrs: { style: 'heading', textAlign: 'center' }, content: [{ type: 'text', text: type.title || type.name }] },
    { type: 'reportTable', attrs: { key: type.code } },
  )
  return { ...value, schemaVersion: 2, orientation: 'portrait', content }
}

export function WordTemplateEditor({value,onChange,types,context="report",readOnly=false,initialZoom=100,period}:{initialZoom?:number;period?:{semesterName:string;weekNo:number;issueDate:string};context?:"report"|"discipline";value:WordTemplateDocument;onChange:(doc:WordTemplateDocument)=>void;types:ReportOutputType[];readOnly?:boolean}) {
  const [layout,setLayout]=useState<CellLayout>()
  const [zoom,setZoom]=useState(initialZoom)
  const fields:Record<string,string> = context==="report" ? reportFields : disciplineFields
  const fieldSamples: Record<string,string>={'report.semesterName':period?.semesterName??'示例学期','report.weekNo':String(period?.weekNo??9),'report.weekNoChinese':chineseReportNumber(period?.weekNo??9),
    'report.reportDate':period ? period.issueDate.replace(/^(\d+)-0?(\d+)-0?(\d+)$/,'$1年$2月$3日') : '通报日期'}
  const current=useRef({value,onChange})
  useLayoutEffect(()=>{current.current={value,onChange}},[value,onChange])
  const atom=(name:string, attr:string | null, label:(key:string)=>string, inline=false)=>Node.create({
    name, group:inline?'inline':'block', inline, atom:true, selectable:true, draggable:!inline,
    addAttributes:()=>attr?{[attr]:{default:null,parseHTML:element=>element.getAttribute('data-word-value')}}:{},
    parseHTML:()=>[{tag:'[data-word-node="'+name+'"]',getAttrs:element=>{
      const key=element.getAttribute('data-word-value')??''
      if(name==='templateField' && !Object.hasOwn(fields,key)) return false
      if(name==='reportTable' && (context!=='report' || !types.some(type=>type.code===key))) return false
      if(name==='sectionBreak' && key!=='portrait' && key!=='landscape') return false
      return attr?{[attr]:key}:{}
    }}],
    renderHTML:({node}):DOMOutputSpec=>{
      const key=attr?String(node.attrs[attr]):''
      const attributes={'data-word-node':name,'data-word-value':attr?key:undefined,contenteditable:'false'}
      if(name==='reportTable'){
        const type=types.find(option=>option.code===key)
        if(type?.columns.length) return ['div',{...attributes,class:'template-table-block','aria-label':type.name},
          ['table',{},['thead',{},['tr',{},...type.columns.map((column):DOMOutputSpec=>['th',{scope:'col'},column])]],
          ['tbody',{},['tr',{},['td',{colspan:type.columns.length},'生成文件时填入本周检查数据']]]]]
      }
      return [inline?'span':'div',{...attributes,title:inline?label(key):undefined,class:inline?'report-template-variable':'word-template-block'},inline?(fieldSamples[key]??label(key)):label(key)]
    },
  })
  const editor=useEditor({
    editable:!readOnly,
    extensions:[...introductionExtensions.filter(ext=>ext.name!=='templateVariable'),
      ...(context==='discipline'?[disciplineRows]:[]),characterFormatting, wordTable, wordRow, wordCell,
      atom('templateField','key',key=>fields[key]??key,true),
      atom('reportTable','key',key=>types.find(type=>type.code===key)?.name??key),
      atom('pageBreak',null,()=>'分页符'),atom('sectionBreak','orientation',key=>'下一页分节 · '+(key==='landscape'?'横向':'纵向'))],
    content:{type:'doc',content:value.content.map(node=>{
      const attrs=node.attrs
      if(context==='discipline' && node.type==='paragraph' && !attrs?.lineSpacing && (!attrs?.lineSpacingMode || attrs.lineSpacingMode==='fixed')) {
        return {...node,attrs:{...attrs,lineSpacingMode:'single'}}
      }
      return tableForEditor(node)
    })},immediatelyRender:false,
    editorProps:{attributes:{'aria-label':readOnly?'模板正文预览':'文件模板正文',class:'word-template-paper'}},
    onUpdate:({editor})=>current.current.onChange({...current.current.value,content:(editor.getJSON().content??[]).map(tableForStorage)}),
  },[introductionExtensions,context,readOnly])
  const section=useEditorState({editor,selector:({editor})=>{
    let position=-1
    let direction=current.current.value.orientation
    if(editor) editor.state.doc.forEach((node,offset)=>{if(offset<editor.state.selection.from && node.type.name==='sectionBreak'){position=offset;direction=node.attrs.orientation}})
    return {position,direction,letterhead:editor?.isActive('paragraph',{letterhead:true}),inTable:editor?.isActive('wordTable'), color:editor?.getAttributes('textStyle').color, spacing:editor?.getAttributes('textStyle').spacing}
  }})
  const direction=section && section.position>=0?section.direction:value.orientation
  const insert=(node:JSONContent)=>editor?.chain().focus().insertContent(node).run()
  function insertBreak(type: 'pageBreak' | 'sectionBreak') {
    if (!editor) return
    const {state, view} = editor
    const marker = state.schema.nodes[type].create(type === 'sectionBreak' ? {orientation:direction} : undefined)
    const tr = closeHistory(state.tr).replaceSelectionWith(marker)
    let after = -1
    tr.doc.forEach((node, offset) => {
      if (node === marker) after = offset + node.nodeSize
    })
    if (after < 0) return
    // Reuse the split-off text when inserting mid-paragraph. At the end of the
    // document (or before another atomic block), the new page needs a typing target.
    if (!tr.doc.nodeAt(after)?.isTextblock) {
      tr.insert(after, state.schema.nodes.paragraph.create(state.selection.$from.parent.isTextblock
        ? state.selection.$from.parent.attrs : undefined))
    }
    tr.setSelection(TextSelection.create(tr.doc, after + 1))
    view.dispatch(tr.setMeta('revealTemplateBreak', true).scrollIntoView())
    view.focus()
  }
  const divider=<span className="report-intro-toolbar-divider" role="separator" aria-orientation="vertical"/>
  return <div className="report-intro-editor">
    {!readOnly && <div className="report-intro-toolbar" role="toolbar" aria-label="模板编辑工具">
      <Button type="text" className="report-intro-tool" aria-label="撤销" title="撤销" icon={<Icon icon={undo}/>} onClick={()=>editor?.chain().focus().undo().run()}/>
      <Button type="text" className="report-intro-tool" aria-label="重做" title="重做" icon={<Icon icon={redo}/>} onClick={()=>editor?.chain().focus().redo().run()}/>
      {divider}
      <div className="report-intro-zoom" role="group" aria-label="页面缩放">
        <Button type="text" className="report-intro-tool" aria-label="缩小" title="缩小" disabled={zoom<=50} icon={<Icon icon={minus}/>} onClick={()=>setZoom(zoom-25)}/>
        <span aria-live="polite">{zoom}%</span>
        <Button type="text" className="report-intro-tool" aria-label="放大" title="放大" disabled={zoom>=125} icon={<Icon icon={plus}/>} onClick={()=>setZoom(zoom+25)}/>
      </div>
      {divider}
      {context==='discipline' && <Button size="small" aria-pressed={!!section?.letterhead} disabled={!editor || section?.inTable} type={section?.letterhead?'primary':'default'} onClick={()=>editor?.chain().focus().updateAttributes('paragraph',{letterhead:!section?.letterhead}).run()}>红头行</Button>}
      <WordFormattingControls discipline={context==='discipline'} editor={editor} characterTools={<CharacterTools editor={editor} color={section?.color}/>}/>
      <div className="word-toolbar-group" role="group" aria-label="插入与表格">
      <Dropdown classNames={{root:"word-control-popup"}} trigger={['click']} menu={{items:Object.entries(fields).map(([key,label])=>({key,label})),onClick:({key})=>insert({type:'templateField',attrs:{key}})}}><Button type="text" className="report-intro-tool" aria-label="插入字段" title="插入字段" icon={<Icon icon={braces}/>}></Button></Dropdown>
      <Dropdown classNames={{root:'word-control-popup word-table-menu'}} trigger={['click']} menu={{items:[
        {key:'insert',icon:<Icon icon={table}/>,label:'插入表格',children:[1,2,3,4].map(rows=>({key:'row'+rows,label:rows+' 行',children:[1,2,3,4,5,6].map(cols=>({key:`insert-${rows}-${cols}`,label:cols+' 列'}))}))},
        {key:'layout',icon:<Icon icon={ruler}/>,label:'尺寸与边线',disabled:!section?.inTable},
        ...Object.entries(tableActions).map(([key,label])=>({key,label,icon:<Icon icon={actionIcons[key] ?? table}/>,disabled:!section?.inTable})),
      ],onClick:({key})=>{
        if(key.startsWith('insert-')){const [,rows,cols]=key.split('-');insert(newTable(Number(rows),Number(cols)));return}
        if(!editor)return
        if(key==='layout'){setLayout(readCellLayout(editor));return}
        editor.commands.focus()
        if(key.startsWith('border-'))editor.commands.updateAttributes('wordTable',{border:key.slice(7)})
        else if(key.startsWith('align-'))setCellAttr('align',key.slice(6))(editor.state,editor.view.dispatch)
        else if(key.startsWith('vertical-'))setCellAttr('verticalAlign',key.slice(9))(editor.state,editor.view.dispatch)
        else tableCommands[key]?.(editor.state,editor.view.dispatch)
      }}}><Button type="text" className="report-intro-tool" aria-label="表格" title="表格" icon={<Icon icon={table}/>}/></Dropdown>
      {context==='report' && <Dropdown classNames={{root:"word-control-popup"}} trigger={['click']} menu={{items:types.map(type=>({key:type.code,label:type.name})),onClick:({key})=>insert({type:'reportTable',attrs:{key}})}}><Button type="text" className="report-intro-tool" aria-label="插入通报表" title="插入通报表" icon={<Icon icon={table}/>} disabled={!types.length}></Button></Dropdown>}
      </div>
      <div className="word-toolbar-group" role="group" aria-label="页面布局">
      <PageMarginTool value={value} onChange={onChange}/>
      <Select classNames={{popup:{root:"word-control-popup"}}} popupMatchSelectWidth={false} size="small" variant="borderless" className="report-intro-select word-direction-select" aria-label="当前节方向" value={direction} options={[{value:'portrait',label:'纵向'},{value:'landscape',label:'横向'}]} onChange={orientation=>{
        if(editor && section && section.position>=0) editor.view.dispatch(editor.state.tr.setNodeMarkup(section.position,undefined,{orientation}))
        else onChange({...value,orientation})
      }}/>
      <ToolButton label="插入分页符" icon={<Icon icon={pageBreak}/>} onClick={()=>insertBreak('pageBreak')}/>
      <ToolButton label="下一页分节" icon={<Icon icon={sectionBreak}/>} onClick={()=>insertBreak('sectionBreak')}/>
      </div>

    </div>}
    {layout && <Modal title="尺寸与边线" width={360} open okText="应用" onCancel={()=>setLayout(undefined)}
      onOk={()=>{if(editor)applyCellLayout(editor,layout);setLayout(undefined)}}>
      <div className="word-cell-layout">
        <label>当前列宽（表宽 %）<InputNumber aria-label="当前列宽" min={1} max={99} precision={2} value={layout.width} onChange={width=>width && setLayout({...layout,width:Math.min(99,Math.max(1,width))})}/></label>
        <label>最小行高（磅）<InputNumber aria-label="最小行高" min={0} max={720} precision={0} value={layout.height} onChange={height=>setLayout({...layout,height:Math.min(720,Math.max(0,height??0))})}/></label>
        <fieldset><legend>透明边线</legend><Space wrap>{['上','右','下','左'].map((label,i)=><Checkbox key={label} checked={!!(layout.hiddenEdges & (1<<i))} onChange={()=>setLayout({...layout,hiddenEdges:layout.hiddenEdges^(1<<i)})}>{label}边</Checkbox>)}</Space></fieldset>
      </div>
    </Modal>}
    <TemplatePaper editor={editor} value={value} onChange={onChange} zoom={zoom} discipline={context==='discipline'} readOnly={readOnly}/>
  </div>
}

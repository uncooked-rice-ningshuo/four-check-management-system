import { tableResize } from "./tableResize"
import { Node, type Editor, type JSONContent } from '@tiptap/core'
import { Plugin } from '@tiptap/pm/state'
import { Decoration, DecorationSet } from '@tiptap/pm/view'
import { CellSelection, goToNextCell, tableEditing } from '@tiptap/pm/tables'

export const wordFontSizes = [5, 5.5, 6.5, 7.5, 8, 9, 10, 10.5, 11, 12, 14, 15, 16, 18, 20, 22, 24, 26, 28, 30, 32, 36, 40, 42, 44, 48, 54, 56, 60, 64, 72, 80, 88, 96, 108].map(points => points + 'pt')

type LayoutCell = {verticalAlign?:'top'|'center'|'bottom'; fontSize?:string; length:number; align:string; width?:number; height?:number; hiddenEdges?:number}
export function newTable(rows:number, columns:number):JSONContent {
  return {type:'wordTable',attrs:{border:'all'},content:Array.from({length:rows},()=>({
    type:'wordRow',content:Array.from({length:columns},()=>({type:'wordCell'})),
  }))}
}

// Older two-cell templates stay editable without rewriting their published revisions.
export function tableForEditor(node:JSONContent):JSONContent {
  const runs=node.content ?? []
  if(node.type==='imprintTable'){
    const split=node.attrs?.cellSplit ?? 0
    return {type:'wordTable',attrs:{border:'horizontalEdges'},content:[{type:'wordRow',content:[
      {type:'wordCell',attrs:{align:'left'},content:runs.slice(0,split)},
      {type:'wordCell',attrs:{align:'right'},content:runs.slice(split)},
    ]}]}
  }
  if(node.type!=='wordTable')return node
  let offset=0
  const rows:LayoutCell[][]=node.attrs?.rows ?? []
  return {type:'wordTable',attrs:{border:node.attrs?.border,insetLeft:node.attrs?.insetLeft,insetRight:node.attrs?.insetRight},content:rows.map(row=>({type:'wordRow',attrs:{height:row[0]?.height ?? 0},content:row.map(cell=>{
    const content=runs.slice(offset,offset+cell.length)
    offset+=cell.length
    return {type:'wordCell',attrs:{align:cell.align,verticalAlign:cell.verticalAlign ?? 'top',fontSize:cell.fontSize ?? null,width:cell.width ?? 0,hiddenEdges:cell.hiddenEdges ?? 0},content}
  })}))}
}

export function tableForStorage(node:JSONContent):JSONContent {
  if(node.type!=='wordTable')return node
  const runs:JSONContent[]=[]
  const rows=node.content?.map(row=>row.content?.map(cell=>{
    const content=cell.content ?? []
    runs.push(...content)
    return {length:content.length,align:cell.attrs?.align ?? 'left',verticalAlign:cell.attrs?.verticalAlign==='top'?undefined:cell.attrs?.verticalAlign,fontSize:cell.attrs?.fontSize || undefined,width:cell.attrs?.width || undefined,height:row.attrs?.height || undefined,hiddenEdges:cell.attrs?.hiddenEdges || undefined}
  }))
  return {type:'wordTable',attrs:{border:node.attrs?.border,insetLeft:node.attrs?.insetLeft||undefined,insetRight:node.attrs?.insetRight||undefined,rows},content:runs}
}

export const wordTable = Node.create({
  name:'wordTable',group:'block',content:'wordRow+',isolating:true,
  extendNodeSchema(extension){return {tableRole:({wordTable:'table',wordRow:'row',wordCell:'cell'} as Record<string,string>)[extension.name]}},
  addAttributes:()=>({insetLeft:{default:0},insetRight:{default:0},border:{default:'all',parseHTML:el=>el.dataset.borders ?? 'all'}}),
  parseHTML:()=>[{tag:'table[data-word-table]'}],
  renderHTML:({node})=>['table',{'data-word-table':'','data-borders':node.attrs.border,'data-inset-left':node.attrs.insetLeft,'data-inset-right':node.attrs.insetRight,'aria-label':'普通表格'},['tbody',0]],
  addProseMirrorPlugins:()=>[tableResize(),tableEditing()],
  addKeyboardShortcuts(){return {Tab:()=>goToNextCell(1)(this.editor.state,this.editor.view.dispatch),'Shift-Tab':()=>goToNextCell(-1)(this.editor.state,this.editor.view.dispatch)}},
})
export const wordRow=Node.create({name:'wordRow',content:'wordCell+',parseHTML:()=>[{tag:'tr'}],addAttributes:()=>({height:{default:0}}),renderHTML:({node})=>['tr',{style:node.attrs.height ? 'height:'+node.attrs.height+'pt' : undefined},0]})
export const wordCell=Node.create({
  name:'wordCell',content:'inline*',isolating:true,
  addProseMirrorPlugins:()=>[new Plugin({props:{decorations(state){
    const decorations:Decoration[]=[]
    state.doc.descendants((cell,pos)=>{
      if(cell.type.name!=='wordCell')return
      const base=cell.attrs.fontSize || '15pt'
      // Remove the inherited 15 pt line box only when the text actually uses a smaller size.
      let smallest=Infinity
      let allExplicit=true
      cell.forEach(run=>{
        const mark=run.marks.find(mark=>mark.type.name==='textStyle')
        if(!mark?.attrs.fontSize)allExplicit=false
        smallest=Math.min(smallest,parseFloat(mark?.attrs.fontSize || base))
      })
      if(allExplicit && smallest<parseFloat(base))decorations.push(Decoration.node(pos,pos+cell.nodeSize,{style:'font-size:'+smallest+'pt'}))
      return false
    })
    return DecorationSet.create(state.doc,decorations)
  }}})],
  addAttributes:()=>({verticalAlign:{default:'top',parseHTML:el=>el.style.verticalAlign==='middle'?'center':el.style.verticalAlign==='bottom'?'bottom':'top'},fontSize:{default:null,parseHTML:el=>wordFontSizes.includes(el.dataset.cellFontSize ?? '')?el.dataset.cellFontSize:null},align:{default:'left',parseHTML:el=>el.style.textAlign||'left'},width:{default:0},hiddenEdges:{default:0},colspan:{default:1},rowspan:{default:1},colwidth:{default:null}}),
  parseHTML:()=>[{tag:'td'}],renderHTML:({node})=>['td',{'data-cell-font-size':node.attrs.fontSize,style:[node.attrs.fontSize && 'font-size:'+node.attrs.fontSize,'text-align:'+node.attrs.align,'vertical-align:'+(node.attrs.verticalAlign==='center'?'middle':node.attrs.verticalAlign),node.attrs.width && 'width:'+node.attrs.width/100+'%', ...edges.map((edge,i)=>node.attrs.hiddenEdges & (1<<i) ? 'border-'+edge+':1px hidden transparent;--guide-'+edge+':1px' : '')].filter(Boolean).join(';')},0],
})

export const edges = ['top','right','bottom','left'] as const
export type CellLayout = {width:number; height:number; hiddenEdges:number}

function selectedCell(editor:Editor){
  const pos=editor.state.selection.$from
  for(let depth=pos.depth;depth>=3;depth--){
    if(pos.node(depth).type.name==='wordCell'){
      return {table:pos.node(depth-2),start:pos.start(depth-2),row:pos.index(depth-2),column:pos.index(depth-1)}
    }
  }
}

export function readCellLayout(editor:Editor):CellLayout|undefined{
  const selected=selectedCell(editor)
  if(!selected)return
  const row=selected.table.child(selected.row)
  const cell=row.child(selected.column)
  return {width:(cell.attrs.width || Math.round(10000/row.childCount))/100,height:row.attrs.height,hiddenEdges:cell.attrs.hiddenEdges}
}

// Keep column proportions across rows, and apply a shared edge to both adjoining cells.
export function applyCellLayout(editor:Editor, layout:CellLayout){
  const selected=selectedCell(editor)
  const previous=readCellLayout(editor)
  if(!selected || !previous)return
  const {table,start,row,column}=selected
  const firstRow=table.child(0)
  const weights=Array.from({length:firstRow.childCount},(_,i)=>firstRow.child(i).attrs.width || 10000/firstRow.childCount)
  const otherWeight=weights.reduce((sum,w,i)=>sum+(i===column?0:w),0)
  const target=firstRow.childCount===1 ? 10000 : Math.round(layout.width*100)
  const widths=weights.map((w,i)=>i===column ? target : Math.max(1,Math.round((10000-target)*w/otherWeight)))
  const tr=editor.state.tr
  table.forEach((r,offset,ri)=>{
    const rowPos=start+offset
    if(ri===row && layout.height!==previous.height)tr.setNodeMarkup(rowPos,undefined,{...r.attrs,height:layout.height})
    r.forEach((cell,cellOffset,ci)=>{
      let mask=cell.attrs.hiddenEdges
      if(ri===row && ci===column)mask=layout.hiddenEdges
      const neighbors=[[row-1,column],[row,column+1],[row+1,column],[row,column-1]]
      neighbors.forEach(([nr,nc],i)=>{
        const bit=1<<i
        if(nr!==ri || nc!==ci || !((previous.hiddenEdges^layout.hiddenEdges)&bit))return
        const opposite=1<<((i+2)%4)
        mask=layout.hiddenEdges&bit ? mask|opposite : mask&~opposite
      })
      const width=layout.width===previous.width ? cell.attrs.width : widths[ci]
      if(mask!==cell.attrs.hiddenEdges || width!==cell.attrs.width)tr.setNodeMarkup(rowPos+1+cellOffset,undefined,{...cell.attrs,width,hiddenEdges:mask})
    })
  })
  editor.view.dispatch(tr)
  editor.commands.focus()
}


export function applyWordFontSize(editor:Editor,size:string){
  const {selection}=editor.state
  const chain=editor.chain().focus()
  const paragraph=selection.$from.parent
  const titleAtCaret=selection.empty && paragraph.content.size>0 &&
    (paragraph.attrs.letterhead || paragraph.attrs.style==='title')
  if(paragraph.type.name==='paragraph' && (titleAtCaret ||
    (selection.from===selection.$from.start() && selection.to===selection.$from.end()))) {
    // Keep the paragraph mark in sync when the whole line changes size.
    chain.updateAttributes('paragraph',{fontSize:size})
    if(titleAtCaret) chain.setTextSelection({from:selection.$from.start(),to:selection.$from.end()})
    chain.setFontSize(size)
    if(titleAtCaret) chain.setTextSelection(selection.from)
    chain.run()
    return
  }
  if(selection instanceof CellSelection){
    chain.command(({tr})=>{
      selection.forEachCell((cell,pos)=>tr.setNodeMarkup(pos,undefined,{...cell.attrs,fontSize:size}))
      return true
    })
  }else if(selection.empty && selection.$from.parent.content.size===0){
    const type=selection.$from.parent.type.name
    if(type==='wordCell' || type==='paragraph') chain.updateAttributes(type,{fontSize:size})
  }else if(!selection.empty){
    chain.command(({tr})=>{
      tr.doc.nodesBetween(selection.from,selection.to,(node,pos)=>{
        if(node.type.name==='paragraph' && node.content.size===0 && pos+node.nodeSize-1 < selection.to){
          tr.setNodeMarkup(pos,undefined,{...node.attrs,fontSize:size})
        }
      })
      return true
    })
  }
  chain.setFontSize(size).run()
}

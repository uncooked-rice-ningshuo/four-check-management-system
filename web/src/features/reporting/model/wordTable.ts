import { tableResize } from "./tableResize"
import { Node, type Editor, type JSONContent } from '@tiptap/core'
import { goToNextCell, tableEditing } from '@tiptap/pm/tables'

type LayoutCell = {length:number; align:string; width?:number; height?:number; hiddenEdges?:number}
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
    return {type:'wordCell',attrs:{align:cell.align,width:cell.width ?? 0,hiddenEdges:cell.hiddenEdges ?? 0},content}
  })}))}
}

export function tableForStorage(node:JSONContent):JSONContent {
  if(node.type!=='wordTable')return node
  const runs:JSONContent[]=[]
  const rows=node.content?.map(row=>row.content?.map(cell=>{
    const content=cell.content ?? []
    runs.push(...content)
    return {length:content.length,align:cell.attrs?.align ?? 'left',width:cell.attrs?.width || undefined,height:row.attrs?.height || undefined,hiddenEdges:cell.attrs?.hiddenEdges || undefined}
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
  addAttributes:()=>({align:{default:'left',parseHTML:el=>el.style.textAlign||'left'},width:{default:0},hiddenEdges:{default:0},colspan:{default:1},rowspan:{default:1},colwidth:{default:null}}),
  parseHTML:()=>[{tag:'td'}],renderHTML:({node})=>['td',{style:['text-align:'+node.attrs.align,node.attrs.width && 'width:'+node.attrs.width/100+'%', ...edges.map((edge,i)=>node.attrs.hiddenEdges & (1<<i) ? 'border-'+edge+':1px hidden transparent;--guide-'+edge+':1px' : '')].filter(Boolean).join(';')},0],
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

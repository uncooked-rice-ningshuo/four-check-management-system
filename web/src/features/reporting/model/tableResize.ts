import { Plugin } from '@tiptap/pm/state'
import type { EditorView } from '@tiptap/pm/view'

function boundary(event: MouseEvent) {
  const cell = event.target instanceof Element ? event.target.closest('td') : null
  const row = cell?.parentElement
  const table = cell?.closest('table[data-word-table]')
  if (!(cell instanceof HTMLTableCellElement) || !(row instanceof HTMLTableRowElement) || !(table instanceof HTMLTableElement)) return
  const rect = cell.getBoundingClientRect()
  const column = cell.cellIndex
  if (rect.width < 1 || rect.height < 1) return
  if(column===0 && Math.abs(event.clientX-rect.left)<=5)
    return {axis:'column' as const, table,index:column,outer:'left' as const}
  if(column===row.cells.length-1 && Math.abs(event.clientX-rect.right)<=5)
    return {axis:'column' as const, table,index:column,outer:'right' as const}
  if (column + 1 < row.cells.length && Math.abs(event.clientX - rect.right) <= 5)
    return {axis:'column' as const, table, index:column}
  if (column > 0 && Math.abs(event.clientX - rect.left) <= 5)
    return {axis:'column' as const, table, index:column-1}
  if (Math.abs(event.clientY - rect.bottom) <= 5)
    return {axis:'row' as const, table, index:row.rowIndex}
  if (row.rowIndex > 0 && Math.abs(event.clientY - rect.top) <= 5)
    return {axis:'row' as const, table, index:row.rowIndex-1}
}

function startDrag(view:EditorView, event:MouseEvent, hit:NonNullable<ReturnType<typeof boundary>>) {
  const {table, axis, index} = hit
  const rect = table.getBoundingClientRect()
  const column = axis === 'column'
  const row = table.rows[column ? 0 : index]
  const box = (column ? row.cells[index] : row).getBoundingClientRect()
  const widths = Array.from(table.rows[0].cells, cell => cell.getBoundingClientRect().width)
  const scale = rect.width / table.offsetWidth
  const minWidth = Math.min(24 * scale, (widths[index] + (widths[index+1] ?? 0))/2)
  const origin = column ? event.clientX : event.clientY
  const doc = view.state.doc
  const tablePos = view.posAtDOM(table, 0) - 1
  const model = doc.nodeAt(tablePos)
  if (model?.type.name !== 'wordTable') return
  const line = document.createElement('div')
  line.className = 'word-table-resize-guide'
  Object.assign(line.style, column ? {top:rect.top+'px',height:rect.height+'px',width:'2px'} : {left:rect.left+'px',width:rect.width+'px',height:'2px'})
  document.body.append(line)
  const outer = hit.outer
  const existingInset = model.attrs[outer === 'left' ? 'insetLeft' : 'insetRight'] || 0
  const available = rect.width * 10000/(10000-(model.attrs.insetLeft||0)-(model.attrs.insetRight||0))
  const listeners = new AbortController()
  function cancel() { listeners.abort(); line.remove(); delete view.dom.dataset.resizeAxis }
  function delta(e:MouseEvent) {
    const raw = (column ? e.clientX : e.clientY) - origin
    if(outer){
      const grow = available*existingInset/10000
      const shrink = Math.max(0,rect.width-Math.max(available/10,24*scale*widths.length))
      return outer==='left' ? Math.max(-grow,Math.min(shrink,raw)) : Math.max(-shrink,Math.min(grow,raw))
    }
    return column ? Math.max(minWidth-widths[index], Math.min(widths[index+1]-minWidth,raw))
      : Math.max(8*scale-box.height,Math.min(960*scale-box.height,raw))
  }
  function move(e:MouseEvent) {
    const d = delta(e)
    if (column) line.style.left = ((outer==='left'?box.left:box.right)+d)+'px'
    else line.style.top = (box.bottom+d)+'px'
  }
  function finish(e:MouseEvent) {
    cancel()
    if (!view.editable || view.state.doc !== doc || !model || Math.abs((column ? e.clientX : e.clientY)-origin)<2) return
    const d=delta(e)
    const tr=view.state.tr
    if(outer){
      const inset=Math.max(0,Math.round(existingInset+(outer==='left'?d:-d)*10000/available))
      const key=outer==='left'?'insetLeft':'insetRight'
      view.dispatch(tr.setNodeMarkup(tablePos,undefined,{...model.attrs,[key]:inset}))
      return
    }
    model.forEach((r,offset,ri)=>{
      const rowPos=tablePos+1+offset
      if (!column) {
        if (ri===index) tr.setNodeMarkup(rowPos,undefined,{...r.attrs,height:Math.round((box.height+d)/scale*0.75)})
        return
      }
      r.forEach((cell,cellOffset,ci)=>{
        const width=widths[ci]+(ci===index ? d : ci===index+1 ? -d : 0)
        tr.setNodeMarkup(rowPos+1+cellOffset,undefined,{...cell.attrs,width:Math.max(1,Math.round(width/rect.width*10000))})
      })
    })
    view.dispatch(tr)
  }
  const options={signal:listeners.signal}
  window.addEventListener('mousemove',move,options)
  window.addEventListener('mouseup',finish,options)
  window.addEventListener('blur',cancel,options)
  window.addEventListener('keydown',e=>{if(e.key==='Escape'){e.preventDefault();e.stopImmediatePropagation();cancel()}},{...options,capture:true})
  move(event)
  return cancel
}

/** Preview the boundary while dragging and commit a single undoable layout change on release. */
export function tableResize() {
  let cancel: (()=>void)|undefined
  return new Plugin({
    props:{handleDOMEvents:{
      mousemove(view,event) {
        if (event.buttons) return false
        const hit=view.editable && boundary(event)
        if(hit)view.dom.dataset.resizeAxis=hit.axis
        else delete view.dom.dataset.resizeAxis
        return false
      },
      mousedown(view,event) {
        if(!view.editable || event.button!==0)return false
        const hit=boundary(event)
        if(!hit)return false
        cancel?.()
        cancel=startDrag(view,event,hit)
        if(!cancel)return false
        event.preventDefault()
        return true
      },
    }},
    view:()=>({destroy:()=>cancel?.()}),
  })
}

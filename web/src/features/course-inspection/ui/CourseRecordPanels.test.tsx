import { render, screen } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import { CourseRecordPanel } from './CourseRecordPanels'
const recordQuery=vi.fn()
vi.mock('../model/queries',()=>({
 useCourseRecord:()=>recordQuery(),
 useInspectionContext:()=>({data:{items:[]}}),
}))
it('does not mount an editor for a draft the server says cannot be edited',()=>{
 recordQuery.mockReturnValue({data:{typeCode:'UNATTENDED_CLASSROOM',canEdit:false,semesterId:'semester'},isError:false})
 render(<CourseRecordPanel id="record" mode="edit" onClose={vi.fn()}/>)
 expect(screen.getByText('记录当前不可编辑，请刷新列表。')).toBeInTheDocument()
 expect(screen.queryByRole('button',{name:'保存修改'})).not.toBeInTheDocument()
})
it('keeps detail loading failures distinct from successful empty content',()=>{
 recordQuery.mockReturnValue({isError:true,refetch:vi.fn()})
 render(<CourseRecordPanel id="record" mode="detail" onClose={vi.fn()}/>)
 expect(screen.getByText('记录加载失败或无权访问。')).toBeInTheDocument()
})

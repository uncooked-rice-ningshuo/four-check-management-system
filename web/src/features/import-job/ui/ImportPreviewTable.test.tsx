import { render, screen } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import { ImportPreviewTable } from './ImportPreviewTable'
it('renders a date-only morning preview without course fields or an invented time', () => {
 render(<ImportPreviewTable typeCode="MORNING_ATTENDANCE" rows={[{id:'row',batchId:'batch',rowNo:5,rawData:{student_no:'000001',student_name:'测试学生',class_name:'测试班',inspection_date:'2026-10-01',remark:'核对'},validationStatus:'VALID',errors:[]}]} loading={false} page={1} pageSize={20} total={1} filter="ALL" onFilterChange={vi.fn()} onPageChange={vi.fn()}/> )
 expect(screen.getByText('000001')).toBeInTheDocument()
 expect(screen.getByRole('columnheader',{name:'检查日期'})).toBeInTheDocument()
 expect(screen.getByText('2026-10-01')).toBeInTheDocument()
 expect(screen.getByText('核对')).toBeInTheDocument()
 expect(screen.queryByRole('columnheader',{name:'课程'})).not.toBeInTheDocument()
 expect(screen.queryByRole('columnheader',{name:'节次'})).not.toBeInTheDocument()
 expect(screen.queryByText(/00:00/)).not.toBeInTheDocument()
})

it('shows the late-return timestamp and assigned dormitory without course fields', () => {
 render(<ImportPreviewTable typeCode="LATE_RETURN" rows={[{id:'row',batchId:'batch',rowNo:5,rawData:{student_no:'000001',inspection_date:'2026-10-01',inspection_time:'23:05:00',dormitory_name:'示例区 示例楼 101'},validationStatus:'VALID',errors:[]}]} loading={false} page={1} pageSize={20} total={1} filter="ALL" onFilterChange={vi.fn()} onPageChange={vi.fn()}/> )
 expect(screen.getByText('2026-10-01 23:05:00')).toBeInTheDocument()
 expect(screen.getByText('示例区 示例楼 101')).toBeInTheDocument()
 expect(screen.queryByRole('columnheader',{name:'课程'})).not.toBeInTheDocument()
})

it('shows an object hygiene preview with responsibilities and round instead of personal course columns',()=>{
 render(<ImportPreviewTable typeCode="DORMITORY_HYGIENE" rows={[{id:'row',batchId:'batch',rowNo:5,rawData:{dormitory_name:'示例区 示例楼 101',inspection_date:'2026-10-01',inspection_round:'首轮',result:'优秀',responsible_class_names:'班甲;班乙',responsible_student_nos:'',remark:'整体检查'},validationStatus:'VALID',errors:[]}]} loading={false} page={1} pageSize={20} total={1} filter="ALL" onFilterChange={vi.fn()} onPageChange={vi.fn()}/> )
 expect(screen.getByRole('columnheader',{name:'寝室'})).toBeInTheDocument()
 expect(screen.getByText('班甲;班乙')).toBeInTheDocument()
 expect(screen.getByText('首轮')).toBeInTheDocument()
 expect(screen.getByText('优秀')).toBeInTheDocument()
 expect(screen.queryByRole('columnheader',{name:'姓名'})).not.toBeInTheDocument()
 expect(screen.queryByRole('columnheader',{name:'课程'})).not.toBeInTheDocument()
})

it('shows a classroom area hygiene preview with responsibilities and round instead of personal course columns',()=>{
 render(<ImportPreviewTable typeCode="CLASSROOM_HYGIENE" rows={[{id:'row',batchId:'batch',rowNo:5,rawData:{classroom_name:'教学楼 101',area_name:'前排',inspection_date:'2026-10-01',inspection_round:'首轮',result:'优秀',responsible_class_names:'班甲;班乙',responsible_student_nos:'',remark:'整体检查'},validationStatus:'VALID',errors:[]}]} loading={false} page={1} pageSize={20} total={1} filter="ALL" onFilterChange={vi.fn()} onPageChange={vi.fn()}/> )
 expect(screen.getByRole('columnheader',{name:'检查区域'})).toBeInTheDocument()
 expect(screen.getByText('班甲;班乙')).toBeInTheDocument()
 expect(screen.getByText('首轮')).toBeInTheDocument()
 expect(screen.getByText('优秀')).toBeInTheDocument()
 expect(screen.queryByRole('columnheader',{name:'姓名'})).not.toBeInTheDocument()
 expect(screen.queryByRole('columnheader',{name:'课程'})).not.toBeInTheDocument()
})

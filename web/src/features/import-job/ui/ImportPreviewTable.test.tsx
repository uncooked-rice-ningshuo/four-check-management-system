import { render, screen } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import type { ImportRow } from '../../../entities/import-job/model/importJob'
import { ImportPreviewTable } from './ImportPreviewTable'

it('shows duplicate source rows with the uploaded event context', () => {
 const row: ImportRow = { id:'r12',batchId:'b',rowNo:12,rawData:{student_no:'S001',student_name:'虚构同学',class_name:'示例班',course_name:'数据结构',absence_date:'2026-09-06',absence_time:'08:00',period:'1-2'}, normalizedRecord:{id:'',studentNo:'',studentName:'',classId:'',className:'',grade:'',courseId:'',courseName:'',occurredAt:'',period:'',absenceHours:0,remark:'',source:'EXCEL',status:'DRAFT',rowVersion:1},validationStatus:'INVALID',errors:[{code:'DUPLICATE_IN_BATCH',field:'student_no',message:'第 12 行与本文件第 5 行重复',suggestion:'核对两行后保留正确的一行'}] }
 render(<ImportPreviewTable rows={[row]} loading={false} page={1} pageSize={20} total={1} filter="INVALID" onFilterChange={vi.fn()} onPageChange={vi.fn()} />)
 for(const value of ['第 12 行与本文件第 5 行重复','S001','虚构同学','数据结构','2026-09-06 08:00','1-2']) expect(screen.getByText(value,{exact:true})).toBeInTheDocument()
})

import { Table } from 'antd'
import { render, screen } from '@testing-library/react'
import { expect, it } from 'vitest'
import type { SubmissionCartRecord } from '../../../entities/inspection/model/submissionCart'
import { submissionRecordColumns } from './submissionRecordColumns'

const base: SubmissionCartRecord = { id: 'r', semesterId: 's', weekId: 'w', inspectionTypeId: 't', departmentId: 'd', typeCode: 'LATE_RETURN', studentNumber: 'S01', studentName: '测试学生', className: '计科一班', grade: '2026级', status: 'DRAFT', source: 'EXCEL', rowVersion: 1, canEdit: true, details: {} }
it('shows a late-return record with dormitory and time', () => {
  const row = { ...base, currentDormitoryName: '桃园 101', details: { date: '2026-09-10', time: '23:05:00', remark: '核对备注' } }
  render(<Table rowKey="id" pagination={false} columns={submissionRecordColumns('LATE_RETURN')} dataSource={[row]} />)
  for (const value of ['桃园 101', '2026-09-10', '23:05:00', '核对备注', 'Excel 导入']) expect(screen.getByText(value)).toBeInTheDocument()
})
it('shows hygiene responsibilities and readable result and reason', () => {
  const row = { ...base, typeCode: 'DORMITORY_HYGIENE', currentDormitoryName: '桃园 102', currentReasonLabel: '地面未清扫', responsibleClasses: [{ id: 'class', label: '计科一班' }], responsibleStudents: [{ id: 'student', label: '测试学生' }], details: { date: '2026-09-10', round: '第一轮', result: 'UNQUALIFIED', reasonCode: 'FLOOR', description: '有灰尘' } }
  render(<Table rowKey="id" pagination={false} columns={submissionRecordColumns('DORMITORY_HYGIENE')} dataSource={[row]} />)
  for (const value of ['桃园 102', '计科一班', '测试学生', '第一轮', '不合格', '地面未清扫', '有灰尘']) expect(screen.getByText(value)).toBeInTheDocument()
  expect(screen.queryByText('UNQUALIFIED')).not.toBeInTheDocument()
  expect(screen.queryByText('FLOOR')).not.toBeInTheDocument()
})
it('does not reveal internal IDs or codes when a referenced label is missing', () => {
  const row = { ...base, typeCode: 'DORMITORY_HYGIENE', details: { dormitoryId: 'internal-room-id', result: 'UNKNOWN_CODE', reasonCode: 'internal-reason-code' } }
  render(<Table rowKey="id" pagination={false} columns={submissionRecordColumns('DORMITORY_HYGIENE')} dataSource={[row]} />)
  expect(screen.getByText('名称不可用')).toBeInTheDocument()
  expect(screen.getByText('原因名称不可用')).toBeInTheDocument()
  expect(screen.getByText('结果不可用')).toBeInTheDocument()
  expect(screen.queryByText('internal-room-id')).not.toBeInTheDocument()
})

it.each([
  ['MORNING_ATTENDANCE', { date: '2026-09-12', absenceCount: 1 }, ['2026-09-12', '1']],
  ['UNATTENDED_CLASSROOM', { date: '2026-09-12', time: '14:00:00', period: '5-6', absenceHours: 2 }, ['2026-09-12', '14:00:00', '5-6']],
  ['EVENING_SELF_STUDY', { date: '2026-09-12', time: '20:15:00', period: '9-10', absenceHours: 2, penaltyMultiplier: 2, penaltyHours: 4 }, ['2026-09-12', '20:15:00', '9-10', '2 倍']],
] as const)('shows type-specific fields for %s', (typeCode, details, values) => {
  render(<Table rowKey="id" pagination={false} columns={submissionRecordColumns(typeCode)} dataSource={[{ ...base, typeCode, details }]} />)
  for (const value of values) expect(screen.getByText(value)).toBeInTheDocument()
})

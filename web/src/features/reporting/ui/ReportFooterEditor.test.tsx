import { act, fireEvent, render, screen } from '@testing-library/react'
import { createRef } from 'react'
import { expect, it, vi } from 'vitest'
import type { IntroductionDocument } from '../../../shared/api/reportOutputApi'
import { ReportFooterEditor, type ReportFooterEditorHandle } from './ReportFooterEditor'

it('edits the footer with automatic page fields and no persistent page toolbar', async () => {
  const document: IntroductionDocument = { type: 'doc', footerLayout: 'custom', footerAlign: 'left', footerText: '学院办公室\t第 {page} 页 共 {totalPages} 页\t', content: [{ type: 'paragraph', attrs: { style: 'body', textAlign: 'left' } }] }
  const onChange = vi.fn()
  const ref = createRef<ReportFooterEditorHandle>()
  render(<ReportFooterEditor ref={ref} value={document} onChange={onChange} />)
  expect(await screen.findByText('总页数')).toBeVisible()
  expect(screen.queryByRole('toolbar', { name: '页脚工具' })).not.toBeInTheDocument()
  act(() => ref.current?.insertField('semesterName'))
  expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ footerText: expect.stringContaining('{semesterName}'), footerLayout: 'custom' }))
})

it('centers a page field without replacing its contents', async () => {
  const value: IntroductionDocument = {type:'doc',content:[],footerLayout:'numberOnly'}
  const onChange=vi.fn()
  const {rerender}=render(<ReportFooterEditor value={value} onChange={onChange}/>)
  const center=await screen.findByLabelText('页尾居中')
  fireEvent.click(center)
  const next=onChange.mock.lastCall?.[0]
  expect(next).toMatchObject({footerLayout:'custom',footerText:'{page}',footerAlign:'center'})
  rerender(<ReportFooterEditor value={next} onChange={onChange}/>)
  expect(screen.getByLabelText('编辑页脚')).toHaveStyle({textAlign:'center'})
  expect(center).toHaveAttribute('aria-pressed','true')
})

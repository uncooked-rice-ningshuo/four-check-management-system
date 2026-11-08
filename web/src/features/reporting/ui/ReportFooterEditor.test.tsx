import { act, render, screen } from '@testing-library/react'
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

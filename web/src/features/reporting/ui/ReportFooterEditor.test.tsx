import { fireEvent, render, screen } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import type { IntroductionDocument } from '../../../shared/api/reportOutputApi'
import { ReportFooterEditor } from './ReportFooterEditor'

it('edits one page footer and moves the page field to the right', async () => {
  const document: IntroductionDocument = { type: 'doc', footerLayout: 'custom', footerAlign: 'center', footerText: '学院办公室{page}', content: [{ type: 'paragraph', attrs: { style: 'body', textAlign: 'left' } }] }
  const onChange = vi.fn()
  render(<ReportFooterEditor value={document} onChange={onChange} />)
  expect(await screen.findByText('1')).toBeVisible()
  expect(screen.getByRole('button', { name: '插入页码' })).toBeDisabled()
  fireEvent.click(screen.getByRole('button', { name: '页码靠右' }))
  expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ footerLayout: 'custom', footerAlign: 'left', footerText: '学院办公室\t{page}' }))
})

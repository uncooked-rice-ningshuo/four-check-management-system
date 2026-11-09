import { render, screen, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { ReportDocumentPreview } from './ReportDocumentPreview'

describe('ReportDocumentPreview', () => {
  it('previews the excellent dormitory two-room layout from backend sections', () => {
    render(<ReportDocumentPreview
      document={{ type: 'doc', content: [] }}
      types={[{
        code: 'DORMITORY_HYGIENE', name: '宿舍卫生', title: '宿舍卫生检查',
        columns: ['日期', '寝室号'], excelColumns: [],
        sections: [
          { title: '不合格寝室', columns: ['日期', '寝室号'] },
          { title: '优秀寝室', columns: ['寝室号', '所在班级', '责任人', '班主任', '寝室号', '所在班级', '责任人', '班主任'] },
        ],
      }]}
    />)
    const attachment = screen.getByRole('article', { name: '宿舍卫生附件预览' })
    expect(within(attachment).getByText('不合格寝室')).toBeVisible()
    expect(within(attachment).getByText('优秀寝室')).toBeVisible()
    expect(within(attachment).getAllByRole('columnheader', { name: '寝室号' })).toHaveLength(3)
  })

  it('shows Chinese week and page numerals with the selected file date', () => {
    render(<ReportDocumentPreview
      document={{ type: 'doc', footerLayout: 'custom', footerText: '第{pageChinese}页，共{totalPagesChinese}页 · 第{weekNoChinese}周', content: [
        { type: 'paragraph', attrs: { style: 'date', textAlign: 'right' }, content: [{ type: 'text', text: '{reportDate}' }] },
      ] }}
      period={{ semesterName: '示例学期', weekNo: 8, issueDate: '2026-11-14' }}
      types={[]}
    />)
    expect(screen.getByText('2026年11月14日')).toBeVisible()
    expect(screen.getByText('第八周', { exact: false })).toBeVisible()
    expect(screen.getByText('第一页，共一页', { exact: false })).toBeVisible()
  })
})

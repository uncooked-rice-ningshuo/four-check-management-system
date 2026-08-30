import { describe, expect, it } from 'vitest'
import { importErrorMessage } from './importErrorMessages'

describe('importErrorMessage', () => {
  it.each([
    ['FORBIDDEN_SCOPE', '无权访问当前学期、周次或部门的数据'],
    ['VERSION_CONFLICT', '数据已被其他人更新，请刷新后重试'],
    ['FILE_TOO_LARGE', '文件超过上传大小限制'],
    ['BATCH_HAS_ERRORS', '请先修正预览中的错误行'],
		['INVALID_HEADER', '模板字段已被修改，请重新下载模板'],
		['ROW_LIMIT_EXCEEDED', '导入数据不能超过 1000 行'],
		['SOURCE_FILE_UNAVAILABLE', '源文件不可用或完整性校验失败，请重新上传'],
		['INVALID_WORKBOOK', '工作簿结构无效，请使用最新模板重新填写'],
		['IMPORT_BATCH_NOT_FOUND', '导入批次不存在或无权访问'],
  ])('maps %s to an actionable message', (code, expected) => {
    expect(importErrorMessage(code)).toBe(expected)
  })

  it('uses a safe fallback for unknown codes', () => {
    expect(importErrorMessage('INTERNAL_SQL_DETAIL')).toBe('操作失败，请稍后重试')
  })
})

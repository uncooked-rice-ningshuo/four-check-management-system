const messages: Record<string, string> = {
  FORBIDDEN_SCOPE: '无权访问当前学期、周次或部门的数据',
  VERSION_CONFLICT: '数据已被其他人更新，请刷新后重试',
  FILE_TOO_LARGE: '文件超过上传大小限制',
  BATCH_HAS_ERRORS: '请先修正预览中的错误行',
  INVALID_FILE: '请选择有效的 XLSX 文件',
  INVALID_FILE_TYPE: '仅支持无宏的 XLSX 文件',
  TEMPLATE_VERSION_MISMATCH: '模板版本已更新，请重新下载模板',
	INVALID_XLSX_SIGNATURE: '文件不是有效的 XLSX 工作簿',
	XLSX_ARCHIVE_LIMIT_EXCEEDED: '工作簿解压内容超过安全限制',
	INVALID_HEADER: '模板字段已被修改，请重新下载模板',
	ROW_LIMIT_EXCEEDED: '导入数据不能超过 1000 行',
	SOURCE_FILE_UNAVAILABLE: '源文件不可用或完整性校验失败，请重新上传',
	INVALID_IDEMPOTENCY_KEY: '确认请求已失效，请重新打开预览后提交',
	INVALID_BATCH_STATE: '当前批次状态不允许此操作，请刷新后重试',
	IMPORT_TEMPLATE_NOT_FOUND: '当前检查类型尚未发布导入模板',
	INVALID_WORKBOOK: '工作簿结构无效，请使用最新模板重新填写',
	BATCH_ALREADY_CONFIRMED: '该批次已确认，请刷新查看最新结果',
	IMPORT_BATCH_NOT_FOUND: '导入批次不存在或无权访问',
	INVALID_TEMPLATE_RESPONSE: '模板信息读取失败，请稍后重试',
}

export function importErrorMessage(code?: string) {
  return (code && messages[code]) || '操作失败，请稍后重试'
}

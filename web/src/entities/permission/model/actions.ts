export const permissionActionOptions = [
  ['USER_MANAGE', '用户管理'], ['PERMISSION_MANAGE', '权限管理'], ['MASTER_DATA_MANAGE', '基础数据管理'], ['DICTIONARY_MANAGE', '字典管理'],
  ['CONFIGURATION_VIEW', '系统配置查看'], ['MORNING_SCHEDULE_MANAGE', '早签计划管理'],
  ['INSPECTION_ENTER', '检查录入'], ['INSPECTION_MANAGE', '管理检查记录'], ['INSPECTION_VIEW', '检查查看'], ['INSPECTION_REVIEW', '检查审核'],
  ['DISCIPLINE_LEDGER_VIEW', '学期违纪台账查看'], ['DISCIPLINE_LEDGER_EXPORT', '学期违纪台账导出'],
  ['REPORT_VIEW', '通报工作台查看'], ['REPORT_GENERATE', '生成通报草稿'], ['REPORT_FILE_OUTPUT', '生成并下载 Word／Excel'], ['REPORT_CORRECT', '通报内修正'], ['REPORT_OBLIGATION_MANAGE', '管理逐周应交范围'], ['REPORT_OUTPUT_SETTINGS_MANAGE', '编辑通报文件设置'], ['REPORT_FORCE_GENERATE', '强制生成未完成通报'], ['REPORT_REVIEW', '通报审核'], ['REPORT_RETURN', '通报退回'], ['REPORT_FINALIZE', '通报定稿'], ['REPORT_PUBLISH', '通报发布'], ['ARCHIVE_VIEW', '通报归档查看'],
].map(([value, label]) => ({ value, label }))
export const permissionActionName = (value: string) => permissionActionOptions.find(item => item.value === value)?.label ?? (value === 'INSPECTION_EXPORT' ? '检查导出（规划中）' : value)


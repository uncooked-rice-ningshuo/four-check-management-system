// API selection value; the server stores college ownership as a null department.
export const COLLEGE_DEPARTMENT_OPTION_ID = '00000000-0000-0000-0000-000000000000'

export const departmentOptionId = (departmentId: string | undefined) => departmentId || COLLEGE_DEPARTMENT_OPTION_ID

export function attributionLabel(mode: string | undefined, departmentId: string | undefined, departmentName?: string) {
  if (mode === 'ADMINISTRATOR') return '管理员独立记录'
  if (mode === 'COLLEGE') return '校级记录'
  return departmentId ? departmentName ?? '部门归属（部门名称不可用）' : '归属信息缺失'
}

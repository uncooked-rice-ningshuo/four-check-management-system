// User 是后端返回给前端的安全账号投影，明确排除密码哈希、刷新会话和权限内部数据。
export type User = {
  id: string
  loginName: string
  studentNo: string
  name: string
  status: string
  accountKind: 'SYSTEM' | 'PERSON'
  systemRole: 'SUPER_ADMIN' | 'ADMIN' | 'MEMBER'
  isSuperAdmin: boolean
  mustChangePassword: boolean
  primaryDepartmentId: string
  requestedDepartmentId: string
  requestedOrgRoleId: string
}

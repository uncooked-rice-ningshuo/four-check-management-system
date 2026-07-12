// User 是后端返回给前端的安全账号投影，明确排除密码哈希、刷新会话和权限内部数据。
export type User = {
  id: string
  studentNo: string
  name: string
  status: string
}

// 登录页和注册页必须复用同一组桌面容器约束，避免表单内容多少影响窗口尺寸。
export const AUTH_SHELL_DESKTOP_LAYOUT = {
  height: 'min(800px, calc(100vh - 64px))',
  columns: '1fr 1fr',
} as const

// 注册表单字段较多，单独采用更紧凑的纵向节奏，避免撑出认证窗口。
export const AUTH_REGISTER_FORM_SPACING = {
  itemGap: '12px',
  labelGap: '4px',
} as const

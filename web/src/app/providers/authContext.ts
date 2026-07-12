import { createContext, useContext } from 'react'
import type { User } from '../../entities/user/model/user'

// AuthState 是提供给跨页面 UI 使用的最小认证状态，令牌细节不会暴露给组件。
export type AuthState = {
  user: User | null
  loading: boolean
  signIn: (input: { studentNo: string; password: string }) => Promise<void>
  signOut: () => Promise<void>
}

export const AuthContext = createContext<AuthState | null>(null)

// useAuth 在组件未挂载到 AuthProvider 时立即报错，避免静默使用无效认证状态。
export function useAuth() {
  const context = useContext(AuthContext)
  if (!context) throw new Error('AuthProvider missing')
  return context
}

import { useEffect, useState, type ReactNode } from 'react'
import type { User } from '../../entities/user/model/user'
import { loginAccount, logoutAccount, refreshSession } from '../../shared/api/authApi'
import { AuthContext } from './authContext'

// AuthProvider 在应用启动时恢复一次浏览器会话，并只向页面暴露认证动作；
// 令牌的存取细节始终封装在 shared API 层中。
export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    refreshSession().then(setUser).catch(() => setUser(null)).finally(() => setLoading(false))
  }, [])

  return (
    <AuthContext.Provider value={{
      user,
      loading,
      signIn: async input => { const signedIn = await loginAccount(input); setUser(signedIn); return signedIn },
      signOut: async () => { await logoutAccount(); setUser(null) },
    }}>
      {children}
    </AuthContext.Provider>
  )
}

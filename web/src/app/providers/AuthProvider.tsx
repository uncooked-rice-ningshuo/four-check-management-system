import { useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import type { User } from '../../entities/user/model/user'
import { changePassword, loginAccount, logoutAccount, refreshSession } from '../../shared/api/authApi'
import { AuthContext } from './authContext'
import { ApiError, subscribeSessionExpired } from '../../shared/api/httpClient'

// AuthProvider 在应用启动时恢复一次浏览器会话，并只向页面暴露认证动作；
// 令牌的存取细节始终封装在 shared API 层中。
export function AuthProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient()
  const [user, setUser] = useState<User | null>(null)
  const [loading, setLoading] = useState(true)
  const [recoveryError, setRecoveryError] = useState<string | null>(null)
  const recoveryInFlight = useRef<Promise<User | null> | null>(null)

  // StrictMode and repeated clicks share one attempt. Only an explicit retry
  // follows a temporary failure; a consumed refresh token is never replayed automatically.
  const recoverSession = useCallback(() => {
    if (recoveryInFlight.current) return recoveryInFlight.current
    setLoading(true)
    recoveryInFlight.current = refreshSession().then(async recovered => {
      await queryClient.cancelQueries()
      queryClient.clear()
      setUser(recovered)
      setRecoveryError(null)
      return recovered
    }).catch((error: unknown) => {
      setUser(null)
      const retryable = !(error instanceof ApiError) || error.status >= 500 || error.status === 408 || error.status === 429
      setRecoveryError(retryable ? '暂时无法恢复登录状态，请重试或使用账号密码登录。' : null)
      return null
    }).finally(() => {
      recoveryInFlight.current = null
      setLoading(false)
    })
    return recoveryInFlight.current
  }, [queryClient])

  useEffect(() => subscribeSessionExpired(() => {
    // Cancel before clearing so late results cannot repopulate private caches.
    void queryClient.cancelQueries()
    queryClient.clear()
    setUser(null)
    setRecoveryError('登录状态已失效，请重新登录。')
  }), [queryClient])

  useEffect(() => {
    void recoverSession()
  }, [recoverSession])

  // Shared keys must never carry one account's records or permissions into another session.
  // Cancellation also prevents late responses from repopulating the removed queries.
  const clearSessionQueries = async () => {
    await queryClient.cancelQueries()
    queryClient.clear()
  }

  return (
    <AuthContext.Provider value={{
      user,
      loading,
      recoveryError,
      recoverSession,
      signIn: async input => { const signedIn = await loginAccount(input); await clearSessionQueries(); setRecoveryError(null); setUser(signedIn); return signedIn },
      signOut: async () => { await logoutAccount(); await clearSessionQueries(); setRecoveryError(null); setUser(null) },
      // The password endpoint already invalidates all sessions and clears the
      // cookie. A second logout request could misreport a successful change.
      changePassword: async input => { await changePassword(input); await clearSessionQueries(); setRecoveryError(null); setUser(null) },
    }}>
      {children}
    </AuthContext.Provider>
  )
}

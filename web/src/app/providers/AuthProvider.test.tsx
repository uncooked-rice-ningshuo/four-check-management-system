import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { StrictMode } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AuthProvider } from './AuthProvider'
import { useAuth } from './authContext'

const user = {
  id: 'admin-id',
  loginName: 'admin',
  studentNo: '',
  name: '系统管理员',
  status: 'ACTIVE',
  accountKind: 'SYSTEM',
  systemRole: 'SUPER_ADMIN',
  isSuperAdmin: true,
  mustChangePassword: false,
  primaryDepartmentId: '',
  requestedDepartmentId: '',
  requestedOrgRoleId: '',
}

function SessionState() {
  const { user: currentUser, loading, recoveryError } = useAuth()
  if (loading) return <span>正在恢复会话</span>
  return <><span>{currentUser?.name ?? '未登录'}</span>{recoveryError && <span>{recoveryError}</span>}</>
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('AuthProvider', () => {
  it('shows the eviction reason when startup refresh finds a displaced device', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ code: 'SESSION_EVICTED' }), { status: 401 })))
    render(<QueryClientProvider client={new QueryClient()}><AuthProvider><SessionState /></AuthProvider></QueryClientProvider>)
    await screen.findByText('账号已在其他设备登录，当前设备已退出。')
    expect(screen.getByText('未登录')).toBeInTheDocument()
  })
  it('StrictMode 重复执行 Effect 时只恢复一次会话', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/auth/refresh')) {
        return new Response(JSON.stringify({ accessToken: 'access-token' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      if (url.endsWith('/auth/me')) {
        return new Response(JSON.stringify(user), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      throw new Error(`unexpected request: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    render(<QueryClientProvider client={new QueryClient()}><StrictMode><AuthProvider><SessionState /></AuthProvider></StrictMode></QueryClientProvider>)

    await screen.findByText('系统管理员')
    await waitFor(() => {
      const refreshRequests = fetchMock.mock.calls.filter(([input]) => String(input).endsWith('/auth/refresh'))
      expect(refreshRequests).toHaveLength(1)
    })
  })
})

function SessionActions() {
 const { user: current, loading, signIn, signOut } = useAuth()
 if (loading) return <span>恢复中</span>
 return <><span>{current?.name ?? '已退出'}</span><button onClick={()=>void signOut()}>退出</button><button onClick={()=>void signIn({loginName:'second',password:'synthetic'})}>切换登录</button></>
}

it('clears private cached queries and cancels an old in-flight query before completing logout', async () => {
 const client=new QueryClient({defaultOptions:{queries:{retry:false}}})
 vi.stubGlobal('fetch',vi.fn(async(input:RequestInfo|URL)=>{
  const path=String(input)
  if(path.endsWith('/auth/refresh'))return new Response(JSON.stringify({accessToken:'synthetic-token'}),{status:200})
  if(path.endsWith('/auth/me'))return new Response(JSON.stringify(user),{status:200})
  if(path.endsWith('/auth/logout'))return new Response(null,{status:204})
  throw new Error('unexpected auth request')
 }))
 render(<QueryClientProvider client={client}><AuthProvider><SessionActions/></AuthProvider></QueryClientProvider>)
 await screen.findByText(user.name)
 client.setQueryData(['course-inspections','records'],{items:[{studentName:'上一账号私有记录'}]})
 let resolveOld: (value:string)=>void=()=>undefined
 const pending=client.fetchQuery({queryKey:['private-pending'],queryFn:()=>new Promise<string>(resolve=>{resolveOld=resolve})}).catch(()=>undefined)
 fireEvent.click(screen.getByRole('button',{name:'退出'}))
 await screen.findByText('已退出')
 expect(client.getQueryData(['course-inspections','records'])).toBeUndefined()
 resolveOld('上一账号迟到响应')
 await pending
 expect(client.getQueryData(['private-pending'])).toBeUndefined()
})

it('clears prior session data on successful sign-in before exposing the new account', async () => {
 const client=new QueryClient({defaultOptions:{queries:{retry:false}}})
 let switched=false
 vi.stubGlobal('fetch',vi.fn(async(input:RequestInfo|URL)=>{
  const path=String(input)
  if(path.endsWith('/auth/refresh'))return new Response(JSON.stringify({accessToken:'synthetic-token'}),{status:200})
  if(path.endsWith('/auth/login')){switched=true;return new Response(JSON.stringify({accessToken:'synthetic-next-token'}),{status:200})}
  if(path.endsWith('/auth/me'))return new Response(JSON.stringify(switched?{...user,id:'second-id',name:'第二账号'}:user),{status:200})
  throw new Error('unexpected auth request')
 }))
 render(<QueryClientProvider client={client}><AuthProvider><SessionActions/></AuthProvider></QueryClientProvider>)
 await screen.findByText(user.name)
 client.setQueryData(['effective-permissions'],{permissions:['previous-account']})
 fireEvent.click(screen.getByRole('button',{name:'切换登录'}))
 await screen.findByText('第二账号')
 expect(client.getQueryData(['effective-permissions'])).toBeUndefined()
})

it('clears the authenticated user and private cache when an active session expires', async () => {
 const { apiRequest } = await import('../../shared/api/httpClient')
 const { act } = await import('@testing-library/react')
 const client = new QueryClient({defaultOptions:{queries:{retry:false}}})
 let expired = false
 vi.stubGlobal('fetch', vi.fn(async (input:RequestInfo|URL) => {
  const path = String(input)
  if(path.endsWith('/auth/refresh')) return new Response(JSON.stringify(expired ? {code:'INVALID_SESSION'} : {accessToken:'synthetic-token'}),{status:expired?401:200})
  if(path.endsWith('/auth/me')) return new Response(JSON.stringify(user),{status:200})
  return new Response(JSON.stringify({code:'UNAUTHORIZED'}),{status:401})
 }))
 render(<QueryClientProvider client={client}><AuthProvider><SessionState/></AuthProvider></QueryClientProvider>)
 await screen.findByText(user.name)
 client.setQueryData(['private-records'],{items:['synthetic-private']})
 expired = true
 await act(async () => { await expect(apiRequest('/work-context')).rejects.toMatchObject({status:401}) })
 await screen.findByText('未登录')
 expect(client.getQueryData(['private-records'])).toBeUndefined()
})

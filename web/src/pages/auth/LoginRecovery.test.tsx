import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { StrictMode } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, expect, it, vi } from 'vitest'
import { AuthProvider } from '../../app/providers/AuthProvider'
import { ProtectedRoute } from '../../app/router/ProtectedRoute'
import { LoginPage } from './LoginPage'

const user = { id: 'recovered', name: 'Recovered user', status: 'ACTIVE', mustChangePassword: false }
afterEach(() => vi.unstubAllGlobals())
function showLogin() {
 render(<QueryClientProvider client={new QueryClient({defaultOptions:{queries:{retry:false}}})}><StrictMode><AuthProvider><MemoryRouter initialEntries={['/original']}><Routes>
  <Route path="/original" element={<ProtectedRoute><div>原业务页面</div></ProtectedRoute>} />
  <Route path="/login" element={<LoginPage/>} />
 </Routes></MemoryRouter></AuthProvider></StrictMode></QueryClientProvider>)
}

it('shows recoverable startup failure, preserves inputs on failed retry and restores original route', async()=>{
 let healthy=false,refreshCount=0
 vi.stubGlobal('fetch',vi.fn(async(input:RequestInfo|URL)=>{
  if(String(input).endsWith('/auth/refresh')) { refreshCount++;return new Response(JSON.stringify(healthy?{accessToken:'synthetic'}:{code:'AUTH_SERVICE_UNAVAILABLE',message:'internal detail must not be shown'}),{status:healthy?200:500}) }
  if(String(input).endsWith('/auth/me'))return new Response(JSON.stringify(user),{status:200})
  throw new Error('Unexpected request')
 }))
 showLogin()
 await screen.findByText('暂时无法恢复登录状态，请重试或使用账号密码登录。')
 expect(refreshCount).toBe(1)
 fireEvent.change(screen.getByLabelText('账号'),{target:{value:'preserved'}})
 fireEvent.change(screen.getByLabelText('密码'),{target:{value:'PreservedPassword123'}})
 fireEvent.click(screen.getByRole('button',{name:'重试恢复登录'}))
 await waitFor(()=>expect(refreshCount).toBe(2))
 await waitFor(()=>expect(screen.getByRole('button',{name:'重试恢复登录'})).not.toBeDisabled())
 expect(screen.getByLabelText('账号')).toHaveValue('preserved')
 expect(screen.getByLabelText('密码')).toHaveValue('PreservedPassword123')
 expect(screen.queryByText('internal detail must not be shown')).not.toBeInTheDocument()
 healthy=true
 fireEvent.click(screen.getByRole('button',{name:'重试恢复登录'}))
 await screen.findByText('原业务页面')
 expect(refreshCount).toBe(3)
})

it('treats network failure as recoverable without silently replaying credentials',async()=>{
 const fetchMock=vi.fn().mockRejectedValue(new TypeError('network unavailable'))
 vi.stubGlobal('fetch',fetchMock);showLogin()
 await screen.findByRole('button',{name:'重试恢复登录'})
 expect(fetchMock).toHaveBeenCalledTimes(1)
})

it('does not offer session recovery after an authoritative 401',async()=>{
 vi.stubGlobal('fetch',vi.fn().mockResolvedValue(new Response(JSON.stringify({code:'INVALID_SESSION',message:'登录状态已失效'}),{status:401})))
 showLogin();await screen.findByRole('heading',{name:'欢迎回来'})
 expect(screen.queryByRole('button',{name:'重试恢复登录'})).not.toBeInTheDocument()
})

it('deduplicates rapid retries and blocks conflicting sign-in until recovery settles',async()=>{
 let attempts=0,finish:(response:Response)=>void=()=>undefined
 const fetchMock=vi.fn(async(input:RequestInfo|URL)=>{
  if(String(input).endsWith('/auth/refresh')) {
   attempts++
   if(attempts===1)return new Response(JSON.stringify({code:'AUTH_SERVICE_UNAVAILABLE'}),{status:500})
   return new Promise<Response>(resolve=>{finish=resolve})
  }
  throw new Error('Unexpected sign-in during recovery')
 })
 vi.stubGlobal('fetch',fetchMock);showLogin()
 const retry=await screen.findByRole('button',{name:'重试恢复登录'})
 fireEvent.change(screen.getByLabelText('账号'),{target:{value:'another-account'}})
 fireEvent.change(screen.getByLabelText('密码'),{target:{value:'AnotherPassword123'}})
 fireEvent.click(retry);fireEvent.click(retry)
 await waitFor(()=>expect(retry).toBeDisabled())
 expect(screen.getByRole('button',{name:'登 录'})).toBeDisabled()
 const form=screen.getByLabelText('账号').closest('form')
 if(!form)throw new Error('Login form missing')
 fireEvent.submit(form)
 await waitFor(()=>expect(attempts).toBe(2))
 expect(fetchMock.mock.calls.every(([input])=>String(input).endsWith('/auth/refresh'))).toBe(true)
 finish(new Response(JSON.stringify({code:'INVALID_SESSION'}),{status:401}))
 await waitFor(()=>expect(screen.queryByRole('button',{name:'重试恢复登录'})).not.toBeInTheDocument())
 expect(screen.getByRole('button',{name:'登 录'})).not.toBeDisabled()
 expect(screen.getByLabelText('账号')).toHaveValue('another-account')
})

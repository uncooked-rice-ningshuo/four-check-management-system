import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App as AntDesignApp } from 'antd'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, expect, it, vi } from 'vitest'
import { AuthProvider } from '../../app/providers/AuthProvider'
import { ProtectedRoute } from '../../app/router/ProtectedRoute'
import { AccountSecurityPage } from '../admin/AccountSecurityPage'
import { ChangePasswordPage } from './ChangePasswordPage'

afterEach(()=>vi.unstubAllGlobals())
it.each(['initial','security'])('keeps %s password inputs on failure and ends the session once after successful change',async(kind)=>{
 let fail=true
 const fetchMock=vi.fn(async(input:RequestInfo|URL)=>{
  const path=String(input)
  if(path.endsWith('/auth/refresh'))return new Response(JSON.stringify({accessToken:'synthetic'}),{status:200})
  if(path.endsWith('/auth/me'))return new Response(JSON.stringify({id:'fixture',name:'Fixture',mustChangePassword:false}),{status:200})
  if(path.endsWith('/auth/change-password'))return fail?new Response(JSON.stringify({code:'AUTH_SERVICE_UNAVAILABLE',message:'认证服务暂时不可用，请稍后重试'}),{status:500}):new Response(null,{status:204})
  if(path.endsWith('/auth/logout'))return new Response(JSON.stringify({code:'AUTH_SERVICE_UNAVAILABLE',message:'Unexpected second mutation'}),{status:500})
  throw new Error('Unexpected path')
 })
 vi.stubGlobal('fetch',fetchMock)
 const client=new QueryClient({defaultOptions:{queries:{retry:false}}})
 render(<AntDesignApp><QueryClientProvider client={client}><AuthProvider><MemoryRouter initialEntries={['/change']}><Routes>
  <Route path="/change" element={<ProtectedRoute>{kind==='initial'?<ChangePasswordPage/>:<AccountSecurityPage/>}</ProtectedRoute>}/>
  <Route path="/login" element={<div>重新登录</div>}/>
 </Routes></MemoryRouter></AuthProvider></QueryClientProvider></AntDesignApp>)
 await screen.findByLabelText('当前密码')
 client.setQueryData(['private-fixture'],{name:'cached sensitive data'})
 for(const [label,value] of [['当前密码','InitialPassword123'],['新密码','ChangedPassword456'],['确认新密码','ChangedPassword456']])fireEvent.change(screen.getByLabelText(label),{target:{value}})
 fireEvent.click(screen.getByRole('button',{name:kind==='initial'?/更新密码$/:/更新密码并重新登录$/}))
 await screen.findByText('认证服务暂时不可用，请稍后重试')
 expect(screen.getByLabelText('当前密码')).toHaveValue('InitialPassword123')
 expect(client.getQueryData(['private-fixture'])).toBeDefined()
 fail=false
 fireEvent.click(screen.getByRole('button',{name:kind==='initial'?/更新密码$/:/更新密码并重新登录$/}))
 await screen.findByText('重新登录')
 if(kind==='initial')await screen.findByText('密码已更新，请重新登录')
 expect(client.getQueryData(['private-fixture'])).toBeUndefined()
 await waitFor(()=>expect(fetchMock.mock.calls.filter(([input])=>String(input).endsWith('/auth/logout'))).toHaveLength(0))
})

it.each(['initial','security'])('blocks duplicate %s password submissions while awaiting a conflict response',async(kind)=>{
 const completions:Array<(response:Response)=>void>=[]
 const fetchMock=vi.fn(async(input:RequestInfo|URL)=>{
  const path=String(input)
  if(path.endsWith('/auth/refresh'))return new Response(JSON.stringify({accessToken:'synthetic'}),{status:200})
  if(path.endsWith('/auth/me'))return new Response(JSON.stringify({id:'fixture',name:'Fixture',mustChangePassword:false}),{status:200})
  if(path.endsWith('/auth/change-password'))return new Promise<Response>(resolve=>completions.push(resolve))
  throw new Error('Unexpected path')
 })
 vi.stubGlobal('fetch',fetchMock)
 render(<AntDesignApp><QueryClientProvider client={new QueryClient()}><AuthProvider><MemoryRouter>{kind==='initial'?<ChangePasswordPage/>:<AccountSecurityPage/>}</MemoryRouter></AuthProvider></QueryClientProvider></AntDesignApp>)
 await waitFor(()=>expect(fetchMock.mock.calls.some(([input])=>String(input).endsWith('/auth/me'))).toBe(true))
 for(const [label,value] of [['当前密码','InitialPassword123'],['新密码','ChangedPassword456'],['确认新密码','ChangedPassword456']])fireEvent.change(screen.getByLabelText(label),{target:{value}})
 const form=screen.getByLabelText('当前密码').closest('form');if(!form)throw new Error('Password form missing')
 fireEvent.submit(form);fireEvent.submit(form)
 try {
  await waitFor(()=>expect(completions.length).toBeGreaterThan(0))
  expect(screen.getByRole('button',{name:kind==='initial'?/更新密码$/:/更新密码并重新登录$/})).toBeDisabled()
  expect(completions).toHaveLength(1)
 } finally {
  for(const complete of completions)complete(new Response(JSON.stringify({code:'VERSION_CONFLICT',message:'账号状态已变化，请重新登录后重试'}),{status:409}))
  await screen.findByText('账号状态已变化，请重新登录后重试')
 }
 expect(screen.getByLabelText('当前密码')).toHaveValue('InitialPassword123')
 await waitFor(()=>expect(screen.getByRole('button',{name:kind==='initial'?/更新密码$/:/更新密码并重新登录$/})).not.toBeDisabled())
})

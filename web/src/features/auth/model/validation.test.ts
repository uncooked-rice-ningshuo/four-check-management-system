import { describe, expect, it } from 'vitest'
import { validateLogin, validateRegistration } from './validation'

describe('auth validation', () => {
  it('requires student number and password for login', () => {
    expect(validateLogin({ studentNo: '', password: '' })).toEqual({ studentNo: '请输入学号', password: '请输入密码' })
  })
  it('requires matching registration passwords with at least 8 characters', () => {
    expect(validateRegistration({ studentNo: '22010101', name: '张三', password: 'short', confirmPassword: 'different' })).toEqual({ password: '密码至少 8 位', confirmPassword: '两次输入的密码不一致' })
  })
})

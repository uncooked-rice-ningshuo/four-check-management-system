import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { AuthLayout } from './AuthLayout'
import { AUTH_REGISTER_FORM_SPACING, AUTH_SHELL_DESKTOP_LAYOUT } from './authLayoutConfig'

describe('认证页面桌面布局', () => {
  it.each(['login', 'register'] as const)('%s 页面使用统一窗口尺寸并等分左右区域', variant => {
    const { container } = render(<AuthLayout variant={variant}><div>表单</div></AuthLayout>)
    const shell = container.querySelector<HTMLElement>('.auth-shell')

    expect(shell?.style.getPropertyValue('--auth-shell-height')).toBe(AUTH_SHELL_DESKTOP_LAYOUT.height)
    expect(shell?.style.getPropertyValue('--auth-shell-columns')).toBe(AUTH_SHELL_DESKTOP_LAYOUT.columns)
  })

  it('注册表单使用紧凑的表单项与标签间距', () => {
    const { container } = render(<AuthLayout variant="register"><div>表单</div></AuthLayout>)
    const shell = container.querySelector<HTMLElement>('.auth-shell')

    expect(shell?.style.getPropertyValue('--auth-register-item-gap')).toBe(AUTH_REGISTER_FORM_SPACING.itemGap)
    expect(shell?.style.getPropertyValue('--auth-register-label-gap')).toBe(AUTH_REGISTER_FORM_SPACING.labelGap)
  })
})

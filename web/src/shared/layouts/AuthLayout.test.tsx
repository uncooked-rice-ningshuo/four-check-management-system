import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { AuthLayout } from './AuthLayout'
import { AUTH_REGISTER_FORM_SPACING, AUTH_SHELL_DESKTOP_LAYOUT } from './authLayoutConfig'

describe('认证页面桌面布局', () => {
  it.each(['login', 'register'] as const)('%s 页面使用统一左右布局，横向 Logo 位于表单侧', variant => {
    const { container } = render(<AuthLayout variant={variant}><div>表单</div></AuthLayout>)
    const shell = container.querySelector<HTMLElement>('.auth-shell')

    expect(shell?.style.getPropertyValue('--auth-shell-height')).toBe(AUTH_SHELL_DESKTOP_LAYOUT.height)
    expect(shell?.style.getPropertyValue('--auth-shell-columns')).toBe(AUTH_SHELL_DESKTOP_LAYOUT.columns)
    expect(container.querySelector('.auth-panel .auth-combination-logo')).toHaveAttribute('src', '/assets/brand/combination-logo.png')
    expect(container.querySelector('.auth-visual')).toHaveAttribute('aria-hidden', 'true')
  })

  it('注册表单使用一致的表单项与标签间距', () => {
    const { container } = render(<AuthLayout variant="register"><div>表单</div></AuthLayout>)
    const shell = container.querySelector<HTMLElement>('.auth-shell')

    expect(shell?.style.getPropertyValue('--auth-register-item-gap')).toBe(AUTH_REGISTER_FORM_SPACING.itemGap)
    expect(shell?.style.getPropertyValue('--auth-register-label-gap')).toBe(AUTH_REGISTER_FORM_SPACING.labelGap)
  })
})

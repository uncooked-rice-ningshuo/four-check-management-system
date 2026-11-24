import { afterEach, describe, expect, it } from 'vitest'
import { createIdempotencyKey, createUuid } from './idempotencyKey'

const uuidV4 = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/

const originalRandomUUID = Object.getOwnPropertyDescriptor(window.crypto, 'randomUUID')

function removeRandomUUID() {
  Object.defineProperty(window.crypto, 'randomUUID', { configurable: true, value: undefined })
}

afterEach(() => {
  if (originalRandomUUID) Object.defineProperty(window.crypto, 'randomUUID', originalRandomUUID)
  else Reflect.deleteProperty(window.crypto, 'randomUUID')
})

describe('createUuid', () => {
  it('安全上下文下直接使用原生 randomUUID', () => {
    const value = createUuid()
    expect(window.crypto.randomUUID).toBeTypeOf('function')
    expect(value).toMatch(uuidV4)
  })

  it('HTTP 非安全上下文缺少 randomUUID 时用 getRandomValues 生成合法 v4', () => {
    removeRandomUUID()
    const first = createUuid()
    const second = createUuid()
    expect(first).toMatch(uuidV4)
    expect(second).toMatch(uuidV4)
    expect(first).not.toBe(second)
  })

  it('createIdempotencyKey 在 HTTP 环境下同样可用且与 createUuid 同源', () => {
    removeRandomUUID()
    expect(createIdempotencyKey()).toMatch(uuidV4)
    expect(createUuid).toBe(createIdempotencyKey)
  })
})

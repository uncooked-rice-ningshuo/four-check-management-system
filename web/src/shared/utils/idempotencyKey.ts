/** Creates a UUID v4 idempotency key on both secure and plain-HTTP browser origins. */
export const createIdempotencyKey = (): string => {
  const browserCrypto = globalThis.crypto
  if (typeof browserCrypto.randomUUID === 'function') return browserCrypto.randomUUID()

  // randomUUID is restricted to secure contexts; getRandomValues also works on the trial's HTTP IP origin.
  const bytes = browserCrypto.getRandomValues(new Uint8Array(16))
  bytes[6] = (bytes[6] & 0x0f) | 0x40
  bytes[8] = (bytes[8] & 0x3f) | 0x80
  const hex = Array.from(bytes, value => value.toString(16).padStart(2, '0')).join('')
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
}

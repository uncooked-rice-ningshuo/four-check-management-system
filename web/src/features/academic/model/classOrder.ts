const collator = new Intl.Collator('zh-CN', { numeric: true })
const digits = '零一二三四五六七八九'
const units: Record<string, number> = { 十: 10, 百: 100, 千: 1000 }

function numericName(name: string) {
  return name.normalize('NFKC').replace(/[零〇一二两三四五六七八九十百千]+(?=\s*班|$)/gu, raw => {
    const number = raw.replaceAll('两', '二').replaceAll('〇', '零')
    if (!/[十百千]/u.test(number)) return [...number].map(char => digits.indexOf(char)).join('')
    let sum = 0
    let digit = 0
    for (const char of number) {
      if (units[char]) {
        sum += (digit || 1) * units[char]
        digit = 0
      } else digit = digits.indexOf(char)
    }
    return String(sum + digit)
  })
}

export function naturalClassOrder(ids: string[], items: { id: string; name: string }[]) {
  const names = new Map(items.map(item => [item.id, item.name]))
  return [...ids].sort((a, b) => {
    const left = names.get(a) ?? a
    const right = names.get(b) ?? b
    return collator.compare(numericName(left), numericName(right)) || collator.compare(left, right) || a.localeCompare(b)
  })
}

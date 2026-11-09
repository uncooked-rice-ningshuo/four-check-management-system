/** Formats report week and preview page numbers as common Chinese counting numerals. */
export function chineseReportNumber(value: number): string {
  if (!Number.isInteger(value) || value < 0 || value >= 10000) return String(value)
  if (value === 0) return '零'
  const digits = ['零', '一', '二', '三', '四', '五', '六', '七', '八', '九']
  const units = ['', '十', '百', '千']
  let output = ''
  let pendingZero = false
  for (let position = 3; position >= 0; position--) {
    const digit = Math.floor(value / 10 ** position) % 10
    if (digit === 0) { pendingZero = output !== ''; continue }
    if (pendingZero) output += '零'
    if (!(position === 1 && digit === 1 && output === '')) output += digits[digit]
    output += units[position]
    pendingZero = false
  }
  return output
}

const wordTitles: Record<string, string> = {
  ATTENDANCE_RATE: '到课率检查通报',
  UNATTENDED_CLASSROOM: '无机课堂检查通报',
  MORNING_ATTENDANCE: '早签通报',
  EVENING_SELF_STUDY: '晚自习检查通报',
  DORMITORY_HYGIENE: '宿舍卫生检查通报',
  PUBLIC_DORMITORY_HYGIENE: '公共宿舍卫生检查通报',
  LATE_RETURN: '晚归检查通报',
  CLASSROOM_HYGIENE: '教室卫生检查通报',
}

export function reportWordTitle(code: string, weekNo: number, fallback: string): string {
  const title = wordTitles[code] ?? fallback
  return weekNo > 0 ? `第${chineseReportNumber(weekNo)}周${title}` : title
}

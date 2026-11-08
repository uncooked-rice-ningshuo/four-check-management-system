import type { IntroductionDocument } from '../../../shared/api/reportOutputApi'

/** Converts saved legacy footer layouts to the single editable footer template. */
export function footerTemplate(document: IntroductionDocument): string {
  if (document.footerLayout === 'custom') return document.footerText ?? ''
  const text = document.footerText ?? ''
  switch (document.footerLayout) {
    case 'inline': return `${text}\t{page}`
    case 'textOnly': return text
    case 'numberOnly': return '{page}'
    default: return text ? `${text}\n{page}` : '{page}'
  }
}

export function appendHygieneSituationSnippet(current: string | undefined, snippet: string) {
  const existing = String(current ?? '').trimEnd()
  return existing ? `${existing}\n${snippet}` : snippet
}

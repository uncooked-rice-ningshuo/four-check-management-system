export type DictionaryType = { id: string; code: string; name: string; description: string; system: boolean; enabled: boolean }
export type DictionaryItem = { id: string; typeId: string; value: string; label: string; description: string; sortOrder: number; enabled: boolean }
export type DictionaryCatalog = { types: DictionaryType[]; items: DictionaryItem[] }

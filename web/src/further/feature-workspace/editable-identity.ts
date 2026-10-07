const identities = new WeakMap<object, string>()
let sequence = 0

export function editableIdentity(value: object): string {
  let identity = identities.get(value)
  if (!identity) { identity = `workspace-item-${++sequence}`; identities.set(value, identity) }
  return identity
}

export function replaceEditable<T extends object>(previous: T, change: Partial<T>): T {
  const value = { ...previous, ...change }
  identities.set(value, editableIdentity(previous))
  return value
}

const listIdentities = new WeakMap<object, string[]>()

// JSON lists can contain repeated primitive values. Keep row identities outside
// the submitted data, and transfer them whenever the editor changes a list.
export function editableListEntries<T>(value: T[]): { id: string; value: T }[] {
  let keys = listIdentities.get(value)
  if (!keys) {
    keys = value.map(() => `workspace-item-${++sequence}`)
    listIdentities.set(value, keys)
  }
  return value.map((item, index) => ({ id: keys[index], value: item }))
}

export function replaceEditableListItem<T>(previous: T[], id: string, item: T): T[] {
  const entries = editableListEntries(previous)
  const value = entries.map(entry => entry.id === id ? item : entry.value)
  listIdentities.set(value, entries.map(entry => entry.id))
  return value
}

export function removeEditableListItem<T>(previous: T[], id: string): T[] {
  const entries = editableListEntries(previous).filter(entry => entry.id !== id)
  const value = entries.map(entry => entry.value)
  listIdentities.set(value, entries.map(entry => entry.id))
  return value
}

export function appendEditableListItem<T>(previous: T[], item: T): T[] {
  const entries = editableListEntries(previous)
  const value = [...previous, item]
  listIdentities.set(value, [...entries.map(entry => entry.id), `workspace-item-${++sequence}`])
  return value
}

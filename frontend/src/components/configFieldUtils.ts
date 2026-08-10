import type { PackageConfigField } from '../store/workspaceApi'

function matchRules(rules: PackageConfigField['rules'], text: string) {
  const messages: string[] = []
  for (const rule of rules ?? []) {
    if (!rule.pattern) continue
    try {
      if (!new RegExp(rule.pattern).test(text))
        messages.push(rule.message || '格式不正确')
    } catch {
      // Invalid patterns are surfaced by the backend as declaration errors.
    }
  }
  return messages
}

export function validateFieldValue(
  field: PackageConfigField,
  value: unknown
): string[] {
  if (value === '' || value === null || value === undefined) return []
  if (field.type === 'object') return []
  if (field.type === 'array<string>' || field.type === 'array<number>') {
    if (!Array.isArray(value)) return []
    const messages: string[] = []
    for (const item of value)
      messages.push(...matchRules(field.rules, String(item)))
    return messages
  }
  return matchRules(field.rules, String(value))
}

export function sameConfigValues(
  left: Record<string, unknown>,
  right: Record<string, unknown>
) {
  const leftKeys = Object.keys(left)
  const rightKeys = Object.keys(right)
  if (leftKeys.length !== rightKeys.length) return false
  return leftKeys.every(key => deepEqual(left[key], right[key]))
}

function deepEqual(left: unknown, right: unknown): boolean {
  if (Object.is(left, right)) return true
  if (Array.isArray(left) && Array.isArray(right)) {
    if (left.length !== right.length) return false
    return left.every((item, index) => deepEqual(item, right[index]))
  }
  if (
    left &&
    right &&
    typeof left === 'object' &&
    typeof right === 'object' &&
    !Array.isArray(left) &&
    !Array.isArray(right)
  ) {
    const leftObject = left as Record<string, unknown>
    const rightObject = right as Record<string, unknown>
    const leftKeys = Object.keys(leftObject)
    const rightKeys = Object.keys(rightObject)
    if (leftKeys.length !== rightKeys.length) return false
    return leftKeys.every(key => deepEqual(leftObject[key], rightObject[key]))
  }
  return false
}

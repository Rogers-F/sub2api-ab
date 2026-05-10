import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

const expectedColumnKeys = [
  'user',
  'email',
  'id',
  'username',
  'notes',
  'role',
  'groups',
  'subscriptions',
  'balance',
  'usage',
  'concurrency',
  'status',
  'lastActive',
  'lastUsed',
  'created',
  'actions'
] as const

describe('admin users locale keys', () => {
  it('contains all users table column labels in zh', () => {
    for (const key of expectedColumnKeys) {
      expect(zh.admin.users.columns[key]).toBeTruthy()
    }
  })

  it('contains all users table column labels in en', () => {
    for (const key of expectedColumnKeys) {
      expect(en.admin.users.columns[key]).toBeTruthy()
    }
  })
})

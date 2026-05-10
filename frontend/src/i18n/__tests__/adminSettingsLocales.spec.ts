import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

const wechatConnectKeys = [
  'title',
  'description',
  'enabledLabel',
  'enabledHint',
  'appIdLabel',
  'appIdPlaceholder',
  'appSecretLabel',
  'appSecretConfiguredPlaceholder',
  'appSecretPlaceholder',
  'appSecretConfiguredHint',
  'appSecretHint',
  'modeLabel',
  'openModeLabel',
  'openModeHint',
  'mpModeLabel',
  'mpModeHint',
  'redirectUrlLabel',
  'redirectUrlPlaceholder',
  'generateAndCopy',
  'redirectUrlSetAndCopied',
  'frontendRedirectUrlLabel',
  'frontendRedirectUrlPlaceholder',
  'frontendRedirectUrlHint'
] as const

const authSourceDefaultsKeys = [
  'title',
  'description',
  'requireEmailLabel',
  'requireEmailHint',
  'enabledHint',
  'grantOnFirstBindLabel',
  'grantOnFirstBindHint',
  'defaultSubscriptionsLabel',
  'defaultSubscriptionsHint',
  'noSourceSubscriptions'
] as const

const authSources = ['email', 'linuxdo', 'oidc', 'wechat'] as const

describe('admin settings locale keys', () => {
  it('contains all WeChat Connect labels in zh and en', () => {
    for (const key of wechatConnectKeys) {
      expect(zh.admin.settings.wechatConnect[key]).toBeTruthy()
      expect(en.admin.settings.wechatConnect[key]).toBeTruthy()
    }
  })

  it('contains all auth source default labels in zh and en', () => {
    for (const key of authSourceDefaultsKeys) {
      expect(zh.admin.settings.authSourceDefaults[key]).toBeTruthy()
      expect(en.admin.settings.authSourceDefaults[key]).toBeTruthy()
    }

    for (const source of authSources) {
      expect(zh.admin.settings.authSourceDefaults.sources[source].title).toBeTruthy()
      expect(zh.admin.settings.authSourceDefaults.sources[source].description).toBeTruthy()
      expect(en.admin.settings.authSourceDefaults.sources[source].title).toBeTruthy()
      expect(en.admin.settings.authSourceDefaults.sources[source].description).toBeTruthy()
    }
  })
})

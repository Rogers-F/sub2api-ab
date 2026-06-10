import { describe, expect, it, vi } from 'vitest'

vi.mock('@/api/admin/accounts', () => ({
  getAntigravityDefaultModelMapping: vi.fn()
}))

import { buildModelMappingObject, getModelsByPlatform, getPresetMappingsByPlatform } from '../useModelWhitelist'

describe('useModelWhitelist', () => {
  it('openai 模型列表包含 GPT-5.5 与 GPT-5.4 官方模型', () => {
    const models = getModelsByPlatform('openai')

    expect(models).toContain('gpt-5.5')
    expect(models).toContain('gpt-5.5-pro')
    expect(models).toContain('gpt-5.5-2026-04-23')
    expect(models).toContain('gpt-5.4')
    expect(models).toContain('gpt-5.4-mini')
    expect(models).toContain('gpt-5.4-2026-03-05')
    expect(models).toContain('gpt-image-2')
  })

  it('openai 模型列表不再暴露已下线的 ChatGPT 登录 Codex 模型', () => {
    const models = getModelsByPlatform('openai')

    expect(models).not.toContain('gpt-5')
    expect(models).not.toContain('gpt-5.1')
    expect(models).not.toContain('gpt-5.1-codex')
    expect(models).not.toContain('gpt-5.1-codex-max')
    expect(models).not.toContain('gpt-5.1-codex-mini')
    expect(models).not.toContain('gpt-5.2-codex')
  })

  it('antigravity 模型列表包含图片模型兼容项', () => {
    const models = getModelsByPlatform('antigravity')

    expect(models).toContain('gemini-2.5-flash-image')
    expect(models).toContain('gemini-3.1-flash-image')
    expect(models).toContain('gemini-3-pro-image')
  })

  it('账号模型列表包含 Claude Opus 4.8', () => {
    expect(getModelsByPlatform('anthropic')).toContain('claude-opus-4-8')
    expect(getModelsByPlatform('antigravity')).toContain('claude-opus-4-8')
  })

  it('账号模型列表包含 Claude Fable 5', () => {
    expect(getModelsByPlatform('anthropic')).toContain('claude-fable-5')
    expect(getModelsByPlatform('antigravity')).not.toContain('claude-fable-5')
  })

  it('相关模型预设包含 Claude Opus 4.8 透传', () => {
    expect(getPresetMappingsByPlatform('anthropic')).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ label: 'Opus 4.8', from: 'claude-opus-4-8', to: 'claude-opus-4-8' })
      ])
    )
    expect(getPresetMappingsByPlatform('antigravity')).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ label: 'Opus 4.8', from: 'claude-opus-4-8', to: 'claude-opus-4-8' })
      ])
    )
    expect(getPresetMappingsByPlatform('bedrock')).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          label: 'Opus 4.8',
          from: 'claude-opus-4-8',
          to: 'us.anthropic.claude-opus-4-8-v1'
        })
      ])
    )
  })

  it('相关模型预设包含 Claude Fable 5', () => {
    expect(getPresetMappingsByPlatform('anthropic')).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ label: 'Fable 5', from: 'claude-fable-5', to: 'claude-fable-5' })
      ])
    )
    expect(getPresetMappingsByPlatform('bedrock')).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          label: 'Fable 5',
          from: 'claude-fable-5',
          to: 'anthropic.claude-fable-5'
        })
      ])
    )
  })

  it('gemini 模型列表包含原生生图模型', () => {
    const models = getModelsByPlatform('gemini')

    expect(models).toContain('gemini-2.5-flash-image')
    expect(models).toContain('gemini-3.1-flash-image')
    expect(models.indexOf('gemini-3.1-flash-image')).toBeLessThan(models.indexOf('gemini-2.0-flash'))
    expect(models.indexOf('gemini-2.5-flash-image')).toBeLessThan(models.indexOf('gemini-2.5-flash'))
  })

  it('gemini 模型列表包含当前可计费的 Gemini 2.5/3 预览模型', () => {
    const models = getModelsByPlatform('gemini')

    expect(models).toContain('gemini-2.5-flash')
    expect(models).toContain('gemini-2.5-pro')
    expect(models).toContain('gemini-3-flash-preview')
    expect(models).toContain('gemini-3.1-pro-preview')
  })

  it('antigravity 模型列表会把新的 Gemini 图片模型排在前面', () => {
    const models = getModelsByPlatform('antigravity')

    expect(models.indexOf('gemini-3.1-flash-image')).toBeLessThan(models.indexOf('gemini-2.5-flash'))
    expect(models.indexOf('gemini-2.5-flash-image')).toBeLessThan(models.indexOf('gemini-2.5-flash-lite'))
  })

  it('whitelist 模式会忽略通配符条目', () => {
    const mapping = buildModelMappingObject('whitelist', ['claude-*', 'gemini-3.1-flash-image'], [])
    expect(mapping).toEqual({
      'gemini-3.1-flash-image': 'gemini-3.1-flash-image'
    })
  })

  it('whitelist 模式会保留 GPT-5.4 官方快照的精确映射', () => {
    const mapping = buildModelMappingObject('whitelist', ['gpt-5.4-2026-03-05'], [])

    expect(mapping).toEqual({
      'gpt-5.4-2026-03-05': 'gpt-5.4-2026-03-05'
    })
  })

  it('whitelist keeps GPT-5.4 mini exact mappings', () => {
    const mapping = buildModelMappingObject('whitelist', ['gpt-5.4-mini'], [])

    expect(mapping).toEqual({
      'gpt-5.4-mini': 'gpt-5.4-mini'
    })
  })
})

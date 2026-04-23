import { applyNonStreamForceFailover } from '../accountExtraBuilder'

describe('applyNonStreamForceFailover', () => {
  it('sets the flag when enabled during create', () => {
    const extra: Record<string, unknown> = {}

    applyNonStreamForceFailover(extra, true, 'create')

    expect(extra.non_stream_force_failover_enabled).toBe(true)
  })

  it('does nothing when disabled during create', () => {
    const extra: Record<string, unknown> = {}

    applyNonStreamForceFailover(extra, false, 'create')

    expect(extra.non_stream_force_failover_enabled).toBeUndefined()
  })

  it('removes the flag when disabled during edit', () => {
    const extra: Record<string, unknown> = {
      non_stream_force_failover_enabled: true,
      keep: 'value'
    }

    applyNonStreamForceFailover(extra, false, 'edit')

    expect(extra.non_stream_force_failover_enabled).toBeUndefined()
    expect(extra.keep).toBe('value')
  })
})

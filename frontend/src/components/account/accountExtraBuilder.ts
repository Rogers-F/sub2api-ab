export function applyNonStreamForceFailover(
  extra: Record<string, unknown>,
  enabled: boolean,
  mode: 'create' | 'edit'
): void {
  if (enabled) {
    extra.non_stream_force_failover_enabled = true
  } else if (mode === 'edit') {
    delete extra.non_stream_force_failover_enabled
  }
}

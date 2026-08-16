import { describe, expect, it } from 'vitest'
import { formatDate } from './api'

describe('formatDate', () => {
  it('uses a humane empty state', () => expect(formatDate()).toBe('기록 없음'))
  it('formats an ISO date in Korean', () => expect(formatDate('2026-01-02T00:00:00Z')).toContain('2026'))
})


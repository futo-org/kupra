import { describe, expect, it } from 'vitest';
import { dnsLabelError, dnsSubdomainError } from './names.ts';

describe(dnsSubdomainError.name, () => {
  it.each(['my snapshot', 'a/b', 'Upper', 'trailing-', '.leading', 'a..b', 'a'.repeat(254)])('rejects %j', (name) => {
    expect(dnsSubdomainError(name)).toBeDefined();
  });

  it.each(['nightly-2026-11-07', 'pr-1.before.migration', 'a'.repeat(253)])('accepts %j', (name) => {
    expect(dnsSubdomainError(name)).toBeUndefined();
  });
});

describe(dnsLabelError.name, () => {
  it('rejects dots and long names, which subdomains allow', () => {
    expect(dnsLabelError('a.b')).toBeDefined();
    expect(dnsLabelError('a'.repeat(64))).toBeDefined();
    expect(dnsLabelError('pr-1234')).toBeUndefined();
  });
});

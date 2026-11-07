import { describe, expect, it } from 'vitest';
import { conditionColor, findCondition, readyColor, readyLabel } from './conditions.ts';
import type { Condition } from './types.ts';

const condition = (type: string, status: Condition['status'], reason = ''): Condition => ({
  type,
  status,
  reason,
  message: '',
  lastTransitionTime: '2026-11-07T00:00:00Z',
});

const withConditions = (...conditions: Condition[]) => ({ status: { conditions } });

describe(findCondition.name, () => {
  it('finds a condition by type', () => {
    const ready = condition('Ready', 'True');
    expect(findCondition(withConditions(condition('Stalled', 'False'), ready), 'Ready')).toBe(ready);
    expect(findCondition({}, 'Ready')).toBeUndefined();
  });
});

describe(conditionColor.name, () => {
  it.each([
    ['Ready', 'True', 'success'],
    ['Ready', 'False', 'danger'],
    ['Ready', 'Unknown', 'warning'],
    ['Stalled', 'True', 'danger'],
    ['Stalled', 'False', 'secondary'],
    ['Reconciling', 'True', 'info'],
    ['Reconciling', 'False', 'secondary'],
  ] as const)('%s=%s is %s', (type, status, color) => {
    expect(conditionColor({ type, status })).toBe(color);
  });
});

describe(readyLabel.name, () => {
  it('says Ready when ready', () => {
    expect(readyLabel(withConditions(condition('Ready', 'True', 'Succeeded')))).toBe('Ready');
  });

  it("gives the reason when it isn't", () => {
    expect(readyLabel(withConditions(condition('Ready', 'False', 'Invalid')))).toBe('Invalid');
    expect(readyLabel(withConditions(condition('Ready', 'Unknown', 'Progressing')))).toBe('Progressing');
  });

  it('is unknown without a Ready condition', () => {
    expect(readyLabel({})).toBe('Unknown');
    expect(readyColor({})).toBe('warning');
  });
});

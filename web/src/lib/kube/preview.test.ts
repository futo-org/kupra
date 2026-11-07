import { describe, expect, it } from 'vitest';
import type { Preview, PreviewApp } from './kupra.gen.ts';
import { createdByAnnotation, ownerLabel, owners, parameterRows, phaseColor, previewsOf } from './preview.ts';

const preview = (spec: Partial<Preview['spec']> = {}, annotations?: Record<string, string>): Preview => ({
  metadata: { name: 'pr-1', namespace: 'ns', annotations },
  spec: { appRef: { name: 'app' }, ...spec },
});

const app = (parameters: NonNullable<PreviewApp['spec']['parameters']>): PreviewApp => ({
  metadata: { name: 'app', namespace: 'ns' },
  spec: { parameters, template: { resources: [] } },
});

describe(owners.name, () => {
  it('lists the creator, then spec.owners, without duplicates', () => {
    const result = owners(
      preview(
        {
          owners: [
            { kind: 'Group', name: 'github:octocat', apiGroup: 'rbac.authorization.k8s.io' },
            { kind: 'User', name: 'oidc:alice' },
          ],
        },
        { [createdByAnnotation]: 'oidc:alice' },
      ),
    );
    expect(result).toEqual([
      { kind: 'User', name: 'oidc:alice', creator: true },
      { kind: 'Group', name: 'github:octocat', creator: false },
    ]);
    expect(result.map((owner) => ownerLabel(owner))).toEqual(['oidc:alice', 'Group github:octocat']);
  });

  it('is empty without a creator or owners', () => {
    expect(owners(preview())).toEqual([]);
  });
});

describe(previewsOf.name, () => {
  it("keeps the app's previews", () => {
    const other = preview({ appRef: { name: 'other' } });
    const mine = preview();
    expect(previewsOf([other, mine], 'app')).toEqual([mine]);
  });
});

describe(parameterRows.name, () => {
  it('orders parameters as the app does, with its titles and defaults, and undeclared ones last', () => {
    const rows = parameterRows(
      preview({ parameters: { zeta: 'z', tag: 'v2', extra: 'x' } }),
      app([
        { name: 'tag', title: 'Image tag' },
        { name: 'size', title: 'Size', default: 'small' },
        { name: 'empty', title: 'Empty' },
        { name: 'zeta', title: 'Zeta' },
      ]),
    );
    expect(rows).toEqual([
      { name: 'tag', title: 'Image tag', value: 'v2', isDefault: false },
      { name: 'size', title: 'Size', value: 'small', isDefault: true },
      { name: 'zeta', title: 'Zeta', value: 'z', isDefault: false },
      { name: 'extra', value: 'x', isDefault: false },
    ]);
  });

  it("shows the preview's own values without the app", () => {
    expect(parameterRows(preview({ parameters: { b: '2', a: '1' } }), undefined)).toEqual([
      { name: 'a', value: '1', isDefault: false },
      { name: 'b', value: '2', isDefault: false },
    ]);
  });
});

describe(phaseColor.name, () => {
  it('colors phases', () => {
    expect(phaseColor('Running')).toBe('success');
    expect(phaseColor('Failed')).toBe('danger');
    expect(phaseColor('Provisioning')).toBe('info');
    expect(phaseColor(undefined)).toBe('secondary');
  });
});

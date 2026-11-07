import { describe, expect, it } from 'vitest';
import {
  createdByAnnotation,
  integerError,
  missingParameters,
  ownerLabel,
  owners,
  parameterDefaults,
  parameterInput,
  parameterRows,
  phaseColor,
  withParameter,
} from './preview.ts';
import type { Preview, PreviewApp } from './types.ts';

const preview = (spec: Partial<Preview['spec']> = {}, annotations?: Record<string, string>): Preview => ({
  apiVersion: 'kupra.futo.tech/v1alpha1',
  kind: 'Preview',
  metadata: { name: 'pr-1', namespace: 'ns', annotations },
  spec: { appRef: { name: 'app' }, ...spec },
});

const app = (parameters: NonNullable<PreviewApp['spec']['parameters']>): PreviewApp => ({
  apiVersion: 'kupra.futo.tech/v1alpha1',
  kind: 'PreviewApp',
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

describe(withParameter.name, () => {
  it('sets a value', () => {
    expect(withParameter({ tag: 'main' }, { name: 'workers', type: 'integer' }, '2')).toEqual({
      tag: 'main',
      workers: '2',
    });
  });

  it('leaves out a cleared integer, so the default applies rather than an invalid empty value', () => {
    expect(withParameter({ tag: 'main', workers: '2' }, { name: 'workers', type: 'integer' }, '')).toEqual({
      tag: 'main',
    });
  });

  it('keeps an empty string, which overrides a default', () => {
    expect(withParameter({ tag: 'main' }, { name: 'tag' }, '')).toEqual({ tag: '' });
  });
});

describe('owners of service accounts', () => {
  it('keeps equally named service accounts in different namespaces apart', () => {
    const result = owners(
      preview({
        owners: [
          { kind: 'ServiceAccount', name: 'ci', namespace: 'a' },
          { kind: 'ServiceAccount', name: 'ci', namespace: 'b' },
          { kind: 'ServiceAccount', name: 'ci', namespace: 'a' },
        ],
      }),
    );
    expect(result.map(ownerLabel)).toEqual(['ServiceAccount a/ci', 'ServiceAccount b/ci']);
  });
});

describe('parameterRows with inherited names', () => {
  const declared = app([{ name: 'constructor', title: 'Constructor', default: 'x' }]);

  it.each([
    ['no values', undefined],
    ['other values', { other: 'y' }],
  ])('uses the default with %s', (_, parameters) => {
    expect(parameterRows(preview({ parameters }), declared)[0]).toEqual({
      name: 'constructor',
      title: 'Constructor',
      value: 'x',
      isDefault: true,
    });
  });
});

describe(parameterDefaults.name, () => {
  it('keeps empty defaults', () => {
    expect(
      parameterDefaults([
        { name: 'a', title: 'A', default: '' },
        { name: 'b', title: 'B' },
      ]),
    ).toEqual({ a: '' });
  });
});

describe(missingParameters.name, () => {
  it('lets a required integer be left out for its default', () => {
    expect(missingParameters([{ name: 'n', title: 'N', type: 'integer', required: true, default: '1' }], {})).toEqual(
      [],
    );
  });

  it('takes an explicitly empty required string', () => {
    expect(missingParameters([{ name: 's', title: 'S', required: true }], { s: '' })).toEqual([]);
  });

  it('reports a required parameter with neither a value nor a default', () => {
    expect(missingParameters([{ name: 's', title: 'S', required: true }], {}).map(({ name }) => name)).toEqual(['s']);
  });
});

describe(parameterInput.name, () => {
  it('offers only the enum values, whatever the type', () => {
    expect(parameterInput({ name: 'n', title: 'N', type: 'integer', enum: ['1', '2'] })).toBe('enum');
    expect(parameterInput({ name: 'b', title: 'B', type: 'boolean', enum: ['true'] })).toBe('enum');
    expect(parameterInput({ name: 'n', title: 'N', type: 'integer', enum: [] })).toBe('integer');
    expect(parameterInput({ name: 's', title: 'S' })).toBe('string');
  });
});

describe('parameterDefaults for required booleans', () => {
  it('starts a required boolean without a default at false, so the form can submit what its switch shows', () => {
    const parameters = [{ name: 'b', title: 'B', type: 'boolean' as const, required: true }];
    const values = parameterDefaults(parameters);
    expect(values).toEqual({ b: 'false' });
    expect(missingParameters(parameters, values)).toEqual([]);
  });
});

describe(integerError.name, () => {
  it('takes a 64-bit integer beyond what a JavaScript number holds exactly', () => {
    expect(integerError('9007199254740993')).toBeUndefined();
    expect(integerError('-9223372036854775808')).toBeUndefined();
  });

  it.each(['1.5', 'ten', '9223372036854775808'])('rejects %j', (value) => {
    expect(integerError(value)).toBeDefined();
  });

  it('leaves empty values to the default', () => {
    expect(integerError('')).toBeUndefined();
  });
});

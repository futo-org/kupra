import type { Color } from './conditions.ts';
import type { Preview, PreviewApp } from './types.ts';

/** createdByAnnotation records who created a Preview. */
export const createdByAnnotation = 'kupra.futo.tech/created-by';

/** sourceAnnotation records where a Preview came from, such as a pull request's URL. Whatever creates the Preview sets it. */
export const sourceAnnotation = 'kupra.futo.tech/source';

/** Phase is a Preview's phase. */
export type Phase = NonNullable<NonNullable<Preview['status']>['phase']>;

/** phaseColor returns the color for a Preview's phase. */
export const phaseColor = (phase: Phase | undefined): Color => {
  switch (phase) {
    case 'Running': {
      return 'success';
    }
    case 'Failed': {
      return 'danger';
    }
    case 'Pending':
    case 'Provisioning':
    case 'Starting': {
      return 'info';
    }
    case 'Stopping':
    case 'Stopped':
    case 'Terminating':
    case undefined: {
      return 'secondary';
    }
  }
};

/** Owner is someone who owns a Preview. */
export type Owner = {
  kind: string;
  name: string;
  /** namespace is a ServiceAccount's namespace. */
  namespace?: string;
  /** creator is set for whoever created the Preview. */
  creator: boolean;
};

const sameOwner = (a: Omit<Owner, 'creator'>, b: Omit<Owner, 'creator'>) =>
  a.kind === b.kind && a.name === b.name && (a.namespace ?? '') === (b.namespace ?? '');

/** owners returns a Preview's owners: whoever created it, then its spec.owners. */
export const owners = (preview: Preview): Owner[] => {
  const result: Owner[] = [];
  const createdBy = preview.metadata?.annotations?.[createdByAnnotation];
  if (createdBy) {
    result.push({ kind: 'User', name: createdBy, creator: true });
  }
  for (const { kind, name, namespace } of preview.spec.owners ?? []) {
    const owner = kind === 'ServiceAccount' ? { kind, name, namespace } : { kind, name };
    if (result.every((other) => !sameOwner(other, owner))) {
      result.push({ ...owner, creator: false });
    }
  }
  return result;
};

/** ParameterRow is one of a Preview's parameters, as the dashboard shows it. */
export type ParameterRow = {
  name: string;
  /** title is the PreviewApp's title for the parameter, if it has one. */
  title?: string;
  value: string;
  /** isDefault is set when the Preview gives no value and the PreviewApp's default applies. */
  isDefault: boolean;
};

/**
 * parameterRows returns a Preview's parameters, in the PreviewApp's order, with the app's defaults for those the
 * Preview leaves out. Values for parameters the app doesn't declare come last.
 */
export const parameterRows = (preview: Preview, app: PreviewApp | undefined): ParameterRow[] => {
  const values = preview.spec.parameters ?? {};
  const declared = app?.spec.parameters ?? [];
  const rows: ParameterRow[] = [];
  for (const parameter of declared) {
    // Otherwise a parameter named like an inherited method, such as constructor, would find that method.
    const value = Object.hasOwn(values, parameter.name) ? values[parameter.name] : undefined;
    if (value !== undefined) {
      rows.push({ name: parameter.name, title: parameter.title, value, isDefault: false });
    } else if (parameter.default !== undefined) {
      rows.push({ name: parameter.name, title: parameter.title, value: parameter.default, isDefault: true });
    }
  }
  const names = new Set(declared.map(({ name }) => name));
  for (const [name, value] of Object.entries(values).toSorted(([a], [b]) => a.localeCompare(b))) {
    if (!names.has(name)) {
      rows.push({ name, value, isDefault: false });
    }
  }
  return rows;
};

/** ownerLabel returns how the dashboard names an owner: users by name, service accounts by namespace/name, and others by kind and name. */
export const ownerLabel = ({ kind, name, namespace }: Owner) => {
  if (kind === 'User') {
    return name;
  }
  return kind === 'ServiceAccount' ? `${kind} ${namespace}/${name}` : `${kind} ${name}`;
};

/** withParameter returns a Preview's parameter values with one set. An empty integer is left out, so the PreviewApp's default applies; an empty string is a value of its own. */
export const withParameter = (
  values: Record<string, string>,
  { name, type }: { name: string; type?: string },
  value: string,
) => {
  const { [name]: _, ...rest } = values;
  return value === '' && type === 'integer' ? rest : { ...rest, [name]: value };
};

/** Parameter is one of a PreviewApp's parameters. */
export type Parameter = NonNullable<PreviewApp['spec']['parameters']>[number];

/**
 * parameterDefaults returns the values a new Preview starts with: the PreviewApp's defaults, empty ones included. A
 * required boolean without a default starts at its first allowed value, or false, which its switch shows.
 */
export const parameterDefaults = (parameters: Parameter[]) =>
  Object.fromEntries(
    parameters.flatMap(({ name, type, required, enum: allowed, default: value }) => {
      if (value !== undefined) {
        return [[name, value]];
      }
      return type === 'boolean' && required ? [[name, allowed?.[0] ?? 'false']] : [];
    }),
  );

const minInt64 = -(2n ** 63n);
const maxInt64 = 2n ** 63n - 1n;

/** integerError says why value isn't an integer parameter's value, a 64-bit integer as kupra parses it. Empty values are left out. */
export const integerError = (value: string) => {
  if (value === '') {
    return;
  }
  if (!/^[+-]?\d+$/.test(value)) {
    return 'must be an integer';
  }
  const number = BigInt(value);
  if (number < minInt64 || number > maxInt64) {
    return 'must be a 64-bit integer';
  }
};

/** missingParameters returns the required parameters that have neither a value nor a default. An empty value counts. */
export const missingParameters = (parameters: Parameter[], values: Record<string, string>) =>
  parameters.filter(
    (parameter) => parameter.required && !Object.hasOwn(values, parameter.name) && parameter.default === undefined,
  );

/** parameterInput returns which input a parameter gets. An enum limits any type to its values. */
export const parameterInput = ({ enum: values, type = 'string' }: Parameter) =>
  values?.length ? 'enum' : (type ?? 'string');

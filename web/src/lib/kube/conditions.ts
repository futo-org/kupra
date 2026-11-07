import type { Condition } from './types.ts';

/** Color is the color the dashboard shows a status in. */
export type Color = 'success' | 'danger' | 'warning' | 'info' | 'secondary';

type WithConditions = { status?: { conditions?: Condition[] } };

/** findCondition returns the condition of this type, if the object has one. */
export const findCondition = (object: WithConditions | undefined, type: string) =>
  object?.status?.conditions?.find((condition) => condition.type === type);

// Reconciling and Stalled are abnormal-true conditions, as in kstatus: True means something is wrong or in progress.
const abnormalTrue = new Set(['Reconciling', 'Stalled']);

/** conditionColor returns the color for a condition, by whether its status is normal for its type. */
export const conditionColor = ({ type, status }: Pick<Condition, 'type' | 'status'>): Color => {
  if (status === 'Unknown') {
    return 'warning';
  }
  const normal = abnormalTrue.has(type) ? status === 'False' : status === 'True';
  if (normal) {
    return type === 'Ready' ? 'success' : 'secondary';
  }
  return type === 'Reconciling' ? 'info' : 'danger';
};

/** readyColor returns the color for an object's Ready condition, which is unknown until the object has one. */
export const readyColor = (object: WithConditions | undefined) =>
  conditionColor(findCondition(object, 'Ready') ?? { type: 'Ready', status: 'Unknown' });

/** readyLabel returns a short label for an object's Ready condition: Ready when it's true, and its reason otherwise. */
export const readyLabel = (object: WithConditions | undefined) => {
  const ready = findCondition(object, 'Ready');
  if (ready?.status === 'True') {
    return 'Ready';
  }
  return ready?.reason || (ready?.status === 'False' ? 'Not ready' : 'Unknown');
};

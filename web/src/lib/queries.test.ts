import { QueryClient, QueryObserver } from '@tanstack/svelte-query';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { ResourceAttributes, SelfSubjectAccessReview } from './kube/types.ts';
import { accessQuery } from './queries.ts';

/** reviews are the access reviews sent, which tests answer by hand. */
let reviews: { attributes: ResourceAttributes | undefined; answer: (allowed: boolean) => void }[];

beforeEach(() => {
  reviews = [];
  vi.stubGlobal(
    'fetch',
    (_path: string, init: RequestInit) =>
      new Promise<Response>((resolve) => {
        const review = JSON.parse(init.body as string) as SelfSubjectAccessReview;
        reviews.push({
          attributes: review.spec.resourceAttributes,
          answer: (allowed) => resolve(Response.json({ status: { allowed } })),
        });
      }),
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
});

const patch = (name: string, namespace = 'ns'): ResourceAttributes => ({
  namespace,
  verb: 'patch',
  resource: 'previews',
  name,
});

/** observe starts observing an access query, and waits for its first review's answer. */
const observe = async (attributes: ResourceAttributes, revision: string, allowed: boolean) => {
  const observer = new QueryObserver(new QueryClient(), accessQuery(attributes, revision));
  const unsubscribe = observer.subscribe(() => {});
  await vi.waitFor(() => expect(reviews).toHaveLength(1));
  reviews[0]?.answer(allowed);
  await vi.waitFor(() => expect(observer.getCurrentResult().data).toBe(allowed));
  return { observer, unsubscribe };
};

describe(accessQuery.name, () => {
  it('asks again when the revision changes, keeping the previous answer until then', async () => {
    const { observer, unsubscribe } = await observe(patch('pr-1'), '1/Pending', false);

    observer.setOptions(accessQuery(patch('pr-1'), '1/Running'));
    await vi.waitFor(() => expect(reviews).toHaveLength(2));
    expect(reviews[1]?.attributes).toMatchObject({ verb: 'patch', name: 'pr-1' });
    expect(observer.getCurrentResult()).toMatchObject({ data: false, isPlaceholderData: true });

    reviews[1]?.answer(true);
    await vi.waitFor(() => expect(observer.getCurrentResult()).toMatchObject({ data: true, isPlaceholderData: false }));
    unsubscribe();
  });

  it("doesn't carry an answer over to another preview", async () => {
    const { observer, unsubscribe } = await observe(patch('pr-1'), '1/Running', true);

    for (const attributes of [patch('pr-2'), patch('pr-1', 'other')]) {
      observer.setOptions(accessQuery(attributes, '1/Running'));
      expect(observer.getCurrentResult().data).toBeUndefined();
    }
    unsubscribe();
  });
});

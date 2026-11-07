import { describe, expect, it } from 'vitest';
import { listPath, nameSelector, watchPath } from './paths.ts';

describe(listPath.name, () => {
  it('lists in a namespace', () => {
    expect(listPath('Preview', 'my-app')).toBe('/api/kube/apis/kupra.futo.tech/v1alpha1/namespaces/my-app/previews');
  });

  it('lists across namespaces', () => {
    expect(listPath('PreviewApp', undefined)).toBe('/api/kube/apis/kupra.futo.tech/v1alpha1/previewapps');
  });

  it('ignores the namespace of cluster-scoped kinds', () => {
    expect(listPath('PreviewPolicy', 'ignored')).toBe('/api/kube/apis/kupra.futo.tech/v1alpha1/previewpolicies');
  });

  it('adds selectors', () => {
    expect(listPath('Preview', 'ns', { fieldSelector: nameSelector('pr-1') })).toBe(
      '/api/kube/apis/kupra.futo.tech/v1alpha1/namespaces/ns/previews?fieldSelector=metadata.name%3Dpr-1',
    );
  });
});

describe(watchPath.name, () => {
  it('watches from a resource version, with bookmarks', () => {
    expect(watchPath('Preview', 'ns', '42')).toBe(
      '/api/kube/apis/kupra.futo.tech/v1alpha1/namespaces/ns/previews?watch=true&allowWatchBookmarks=true&resourceVersion=42',
    );
  });
});

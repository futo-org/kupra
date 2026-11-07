// A minimal client for the dashboard's backend. Kubernetes requests go through /api/kube as the signed-in user.

import type { Status } from './kube/types.ts';

/** ApiError is an error response from the backend or the Kubernetes API. */
export class ApiError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }
}

/** signIn sends the browser to sign in, and back to the current page afterwards. */
export const signIn = () => {
  const { pathname, search, hash } = location;
  location.assign(`/auth/login?${new URLSearchParams({ redirect: pathname + search + hash })}`);
};

/** request sends a request to the backend and returns its JSON body. A 401 sends the browser to sign in. */
export const request = async <T>(path: string, init: RequestInit = {}): Promise<T> => {
  const headers = new Headers(init.headers);
  if (init.method && init.method !== 'GET') {
    // The backend refuses changes without it, which keeps cross-site forms out.
    headers.set('X-Kupra-CSRF', '1');
  }
  const response = await fetch(path, { ...init, headers, credentials: 'same-origin' });
  if (response.status === 401) {
    signIn();
  }
  const text = await response.text();
  if (!response.ok) {
    let message = text.trim() || response.statusText;
    try {
      message = (JSON.parse(text) as Status).message || message;
    } catch {
      // The backend's own errors are plain text.
    }
    throw new ApiError(response.status, message);
  }
  return (text ? JSON.parse(text) : undefined) as T;
};

/** kube sends a request to the Kubernetes API, with body as JSON. */
export const kube = <T>(path: string, method = 'GET', body?: unknown, contentType = 'application/json') =>
  request<T>(`/api/kube${path}`, {
    method,
    headers: body === undefined ? {} : { 'Content-Type': contentType },
    body: body === undefined ? undefined : JSON.stringify(body),
  });

/** Session is the signed-in user, as the API server sees them. */
export type Session = { username: string; groups: string[] };

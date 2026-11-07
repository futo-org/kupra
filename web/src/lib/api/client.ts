import type { Status, WatchEvent } from '#lib/kube/meta.ts';

/** ApiError is an error response from the dashboard's backend or the Kubernetes API. */
export class ApiError extends Error {
  readonly status: number;
  readonly reason: string | undefined;

  constructor(status: number, message: string, reason?: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.reason = reason;
  }
}

/** isForbidden reports whether error is the API's answer that the user may not do something. */
export const isForbidden = (error: unknown) => error instanceof ApiError && error.status === 403;

/** isGone reports whether error says that a resource version is too old to watch from. */
export const isGone = (error: unknown) => error instanceof ApiError && error.status === 410;

/** loginPath returns the path that signs the user in and then returns to redirect. */
export const loginPath = (redirect: string) => `/auth/login?${new URLSearchParams({ redirect })}`;

const readError = async (response: Response) => {
  let status: Partial<Status> = {};
  try {
    status = (await response.json()) as Partial<Status>;
  } catch {
    // Not every error has a Status body, such as those from proxies.
  }
  return new ApiError(
    response.status,
    status.message || response.statusText || `HTTP ${response.status}`,
    status.reason,
  );
};

export type ClientOptions = {
  fetch?: typeof fetch;
  /** onUnauthorized is called when the user isn't signed in. */
  onUnauthorized?: () => void;
};

/** Client calls the dashboard's backend. */
export class Client {
  readonly #fetch: typeof fetch;
  readonly #onUnauthorized: () => void;

  constructor({ fetch: fetcher = (...args) => fetch(...args), onUnauthorized = () => {} }: ClientOptions = {}) {
    this.#fetch = fetcher;
    this.#onUnauthorized = onUnauthorized;
  }

  /** request sends a request and returns the response, or throws an ApiError for an error status. */
  async request(path: string, init: RequestInit = {}): Promise<Response> {
    const headers = new Headers(init.headers);
    const method = (init.method ?? 'GET').toUpperCase();
    if (method !== 'GET' && method !== 'HEAD') {
      headers.set('X-Kupra-CSRF', '1');
    }
    const response = await this.#fetch(path, { ...init, method, headers, credentials: 'same-origin' });
    if (response.status === 401) {
      this.#onUnauthorized();
    }
    if (!response.ok) {
      throw await readError(response);
    }
    return response;
  }

  async get<T>(path: string, signal?: AbortSignal): Promise<T> {
    const response = await this.request(path, { signal });
    return (await response.json()) as T;
  }

  async post(path: string, body?: unknown): Promise<Response> {
    return this.request(path, {
      method: 'POST',
      headers: body === undefined ? {} : { 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  }

  /** watch starts a Kubernetes watch request, and returns its events until the server ends it. */
  async watch<T>(path: string, signal?: AbortSignal): Promise<AsyncIterable<WatchEvent<T>>> {
    const response = await this.request(path, { signal });
    return readEvents<T>(response.body);
  }
}

async function* readEvents<T>(body: ReadableStream<Uint8Array> | null): AsyncGenerator<WatchEvent<T>> {
  if (!body) {
    return;
  }
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  for (;;) {
    const { done, value } = await reader.read();
    if (done) {
      break;
    }
    buffer += decoder.decode(value, { stream: true });
    const lines = buffer.split('\n');
    buffer = lines.pop() ?? '';
    for (const line of lines) {
      if (line.trim()) {
        yield JSON.parse(line) as WatchEvent<T>;
      }
    }
  }
  buffer += decoder.decode();
  if (buffer.trim()) {
    yield JSON.parse(buffer) as WatchEvent<T>;
  }
}

import { describe, expect, it, vi } from 'vitest';
import { ApiError, Client, isForbidden, loginPath } from './client.ts';

const respond = (body: BodyInit | null, init?: ResponseInit) => vi.fn(() => Promise.resolve(new Response(body, init)));

const sentHeaders = (fetch: ReturnType<typeof respond>) => {
  const init = fetch.mock.calls[0] as unknown as [string, RequestInit];
  return new Headers(init[1].headers);
};

describe(Client.name, () => {
  it("doesn't send the CSRF header with GET requests", async () => {
    const fetch = respond('{}');
    await new Client({ fetch }).get('/api/session');
    expect(sentHeaders(fetch).has('X-Kupra-CSRF')).toBe(false);
  });

  it('sends the CSRF header with other requests', async () => {
    const fetch = respond(null, { status: 204 });
    await new Client({ fetch }).post('/auth/logout');
    expect(sentHeaders(fetch).get('X-Kupra-CSRF')).toBe('1');
  });

  it('calls onUnauthorized on 401', async () => {
    const onUnauthorized = vi.fn();
    const client = new Client({ fetch: respond('', { status: 401 }), onUnauthorized });
    await expect(client.get('/api/session')).rejects.toBeInstanceOf(ApiError);
    expect(onUnauthorized).toHaveBeenCalledOnce();
  });

  it("reads errors from Kubernetes' Status", async () => {
    const status = { kind: 'Status', code: 403, reason: 'Forbidden', message: 'previews is forbidden' };
    const client = new Client({ fetch: respond(JSON.stringify(status), { status: 403 }) });
    const error = await client.get('/api/kube/apis/x').catch((error_: unknown) => error_);
    expect(error).toEqual(new ApiError(403, 'previews is forbidden', 'Forbidden'));
    expect(isForbidden(error)).toBe(true);
  });

  it('falls back to the status text for errors without a Status', async () => {
    const client = new Client({ fetch: respond('<html>', { status: 502, statusText: 'Bad Gateway' }) });
    await expect(client.get('/api/session')).rejects.toEqual(new ApiError(502, 'Bad Gateway'));
  });

  it('reads watch events split across chunks', async () => {
    const encoder = new TextEncoder();
    const chunks = [
      '{"type":"ADDED","object":{"a":1}}\n{"type":"MOD',
      'IFIED","object":{"a":2}}\n',
      '{"type":"DELETED","object":{}}',
    ];
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        for (const chunk of chunks) {
          controller.enqueue(encoder.encode(chunk));
        }
        controller.close();
      },
    });
    const events = await new Client({ fetch: respond(body) }).watch('/watch');
    const types = [];
    for await (const event of events) {
      types.push(event.type);
    }
    expect(types).toEqual(['ADDED', 'MODIFIED', 'DELETED']);
  });
});

describe(loginPath.name, () => {
  it('returns to the page', () => {
    expect(loginPath('/apps/foo?x=1')).toBe('/auth/login?redirect=%2Fapps%2Ffoo%3Fx%3D1');
  });
});

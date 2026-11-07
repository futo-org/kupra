import { Client, loginPath } from './client.ts';

/** api is the client the dashboard uses. When the user isn't signed in, it sends the browser to sign in. */
export const api = new Client({
  onUnauthorized: () => {
    const { pathname, search, hash } = location;
    location.assign(loginPath(pathname + search + hash));
  },
});

/** Session is the signed-in user. */
export type Session = {
  username: string;
  groups: string[];
};

export const getSession = (signal?: AbortSignal) => api.get<Session>('/api/session', signal);

export const logout = () => api.post('/auth/logout');

/** getAppNamespaces returns the namespaces that contain PreviewApps, whether or not the user can see them. */
export const getAppNamespaces = async (signal?: AbortSignal) => {
  const { namespaces } = await api.get<{ namespaces: string[] }>('/api/app-namespaces', signal);
  return namespaces;
};

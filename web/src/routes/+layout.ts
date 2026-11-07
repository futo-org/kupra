import { getSession } from '#lib/api/index.ts';
import type { LayoutLoad } from './$types';

export const ssr = false;

export const load: LayoutLoad = async () => ({ session: await getSession() });

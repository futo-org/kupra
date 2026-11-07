import { SvelteDate } from 'svelte/reactivity';

const now = new SvelteDate();
setInterval(() => now.setTime(Date.now()), 10_000);

/** clock is the current time, updated every ten seconds, for showing ages. */
export const clock = {
  get now() {
    return now.getTime();
  },
};

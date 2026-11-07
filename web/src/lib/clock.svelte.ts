import { createSubscriber } from 'svelte/reactivity';

/** Clock is the current time, kept up to date every interval for as long as anything reads it in an effect. */
export class Clock {
  readonly #subscribe: () => void;

  constructor(interval: number) {
    this.#subscribe = createSubscriber((update) => {
      const timer = setInterval(update, interval);
      return () => clearInterval(timer);
    });
  }

  /** now returns the current time in milliseconds. */
  get now() {
    this.#subscribe();
    return Date.now();
  }
}

/** clock moves relative times on, such as "2 minutes ago", with one timer however many are shown. */
export const clock = new Clock(10_000);

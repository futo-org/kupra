/** maxCollections is how many collections one GET /api/watch may ask for, as the backend's watch.go limits it. */
export const maxCollections = 50;

/** chunk splits items into groups of at most size. */
export const chunk = <T>(items: T[], size: number): T[][] =>
  Array.from({ length: Math.ceil(items.length / size) }, (_, i) => items.slice(i * size, (i + 1) * size));

/** age formats the time since timestamp the way kubectl does, such as 5m or 3d. */
export const age = (timestamp: string | undefined, now: number = Date.now()) => {
  if (!timestamp) {
    return '';
  }
  const seconds = Math.max(0, Math.floor((now - Date.parse(timestamp)) / 1000));
  if (seconds < 120) {
    return `${seconds}s`;
  }
  const minutes = Math.floor(seconds / 60);
  if (minutes < 10) {
    return `${minutes}m${seconds % 60 ? `${seconds % 60}s` : ''}`;
  }
  if (minutes < 180) {
    return `${minutes}m`;
  }
  const hours = Math.floor(minutes / 60);
  if (hours < 8) {
    return `${hours}h${minutes % 60 ? `${minutes % 60}m` : ''}`;
  }
  if (hours < 48) {
    return `${hours}h`;
  }
  const days = Math.floor(hours / 24);
  if (days < 8) {
    return `${days}d${hours % 24 ? `${hours % 24}h` : ''}`;
  }
  if (days < 365 * 2) {
    return `${days}d`;
  }
  const years = Math.floor(days / 365);
  if (years < 8) {
    return `${years}y${days % 365 ? `${days % 365}d` : ''}`;
  }
  return `${years}y`;
};

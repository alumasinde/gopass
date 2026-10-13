/**
 * Format a UTC date string to the user's local timezone
 * @param utcDateString - ISO 8601 date string in UTC
 * @returns Formatted date string in local timezone
 */
export function formatLocalDate(utcDateString: string): string {
  const date = new Date(utcDateString);
  return date.toLocaleString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

/**
 * Format a UTC date string to a short date only (no time)
 * @param utcDateString - ISO 8601 date string in UTC
 * @returns Formatted date string in local timezone
 */
export function formatLocalDateOnly(utcDateString: string): string {
  const date = new Date(utcDateString);
  return date.toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  });
}

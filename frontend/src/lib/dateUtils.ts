/**
 * The backend stores and returns all timestamps in UTC (UTC_TIMESTAMP() in SQL,
 * RFC3339 with "Z" in JSON). Convert to Kenyan time (EAT, UTC+3) only for display.
 */
const DISPLAY_TZ = 'Africa/Nairobi';

/**
 * Parse an API/DB timestamp as UTC. If the string has no timezone marker
 * (e.g. "2026-10-13 14:14:00"), `new Date()` would treat it as the device's
 * local time, so we append "Z" to force UTC.
 */
function parseUtc(value: string): Date {
  const s = value.trim();
  const hasTz = /([zZ]|[+-]\d{2}:?\d{2})$/.test(s);
  return new Date(hasTz ? s : s.replace(' ', 'T') + 'Z');
}

/** Date + time in Kenyan time, e.g. "13 Oct 2026, 17:14" */
export function formatLocalDate(utcDateString: string): string {
  const date = parseUtc(utcDateString);
  if (Number.isNaN(date.getTime())) return '';
  return date.toLocaleString('en-KE', {
    timeZone: DISPLAY_TZ,
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  });
}

/** Date only in Kenyan time */
export function formatLocalDateOnly(utcDateString: string): string {
  const date = parseUtc(utcDateString);
  if (Number.isNaN(date.getTime())) return '';
  return date.toLocaleDateString('en-KE', {
    timeZone: DISPLAY_TZ,
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  });
}
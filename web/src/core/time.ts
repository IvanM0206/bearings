/** Timestamps and dates (§4). */

const TIMESTAMP_RE = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})Z$/;
const DATE_RE = /^(\d{4})-(\d{2})-(\d{2})$/;

export const MIN_DATE = '1970-01-01';
export const MAX_DATE = '9999-12-31';

function isRealDate(y: number, m: number, d: number): boolean {
  if (m < 1 || m > 12 || d < 1) return false;
  const days = [31, isLeap(y) ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31][m - 1] ?? 0;
  return d <= days;
}

function isLeap(y: number): boolean {
  return (y % 4 === 0 && y % 100 !== 0) || y % 400 === 0;
}

/** `YYYY-MM-DD`, a real calendar date between 1970-01-01 and 9999-12-31. */
export function isDate(value: string): boolean {
  const m = DATE_RE.exec(value);
  if (!m) return false;
  return isRealDate(Number(m[1]), Number(m[2]), Number(m[3])) && value >= MIN_DATE && value <= MAX_DATE;
}

/**
 * RFC 3339 in UTC with second precision and `Z`, e.g. `2026-11-03T14:05:09Z`.
 * The date part follows the same range as dates. Leap seconds are rejected.
 */
export function isTimestamp(value: string): boolean {
  const m = TIMESTAMP_RE.exec(value);
  if (!m) return false;
  const date = value.slice(0, 10);
  if (!isDate(date)) return false;
  return Number(m[4]) <= 23 && Number(m[5]) <= 59 && Number(m[6]) <= 59;
}

/** Current time as a spec timestamp. */
export function nowTimestamp(now: Date = new Date()): string {
  return now.toISOString().slice(0, 19) + 'Z';
}

/** Local calendar date (the user's "today") as `YYYY-MM-DD`. */
export function localDate(now: Date = new Date()): string {
  const y = String(now.getFullYear()).padStart(4, '0');
  const m = String(now.getMonth() + 1).padStart(2, '0');
  const d = String(now.getDate()).padStart(2, '0');
  return `${y}-${m}-${d}`;
}

/** The date `days` days before `date` (both `YYYY-MM-DD`). */
export function addDays(date: string, days: number): string {
  const t = Date.UTC(Number(date.slice(0, 4)), Number(date.slice(5, 7)) - 1, Number(date.slice(8, 10)));
  return new Date(t + days * 86_400_000).toISOString().slice(0, 10);
}

/** A Date at noon UTC on `date`, for formatting with `timeZone: 'UTC'`. */
export function dateAtNoonUtc(date: string): Date {
  return new Date(`${date}T12:00:00Z`);
}

import { useEffect, useState } from 'react';
import { formatDistanceToNow } from 'date-fns';

interface RelativeTimeProps {
  /** ISO 8601 timestamp string. */
  time: string;
  /** Refresh interval in ms. Default 30s — granular enough for "5m ago" copy. */
  intervalMs?: number;
}

// RelativeTime auto-refreshes its label so a row that says "1 minute ago"
// rolls forward without a page refetch. The full ISO timestamp is exposed
// via the title attribute so the user can hover for the precise time.
export function RelativeTime({ time, intervalMs = 30_000 }: RelativeTimeProps) {
  const [, tick] = useState(0);

  useEffect(() => {
    const id = window.setInterval(() => tick(t => t + 1), intervalMs);
    return () => window.clearInterval(id);
  }, [intervalMs]);

  const date = new Date(time);
  if (Number.isNaN(date.getTime())) return <span>—</span>;

  return (
    <time dateTime={time} title={date.toISOString()}>
      {formatDistanceToNow(date, { addSuffix: true })}
    </time>
  );
}

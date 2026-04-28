-- Postgres LISTEN/NOTIFY-backed change feed.
--
-- The backend has one process-wide goroutine that LISTENs on 'uau_events'
-- and fans notifications out to every connected /api/v1/events WebSocket.
-- This trigger is the publisher; pkg/events is the subscriber. Coalescing
-- changes from any source — agent reports, manual edits, finished runs —
-- into a single channel means the API can have one cheap reconnect path
-- instead of one-per-resource subscriptions.
--
-- Payload schema (kept small so we stay well under the 8 KB pg_notify cap):
--   { "table": "hosts" | "update_runs", "op": "INSERT" | "UPDATE" | "DELETE", "id": <int> }
-- The frontend uses { table, id } as a hint to fetch the canonical row via
-- REST. We never publish row contents — that would race with the writer's
-- transaction commit and bloat the channel.

CREATE OR REPLACE FUNCTION uau_notify_event() RETURNS TRIGGER AS $$
DECLARE
    rec_id INTEGER;
BEGIN
    -- DELETE only has OLD; INSERT/UPDATE only have NEW. Pick whichever has
    -- a usable id — both share the same SERIAL primary key column name.
    IF TG_OP = 'DELETE' THEN
        rec_id := OLD.id;
    ELSE
        rec_id := NEW.id;
    END IF;

    PERFORM pg_notify(
        'uau_events',
        json_build_object(
            'table', TG_TABLE_NAME,
            'op',    TG_OP,
            'id',    rec_id
        )::text
    );

    -- AFTER triggers can't influence the row anyway; return value is ignored.
    RETURN NULL;
END $$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS hosts_notify_event ON hosts;
CREATE TRIGGER hosts_notify_event
    AFTER INSERT OR UPDATE OR DELETE ON hosts
    FOR EACH ROW EXECUTE FUNCTION uau_notify_event();

DROP TRIGGER IF EXISTS update_runs_notify_event ON update_runs;
CREATE TRIGGER update_runs_notify_event
    AFTER INSERT OR UPDATE OR DELETE ON update_runs
    FOR EACH ROW EXECUTE FUNCTION uau_notify_event();

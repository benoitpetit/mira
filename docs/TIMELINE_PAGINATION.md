# Timeline dates and pagination

`since` and `until` accept RFC3339 timestamps (including offsets) and ISO calendar dates (`YYYY-MM-DD`). Timestamp offsets are normalized to UTC. Calendar-date `since` values include the day from UTC midnight; calendar-date `until` values include the full UTC day. Both timestamp bounds are inclusive.

Timeline results keep their existing display timestamp format. Pass `next_cursor` back unchanged to continue pagination. New cursors use the opaque versioned format `v1:<RFC3339 timestamp>|<UUID>` and order rows by timestamp descending, then UUID descending. The timestamp reflects the precision currently stored by MIRA: Unix seconds in the SQLite/PostgreSQL `extracted_at` numeric column. No schema migration is needed for this behavior; increasing stored precision would require a separate data-format migration.

Legacy RFC3339 timestamp-only cursors remain accepted. Because they do not contain a row ID, resuming from one cannot continue inside a group of rows with equal timestamps; rows at that exact timestamp are skipped. Use the returned versioned cursor for reliable pagination through ties.

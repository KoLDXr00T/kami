// Package eventbus is Kami's append-only event log (local SQLite) with live
// subscribers. Events are a record of what happened; state is never rebuilt
// from them.
package eventbus

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Event is one log entry.
type Event struct {
	ID     int64          `json:"id"`
	TS     time.Time      `json:"ts"`
	Kekkai string         `json:"kekkai,omitempty"`
	Run    string         `json:"run,omitempty"`
	Type   string         `json:"type"`
	Data   map[string]any `json:"data,omitempty"`
}

// Filter selects events. Empty fields match everything; Type matches a prefix
// ("contract." matches every contract event).
type Filter struct {
	Kekkai, Run, Type string
}

func (f Filter) match(e Event) bool {
	return (f.Kekkai == "" || f.Kekkai == e.Kekkai) && (f.Run == "" || f.Run == e.Run) &&
		(f.Type == "" || strings.HasPrefix(e.Type, f.Type))
}

// Bus is the log plus its subscribers.
type Bus struct {
	db   *sql.DB
	mu   sync.Mutex
	subs map[chan Event]Filter
}

// Open opens (or creates) the log at path.
func Open(path string) (*Bus, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS events(id INTEGER PRIMARY KEY AUTOINCREMENT,
ts TEXT NOT NULL, kekkai TEXT, run TEXT, type TEXT NOT NULL, data TEXT);
CREATE INDEX IF NOT EXISTS events_run ON events(run);`); err != nil {
		db.Close()
		return nil, fmt.Errorf("events.db: %w", err)
	}
	return &Bus{db: db, subs: map[chan Event]Filter{}}, nil
}

// Close closes the log.
func (b *Bus) Close() error { return b.db.Close() }

// Append writes an event and fans it out. A write error is returned but the
// event still reaches live subscribers: the log is a record, not state.
func (b *Bus) Append(kekkai, run, typ string, data map[string]any) (Event, error) {
	e := Event{TS: time.Now().UTC(), Kekkai: kekkai, Run: run, Type: typ, Data: data}
	raw, _ := json.Marshal(data)
	b.mu.Lock()
	defer b.mu.Unlock()
	res, err := b.db.Exec(`INSERT INTO events(ts,kekkai,run,type,data) VALUES(?,?,?,?,?)`,
		e.TS.Format(time.RFC3339Nano), e.Kekkai, e.Run, e.Type, string(raw))
	if err == nil {
		e.ID, _ = res.LastInsertId()
	}
	for ch, f := range b.subs {
		if f.match(e) {
			select {
			case ch <- e:
			default: // a slow reader drops events rather than stalling the engine
			}
		}
	}
	return e, err
}

// Since returns events after id that match f, oldest first, at most limit.
func (b *Bus) Since(id int64, f Filter, limit int) ([]Event, error) {
	q := `SELECT id, ts, kekkai, run, type, data FROM events WHERE id > ?`
	args := []any{id}
	if f.Kekkai != "" {
		q += ` AND kekkai = ?`
		args = append(args, f.Kekkai)
	}
	if f.Run != "" {
		q += ` AND run = ?`
		args = append(args, f.Run)
	}
	if f.Type != "" {
		q += ` AND type LIKE ? ESCAPE '\'`
		args = append(args, strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(f.Type)+"%")
	}
	q += ` ORDER BY id LIMIT ?`
	args = append(args, limit)
	b.mu.Lock()
	rows, err := b.db.Query(q, args...)
	b.mu.Unlock()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var ts string
		var kek, run, data sql.NullString
		if err := rows.Scan(&e.ID, &ts, &kek, &run, &e.Type, &data); err != nil {
			return nil, err
		}
		e.TS, _ = time.Parse(time.RFC3339Nano, ts)
		e.Kekkai, e.Run = kek.String, run.String
		if data.Valid && data.String != "" && data.String != "null" {
			json.Unmarshal([]byte(data.String), &e.Data)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Subscribe returns a channel of new events matching f and a cancel func.
func (b *Bus) Subscribe(f Filter) (<-chan Event, func()) {
	ch := make(chan Event, 256)
	b.mu.Lock()
	b.subs[ch] = f
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		if _, ok := b.subs[ch]; ok {
			delete(b.subs, ch)
			close(ch)
		}
		b.mu.Unlock()
	}
}

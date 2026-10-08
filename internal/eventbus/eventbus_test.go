package eventbus

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAppendSinceSubscribe(t *testing.T) {
	b, err := Open(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	ch, cancel := b.Subscribe(Filter{Type: "contract."})
	defer cancel()
	b.Append("test", "r1", "run.started", nil)
	b.Append("test", "r1", "contract.opened", map[string]any{"id": "c1"})
	b.Append("other", "r2", "contract_x", nil) // underscore must not match as a LIKE wildcard
	select {
	case e := <-ch:
		if e.Type != "contract.opened" || e.Data["id"] != "c1" {
			t.Fatalf("got %+v", e)
		}
	case <-time.After(time.Second):
		t.Fatal("no event")
	}
	all, _ := b.Since(0, Filter{}, 100)
	if len(all) != 3 {
		t.Fatalf("since = %d events", len(all))
	}
	got, _ := b.Since(0, Filter{Type: "contract."}, 100)
	if len(got) != 1 {
		t.Fatalf("type filter = %+v", got)
	}
	got, _ = b.Since(all[0].ID, Filter{Run: "r1"}, 100)
	if len(got) != 1 || got[0].Type != "contract.opened" {
		t.Fatalf("since+run = %+v", got)
	}
}

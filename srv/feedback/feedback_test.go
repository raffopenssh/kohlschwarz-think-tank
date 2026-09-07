package feedback

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestPromptHints(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	db.ExecContext(ctx, `CREATE TABLE feedback_log (id INTEGER PRIMARY KEY AUTOINCREMENT, at TEXT DEFAULT '', radar TEXT, item_id INTEGER, action TEXT, reason TEXT DEFAULT '', detail TEXT DEFAULT '', title TEXT DEFAULT '', org TEXT DEFAULT '')`)
	if h := PromptHints(ctx, db, "job", 40); h != "" {
		t.Fatalf("expected empty hints, got %q", h)
	}
	Log(ctx, db, "job", 1, "trash", "level", "", "Ranger", "ZAWA")
	Log(ctx, db, "job", 1, "trash", "level", "", "Ranger", "ZAWA") // duplicate collapses
	Log(ctx, db, "job", 2, "up", "", "", "Park Director", "APN")
	Log(ctx, db, "grant", 3, "trash", "eligible", "", "Some grant", "eu")
	h := PromptHints(ctx, db, "job", 40)
	for _, want := range []string{"- Ranger (ZAWA) — too junior", "+ Park Director (APN)"} {
		if !strings.Contains(h, want) {
			t.Errorf("hints missing %q:\n%s", want, h)
		}
	}
	if strings.Count(h, "Ranger") != 1 || strings.Contains(h, "Some grant") {
		t.Errorf("dedupe/radar filter broken:\n%s", h)
	}
}

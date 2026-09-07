package jobs

import (
	"context"
	"os"
	"testing"
	"time"
)

// go test -run TestAustriaLive -v ./srv/jobs -args -live   (network; skipped by default)
func TestAustriaLive(t *testing.T) {
	if os.Getenv("JOBS_LIVE") == "" {
		t.Skip("set JOBS_LIVE=1")
	}
	only := os.Getenv("JOBS_ONLY")
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	for _, s := range Sources {
		if s.Region != "austria" || s.Kind == "linkedin" {
			continue
		}
		if only != "" && s.Name != only {
			continue
		}
		ps, err := Fetch(ctx, s)
		m := 0
		for _, p := range ps {
			if Match(p) {
				m++
				t.Logf("  ✓ %s | %s | %s", p.Title, p.Org, p.URL)
			}
		}
		t.Logf("%-45s kind=%-11s found=%3d matched=%2d err=%v", s.Name, s.Kind, len(ps), m, err)
	}
}

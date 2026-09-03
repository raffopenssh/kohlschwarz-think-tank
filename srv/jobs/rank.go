package jobs

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Model is the cheap ranker used through the exe.dev keyless gateway.
const Model = "fireworks/muse-glimmer-30b"

// Fireworks list price for muse-glimmer-30b (USD per 1M tokens). Reasoning tokens bill as output.
const (
	priceInPerM  = 0.35
	priceOutPerM = 1.50
)

const llmURL = "https://llm.int.exe.xyz/v1/chat/completions"

// MaxMonthUSD is the hard monthly LLM budget (default $0.17); ranking stops when exceeded.
func MaxMonthUSD() float64 {
	if v, err := strconv.ParseFloat(os.Getenv("JOBS_LLM_BUDGET_USD"), 64); err == nil && v > 0 {
		return v
	}
	return 0.17
}

const systemPrompt = `You rank job postings, tenders and consultancy calls for ONE specific person. Be strict.

THE PERSON: former national park director, senior protected-area (PA) leader, fluent EN/DE/FR. Interested in EXACTLY two things:
  A) Leading a park: Director / CEO / general manager / park manager / warden-in-charge / Geschäftsführer / Nationalparkdirektor / directeur / conservateur of a national park, protected area, reserve, conservancy or PA agency. Also deputy director of a large park in Africa.
  B) High-level consultancy for park directors, PA agencies or ministries: PA management plans, PA governance / finance / institutional reform, PA system reviews & strategies, park business plans, mid-term reviews / evaluations of park PPPs (public-private / collaborative management partnerships, e.g. African Parks, Peace Parks, WCS, FZS, Noé, Wildlife Alliance mandates), evaluations of PA programmes, team leader or key expert (senior) in EU (NaturAfrica, DG INTPA), GIZ, KfW, AFD, UNDP, GEF, World Bank, IUCN tenders that are about parks / PAs. Anything issued by or about African Parks Network is high interest.
Regions: Austria and Sub-Saharan Africa = top priority; global / EU-level = good. Other regions only for exceptional director or consultancy roles.

NOT of interest (score 0-20): anything below director/senior level (officer, coordinator, specialist, ranger, researcher, M&E, comms, finance, HR, fundraising, tourism ops), generic conservation-NGO staff roles, species/research projects, fellowships, internships, tenders for goods/works/IT/supplies, car parks, urban parks, business/industrial parks, forestry harvesting, agriculture, climate/energy unrelated to PAs.

SCORING 0-100:
  85-100: park director/CEO/manager (Austria, SSA, or global), or senior PA consultancy clearly for park authorities/ministries.
  65-84: senior PA-management role or PA consultancy where seniority/PA-focus is likely but not explicit; head of conservation / landscape director in SSA.
  35-64: senior conservation leadership only partly about PAs (e.g. country director of a conservation NGO), or PA tenders of unclear level.
  0-34: everything else.

OUTPUT: a JSON array, one object per input id, no prose:
[{"id":<int>,"score":<0-100>,"region":"austria|ssa|global|other","kind":"director|consultancy|senior|other","why":"<=12 words, English"}]`

type rankResult struct {
	ID     int64  `json:"id"`
	Score  int    `json:"score"`
	Region string `json:"region"`
	Kind   string `json:"kind"`
	Why    string `json:"why"`
}

var jsonArrRe = regexp.MustCompile(`(?s)\[\s*\{.*\}\s*\]`)

// RankPending scores unranked postings in batches, respecting the monthly budget.
func RankPending(ctx context.Context, db *sql.DB, maxItems int) Run {
	run := Run{Started: time.Now().UTC().Format("2006-01-02 15:04:05"), Kind: "rank", Model: Model}
	var logb strings.Builder
	cost := GetCost(ctx, db)
	budget := MaxMonthUSD()
	if cost.MonthUSD >= budget {
		fmt.Fprintf(&logb, "budget exhausted: %s >= %s this month; skipping\n", usd(cost.MonthUSD), usd(budget))
		run.Log = logb.String()
		insertRun(ctx, db, run)
		return run
	}
	rows, err := Unranked(ctx, db, maxItems)
	if err != nil {
		run.Log = "unranked: " + err.Error()
		insertRun(ctx, db, run)
		return run
	}
	if len(rows) == 0 {
		run.Log = "nothing to rank"
		insertRun(ctx, db, run)
		return run
	}
	const batch = 12
	spent := 0.0
	for i := 0; i < len(rows); i += batch {
		if cost.MonthUSD+spent >= budget {
			fmt.Fprintf(&logb, "budget reached mid-run after %d items\n", run.Ranked)
			break
		}
		end := min(i+batch, len(rows))
		res, in, out, err := rankBatch(ctx, rows[i:end])
		run.InTokens += in
		run.OutTokens += out
		c := float64(in)*priceInPerM/1e6 + float64(out)*priceOutPerM/1e6
		spent += c
		run.CostUSD += c
		if err != nil {
			fmt.Fprintf(&logb, "✗ batch %d: %v\n", i/batch, err)
			slog.Warn("jobs rank", "error", err)
			continue
		}
		for _, r := range res {
			if r.Score < 0 {
				r.Score = 0
			}
			if r.Score > 100 {
				r.Score = 100
			}
			_, err := db.ExecContext(ctx, `UPDATE job_postings SET score = ?, kind = ?, why = ?, scored_at = datetime('now'),
				region = CASE WHEN ? IN ('austria','ssa','global','other') THEN ? ELSE region END WHERE id = ? AND score IS NULL`,
				r.Score, r.Kind, truncate(r.Why, 120), r.Region, r.Region, r.ID)
			if err == nil {
				run.Ranked++
			}
		}
		fmt.Fprintf(&logb, "✓ batch %d: %d scored, %d in / %d out tokens, %s\n", i/batch, len(res), in, out, usd(c))
	}
	run.Log = logb.String()
	if err := insertRun(ctx, db, run); err != nil {
		slog.Warn("jobs insert rank run", "error", err)
	}
	return run
}

func rankBatch(ctx context.Context, rows []Row) ([]rankResult, int64, int64, error) {
	var sb strings.Builder
	for _, r := range rows {
		fmt.Fprintf(&sb, "id=%d | %s | org: %s | loc: %s | posted: %s | deadline: %s | src: %s\n  %s\n",
			r.ID, r.Title, r.Org, r.Location, r.Posted, r.Deadline, r.Source, truncate(r.Snippet, 350))
	}
	body, _ := json.Marshal(map[string]any{
		"model":       Model,
		"temperature": 0,
		"max_tokens":  600 + 220*len(rows), // room for reasoning + JSON
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": sb.String()},
		},
	})
	req, _ := http.NewRequestWithContext(ctx, "POST", llmURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 120 * time.Second}).Do(req)
	if err != nil {
		return nil, 0, 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	var r struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			In  int64 `json:"prompt_tokens"`
			Out int64 `json:"completion_tokens"`
		} `json:"usage"`
		Error any `json:"error"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, 0, 0, fmt.Errorf("decode (HTTP %d): %.200s", resp.StatusCode, b)
	}
	if r.Error != nil || len(r.Choices) == 0 {
		return nil, r.Usage.In, r.Usage.Out, fmt.Errorf("llm error (HTTP %d): %.300s", resp.StatusCode, b)
	}
	content := r.Choices[0].Message.Content
	m := jsonArrRe.FindString(content)
	if m == "" {
		return nil, r.Usage.In, r.Usage.Out, fmt.Errorf("no JSON array in reply: %.200s", content)
	}
	var res []rankResult
	if err := json.Unmarshal([]byte(m), &res); err != nil {
		return nil, r.Usage.In, r.Usage.Out, fmt.Errorf("parse JSON: %w", err)
	}
	// Only accept ids we sent.
	valid := map[int64]bool{}
	for _, row := range rows {
		valid[row.ID] = true
	}
	var out []rankResult
	for _, x := range res {
		if valid[x.ID] {
			out = append(out, x)
		}
	}
	return out, r.Usage.In, r.Usage.Out, nil
}

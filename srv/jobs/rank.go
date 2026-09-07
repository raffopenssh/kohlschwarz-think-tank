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
	"srv.exe.dev/srv/feedback"
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

// MaxMonthUSD is the hard monthly LLM budget (default $0.30: ~$0.06 rank,
// ~$0.06 briefs, ~$0.04 weekly summaries at steady state); LLM steps stop when exceeded.
func MaxMonthUSD() float64 {
	if v, err := strconv.ParseFloat(os.Getenv("JOBS_LLM_BUDGET_USD"), 64); err == nil && v > 0 {
		return v
	}
	return 0.30
}

const systemPrompt = `You rank job postings, tenders and consultancy calls for ONE specific person. Be strict.

THE PERSON: former national park director, senior protected-area (PA) leader, fluent EN/DE/FR. Interested in EXACTLY three things:
  A) Leading a park: Director / CEO / general manager / park manager / warden-in-charge / Geschäftsführer / Nationalparkdirektor / directeur / conservateur of a national park, protected area, reserve, conservancy or PA agency. Also deputy director of a large park in Africa.
  B) High-level consultancy for park directors, PA agencies or ministries: PA management plans, PA governance / finance / institutional reform, PA system reviews & strategies, park business plans, mid-term reviews / evaluations of park PPPs (public-private / collaborative management partnerships, e.g. African Parks, Peace Parks, WCS, FZS, Noé, Wildlife Alliance mandates), evaluations of PA programmes, team leader or key expert (senior) in EU (NaturAfrica, DG INTPA), GIZ, KfW, AFD, UNDP, GEF, World Bank, IUCN tenders that are about parks / PAs. Anything issued by or about African Parks Network is high interest.
  C) AUSTRIA "foot in the door": a substantive professional post inside the public authority that governs / co-funds an Austrian national park, as a strategic step towards directing that park. Austrian park directors are appointed by Land + BML; the people who get those jobs usually come from the Land's Naturschutz department or the ministry's Nationalpark section. Relevant authorities and the park they lead to:
     Land OÖ (Abt. Naturschutz, Direktion Umwelt und Wasserwirtschaft) → NP Kalkalpen · Land Steiermark Abt. 13 Umwelt und Raumordnung / Referat Naturschutz → NP Gesäuse · Land NÖ Abt. Naturschutz (RU5) → NP Donau-Auen, NP Thayatal · Land Burgenland Abt. 4 Naturschutz → NP Neusiedler See · Land Salzburg Abt. 5 Natur- und Umweltschutz, Land Tirol Abt. Umweltschutz, Land Kärnten Abt. 8 Umwelt/Naturschutz → NP Hohe Tauern · Stadt Wien MA 22 Umweltschutz / MA 49 Forst → NP Donau-Auen, Biosphärenpark Wienerwald · BML Sektion Nationalparks/Naturschutz, Umweltbundesamt, Österreichische Bundesforste Naturraummanagement → all parks.
     COUNTS as C (score 65-84): Referent/in, Amtssachverständige/r, Jurist/in, Fachexperte/in, Fachbereichs-/Projekt-/Gruppenleitung, Natura-2000 / Schutzgebiets- / Biodiversitäts-Beauftragte, wissenschaftlicher / höherer Dienst, A1/LD14+ posts in a Naturschutz-, Umwelt-, Forst-, Raumordnungs- or Nationalpark-related unit; Abteilungs-/Sektionsleitung of such a unit scores 85+.
     Does NOT count (score 0-20): Reinigung, Praktikum/Ferialjob/Lehrling, Ranger, Saisonkraft, Sachbearbeiter/Sekretariat/Kanzlei, Techniker/Messtechnik/Labor, Straßen-/Bauhof, Ärzte, Pädagogen, Sozialarbeit, IT, Buchhaltung, Verkehr, Gesundheit, Kultur. Also 0-20 for professional posts in an Austrian authority whose unit has nothing to do with nature / land use (e.g. Grafik, Justiz, Finanz).
Regions: Austria and Sub-Saharan Africa = top priority; global / EU-level = good. Other regions only for exceptional director or consultancy roles.

NOT of interest (score 0-20) outside pathway C: anything below director/senior level (officer, coordinator, specialist, ranger, researcher, M&E, comms, finance, HR, fundraising, tourism ops), generic conservation-NGO staff roles, species/research projects, fellowships, internships, tenders for goods/works/IT/supplies, car parks, urban parks, business/industrial parks, forestry harvesting, agriculture, climate/energy unrelated to PAs.

SCORING 0-100:
  85-100: park director/CEO/manager (Austria, SSA, or global), senior PA consultancy clearly for park authorities/ministries, or head of an Austrian Land/Bund nature-conservation unit.
  65-84: senior PA-management role or PA consultancy where seniority/PA-focus is likely but not explicit; head of conservation / landscape director in SSA; Austrian pathway-C professional post in a nature-related unit.
  35-64: senior conservation leadership only partly about PAs (e.g. country director of a conservation NGO), PA tenders of unclear level, or an Austrian authority professional post whose unit is adjacent (Forst, Wasserwirtschaft, Raumordnung, Regionalentwicklung) rather than Naturschutz itself.
  0-34: everything else.

OUTPUT: a JSON array, one object per input id, no prose:
[{"id":<int>,"score":<0-100>,"region":"austria|ssa|global|other","kind":"director|consultancy|senior|pathway|other","why":"English, terse, <=14 words, no full sentences, do NOT restate the title — add what it lacks (level, employer type, fit). Austrian authority postings: unit as named in the posting → park (or 'no park link'); verdict, e.g. 'Abt. Naturschutz RU5 → NP Donau-Auen; Referent-level legal post'. Non-Austrian: level + fit, e.g. 'LMMA finance consultancy, not PA management'"}]`

type rankResult struct {
	ID     int64  `json:"id"`
	Score  int    `json:"score"`
	Region string `json:"region"`
	Kind   string `json:"kind"`
	Why    string `json:"why"`
}

var mdLinkRe = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
var jsonArrRe = regexp.MustCompile(`(?s)\[\s*\{.*\}\s*\]`)
var jsonObjRe = regexp.MustCompile(`\{[^{}]*\}`)

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
	hints := feedback.PromptHints(ctx, db, "job", 40)
	if hints != "" {
		fmt.Fprintf(&logb, "· owner feedback in prompt: %d chars\n", len(hints))
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
	batch := 12
	spent := 0.0
	// pending is processed in batches; a failed batch is split in half and
	// retried so a single timeout / truncated reply doesn't leave items unranked.
	pending := rows
	for len(pending) > 0 {
		if cost.MonthUSD+spent >= budget {
			fmt.Fprintf(&logb, "budget reached mid-run, %d left unranked\n", len(pending))
			break
		}
		n := min(batch, len(pending))
		cur := pending[:n]
		pending = pending[n:]
		res, in, out, err := rankBatch(ctx, cur, hints)
		run.InTokens += in
		run.OutTokens += out
		c := costUSD(in, out)
		spent += c
		run.CostUSD += c
		if err != nil {
			slog.Warn("jobs rank", "error", err)
			if len(cur) > 1 && ctx.Err() == nil {
				fmt.Fprintf(&logb, "✗ batch of %d: %v → retrying in halves\n", len(cur), err)
				batch = max(1, len(cur)/2)
				pending = append(append([]Row{}, cur...), pending...)
				continue
			}
			fmt.Fprintf(&logb, "✗ %d item(s) failed: %v\n", len(cur), err)
			continue
		}
		scored := map[int64]bool{}
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
				scored[r.ID] = true
			}
		}
		// Items the model silently dropped go back to the queue once.
		var missed []Row
		for _, r := range cur {
			if !scored[r.ID] && !r.retried {
				r.retried = true
				missed = append(missed, r)
			}
		}
		if len(missed) > 0 {
			fmt.Fprintf(&logb, "· %d item(s) missing from reply, requeued\n", len(missed))
			pending = append(missed, pending...)
		}
		fmt.Fprintf(&logb, "✓ batch of %d: %d scored, %d in / %d out tokens, %s\n", len(cur), len(res), in, out, usd(c))
		batch = min(12, batch*2) // grow back after a success
	}
	run.Log = logb.String()
	if err := insertRun(ctx, db, run); err != nil {
		slog.Warn("jobs insert rank run", "error", err)
	}
	return run
}

func rankBatch(ctx context.Context, rows []Row, hints string) ([]rankResult, int64, int64, error) {
	var sb strings.Builder
	for _, r := range rows {
		fmt.Fprintf(&sb, "id=%d | %s | org: %s | loc: %s | posted: %s | deadline: %s | src: %s\n  %s\n",
			r.ID, r.Title, r.Org, r.Location, r.Posted, r.Deadline, r.Source, truncate(r.Snippet, 700))
	}
	content, nIn, nOut, err := chat(ctx, systemPrompt+hints, sb.String(), 1500+400*len(rows))
	if err != nil {
		return nil, nIn, nOut, err
	}
	r := struct{ Usage struct{ In, Out int64 } }{}
	r.Usage.In, r.Usage.Out = nIn, nOut
	var res []rankResult
	if m := jsonArrRe.FindString(content); m != "" {
		if err := json.Unmarshal([]byte(m), &res); err != nil {
			res = nil
		}
	}
	if res == nil {
		// Truncated / malformed array: salvage every complete object.
		for _, o := range jsonObjRe.FindAllString(content, -1) {
			var x rankResult
			if json.Unmarshal([]byte(o), &x) == nil && x.ID != 0 {
				res = append(res, x)
			}
		}
	}
	if len(res) == 0 {
		return nil, r.Usage.In, r.Usage.Out, fmt.Errorf("no JSON array in reply: %.200s", content)
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

// chat performs one completion against the keyless gateway and returns the
// reply text plus prompt/completion token counts (for cost accounting).
func chat(ctx context.Context, system, user string, maxTokens int) (string, int64, int64, error) {
	body, _ := json.Marshal(map[string]any{
		"model":       Model,
		"temperature": 0,
		"max_tokens":  maxTokens,
		// muse-glimmer is a reasoning model: at default effort its thinking
		// alone regularly exceeds max_tokens and content comes back null.
		"reasoning_effort": "low",
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
	})
	req, _ := http.NewRequestWithContext(ctx, "POST", llmURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 240 * time.Second}).Do(req)
	if err != nil {
		return "", 0, 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	var r struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			In  int64 `json:"prompt_tokens"`
			Out int64 `json:"completion_tokens"`
		} `json:"usage"`
		Error any `json:"error"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return "", 0, 0, fmt.Errorf("decode (HTTP %d): %.200s", resp.StatusCode, b)
	}
	if r.Error != nil || len(r.Choices) == 0 {
		return "", r.Usage.In, r.Usage.Out, fmt.Errorf("llm error (HTTP %d): %.300s", resp.StatusCode, b)
	}
	c := r.Choices[0]
	if c.FinishReason == "length" && strings.TrimSpace(c.Message.Content) == "" {
		return "", r.Usage.In, r.Usage.Out, fmt.Errorf("reasoning exhausted max_tokens=%d (%d out tokens, no answer)", maxTokens, r.Usage.Out)
	}
	return c.Message.Content, r.Usage.In, r.Usage.Out, nil
}

func costUSD(in, out int64) float64 {
	return float64(in)*priceInPerM/1e6 + float64(out)*priceOutPerM/1e6
}

const summaryPrompt = `You write the 2-3 sentence editorial intro of a weekly job-radar email for a former national park director (fluent EN/DE/FR) who wants (A) to lead a national park / protected area, ideally in Austria or Sub-Saharan Africa, or (B) senior consultancies on protected-area management/governance. You get this week's picks, the runner-ups and upcoming funding deadlines. Write plain text, no markdown, no bullet points, no greeting, max 90 words: what stands out among the jobs, which one to act on first and why (deadlines!), then one sentence on the most pressing funding deadline(s). Be concrete and use the titles/orgs. Skip anything scored below 50 unless it is notable.`

// Summarize produces the short LLM intro for the digest and books its cost as
// a "summary" run. Returns "" (no error surfaced) if the budget is exhausted
// or the model fails — the email is still useful without it.
func Summarize(ctx context.Context, db *sql.DB, picks, others []Row, funding []string) string {
	if len(picks) == 0 && len(others) == 0 {
		return ""
	}
	cost := GetCost(ctx, db)
	if cost.MonthUSD >= MaxMonthUSD() {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("PICKS:\n")
	for _, r := range picks {
		fmt.Fprintf(&sb, "- [%d] %s — %s | deadline: %s | %s\n", r.ScoreVal(), r.Title, orgLoc(r), orDash(r.Deadline), r.Why)
	}
	if len(others) > 0 {
		sb.WriteString("RUNNER-UPS:\n")
		for _, r := range others {
			fmt.Fprintf(&sb, "- [%d] %s — %s | deadline: %s | %s\n", r.ScoreVal(), r.Title, orgLoc(r), orDash(r.Deadline), r.Why)
		}
	}
	if len(funding) > 0 {
		sb.WriteString("FUNDING DEADLINES (grants/programmes the person tracks for their own venture):\n")
		for _, l := range funding {
			sb.WriteString("- " + l + "\n")
		}
	}
	run := Run{Started: time.Now().UTC().Format("2006-01-02 15:04:05"), Kind: "summary", Model: Model}
	text, in, out, err := chat(ctx, summaryPrompt, sb.String(), 2500)
	run.InTokens, run.OutTokens, run.CostUSD = in, out, costUSD(in, out)
	if err != nil {
		run.Log = "summary failed: " + err.Error()
		insertRun(ctx, db, run)
		slog.Warn("jobs summary", "error", err)
		return ""
	}
	text = strings.TrimSpace(stripThink(text))
	if text == "" {
		run.Log = fmt.Sprintf("summary empty (reasoning exhausted max_tokens?): %d in / %d out tokens, %s", in, out, usd(run.CostUSD))
		insertRun(ctx, db, run)
		return ""
	}
	run.Log = fmt.Sprintf("summary ok: %d in / %d out tokens, %s", in, out, usd(run.CostUSD))
	insertRun(ctx, db, run)
	return text
}

var thinkRe = regexp.MustCompile(`(?s)<think>.*?</think>`)

func stripThink(s string) string { return thinkRe.ReplaceAllString(s, "") }

func orDash(s string) string {
	if s == "" {
		return "–"
	}
	return s
}

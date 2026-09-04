package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL   = "https://internal.llmgateway.io"
	originURL        = "https://devpass.llmgateway.io"
	sessionCookie    = "__Secure-better-auth.session_token"
	defaultHookTTL   = 5 * time.Minute
	defaultShowTTL   = 30 * time.Second
	requestTimeout   = 15 * time.Second
	defaultShowRange = "7d"
)

func main() {
	os.Exit(run())
}

func run() int {
	args := os.Args[1:]
	cmd := "hook"
	if len(args) > 0 {
		cmd = args[0]
		args = args[1:]
	}
	switch cmd {
	case "hook":
		hookCmd()
		return 0
	case "show":
		return showCmd(args)
	case "refresh":
		return refreshCmd()
	case "-h", "--help", "help":
		usage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		usage()
		return 2
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `devpass-usage - LLM Gateway (DevPass) usage for Crush

Usage:
  devpass-usage hook              PreToolUse hook: emit {"decision":"allow","context":"..."} (throttled)
  devpass-usage show [--range R]  Print usage summary (R: 24h|7d|30d, default `+defaultShowRange+`)
  devpass-usage refresh           Force refresh of cached usage and exit

Credentials (first match wins):
  LLM_GATEWAY_SESSION_TOKEN       better-auth session token (30d)
  LLM_GATEWAY_EMAIL + LLM_GATEWAY_PASSWORD  auto sign-in fallback
  LLM_GATEWAY_BASE_URL            default `+defaultBaseURL+`

Environment:
  LLM_GATEWAY_USAGE_TTL           hook cache TTL (default `+defaultHookTTL.String()+`)
`)
}

func hookCmd() {
	_, _ = io.Copy(io.Discard, os.Stdin)

	ttl := defaultHookTTL
	if v := strings.TrimSpace(os.Getenv("LLM_GATEWAY_USAGE_TTL")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			ttl = d
		}
	}

	c := newClient()
	snap := readSnapshot()
	if snap != nil && time.Since(snap.FetchedAt) < ttl {
		emitContext("")
		return
	}

	fresh, err := c.fetchSnapshot(context.Background())
	if err != nil {
		emitContext("")
		return
	}
	writeSnapshot(fresh)
	emitContext(fresh.contextLine())
}

func showCmd(args []string) int {
	fs := flag.NewFlagSet("show", flag.ContinueOnError)
	rangeID := fs.String("range", defaultShowRange, "usage range: 24h, 7d or 30d")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if !validRange(*rangeID) {
		fmt.Fprintf(os.Stderr, "invalid range %q (use 24h, 7d or 30d)\n", *rangeID)
		return 2
	}

	c := newClient()
	snap := readSnapshot()
	if snap == nil || time.Since(snap.FetchedAt) > defaultShowTTL || snap.Range != *rangeID {
		fresh, err := c.fetchRange(context.Background(), *rangeID)
		if err != nil {
			if snap != nil && snap.Range == *rangeID {
				fmt.Fprintln(os.Stderr, "warning: fetch failed, showing cached:", err)
				printShow(snap)
				return 0
			}
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		writeSnapshot(fresh)
		snap = fresh
	}
	printShow(snap)
	return 0
}

func refreshCmd() int {
	c := newClient()
	snap, err := c.fetchSnapshot(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	writeSnapshot(snap)
	fmt.Println(snap.contextLine())
	return 0
}

type modelRow struct {
	ID           string  `json:"id"`
	RequestCount int64   `json:"requestCount"`
	InputTokens  int64   `json:"inputTokens"`
	OutputTokens int64   `json:"outputTokens"`
	TotalTokens  int64   `json:"totalTokens"`
	Cost         float64 `json:"cost"`
}

type snapshot struct {
	FetchedAt   time.Time      `json:"fetchedAt"`
	Range       string         `json:"range"`
	Status      *planStatus    `json:"status,omitempty"`
	RangeTotals *usageTotals   `json:"rangeTotals,omitempty"`
	Other       *usageTotals   `json:"otherTotals,omitempty"`
	Models      []modelRow     `json:"models,omitempty"`
	ContextLine string         `json:"contextLine,omitempty"`
	raw         map[string]any `json:"-"`
}

func (s *snapshot) contextLine() string {
	if s == nil || s.Status == nil {
		return ""
	}
	st := s.Status
	if st.DevPlan == "" {
		return ""
	}
	line := fmt.Sprintf("DevPass usage | credits_left: %.2f", st.CreditsRemaining)
	if t := s.RangeTotals; t != nil {
		line += fmt.Sprintf(" | spend_%s: $%.2f", s.Range, t.Cost)
	}
	return line
}

func emitContext(line string) {
	out := map[string]string{"decision": "allow"}
	if line != "" {
		out["context"] = line
	}
	b, _ := json.Marshal(out)
	fmt.Println(string(b))
}

func printShow(s *snapshot) {
	models := s.Models
	nameW := 0
	for _, m := range models {
		if l := len(truncate(m.ID, 28)); l > nameW {
			nameW = l
		}
	}

	if s.Status != nil {
		st := s.Status
		if st.DevPlan != "" {
			title := "DevPass " + strings.ToUpper(st.DevPlan[:1]) + st.DevPlan[1:]
			if st.Cycle != "" {
				title += " (" + st.Cycle + ")"
			}
			if st.Cancelled {
				title += " · cancelled"
			}
			if !st.ExpiresAt.IsZero() {
				title += " · " + verb(st.Cancelled) + " " + st.ExpiresAt.Local().Format("Jan 2")
			}
			fmt.Println(title)
			fmt.Printf("├─ Credits\n")
			fmt.Printf("│  ├─ Monthly  %s %.2f / %.0f left\n",
				bar(remaining(st.CreditsRemaining, st.CreditsLimit), 10), st.CreditsRemaining, st.CreditsLimit)
			if st.PremiumWeeklyLimit > 0 {
				resets := ""
				if !st.PremiumWeekResets.IsZero() {
					resets = fmt.Sprintf("  resets %s", st.PremiumWeekResets.Local().Format("Jan 2"))
				}
				fmt.Printf("│  ╰─ Premium  %s %.2f / %.2f left%s\n",
					bar(remaining(st.PremiumWeeklyLimit-st.PremiumCreditsUsed, st.PremiumWeeklyLimit), 10),
					st.PremiumWeeklyLimit-st.PremiumCreditsUsed, st.PremiumWeeklyLimit, resets)
			}
			fmt.Println()
		}
	}

	if t := s.RangeTotals; t != nil {
		line := fmt.Sprintf("├─ %s   %s reqs · $%.2f · %s tok",
			s.Range, humanCount(t.Requests), t.Cost, humanTokens(t.InputTokens+t.OutputTokens))
		if t.Errors > 0 {
			if t.Errors == 1 {
				line += " · 1 error"
			} else {
				line += fmt.Sprintf(" · %d errors", t.Errors)
			}
		}
		fmt.Println(line)
		for i, m := range models {
			guide := "├─"
			if i == len(models)-1 {
				guide = "╰─"
			}
			maxCost := 0.0
			for _, mm := range models {
				if mm.Cost > maxCost {
					maxCost = mm.Cost
				}
			}
			fmt.Printf("│  %s %-*s  %s  $%.2f\n",
				guide, nameW, truncate(m.ID, 28), bar(m.Cost/maxCost, 8), m.Cost)
		}
	}
	if t := s.Other; t != nil && s.Range != "30d" {
		fmt.Printf("╰─ 30d   %s reqs · $%.2f\n", humanCount(t.Requests), t.Cost)
	} else if s.Range == "30d" {
		fmt.Printf("╰─ 30d\n")
	}
	fmt.Printf("\n(as of %s)\n", s.FetchedAt.Local().Format("Jan 2, 2006 15:04"))
}

func verb(cancelled bool) string {
	if cancelled {
		return "expires"
	}
	return "renews"
}

func remaining(left, limit float64) float64 {
	if limit <= 0 {
		return 0
	}
	return left / limit
}

func bar(frac float64, width int) string {
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	f := int(math.Round(frac * float64(width)))
	return strings.Repeat("█", f) + strings.Repeat("░", width-f)
}

func humanTokens(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func humanCount(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 10_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func truncate(s string, w int) string {
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	return string(r[:w-1]) + "…"
}

func validRange(s string) bool { return s == "24h" || s == "7d" || s == "30d" }

func rangeLabel(s string) string {
	switch s {
	case "24h":
		return "Last 24 hours"
	case "7d":
		return "Last 7 days"
	case "30d":
		return "Last 30 days"
	default:
		return s
	}
}

type flexFloat float64

func (f *flexFloat) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if string(b) == "null" {
		*f = 0
		return nil
	}
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		if s == "" {
			*f = 0
			return nil
		}
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return err
		}
		*f = flexFloat(v)
		return nil
	}
	var v float64
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*f = flexFloat(v)
	return nil
}

type planStatus struct {
	DevPlan            string    `json:"devPlan"`
	Cycle              string    `json:"devPlanCycle"`
	CreditsUsed        float64   `json:"devPlanCreditsUsed"`
	CreditsLimit       float64   `json:"devPlanCreditsLimit"`
	CreditsRemaining   float64   `json:"devPlanCreditsRemaining"`
	PremiumWeeklyLimit float64   `json:"devPlanPremiumWeeklyLimit"`
	PremiumCreditsUsed float64   `json:"devPlanPremiumCreditsUsed"`
	PremiumWeekResets  time.Time `json:"devPlanPremiumWeekResetsAt"`
	ExpiresAt          time.Time `json:"devPlanExpiresAt"`
	Cancelled          bool      `json:"devPlanCancelled"`
	ProjectID          string    `json:"projectId"`
}

func (p *planStatus) UnmarshalJSON(b []byte) error {
	type alias struct {
		DevPlan            string    `json:"devPlan"`
		Cycle              string    `json:"devPlanCycle"`
		CreditsUsed        flexFloat `json:"devPlanCreditsUsed"`
		CreditsLimit       flexFloat `json:"devPlanCreditsLimit"`
		CreditsRemaining   flexFloat `json:"devPlanCreditsRemaining"`
		PremiumWeeklyLimit flexFloat `json:"devPlanPremiumWeeklyLimit"`
		PremiumCreditsUsed flexFloat `json:"devPlanPremiumCreditsUsed"`
		PremiumWeekResets  time.Time `json:"devPlanPremiumWeekResetsAt"`
		ExpiresAt          time.Time `json:"devPlanExpiresAt"`
		Cancelled          bool      `json:"devPlanCancelled"`
		ProjectID          string    `json:"projectId"`
	}
	var a alias
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	*p = planStatus{
		DevPlan:            a.DevPlan,
		Cycle:              a.Cycle,
		CreditsUsed:        float64(a.CreditsUsed),
		CreditsLimit:       float64(a.CreditsLimit),
		CreditsRemaining:   float64(a.CreditsRemaining),
		PremiumWeeklyLimit: float64(a.PremiumWeeklyLimit),
		PremiumCreditsUsed: float64(a.PremiumCreditsUsed),
		PremiumWeekResets:  a.PremiumWeekResets,
		ExpiresAt:          a.ExpiresAt,
		Cancelled:          a.Cancelled,
		ProjectID:          a.ProjectID,
	}
	return nil
}

type rawBucket struct {
	RequestCount     int64     `json:"requestCount"`
	InputTokens      int64     `json:"inputTokens"`
	OutputTokens     int64     `json:"outputTokens"`
	CachedTokens     int64     `json:"cachedTokens"`
	CacheWriteTokens int64     `json:"cacheWriteTokens"`
	Cost             flexFloat `json:"cost"`
	ErrorCount       int64     `json:"errorCount"`
	ModelBreakdown   []struct {
		ID           string    `json:"id"`
		RequestCount int64     `json:"requestCount"`
		InputTokens  int64     `json:"inputTokens"`
		OutputTokens int64     `json:"outputTokens"`
		TotalTokens  int64     `json:"totalTokens"`
		Cost         flexFloat `json:"cost"`
	} `json:"modelBreakdown"`
}

type activityResp struct {
	Activity []rawBucket `json:"activity"`
}

type usageTotals struct {
	Requests         int64
	InputTokens      int64
	OutputTokens     int64
	CachedTokens     int64
	CacheWriteTokens int64
	Errors           int64
	Cost             float64
}

type client struct {
	baseURL      string
	sessionToken string
	email        string
	password     string
	http         *http.Client
}

func newClient() *client {
	baseURL := envOr("LLM_GATEWAY_BASE_URL", "LLMGATEWAY_BASE_URL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &client{
		baseURL:      baseURL,
		sessionToken: envOr("LLM_GATEWAY_SESSION_TOKEN", "LLMGATEWAY_SESSION_TOKEN", "DEVPASS_SESSION_TOKEN"),
		email:        envOr("LLM_GATEWAY_EMAIL", "LLMGATEWAY_EMAIL", "DEVPASS_EMAIL"),
		password:     envOr("LLM_GATEWAY_PASSWORD", "LLMGATEWAY_PASSWORD", "DEVPASS_PASSWORD"),
		http:         &http.Client{Timeout: requestTimeout},
	}
}

func envOr(names ...string) string {
	for _, n := range names {
		if v := strings.TrimSpace(os.Getenv(n)); v != "" {
			return v
		}
	}
	return ""
}

func cacheDir() string {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "devpass-usage")
}

func snapshotPath() string { return filepath.Join(cacheDir(), "usage.json") }
func sessionPath() string  { return filepath.Join(cacheDir(), "session.json") }

type cachedSession struct {
	Token     string    `json:"token"`
	FetchedAt time.Time `json:"fetchedAt"`
}

func (c *client) ensureSession(ctx context.Context) error {
	if c.sessionToken != "" {
		return nil
	}
	if b, err := os.ReadFile(sessionPath()); err == nil {
		var cs cachedSession
		if json.Unmarshal(b, &cs) == nil && cs.Token != "" && time.Since(cs.FetchedAt) < 25*24*time.Hour {
			c.sessionToken = cs.Token
			return nil
		}
	}
	return c.signIn(ctx)
}

func (c *client) signIn(ctx context.Context) error {
	if c.email == "" || c.password == "" {
		return errors.New("no credentials: set LLM_GATEWAY_SESSION_TOKEN or LLM_GATEWAY_EMAIL + LLM_GATEWAY_PASSWORD")
	}
	payload, _ := json.Marshal(map[string]string{"email": c.email, "password": c.password})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/auth/sign-in/email", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Origin", originURL)
	req.Header.Set("Referer", originURL+"/")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("sign-in: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("sign-in failed: %s %s", resp.Status, strings.TrimSpace(string(body)))
	}
	for _, ck := range resp.Cookies() {
		if ck.Name == sessionCookie && ck.Value != "" {
			c.sessionToken = ck.Value
			_ = os.MkdirAll(cacheDir(), 0o700)
			b, _ := json.Marshal(cachedSession{Token: ck.Value, FetchedAt: time.Now()})
			_ = os.WriteFile(sessionPath(), b, 0o600)
			return nil
		}
	}
	return errors.New("sign-in response contained no session cookie")
}

func (c *client) get(ctx context.Context, path string, query url.Values, out any) error {
	if err := c.ensureSession(ctx); err != nil {
		return err
	}
	u := strings.TrimSuffix(c.baseURL, "/") + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Origin", originURL)
	req.Header.Set("Referer", originURL+"/")
	req.Header.Set("Cookie", sessionCookie+"="+c.sessionToken)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized && c.email != "" && c.password != "" {
		if err := c.signIn(ctx); err == nil {
			return c.get(ctx, path, query, out)
		}
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("%s: %s %s", path, resp.Status, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *client) fetchSnapshot(ctx context.Context) (*snapshot, error) {
	return c.fetchRange(ctx, "24h")
}

func (c *client) fetchRange(ctx context.Context, rangeID string) (*snapshot, error) {
	var st planStatus
	if err := c.get(ctx, "/dev-plans/status", nil, &st); err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("projectId", st.ProjectID)
	q.Set("timeRange", rangeID)
	q.Set("timezone", localTimezone())
	var act activityResp
	if err := c.get(ctx, "/activity", q, &act); err != nil {
		return nil, err
	}
	otherRange := "30d"
	if rangeID == "30d" {
		otherRange = "24h"
	}
	q2 := url.Values{}
	q2.Set("projectId", st.ProjectID)
	q2.Set("timeRange", otherRange)
	q2.Set("timezone", localTimezone())
	var act2 activityResp
	_ = c.get(ctx, "/activity", q2, &act2)

	tot := totalsOf(act)
	models := modelsOf(act)
	sort.Slice(models, func(i, j int) bool { return models[i].Cost > models[j].Cost })

	s := &snapshot{
		FetchedAt:   time.Now(),
		Range:       rangeID,
		Status:      &st,
		RangeTotals: tot,
		Models:      models,
	}
	if act2b := totalsOf(act2); act2b != nil {
		s.Other = act2b
	}
	s.ContextLine = s.contextLine()
	return s, nil
}

func totalsOf(a activityResp) *usageTotals {
	var t usageTotals
	for _, b := range a.Activity {
		t.Requests += b.RequestCount
		t.InputTokens += b.InputTokens
		t.OutputTokens += b.OutputTokens
		t.CachedTokens += b.CachedTokens
		t.CacheWriteTokens += b.CacheWriteTokens
		t.Errors += b.ErrorCount
		t.Cost += float64(b.Cost)
	}
	return &t
}

func modelsOf(a activityResp) []modelRow {
	byID := map[string]*modelRow{}
	for _, b := range a.Activity {
		for _, m := range b.ModelBreakdown {
			agg, ok := byID[m.ID]
			if !ok {
				byID[m.ID] = &modelRow{ID: m.ID, RequestCount: m.RequestCount, InputTokens: m.InputTokens, OutputTokens: m.OutputTokens, TotalTokens: m.TotalTokens, Cost: float64(m.Cost)}
				continue
			}
			agg.RequestCount += m.RequestCount
			agg.InputTokens += m.InputTokens
			agg.OutputTokens += m.OutputTokens
			agg.TotalTokens += m.TotalTokens
			agg.Cost += float64(m.Cost)
		}
	}
	out := make([]modelRow, 0, len(byID))
	for _, m := range byID {
		out = append(out, *m)
	}
	return out
}

func readSnapshot() *snapshot {
	b, err := os.ReadFile(snapshotPath())
	if err != nil {
		return nil
	}
	var s snapshot
	if json.Unmarshal(b, &s) != nil {
		return nil
	}
	return &s
}

func writeSnapshot(s *snapshot) {
	_ = os.MkdirAll(cacheDir(), 0o700)
	b, err := json.Marshal(s)
	if err != nil {
		return
	}
	_ = os.WriteFile(snapshotPath(), b, 0o600)
}

func localTimezone() string {
	if tz := envOr("LLM_GATEWAY_TIMEZONE", "LLMGATEWAY_TIMEZONE"); tz != "" {
		return tz
	}
	if tz := os.Getenv("TZ"); tz != "" {
		return tz
	}
	if link, err := os.Readlink("/etc/localtime"); err == nil {
		if i := strings.LastIndex(link, "/zoneinfo/"); i >= 0 {
			return link[i+len("/zoneinfo/"):]
		}
	}
	return "UTC"
}

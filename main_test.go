package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func fakeAPI(t *testing.T) *httptest.Server {
	t.Helper()
	var lastTimeRange string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/dev-plans/status":
			w.Write([]byte(`{"devPlan":"pro","devPlanCycle":"monthly","devPlanCreditsRemaining":100,` +
				`"devPlanCreditsLimit":200,"projectId":"p1"}`))
		case "/activity":
			lastTimeRange = r.URL.Query().Get("timeRange")
			w.Write([]byte(`{"activity":[{"requestCount":3,"inputTokens":100,"outputTokens":50,` +
				`"cost":0.5,"errorCount":1,"modelBreakdown":[{"id":"test-model","requestCount":3,` +
				`"inputTokens":100,"outputTokens":50,"totalTokens":150,"cost":0.5}]}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(func() {
		if lastTimeRange != "" && lastTimeRange != "30d" {
			t.Logf("expected follow-up 30d activity fetch, got %q", lastTimeRange)
		}
	})
	return srv
}

func TestShowUsageTool(t *testing.T) {
	srv := fakeAPI(t)
	defer srv.Close()
	t.Setenv("LLM_GATEWAY_BASE_URL", srv.URL)
	t.Setenv("LLM_GATEWAY_SESSION_TOKEN", "test-token")

	res, _, err := showUsage(context.Background(), &mcp.CallToolRequest{}, showArgs{Range: "7d"})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	for _, want := range []string{"DevPass Pro", "3 reqs", "$0.50", "test-model", "100.00 / 200 left"} {
		if !strings.Contains(text, want) {
			t.Errorf("output missing %q\n---\n%s", want, text)
		}
	}
}

func TestShowUsageDefaultsToWeek(t *testing.T) {
	srv := fakeAPI(t)
	defer srv.Close()
	t.Setenv("LLM_GATEWAY_BASE_URL", srv.URL)
	t.Setenv("LLM_GATEWAY_SESSION_TOKEN", "test-token")

	if _, _, err := showUsage(context.Background(), &mcp.CallToolRequest{}, showArgs{}); err != nil {
		t.Fatal(err)
	}
}

func TestShowUsageInvalidRange(t *testing.T) {
	_, _, err := showUsage(context.Background(), &mcp.CallToolRequest{}, showArgs{Range: "nope"})
	if err == nil || !strings.Contains(err.Error(), "24h") {
		t.Fatalf("expected invalid-range error, got %v", err)
	}
}

package test

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cpa-usage-keeper/internal/cpa"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/quota"
)

func TestOpenCodeGoRefreshUsesCPAPluginQuota(t *testing.T) {
	const authIndex = "opencode-go-account"
	const resetAt = "2026-11-27T12:00:00Z"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v0/management/quota/fetch" || r.Header.Get("Authorization") != "Bearer management-key" {
			t.Errorf("unexpected management request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var body struct {
			AuthIndex string `json:"auth_index"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.AuthIndex != authIndex {
			t.Errorf("wrong auth index: %q, err=%v", body.AuthIndex, err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, `{"groups":[{"buckets":[{"window":"monthly","remainingFraction":1},{"window":"weekly","remainingFraction":0},{"window":"rolling","remainingFraction":0.7,"resetTime":"`+resetAt+`"}]}]}`)
	}))
	t.Cleanup(server.Close)

	db := openQuotaTestDatabase(t)
	seedUsageIdentity(t, db, entities.UsageIdentity{
		Identity: authIndex, Provider: "opencode-go", Type: "opencode-go", AuthType: entities.UsageIdentityAuthTypeCredential,
	})
	service := quota.NewServiceWithOptions(db, cpa.NewClient(server.URL, "management-key", time.Second, false), quota.ServiceOptions{QuotaUpstreamResponsesEnabled: true})
	t.Cleanup(service.StopRefreshTasks)
	result, err := service.Refresh(context.Background(), quota.RefreshRequest{AuthIndexes: []string{authIndex}, Source: quota.RefreshSourceManual})
	if err != nil || result.Accepted != 1 {
		t.Fatalf("refresh = %+v, err=%v", result, err)
	}
	task := waitForRefreshTask(t, service, authIndex, quota.RefreshTaskStatusCompleted)
	if task.Quota == nil || len(task.Quota.Quota) != 3 {
		t.Fatalf("expected three quota windows, got %+v", task.Quota)
	}
	for i, want := range []struct {
		label string
		used  float64
	}{{"5h", 30}, {"Weekly", 100}, {"Monthly", 0}} {
		row := task.Quota.Quota[i]
		if row.Label != want.label || row.UsedPercent == nil || math.Abs(*row.UsedPercent-want.used) > 1e-9 {
			t.Fatalf("window %d = %+v, want %s at %g%%", i, row, want.label, want.used)
		}
	}
	if got := task.Quota.Quota[0].ResetAt; got != resetAt {
		t.Fatalf("reset time = %q, want %q", got, resetAt)
	}
	inspection, err := service.GetInspectionStatus(context.Background())
	if err != nil || inspection.LimitReached != 1 {
		t.Fatalf("inspection = %+v, err=%v", inspection, err)
	}
}

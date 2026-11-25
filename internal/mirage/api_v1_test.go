package mirage

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/danmuck/edgectl/internal/adminauth"
	"github.com/danmuck/edgectl/internal/ghost"
	"github.com/danmuck/edgectl/internal/identity"
	"github.com/danmuck/edgectl/internal/protocol/schema"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
	"github.com/danmuck/edgectl/internal/testutil/tlstest"
)

// Decoded api/v1 response envelope with data left raw so tests can check field names.
type apiV1TestReply struct {
	Status      int
	OK          bool            `json:"ok"`
	RequestID   string          `json:"request_id"`
	TraceID     string          `json:"trace_id"`
	TimestampMS int64           `json:"timestamp_ms"`
	Data        json.RawMessage `json:"data"`
	Error       *apiV1Error     `json:"error"`
}

// Serves svc's api/v1 handler under the admin transport rules; returns the base URL.
func serveAPIv1ForTest(t *testing.T, svc *Service) string {
	t.Helper()
	ln, err := adminauth.Listen("127.0.0.1:0", svc.cfg.AdminTLS, svc.selfPrincipal(), svc.cfg.AdminTLS.Enabled())
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- svc.ServeAPIv1(ctx, ln) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("ServeAPIv1: %v", err)
		}
	})
	scheme := "http"
	if svc.cfg.AdminTLS.Enabled() {
		scheme = "https"
	}
	return scheme + "://" + ln.Addr().String()
}

// HTTP client presenting role/id's certificate and pinning the Mirage server principal.
func apiV1ClientFor(t *testing.T, p *tlstest.PKI, role identity.Role, id string) *http.Client {
	t.Helper()
	cert, key, _ := p.Issue(t, role, id)
	cfg, err := adminauth.ClientTLSConfig(adminauth.Credentials{CertFile: cert, KeyFile: key, CAFile: p.CAFile()},
		identity.Principal{Role: role, ID: id},
		adminauth.ExpectPrincipal(identity.Principal{Role: identity.RoleMirage, ID: "mirage.main"}))
	if err != nil {
		t.Fatal(err)
	}
	tr := &http.Transport{TLSClientConfig: cfg}
	t.Cleanup(tr.CloseIdleConnections)
	return &http.Client{Transport: tr, Timeout: 10 * time.Second}
}

func apiV1Do(t *testing.T, c *http.Client, method string, url string, body string, headers ...string) apiV1TestReply {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, rd)
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Fatalf("%s %s: content type %q body %s", method, url, ct, raw)
	}
	out := apiV1TestReply{Status: resp.StatusCode}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("%s %s: decode %q: %v", method, url, raw, err)
	}
	if out.RequestID == "" || out.TraceID == "" || out.TimestampMS == 0 {
		t.Fatalf("%s %s: envelope missing correlation fields: %s", method, url, raw)
	}
	if out.OK == (out.Error != nil) {
		t.Fatalf("%s %s: ok and error disagree: %s", method, url, raw)
	}
	return out
}

func wantAPIv1Failure(t *testing.T, got apiV1TestReply, status int, code uint32, class string) {
	t.Helper()
	if got.OK || got.Status != status || got.Error == nil || got.Error.Code != code || got.Error.Class != class {
		t.Fatalf("want status=%d code=%d class=%s, got status=%d ok=%v error=%+v", status, code, class, got.Status, got.OK, got.Error)
	}
}

const apiV1TestIssue = `{"intent_id":"intent.api.1","target_scope":"ghost:ghost.alpha","objective":"status",
"stages":[{"id":"stage.1","barrier":true,"commands":[{"ghost_id":"ghost.alpha","seed_selector":"seed.flow","operation":"status","args":{"k":"v"},"blocking":true}]}]}`

// Operator over mTLS: issue in, reconcile, report out, with the contract's envelope and field names.
func TestAPIv1OperatorSubmitReconcileRead(t *testing.T) {
	testlog.Start(t)
	p := tlstest.NewPKI(t)
	svc := tlsMirageService(t, p)
	if err := svc.Server().RegisterExecutor("ghost.alpha", &fakeExecutor{}); err != nil {
		t.Fatal(err)
	}
	base := serveAPIv1ForTest(t, svc)
	op := apiV1ClientFor(t, p, identity.RoleOperator, "dan")

	st := apiV1Do(t, op, "GET", base+"/api/v1/status", "", "X-Request-Id", "req.test.1")
	if !st.OK || st.RequestID != "req.test.1" || st.TraceID != "trace.req.test.1" {
		t.Fatalf("status: %+v", st)
	}
	// The test service is not running, so only the shape is checked here.
	var status map[string]any
	if err := json.Unmarshal(st.Data, &status); err != nil || len(status) != 5 || status["phase"] != "boot" {
		t.Fatalf("status data %s err=%v", st.Data, err)
	}
	for _, key := range []string{"mirage_id", "phase", "registered_ghosts", "active_intents", "report_count"} {
		if _, ok := status[key]; !ok {
			t.Fatalf("status missing %q: %s", key, st.Data)
		}
	}

	sub := apiV1Do(t, op, "POST", base+"/api/v1/intents", apiV1TestIssue)
	if !sub.OK || sub.TraceID != "trace.intent.api.1" || !bytes.Contains(sub.Data, []byte(`"intent_id":"intent.api.1"`)) {
		t.Fatalf("submit: %+v data=%s", sub, sub.Data)
	}

	list := apiV1Do(t, op, "GET", base+"/api/v1/intents", "")
	if !list.OK || string(list.Data) != `{"intent_ids":["intent.api.1"]}` {
		t.Fatalf("list: %s", list.Data)
	}

	rec := apiV1Do(t, op, "POST", base+"/api/v1/intents/intent.api.1/reconcile", "")
	var report apiV1Report
	if err := json.Unmarshal(rec.Data, &report); err != nil || !rec.OK {
		t.Fatalf("reconcile: %+v err=%v", rec, err)
	}
	if report.IntentID != "intent.api.1" || report.CompletionState != "satisfied" || report.CommandID == "" {
		t.Fatalf("report: %+v", report)
	}

	snap := apiV1Do(t, op, "GET", base+"/api/v1/intents/intent.api.1", "")
	var view apiV1IntentSnapshot
	if err := json.Unmarshal(snap.Data, &view); err != nil || !snap.OK {
		t.Fatalf("snapshot: %+v err=%v", snap, err)
	}
	if view.Issue.Actor != "edgectl://operator/dan" || view.Issue.Objective != "status" || view.ReceivedAtMS == 0 {
		t.Fatalf("snapshot issue: %+v", view)
	}
	if len(view.Commands) != 1 || view.Commands[0].CommandID != report.CommandID ||
		view.Commands[0].State != string(CommandComplete) || view.Commands[0].Args["k"] != "v" {
		t.Fatalf("snapshot commands: %+v", view.Commands)
	}
	if len(view.Reports) == 0 || view.PendingCount != 0 || !view.HasObserved {
		t.Fatalf("snapshot observed: %+v", view)
	}
	// Go field names must not leak into the contract shapes.
	for _, leaked := range []string{"IntentID", "CommandID", "CompletionState", "TimestampMS", "Desired", "Observed"} {
		if bytes.Contains(snap.Data, []byte(`"`+leaked+`"`)) {
			t.Fatalf("snapshot leaks Go field name %q: %s", leaked, snap.Data)
		}
	}

	reports := apiV1Do(t, op, "GET", base+"/api/v1/reports?limit=5", "")
	var rs struct {
		Reports []apiV1Report `json:"reports"`
	}
	if err := json.Unmarshal(reports.Data, &rs); err != nil || len(rs.Reports) == 0 {
		t.Fatalf("reports: %s err=%v", reports.Data, err)
	}

	all := apiV1Do(t, op, "POST", base+"/api/v1/reconcile", "")
	if !all.OK || !bytes.Contains(all.Data, []byte(`"reports":[`)) {
		t.Fatalf("reconcile all: %+v %s", all, all.Data)
	}

	for _, path := range []string{"/api/v1/ghosts", "/api/v1/routing", "/api/v1/services", "/api/v1/seeds"} {
		if got := apiV1Do(t, op, "GET", base+path, ""); !got.OK {
			t.Fatalf("%s: %+v", path, got.Error)
		}
	}
}

// Default deny and request validation map to the contract's status and wire code.
func TestAPIv1DenialsAndRequestErrors(t *testing.T) {
	testlog.Start(t)
	p := tlstest.NewPKI(t)
	svc := tlsMirageService(t, p)
	base := serveAPIv1ForTest(t, svc)
	op := apiV1ClientFor(t, p, identity.RoleOperator, "dan")
	gh := apiV1ClientFor(t, p, identity.RoleGhost, "ghost.alpha")

	// Non-operator principals are denied everywhere, including the execution read.
	wantAPIv1Failure(t, apiV1Do(t, gh, "GET", base+"/api/v1/status", ""), http.StatusForbidden, schema.ErrCodeTransportFailure, "transport")
	wantAPIv1Failure(t, apiV1Do(t, gh, "POST", base+"/api/v1/intents", apiV1TestIssue), http.StatusForbidden, schema.ErrCodeTransportFailure, "transport")
	wantAPIv1Failure(t, apiV1Do(t, gh, "GET", base+"/api/v1/executions/cmd.1", ""), http.StatusForbidden, schema.ErrCodeTransportFailure, "transport")
	if ids := svc.Server().ListIntentIDs(); len(ids) != 0 {
		t.Fatalf("denied submit mutated state: %v", ids)
	}

	// Actor must be absent or the caller.
	spoofed := strings.Replace(apiV1TestIssue, `"intent_id"`, `"actor":"edgectl://operator/someone","intent_id"`, 1)
	wantAPIv1Failure(t, apiV1Do(t, op, "POST", base+"/api/v1/intents", spoofed), http.StatusForbidden, schema.ErrCodeTransportFailure, "transport")

	semantic := func(got apiV1TestReply, status int) {
		t.Helper()
		wantAPIv1Failure(t, got, status, schema.ErrCodeSemanticValidation, "semantic")
		if got.Error.Retryable {
			t.Fatalf("semantic failure marked retryable")
		}
	}
	semantic(apiV1Do(t, op, "POST", base+"/api/v1/intents", `{"intent_id":`), http.StatusBadRequest)
	semantic(apiV1Do(t, op, "POST", base+"/api/v1/intents", `{"intent_id":"x","surprise":1}`), http.StatusBadRequest)
	semantic(apiV1Do(t, op, "POST", base+"/api/v1/intents", `{"intent_id":"x"}`), http.StatusBadRequest)
	semantic(apiV1Do(t, op, "GET", base+"/api/v1/reports?limit=-1", ""), http.StatusBadRequest)
	semantic(apiV1Do(t, op, "POST", base+"/api/v1/ghosts/spawn-local", `{"target_name":""}`), http.StatusBadRequest)
	semantic(apiV1Do(t, op, "POST", base+"/api/v1/ghosts/attach", `{}`), http.StatusBadRequest)
	semantic(apiV1Do(t, op, "GET", base+"/api/v1/intents/intent.none", ""), http.StatusNotFound)
	semantic(apiV1Do(t, op, "POST", base+"/api/v1/intents/intent.none/reconcile", ""), http.StatusNotFound)
	semantic(apiV1Do(t, op, "GET", base+"/api/v1/executions/cmd.none", ""), http.StatusNotFound)
	semantic(apiV1Do(t, op, "GET", base+"/api/v1/nope", ""), http.StatusNotFound)
	semantic(apiV1Do(t, op, "DELETE", base+"/api/v1/status", ""), http.StatusNotFound)
}

// Stub Ghost admin endpoint over mTLS holding one execution record per command_id, encoded the
// way a real Ghost encodes it.
func stubGhostExecutions(t *testing.T, p *tlstest.PKI, ghostID string, records map[string]ghost.ExecutionState) string {
	t.Helper()
	cert, key, _ := p.Issue(t, identity.RoleGhost, ghostID)
	ln, err := adminauth.Listen("127.0.0.1:0", adminauth.Credentials{CertFile: cert, KeyFile: key, CAFile: p.CAFile()},
		identity.Principal{Role: identity.RoleGhost, ID: ghostID}, true)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				if _, err := adminauth.PeerFromConn(conn, 2*time.Second); err != nil {
					return
				}
				r := bufio.NewReader(conn)
				for {
					line, err := r.ReadBytes('\n')
					if err != nil {
						return
					}
					var req ghostControlRequest
					_ = json.Unmarshal(line, &req)
					state, ok := records[req.CommandID]
					data, _ := json.Marshal(map[string]any{"found": ok, "execution": state})
					resp, _ := json.Marshal(ghostControlResponse{OK: true, Data: data})
					if _, err := conn.Write(append(resp, '\n')); err != nil {
						return
					}
				}
			}()
		}
	}()
	return ln.Addr().String()
}

// The execution read goes Mirage -> Ghost as the Mirage principal and returns seed.result output.
func TestAPIv1ExecutionReadThroughMirage(t *testing.T) {
	testlog.Start(t)
	p := tlstest.NewPKI(t)
	svc := tlsMirageService(t, p)
	if err := svc.Server().RegisterExecutor("ghost.alpha", &fakeExecutor{}); err != nil {
		t.Fatal(err)
	}
	base := serveAPIv1ForTest(t, svc)
	op := apiV1ClientFor(t, p, identity.RoleOperator, "dan")

	two := strings.Replace(apiV1TestIssue, `"blocking":true}`,
		`"blocking":true},{"ghost_id":"ghost.alpha","seed_selector":"seed.flow","operation":"status","blocking":true}`, 1)
	if got := apiV1Do(t, op, "POST", base+"/api/v1/intents", two); !got.OK {
		t.Fatalf("submit: %+v", got.Error)
	}
	var view apiV1IntentSnapshot
	if err := json.Unmarshal(apiV1Do(t, op, "GET", base+"/api/v1/intents/intent.api.1", "").Data, &view); err != nil || len(view.Commands) != 2 {
		t.Fatalf("snapshot: %+v err=%v", view, err)
	}
	textID, binaryID := view.Commands[0].CommandID, view.Commands[1].CommandID

	// Known command but no route to its Ghost yet.
	wantAPIv1Failure(t, apiV1Do(t, op, "GET", base+"/api/v1/executions/"+textID, ""), http.StatusNotFound, schema.ErrCodeSemanticValidation, "semantic")

	done := time.UnixMilli(1760000000000)
	addr := stubGhostExecutions(t, p, "ghost.alpha", map[string]ghost.ExecutionState{
		textID: {
			CommandID: textID, ExecutionID: "exec.1", IntentID: "intent.api.1", GhostID: "ghost.alpha",
			SeedSelector: "seed.flow", Operation: "status", Outcome: "success", Phase: ghost.ExecutionComplete,
			CompletedAt: done,
			SeedResult:  ghost.SeedResultEnv{ExecutionID: "exec.1", SeedID: "seed.flow", Status: "ok", Stdout: []byte("hello\n")},
		},
		binaryID: {
			CommandID: binaryID, ExecutionID: "exec.2", GhostID: "ghost.alpha",
			SeedResult: ghost.SeedResultEnv{SeedID: "seed.flow", Status: "error", Stdout: []byte{0xff, 0xfe}, Stderr: []byte("bad"), ExitCode: 3},
		},
	})
	svc.bindGhostAdmin("ghost.alpha", addr)

	got := apiV1Do(t, op, "GET", base+"/api/v1/executions/"+textID, "")
	var exec apiV1Execution
	if err := json.Unmarshal(got.Data, &exec); err != nil || !got.OK {
		t.Fatalf("execution: %+v err=%v", got.Error, err)
	}
	if got.TraceID != "trace.intent.api.1" || exec.CommandID != textID || exec.ExecutionID != "exec.1" ||
		exec.Phase != "complete" || exec.Outcome != "success" || exec.CompletedAtMS != done.UnixMilli() {
		t.Fatalf("execution fields: trace=%s %+v", got.TraceID, exec)
	}
	if exec.SeedResult.Stdout != "hello\n" || exec.SeedResult.OutputEncoding != "utf8" || exec.SeedResult.Status != "ok" {
		t.Fatalf("seed result: %+v", exec.SeedResult)
	}

	got = apiV1Do(t, op, "GET", base+"/api/v1/executions/"+binaryID, "")
	if err := json.Unmarshal(got.Data, &exec); err != nil || !got.OK {
		t.Fatalf("binary execution: %+v err=%v", got.Error, err)
	}
	if exec.SeedResult.OutputEncoding != "base64" || exec.SeedResult.Stdout != "//4=" || exec.SeedResult.Stderr != "YmFk" || exec.SeedResult.ExitCode != 3 {
		t.Fatalf("binary seed result: %+v", exec.SeedResult)
	}

	// A Ghost that does not answer is a retryable transport failure.
	svc.bindGhostAdmin("ghost.alpha", "127.0.0.1:1")
	down := apiV1Do(t, op, "GET", base+"/api/v1/executions/"+textID, "")
	if down.OK || down.Status != http.StatusBadGateway || !down.Error.Retryable {
		t.Fatalf("unreachable ghost: status=%d error=%+v", down.Status, down.Error)
	}
}

// Development plaintext is loopback only; an unauthenticated loopback caller gets read actions.
func TestAPIv1PlaintextLoopbackOnly(t *testing.T) {
	testlog.Start(t)
	cfg := DefaultServiceConfig()
	cfg.LocalGhostID = ""
	cfg.LocalGhostAdminAddr = ""
	svc := NewServiceWithConfig(cfg)

	err := svc.serveAPIv1(context.Background(), "0.0.0.0:0")
	if !errors.Is(err, adminauth.ErrPlaintextNotAllowed) {
		t.Fatalf("non-loopback plaintext: want ErrPlaintextNotAllowed, got %v", err)
	}

	base := serveAPIv1ForTest(t, svc)
	plain := &http.Client{Timeout: 5 * time.Second}
	if got := apiV1Do(t, plain, "GET", base+"/api/v1/status", ""); !got.OK {
		t.Fatalf("loopback status: %+v", got.Error)
	}
	// Privileged endpoints need an mTLS peer even on loopback, decided before any Ghost is asked.
	wantAPIv1Failure(t, apiV1Do(t, plain, "POST", base+"/api/v1/ghosts/spawn-local", `{"target_name":"a","admin_addr":"127.0.0.1:7031"}`),
		http.StatusForbidden, schema.ErrCodeTransportFailure, "transport")
}

// A web page must not be able to drive the plaintext listener: cross-site requests, form-style
// bodies and rebound hostnames are refused before any action runs.
func TestAPIv1RefusesBrowserRequests(t *testing.T) {
	testlog.Start(t)
	cfg := DefaultServiceConfig()
	cfg.LocalGhostID = ""
	cfg.LocalGhostAdminAddr = ""
	svc := NewServiceWithConfig(cfg)
	base := serveAPIv1ForTest(t, svc)
	plain := &http.Client{Timeout: 5 * time.Second}
	denied := func(got apiV1TestReply) {
		t.Helper()
		wantAPIv1Failure(t, got, http.StatusForbidden, schema.ErrCodeTransportFailure, "transport")
	}

	denied(apiV1Do(t, plain, "GET", base+"/api/v1/status", "", "Origin", "http://evil.example"))
	denied(apiV1Do(t, plain, "GET", base+"/api/v1/status", "", "Sec-Fetch-Site", "cross-site"))
	denied(apiV1Do(t, plain, "GET", base+"/api/v1/status", "", "Sec-Fetch-Site", "same-origin"))
	denied(apiV1Do(t, plain, "POST", base+"/api/v1/intents", apiV1TestIssue, "Origin", "null"))

	// DNS rebinding: the connection reaches loopback but the Host header names another site.
	req, err := http.NewRequest("GET", base+"/api/v1/status", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "evil.example:7021"
	resp, err := plain.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("rebound host: status %d", resp.StatusCode)
	}

	// A body a page can send without a preflight (text/plain) is not accepted as an issue.
	wantAPIv1Failure(t, apiV1Do(t, plain, "POST", base+"/api/v1/intents", apiV1TestIssue, "Content-Type", "text/plain"),
		http.StatusBadRequest, schema.ErrCodeSemanticValidation, "semantic")
	if ids := svc.Server().ListIntentIDs(); len(ids) != 0 {
		t.Fatalf("refused requests mutated state: %v", ids)
	}

	// Ordinary clients are unaffected.
	if got := apiV1Do(t, plain, "GET", strings.Replace(base, "127.0.0.1", "localhost", 1)+"/api/v1/status", "", "Sec-Fetch-Site", "none"); !got.OK {
		t.Fatalf("localhost client: %+v", got.Error)
	}
}

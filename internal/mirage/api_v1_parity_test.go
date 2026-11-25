package mirage

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/danmuck/edgectl/internal/activity"
	"github.com/danmuck/edgectl/internal/adminauth"
	"github.com/danmuck/edgectl/internal/approval"
	"github.com/danmuck/edgectl/internal/identity"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
	"github.com/danmuck/edgectl/internal/testutil/tlstest"
)

// Fails the test unless got is a failure with this HTTP status and wire code.
func wantAPIv1Status(t *testing.T, what string, got apiV1TestReply, status int, code uint32) {
	t.Helper()
	if got.OK || got.Status != status || got.Error == nil || got.Error.Code != code {
		t.Fatalf("%s: want status=%d code=%d, got status=%d ok=%v error=%+v", what, status, code, got.Status, got.OK, got.Error)
	}
}

// One operator identity used both ways: as the api/v1 client certificate and as the signer.
func apiV1OperatorFor(t *testing.T, p *tlstest.PKI, id string) (*http.Client, *approval.Signer) {
	t.Helper()
	cert, key, _ := p.Issue(t, identity.RoleOperator, id)
	signer, err := approval.LoadSigner(cert, key)
	if err != nil {
		t.Fatalf("load signer: %v", err)
	}
	cfg, err := adminauth.ClientTLSConfig(adminauth.Credentials{CertFile: cert, KeyFile: key, CAFile: p.CAFile()},
		identity.Principal{Role: identity.RoleOperator, ID: id},
		adminauth.ExpectPrincipal(identity.Principal{Role: identity.RoleMirage, ID: "mirage.main"}))
	if err != nil {
		t.Fatal(err)
	}
	tr := &http.Transport{TLSClientConfig: cfg}
	t.Cleanup(tr.CloseIdleConnections)
	return &http.Client{Transport: tr, Timeout: 10 * time.Second}, signer
}

func decodeProposal(t *testing.T, what string, got apiV1TestReply) DependencyProposal {
	t.Helper()
	if !got.OK {
		t.Fatalf("%s: status=%d error=%+v", what, got.Status, got.Error)
	}
	var p DependencyProposal
	if err := json.Unmarshal(got.Data, &p); err != nil {
		t.Fatalf("%s: decode %s: %v", what, got.Data, err)
	}
	return p
}

// Dependency install over api/v1 keeps the two-operator rule: the proposer signs and submits, a
// second operator signs and submits the approval on its own connection, then either executes
// (api_v1.toml [parity.signing]; admin_security.toml [install_approval]).
func TestAPIv1DependencyProposalFlow(t *testing.T) {
	testlog.Start(t)
	p := tlstest.NewPKI(t)
	svc := tlsMirageService(t, p)
	svc.bindGhostAdmin("ghost.alpha", "127.0.0.1:7999")
	var dispatched DependencyProposal
	svc.dependencyInstaller = func(_ context.Context, _ string, dp DependencyProposal) (DependencyExecution, error) {
		dispatched = dp
		return DependencyExecution{Succeeded: true}, nil
	}
	base := serveAPIv1ForTest(t, svc) + "/api/v1/dependencies/proposals"
	ops, opsSigner := apiV1OperatorFor(t, p, "ops")
	lead, leadSigner := apiV1OperatorFor(t, p, "lead")
	body := func(key string, v any) string {
		raw, err := json.Marshal(map[string]any{key: v})
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}

	empty := apiV1Do(t, ops, http.MethodGet, base, "")
	if !empty.OK || string(empty.Data) != `{"proposals":[]}` {
		t.Fatalf("empty list: %d %s", empty.Status, empty.Data)
	}
	wantAPIv1Status(t, "propose without intent", apiV1Do(t, ops, http.MethodPost, base, `{}`), 400, 1300)

	intent := intentFor(t, opsSigner, "ghost.alpha", "seed.docker")
	// The intent's signer must be the calling operator.
	wantAPIv1Status(t, "intent submitted by another operator", apiV1Do(t, lead, http.MethodPost, base, body("intent", intent)), 400, 1300)
	prop := decodeProposal(t, "propose", apiV1Do(t, ops, http.MethodPost, base, body("intent", intent)))
	if prop.ID == "" || prop.State != ProposalPending || prop.ProposedBy != "edgectl://operator/ops" || len(prop.Steps) == 0 {
		t.Fatalf("proposal: %+v", prop)
	}
	one := base + "/" + prop.ID

	wantAPIv1Status(t, "execute before approval", apiV1Do(t, ops, http.MethodPost, one+"/execute", ""), 400, 1300)
	wantAPIv1Status(t, "approve without approval", apiV1Do(t, lead, http.MethodPost, one+"/approve", `{}`), 400, 1300)
	wantAPIv1Status(t, "self approval", apiV1Do(t, ops, http.MethodPost, one+"/approve", body("approval", signedFor(t, opsSigner, prop))), 400, 1300)
	signed := signedFor(t, leadSigner, prop)
	// A valid approval by lead cannot be submitted on the proposer's connection.
	wantAPIv1Status(t, "approval submitted by another operator", apiV1Do(t, ops, http.MethodPost, one+"/approve", body("approval", signed)), 400, 1300)
	wantAPIv1Status(t, "approval for another path id", apiV1Do(t, lead, http.MethodPost, base+"/depprop.other/approve", body("approval", signed)), 400, 1300)

	approved := decodeProposal(t, "approve", apiV1Do(t, lead, http.MethodPost, one+"/approve", body("approval", signed)))
	if approved.State != ProposalApproved || approved.ApprovedBy != "edgectl://operator/lead" {
		t.Fatalf("approved: %+v", approved)
	}
	read := decodeProposal(t, "get", apiV1Do(t, ops, http.MethodGet, one, ""))
	if read.ID != prop.ID || read.State != ProposalApproved {
		t.Fatalf("get: %+v", read)
	}
	wantAPIv1Status(t, "unknown proposal", apiV1Do(t, ops, http.MethodGet, base+"/depprop.missing", ""), 404, 1300)
	wantAPIv1Status(t, "execute unknown proposal", apiV1Do(t, ops, http.MethodPost, base+"/depprop.missing/execute", ""), 404, 1300)

	done := decodeProposal(t, "execute", apiV1Do(t, lead, http.MethodPost, one+"/execute", ""))
	if done.State != ProposalSucceeded || done.ExecutedBy != "edgectl://operator/lead" {
		t.Fatalf("executed: %+v", done)
	}
	if dispatched.Approval == nil || dispatched.Approval.SignatureDigest() != signed.SignatureDigest() {
		t.Fatalf("signed approval was not forwarded to the installer")
	}
	var list struct {
		Proposals []DependencyProposal `json:"proposals"`
	}
	got := apiV1Do(t, ops, http.MethodGet, base, "")
	if err := json.Unmarshal(got.Data, &list); err != nil || len(list.Proposals) != 1 || list.Proposals[0].State != ProposalSucceeded {
		t.Fatalf("list: %v %s", err, got.Data)
	}

	// Only operators reach the dependency routes.
	ghostClient := apiV1ClientFor(t, p, identity.RoleGhost, "ghost.alpha")
	wantAPIv1Status(t, "ghost lists proposals", apiV1Do(t, ghostClient, http.MethodGet, base, ""), 403, 1000)
	wantAPIv1Status(t, "ghost proposes", apiV1Do(t, ghostClient, http.MethodPost, base, body("intent", intent)), 403, 1000)
}

// Approve, execute and deploy are privileged: the development plaintext listener refuses them
// before any state is read (api_v1.toml [transport]).
func TestAPIv1ParityPrivilegedNeedsMutualTLS(t *testing.T) {
	testlog.Start(t)
	svc := NewServiceWithConfig(ServiceConfig{ListenAddr: "127.0.0.1:0"})
	base := serveAPIv1ForTest(t, svc) + "/api/v1"
	c := &http.Client{Timeout: 5 * time.Second}
	signed := approval.Signed{Payload: approval.Payload{ProposalID: "depprop.x"}}
	raw, _ := json.Marshal(map[string]any{"approval": signed})
	for what, r := range map[string]apiV1TestReply{
		"approve": apiV1Do(t, c, http.MethodPost, base+"/dependencies/proposals/depprop.x/approve", string(raw)),
		"execute": apiV1Do(t, c, http.MethodPost, base+"/dependencies/proposals/depprop.x/execute", ""),
		"deploy":  apiV1Do(t, c, http.MethodPost, base+"/ghosts/deploy", `{"ghost_id":"ghost.pi"}`),
	} {
		if r.OK || r.Status != 403 {
			t.Errorf("%s over plaintext: status=%d ok=%v error=%+v", what, r.Status, r.OK, r.Error)
		}
	}
}

// Deploy takes only a ghost_id naming a configured manifest; targets expose identity and address
// and nothing about credentials (admin_security.toml [deploy]).
func TestAPIv1DeployAndTargets(t *testing.T) {
	testlog.Start(t)
	p := tlstest.NewPKI(t)
	svc := tlsMirageService(t, p)
	svc.cfg.GhostManifests = []GhostManifest{
		{GhostID: "ghost.pi", Host: "10.0.0.5", User: "pi", SSHKeyFile: "/secret/id_ed25519", SSHHostKey: "ssh-ed25519 AAAA", TLSKeyFile: "/secret/ghost.pi.key", AdminListen: "0.0.0.0:7444"},
		{GhostID: "ghost.nuc", Host: "10.0.0.6"},
	}
	base := serveAPIv1ForTest(t, svc) + "/api/v1/ghosts"
	ops := apiV1ClientFor(t, p, identity.RoleOperator, "ops")

	got := apiV1Do(t, ops, http.MethodGet, base+"/deploy-targets", "")
	want := `{"targets":[{"ghost_id":"ghost.pi","host":"10.0.0.5","admin_addr":"10.0.0.5:7444"},{"ghost_id":"ghost.nuc","host":"10.0.0.6","admin_addr":"10.0.0.6:7010"}]}`
	if !got.OK || string(got.Data) != want {
		t.Fatalf("targets: %d %s", got.Status, got.Data)
	}

	wantAPIv1Status(t, "deploy without ghost_id", apiV1Do(t, ops, http.MethodPost, base+"/deploy", `{}`), 400, 1300)
	wantAPIv1Status(t, "deploy with a host field", apiV1Do(t, ops, http.MethodPost, base+"/deploy", `{"ghost_id":"ghost.pi","host":"evil.example"}`), 400, 1300)
	if r := apiV1Do(t, ops, http.MethodPost, base+"/deploy", `{"ghost_id":"ghost.unknown"}`); r.OK {
		t.Fatalf("deploy of an unconfigured ghost succeeded: %s", r.Data)
	}
	ghostClient := apiV1ClientFor(t, p, identity.RoleGhost, "ghost.alpha")
	wantAPIv1Status(t, "ghost deploys", apiV1Do(t, ghostClient, http.MethodPost, base+"/deploy", `{"ghost_id":"ghost.pi"}`), 403, 1000)
	wantAPIv1Status(t, "ghost reads targets", apiV1Do(t, ghostClient, http.MethodGet, base+"/deploy-targets", ""), 403, 1000)
}

// Ghost admin stub answering each action with canned data; records the limits it was asked for.
func stubGhostReads(t *testing.T, p *tlstest.PKI, ghostID string, replies map[string]any, limits chan<- int) string {
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
					out := ghostControlResponse{Error: "unknown action " + req.Action}
					if v, ok := replies[req.Action]; ok {
						data, _ := json.Marshal(v)
						out = ghostControlResponse{OK: true, Data: data}
						if req.Action != "status" {
							limits <- req.Limit
						}
					}
					resp, _ := json.Marshal(out)
					if _, err := conn.Write(append(resp, '\n')); err != nil {
						return
					}
				}
			}()
		}
	}()
	return ln.Addr().String()
}

// Ghost status, events and verification are read by Mirage for the client, in contract field
// names (api_v1.toml [parity.ghost_reads]).
func TestAPIv1GhostReadsThroughMirage(t *testing.T) {
	testlog.Start(t)
	p := tlstest.NewPKI(t)
	svc := tlsMirageService(t, p)
	limits := make(chan int, 4)
	addr := stubGhostReads(t, p, "ghost.alpha", map[string]any{
		"status": map[string]any{"GhostID": "ghost.alpha", "Phase": "serving", "SeedCount": 3},
		"recent_events": []GhostRecordedEvent{{EventID: "evt.1", CommandID: "cmd.1", IntentID: "intent.1", GhostID: "ghost.alpha",
			SeedID: "seed.kv", Outcome: "success", TimestampMS: 42}},
		"verification": []map[string]any{{"request_id": "req.1", "trace_id": "trace.intent.1", "command_id": "cmd.1", "outcome": "success"}},
	}, limits)
	svc.bindGhostAdmin("ghost.alpha", addr)
	base := serveAPIv1ForTest(t, svc) + "/api/v1/ghosts"
	ops := apiV1ClientFor(t, p, identity.RoleOperator, "ops")

	st := apiV1Do(t, ops, http.MethodGet, base+"/ghost.alpha/status", "")
	if !st.OK || string(st.Data) != `{"ghost_id":"ghost.alpha","phase":"serving","seed_count":3}` {
		t.Fatalf("status: %d %s %+v", st.Status, st.Data, st.Error)
	}
	ev := apiV1Do(t, ops, http.MethodGet, base+"/ghost.alpha/events?limit=7", "")
	wantEvents := `{"events":[{"event_id":"evt.1","command_id":"cmd.1","intent_id":"intent.1","ghost_id":"ghost.alpha","seed_id":"seed.kv","outcome":"success","timestamp_ms":42}]}`
	if !ev.OK || string(ev.Data) != wantEvents {
		t.Fatalf("events: %d %s %+v", ev.Status, ev.Data, ev.Error)
	}
	if got := <-limits; got != 7 {
		t.Fatalf("events limit forwarded as %d, want 7", got)
	}
	ver := apiV1Do(t, ops, http.MethodGet, base+"/ghost.alpha/verification", "")
	var view struct {
		Records []GhostVerificationRecord `json:"records"`
	}
	if err := json.Unmarshal(ver.Data, &view); !ver.OK || err != nil || len(view.Records) != 1 ||
		view.Records[0].RequestID != "req.1" || view.Records[0].CommandID != "cmd.1" {
		t.Fatalf("verification: %d %s %v", ver.Status, ver.Data, err)
	}
	if got := <-limits; got != 0 {
		t.Fatalf("verification limit forwarded as %d, want the Ghost default (0)", got)
	}

	wantAPIv1Status(t, "unknown ghost", apiV1Do(t, ops, http.MethodGet, base+"/ghost.missing/status", ""), 404, 1300)
	wantAPIv1Status(t, "bad limit", apiV1Do(t, ops, http.MethodGet, base+"/ghost.alpha/events?limit=-1", ""), 400, 1300)
	ghostClient := apiV1ClientFor(t, p, identity.RoleGhost, "ghost.beta")
	wantAPIv1Status(t, "ghost reads ghost status", apiV1Do(t, ghostClient, http.MethodGet, base+"/ghost.alpha/status", ""), 403, 1000)

	// A Ghost that cannot be reached is a transport failure the client may retry.
	svc.bindGhostAdmin("ghost.down", "127.0.0.1:1")
	down := apiV1Do(t, ops, http.MethodGet, base+"/ghost.down/status", "")
	if down.OK || down.Status != 502 || down.Error == nil || !down.Error.Retryable {
		t.Fatalf("unreachable ghost: status=%d error=%+v", down.Status, down.Error)
	}
}

// Activity returns this process's audit and custody records; denials are recorded too
// (api_v1.toml [parity.activity]).
func TestAPIv1Activity(t *testing.T) {
	testlog.Start(t)
	activity.Reset()
	t.Cleanup(activity.Reset)
	p := tlstest.NewPKI(t)
	svc := tlsMirageService(t, p)
	base := serveAPIv1ForTest(t, svc) + "/api/v1"
	ops := apiV1ClientFor(t, p, identity.RoleOperator, "ops")
	ghostClient := apiV1ClientFor(t, p, identity.RoleGhost, "ghost.alpha")

	if r := apiV1Do(t, ops, http.MethodGet, base+"/status", ""); !r.OK {
		t.Fatalf("status: %+v", r.Error)
	}
	wantAPIv1Status(t, "ghost reads activity", apiV1Do(t, ghostClient, http.MethodGet, base+"/activity", ""), 403, 1000)

	got := apiV1Do(t, ops, http.MethodGet, base+"/activity?kind=audit", "")
	var out struct {
		Records []activity.Record `json:"records"`
	}
	if err := json.Unmarshal(got.Data, &out); !got.OK || err != nil || len(out.Records) != 2 {
		t.Fatalf("activity: %d %s %v", got.Status, got.Data, err)
	}
	allow, deny := out.Records[0], out.Records[1]
	if allow.Kind != activity.KindAudit || allow.TimestampMS == 0 || allow.Fields["action"] != "status" ||
		allow.Fields["decision"] != "allow" || allow.Fields["principal"] != "edgectl://operator/ops" {
		t.Fatalf("allow record: %+v", allow)
	}
	if deny.Fields["action"] != "activity" || deny.Fields["decision"] != "deny" || deny.Fields["principal"] != "edgectl://ghost/ghost.alpha" {
		t.Fatalf("deny record: %+v", deny)
	}

	one := apiV1Do(t, ops, http.MethodGet, base+"/activity?limit=1", "")
	if err := json.Unmarshal(one.Data, &out); !one.OK || err != nil || len(out.Records) != 1 {
		t.Fatalf("limit=1: %s %v", one.Data, err)
	}
	wantAPIv1Status(t, "bad kind", apiV1Do(t, ops, http.MethodGet, base+"/activity?kind=secrets", ""), 400, 1300)
}

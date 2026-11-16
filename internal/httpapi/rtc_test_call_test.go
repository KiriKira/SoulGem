package httpapi

import (
	"encoding/json"
	"testing"
)

type testCallReply struct {
	RequestID string `json:"request_id"`
	GatewayID string `json:"gateway_id"`
}

func requestTestCall(t *testing.T, api *Server, token, gateway, operation string, status int) testCallReply {
	t.Helper()
	reply := requestJSON(t, api.Handler(), "POST", "/v1/rtc/control/test-calls", token,
		map[string]any{"version": 1, "operation_id": operation, "gateway_id": gateway}, "")
	requireStatus(t, reply, status)
	var out testCallReply
	_ = json.Unmarshal(reply.Body.Bytes(), &out)
	return out
}

func gatewayTestRequests(t *testing.T, api *Server, token string) []rtcTestCallPending {
	t.Helper()
	reply := requestJSON(t, api.Handler(), "GET", "/v1/rtc/control/calls", token, nil, "")
	requireStatus(t, reply, 200)
	var out struct {
		Requests []rtcTestCallPending `json:"test_call_requests"`
	}
	if err := json.Unmarshal(reply.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out.Requests
}

func createTestIncoming(t *testing.T, api *Server, f integrationFixture, request string, status int) rtcControlSnapshot {
	t.Helper()
	body := rtcCreateCall{Version: 1, OperationID: newUUID(), CallID: newUUID(), GatewayID: f.gatewayID, SIMID: f.simID,
		MappingRevision: 1, From: "测试通话", TestRequestID: request}
	reply := requestJSON(t, api.Handler(), "POST", "/v1/rtc/control/incoming", f.gatewayToken, body, "")
	requireStatus(t, reply, status)
	var out rtcControlSnapshot
	_ = json.Unmarshal(reply.Body.Bytes(), &out)
	return out
}

func TestRTCTestCallRingsOnlyTheRequesterAndPlaysThrough(t *testing.T) {
	db, f, api := controlFixture(t)
	// A second host of the same owner receives ordinary incoming calls.
	second, secondToken := newUUID(), randomToken()
	for _, q := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO devices(id,owner_id,role,name) VALUES ($1,$2,'client','client-a2')`, []any{second, f.ownerID}},
		{`INSERT INTO sessions(id,device_id,access_hash,access_expires_at,refresh_hash,refresh_expires_at) VALUES ($1,$2,$3,now()+interval '1 day',$4,now()+interval '30 days')`, []any{newUUID(), second, tokenHash(secondToken), tokenHash(randomToken())}},
	} {
		if _, err := db.db.Exec(q.query, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	requireStatus(t, requestJSON(t, api.Handler(), "POST", "/v1/rtc/control/capability", secondToken, map[string]any{"version": 1, "enabled": true}, ""), 200)

	// Only hosts ask; the gateway and other owners cannot.
	requestTestCall(t, api, f.gatewayToken, f.gatewayID, newUUID(), 403)
	requestTestCall(t, api, f.otherToken, f.gatewayID, newUUID(), 403)
	operation := newUUID()
	first := requestTestCall(t, api, f.clientToken, f.gatewayID, operation, 201)
	if again := requestTestCall(t, api, f.clientToken, f.gatewayID, operation, 200); again.RequestID != first.RequestID {
		t.Fatal("retry created another request")
	}
	requestTestCall(t, api, f.clientToken, f.otherGatewayID, operation, 403)
	// One pending test per gateway.
	requestTestCall(t, api, secondToken, f.gatewayID, newUUID(), 409)

	if list := gatewayTestRequests(t, api, f.gatewayToken); len(list) != 1 || list[0].RequestID != first.RequestID || list[0].ClientID != f.clientID {
		t.Fatalf("gateway did not see the request: %+v", list)
	}
	if list := gatewayTestRequests(t, api, f.clientToken); list != nil {
		t.Fatal("a host was shown gateway test requests")
	}

	// An unknown request id never creates a call.
	createTestIncoming(t, api, f, newUUID(), 404)
	out := createTestIncoming(t, api, f, first.RequestID, 201)
	if !out.Test || out.State != "ringing" {
		t.Fatalf("test call not marked: %+v", out)
	}
	var ringing []string
	rows, err := db.db.Query(`SELECT client_device_id::text FROM call_participants WHERE call_id=$1`, out.CallID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		ringing = append(ringing, id)
	}
	rows.Close()
	if len(ringing) != 1 || ringing[0] != f.clientID {
		t.Fatalf("test call rang %v, want only the requester", ringing)
	}
	if list := gatewayTestRequests(t, api, f.gatewayToken); len(list) != 0 {
		t.Fatal("consumed request still pending")
	}
	// The consumed request cannot start a second call: the slot is held.
	createTestIncoming(t, api, f, first.RequestID, 409)
	requestTestCall(t, api, f.clientToken, f.gatewayID, newUUID(), 409)
	if snap := controlGet(t, api, f.clientToken, out.CallID); !snap.Test {
		t.Fatal("host snapshot lost the test mark")
	}
	requireStatus(t, requestJSON(t, api.Handler(), "GET", "/v1/rtc/control/calls/"+out.CallID, secondToken, nil, ""), 404)

	// The rest is an ordinary incoming call.
	out = controlOp(t, api, f.clientToken, out, "accept", 200)
	admission := negotiateControl(t, api, f, out.CallID)
	out = controlOp(t, api, f.gatewayToken, out, "winner", 200)
	requireStatus(t, requestJSON(t, api.Handler(), "POST", "/v1/calls/"+out.CallID+"/media/active", f.gatewayToken, map[string]string{"leg_id": admission.Binding.LegID}, ""), 200)
	out = controlOp(t, api, f.gatewayToken, out, "active", 200)
	out = controlOp(t, api, f.gatewayToken, out, "ended", 200)
	if out.State != "ended" || !out.Test {
		t.Fatal("test call did not end")
	}
	// Once the slot is free, the used request still cannot start a call.
	createTestIncoming(t, api, f, first.RequestID, 404)
	// The gateway is free for the next test.
	requestTestCall(t, api, f.clientToken, f.gatewayID, newUUID(), 201)
}

func TestRTCTestCallRequestExpiresAndNeedsALiveGateway(t *testing.T) {
	db, f, api := controlFixture(t)
	first := requestTestCall(t, api, f.clientToken, f.gatewayID, newUUID(), 201)
	if _, err := db.db.Exec(`UPDATE rtc_test_call_requests SET expires_at=clock_timestamp()-interval '1 second' WHERE request_id=$1`, first.RequestID); err != nil {
		t.Fatal(err)
	}
	if list := gatewayTestRequests(t, api, f.gatewayToken); len(list) != 0 {
		t.Fatal("expired request listed")
	}
	createTestIncoming(t, api, f, first.RequestID, 404)
	// An expired request no longer blocks a new one.
	second := requestTestCall(t, api, f.clientToken, f.gatewayID, newUUID(), 201)
	if second.RequestID == first.RequestID {
		t.Fatal("expired request reused")
	}
	if _, err := db.db.Exec(`UPDATE gateways SET last_seen_at=now()-interval '5 minutes' WHERE device_id=$1`, f.gatewayID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`DELETE FROM rtc_test_call_requests`); err != nil {
		t.Fatal(err)
	}
	requestTestCall(t, api, f.clientToken, f.gatewayID, newUUID(), 503)
	// A test request id is for incoming calls only.
	body := rtcCreateCall{Version: 1, OperationID: newUUID(), CallID: newUUID(), GatewayID: f.gatewayID, SIMID: f.simID, MappingRevision: 1, To: "15550100200", TestRequestID: second.RequestID}
	requireStatus(t, requestJSON(t, api.Handler(), "POST", "/v1/rtc/control/calls", f.clientToken, body, ""), 400)
}

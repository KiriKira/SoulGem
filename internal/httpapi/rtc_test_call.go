package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/kirikira/gsm2sip-server/internal/calls"
)

// testCallWindow is how long a request waits for its gateway. The gateway
// polls call control every 750 ms while idle, so this leaves room for a
// briefly unreachable gateway without letting a stale request ring later.
const testCallWindow = 30 * time.Second

type rtcTestCallRequest struct {
	Version     int    `json:"version"`
	OperationID string `json:"operation_id"`
	GatewayID   string `json:"gateway_id"`
}

type rtcTestCallPending struct {
	RequestID string    `json:"request_id"`
	ClientID  string    `json:"client_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

// rtcTestCallCreate lets a host ask its gateway for a test call: a virtual
// incoming call that plays a recording and rings this host alone. The
// request reserves nothing; the gateway creates the call through the
// ordinary incoming path, which applies every SIM, slot and lease rule.
func (s *Server) rtcTestCallCreate(w http.ResponseWriter, r *http.Request, p Principal) {
	if !s.requireRTCControl(w) {
		return
	}
	if p.Role != "client" {
		writeCallError(w, calls.ErrForbidden)
		return
	}
	var body rtcTestCallRequest
	if !decodeOrError(w, r, &body, 1024) {
		return
	}
	body.OperationID = strings.ToLower(body.OperationID)
	body.GatewayID = strings.ToLower(body.GatewayID)
	if body.Version != 1 || !validUUID(body.OperationID) || !validUUID(body.GatewayID) {
		writeCallError(w, calls.ErrInvalid)
		return
	}
	ctx := r.Context()
	tx, err := beginDurable(ctx, s.db)
	if err != nil {
		writeDBUnavailable(w)
		return
	}
	defer tx.Rollback()
	if err = s.rtcControlIdentity(ctx, tx, r, p, body.GatewayID); err != nil {
		writeCallError(w, err)
		return
	}
	// A retry of the same operation answers with the request it created.
	var out rtcTestCallPending
	var gateway string
	err = tx.QueryRowContext(ctx, `SELECT request_id::text,gateway_id::text,expires_at FROM rtc_test_call_requests WHERE client_device_id=$1 AND operation_id=$2`, p.DeviceID, body.OperationID).Scan(&out.RequestID, &gateway, &out.ExpiresAt)
	if err == nil {
		if gateway != body.GatewayID {
			writeCallError(w, calls.ErrConflict)
			return
		}
		if err = tx.Commit(); err != nil {
			writeDBUnavailable(w)
			return
		}
		out.ClientID = p.DeviceID
		writeJSON(w, 200, testCallResponse(out, body.GatewayID))
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		writeDBUnavailable(w)
		return
	}
	var available bool
	err = tx.QueryRowContext(ctx, `SELECT g.last_seen_at>clock_timestamp()-interval '90 seconds' AND EXISTS(SELECT 1 FROM rtc_control_capabilities c WHERE c.device_id=g.device_id AND c.enabled AND c.last_seen_at>clock_timestamp()-interval '90 seconds')
      FROM gateways g JOIN devices d ON d.id=g.device_id WHERE g.device_id=$1 AND d.owner_id=$2 AND d.state='active' FOR UPDATE OF g`, body.GatewayID, p.OwnerID).Scan(&available)
	if errors.Is(err, sql.ErrNoRows) || err == nil && !available {
		writeCallError(w, calls.ErrGatewayUnavailable)
		return
	}
	if err != nil {
		writeDBUnavailable(w)
		return
	}
	// One test at a time, and never over a real call: the gateway would
	// refuse the incoming create anyway, after the host had waited for it.
	var busy bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM gateway_call_slots WHERE gateway_id=$1)
      OR EXISTS(SELECT 1 FROM rtc_test_call_requests WHERE gateway_id=$1 AND call_id IS NULL AND expires_at>clock_timestamp())`, body.GatewayID).Scan(&busy)
	if err != nil {
		writeDBUnavailable(w)
		return
	}
	if busy {
		writeCallError(w, calls.ErrBusy)
		return
	}
	out.RequestID = newUUID()
	out.ClientID = p.DeviceID
	err = tx.QueryRowContext(ctx, `INSERT INTO rtc_test_call_requests(request_id,owner_id,client_device_id,gateway_id,operation_id,expires_at) VALUES($1,$2,$3,$4,$5,clock_timestamp()+($6*interval '1 millisecond')) RETURNING expires_at`,
		out.RequestID, p.OwnerID, p.DeviceID, body.GatewayID, body.OperationID, testCallWindow.Milliseconds()).Scan(&out.ExpiresAt)
	if err != nil {
		writeDBUnavailable(w)
		return
	}
	if err = tx.Commit(); err != nil {
		writeDBUnavailable(w)
		return
	}
	s.logger.Info("RTC test call requested", "request_id", out.RequestID, "gateway_id", body.GatewayID)
	s.kickWake()
	writeJSON(w, 201, testCallResponse(out, body.GatewayID))
}

func testCallResponse(out rtcTestCallPending, gateway string) map[string]any {
	return map[string]any{"version": 1, "request_id": out.RequestID, "gateway_id": gateway, "expires_at": out.ExpiresAt.UTC().Format(time.RFC3339Nano)}
}

// pendingTestCalls lists the unexpired, unconsumed test requests for a
// gateway, oldest first. The gateway reads them with its call snapshot.
func pendingTestCalls(ctx context.Context, tx *sql.Tx, p Principal) ([]rtcTestCallPending, error) {
	rows, err := tx.QueryContext(ctx, `SELECT request_id::text,client_device_id::text,expires_at FROM rtc_test_call_requests
      WHERE gateway_id=$1 AND owner_id=$2 AND call_id IS NULL AND expires_at>clock_timestamp() ORDER BY created_at LIMIT 4`, p.DeviceID, p.OwnerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []rtcTestCallPending{}
	for rows.Next() {
		var item rtcTestCallPending
		if err = rows.Scan(&item.RequestID, &item.ClientID, &item.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// claimTestCall locks a gateway's pending request and returns the host it
// rings. The request stays pending until the call row exists and is linked.
func claimTestCall(ctx context.Context, tx *sql.Tx, p Principal, request string) (string, error) {
	var client string
	err := tx.QueryRowContext(ctx, `SELECT r.client_device_id::text FROM rtc_test_call_requests r JOIN devices d ON d.id=r.client_device_id
      WHERE r.request_id=$1 AND r.owner_id=$2 AND r.gateway_id=$3 AND r.call_id IS NULL AND r.expires_at>clock_timestamp()
        AND d.state='active' AND EXISTS(SELECT 1 FROM sessions ss WHERE ss.device_id=d.id AND ss.refresh_expires_at>clock_timestamp())
      FOR UPDATE OF r`, request, p.OwnerID, p.DeviceID).Scan(&client)
	if errors.Is(err, sql.ErrNoRows) {
		return "", calls.ErrNotFound
	}
	return client, err
}

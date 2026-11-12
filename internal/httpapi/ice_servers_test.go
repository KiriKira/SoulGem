package httpapi

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestIceSourceWithoutCallAndRevocation(t *testing.T) {
	db := openIntegrationDatabase(t)
	f := seedIntegrationFixture(t, db.db)
	secret := strings.Repeat("s", 32)
	urls := []string{"turn:relay.example:49100?transport=udp", "turns:relay.example:5349?transport=tcp"}
	api := NewWithOptions(db.db, nil, Options{RTC: RTCMediaOptions{TURNURLs: urls, TURNSecret: secret}})
	t.Cleanup(api.ShutdownWebSockets)
	for _, token := range []string{f.clientToken, f.gatewayToken} {
		reply := requestJSON(t, api.Handler(), "GET", "/v1/rtc/ice-servers", token, nil, "")
		requireStatus(t, reply, 200)
		if reply.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("credentials may be cached")
		}
		var out struct {
			Servers []struct {
				URLs       []string `json:"urls"`
				Username   string   `json:"username"`
				Credential string   `json:"credential"`
			} `json:"ice_servers"`
			Expiry time.Time `json:"expires_at"`
		}
		if err := json.Unmarshal(reply.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if len(out.Servers) != 1 || fmt.Sprint(out.Servers[0].URLs) != fmt.Sprint(urls) {
			t.Fatal("custom endpoints changed")
		}
		mac := hmac.New(sha1.New, []byte(secret))
		mac.Write([]byte(out.Servers[0].Username))
		if base64.StdEncoding.EncodeToString(mac.Sum(nil)) != out.Servers[0].Credential {
			t.Fatal("invalid TURN credential")
		}
		if !strings.HasPrefix(out.Servers[0].Username, fmt.Sprintf("%d:", out.Expiry.Unix())) {
			t.Fatal("expiry mismatch")
		}
	}
	requireStatus(t, requestJSON(t, api.Handler(), "GET", "/v1/rtc/ice-servers", "invalid", nil, ""), 401)
	if _, err := db.db.Exec(`UPDATE devices SET state='revoked' WHERE id=$1`, f.clientID); err != nil {
		t.Fatal(err)
	}
	requireStatus(t, requestJSON(t, api.Handler(), "GET", "/v1/rtc/ice-servers", f.clientToken, nil, ""), 401)
	var calls int
	if err := db.db.QueryRow(`SELECT count(*) FROM call_sessions`).Scan(&calls); err != nil || calls != 0 {
		t.Fatal("issuing ICE credentials created a call")
	}
}

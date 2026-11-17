-- Test calls: a host asks its gateway for a virtual incoming call that plays
-- a recording instead of bridging a cellular call. The request only waits for
-- the gateway; the call itself is an ordinary RTC incoming call, marked test,
-- that rings the requesting host alone. A request expires unconsumed, and a
-- consumed request names its call so it can never start a second one.
CREATE TABLE rtc_test_call_requests (
    request_id UUID PRIMARY KEY,
    owner_id UUID NOT NULL REFERENCES owners(id) ON DELETE CASCADE,
    client_device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    gateway_id UUID NOT NULL REFERENCES gateways(device_id) ON DELETE CASCADE,
    operation_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    expires_at TIMESTAMPTZ NOT NULL,
    call_id UUID UNIQUE REFERENCES call_sessions(call_id) ON DELETE SET NULL,
    UNIQUE (client_device_id, operation_id)
);
CREATE INDEX rtc_test_call_requests_pending ON rtc_test_call_requests(gateway_id, expires_at) WHERE call_id IS NULL;

ALTER TABLE call_sessions ADD COLUMN test BOOLEAN NOT NULL DEFAULT false;

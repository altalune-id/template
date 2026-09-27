package webhook_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"altalune.id/template/httpclient"
	"altalune.id/template/internal/platform/events"
	"altalune.id/template/internal/platform/outbox"
	"altalune.id/template/internal/platform/sealer"
	"altalune.id/template/internal/platform/tenant"
	"altalune.id/template/internal/testutil/fakes"
	"altalune.id/template/internal/webhook"
)

type captured struct {
	header http.Header
	body   []byte
}

type receiver struct {
	srv  *httptest.Server
	reqs chan captured
	hits atomic.Int32
}

func newReceiver(t *testing.T, status int) *receiver {
	t.Helper()
	r := &receiver{reqs: make(chan captured, 8)}
	mux := http.NewServeMux()
	mux.HandleFunc("/hook", func(w http.ResponseWriter, req *http.Request) {
		r.hits.Add(1)
		body, _ := io.ReadAll(req.Body)
		r.reqs <- captured{header: req.Header.Clone(), body: body}
		w.WriteHeader(status)
	})
	r.srv = httptest.NewTLSServer(mux)
	t.Cleanup(r.srv.Close)
	return r
}

func (r *receiver) request(t *testing.T) captured {
	t.Helper()
	select {
	case c := <-r.reqs:
		return c
	default:
		t.Fatal("receiver got no request")
		return captured{}
	}
}

type delivery struct {
	store   *fakes.WebhookStore
	sealer  sealer.Sealer
	logs    *bytes.Buffer
	d       *webhook.Deliverer
	orgID   uuid.UUID
	project uuid.UUID
}

func newDelivery(t *testing.T, client *http.Client) *delivery {
	t.Helper()
	x := &delivery{
		store:   fakes.NewWebhookStore(),
		sealer:  newSealer(t),
		logs:    &bytes.Buffer{},
		orgID:   uuid.New(),
		project: uuid.New(),
	}
	log := slog.New(slog.NewJSONHandler(x.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	x.d = webhook.NewDeliverer(x.store, x.sealer, client, log)
	return x
}

func (x *delivery) orgCtx(t *testing.T) context.Context {
	t.Helper()
	return tenant.Into(t.Context(), tenant.Context{OrgID: x.orgID})
}

func (x *delivery) endpoint(t *testing.T, rawURL string) (*webhook.Endpoint, string) {
	t.Helper()
	e, err := webhook.New(x.orgID, x.project, rawURL, "orders", []events.Type{events.PostPublished})
	require.NoError(t, err)
	secret, err := webhook.NewSecret()
	require.NoError(t, err)
	e.Secrets.Primary, err = webhook.SealSecret(x.sealer, e.ID, webhook.SlotPrimary, secret)
	require.NoError(t, err)
	x.store.Seed(e)
	return e, secret
}

func (x *delivery) entry(t *testing.T, e *webhook.Endpoint) outbox.Entry {
	t.Helper()
	eventID := uuid.Must(uuid.NewV7())
	payload, err := json.Marshal(map[string]any{
		"id":          webhook.EventIDPrefix + eventID.String(),
		"type":        string(events.PostPublished),
		"api_version": "v1",
		"data":        map[string]any{"id": uuid.NewString()},
	})
	require.NoError(t, err)
	return outbox.Entry{
		ID:        uuid.Must(uuid.NewV7()),
		EventID:   eventID,
		OrgID:     e.OrgID,
		ProjectID: e.ProjectID,
		Target:    e.ID.String(),
		Payload:   payload,
		Attempt:   1,
	}
}

func hookURL(r *receiver) string { return r.srv.URL + "/hook" }

func TestDeliverer_Success(t *testing.T) {
	r := newReceiver(t, http.StatusOK)
	x := newDelivery(t, r.srv.Client())
	e, secret := x.endpoint(t, hookURL(r))
	entry := x.entry(t, e)

	require.NoError(t, x.d.Deliver(x.orgCtx(t), entry))

	got := r.request(t)
	assert.Equal(t, entry.Payload, got.body, "the body is the stored payload byte for byte")
	assert.Equal(t, "application/json", got.header.Get("Content-Type"))
	assert.Equal(t, "Altempl-Webhooks/1", got.header.Get("User-Agent"))
	assert.Equal(t, webhook.EventIDPrefix+entry.EventID.String(), got.header.Get("X-Altempl-Event-Id"))
	assert.Equal(t, string(events.PostPublished), got.header.Get("X-Altempl-Event-Type"))
	assert.Equal(t, webhook.DeliveryIDPrefix+entry.ID.String(), got.header.Get("X-Altempl-Delivery-Id"))

	ts := got.header.Get("X-Altempl-Timestamp")
	unix, err := strconv.ParseInt(ts, 10, 64)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now(), time.Unix(unix, 0), time.Minute)
	assert.Equal(t, webhook.Sign(secret, ts, got.body), got.header.Get("X-Altempl-Signature"))

	attempts := x.store.Attempts()
	require.Len(t, attempts, 1)
	a := attempts[0]
	assert.Equal(t, http.StatusOK, a.StatusCode)
	assert.Empty(t, a.Error)
	assert.Equal(t, 1, a.Attempt)
	assert.Equal(t, x.orgID, a.OrgID)
	assert.Equal(t, x.project, a.ProjectID)
	assert.Equal(t, e.ID, a.EndpointID)
	assert.Equal(t, entry.ID, a.DeliveryID)
	assert.Equal(t, entry.EventID, a.EventID)
	assert.Equal(t, events.PostPublished, a.EventType)
	assert.NotContains(t, x.logs.String(), "delivery exhausted")
}

func TestDeliverer_SignsWithBothSecretsPrimaryFirst(t *testing.T) {
	r := newReceiver(t, http.StatusNoContent)
	x := newDelivery(t, r.srv.Client())
	e, primary := x.endpoint(t, hookURL(r))
	secondary, err := webhook.NewSecret()
	require.NoError(t, err)
	e.Secrets.Secondary, err = webhook.SealSecret(x.sealer, e.ID, webhook.SlotSecondary, secondary)
	require.NoError(t, err)
	x.store.Seed(e)

	require.NoError(t, x.d.Deliver(x.orgCtx(t), x.entry(t, e)))

	got := r.request(t)
	ts := got.header.Get("X-Altempl-Timestamp")
	want := webhook.Sign(primary, ts, got.body) + " " + webhook.Sign(secondary, ts, got.body)
	assert.Equal(t, want, got.header.Get("X-Altempl-Signature"))
	assert.Len(t, got.header.Values("X-Altempl-Signature"), 1)
}

func TestDeliverer_Non2xxFails(t *testing.T) {
	r := newReceiver(t, http.StatusInternalServerError)
	x := newDelivery(t, r.srv.Client())
	e, _ := x.endpoint(t, hookURL(r))

	err := x.d.Deliver(x.orgCtx(t), x.entry(t, e))

	failed, ok := errors.AsType[*webhook.DeliveryFailedError](err)
	require.True(t, ok, "got %T: %v", err, err)
	assert.Equal(t, http.StatusInternalServerError, failed.StatusCode)
	attempts := x.store.Attempts()
	require.Len(t, attempts, 1)
	assert.Equal(t, http.StatusInternalServerError, attempts[0].StatusCode)
	assert.NotEmpty(t, attempts[0].Error)
}

func TestDeliverer_RedirectIsAFailure(t *testing.T) {
	var followed atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/hook", func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, "/elsewhere", http.StatusFound)
	})
	mux.HandleFunc("/elsewhere", func(w http.ResponseWriter, _ *http.Request) {
		followed.Add(1)
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	x := newDelivery(t, srv.Client())
	e, _ := x.endpoint(t, srv.URL+"/hook")

	err := x.d.Deliver(x.orgCtx(t), x.entry(t, e))

	failed, ok := errors.AsType[*webhook.DeliveryFailedError](err)
	require.True(t, ok, "got %T: %v", err, err)
	assert.Equal(t, http.StatusFound, failed.StatusCode)
	assert.Zero(t, followed.Load(), "the redirect target must never be hit")
	assert.Nil(t, srv.Client().CheckRedirect, "the injected client is copied, not mutated")
}

func TestDeliverer_DeletedEndpointSettlesSilently(t *testing.T) {
	r := newReceiver(t, http.StatusOK)
	x := newDelivery(t, r.srv.Client())
	e, _ := x.endpoint(t, hookURL(r))
	entry := x.entry(t, e)
	entry.Target = uuid.NewString()

	require.NoError(t, x.d.Deliver(x.orgCtx(t), entry))

	assert.Zero(t, r.hits.Load())
	assert.Empty(t, x.store.Attempts())
	assert.Contains(t, x.logs.String(), `"level":"INFO"`)
}

func TestDeliverer_ProjectMismatchNeverDelivers(t *testing.T) {
	r := newReceiver(t, http.StatusOK)
	x := newDelivery(t, r.srv.Client())
	e, _ := x.endpoint(t, hookURL(r))
	entry := x.entry(t, e)
	entry.ProjectID = uuid.New()

	require.NoError(t, x.d.Deliver(x.orgCtx(t), entry))

	assert.Zero(t, r.hits.Load())
	assert.Empty(t, x.store.Attempts())
	logs := x.logs.String()
	assert.Contains(t, logs, `"level":"ERROR"`)
	assert.Contains(t, logs, e.ID.String())
	assert.Contains(t, logs, entry.ProjectID.String())
}

func TestDeliverer_InactiveEndpointFails(t *testing.T) {
	r := newReceiver(t, http.StatusOK)
	x := newDelivery(t, r.srv.Client())
	e, _ := x.endpoint(t, hookURL(r))
	e.Active = false
	x.store.Seed(e)

	err := x.d.Deliver(x.orgCtx(t), x.entry(t, e))

	assert.True(t, webhook.IsEndpointInactiveError(err), "got %T: %v", err, err)
	assert.Zero(t, r.hits.Load())
}

func TestDeliverer_UnopenableSecretFails(t *testing.T) {
	r := newReceiver(t, http.StatusOK)
	x := newDelivery(t, r.srv.Client())
	e, _ := x.endpoint(t, hookURL(r))
	var err error
	e.Secrets.Primary, err = webhook.SealSecret(newSealer(t), e.ID, webhook.SlotPrimary, "whsec_other")
	require.NoError(t, err)
	x.store.Seed(e)

	err = x.d.Deliver(x.orgCtx(t), x.entry(t, e))

	assert.True(t, webhook.IsSecretUnavailableError(err), "got %T: %v", err, err)
	assert.Contains(t, err.Error(), "rotate")
	assert.Zero(t, r.hits.Load())
}

func TestDeliverer_AttemptRecordFailureKeepsOutcome(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		wantErr bool
	}{
		{"success", http.StatusOK, false},
		{"failure", http.StatusBadGateway, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newReceiver(t, tc.status)
			x := newDelivery(t, r.srv.Client())
			e, _ := x.endpoint(t, hookURL(r))
			x.store.SaveAttemptErr = errors.New("db down")

			err := x.d.Deliver(x.orgCtx(t), x.entry(t, e))

			assert.Equal(t, tc.wantErr, webhook.IsDeliveryFailedError(err), "got %T: %v", err, err)
			if !tc.wantErr {
				assert.NoError(t, err)
			}
			assert.Contains(t, x.logs.String(), "db down")
		})
	}
}

func TestDeliverer_FinalAttemptLogsExhausted(t *testing.T) {
	r := newReceiver(t, http.StatusServiceUnavailable)
	x := newDelivery(t, r.srv.Client())
	e, _ := x.endpoint(t, hookURL(r))
	entry := x.entry(t, e)
	entry.Attempt = outbox.MaxAttempts - 1

	require.Error(t, x.d.Deliver(x.orgCtx(t), entry))
	assert.NotContains(t, x.logs.String(), "webhook: delivery exhausted")

	entry.Attempt = outbox.MaxAttempts
	require.Error(t, x.d.Deliver(x.orgCtx(t), entry))

	var warn map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(x.logs.String()), "\n") {
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec))
		if rec["msg"] == "webhook: delivery exhausted" {
			warn = rec
		}
	}
	require.NotNil(t, warn, "logs: %s", x.logs.String())
	assert.Equal(t, "WARN", warn["level"])
	assert.Equal(t, x.orgID.String(), warn["org_id"])
	assert.Equal(t, x.project.String(), warn["project_id"])
	assert.Equal(t, e.ID.String(), warn["endpoint_id"])
	assert.Equal(t, entry.ID.String(), warn["delivery_id"])
}

func TestDeliverer_RefusesPrivateAddresses(t *testing.T) {
	r := newReceiver(t, http.StatusOK)
	x := newDelivery(t, httpclient.New())
	e, _ := x.endpoint(t, hookURL(r))
	require.True(t, strings.HasPrefix(e.URL, "https://127.0.0.1:"), e.URL)

	err := x.d.Deliver(x.orgCtx(t), x.entry(t, e))

	assert.True(t, httpclient.IsPrivateAddressError(err), "got %T: %v", err, err)
	assert.Zero(t, r.hits.Load())
	attempts := x.store.Attempts()
	require.Len(t, attempts, 1)
	assert.Zero(t, attempts[0].StatusCode)
	assert.NotEmpty(t, attempts[0].Error)
}

type sqliteDelivery struct {
	store  webhook.Store
	tc     tenant.Context
	e      *webhook.Endpoint
	secret string
	d      *webhook.Deliverer
	logs   *bytes.Buffer
}

func newSQLiteDelivery(t *testing.T, client *http.Client, rawURL string) *sqliteDelivery {
	t.Helper()
	store, _, tc := newSQLiteStore(t)
	sl := newSealer(t)
	e, err := webhook.New(tc.OrgID, tc.ProjectID, rawURL, "orders", []events.Type{events.PostPublished})
	require.NoError(t, err)
	secret, err := webhook.NewSecret()
	require.NoError(t, err)
	e.Secrets.Primary, err = webhook.SealSecret(sl, e.ID, webhook.SlotPrimary, secret)
	require.NoError(t, err)
	require.NoError(t, store.Save(tenant.Into(t.Context(), tc), e))
	logs := &bytes.Buffer{}
	d := webhook.NewDeliverer(store, sl, client, slog.New(slog.NewJSONHandler(logs, nil)))
	return &sqliteDelivery{store: store, tc: tc, e: e, secret: secret, d: d, logs: logs}
}

func (s *sqliteDelivery) entry(t *testing.T) outbox.Entry {
	t.Helper()
	x := &delivery{orgID: s.tc.OrgID, project: s.tc.ProjectID}
	return x.entry(t, s.e)
}

func (s *sqliteDelivery) attempts(t *testing.T, deliveryID uuid.UUID) []webhook.Attempt {
	t.Helper()
	got, err := s.store.ListAttempts(tenant.Into(t.Context(), s.tc), s.e.ID, deliveryID)
	require.NoError(t, err)
	return got
}

func TestDeliverer_RecordsAttemptInSQLite(t *testing.T) {
	r := newReceiver(t, http.StatusAccepted)
	s := newSQLiteDelivery(t, r.srv.Client(), hookURL(r))
	entry := s.entry(t)

	require.NoError(t, s.d.Deliver(tenant.Into(t.Context(), tenant.Context{OrgID: s.tc.OrgID}), entry))

	got := r.request(t)
	assert.Equal(t, webhook.Sign(s.secret, got.header.Get("X-Altempl-Timestamp"), got.body), got.header.Get("X-Altempl-Signature"))
	attempts := s.attempts(t, entry.ID)
	require.Len(t, attempts, 1)
	assert.Equal(t, http.StatusAccepted, attempts[0].StatusCode)
	assert.Equal(t, events.PostPublished, attempts[0].EventType)
	assert.Equal(t, entry.EventID, attempts[0].EventID)
}

func TestDeliverer_RecordsAttemptWhenCancelledMidPost(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	mux := http.NewServeMux()
	mux.HandleFunc("/hook", func(w http.ResponseWriter, _ *http.Request) {
		cancel()
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	s := newSQLiteDelivery(t, srv.Client(), srv.URL+"/hook")
	entry := s.entry(t)

	_ = s.d.Deliver(tenant.Into(ctx, tenant.Context{OrgID: s.tc.OrgID}), entry)

	require.Error(t, ctx.Err())
	assert.Len(t, s.attempts(t, entry.ID), 1, "logs: %s", s.logs.String())
	assert.NotContains(t, s.logs.String(), "webhook: record attempt")
}

func TestDeliverer_RefusesEnvelopeWithoutType(t *testing.T) {
	for name, payload := range map[string]string{
		"undecodable": `not json`,
		"empty type":  `{"type":""}`,
		"no type":     `{"id":"evt_1"}`,
	} {
		t.Run(name, func(t *testing.T) {
			r := newReceiver(t, http.StatusOK)
			x := newDelivery(t, r.srv.Client())
			e, _ := x.endpoint(t, hookURL(r))
			entry := x.entry(t, e)
			entry.Payload = []byte(payload)

			require.Error(t, x.d.Deliver(x.orgCtx(t), entry))
			assert.Zero(t, r.hits.Load())
			assert.Empty(t, x.store.Attempts())
		})
	}
}

func TestDeliverer_LogsNeverCarryTheEndpointURL(t *testing.T) {
	const token = "tok_s3cr3t"
	r := newReceiver(t, http.StatusOK)
	rawURL := hookURL(r) + "?token=" + token
	r.srv.Close()
	x := newDelivery(t, r.srv.Client())
	e, _ := x.endpoint(t, rawURL)
	entry := x.entry(t, e)
	entry.Attempt = outbox.MaxAttempts

	err := x.d.Deliver(x.orgCtx(t), entry)

	require.ErrorContains(t, err, token, "the returned error still feeds the tenant-visible last_error")
	logs := x.logs.String()
	assert.Contains(t, logs, "webhook: delivery exhausted")
	assert.Contains(t, logs, `"err":"Post: `)
	assert.NotContains(t, logs, token)
	assert.NotContains(t, logs, "/hook")
	attempts := x.store.Attempts()
	require.Len(t, attempts, 1)
	assert.NotEmpty(t, attempts[0].Error)
}

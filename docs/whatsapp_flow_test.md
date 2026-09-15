# Testing the WhatsApp order-event flow

There are two ways to exercise the order-status events
(**confirmed → in-transit → delivered**) that drive the WhatsApp templates:

| Harness | File(s) | Network? | DB? | Best for |
|---|---|---|---|---|
| Offline unit tests | `app/services/whatsapp_flow_test.go` | no | in-memory sqlite | CI / quick regression |
| Manual live harness | `cmd/fake-confirmed`, `cmd/fake-inprogress`, `cmd/fake-delivered` | yes (real WhatsApp) | real Postgres | end-to-end check of real links |

---

## 1. Offline unit tests (recommended)

These simulate the three phases with an in-memory SQLite DB and a **fake Cloud
API client** that records what would have been sent. Nothing touches the
network, the settings table, or prod data, so `go test ./...` stays hermetic.

Run:

```bash
make test-whatsapp
# or directly:
go test ./app/services/ -count=1 -run TestWhatsApp -v
```

(`-count=1` bypasses Go's test cache.) All tests pass/fail in a fraction of a
second.

### What each test validates

| Test | Phase | Asserts |
|---|---|---|
| `TestWhatsAppFlowThreePhases` | 1→2→3 | confirmation template + URL button; `shipping` template with driver name + normalized phone (`20123456`→`21620123456`); `delivred_order` template with a rating URL button whose `?id=N&t=<hmac>` token validates; `in_transit_notified_at` / `delivered_notified_at` stamped; no double sends |
| `TestWhatsAppFlowPhoneLanguage` | 2, 3 | a non-216 phone gets `fr_FR` locale |
| `TestWhatsAppFlowBlockedRecipient` | 1, 2, 3 | a phone suppressed after a `#131026` undeliverable (`whatsapp_blocked`) receives none |
| `TestOrderTrackingTokenRoundTrip` | — | signed `t=` token validity (right order: valid; wrong order / empty: invalid; deterministic) |
| `TestWhatsAppSlangAndPhoneNormalization` | — | `216…`→`ar`, else configured default; 8-digit local numbers normalized to `216XXXXXXXX` |

### How it works (test seams)

The production send functions in `app/services/mescolis_handler.go` build their
Cloud API client and shop config through two package-level hooks that the tests
replace:

```go
var newWhatsAppCloudClient = func(phoneNumberID, accessToken string) whatsappSender { ... }
var loadScopedConfigFunc = loadScopedConfig
```

`whenWhatsAppFlowTest` swaps the client for a recorder and the config for an
in-memory `config.Config`, then asserts on the recorded sends (`flowRecordedSend`).
Stamps are verified by reloading the order from the in-memory DB.

### Adding a new scenario

```go
func TestSomething(t *testing.T) {
	_, sends := setupWhatsAppFlowTest(t)
	order := createFlowOrder(t, "21654116584")

	// tweak the order in the DB, then trigger the phase you care about
	db.Get().Model(&order).Update("mescolis_status", "in-progress")
	SendInTransitForOrder(order.ID)

	s := lastSend(t, sends)
	if s.template != "shipping" { t.Errorf(...) }
}
```

<details>
<summary>Real-DB (live-send) variant</summary>

`app/services/mescolis_fake_test.go` is an older integration style that sends the
real templates. It only runs when `TEST_DATABASE_URL` is set, and it **really
sends WhatsApp messages** — use it only when you explicitly want that:

```bash
TEST_DATABASE_URL="postgresql://..." go test ./app/services/ -run TestFakeMescolis -v
```

</details>

---

## 2. Manual live harness (`cmd/fake-*`)

These run against the **real prod DB** and send **real WhatsApp messages** to the
order's phone. Use them when you want the actual tracking/rating links to click
from a phone browser.

### 1. Environment

```bash
export DATABASE_URL="postgresql://<user>:<pass>@<host>/neondb?sslmode=require&channel_binding=require"
export DB_DRIVER=postgres
export SUPERKIT_SECRET="<prod signing secret>"
```

> `DATABASE_URL` and `SUPERKIT_SECRET` are sensitive — never commit them. The
> WhatsApp access token is **not** needed here: the harness reads it from the
> `settings` table (`app_config:AFF-001` → `whatsapp.access_token`).

### 2. Walk the phases

```bash
# Phase 1 — creates a REAL (is_test=false) order and sends order_confirmed_v2.
# Prints the TRACKING URL.
go run ./cmd/fake-confirmed

# Phase 2 — sends the in-transit "shipping" template for that order.
go run ./cmd/fake-inprogress <orderID>

# Phase 3 — marks it delivered and sends "delivred_order" with a RATING URL button.
go run ./cmd/fake-delivered <orderID>
```

Open the printed **TRACKING URL** and **RATING URL** in a phone browser
(RTL / Arabic because the order phone is a 216 number). On the rating page, tap
the stars, then submit — you should land on the "شكرًا لك!" thank-you view
(no CSRF error).

### Notes

- The order phone is hardcoded to `21654116584` in `cmd/fake-confirmed`.
- Orders created by the harness are **kept** on purpose so the tracking/rating
  links stay live; delete them manually later if you don't want them.
- The prod links only resolve after the latest change is deployed to Render.
- Prefix a command with the hooks above, e.g.
  `DATABASE_URL=... go run ./cmd/fake-delivered 24`.
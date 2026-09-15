# shop-ecommerce (shopTemplate)

Multi-tenant e-commerce platform built with Go. Each shop runs behind a store
domain with its own catalog, checkout, WhatsApp order-status notifications and
admin panel.

## Tech Stack

- **Go** (chi + anthdm/superkit, a-h/templ)
- **GORM** — SQLite for development, Postgres for production
- **goose** database migrations
- **Auth**: JWT + gorilla/sessions + OAuth2
- **Frontend**: TailwindCSS 3.4 + esbuild (server-rendered templ views)
- **i18n**: en / fr / ar (Arabic is RTL)
- **Payments**: COD + Flouci
- **Notifications**: WhatsApp Cloud API (status updates, tracking + rating links), Email (Brevo), Telegram, Facebook CAPI
- **Shipping**: Mescolis integration
- **Deploy**: Render (https://shop-ecommerce-9kak.onrender.com)

## Quick start

```bash
cp .env.example .env      # set DATABASE_URL / DB_* / SUPERKIT_SECRET
make db-up                # start local Postgres (or use SQLite)
make db-up                # apply schema migrations (goose)
make db-seed              # seed admin + demo shop
make dev                  # templ + server + Tailwind + esbuild with hot reload
```

Useful make targets: `make server`, `make templ`, `make build`,
`make watch-assets`, `make watch-esbuild`, `make db-down`, `make db-status`,
`make db-mig-create NAME=x`, `make reset-admin-password`,
`make reset-shop AFF_ID=AFF-001`. Config comes from a `.env` file (loaded with
godotenv) or system env vars.

## Testing

```bash
go test ./...          # offline; includes the WhatsApp order-event flow tests
make test-whatsapp     # verbose run of the confirmed → in-transit → delivered flow
```

See [docs/whatsapp_flow_test.md](docs/whatsapp_flow_test.md) for how the
WhatsApp order-event harnesses work (offline unit tests + manual live
`cmd/fake-*` tools).

## Key routes

- `/` — storefront
- `/products`, `/products/{id}` — catalog
- `/cart`, `/checkout` — ordering (COD / Flouci)
- `/tracking?id={id}&t={token}`, `/rating?id={id}&t={token}` — public order pages
  from WhatsApp template buttons (HMAC-signed tokens)
- `/admin/*` — admin panel
- `/api/chat/ws` — customer chat (WebSocket)
- `/webhooks/whatsapp`, `/api/orders`, `/api/commission` — integrations
- `/setup` — first-run shop setup
- `/health`, `/privacy`, `/set-lang/{lang}`

## Environment

Config is stored both in a JSON file and the DB-backed `settings` table
(`app_config` + per-affiliate `app_config:AFF-xxx`). Services read
`config.FromContext(ctx)` per request; background jobs use
`config.LoadByAffiliateID()`.

## Repository layout

- `cmd/app` — entrypoint; `cmd/fake-*` — WhatsApp event test harnesses
- `app/routes.go`, `app/handlers` — HTTP layer
- `app/models`, `app/db` (with `app/db/migrations`) — persistence
- `app/services` — business logic (WhatsApp, Mescolis, payments, i18n)
- `app/views` — templ components
- `docs/` — runbooks
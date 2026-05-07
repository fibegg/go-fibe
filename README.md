# Go Fibe Starter

Production-like Go starter for live development environments. It ships an Uptime Console example with Chi, gqlgen, PostgreSQL, Redis, Asynq jobs, React, TypeScript, Vite HMR, RBAC, rate limits, maintenance tasks, and iframe/proxy security controls.

## Run

```sh
docker compose up --build
```

Open the frontend at `http://localhost:5173` and sign in with:

- Email: `admin@example.com`
- Password: `password`

## Commands

```sh
go-fibe setup
go-fibe serve
go-fibe worker
just check
```

## Security Environment

The template is permissive by default for iframe-based deployments and easy to tighten later:

- `APP_ALLOWED_HOSTS=*`
- `APP_TRUST_PROXY_HEADERS=true`
- `APP_PUBLIC_ORIGIN=`
- `APP_FRAME_ANCESTORS=`
- `APP_X_FRAME_OPTIONS=`
- `APP_CSP_MODE=off|report-only|enforce`
- `APP_CSP_CONNECT_SRC=`
- `APP_CORS_ALLOWED_ORIGINS=*`
- `APP_COOKIE_SECURE=auto`
- `APP_COOKIE_SAMESITE=lax`
- `APP_ALLOW_PRIVATE_MONITOR_URLS=false`

Private, loopback, link-local, and credentialed monitor URLs are blocked unless `APP_ALLOW_PRIVATE_MONITOR_URLS=true`.

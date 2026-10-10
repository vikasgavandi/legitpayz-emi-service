# legitpayz-emi-service (legitPayz)

EMI plans, convert purchase to EMI, pay installment, foreclose. (port `8004`). Part of the legitPayz credit-card microservices system.

## Run locally

Prereqs: Go 1.26+, Postgres (`cc/creditcard` migrated with `migrations/001_init.sql`), Redis.

```bash
PORT=8004 go run .
curl localhost:8004/health
```

## Docker

```bash
docker build -t legitpayz-emi-service .
docker run -p 8004:8004 legitpayz-emi-service
```

## Env

| Var | Purpose |
|-----|---------|
`DATABASE_URL` Postgres DSN

# Installation

## En local

```bash
cd deploy
cp .env.example .env
```

Dans le `.env` : mettre un `POSTGRES_PASSWORD`, et `COOKIE_SECURE=false` parce
que le navigateur refuse un cookie `Secure` en http.

```bash
docker compose up -d --build
```

Ça écoute sur http://localhost:8088.

## Sur le NAS

Il faut en plus un `TUNNEL_TOKEN` dans le `.env`. Il se crée dans Cloudflare
Zero Trust, Networks > Tunnels. Le service public du tunnel doit pointer sur
`http://web:80`.

```bash
docker compose --profile tunnel up -d --build
docker compose logs -f api
```

## Développer sans Docker

```bash
cd deploy && docker compose up -d db
cd ../backend
export DATABASE_URL="postgres://casino:MOTDEPASSE@localhost:5432/casino?sslmode=disable"
export COOKIE_SECURE=false
go run ./cmd/api
```

Le site et l'API écoutent sur http://localhost:8080. Les tables sont créées au
démarrage.

Si un PostgreSQL tourne déjà sur la machine, mettre `DB_PORT=5433` dans le
`.env` et adapter le port dans `DATABASE_URL`.

## Modifier l'interface

Les pages sont des composants [templ](https://templ.guide) dans
`backend/internal/web/views`, le style est dans
`backend/internal/web/static/app.css`. Après avoir touché un `.templ` :

```bash
cd backend
go generate ./internal/web/views
```

Les fichiers `*_templ.go` générés sont commités, l'image Docker n'a donc pas
besoin de templ.

## Tests

```bash
cd backend
go test ./... -short
```

Les tests de concurrence ont besoin d'une vraie base, sinon ils sont skip :

```bash
export TEST_DATABASE_URL="postgres://casino:MOTDEPASSE@localhost:5432/casino?sslmode=disable"
go test ./internal/wallet -v
```

Ils couvrent 50 mises simultanées sur le même compte, le découvert impossible,
30 joueurs en parallèle et le double envoi d'une même requête.

Pour mesurer les taux de retour des jeux :

```bash
go test ./internal/games/... -run TestRTP -v
```

## Variables

| | |
|---|---|
| `DATABASE_URL` | obligatoire |
| `APP_ADDR` | `:8080` |
| `START_BALANCE` | 1000 |
| `MIN_BET` / `MAX_BET` | 1 / 500 |
| `SESSION_TTL` | 24h |
| `COOKIE_SECURE` | true, à passer à false en http |
| `TRUSTED_PROXIES` | réseaux des relais devant l'API |
| `ALLOWED_ORIGINS` | vide si le front est servi par le même nginx |
| `RATE_GLOBAL_PER_MIN` | 300 |
| `RATE_AUTH_PER_MIN` | 10 |
| `RATE_PLAY_PER_MIN` | 120 |

## Ajouter un jeu

Une machine à sous, c'est une `Definition` dans
`backend/internal/games/slots/machines.go` : les rouleaux, les symboles avec
leurs wilds et scatters, les lignes de paiement, la table de gains et le thème.
Rien d'autre à toucher, elle apparaît toute seule dans l'API et sur le site.

Après avoir changé une table de gains, relancer le test de RTP et reporter la
valeur dans `TheoreticalRTP`.

Pour un autre type de jeu, implémenter `games.Game` et l'enregistrer dans
`backend/internal/games/catalog`. Côté site, il faut en plus sa vue dans
`backend/internal/web/views` et un cas dans `gameStage`
(`backend/internal/httpapi/page_handlers.go`).

Le blackjack n'est pas fait. L'interface pour les jeux qui durent plusieurs
requêtes est dans `backend/internal/games/stateful.go`.

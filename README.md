# Casino R&T

Casino web multijoueur avec des crédits fictifs, projet SAE 5.02.

Ce dépôt contient le back-end (API Go + PostgreSQL) et le déploiement. Le front
est dans `frontend/`.

## Stack

Go 1.23 (Gin), PostgreSQL 16 (GORM), Docker Compose, nginx en frontal,
Cloudflare Tunnel pour l'accès depuis Internet.

## Lancer en local

```bash
cd deploy
cp .env.example .env
# mettre un POSTGRES_PASSWORD et COOKIE_SECURE=false pour du http
docker compose up -d --build
```

Ça écoute sur http://localhost:8088.

## Lancer sur le NAS

Pareil mais avec le tunnel. Il faut un `TUNNEL_TOKEN` dans le `.env`, et côté
Cloudflare Zero Trust le service public doit pointer sur `http://web:80`.

```bash
docker compose --profile tunnel up -d --build
```

## Dev sur le back seul

```bash
cd deploy && docker compose up -d db
cd ../backend
export DATABASE_URL="postgres://casino:MOTDEPASSE@localhost:5432/casino?sslmode=disable"
export COOKIE_SECURE=false
go run ./cmd/api
```

Les tables sont créées au démarrage.

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

Pour mesurer les taux de retour :

```bash
go test ./internal/games/... -run TestRTP -v
```

Actuellement 91.5% sur neon777, 92.9% sur cyberjungle, 97.3% à la roulette
(36/37, roulette européenne).

## Gestion des soldes

Une mise = une transaction. On verrouille la ligne du joueur avec
`SELECT ... FOR UPDATE`, on vérifie le solde, on tire le résultat, puis on
écrit le nouveau solde et la partie. Sans le verrou, deux mises qui arrivent en
même temps lisent le même solde et une des deux écritures écrase l'autre.

Le front peut envoyer un `idempotency_key` par tour pour qu'un double-clic ou
une requête renvoyée ne rejoue pas la partie.

## Ajouter un jeu

Une machine à sous, c'est juste une `Definition` dans
`internal/games/slots/machines.go` : les rouleaux, les symboles (avec les wilds
et les scatters), les lignes de paiement, la table de gains et le thème. Rien
d'autre à toucher, elle apparaît toute seule dans l'API.

Pour un autre type de jeu, implémenter `games.Game` et l'enregistrer dans
`internal/games/catalog`.

Le blackjack n'est pas fait. L'interface pour les jeux qui durent plusieurs
requêtes est dans `internal/games/stateful.go`.

## API

Voir [docs/api.md](docs/api.md).

## Arborescence

```
backend/
  cmd/api/            main
  internal/auth/      argon2id, sessions
  internal/games/     registre + jeux (slots, roulette)
  internal/wallet/    transactions et journal des parties
  internal/httpapi/   routes
deploy/               compose, nginx, .env.example
frontend/             interface
docs/api.md           contrat d'API
```

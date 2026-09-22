# API

Base `/api`. nginx sert le front et l'API sur la même origine, donc appels en
relatif et pas de CORS à gérer.

La session est un cookie `HttpOnly`, donc il faut `credentials: "same-origin"`
dans les `fetch`, sinon rien ne marche.

Les montants sont des entiers (crédits fictifs).

## Erreurs

```json
{ "error": { "code": "insufficient_funds", "message": "solde insuffisant pour cette mise" } }
```

Se baser sur `code`, le `message` peut changer.

| code | http |
|---|---|
| `invalid_body` | 400 |
| `invalid_username` | 400 |
| `invalid_password` | 400 |
| `invalid_bet` | 400 |
| `invalid_metric` | 400 |
| `invalid_credentials` | 401 |
| `unauthenticated` | 401 |
| `session_expired` | 401 |
| `insufficient_funds` | 402 |
| `unknown_game` | 404 |
| `username_taken` | 409 |
| `rate_limited` | 429 |
| `internal` | 500 |

## Comptes

### POST /api/auth/register

```json
{ "username": "idrys", "password": "motdepasse123" }
```

Renvoie 201, pose le cookie et crédite 1000. Pseudo 3-32 caractères
(lettres, chiffres, `-`, `_`), mot de passe 8-128.

```json
{
  "user": { "id": "...", "username": "idrys", "balance": 1000, "created_at": "..." },
  "expires_at": "..."
}
```

### POST /api/auth/login

Même corps, même réponse en 200. En cas d'échec c'est toujours
`invalid_credentials`, qu'on se trompe de pseudo ou de mot de passe.

### POST /api/auth/logout

Supprime la session côté serveur.

### GET /api/me

Session requise. Renvoie le user avec le solde relu en base.

## Jeux

### GET /api/games

Liste tout le catalogue. `config` contient de quoi construire l'interface du
jeu, donc pas besoin de coder quoi que ce soit en dur côté front : une machine
ajoutée côté back apparaît toute seule.

```json
{ "games": [
  { "key": "neon777", "name": "Neon 777", "description": "...",
    "min_bet": 1, "max_bet": 200, "config": { ... } }
] }
```

### GET /api/games/:key

Une seule fiche.

### config des machines à sous

`type: "slots"`.

- `rows`, `reel_count` : taille de la grille
- `reels` : la bande de chaque rouleau, utile pour l'animation
- `paylines` : `paylines[j][i]` = la ligne lue sur le rouleau `i`
- `symbols` : `id`, `name`, `emoji`, `color`, `wild`, `scatter`
- `paytable[symbole][nb aligné]` en pour-mille de la mise (2500 = 2.5x)
- `scatter_pays[nb]`, pareil en pour-mille
- `wild_multiplier` : multiplie la ligne si un wild a remplacé un symbole
- `theme` : couleurs, type d'animation, vitesse des rouleaux
- `rtp`, `volatility` : info affichable

### config de la roulette

`type: "roulette"`. 37 cases, un seul zéro.

`bet_options` liste les mises possibles avec leur `multiplier`, qui est le
total rendu mise comprise : 2 pour rouge/noir, 3 pour une douzaine, 36 pour un
plein. `colors` donne la couleur de chaque numéro.

## Jouer

### POST /api/games/:key/play

Session requise.

```json
{ "amount": 50, "option": "red", "idempotency_key": "a3f9c1e2-..." }
```

`option` vaut `"spin"` (ou rien) pour une machine à sous, et pour la roulette
un id de `bet_options`, avec le numéro pour un plein : `"number:17"`.

`idempotency_key` est optionnel mais conseillé, un `crypto.randomUUID()` par
tour suffit. Si la requête part deux fois, la partie n'est jouée qu'une fois.

```json
{
  "round": {
    "id": "...",
    "game": "roulette",
    "bet_amount": 50,
    "bet_option": "red",
    "payout": 100,
    "net": 50,
    "balance_after": 1050,
    "label": "18 rouge - gagne",
    "detail": { "result": 18, "color": "red", "won": true, "multiplier": 2 },
    "created_at": "..."
  },
  "balance": 1050,
  "replayed": false
}
```

`payout` c'est ce qui revient au joueur, mise comprise, donc 0 si perdu.
`net` = `payout - bet_amount`. `replayed: true` veut dire que la requête avait
déjà été traitée : rejouer l'animation avec `detail` sans redébiter.

`detail` pour une machine à sous :

```json
{
  "grid": [["cherry","lemon","bell","bar","seven"], ...],
  "stops": [3, 17, 8, 12, 1],
  "line_wins": [
    { "line": 0, "symbol": "cherry", "count": 3, "with_wild": true,
      "payout": 50, "positions": [[1,0],[1,1],[1,2]] }
  ],
  "scatter_count": 0,
  "scatter_win": 0,
  "total_payout": 50
}
```

`grid[ligne][rouleau]`, et `positions` ce sont des paires `[ligne, rouleau]` à
mettre en surbrillance. `stops` permet d'arrêter les rouleaux au bon endroit
pendant l'animation.

## Historique et stats

### GET /api/me/history?limit=25&offset=0&game=roulette

Session requise. Du plus récent au plus ancien. `limit` max 100, `game`
optionnel.

```json
{ "rounds": [ ... ], "total": 132, "limit": 25, "offset": 0 }
```

### GET /api/me/stats

Session requise.

```json
{
  "balance": 1050,
  "total": { "rounds": 132, "wagered": 6600, "won": 6150, "net": -450,
             "win_rate": 0.31, "return_rate": 0.93 },
  "per_game": [
    { "game": "neon777", "rounds": 80, "wagered": 4000, "won": 3680,
      "net": -320, "win_rate": 0.33, "best_win": 900, "worst_net": -50 }
  ]
}
```

`return_rate` c'est le retour réel du joueur, il se rapproche du `rtp` du jeu
au fil des parties.

### GET /api/leaderboard?metric=balance&limit=10

Public. `metric` : `balance`, `net`, `rounds` ou `biggest_win`.

```json
{ "metric": "net", "leaderboard": [
  { "rank": 1, "username": "idrys", "balance": 1840, "rounds": 132,
    "net": 840, "biggest_win": 2500 }
] }
```

Un joueur qui n'a jamais misé apparaît avec des compteurs à zéro.

## Divers

`GET /api/config` renvoie `min_bet`, `max_bet` et `start_balance`, à lire au
démarrage du front plutôt que de les recopier.

`GET /healthz` et `GET /readyz` pour les healthchecks Docker.

## Limites de débit

10 req/min par IP sur login et register, 120 mises/min par joueur, 300 req/min
par IP sur le reste. Au-delà c'est un 429.

## Exemple

```js
async function api(path, options = {}) {
  const res = await fetch(`/api${path}`, {
    ...options,
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", ...(options.headers || {}) },
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw Object.assign(new Error(data?.error?.message || "erreur"), {
      code: data?.error?.code,
      status: res.status,
    });
  }
  return data;
}

await api("/auth/login", {
  method: "POST",
  body: JSON.stringify({ username, password }),
});

const { round, balance } = await api("/games/roulette/play", {
  method: "POST",
  body: JSON.stringify({ amount: 50, option: "red", idempotency_key: crypto.randomUUID() }),
});
```

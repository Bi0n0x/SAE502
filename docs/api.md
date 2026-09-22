# Contrat d'API — Casino R&T

Document de reference pour l'interface. Toute modification de ce contrat est
annoncee au groupe avant d'etre deployee.

Base : `/api` — servie par nginx sur la meme origine que l'interface, donc
appels en chemin relatif (`fetch("/api/me")`), sans CORS a gerer.

## Regles generales

- Les requetes et reponses sont en JSON (`Content-Type: application/json`).
- **Toujours envoyer `credentials: "same-origin"`** dans `fetch` : la session
  est portee par un cookie `HttpOnly`, inaccessible au JavaScript.
- Les erreurs ont toujours cette forme :

  ```json
  { "error": { "code": "insufficient_funds", "message": "solde insuffisant pour cette mise" } }
  ```

  Reagir au `code`, pas au `message` (qui peut evoluer).
- Les montants sont des entiers : des credits fictifs, jamais de centimes ni
  de decimales.

### Codes d'erreur

| Code | HTTP | Sens |
|---|---|---|
| `invalid_body` | 400 | JSON illisible ou champ hors bornes |
| `invalid_username` | 400 | 3 a 32 caracteres, lettres/chiffres/`-`/`_` |
| `invalid_password` | 400 | 8 a 128 caracteres |
| `invalid_bet` | 400 | mise nulle, negative, hors bornes, ou option inconnue |
| `unknown_game` | 404 | cle de jeu inconnue |
| `invalid_credentials` | 401 | pseudo ou mot de passe incorrect |
| `unauthenticated` | 401 | pas de session |
| `session_expired` | 401 | session expiree : rediriger vers la connexion |
| `insufficient_funds` | 402 | solde insuffisant |
| `username_taken` | 409 | pseudo deja pris |
| `rate_limited` | 429 | trop de requetes : afficher un message et ralentir |
| `internal` | 500 | erreur serveur |

---

## Comptes

### `POST /api/auth/register`

```json
{ "username": "idrys", "password": "motdepasse123" }
```

`201` — le cookie de session est pose, le compte est credite du solde de
depart (1 000 credits).

```json
{
  "user": { "id": "…", "username": "idrys", "balance": 1000, "created_at": "2026-01-12T10:22:00Z" },
  "expires_at": "2026-01-13T10:22:00Z"
}
```

Limite : 10 requetes par minute et par adresse IP.

### `POST /api/auth/login`

Meme corps, meme reponse en `200`. En cas d'echec, `401 invalid_credentials` —
le meme message que le compte existe ou non, volontairement.

### `POST /api/auth/logout`

`200`. La session est supprimee en base : le cookie devient inutilisable
immediatement.

### `GET /api/me` *(session requise)*

```json
{ "user": { "id": "…", "username": "idrys", "balance": 940, "created_at": "…" } }
```

Le solde est relu en base a chaque appel.

---

## Catalogue des jeux

### `GET /api/games` *(public)*

```json
{ "games": [ { "key": "neon777", "name": "Neon 777", "description": "…",
               "min_bet": 1, "max_bet": 200, "config": { … } } ] }
```

`config` contient tout ce qu'il faut pour construire l'interface du jeu :
**l'interface n'a rien a coder en dur pour un jeu donne.** Ajouter une machine
a sous cote back-end la fait apparaitre toute seule.

### `GET /api/games/:key` *(public)*

La fiche d'un seul jeu.

### Contenu de `config` pour une machine a sous (`type: "slots"`)

| Champ | Sens |
|---|---|
| `rows`, `reel_count` | taille de la grille |
| `reels` | bande de chaque rouleau (utile pour l'animation de defilement) |
| `paylines` | `paylines[j][i]` = ligne lue sur le rouleau `i` par la ligne `j` |
| `symbols` | `id`, `name`, `emoji`, `color`, `wild`, `scatter` |
| `paytable` | `paytable[symbole][nombre aligne]` en **pour-mille de la mise** (2500 = 2,5 fois la mise) |
| `scatter_pays` | gains du scatter selon le nombre sur la grille |
| `wild_multiplier` | multiplicateur applique a une ligne ou un wild a remplace un symbole |
| `theme` | `palette`, `animation`, `reel_speed`, `sound` — propre a chaque machine |
| `rtp`, `volatility` | taux de retour mesure, a titre informatif |

### Contenu de `config` pour la roulette (`type: "roulette"`)

| Champ | Sens |
|---|---|
| `pockets` | 37 (roulette europeenne, un seul zero) |
| `bet_options` | liste de `{id, label, description, multiplier, needs_number}` |
| `colors` | couleur de chaque numero, pour dessiner la roue et le tapis |

`multiplier` est le **total rendu** en cas de gain, mise comprise : 2 pour
rouge/noir, 3 pour une douzaine, 36 pour un plein.

---

## Jouer

### `POST /api/games/:key/play` *(session requise)*

```json
{
  "amount": 50,
  "option": "red",
  "idempotency_key": "a3f9c1e2-…"
}
```

- `option` : `"spin"` (ou vide) pour une machine a sous ; un `id` de
  `bet_options` pour la roulette, avec le numero pour un plein
  (`"number:17"`).
- `idempotency_key` : **facultatif mais recommande.** Tirer un identifiant
  aleatoire par tour (`crypto.randomUUID()`) et le renvoyer tel quel si la
  requete est rejouee. Le serveur garantit alors qu'une seule partie est jouee,
  meme sur double-clic ou coupure reseau.

Reponse `200` :

```json
{
  "round": {
    "id": "…",
    "game": "roulette",
    "bet_amount": 50,
    "bet_option": "red",
    "payout": 100,
    "net": 50,
    "balance_after": 1050,
    "label": "18 rouge - gagne",
    "detail": { "result": 18, "color": "red", "won": true, "multiplier": 2 },
    "created_at": "2026-01-12T10:25:11Z"
  },
  "balance": 1050,
  "replayed": false
}
```

- `payout` = credits rendus, **mise comprise** : 0 si perdu.
- `net` = `payout - bet_amount`, negatif en cas de perte.
- `balance` = nouveau solde, a afficher directement.
- `replayed: true` signale une requete deja traitee : rejouer l'animation a
  partir de `detail`, sans redebiter.

`detail` pour une machine a sous :

```json
{
  "grid": [["cherry","lemon","bell","bar","seven"], …],
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

`grid[ligne][rouleau]`, et `positions` liste des paires `[ligne, rouleau]` a
mettre en valeur. `stops` permet de faire tomber les rouleaux exactement au bon
endroit pendant l'animation.

Limite : 120 mises par minute et par joueur.

---

## Historique, statistiques, classement

### `GET /api/me/history?limit=25&offset=0&game=roulette` *(session requise)*

```json
{ "rounds": [ { … meme forme que round ci-dessus … } ],
  "total": 132, "limit": 25, "offset": 0 }
```

Trie de la partie la plus recente a la plus ancienne. `limit` est borne a 100.
`game` est facultatif.

### `GET /api/me/stats` *(session requise)*

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

`return_rate` est le retour effectif du joueur : il converge vers le `rtp`
annonce par le jeu au fil des parties, ce qui fait une bonne courbe pour la
soutenance.

### `GET /api/leaderboard?metric=balance&limit=10` *(public)*

`metric` : `balance` (defaut), `net`, `rounds`, `biggest_win`.

```json
{ "metric": "net",
  "leaderboard": [
    { "rank": 1, "username": "idrys", "balance": 1840, "rounds": 132,
      "net": 840, "biggest_win": 2500 }
  ] }
```

Un joueur inscrit qui n'a encore rien mise apparait avec des compteurs a zero.

---

## Divers

### `GET /api/config` *(public)*

```json
{ "min_bet": 1, "max_bet": 500, "start_balance": 1000, "currency": "credits fictifs" }
```

A lire au demarrage de l'interface pour ne pas recopier ces bornes dans le
code du front.

### `GET /healthz` et `GET /readyz`

Sondes techniques (healthcheck Docker). `readyz` verifie aussi la base.

---

## Exemple minimal cote interface

```js
async function api(path, options = {}) {
  const res = await fetch(`/api${path}`, {
    ...options,
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", ...(options.headers || {}) },
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw Object.assign(new Error(data?.error?.message || "erreur"), {
    code: data?.error?.code, status: res.status,
  });
  return data;
}

await api("/auth/login", { method: "POST", body: JSON.stringify({ username, password }) });

const { round, balance } = await api("/games/roulette/play", {
  method: "POST",
  body: JSON.stringify({ amount: 50, option: "red", idempotency_key: crypto.randomUUID() }),
});
```

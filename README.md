# Casino R&T — back-end, base de donnees et hebergement

SAE 5.02 — groupe Tello / Idrys / Loic.
Ce dossier couvre la partie **Idrys** : API Go, PostgreSQL, conteneurs,
deploiement sur le NAS et acces HTTPS.

L'interface (dossier `frontend/`) est la partie de Loic ; elle est servie par
le meme nginx que l'API, et consomme le contrat decrit dans
[docs/api.md](docs/api.md).

---

## 1. Architecture

```
Navigateur
    |  HTTPS (certificat Cloudflare)
    v
Cloudflare  ──tunnel sortant──>  cloudflared (conteneur)
                                      |
                                      v
                                 nginx (web)          sert frontend/ + relaie /api
                                      |
                                      v
                                  API Go (api)        logique des jeux, comptes, mises
                                      |
                                      v
                                PostgreSQL (db)       comptes, sessions, parties
```

Points a retenir :

- **aucun port n'est ouvert sur la box.** cloudflared etablit une connexion
  sortante vers Cloudflare ; l'administration du NAS n'est jamais exposee ;
- **PostgreSQL n'est joignable que par l'API**, sur un reseau Docker separe, et
  publie seulement sur `127.0.0.1` du NAS pour les tests ;
- **l'API ne sert pas de fichiers**, nginx s'en charge : interface et API
  partagent la meme origine, donc pas de CORS et un cookie de session en
  `SameSite=Lax`.

## 2. Choix techniques et justification

| Sujet | Choix | Pourquoi |
|---|---|---|
| Mots de passe | Argon2id (19 Mio, 2 passes) | Recommandation OWASP actuelle. Couteux en memoire, donc bien plus resistant qu'un bcrypt a une attaque par GPU. |
| Sessions | Jeton opaque de 256 bits, SHA-256 en base, cookie `HttpOnly` + `Secure` + `SameSite=Lax` | La deconnexion est immediate (la ligne est supprimee). Un JWT resterait valable jusqu'a expiration et devrait etre stocke la ou du JavaScript injecte pourrait le lire. |
| Soldes | `int64`, jamais de flottant | Un arrondi flottant creerait ou detruirait des credits et rendrait le test de coherence impossible a passer. |
| Concurrence | Transaction + `SELECT ... FOR UPDATE` sur la ligne du joueur | Repond directement au risque "incoherence de solde en cas de mises simultanees" du cahier des charges. Deux joueurs differents ne s'attendent jamais. |
| Aleatoire | `crypto/rand` | Resultats non rejouables meme en connaissant tout l'historique. `math/rand` n'est utilise que dans les tests. |
| Jeux | Registre + interface `Game` | Ajouter un jeu ne touche ni a l'API, ni au portefeuille, ni a la base. Une machine a sous se decrit entierement en donnees. |

### Comment le solde reste juste quand plusieurs mises arrivent en meme temps

Tout passe par `internal/wallet/wallet.go`, seul endroit autorise a modifier un
solde. Une mise se deroule ainsi, dans **une seule transaction** :

1. `SELECT ... FOR UPDATE` sur la ligne du joueur : une deuxieme mise sur le
   meme compte attend son tour au lieu de lire un solde perime ;
2. verification du solde, maintenant qu'il ne peut plus bouger ;
3. tirage du resultat ;
4. mise a jour relative du solde (`balance - mise + gain`) avec la condition
   `balance >= mise` : seconde barriere contre un solde negatif ;
5. ecriture de la partie dans `rounds`, dans la meme transaction, pour que
   l'historique ne puisse jamais diverger du solde.

Sans l'etape 1, deux requetes liraient le meme solde et la derniere ecriture
ecraserait la premiere : c'est exactement ce que verifie
`TestConcurrentBetsNoLostUpdate`.

Une cle d'idempotence facultative (`idempotency_key`) evite en plus qu'un
double-clic ou une retransmission reseau ne joue la partie deux fois.

## 3. Lancement

### 3.1 En ligne, sur le NAS

```bash
cd deploy
cp .env.example .env
# renseigner POSTGRES_PASSWORD et TUNNEL_TOKEN
docker compose --profile tunnel up -d --build
docker compose ps
docker compose logs -f api
```

Cote Cloudflare Zero Trust, le service public du tunnel doit pointer vers
`http://web:80`.

### 3.2 En local (solution de secours pour la soutenance)

Aucun tunnel, aucun nom de domaine, tout tourne sur le poste :

```bash
cd deploy
cp .env.example .env
# mettre COOKIE_SECURE=false, le navigateur refuse un cookie Secure en http://
docker compose up -d --build
```

Le casino repond alors sur <http://localhost:8088>.

### 3.3 Back-end seul, pour developper

```bash
cd deploy && docker compose up -d db
cd ../backend
export DATABASE_URL="postgres://casino:<mot de passe>@localhost:5432/casino?sslmode=disable"
export COOKIE_SECURE=false
go run ./cmd/api
```

L'API ecoute sur <http://localhost:8080>. Le schema est cree automatiquement au
demarrage.

## 4. Tests

```bash
cd backend

# Regles des jeux, hachage des mots de passe : aucune base necessaire.
go test ./... -short

# Taux de retour des jeux par simulation (quelques minutes).
go test ./internal/games/... -run TestRTP -v

# Concurrence et charge : une base est necessaire.
cd ../deploy && docker compose up -d db && cd ../backend
export TEST_DATABASE_URL="postgres://casino:<mot de passe>@localhost:5432/casino?sslmode=disable"
go test ./internal/wallet -v
```

Correspondance avec le point 8 du cahier des charges :

| Test prevu | Ou il est couvert |
|---|---|
| Mise normale, solde correctement modifie | `TestConcurrentWinsAndLosses` |
| Mise superieure au solde, valeur invalide | `TestRejectsInvalidBets` |
| Suite de victoires et defaites, solde attendu vs enregistre | `TestConcurrentWinsAndLosses` |
| 50 mises simultanees, sans mise perdue ni double debit | `TestConcurrentBetsNoLostUpdate`, `TestConcurrentBetsCannotOverdraw` |
| 30 joueurs en parallele | `TestLoadThirtyPlayers` |
| Double envoi de la meme requete | `TestIdempotencyKeyPreventsDoubleBet` |
| Deconnexion / reconnexion, donnees conservees | a faire manuellement via l'interface |
| Acces par le nom de domaine en HTTPS depuis un autre reseau | a faire manuellement |
| Administration du NAS injoignable publiquement | a faire manuellement |

Les taux de retour des machines a sous ont ete regles par simulation :
**91,48 %** pour Neon 777, **92,92 %** pour Cyber Jungle. La roulette
europeenne donne 36/37 = **97,30 %** sur tous les types de mise.

## 5. Ajouter un jeu

Le registre (`internal/games`) decouvre les jeux au demarrage ; l'API, le
portefeuille et la base n'ont rien a savoir d'eux.

**Une machine a sous** : copier une `Definition` dans
`internal/games/slots/machines.go`, changer les symboles, la bande des
rouleaux, les lignes de paiement, la table de gains et le theme, puis
l'ajouter a `BuiltIn()`. Aucun code a ecrire. Relancer ensuite
`go test ./internal/games/slots -run TestRTP -v` pour mesurer le nouveau taux
de retour et reporter la valeur dans `TheoreticalRTP`.

**Un jeu d'un autre genre** : creer un paquet qui implemente
`games.Game` (`Key`, `Name`, `Description`, `Config`, `ValidateBet`, `Play`),
et l'enregistrer dans `internal/games/catalog/catalog.go`.

**Un jeu en plusieurs coups** (blackjack) : le point d'extension est decrit
dans `internal/games/stateful.go`. Il n'est volontairement pas implemente :
le cahier des charges demande d'abord deux jeux termines.

## 6. Organisation du code

```
backend/
  cmd/api/                 point d'entree, arret propre
  internal/config/         lecture de la configuration
  internal/models/         schema PostgreSQL (GORM)
  internal/db/             connexion, attente de la base, migration
  internal/auth/           Argon2id, sessions
  internal/rng/            aleatoire cryptographique + variante deterministe pour les tests
  internal/games/          registre et contrat commun
    slots/                 moteur generique de machine a sous + catalogue
    roulette/              roulette europeenne
    catalog/               enregistrement des jeux
  internal/wallet/         transactions, verrouillage, journal des parties
  internal/httpapi/        routeur Gin, middlewares, handlers
deploy/                    docker-compose, nginx, exemple de configuration
docs/api.md                contrat d'API pour l'interface
frontend/                  interface (partie de Loic)
```

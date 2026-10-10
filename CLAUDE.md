# CLAUDE.md — OCC DELIVERIES

Plateforme de **commandes groupées** entre collègues : on ouvre une commande
(« party »), les collègues rejoignent, votent pour un restaurant, chacun compose
son panier, on envoie vers Uber Eats / Takeaway (ou on exporte), on désigne le
payeur et chacun rembourse sa part (QR virement SEPA lu par l'app bancaire, liens Revolut / PayPal.me avec montant, espèces, plus tard).

Production : `https://eat.fs0ciety.org` (Coolify, Docker).

## Documents de référence — à lire avant de coder
| sujet | fichier |
|---|---|
| Modèle de données, state machine, API `/api/occ` | `docs/ARCHITECTURE.md` (**contrat**) |
| Design system « Ember » (tokens, composants, ton) | `docs/DESIGN_SYSTEM.md` |
| Workflow, branches, Definition of Done, agents | `docs/WORKFLOW.md` |
| Déploiement Coolify | `docs/DEPLOYMENT.md` |
| Décisions (ADR) | `docs/adr/` |
| Feuille de route | `docs/ROADMAP.md` |

## Stack
* **backend/** — Go 1.27.2 (`toolchain` dans `go.mod`), PocketBase **v0.40.5** utilisé comme framework
  (SQLite WAL, auth, realtime SSE, admin `/_/`). Migrations Go dans `backend/migrations`.
* **frontend/** — React 19, Vite, TypeScript strict, Tailwind CSS v4, TanStack
  Query, React Router, `pocketbase` JS SDK, `motion`, `lucide-react`, `sonner`,
  `qrcode.react`. Tests : Vitest + Testing Library. Node **24 LTS** (`.nvmrc`, `engines`).
* **Déploiement** — un seul conteneur (Dockerfile multi-étapes) : le binaire Go
  sert l'API **et** la SPA (`pb_public`). Volume persistant `/pb/pb_data`.
  Images : `node:24-alpine` → `golang:1.27-alpine` → `alpine:3.24`.

## Commandes
```bash
# Backend
cd backend && go run . serve --http=127.0.0.1:8090   # API + admin sur :8090
cd backend && go test ./...                           # tests
cd backend && go vet ./... && gofmt -l .              # lint

# Frontend
cd frontend && npm ci
cd frontend && npm run dev        # Vite :5173, proxy /api et /_ → :8090
cd frontend && npm run typecheck && npm run lint && npm test && npm run build

# Tout-en-un
make dev | make test | make build | make docker
docker compose up --build         # http://localhost:8090
```

## Règles du projet (non négociables)
1. **Argent en centimes `int`** partout (Go, JSON, TS). Formatage uniquement à l'affichage.
2. **Le serveur fait foi** : prix, totaux, statuts, codes et parts sont calculés
   côté Go. Le front n'envoie que des intentions (`menu_item`, options, quantité…).
3. Toute écriture sensible passe par une **rule PocketBase** *et* une validation
   dans un hook/route. Jamais de rule `""` en écriture.
4. La logique métier pure vit dans `backend/internal/domain` (zéro import
   PocketBase) et est **testée** (tables de tests).
5. Le contrat `docs/ARCHITECTURE.md` est mis à jour **dans le même commit** que
   tout changement de schéma ou d'endpoint ; les types TS (`frontend/src/lib/types.ts`)
   aussi.
6. Front : uniquement les tokens du design system (pas de hex en dur), textes en
   français, mobile d'abord, accessibilité AA, `prefers-reduced-motion`.
7. Pas de secrets dans le repo. Config via variables `OCC_*` (voir ARCHITECTURE §7).
8. Commits conventionnels (`feat:`, `fix:`, `docs:`, `chore:`, `refactor:`, `test:`).
9. Dépendances : toujours les dernières versions stables (LTS pour Node), épinglées
   exactement ; exceptions documentées dans le journal.

## Agents du projet (`.claude/agents/`)
* `backend-engineer` — Go/PocketBase, migrations, hooks, routes, tests.
* `frontend-engineer` — React/Tailwind, écrans, realtime, design system.
* `product-designer` — revue UX/UI contre le design system, accessibilité.
* `qa-engineer` — scénarios end-to-end, revue de régressions, tests.
* `devops-engineer` — Docker, CI, Coolify, sauvegardes.

## Journal de bord
Les apprentissages importants (pièges PocketBase, décisions d'UX) sont ajoutés
ci-dessous, du plus récent au plus ancien.

* 2026-10-10 — **Retrait de Wero et Bancontact Pay** (demande utilisateur « pour éviter la confusion » ; ADR 0003 maj 3,
  migration `1760000021`). Aucun des deux ne permet à un tiers de pré-remplir un montant ; le QR virement EPC (lu par les
  apps bancaires belges, celles où vit Wero) et les liens Revolut / PayPal.me à montant couvrent le besoin.
  `payout_profiles.wero_id|bancontact_phone|wero_qr|bancontact_qr` **supprimés** et fichiers QR effacés (lister les noms
  **avant** le `Save` de la collection : PocketBase n'efface pas les fichiers d'un champ retiré) ; `/wallet-qr/{kind}`
  supprimé ; `declare` avec `wero`/`bancontact` → 400 FR. `payments.method` garde les deux valeurs (anciens paiements
  affichés, notifications inchangées). Tuile « Virement (QR) ». Montée testée depuis un `pb_data` de prod (8045736).
* 2026-10-10 — **Passkeys / WebAuthn** (migration `1760000018`, `app/passkeys.go`, `domain/passkey.go`, `lib/webauthn.ts`,
  `lib/passkeys.ts`). PocketBase v0.40.5 n'a pas de passkeys natives → `go-webauthn/webauthn` v0.18.2 derrière
  `/api/occ/passkeys*`, collection `passkeys` sans aucune rule, connexion via `apis.RecordAuthResponse(e, u, "passkey", nil)`
  (hooks ban/MFA conservés). Défis en mémoire, usage unique, 10 min ; RP ID = hôte d'`OCC_PUBLIC_URL` (prod
  `eat.fs0ciety.org` ; le changer casse toutes les passkeys). Pièges : `RecordAuthResponse` relit le corps → lire avec
  `e.BindBody` (un decoder brut le consomme → 400 générique) ; options encodées en `encoding/json` v1 ; garder BE/BS/UV en
  base ; `CloneWarning` n'échoue pas seul → refuser un compteur qui recule (0/0 = passkeys synchronisées) ; demander
  `credProps`. Front : options *begin* préchargées pour appeler `navigator.credentials.*` dans le clic (Safari),
  autoremplissage `autocomplete="username webauthn"` + `mediation: 'conditional'` interrompu avant la modale. E2E : le bouton
  « Se connecter avec une passkey » rend `name: 'Se connecter'` ambigu → `exact: true`.
* 2026-10-10 — **Vote par classement** (ADR 0005, migration `1760000019`, `domain/vote.go`, `app/routes_vote.go`). Borda
  tronqué : rang r → `max(1, K − r + 1)` points (K = candidats actifs) ; égalité → 1ers choix, votants, note, nom ;
  `domain.ComputeTally` unique pour l'hôte, le planificateur et `GET /tally`. Bulletin écrit **uniquement** par
  `PUT /parties/{id}/ballot` (rules d'écriture `votes` à `nil`), réécrit entier en transaction (l'index unique
  `(party, user, rank)` interdit d'échanger deux rangs ligne à ligne). Anciens votes rangés par ordre de création
  (vérifié par simulation de mise à jour depuis la prod). File hors ligne : action `ballot` (dernier état par party).
  Piège : relecture temps réel entre deux gestes rapides → bulletin local en attente prioritaire tant que
  `isMutating(['ballot', id]) > 1`.
* 2026-10-10 — **E-mail « bon de commande »** (`app/ordermail.go`, migration `1760000020`). À `review → paying` (ou `closed`
  si seul le payeur a commandé), chaque membre ayant commandé reçoit le bon complet, son bloc surligné et le montant dû au
  payeur. Hook après commit + goroutine ; idempotence par `parties.order_mail_sent_at` (caché) réclamé par
  `UPDATE … WHERE order_mail_sent_at = ''` avant l'envoi ; rien sans SMTP. Opt-out `notify_prefs.emails`. Pièges :
  `html/template` échappe `+` en `&#43;` ; jetons de palette remplacés avant `Parse`, jamais interpolés dans `style`.
* 2026-10-10 — **« Encaisser » côté payeur** (`GET /api/occ/parties/{id}/payments/qr`, payeur seul, `CollectPanel.tsx`,
  ADR 0003 mise à jour 2) : QR EPC / liens à montant par collègue, mode « Présenter » plein écran (Wake Lock, `#root`
  inerte, focus piégé). Recherche 2026 : aucun format P2P à montant générable par un tiers pour Wero, Bancontact Pay,
  Lydia ; Tikkie exige un compte néerlandais ; SRTP en adoption précoce. `TestPushPartyEvents` stabilisé (centime de
  livraison attribué selon l'ordre des ids → montant lu côté serveur).

* 2026-10-09 — **Revue design complète** (product-designer, 108 captures 390/1280 × clair/sombre, 28 points corrigés ;
  patterns dans `DESIGN_SYSTEM.md` « Patterns — passe 2 »). Pièges : une grille mobile sans `grid-cols-1` explicite
  laisse la piste implicite s'élargir au contenu (accueil à 480 px) ; un `<table className="sr-only">` ne rétrécit pas
  sous son contenu → `sr-only` sur un `div` parent ; `main` en flex colonne rétrécit les pages `mx-auto max-w-…` → contenu
  dans un bloc, footer `mt-auto`. Statut `ordering` = « Paniers ouverts » (≠ « commande en cours »), couleurs d'avatar
  rendues distinctes par groupe (`distinctColors`, couleurs stockées tirées parmi 12 à l'inscription), un seul `primary`
  par moyen de paiement. Contrôle de débordement : `document.documentElement.scrollWidth` sur 375/390/1024/1280.

* 2026-10-09 — **Notifications push, heures limites automatiques, hors ligne** (migration `1760000015`, `internal/notify`,
  `app/push.go`, `app/deadlines.go`, `frontend/pwa/`). Web Push VAPID (`webpush-go` v1.4.0) : clés `OCC_VAPID_*` ou
  générées une fois dans `server_secrets` (superuser) ; envoi asynchrone (4 workers), 404/410 → abonnement supprimé ;
  évènements par hooks `OnRecordAfter*Success` (exécutés **après commit**), l'auteur exclu via `SaveWithContext(ctx)`
  (l'`e.Context` du hook est celui du `Save`) ; `applyTransition` partagé entre l'hôte et le planificateur (cron chaque
  minute, drapeaux indexés par l'heure limite dans `parties.auto_state` caché → idempotent). Toasts in-app par un
  **sujet temps réel personnalisé** `occ/notifications` (`SubscriptionsBroker`, filtre sur le client authentifié).
  Pièges : `Record.Original()` n'est pas rafraîchi après `Save` (deux saves dans une transaction → deux hooks avec le
  même original : dédoublonner) ; `/index.html` est redirigé (301) par `apis.Static` → précacher `/` ; un
  *stale-while-revalidate* sur les données d'une party renvoie la liste **d'avant** l'écriture à l'invalidation
  TanStack (panier « vide » après ajout, e2e cassés) → réseau d'abord pour les données vivantes, SWR pour le catalogue ;
  `Notification.requestPermission()` dans le clic (pas dans `useMutation`). Doublon de vote = 200 idempotent (contrat).

* 2026-10-09 — **Salons d'équipe & invités sans compte** (migration `1760000016`, `app/teams.go`, `app/guests.go`,
  `features/teams`). `teams` (lien fixe `/e/:code` 8 car., rules `members.id ?=`, admins choisis par le propriétaire,
  archivage au lieu de suppression), `parties.team` immuable, « Lancer la commande du jour » idempotent (adresse,
  candidats encore actifs, partage de l'équipe), un membre rejoint en un geste (`POST /parties/{id}/join`),
  toast realtime via `teams.last_party` ; push = `app.SetTeamNotifier` (point d'extension, non branché).
  Invités : `POST /api/occ/guest` (code valide, 10/h/IP), `users.is_guest` **visible mais écrit par le serveur
  seul** (un champ `Hidden` aurait privé l'UI de l'info : le hook force `false` / refuse le changement),
  e-mail `guest-<id>@guest.occ.invalid`, jeton `NewStaticAuthToken(30 j)`. Pièges : un jeton statique n'est
  **pas renouvelé** par `auth-refresh` (il renvoie le même) → hook `OnRecordAuthRefreshRequest` qui émet un
  nouveau jeton statique (fenêtre glissante) ; un hook `OnRecordCreateRequest(parties)` lié **après**
  `bindHooks` s'exécute **dans** `onPartyCreate` (hôte déjà forcé, transaction ouverte) — mais celui-ci a déjà
  mis `split_mode = equal` : lire `RequestInfo().Body` pour savoir si le client l'a choisi ; OAuth2 d'un invité
  connecté = PocketBase lie le compte **connecté** (avant la recherche par e-mail) → on y pose l'e-mail Google
  et `is_guest = false` ; `OnMailerSend` jette tout envoi vers `*.invalid`. Front : `/j/:code` n'est plus sous
  `RequireAuth` (page `InviteGate`) — garder l'état « non connecté » à l'ouverture, sinon l'invité créé par le
  formulaire déclenche aussi l'auto-join (« Te revoilà » en double). E2E `guest.spec.ts` (Léa) vert sur :8102.

* 2026-10-09 — **Recherche globale** (`GET /api/occ/search`, `internal/search`, migration `1760000017`,
  palette `features/search` : Ctrl K / ⌘K / « / », icône d'en-tête, « Plats correspondants » sur Restos,
  `?plat=<id>` défile jusqu'au plat). SQLite FTS5 **disponible** dans le SQLite modernc de PocketBase (3.53,
  `fts5vocab` aussi) : tables SQL hors collections, texte **plié en Go** (accents, ligatures) à l'indexation
  et à la requête, préfixes, corrections Damerau-Levenshtein via `fts5vocab` ; visibilité (actif, cartes
  incomplètes, `available`) **jointe à la requête**, jamais indexée. Pièges : une colonne id `UNINDEXED` dans
  une table FTS est parcourue à chaque `DELETE` → tables `*_docs` (rowid ↔ id) ; hooks `OnRecordCreate/
  Update/Delete` (pas `AfterSuccess`) pour indexer **dans** la transaction de l'écriture (`e.App` = `txApp`) ;
  `Record.Original()` comparé avant `e.Next()` pour ne réindexer que si le texte change ; bm25 classe
  « pita » (rare) devant « pizza » pour « piza » → bonus à la meilleure correction (préfixe commun le plus
  long). Collègues = party ou équipe partagée, rien d'autre n'est lu. 6–20 ms pour 10 000 plats.
  Le scratchpad est **partagé** entre agents parallèles : y travailler dans un sous-dossier à son nom.

* 2026-10-09 — **Animations de party** (`components/food`, `features/party/steps`) : panier vivant (vols
  realtime vers l'avatar du collègue, total du groupe qui compte au centime), transitions d'étapes
  directionnelles + annonce `aria-live`, livreur / ingrédients d'attente, scène « Commande envoyée » et
  révélation du gagnant (une fois par party, `localStorage`, lazy), cœur liquide, coche dessinée, `press`
  tokenisé, haptique (`lib/haptics.ts`). Réglage **« Animations réduites »** (`useMotionPref`, `occ-motion`,
  `data-motion`) branché sur `withMotion`, `MotionConfig`, `motion-reduce:` et la CSS globale. Pièges :
  les petits composants de party s'importent **par leur fichier** — le barrel `components/food` est importé
  par le shell et les tire dans le bundle initial (+1,8 Ko gzip constaté) ; un mock `matchMedia` en
  `query.includes('reduce')` matche aussi `no-preference` (« reduce**d**-motion ») → tester `': reduce'` ;
  « une fois par party » = lecture dans l'initialiseur `useState`, écriture dans un effet (StrictMode).
* 2026-10-09 — **Historique & reprise de commande** : `GET /api/occ/me/history` / `me/stats`,
  `GET|POST /parties/{id}/reorder`, `join` renvoie `alreadyMember`. Cause du « salon introuvable » :
  `partiesApi.mineActive` filtrait `members ?= {:u}` — le piège relation multiple vaut aussi pour les
  `?filter=` client (aucun résultat, même pour l'hôte) → `members.id ?=`. Bandeau « Commande en cours »
  + toasts de statut globaux (abonnement `parties` `*` dans le shell, filtré par les rules), dernière party
  en localStorage. Totaux d'historique = `summaryFromRecords` (même code que `/summary`), chargés par lots.
* 2026-10-09 — **Remboursements à montant exact** (retours : « un QR Wero varie avec le montant », « les liens
  Revolut ne s'affichent pas »). Cause du 2e : le profil refusait tout lien sans `https://` (jamais enregistré),
  Revolut sans montant derrière une tuile générique non présélectionnée, `PaymentQR` en cache 5 min. Désormais
  `revolut_tag` / `paypal_me` structurés (migration `1760000013`, reprise des anciens liens), `PaymentQR.links[]`
  par paiement avec `amountPrefilled` (Revolut `?amount=<centimes>&currency=EUR&note=` lu par la page revolut.me,
  PayPal.me `/<montant>EUR`, Wise Business `?amount=`), méthodes `revolut` / `paypal`, ordre QR EPC → liens →
  Wero/Bancontact ; mobile : liens d'abord + « Afficher le QR pour un collègue ». QR perso Wero/Bancontact gardé
  mais toujours averti « sans montant ». Wero / Bancontact Pay / Lydia / Wisetag : aucun format tiers (ADR 0003 maj 1).
  Piège e2e : un autre agent peut lancer Playwright en parallèle → `--output` dédié, sinon artefacts ENOENT.
* 2026-10-09 — **Comptes : Google, e-mails, modération** (migration `1760000014`). Google OAuth2 et SMTP
  configurés au démarrage depuis `OCC_GOOGLE_CLIENT_*` / `OCC_SMTP_*` / `OCC_MAIL_*` (rien n'est touché sans
  ces variables : la config `/_/` reste) ; modèles d'e-mails français (`app/mailtemplates.go`) vers les routes
  SPA `/auth/verifier|reinitialiser|changer-email/{TOKEN}`. Pièges PocketBase v0.40 :
  - champs `Hidden` : jamais renvoyés (même au titulaire), **ignorés en écriture** pour les non-superusers ;
    lisibles en Go et dans les rules → `banned`, `password_set`… exposés par `/api/occ/me/account` ;
  - OAuth2 lie par **e-mail** : compte vérifié = simple liaison ; **non vérifié = mot de passe remplacé**
    (aléatoire) → `password_set = false` ; un compte créé par Google a un mot de passe aléatoire (le `Plain`
    est vidé avant le `Save` → détecté dans `OnRecordCreate`) ;
  - `request-password-reset` envoie l'e-mail **en arrière-plan** (tests : attendre le `TestMailer`) ;
    `request-email-change` est synchrone ;
  - suspendre = `RefreshTokenKey()` (jetons émis → 401, realtime désauthentifié) + `OnRecordAuthRequest`
    (403 « Compte suspendu ») + middleware après `pbLoadAuthToken` (priorité `DefaultLoadAuthTokenMiddlewarePriority + 6`) ;
  - `dbx.And()` vide dans `AndWhere` casse la requête (400 générique) : n'ajouter le `WHERE` que s'il y a des conditions ;
  - SDK JS : sans `urlCallback`, une pop-up bloquée laisse `authWithOAuth2` **pendant à jamais** → ouvrir la
    fenêtre nous-mêmes dans le clic (jamais dans un `useMutation`, qui l'ouvre hors geste) ; ne pas se fier à
    `popup.closed` (avec le COOP `same-origin` de PocketBase il peut passer à `true` dès la navigation vers Google) :
    bouton « Annuler » côté UI.
  Suppression de compte = anonymisation (historique et totaux conservés), `users.deleteRule = nil`.

* 2026-10-09 — **Coordonnées des restos** : téléphone E.164 / adresse « Rue X 12, 7000 Mons » normalisés
  partout (`domain/contact.go`, migration `1760000012`) ; enrichissement OpenStreetMap (`internal/enrich`,
  Nominatim : UA identifié, ≥ 1,1 s, cache 30 j y compris les absences, ≤ 60 requêtes/exécution, arrêt sur
  429/403), champs vides seulement, homonymes éloignés rejetés ; attribution ODbL via
  `restaurants.enriched_from` (pas dans `sources`, qui sert à la provenance/obsolescence). `OCC_ENRICH_ENABLED`.
  Footer « Développé par OCC MONS Studios » + logo (`public/brand/`).
* 2026-10-09 — **Cartes incomplètes masquées** (demande : « masquer les restos avec moins de 10 plats,
  mais possible de les ré-afficher »). Seuil global `app_settings.min_menu_items` (0–100, 0 = off ;
  migration `1760000010` : 10 si données réelles, 0 en démo) + `restaurants.items_count` (plats
  **disponibles**, serveur). Le filtre est évalué **à la lecture** dans `/nearby` (`domain.HiddenIncomplete`) :
  rien n'est écrit sur les restaurants, donc la synchronisation ne « défait » rien et un menu complété
  réapparaît seul. Comptage par `catalog.RefreshItemsCount` (`UPDATE … COUNT(*)` brut, sans hook) après
  les écritures d'articles via la collection, en fin de `catalog.Import` et de chaque restaurant
  synchronisé ; le hook restaurants écrase toute valeur client. Piège : un `Save` d'un restaurant chargé
  avant ses plats réécrit l'ancien `items_count` → toujours recompter **après** (fin de transaction).
  Admin : carte « Cartes incomplètes » (`/admin/restaurants`, `?filtre=incompletes`),
  `GET/PATCH /api/occ/admin/settings` ; `/api/occ/config` expose `minMenuItems`.
* 2026-10-09 — **« Découvrir un site Takeaway »** (`/admin/synchronisation`, `POST /api/occ/admin/sync/discover`,
  `POST /admin/sync/sources`, `POST /admin/sync/run {sourceId}`). Candidats purs `menusync.DiscoverHosts`
  (slug / sans tirets / `-mons` / `.com` / sans mots génériques, ≤ 16 domaines × `www.`/nu), `menusync.Prober`
  dédié (pas le `Fetcher` : 2,5 s + 1 nouvel essai sur erreur réseau = minutes pour des domaines inexistants) :
  DNS d'abord (absent = 0 requête), robots.txt, ≥ 1 s, 6 s, 403/anti-robot = hôte abandonné, redirection vers
  takeaway.com jamais suivie, variante nue sautée si `www.` a répondu. Exécution ciblée = `feedsync.Options.Scoped`
  (pas d'obsolètes, provenance complétée, menu gardé si une source liée est prioritaire). Le bac à sable n'a pas
  de DNS (proxy HTTPS seul) : smoke test réel fait avec un `Lookup` DoH injecté → snack-a-la-gare.be 196 plats
  (4 requêtes), « Tomo » → tomomons.be 67 plats (6 requêtes).
* 2026-10-09 — **Verrouillage explicite uniquement** (retour utilisateur : le formulaire admin
  verrouillait par défaut et empêchait la resynchronisation). `autoLock` supprimé, interrupteur
  initialisé sur l'état réel, migration `1760000007` qui lève tous les verrous existants.
  `catalog.Import` retrouve le restaurant par lien de plateforme (hôte + chemin, liens génériques
  ignorés, correspondance unique) puis par nom normalisé, garde le slug et les champs non fournis :
  le favori « Exporter vers OCC » complète les cartes partielles Uber Eats sans doublon.
* 2026-10-09 — **Instantané Uber Eats** (`migrations/data/mons_ubereats.json`, source `ubereats-snapshot`,
  migration `1760000006`, ADR 0002 mise à jour 3). Le connecteur MCP Uber Eats n'existe que dans les
  sessions Claude et limite fortement le débit (429 dès le 2e appel rapproché ; une reconnexion du
  connecteur a levé un 429 permanent) ; il ne donne que 0–5 plats par resto. Restos inconnus créés actifs
  avec `partial_menu` (catégorie « Aperçu », badge + bandeau vers Uber Eats), `geo_approx` si OpenStreetMap
  ne les trouve pas ; restos connus : lien + note/délai si 0, menu jamais touché ; un flux ≥ 5 plats
  remplace l'aperçu. Rafraîchir = régénérer le JSON depuis une session Claude puis déployer.
* 2026-10-09 — **Synchronisation automatique** des restaurants / menus dans le serveur
  (`internal/feedsync` + `app/sync.go`, page `/admin/synchronisation`, ADR 0002 mise à jour 2).
  - réconciliation pure (`feedsync.Reconcile`) testée sur fixtures ; écriture par restaurant en transaction ;
  - `locked` posé par les hooks catalogue sur toute modification admin (sauf `active` / `position` / `locked` seuls) ;
  - cron PocketBase en **UTC** : job chaque minute + `cron.NewSchedule(...).IsDue` à l'heure de Bruxelles
    (`time/tzdata` importé pour l'image alpine) ;
  - `NumberField{Required:true}` refusant 0, `menu_items.price` devient optionnel (min 0) ;
  - le hook restaurants refusait les liens Deliveroo / weloveat (corrigé : `providers.IsPlatform`) ;
  - tests : sources pointées sur un `httptest.Server` servant les fixtures `menusync/testdata`,
    `Fetcher.Sleep` remplacé (aucune vraie pause, aucun réseau).

* 2026-10-09 — **Panneau `/admin`** + rôle `users.role` (`user`/`admin`). Bootstrap par
  `OCC_ADMIN_EMAIL` / `OCC_ADMINS` (au démarrage et à l'inscription). Rules catalogue
  `@request.auth.role = "admin"` ; lecture des parties ouverte aux admins (`… || @request.auth.role = "admin"`).
  - la rule update de `users` reste `id = @request.auth.id` : un admin change les rôles via
    `PATCH /api/occ/admin/users/{id}/role`, jamais par la collection ;
  - import tout-ou-rien (`catalog.ImportAll` en transaction) avec rapport / `?dryRun=1` ; CSV avec décimales FR ;
  - données réelles embarquées (`go:embed data`, `migrations/data/mons_restaurants.json`) ; les tests
    d'intégration figent le catalogue sur la démo via `migrations.SetRealDataForTesting([]byte("[]"))` ;
  - E2E : restaurants / articles choisis via l'API (plus de noms codés en dur) ; attention aux
    sélecteurs `^nom` qui attrapent aussi les onglets de catégorie (« Frites & desserts »).

* 2026-10-09 — **Mise en production** sur `https://eat.fs0ciety.org` (Coolify, app
  `ichbnb7y7rucoiqfcs0xlhfd`, branche `main`, auto-deploy). CI GitHub verte (build Docker
  complet inclus). API Coolify 4.4 : `POST /api/v1/deploy` (plus de GET),
  `PATCH /applications/{uuid}/envs/bulk`, `POST /applications/{uuid}/storages`.
  `/api/occ/health` renvoie `version: dev` tant que « Include Source Commit in Build »
  n'est pas activé dans Coolify.
* 2026-10-09 — E2E Playwright (`e2e/`) : parcours complet hôte + 2 invités + non-membre,
  vert sur PocketBase v0.36 puis v0.40.5. Exception de version : `@playwright/test`
  épinglé en **1.56.1** pour correspondre au Chromium préinstallé de l'environnement
  cloud (chromium-1194) ; à monter avec l'image navigateur.
* 2026-10-09 — Docker Hub limite les pulls (429) dans l'environnement cloud : utiliser
  `mirror.gcr.io/library/<image>` + `docker tag`. Les builds Docker n'y ont pas de
  réseau : valider l'étape runtime avec les artefacts construits localement ;
  la CI GitHub fait le build complet.
* 2026-10-09 — Mise à niveau complète vers les dernières versions stables :
  Go 1.24.7 → **1.27.2** · PocketBase v0.36.6 → **v0.40.5** (+ `go get -u ./...`) ·
  Node 22 → **24 LTS** · `golang:1.27-alpine`, `node:24-alpine`, `alpine:3.24` ·
  actions checkout/setup-go/setup-node **v7**, setup-buildx **v4**, build-push **v7** ·
  jsdom 29 → 30 (les autres paquets npm étaient déjà au dernier stable).
  - PocketBase v0.40 passe à `encoding/json/v2` : réponses JSON avec slices nil → `[]`
    (plus `null`) et clés des records non triées ; `BindBody` reste insensible à la casse ;
  - `app.ResetBootstrapState()` déprécié → `ClearBootstrap()` (harness de tests) ;
  - v0.40 : les erreurs des commandes CLI sortent en code ≠ 0 ; en-tête
    `Cross-Origin-Opener-Policy: same-origin` ajouté par défaut ;
  - ozzo-validation remplacé par le fork `github.com/pocketbase/ozzo-validation` (indirect) ;
  - rules inchangées : `members.id ?= @request.auth.id` reste la bonne forme (tests authz verts).
  - **Exceptions sous la dernière version** : `typescript` **6.0.3** (7.0.2 existe, mais
    `typescript-eslint` 8.71.1, dernier stable, exige `typescript <6.1.0`) ;
    `@types/node` **24.19.1** (26.x existe mais suit Node 26 « Current » ; on aligne les
    types sur le runtime LTS 24). `pocketbase` JS SDK 0.28.1 est déjà le dernier.
* 2026-10-09 — Pièges PocketBase v0.36 découverts au bootstrap :
  - relations multiples dans les rules : `members.id ?= @request.auth.id`, **jamais** `members ?= …` (compare le JSON brut → personne ne matche) ;
  - une update rule ne voit que l'enregistrement stocké : bloquer les champs immuables dans le hook en comparant à `e.Record.Original()` ;
  - les create rules s'exécutent avant `OnRecordCreateRequest` → le hook peut écraser ce que le client envoie ;
  - effets de bord atomiques : `e.App.RunInTransaction(func(tx){ e.App = tx; e.Next() … })` puis restaurer `e.App` ;
  - `BoolField{Required:true}` exige `true`, `NumberField{Required:true}` refuse 0 ;
  - `apis.Static(…, true)` sur `/{path...}` répond aussi aux `/api/*` inconnus → garde dans `backend/spa.go` ;
  - tests d'intégration : un `pb_data` migré dans `TestMain`, cloné par test avec `tests.NewTestApp(dir)`.
* 2026-10-09 — Bootstrap : PocketBase **v0.36.6** (v0.36.7+ et v0.37+ exigent Go ≥ 1.25 ; monter Go avant de monter PocketBase — fait le jour même, voir ci-dessus).
  Le connecteur MCP Uber Eats n'était pas joignable pendant le bootstrap :
  l'intégration passe par l'adaptateur deep link (ADR 0002).

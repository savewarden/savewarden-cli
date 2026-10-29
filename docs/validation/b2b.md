# B2b — Rapport de validation

Date : **29 octobre 2026**. Lot B2b, première publication sécurisée de `sync`,
selon le [plan B2 v0.2](../../.plans/Savewarden_Plan_B2_Synchronisation_v0.2.md)
§8.2. Ce rapport accompagne la future PR ; il ne constitue pas une
auto-approbation.

## Cadre

| Élément | Valeur |
| --- | --- |
| GO humain | GO B2b du propriétaire, transmis par la session principale, puis arbitrages E1 et `fields` confirmés, avec le contrôle des temporaires abandonnés demandé avant les premières écritures, le 29 octobre 2026 |
| Branche | `feat/b2b-first-publication`, créée depuis `release/v0.1.0-a1` en `b4f649f`, Go 1.27.3 |
| PR | Pas encore ouverte à la rédaction ; puis, sur GO distincts, [savewarden/savewarden-cli#6](https://github.com/savewarden/savewarden-cli/pull/6), fusionnée dans `release/v0.1.0-a1` par le merge commit `8f9f5e1` |
| Écritures | Seul `internal/platform/vault` écrit, ADR 0005 ; tout autre package reste sous le garde de lecture seule |
| Revues indépendantes | Revue de l'assouplissement du garde, point de contrôle du §8.2 ; revue obligatoire de `platform/vault` et du chemin d'écriture, consignée ci-dessous |
| Pilote B2b | Non exécuté à la fusion ; **exécuté par le propriétaire après fusion, le 29 octobre 2026, et validé**, voir [Pilote B2b](#pilote-b2b) |

Commits, dans l'ordre :

| SHA | Commit | §8.2 |
| --- | --- | --- |
| `4fd1e79` | Temporaires abandonnés acceptés aux conditions exactes, ADR 0006 amendé | Contrôle demandé par le propriétaire |
| `5136636` | ADR 0005, garde assoupli pour `platform/vault` seul | 1 — **revue de l'assouplissement** |
| `ea33f7c` | Corrections de la revue de l'assouplissement | 1 |
| `a96e109` | Adaptateur `platform/vault`, `x/sys` v0.49.0 | 2 |
| `3a9fc56` | Champs d'étape stricts et plan complet de `planned` | Demande `fields` du propriétaire |
| `fb32239` | Volume et espace qualifiés au plan ; ports d'écriture | 3, préflight |
| `26319ea` | Initialisation, staging, vérification, publication, receipt | 3, 4 et 5 |
| `1a0cb5b` | Récupération d'une première publication interrompue | 6 — **revue de récupération** |
| `79a562f` | CLI `--apply` et `--recover` ; lecture d'un fichier verrouillé | 7 |
| `5ce7322` | Documentation B2b | 8 |
| `8a2ca83` | Corrections de la revue indépendante | 9 |
| `7abdedb` | Reste de la vérification des corrections | 9 |
| ce commit | Rapport de validation | 9 |

Diff total depuis `b4f649f`, ce rapport exclu : 62 fichiers, 7 588 insertions
et 324 suppressions, dont 3 897 lignes de tests.

## Décisions du propriétaire intégrées

- **E1 au sens strict** : neuf temporaires par numéro d'enregistrement,
  toutes étapes confondues ; le `k` est unique par numéro ; enveloppe propre
  à chaque receipt et au marqueur. `Journal.Attempts()` garde sa sémantique.
- **`fields`** : les champs propres à l'étape restent sous `fields`, avec un
  contrat fermé et typé par étape et une cohérence entre enregistrements
  vérifiée à la lecture, ADR 0006 §3.
- **Temporaires abandonnés** : un temporaire d'un numéro déjà validé est
  accepté si son étape était admise à ce numéro et si au plus huit restent à
  côté de l'enregistrement, y compris après un état terminal ; un `k` partagé,
  un numéro hors suite ou neuf temporaires restent des ambiguïtés. Le lecteur
  de B2a aurait rendu irrécupérables les reprises A1 et A6.

## Critères d'acceptation B2b (plan §12)

| # | Critère | Preuve |
| --- | --- | --- |
| 6 | Premier `current` égal au plan, relu avant publication, avec receipt v1 | `TestFirstPublicationIntoANewVault` ; `TestSyncApplyEndToEnd`, binaire réel : `inspect` de `current` donne les SHA-256 du receipt |
| 7 | Source jamais écrite | `TestSyncApplyEndToEnd` : écriture refusée par ACL sur l'installation pendant `--apply`, empreinte identique ; source identique après chaque crash simulé |
| 8 | Changement de la source à chaque phase : pas de publication | `TestSourceChangedDuringTheCopy`, `TestSourceChangedBetweenCopyAndRecheck`, `TestVerificationMismatch`, `TestWriteFailureKeepsTheStaging` |
| 9 | Rien d'existant écrasé, tronqué, supprimé ; `current` jamais remplacé | `TestCreationIsExclusive`, `TestMoveNeverReplaces`, Windows réel ; garde AST d'ADR 0005 ; `TestExistingCurrentIsRefused`, `TestSyncApplyRefusesToReplaceCurrent` |
| 10 | Initialisation : crash après chaque étape I2 à I8, jamais requalifiée, reprise par `--recover --apply` seulement | `TestCrashAtEveryVaultCall` et `TestRecoveryAfterEveryCrash` couvrent chaque appel d'I2 à I9 ; `TestRecoveryOfAnInterruptedInitialization`, `TestRecoveryOfAnUnstartedOperation`, `TestApplyRefusals` |
| 11 | Énumération des crashs, A1 à A8, ordre de bascule, course | Crash avant et après chacun des quelque 45 appels au coffre, puis récupération : chaque point aboutit à sa ligne d'ADR 0006, A1 abandonné, A2 annulé, A3 à A7 publiés ; A3 `TestRecoveryRebuildsTheReceipt` ; A5 et A6 `TestRecoveryKeepsAPublishedReceiptWithItsCurrent` ; A8 `TestRecoveryRefusesWhatTheProtocolCannotProduce` ; course `TestSwitchRaceIsCaughtAfterTheRename` ; `TestCurrentAppearingBeforeTheSwitch` ; portées fermées et identités, `TestParentPinsDoNotBlockChildRename`, `TestOpenHandlesBlockTheRename` |
| 12 | Propriétés du §5.9 dans l'enveloppe | `TestRecoveryIsInterruptible`, crash à chaque appel de la récupération, remise en staging comprise ; `TestAmbiguityIsStable`, trois exécutions identiques ; `TestRecoveryEnvelope`, huit et neuf temporaires d'un enregistrement, d'un receipt, du marqueur ; `TestRollbackFailureIsRecordedOnce`, E3 ; `TestRecoveryRefusesAnInvalidReceipt` ; provenance du receipt reconstruit |
| 13 | Espace selon V2, au préflight et sous verrou | `TestSpaceIsCheckedAtPlan`, `TestSpaceNeeded`, débordement 64 bits compris ; « space gone under lock » : ni journal ni staging, `metadata only` ; disque plein au fichier N, `TestWriteFailureKeepsTheStaging` |
| 14 | Garde pour `platform/vault` seul, `MoveFileEx` à `0`, `x/sys` limité, retries 32 et 33 | `readonly_test.go` et `architecture_test.go`, listes attendues indépendantes ; `TestRenameRetries`, `TestOtherRenameErrorsAreNotRetried` |
| 15 | Revue indépendante consignée ; pilote exécuté ou absence déclarée | Revue ci-dessous ; pilote non exécuté à la fusion, puis **exécuté après fusion et validé par le propriétaire** : le préalable de B2c est levé |
| 19 | 7 checks verts sur le SHA exact | Non établi à la rédaction ; puis 7 checks verts sur `971631b`, tête de la PR, et sur `8f9f5e1` après fusion, voir [CI](#ci) |
| 20 | Docs cohérentes, exemples vérifiés | Guide de commande, guide du coffre et de récupération, README, CHANGELOG, ADR 0004 à 0007 ; exemples de `docs/commands/sync.md` et du README vérifiés par test |
| 21 | Aucun accès aux données protégées | Tests en `t.TempDir()` ou en mémoire, chemins fictifs ; aucun accès à `D:\Ga`, `D:\Savewarden`, `E:\GMS` ni aux clés SSH |

## Contrôles locaux

Poste Windows 11, `go1.27.3 windows/amd64`, sur `8a2ca83` puis, pour gofmt,
vet, lint et tests, sur `7abdedb`.

| Commande | Résultat |
| --- | --- |
| `gofmt -l cmd internal .github/scripts` | Aucune sortie |
| `go mod tidy`, puis `git diff --exit-code -- go.mod go.sum` | Aucun écart ; `golang.org/x/sys` v0.49.0 ajouté |
| `go mod verify` | `all modules verified` |
| `go vet ./...`, `GOOS` windows, linux, darwin, freebsd, plan9 ; `go vet ./.github/scripts` | Succès |
| `go test -count=1 ./... ./.github/scripts` | `ok` pour les 11 packages |
| `go run ./.github/scripts` | `Workflow Action, Go tool and OS pins verified` |
| `CGO_ENABLED=0 go build -trimpath` pour windows, linux, darwin × amd64, arm64 | Succès, compilation seulement hors Windows |
| golangci-lint v2.14.0, compilé pour l'hôte, `GOOS` windows, linux, darwin | `0 issues.` trois fois |
| govulncheck v1.6.0 | `No vulnerabilities found.` |
| `go test -race ./...` | Non exécuté : pas de cgo local ; prouvé seulement par le job Linux de la CI |

Tests réels Windows de ce lot :

- `platform/vault` : création exclusive, `MoveFileEx` refusé sur toute cible
  existante, erreur 183 ; parent épinglé sans `FILE_SHARE_DELETE` qui laisse
  renommer un enfant, identité conservée ; dossier épinglé, erreur 32 ;
  fichier ouvert dedans, erreur 5 ; jonctions et lien symbolique relatif
  qu'`os.Root` suit, refusés ; chemins au-delà de `MAX_PATH` ; date à 100 ns ;
  verrou exclusif, libéré à la mort du processus détenteur ; volume
  temporaire NTFS.
- `platform/filesystem` : identité stable à travers un renommage, jonction
  refusée ; fichier tenu sans partage décrit, jamais ouvert.
- Binaire : publication sous ACL, second `--apply` à jour, `inspect` de
  `current`, refus de remplacement, processus tué pendant le staging de
  2 000 fichiers puis `--recover` et `--recover --apply`.

## CI

**À la rédaction : aucun run.** La branche n'était pas poussée ; le code Unix
nouveau, `Identity` de la portée et son test, n'avait été que compilé et
analysé localement.

**Après le GO de push :** les sept checks requis sont verts sur le SHA exact
de la PR, puis sur le commit de fusion. Ils couvrent aussi ce qui n'avait pas
pu tourner localement : le détecteur de courses et le code Unix.

| Commit | Run | Résultat |
| --- | --- | --- |
| `971631b`, tête de la PR #6 | [40912389777](https://github.com/savewarden/savewarden-cli/actions/runs/40912389777) | 7/7 : Verify Ubuntu, Windows et macOS, Race detector, Lint, Vulnerability scan, Pins |
| `8f9f5e1`, fusion dans `release/v0.1.0-a1` | [40915661887](https://github.com/savewarden/savewarden-cli/actions/runs/40915661887) | 7/7, mêmes jobs |

## Revues indépendantes

**Revue de l'assouplissement**, point de contrôle du §8.2, menée en lecture
seule par un agent séparé sur `4fd1e79` et `5136636`. Le lecteur du journal
est confirmé. Constats sur le garde, tous corrigés en `ea33f7c` avant tout
code d'écriture :

| Gravité | Constat | Correction |
| --- | --- | --- |
| Bloquant | `Unlink` et `Rmdir` absents des interdits | Ajoutés, avec les appels de mutation Unix |
| Important | Accès en écriture non lié à `CREATE_NEW`, masques larges non refusés | Règle d'ouverture sur l'arbre syntaxique ; masques refusés |
| Important | Chargement dynamique encore possible, `LazyDLL`, `NewProc` | Refusés, avec les appels système bruts restants |
| Important | Deux tests du garde itéraient ses propres listes | Listes attendues écrites à part du garde |
| Mineur | Services, sockets, `MethodByName`, `go:linkname`, sources non Go | Refusés ; imports `unsafe`, `reflect`, `net/*`, `crypto/tls` refusés |

**Revue obligatoire de `platform/vault` et du chemin d'écriture**, menée en
lecture seule par un agent séparé, à partir des ADR et du plan, sans mes
conclusions, sur `1a0cb5b` et la CLI en cours. Un constat bloquant, quatre
importants, huit mineurs, corrigés en `8a2ca83` :

| # | Constat | Correction |
| --- | --- | --- |
| B1 | En A5 ou A6, un `current` différent du plan était remis en staging à côté d'un receipt validé : receipt orphelin, jeu bloqué | Arrêt en code 3 avant toute écriture |
| I1 | Un receipt validé illisible faisait écrire un temporaire à chaque relance | Refus avant d'ouvrir le coffre |
| I2 | Un second `--apply` pendant le premier sortait en 3, `pending-operation` | Sonde du verrou en lecture seule : `another sync is running`, code 1, dans tous les modes |
| I3 | Une opération laissée ouverte était annoncée « kept in staging » | Intervention, code 3, `run sync --recover` |
| I4 | Trois tests plus faibles qu'annoncé | Identité comparée à `staged` du même run ; issue attendue par point de crash ; crash dans la remise en staging |
| M1 | Un processus hostile pourrait convertir sur place un dossier épinglé vide en point de montage | Limite documentée, hors modèle de menace (plan §6.1) |
| M2 | Reprise d'initialisation sans relecture après l'ouverture | Racine relue une fois épinglée |
| M3 | Aucun enregistrement et un `current` présent donnent `001-abandoned` | Conservé : la ligne I8 d'ADR 0006 ne regarde que `staging\<op>`, et B2c en aura besoin |
| M4 | Un `\r` dans une raison rendait `failed` invalide | Raison ramenée à une ligne |
| M5 | Relecture sous verrou incomplète | Receipts comparés par opération, receipts invalides, volume requalifié |
| M6 | Pré-contrôle de bascule en échec enregistré comme `rolled-back` | Ambiguïté, code 3, rien enregistré |
| M7 | Receipt existant comparé partiellement ; `started_at` pris hors de `planned` | Comparaison champ par champ ; `started_at` égal à la date de `planned`, vérifié à la lecture |
| M8 | Échec de `abandoned` annoncé « not modified » | Intervention dès qu'une écriture de récupération a eu lieu |

Les corrections B1, I1, I2, I3, M6 et la date de M7 ont été contrôlées par
mutation : le test associé échoue si elles sont retirées ; sans la date de
M7, 18 tests échouent.

**Vérification des corrections** par le même relecteur, en lecture seule sur
`8a2ca83`, sans exécuter de commande : tous les constats sont corrigés, M3
est accepté comme décision. Restait une partie de M6 : la remise en staging
de la récupération écrivait encore `rollback-failed` quand son pré-contrôle
trouvait un objet inexpliqué ; corrigé en `7abdedb`, avec
`TestRecoveryMoveBackRefusesAnUnexplainedStaging`, contrôlé par mutation.
Le message d'une reprise d'initialisation qui échoue à nouveau et la
première ligne du tableau d'ADR 0006, contradictoire avec I8, y sont aussi
corrigés. Le relecteur jugeait le repli de lecture d'un fichier verrouillé
non testé sous Windows : `TestLockedFileIsDescribed` le couvre et passe sur
ce poste.

Le binaire réel a révélé deux écarts que les faux ne pouvaient pas montrer :
la relecture sous verrou ne pouvait pas décrire `sync.lock`, tenu sans
partage, d'où la lecture de l'entrée de répertoire en `79a562f` ; et la date
de `planned` différait de celle du plan, d'où une fausse horloge qui avance
désormais à chaque lecture.

## Pilote B2b

**Au Human GO de fusion :** non exécuté. Aucune autorisation d'accès aux
sauvegardes réelles n'avait été donnée à l'agent ; il n'a lu aucune des zones
protégées par `AGENTS.md` et n'a pas exécuté le pilote ensuite.

**Après fusion, 29 octobre 2026 :** exécuté par le propriétaire, sur
Assassin's Creed Odyssey, selon [la page du pilote](../pilots/odyssey.md) :
une première publication vers un coffre neuf, puis un contrôle d'idempotence,
le second `--apply` n'étant lancé qu'après un plan `up to date`.

| Élément | Résultat |
| --- | --- |
| Binaire | Compilé depuis un clone distinct, dont le script du pilote vérifie que `HEAD` est le commit de fusion `8f9f5e1` avant toute exécution |
| Poste | Windows 11 |
| Source | `saves` sous l'installation déclarée par le manifeste du pilote |
| Coffre | Neuf, distinct de l'ancien coffre du pilote ; son volume a été qualifié par `sync` lui-même, NTFS local |
| Première publication | `sync --apply`, code `0` |
| Contenu publié | 14 fichiers, 9 520 705 octets ; 14 SHA-256 sur 14 identiques à la source |
| Source | Inchangée après la publication |
| Receipt | 1 receipt créé |
| Plan suivant | Code `0`, `plan: up to date`, aucune anomalie |
| Second `sync --apply` | Code `0`, `up to date`, dernière ligne de stderr `savewarden: vault: not modified` |
| Coffre après le second `--apply` | Inchangé : empreinte identique des chemins, tailles et SHA-256 de toutes ses entrées |
| Verdict | `IdempotencePassed = True` |

**Qualification fonctionnelle, 29 octobre 2026 :** le propriétaire valide le
pilote B2b. Il démontre sur une sauvegarde réelle les deux propriétés
nominales du lot : première publication copiée en entier, vérifiée par
SHA-256, avec receipt et source inchangée ; seconde exécution sans aucune
mutation du coffre.

Limites du pilote :

- Il qualifie le chemin nominal. Les crashs, l'initialisation interrompue et
  la récupération restent prouvés par les tests synthétiques et la CI ; le
  pilote ne simule aucune coupure de courant.
- Un seul jeu, une seule source, un seul poste ; la rotation de `current`
  relève de B2c.
- Ne sont versionnés ni les empreintes individuelles, ni le receipt réel, ni
  le nom de profil, ni aucun chemin personnel, ni la sortie complète ; selon
  les vérifications du propriétaire, l'ancien coffre et les sauvegardes
  sources restent intacts.

## Écarts au plan

- Les éléments 3, 4 et 5 du §8.2 forment un seul commit, `26319ea` :
  `Apply` les compose, et aucun n'aurait laissé seul un journal clos.
- Commits supplémentaires : `4fd1e79` et `3a9fc56` pour les demandes du
  propriétaire, `fb32239` pour le préflight, `ea33f7c` et `8a2ca83` pour les
  revues.
- L'adaptateur n'exécute pas l'initialisation : il n'offre que des
  primitives, `features/sync` porte le protocole. Les ports `Vault`,
  `Vaults`, `Volumes` et `Clock` s'écartent de la forme indicative du §3.2 ;
  ADR 0004 est amendé. La création de fichier s'appelle `NewFile`, le garde
  contrôlant tout appel `CreateFile`.
- `x/sys` v0.49.0 entre dans `go.mod` au commit de l'adaptateur et non au
  premier : `go mod tidy` retire une dépendance que rien n'importe.
- `platform/filesystem` est modifié, hors de la liste du §7 : identité de
  fichier pour la récupération, description d'un fichier tenu sans partage,
  erreur de partage marquée comme telle.
- Le plan contrôle déjà volume et espace et affiche l'espace nécessaire, pas
  l'espace libre, qui rendrait deux plans d'un même état différents.
- La ligne `rolled back <op>; current unchanged` sert dès B2b, quand le
  renommage de la publication échoue sans rien déplacer.
- Les temporaires de receipt orphelins restent tolérés comme en B2a ; la
  borne « huit à côté d'un fichier validé » n'est appliquée qu'aux
  enregistrements du journal.

## Risques résiduels et limites

- Pas d'instantané atomique ; jeu et launcher doivent être fermés.
- Windows ne garantit pas la durabilité d'un renommage de dossier après une
  coupure ; la récupération repose sur l'observation, ADR 0006 §6.
- Le garde reste un filet fondé sur les noms ; un drapeau numérique dans les
  attributs d'un `CreateFile` lui échappe ; les tests dynamiques le
  complètent.
- Processus hostile du même compte : hors modèle ; un dossier épinglé vide
  peut être converti sur place.
- ReFS non qualifié ; FAT32, exFAT et réseau refusés par faux `Volumes`
  seulement, aucun volume de ce type n'étant disponible sur ce poste.
- `subst` et deux volumes d'un même disque ne sont pas détectés.
- Le test du processus tué dépend du temps de copie ; il est sauté, et le
  dit, si la copie finit avant l'interruption.
- La sonde du verrou ouvre `sync.lock` en lecture un instant quand une
  opération est en attente : un `sync --apply` lancé à cet instant précis
  échoue à tort, `another sync is running`, code 1, sans rien perdre.
- Les champs du receipt autres que les dates et les sources n'ont pas de
  test dédié de comparaison en A5 ; la relecture sous verrou et la raison
  sur une ligne n'ont pas de test propre.
- Mainteneur unique : les revues sont des avis, pas des approbations ; le
  Human GO reste le contrôle final.

## Questions pour le propriétaire

1. Autoriser le push de `feat/b2b-first-publication` et l'ouverture de la PR
   vers `release/v0.1.0-a1`. *Réglée : GO de push, puis GO de fusion ; PR #6
   fusionnée en `8f9f5e1`.*
2. Valider le regroupement des éléments 3 à 5 du §8.2 en un commit, et les
   écarts ci-dessus, dont la modification de `platform/filesystem`.
   *Sans réponse explicite à ce jour.*
3. Confirmer la décision M3 : une opération sans enregistrement est
   abandonnée même si un `current` existe. *Sans réponse explicite à ce
   jour.*
4. Après fusion, fixer le cadre du pilote B2b : coffre neuf, volume, jeu, et
   autorisation nominative. *Réglée : pilote exécuté par le propriétaire et
   validé, voir [Pilote B2b](#pilote-b2b).*

## Recommandation

**GO technique pour revue de la PR**, sous réserve des sept checks verts sur
le SHA exact : critères 6 à 14, 20 et 21 couverts ; revues indépendantes
menées et intégrées ; critère 15 partiel, le pilote attendant son GO. **Pas
d'auto-approbation** : push, PR, fusion et pilote attendent chacun une
décision du propriétaire. Aucun tag ni release.

**Après fusion :** CI verte sur la tête de la PR et sur le commit de fusion,
pilote exécuté et validé par le propriétaire. B2b est clos fonctionnellement ;
le préalable de B2c est levé, mais B2c attend son propre GO de développement.
Aucun tag ni release.

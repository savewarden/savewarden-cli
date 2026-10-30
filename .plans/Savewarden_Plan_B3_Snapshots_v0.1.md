# Savewarden — Plan B3 : snapshots immuables

> **Statut : v0.1 — BROUILLON DE CADRAGE, soumis à la revue du propriétaire ; décisions S1 à S14 proposées, aucune prise ; aucun GO de développement**
> Version du plan : `0.1` — 30 octobre 2026, revue indépendante intégrée
> Lot : **B3**, cible `release/v0.1.0-a1`
> Cadre : [plan directeur v0.1](Savewarden_Plan_Bootstrap_Architecture_GitOps_v0.1.md) §7 B3,
> [plan B2 v0.2](Savewarden_Plan_B2_Synchronisation_v0.2.md), [AGENTS.md](../AGENTS.md),
> [modèle de sécurité](../docs/safety-model.md), ADR [0001](../docs/adr/0001-fsd-inspired-go.md),
> [0004](../docs/adr/0004-sync-boundaries.md) à [0007](../docs/adr/0007-sync-contract-receipt-v1.md).

B2 protège l'état courant d'un jeu : `current` est remplacé à chaque `sync`,
et l'ancienne génération rejoint `retained\`. Ces générations suivent la
rotation ; elles ne sont pas des snapshots approuvés (plan B2 §2.2). B3 ajoute
des **snapshots** : des copies vérifiées, nommées, que l'utilisateur crée
explicitement et que rien ne modifie, ne déplace ni ne supprime ensuite. Ce
sont eux que B4 restaurera dans un dossier isolé.

Le plan directeur fixe le cap : snapshots indépendants de `current`,
identifiant unique, `.incomplete` tant que non vérifiés, publication
uniquement après SHA-256 complet ; tests de versions, d'idempotence, de crash
et d'impossibilité de réécrire un snapshot approuvé ; test sur Odyssey après
accord explicite. Ce brouillon propose les arbitrages qui manquent pour en
faire un plan exécutable.

ADR 0006 reste la **référence de sécurité** : son §1 inclut déjà les
snapshots de B3. B3 l'amende avant tout code d'écriture ; les deux
assouplissements proposés, I8 par domaine (S9) et le traitement des anomalies
d'un snapshot déjà approuvé (S13), sont nommés comme tels et soumis au
propriétaire.

Ce plan ne vaut ni GO de développement, ni autorisation de push, de tag ou de
release, ni accès aux sauvegardes réelles.

---

## 0. Décisions proposées au propriétaire

Chaque ligne est une **proposition** ; le propriétaire la retient, la modifie
ou la rejette. La version 0.2 intégrera ses arbitrages.

| # | Question | Proposition | Alternatives | Où |
| --- | --- | --- | --- | --- |
| S1 | Que copie un snapshot ? | La génération **`current` du coffre**, reconnue par toute sa chaîne de preuve et relue en entier contre son receipt sous verrou : le snapshot est la copie exacte de ce receipt | (b) Les sources du manifeste, comme `sync` ; (c) une commande qui enchaîne `sync --apply` puis le snapshot | §2.1, §4.3 |
| S2 | Où vit le code ? | **Dans `features/sync`**, comme seconde opération du même moteur de publication ; commande CLI `snapshot` distincte | (b) `features/snapshot` qui duplique le moteur ; (c) extraction du moteur dans un nouveau package, puis `features/snapshot` | §3.1 |
| S3 | Où sont rangés les snapshots ? | **Tout sous `games\<game-id>\snapshots\`** : snapshots publiés, stagings, receipts et journaux de snapshot ; `sync` accepte cette entrée et ne lit rien dedans | Receipts et journaux de snapshot mêlés à ceux de `current`, ce qui casse la recherche du receipt de tête et le lecteur du journal | §4.2, §4.8 |
| S4 | Identifiant | **L'identifiant de l'opération de snapshot**, `AAAAMMJJTHHMMSS-xxxxxxxx`, UTC et 8 chiffres hexadécimaux aléatoires ; distinct de tout identifiant des deux domaines ; jamais réutilisé | Identifiant choisi par l'utilisateur | §4.2 |
| S5 | Staging | **`snapshots\<snap-id>.incomplete\`**, publié par renommage en `snapshots\<snap-id>\` ; jamais supprimé, même abandonné ; le plan en affiche le nombre et la taille | Staging commun `staging\` | §4.2, §5 |
| S6 | Quand un snapshot est-il approuvé ? | **À sa publication** : receipt de snapshot validé et enregistrement `published` ; aucun état d'approbation humaine distinct | Étape `--approve` explicite après publication | §4.6, §6 |
| S7 | Deux `--apply` sans nouvelle génération | **Un snapshot conforme par génération source** : si un snapshot approuvé de la génération `current` est présent et conforme à son receipt, `--apply` n'écrit rien, code `0`, `not modified` | Un nouveau snapshot à chaque appel, contenu identique | §4.6 |
| S8 | Receipt | **Nouveau schéma strict `savewarden.snapshot` v1** : provenance (opération et SHA-256 du receipt de la génération copiée), `sources` recopiées de ce receipt | Receipt v1 de `sync` étendu, ce qui casse sa relecture stricte et la recherche du receipt de tête | §4.5 |
| S9 | Exclusivité | **Même verrou par jeu, `sync.lock` ; I8 par domaine** : une opération non terminale de `current` bloque `sync --apply` et `snapshot --apply` ; une opération de snapshot non terminale ne bloque que `snapshot --apply` ; `sync` ne lit rien sous `snapshots\` | I8 sur l'union des deux journaux : `sync` lit les journaux de snapshot, et une anomalie de snapshot bloque toute la protection de `current`, récupération comprise | §5.6 |
| S10 | Commande | `snapshot --manifest <m> --vault <v> [--apply] [--recover]` ; **le plan liste les snapshots publiés** ; ni libellé, ni sous-commande `list` | `--label <texte>` ; `snapshot list` | §4.1 |
| S11 | Livraison | **Une PR** `feat/b3-snapshot`, partie de commits de cadrage validés, sur le modèle de B2c, avec revues indépendantes aux points de contrôle | Deux PR, B3a en lecture seule puis B3b qui écrit, sur le modèle de B2a et B2b | §8 |
| S12 | Immutabilité | **Par protocole seulement** : créations exclusives, aucun déplacement après `published`, aucune API d'écriture nouvelle, vérification de tous les snapshots par `doctor` (B5) | Attribut lecture seule par `SetFileAttributes`, qui exige un amendement d'ADR 0005 ; ACL de refus d'écriture | §6 |
| S13 | Snapshot approuvé absent, remplacé ou altéré | **Signalé, jamais bloquant ni réparé** : `snapshot-missing` ou `snapshot-modified`, information ; il ne compte plus pour S7 ; Savewarden ne le recrée, ne le déplace ni ne l'adopte | Fail-closed, code 3, comme `current` : sans issue, puisque B3 ne supprime rien et ne réécrit aucun journal | §4.6, §5.6 |
| S14 | Coffre du pilote B3 | **Un coffre neuf**, rempli par `sync --apply` avec le binaire B3, puis snapshot ; le coffre des pilotes B2 reste intact et lisible par les binaires B2 | Le coffre des pilotes B2 : plus réaliste, mais il ne serait plus utilisable que par un binaire B3 ou postérieur, même après un revert | §11 |

**Pourquoi S1.** Le coffre détient déjà une génération vérifiée et décrite
par un receipt. La copier ne lit pas le contenu de la source du jeu : le jeu
peut rester ouvert, et aucun second pipeline de copie depuis la source n'est
nécessaire. Le snapshot est alors égal, fichier par fichier, à un receipt
existant : sa provenance est prouvable. « Indépendant de `current` » signifie
que le snapshot ne partage aucun fichier avec `current` : copie complète, sans
lien physique, que `CreateHardLink`, interdit par le garde, exclut de toute
façon ; aucune rotation ultérieure ne le touche ; sa récupération ne lit
jamais `current`. Pour figer l'état du jeu, l'utilisateur lance `sync --apply`
puis `snapshot --apply`. L'alternative (c) mêlerait deux opérations dans un
même appel, avec deux journaux et deux récupérations.

**Pourquoi S2** : voir §3.1.

**Pourquoi S9 par domaine.** Les deux domaines ne dépendent pas l'un de
l'autre dans le sens qui compte : un snapshot ne se récupère jamais à partir de
`current`, et `current` ne dépend d'aucun snapshot. La forme stricte ferait
lire à `sync` les journaux de snapshot ; or `sync --recover` refuse toute
intervention autre qu'une opération en attente (`recover.go`) : un journal de
snapshot illisible empêcherait alors de récupérer un `current` absent dans la
fenêtre d'une rotation. La protection de l'état courant ne doit pas dépendre
d'une copie secondaire. L'amendement d'I8 qui en découle est nommé au §5.6.

**Pourquoi S13.** ADR 0006 tient un `published` sans objet de l'identité
attendue pour une ambiguïté. Pour `current`, c'est juste : la rotation
suivante en dépend. Aucune opération ne dépend d'un snapshot déjà approuvé, et
B3 ne supprime ni ne réécrit rien : un code 3 sur un snapshot disparu serait
définitif. S13 limite cette exception aux opérations de snapshot terminales en
`published` ; une opération de snapshot non terminale reste fail-closed.

---

## 1. Contexte et preuves

| Élément | État vérifié le 30 octobre 2026 |
| --- | --- |
| `release/v0.1.0-a1` | `56cada2`, merge de la PR #10, décisions de clôture de B2c |
| CI | Run 41132916039 sur la tête de la PR #10 et run post-fusion 41133566052 sur `56cada2` : terminés, `success`, 7 checks chacun |
| B2 | Clos : B2a, B2b, B2c fusionnés ; pilotes B2b et B2c exécutés et qualifiés par le propriétaire ; amendements d'ADR 0006 postérieurs au cadrage B2c ratifiés le 30 octobre 2026 |
| Toolchain | Go 1.27.3, golangci-lint 2.14.0 |
| Ancien coffre du pilote | Constat rapporté par le plan directeur, **non exploré** : un snapshot `20261009T183208-cacd3cae`, créé hors de Savewarden. Jamais lu, adopté, importé ni réécrit par B3 (D17 de B2, invariant 7 du modèle de sécurité) |

Constats du code, relus à `56cada2` :

- **Disposition.** Les noms d'un dossier de jeu appartiennent à
  `features/sync` (`vault.go`, constantes `gamesDir` à `journalDir`) ;
  `platform/vault` n'en code aucun. Aucun code de production ne crée ni ne lit
  `snapshots` ; seul le texte d'aide de la commande racine annonce les
  snapshots pour plus tard.
- **Refus des entrées inconnues.** La lecture du coffre par `sync` traite
  toute entrée imprévue de `games\<game-id>` comme `unexpected-entry`, code 3,
  en plan, `--apply` et `--recover`. Un dossier `snapshots`, même vide, fait
  donc échouer, fail-closed, tout binaire antérieur à B3.
- **Receipt de tête.** `sync` reconnaît `current` par le seul receipt valide
  qu'aucun autre ne nomme `previous` ; un receipt d'un autre jeu est invalide.
  Un receipt de snapshot rangé dans `receipts\` créerait une seconde tête :
  `current-modified`. D'où S3 et S8.
- **Journal.** Le lecteur applique l'automate de `sync` à tout dossier de
  `journal\`. Ses journaux expliquent aussi les dossiers de `staging\` et
  nourrissent la règle M3 (`generationExpected`) : un journal de snapshot y
  fausserait les deux. D'où S3 et S9.
- **Récupération.** `sync --recover` refuse toute intervention autre qu'une
  opération en attente : une anomalie lue dans un autre domaine bloquerait la
  récupération de `current`.
- **Moteur.** Validation d'un enregistrement (`record`, `commitFile`, choix de
  `k`, enveloppe E1, retries), bascule contrôlée par identité (`switchTo`),
  copie hachée (`copyFile`, `copyHashed`), relecture (`verify`,
  `compareWithReceipt`), reconnaissance de `current` (`recognize`,
  `checkCurrent`) : tout est non exporté dans `features/sync`. Noms de
  disposition, codes (`changed-during-sync`) et textes y sont codés en dur
  dans `apply.go`, `publish.go`, `copy.go`, `vault.go` et `recover.go`. Seuls
  les types de `entities/backup` sont partagés.
- **A5 et A6.** Quand le receipt d'une opération est déjà validé, la
  récupération refuse de remettre la génération en staging, code 3
  (`recover.go`), pour ne pas laisser un receipt orphelin.
- **Identifiant.** `ValidOperationID` (`entities/backup/strict.go`) fixe la
  forme ; `features/sync/apply.go` la produit, aléa de `crypto/rand` injecté
  par `app`.
- **Écritures.** `platform/vault` offre déjà toutes les primitives d'un
  snapshot : `CreateDir`, `NewFile`, `Move` sans drapeau, `Identity`, `Hold`,
  `Release`, `Lock`, et `Describe`. `Lock` **crée** le fichier de verrou s'il
  manque. Le garde de lecture seule refuse `SetFileAttributes` et
  `CreateHardLink` partout.
- **Tests.** Les faux du package `sync_test` (`fakeFS`, `fakeVaults` avec
  `crashBefore` et `crashAfter`, empreinte du coffre, énumérations des
  crashs) et les helpers binaires de `cmd/savewarden` ne sont réutilisables
  que depuis leur propre package.
- **Détails à traiter.** `app_test.go` emploie `snapshot` comme exemple de
  commande inconnue ; la documentation du package `sync` décrit encore un
  build qui n'écrit rien ; le verrou occupé s'annonce « another sync is
  running » ; `.gitignore` ignore tout dossier `snapshots/`, ce qui interdit
  ce nom à des fixtures versionnées.

---

## 2. Objectif, non-objectifs, permissions

### 2.1 Objectif

```text
savewarden snapshot --manifest <chemin> --vault <dossier> [--apply] [--recover]
```

1. Sans `--apply`, affiche un **plan** sans rien écrire : génération `current`
   reconnue, snapshots publiés et leurs anomalies, `.incomplete` conservés,
   ce que `--apply` copierait, espace requis, opérations en attente.
2. Avec `--apply`, sous le verrou du jeu, relit `current` en entier contre son
   receipt, le copie dans `snapshots\<snap-id>.incomplete\`, relit la copie
   depuis le disque, la compare au receipt, puis la publie par renommage en
   `snapshots\<snap-id>\` avec un receipt de snapshot.
3. Ne supprime, ne tronque, n'écrase et ne réécrit **rien** ; n'écrit que sous
   `snapshots\` ; ne lit le contenu ni de la source du jeu, ni de son
   installation, dont seules les identités servent au contrôle de distinction.
4. Un snapshot publié n'est **plus jamais** ouvert en écriture, déplacé ni
   supprimé, par aucune commande.
5. Une opération interrompue se récupère par `snapshot --recover`, selon
   ADR 0006 amendé.

### 2.2 Non-objectifs B3, opposables en revue

- **Restauration**, en place ou ailleurs : B4.
- **Suppression, rétention, nettoyage** de snapshots, de stagings
  `.incomplete` ou de temporaires : jamais en B3.
- **Snapshot d'une génération `retained\`** ou d'une source isolée.
- **Libellés, notes, renommage** d'un snapshot (S10).
- **Export, copie hors du coffre, archive, compression, chiffrement.**
- **Import** d'un snapshot créé hors de Savewarden, dont celui de l'ancien
  coffre du pilote.
- **Vérification du contenu de tous les snapshots** : `doctor`, B5.
- **Réparation** d'un snapshot approuvé absent ou altéré (S13).
- Initialisation d'un coffre par `snapshot` : seul `sync --apply` crée un
  coffre.
- Attribut lecture seule, ACL, ReFS, coffre copié ou déplacé (§13).
- Support fonctionnel Linux et macOS, sortie JSON de la CLI.

### 2.3 Permissions : agent, produit, propriétaire

Inchangées depuis le plan B2 §2.3 :

| Acteur | Peut | Ne peut pas |
| --- | --- | --- |
| **Agent** | Lire et modifier le dépôt ; tests synthétiques en `t.TempDir()` | Explorer ou lire `D:\Ga`, `D:\Savewarden`, `E:\GMS` ou des clés SSH ; lancer Savewarden sur des données réelles sans autorisation explicite et nominative pour cette exécution |
| **Savewarden**, lancé par le propriétaire | Lire le manifeste désigné ; relever sans suivi l'identité de l'installation et des sources ; lire et écrire **seulement** dans le coffre passé à `--vault`, et pour `snapshot` n'écrire que sous `games\<game-id>\snapshots\` | Lire le contenu de la source, écrire ailleurs, adopter, suivre un lien, supprimer |
| **Propriétaire** | Exécuter les pilotes, autoriser nominativement une exécution par l'agent | — |

---

## 3. Architecture et ADR

### 3.1 Où vit le code (S2)

ADR 0001 interdit qu'une feature en importe une autre. Le moteur qu'un
snapshot exige est entièrement dans `features/sync`, non exporté, qualifié par
deux pilotes et par l'énumération des crashs. Ses noms de disposition et ses
codes y sont codés en dur : **toutes les options modifient du code qualifié**.
Elles diffèrent par la frontière de package et par ce qu'il faut déplacer.

| Option | Contenu | Pour | Contre |
| --- | --- | --- | --- |
| **(a) Proposée : opération de `features/sync`** | Fichiers `snapshot*.go` dans `features/sync` ; méthodes de service propres au snapshot ; commande CLI `snapshot` qui les appelle ; moteur paramétré par la disposition et les codes de l'opération, dans le même package | Aucune duplication ; aucun code déplacé ni frontière nouvelle ; verrou, lecture du coffre et reconnaissance de `current` en un seul endroit ; faux et énumérations de crash réutilisés tels quels | Refactor interne du moteur qualifié ; le nom `sync` couvre désormais toutes les publications du coffre d'un jeu ; le package grandit d'environ un tiers |
| (b) `features/snapshot` dupliqué | Copie du moteur et des faux | Package nommé comme la commande ; code de B2 intact | Deux copies du code d'écriture le plus sensible, qui divergeraient ; reconnaissance de `current` dupliquée, avec un risque de désaccord entre les deux commandes ; ADR 0004 a rejeté la duplication du parcours pour cette raison |
| (c) Extraction du moteur | Nouveau package de protocole sous `internal/`, consommé par `sync` et `snapshot` | Prépare B4, qui publiera aussi selon ADR 0006 | Même refactor que (a), plus un déplacement de code, une frontière durable nouvelle, son ADR et ses tests d'imports, et des faux de test à déplacer ou dupliquer |

**Proposition : (a) en B3**, et réexamen de (c) au cadrage de B4. B4 sera le
premier consommateur réellement différent, puisqu'il écrira hors du coffre :
c'est lui qui justifiera, ou non, une extraction. ADR 0004 est amendé pour
dire que `features/sync` possède toutes les publications dans le coffre d'un
jeu, `current` et snapshots ; aucune règle d'import ne change.

Fichiers prévus dans `features/sync`, sans `manager.go` ni fourre-tout :
`snapshot.go` (plan et préconditions), `snapshot_apply.go` (automate),
`snapshot_recover.go` (récupération). Le refactor du moteur est un commit
distinct, sans changement de comportement pour `current` : la suite B2
inchangée, énumérations de crash comprises, et une sortie de `sync` identique
octet pour octet en sont la preuve.

### 3.2 Ports

Aucun port nouveau. Le plan de snapshot reçoit `ManifestReader`, `Files`,
`Inventories` et `Volumes` ; `--apply` et `--recover --apply` reçoivent en
plus `Vaults`, `Clock` et l'aléa, comme `sync`. `platform/vault` est
inchangé ; `internal/app/readonly_test.go` et ADR 0005 aussi.

### 3.3 ADR à produire

Tous sont rédigés sur `docs/b3-framing` après les arbitrages du propriétaire,
revus avec le cadrage, puis versés comme premiers commits de la PR, avant tout
code d'écriture (§8).

| ADR | Objet |
| --- | --- |
| **0006, amendement** | Publication d'un snapshot : disposition, structure, automate, bascule, points d'arrêt, tableau de récupération, enveloppe ; I8 par domaine (S9) ; anomalies d'un snapshot approuvé (S13) |
| **0008 — Contrat de `snapshot` et receipt de snapshot v1** | Modes, flags, sortie du plan, codes 0 à 3, ligne d'état, codes stables, schéma du receipt de snapshot |
| **0004, amendement** | `features/sync` possède les publications du coffre d'un jeu, snapshots compris ; la phrase qui annonçait une feature `snapshot` séparée est révisée |
| **0007, amendement** | `sync` accepte `games\<game-id>\snapshots\` s'il s'agit d'un dossier ordinaire, sans rien y lire ; message neutre du verrou occupé, commun aux deux commandes |

ADR 0005 n'est pas amendé : B3 n'ajoute aucune API d'écriture.

---

## 4. Contrats

### 4.1 Commande et modes

```text
savewarden snapshot --manifest <m> --vault <v>                     plan seul, rien n'est écrit
savewarden snapshot --manifest <m> --vault <v> --apply             copie current, vérifie, publie
savewarden snapshot --manifest <m> --vault <v> --recover           plan de récupération, rien n'est écrit
savewarden snapshot --manifest <m> --vault <v> --recover --apply   exécute la récupération
```

- `--manifest` et `--vault` obligatoires, mêmes grammaires que `sync` ; aucun
  défaut, aucune variable d'environnement, aucun fichier de configuration.
- Le manifeste fournit l'identifiant du jeu, donc le dossier
  `games\<game-id>`, ainsi que l'installation et les sources pour le contrôle
  de distinction. Le contenu des sources n'est pas lu.
- Le mode plan ne reçoit pas `Vaults` : il ne peut rien écrire, prouvé par un
  test binaire sous ACL de refus d'écriture ; de même pour `--recover` sans
  `--apply`.
- `--recover` exclut toute nouvelle copie ; il ne récupère que des opérations
  de snapshot et renvoie à `sync --recover` pour une opération de `current`.
- Windows seulement, comme `sync`.

### 4.2 Disposition du coffre (S3, S4, S5)

```text
<vault>\games\<game-id>\
  sync.lock, current\, retained\, staging\, receipts\, journal\   inchangés (B2)
  snapshots\
    <snap-id>\<source-id>\...              snapshot publié, immuable
    <snap-id>.incomplete\<source-id>\...   copie en cours, échouée ou abandonnée ; jamais publiée sous un autre nom
    receipts\<snap-id>.json                receipt de snapshot
    journal\<snap-id>\NNN-<étape>.rec      journal de l'opération de snapshot
```

- `<snap-id>` est l'identifiant de l'opération de snapshot, de forme
  `ValidOperationID` ; il ne peut donc pas valoir `receipts` ni `journal`. Un
  identifiant déjà employé dans l'un ou l'autre domaine, par un journal, un
  receipt, `staging\`, `retained\` ou `snapshots\`, est refusé avant tout
  journal, code 1 ; une nouvelle exécution en tire un autre.
- **Grammaire** de `snapshots\`, contrôlée par `snapshot` : `receipts`,
  `journal`, et des dossiers ordinaires `<snap-id>` ou `<snap-id>.incomplete`.
  Toute autre entrée, un lien, un fichier, un point d'analyse :
  `unexpected-entry`, code 3.
- **Explication**, comme ADR 0006 §3, par nom **ou** par identité : chaque
  dossier doit porter le nom d'une opération de snapshot journalisée ; son
  identité est exigée dès que `staged` l'a fixée. Un `<snap-id>.incomplete`
  dont le journal s'arrête avant `staged` est accepté s'il est un dossier
  ordinaire sans point d'analyse, jamais lu ni déplacé. Un snapshot approuvé
  absent ou d'une autre identité relève de S13.
- `snapshots`, `snapshots\receipts` et `snapshots\journal` sont des dossiers
  de **structure** : créés par `snapshot --apply` sous verrou, **après** le
  second contrôle d'espace et avant tout journal, par création exclusive, dans
  cet ordre et seulement s'ils manquent. Suit le dossier d'opération
  `snapshots\journal\<snap-id>`, puis `001-planned`. Une structure partielle,
  faite de dossiers de structure vides, ou un dossier d'opération vide, est
  un état reconnu : le premier est complété au `--apply` suivant, le second
  relève de la ligne « Aucun » du §5.4.
- Le verrou du jeu reste `sync.lock`. `snapshot` ne le crée jamais : il exige
  qu'il existe avant de le prendre, puisqu'une génération publiée l'a créé ;
  son absence à côté d'un `current` reconnu est une contradiction, code 3.

### 4.3 Préconditions et pipeline

| Phase | Écrit ? | Contenu | En cas d'échec |
| --- | --- | --- | --- |
| 1. Préflight | Non | Manifeste strict ; coffre existant, marqué, sans lien ; distinction du coffre, de l'installation et des sources par segments et par identité, sans lire leur contenu ; dossier du jeu et `sync.lock` présents ; aucune opération non terminale de `current` ; aucune opération de snapshot non terminale ; `current` reconnu par toute sa chaîne de preuve, égal à son receipt, identité relue après le contenu ; grammaire et explication de `snapshots\`, receipts et journaux de snapshot relus strictement ; volume et espace | Code 1, 2 ou 3, rien écrit |
| 2. Décision | Non | Snapshot approuvé de la génération `current`, présent sous son identité publiée, relu en entier conforme à son receipt, identité relue après : `up to date` (S7). Sinon, nouveau snapshot ; une anomalie est signalée (S13) | Code 0, rien écrit |
| 3. Verrou et structure | Structure | `sync.lock` existant pris ; état relu, `vault-changed` s'il a changé depuis le plan ; espace recontrôlé ; identifiant tiré et contrôlé ; **puis** structure complétée et dossier d'opération créé | Avant la structure : code 1 ou 2, rien écrit ; après : au plus `metadata only` |
| 4. `planned` | Journal | Plan : génération source, SHA-256 de son receipt, identité de `current`, `sources` du receipt | — |
| 5. Copie | `.incomplete` | Juste avant : `current` relu en entier contre son receipt, identité relue après ; `staging-intent`, création de `I`, copie hachée de chaque fichier, date conservée ; identité de `current` relue après la copie | Contenu différent : `current-modified`, code 2, `failed` ; identité changée : code 3, rien enregistré de plus ; `I` conservé |
| 6. Vérification | Journal | Relecture de `I` depuis le disque, comparée au receipt source : chemins, tailles, dates, SHA-256, dossiers ; identité de `I` relue après le contenu | `verification-mismatch`, code 2, `I` conservé |
| 7. Publication | Renommage, receipt | `publish-intent`, bascule `I` → `P` selon l'ordre d'ADR 0006 §4, receipt de snapshot, `published` | Rollback par défaut (§5) |

- **Espace :** taille complète de la génération source plus max(64 Mio, 5 %),
  disponible pour l'utilisateur appelant, au préflight puis sous verrou avant
  toute écriture, comme V2.
- **Chemins longs :** `long-path`, information, est mesuré sur le chemin le
  plus long écrit, celui de `I`, qui dépasse celui de `current` de
  38 caractères (`snapshots\`, `<snap-id>` et `.incomplete` au lieu de
  `current`) ; le chemin publié `P` le dépasse de 27.
- **Jeu ouvert :** sans effet sur `snapshot`, qui ne lit pas la source.

### 4.4 Ce que `snapshot` ne touche jamais

`current`, `retained\`, `staging\`, `receipts\`, `journal\` et `sync.lock` de
`sync`, en écriture comme en déplacement ; un snapshot publié ; un
`.incomplete` d'une autre opération ; la source, l'installation, le
manifeste ; un coffre non marqué.

### 4.5 Receipt de snapshot v1 (S8)

Fichier `snapshots\receipts\<snap-id>.json`, validé par temporaire et
renommage, enveloppe E1 propre. JSON canonique, relecture stricte comme le
receipt v1 : champ inconnu, manquant ou en double refusé.

| Champ | Contenu |
| --- | --- |
| `schema`, `schema_version` | `savewarden.snapshot` et `1` |
| `snapshot` | `<snap-id>` |
| `tool_version`, `recovered_by` | Comme le receipt v1 |
| `game` | Recopié du receipt source |
| `started_at`, `publish_intent_at`, `published_at`, `recovered_at` | Comme le receipt v1, mêmes règles d'ordre et de reconstruction |
| `source.operation` | Opération de la génération copiée |
| `source.receipt_sha256` | SHA-256 des octets de son receipt |
| `plan_sha256` | Empreinte du plan journalisé |
| `sources` | Recopiées du receipt source, octet pour octet en forme canonique |

Pas de `previous` ni de `changes` : un snapshot ne remplace rien. L'empreinte
de contenu des `sources` d'un snapshot est égale à celle de sa génération
source : c'est le recoupement de provenance.

### 4.6 Approbation, versions, idempotence (S6, S7, S13)

- Un snapshot est **approuvé** quand son receipt est validé et que son journal
  se termine par `published`. Avant, il n'est pas approuvé et peut encore être
  renommé en arrière par un rollback ; après, plus jamais.
- Les snapshots s'accumulent, du plus ancien au plus récent par identifiant ;
  aucun n'est supprimé. Le plan affiche aussi le nombre et la taille des
  `.incomplete` conservés.
- `--apply` est **idempotent par génération** : si un snapshot approuvé a pour
  `source.operation` l'opération de `current`, pour `source.receipt_sha256`
  l'empreinte de son receipt, et s'il est présent et conforme, rien n'est
  écrit, code `0`, `up to date`, `vault: not modified`. Un `.incomplete` ne
  compte pas.
- **Anomalies d'un snapshot approuvé (S13).** Absent : `snapshot-missing` ;
  présent sous une autre identité, ou altéré quand il est relu :
  `snapshot-modified`. Les deux sont des informations : ils ne bloquent ni
  `snapshot` ni `sync`, et le snapshot en cause ne compte plus pour
  l'idempotence ; un nouveau snapshot de la même génération est alors
  possible. Savewarden ne le recrée, ne le déplace ni ne l'adopte.
- Après une rotation de `current`, un nouveau snapshot est possible ; les
  anciens restent intacts.

### 4.7 Sortie et codes (ADR 0008)

Codes de sortie : même sens qu'ADR 0007. Ligne d'état du coffre en dernière
ligne de stderr avec `--apply` ou `--recover`. États proposés, fixés par
ADR 0008 : `not modified` ; `metadata only; nothing staged or published` ;
`snapshot <snap-id> kept incomplete` ; `published snapshot <snap-id>` ;
`rolled back snapshot <snap-id>` ; `intervention required for <snap-id>:
<cause>`.

Plan, forme indicative avec des identifiants fictifs :

```text
game: synthetic-game
vault: F:\Vault
current: 20261105T101500-1a2b3c4d, recognized
snapshots: 1 published, 0 incomplete
  20261101T090000-0a1b2c3d  from 20261030T120000-11223344  2 files  6 bytes
snapshot: new, from 20261105T101500-1a2b3c4d
files: 2  bytes: 6  space needed: 67108870 bytes
findings: none
```

Codes stables :

| Code | Sévérité | Sens |
| --- | --- | --- |
| `current-modified`, `insufficient-space` | bloquante | Repris de `sync` |
| `no-current` | bloquante | Aucune génération publiée : rien à copier |
| `verification-mismatch` | bloquante, `--apply` | Repris de `sync` |
| `long-path` | information | Repris de `sync`, mesuré sur `I` |
| `snapshot-missing`, `snapshot-modified` | information | S13 |

Causes d'intervention reprises : `pending-operation`, `unexpected-entry`,
`journal-ambiguous`. Verrou occupé : message neutre, « another operation is
running on this game », pour les deux commandes.

### 4.8 Compatibilité avec `sync` et les binaires B2

- `sync` accepte l'entrée `snapshots` de `games\<game-id>` si c'est un dossier
  ordinaire, sans point d'analyse ; un fichier ou un lien sous ce nom reste
  `unexpected-entry`, code 3. Il ne lit **rien** sous `snapshots\` : ni
  grammaire, ni receipts, ni journaux.
- Les journaux de snapshot n'entrent jamais dans les ensembles de `sync` :
  explication de `staging\`, règle M3, receipt de tête, opérations en attente.
  `snapshot` lit l'état de `current` par le lecteur de `sync`, en gardant ses
  propres journaux à part.
- Sur un coffre sans `snapshots`, la sortie de `sync` reste identique octet
  pour octet.
- Un binaire antérieur à B3 refuse en code 3, `unexpected-entry`, sans rien
  écrire, un coffre dès que `snapshots\` existe, même vide, c'est-à-dire dès
  qu'un `snapshot --apply` a passé son second contrôle d'espace.
  Fail-closed, documenté.

---

## 5. Protocole : amendement proposé d'ADR 0006

### 5.1 Notation

`P` : `snapshots\<snap-id>` ; `I` : `snapshots\<snap-id>.incomplete`. `new` est
l'identité de `I` relevée dans `staged` ; `—` un nom absent ; toute autre
identité, `other`, est une ambiguïté.

### 5.2 Automate

Journal `snapshots\journal\<snap-id>\`, même format d'enregistrement que le
§3 d'ADR 0006, mêmes noms d'étapes que la première publication ; le lecteur
applique l'automate de snapshot à ce dossier, et à lui seul.

| # | Enregistrement validé | Champs (`fields`) | Action qui suit |
| --- | --- | --- | --- |
| 1 | `planned` | `source_operation`, `source_receipt_sha256`, `source_identity` (identité de `current`), `plan`, `plan_sha256` | — |
| 2 | `staging-intent` | `staging` : `snapshots/<snap-id>.incomplete` | Créer `I`, copier, flusher |
| 3 | `staged` | `staging_identity`, `copied_sha256` | Relecture de `I` |
| 4 | `verified` | `verified_sha256` | — |
| 5 | `publish-intent` | `staging_identity` ; `P` absent attendu | Bascule `I` → `P` ; receipt de snapshot |
| 6 | `published` | `receipt_sha256` | Terminal |

`source_identity` porte un nom distinct de `current_identity` : le lecteur de
`sync` donne à ce dernier le sens de l'ancien `current` d'une rotation.
Rollback : `unpublish-intent` (`P` → `I`), puis `rolled-back` ;
`rollback-failed` au plus une fois (E3). Terminaux : `published`, `failed`,
`abandoned`, `rolled-back`. `copied_sha256` et `verified_sha256` sont égaux à
l'empreinte de contenu du receipt source.

### 5.3 Bascule et points d'arrêt

- Ordre de la bascule d'ADR 0006 §4 : fermer toute portée ouverte sur `I`,
  `P` et `current` ; ne garder épinglés que les ancêtres de
  `snapshots\journal\<snap-id>` et ce dossier ; pré-contrôle de `I`, nom `P`
  absent ; `MoveFileEx` sans drapeau ; post-contrôle : `P` de l'identité
  attendue, `I` absent.
- Points d'arrêt : ceux de la première publication, A1 à A8, avec `P` et `I`
  à la place de `current` et `staging\<op>`. En A3 et A4, `P` est revérifié en
  entier contre le plan, sans relire `current`. En A5 et A6, un receipt de
  snapshot déjà validé interdit tout retour en arrière.

### 5.4 Récupération `--recover --apply`

| Dernier validé | Observation (`P`, `I`) | Action |
| --- | --- | --- |
| Aucun : dossier d'opération vide ou temporaires `001` seuls | (—, —) | `001-abandoned` ; sinon code 3, rien écrit |
| `planned` | (—, —) | `abandoned` |
| `staging-intent` | (—, —), ou (—, dossier ordinaire sans point d'analyse) | `abandoned` ; `I` laissé en place, ni lu ni déplacé |
| `staged`, `verified` | (—, `new`) | `abandoned` ; `I` conservé |
| `publish-intent` | (—, `new`), aucun receipt de snapshot validé | Renommage non effectué : `rolled-back` |
| `publish-intent` | (`new`, —), aucun receipt de snapshot validé | Renommage effectué : revérification intégrale de `P` contre le plan ; égal : receipt puis `published` ; différent : `unpublish-intent`, puis `P` → `I` |
| `publish-intent` | (`new`, —), receipt de snapshot validé | Receipt relu, égal au plan, et `P` revérifié en entier : `published` ; sinon code 3, rien écrit ni déplacé (A5, A6) |
| `unpublish-intent` | (`new`, —) | Déplacement non effectué : l'exécuter, puis `rolled-back` ; échec : `rollback-failed`, code 3 |
| `unpublish-intent` ou `rollback-failed` | (—, `new`) | `rolled-back` |
| `rollback-failed` | (`new`, —) | Nouvel essai ; succès : `rolled-back` ; échec : code 3, sans nouvel enregistrement (E3) |
| `published` | (`new`, —), receipt conforme | Rien |
| Tout autre cas, dont `P` et `I` présents ensemble, une identité `other`, un receipt de snapshot validé sans bascule observée | — | Fail-closed, code 3, rien écrit |

Les amendements ratifiés le 30 octobre 2026 s'appliquent tels quels :
pré-contrôle illisible tenu pour une ambiguïté, identité relue après chaque
contenu, receipt validé sans bascule observée refusé. Une opération terminale
`published` dont `P` manque ou a changé relève de S13, pas de ce tableau.

### 5.5 Enveloppe

E1, E3 et E4 inchangées. E2 : au plus 11 enregistrements ; le plus long chemin
d'un snapshot en compte 8 (`planned` à `publish-intent`, puis
`unpublish-intent`, `rollback-failed`, `rolled-back`).

### 5.6 Exclusivité et domaines (S9, S13)

- **Verrou :** un seul par jeu, `sync.lock`, pour les deux commandes.
- **I8 par domaine**, amendement proposé : le domaine de `current` est
  `journal\`, celui des snapshots `snapshots\journal\`. Deux opérations non
  terminales d'un même domaine sont une ambiguïté, code 3. Une opération non
  terminale de `current` bloque `sync --apply` et `snapshot --apply` ; une
  opération de snapshot non terminale ne bloque que `snapshot --apply`. Une
  opération non terminale dans chaque domaine est un état admis.
- **Récupération :** chaque `--recover` ne récupère que son domaine et nomme
  l'autre commande pour l'autre. La récupération d'un snapshot ne lit jamais
  `current` ; celle de `current` ne lit rien sous `snapshots\`.
- **Exception d'ADR 0006 §3 pour S13 :** une opération de snapshot terminale en
  `published` dont `P` est absent ou d'une autre identité n'est pas une
  ambiguïté ; elle est signalée et rien n'est écrit pour elle. Une opération
  de snapshot non terminale reste soumise à toutes les ambiguïtés du §3.

### 5.7 Ce que la publication d'un snapshot ne fait jamais

Écrire, renommer ou supprimer hors de `snapshots\` ; créer `sync.lock` ;
déplacer `P` après `published` ; remplacer une cible ; copier vers un nom
publié ; lire un objet dont l'identité n'est pas celle attendue ; lire
`current` pendant une récupération ; supprimer un temporaire ou un
`.incomplete` ; recréer ou réparer un snapshot approuvé.

---

## 6. Immutabilité et garde de lecture seule (S12)

Ce qui garantit qu'un snapshot approuvé n'est jamais réécrit :

1. aucune API ne peut ouvrir un fichier existant en écriture : `NewFile` est
   `CREATE_NEW`, le garde d'ADR 0005 l'impose ;
2. `Move` ne remplace jamais une cible, et aucun chemin du code ne nomme `P`
   comme source d'un déplacement après `published`, propriété testée sur la
   trace des appels au faux coffre ;
3. un identifiant n'est jamais réutilisé : création exclusive de `I` ; nom `P`
   absent exigé avant la bascule ; après elle, `P` de l'identité attendue et
   `I` absent ;
4. aucune commande ne supprime ; `CreateHardLink` et `SetFileAttributes`
   restent interdits partout.

Limite déclarée : rien n'empêche un autre programme, ou l'utilisateur, de
modifier ou de supprimer un snapshot. Le plan de `snapshot` contrôle la
présence et l'identité de chaque snapshot publié, et relit en entier celui qui
le rendrait `up to date` ; la vérification du contenu de tous les snapshots
relève de `doctor` (B5).

`readonly_test.go`, `architecture_test.go` et `platform/vault` restent
inchangés.

---

## 7. Périmètre des fichiers

| Zone | À créer | À modifier |
| --- | --- | --- |
| Plan et ADR | `docs/adr/0008-snapshot-contract-receipt-v1.md` | Ce plan, version validée ; ADR 0004, 0006, 0007 |
| `internal/entities/backup` | Receipt de snapshot v1, sa relecture stricte et ses tests | Contrat des champs et automate du mode snapshot ; classement des observations du §5.4 |
| `internal/features/sync` | `snapshot.go`, `snapshot_apply.go`, `snapshot_recover.go` et leurs tests | Moteur paramétré par la disposition et les codes ; lecture de l'entrée `snapshots` ; I8 par domaine ; documentation du package ; faux coffre |
| `internal/cli` | `snapshot.go` et tests | `root.go`, texte de la commande racine et câblage ; message du verrou occupé |
| `internal/app` | — | `app.go` ; `app_test.go`, exemple de commande inconnue |
| `cmd/savewarden` | Tests binaires Windows du snapshot | `main_test.go`, exemples README |
| `docs/` | `commands/snapshot.md` ; page du receipt de snapshot ; `validation/b3.md` | `vault.md`, guide de récupération compris ; `safety-model.md` ; `architecture.md` ; `pilots/odyssey.md` |
| Racine | — | `README.md`, `CHANGELOG.md`, `AGENTS.md` (lot courant) |

Changements interdits :

- toute modification de `internal/platform/vault`,
  `internal/app/readonly_test.go`, `internal/app/architecture_test.go`,
  ADR 0005, `go.mod` et `go.sum` ;
- toute API d'écriture, de suppression ou de remplacement nouvelle ; toute
  écriture de `snapshot` hors de `snapshots\` ;
- les packages `features/snapshot`, `restore`, `doctor`, et tout dossier vide ;
- toute modification du schéma de manifeste v1, du receipt v1, du marqueur et
  du format d'enregistrement du journal ;
- tout changement de comportement de `sync` sur un coffre sans `snapshots` ;
- tout chemin personnel ou propre au pilote dans le code, les tests ou les
  exemples ; toute fixture versionnée dans un dossier nommé `snapshots/` ;
- toute modification des rulesets, des versions d'Actions ou de la CI hors
  ajout de contrôles ;
- tout accès agent à `D:\Ga`, `D:\Savewarden`, `E:\GMS` ou aux clés SSH ;
- tout test lisant ou écrivant hors de `t.TempDir()`.

---

## 8. Plan d'exécution (S11 proposé)

Avant tout code : ce plan validé, puis ses amendements d'ADR rédigés et
revus sur `docs/b3-framing`, comme le cadrage de B2c. La PR
`feat/b3-snapshot` part de ces commits de cadrage.

| # | Commit | Point de contrôle |
| --- | --- | --- |
| 0 | Plan validé ; ADR 0006, 0004 et 0007 amendés ; ADR 0008 | **Revue du cadrage** par le propriétaire |
| 1 | `entities/backup` : receipt de snapshot, automate et champs, classement, tests purs | — |
| 2 | Moteur de `features/sync` paramétré, sans changement de comportement : suite B2 et sortie de `sync` inchangées | **Revue indépendante de non-régression** |
| 3 | `sync` accepte l'entrée `snapshots` ; I8 par domaine | — |
| 4 | Plan de snapshot en lecture seule ; CLI `snapshot` sans `--apply` | — |
| 5 | `--apply` : structure, automate, copie, vérification, bascule, receipt ; énumération des crashs | — |
| 6 | `--recover` ; énumération des crashs pendant la récupération | **Revue indépendante du protocole** |
| 7 | Tests Windows réels et binaires ; documentation | — |
| 8 | Rapport `docs/validation/b3.md` | **Revue de sécurité indépendante**, avant le GO de fusion |

Chaque commit passe la suite complète à son SHA. Push, PR, fusion et pilote
attendent chacun un GO distinct du propriétaire.

---

## 9. Matrice de tests

Noms proposés, à confirmer en v0.2. Faux coffre sur les trois OS sauf mention.

**Entités**

| Test | Preuve |
| --- | --- |
| `TestSnapshotReceiptRoundTrip`, `TestSnapshotReceiptIsStrict` | Forme canonique ; champs inconnus, manquants, en double, dates, reconstruction refusés |
| `TestSnapshotStepFieldsAreStrict`, `TestSnapshotAutomaton` | Contrat de chaque étape ; transitions permises seulement ; étapes et champs de rotation refusés en mode snapshot |
| `TestSnapshotRecoveryRows` (pur) | Chaque ligne du §5.4, exception A5 et A6 et défaut fail-closed compris |

**Non-régression et compatibilité de `sync`**

| Test | Preuve |
| --- | --- |
| Suite B2 complète, inchangée | Moteur paramétré sans changement de comportement, énumérations de crash comprises |
| `TestSyncOutputUnchangedWithoutSnapshots` | Sortie identique octet pour octet sur un coffre B2 |
| `TestSyncAcceptsSnapshotsDirectory` | Plan, `--apply`, rotation et `--recover` inchangés à côté de snapshots |
| `TestSnapshotsAsFileOrLinkIsUnexpectedForSync` | Fichier ou lien nommé `snapshots` : code 3 |
| `TestSnapshotAnomaliesNeverBlockSync` | Journal de snapshot ambigu, entrée hors grammaire, opération de snapshot en attente : `sync` plan, `--apply`, rotation et `--recover` inchangés |
| `TestPendingCurrentOperationBlocksSnapshot` | Opération de `current` non terminale : `snapshot --apply` refusé, code 3 |
| `TestSnapshotJournalsStayOutOfSyncSets` | Un `staging\<snap-id>` reste inexpliqué ; la règle M3 ignore un snapshot publié |
| `TestRotationLeavesSnapshotsUntouched` | Empreinte de `snapshots\` inchangée par une rotation et sa récupération |

**Snapshot**

| Test | Preuve |
| --- | --- |
| `TestSnapshotPlanWritesNothing`, `TestSnapshotRecoverPlanWritesNothing` | Aucun `Vaults` reçu ; empreinte du coffre inchangée |
| `TestSnapshotRequiresRecognizedCurrent`, `TestSnapshotWithoutCurrentIsBlocked` | `current-modified`, `no-current`, coffre copié en code 3 |
| `TestSnapshotNeverCreatesTheLock` | `sync.lock` absent : code 3, rien écrit |
| `TestSnapshotChecksDistinction` | Coffre dans l'installation ou une source, par nom et par identité : code 1 |
| `TestSnapshotCopiesCurrentExactly` | Entrées, tailles, dates, SHA-256, dossiers ; `sources` du receipt égales à celles du receipt source ; empreintes égales |
| `TestSnapshotWritesOnlyUnderSnapshots` | Trace des appels : toute écriture sous `snapshots\` ; empreinte du reste du coffre inchangée |
| `TestSnapshotIsIdempotentForTheSameGeneration`, `TestNewSnapshotAfterRotation` | S7 ; deux snapshots distincts, tous deux intacts |
| `TestMissingOrAlteredSnapshotIsReported` | S13 : information, rien écrit pour lui ; un nouveau snapshot de la même génération est possible |
| `TestIncompleteBeforeStagedIsExplainedByName` | `I` sans identité journalisée, opération terminale : accepté, jamais lu ni déplacé |
| `TestSnapshotOfCurrentChangedDuringCopy` | Contenu : `current-modified`, `failed` ; identité : code 3 ; `I` conservé, rien publié |
| `TestSnapshotIDCollisionIsRefused` | Identifiant déjà employé dans l'un ou l'autre domaine : code 1, avant tout journal |
| `TestSnapshotStructureCreationIsResumable` | Crash entre les créations de structure : état reconnu, complété au `--apply` suivant |
| `TestSnapshotSpaceIsCheckedBeforeAnyWrite`, `TestSnapshotLongPathIsReported` | Espace insuffisant sous verrou : aucune structure créée ; chemins longs mesurés sur `I` |
| `TestCrashAtEverySnapshotCall` | Crash avant et après chaque appel au coffre, état final exact par point |
| `TestSnapshotRecoveryIsInterruptible`, `TestRepeatedSnapshotRecoveryIsStable` | Propriétés du §9 d'ADR 0006 ; trois exécutions sur une ambiguïté : coffre identique octet pour octet, diagnostic identique |
| `TestPublishedSnapshotIsNeverMovedOrReopened` | Après `published`, aucun appel ne vise `P` en déplacement ou création, à travers `snapshot`, `sync`, une rotation et chaque récupération |
| `TestSnapshotEnvelopeIsBounded` | Dépassement de E1, E2 et E3 : code 3 sans mutation |

**Windows réel et binaire**

| Test | Preuve |
| --- | --- |
| `TestSnapshotEndToEnd` | `sync --apply`, `snapshot --apply`, `inspect` du snapshot égal à son receipt, second `--apply` `not modified`, plan de `sync` `up to date` |
| `TestSnapshotPlanUnderDeniedWrites`, `TestSnapshotRecoverPlanUnderDeniedWrites` | Plan et plan de récupération sous ACL de refus d'écriture : rien écrit |
| `TestSnapshotRenameRefusedByAnOpenFile` | Fichier ouvert dans `I` : erreur 5, `rolled back`, code 1 |
| `TestSnapshotKilledDuringCopyThenRecovered` | Processus tué pendant la copie ; récupération : `abandoned`, `I` conservé |

**CLI** : aide, flags, `--recover` sans copie, codes documentés
(`TestStableCodesAreDocumented` étendu), exemples README.

Vérification manuelle consignée au rapport, comme en B2c : un binaire
construit depuis `56cada2` refuse en code 3 un coffre synthétique qui contient
`snapshots\`, sans rien écrire.

---

## 10. Documentation livrée avec le code

`docs/commands/snapshot.md` ; `docs/vault.md`, disposition, anomalies S13 et
guide de récupération du snapshot ; page du receipt de snapshot ; modèle de
sécurité, invariant 7 implémenté ; architecture ; README et ses exemples ;
CHANGELOG ; ADR ; `AGENTS.md`, lot courant.

---

## 11. Rollout, rollback et pilote

- Le code ne rejoint `release/v0.1.0-a1` que par PR ; aucun tag ni release.
- **Rollback du code** : revert de la PR. Il n'est pas neutre pour les coffres
  déjà munis de `snapshots\` : `sync` les refuse alors en code 3, sans rien
  écrire, jusqu'à un binaire B3 ou postérieur. Rien n'y est perdu : les
  snapshots restent lisibles par `inspect`, avec un manifeste dont la source
  pointe sur `snapshots\<snap-id>\<source-id>`. Les coffres sans
  `snapshots\` ne sont pas affectés.

**Pilote B3 (S14)**, exécuté par le propriétaire après fusion, avec un binaire
du commit de fusion, dans un **coffre neuf**, distinct de celui des pilotes B2,
qui reste intact :

1. `sync --apply` vers le coffre neuf : code `0`, `published <op>` ;
   facultatif : sauvegarder en jeu, puis second `sync --apply`, pour un coffre
   qui a connu une rotation.
2. `snapshot`, plan seul : code `0`, `current` reconnu, nouveau snapshot de 14
   fichiers attendu, aucune anomalie ; coffre inchangé.
3. `snapshot --apply` : code `0`, `published snapshot <snap-id>`.
4. `inspect` d'un manifeste écrit hors du dépôt qui pointe sur le snapshot :
   fichiers, tailles et SHA-256 égaux à son receipt et au receipt de `current`.
5. Plan suivant `up to date`, puis second `snapshot --apply` : `not
   modified`, empreinte du coffre inchangée.
6. `sync`, plan seul : code `0`, `up to date` ; empreinte de tout le coffre
   hors `snapshots\` inchangée depuis l'étape 2.
7. Consigner codes, nombres et verdicts, sans hash individuel, identifiant
   d'opération, receipt réel, nom de profil ni chemin personnel.

---

## 12. Critères d'acceptation

1. Le mode plan et `--recover` sans `--apply` n'écrivent rien : binaire sous
   ACL de refus, empreinte complète inchangée.
2. ADR 0006, 0004 et 0007 amendés et ADR 0008 accepté avant tout code
   d'écriture ; ADR 0005, `readonly_test.go`, `architecture_test.go` et
   `platform/vault` inchangés.
3. Un snapshot publié est égal, fichier par fichier, au receipt de la
   génération source, relu depuis le disque avant publication ; son receipt
   le prouve par `source.receipt_sha256` et l'empreinte de contenu.
4. `snapshot` n'écrit que sous `snapshots\` et ne crée jamais `sync.lock` ;
   `current`, `retained\`, `staging\`, `receipts\` et `journal\` restent
   identiques octet pour octet ; le contenu de la source n'est jamais lu.
5. Un snapshot approuvé n'est jamais rouvert, déplacé, recréé ni supprimé, à
   travers toutes les commandes et récupérations.
6. Chaque point d'arrêt et chaque ligne du §5.4 sont atteints par un test,
   exception A5 et A6 comprise ; la récupération respecte le §9 d'ADR 0006
   dans l'enveloppe E1 à E4.
7. Un verrou unique par jeu ; I8 par domaine ; aucune anomalie de
   `snapshots\` ne bloque `sync`, récupération de `current` comprise.
8. La suite B2 passe inchangée ; la sortie de `sync` est identique octet pour
   octet sur un coffre sans snapshot ; `sync` accepte un coffre avec
   snapshots ; un binaire antérieur le refuse sans écrire.
9. Idempotence par génération ; snapshots successifs intacts après rotation ;
   anomalies S13 signalées sans blocage ni écriture.
10. Revues indépendantes consignées : non-régression, protocole, sécurité.
11. 7 checks requis verts sur le SHA exact, avec lien du run.
12. Documentation, guide de récupération, CHANGELOG et ADR cohérents ;
    exemples README vérifiés par test.
13. Aucun accès agent aux données protégées ; aucun chemin personnel, save ou
    receipt réel dans le diff.
14. Pilote B3 exécuté et qualifié par le propriétaire, ou absence déclarée.

---

## 13. Décisions reportées hors de B3

| Sujet | Comportement en B3 |
| --- | --- |
| Rétention, suppression ou nettoyage de snapshots et de `.incomplete` | Aucune suppression ; accumulation documentée ; le plan en affiche le nombre et la taille |
| Snapshot d'une génération `retained\` | Non proposé |
| Libellés, notes | Absents ; l'identifiant et la provenance seuls nomment un snapshot |
| Vérification du contenu de tous les snapshots publiés | `doctor`, B5 ; le plan de `snapshot` n'en contrôle que la présence et l'identité, et relit en entier le seul snapshot qui le rendrait `up to date` |
| Réparation d'un snapshot approuvé absent ou altéré | Jamais ; signalé (S13) |
| Attribut lecture seule, ACL | Refusés par le garde ; immutabilité par protocole |
| Extraction du moteur de publication | Réexaminée au cadrage de B4 (§3.1) |
| Portabilité d'un coffre copié ou déplacé | Inchangée : code 3, sans rien écrire |
| Import de snapshots créés hors de Savewarden | Jamais adoptés |
| ReFS | Toujours non qualifié |

---

## 14. Risques

| Risque | Effet | Parade |
| --- | --- | --- |
| Régression de `sync` par le paramétrage du moteur | Perte d'une garantie qualifiée de B2 | Commit de refactor distinct, suite B2 et énumérations inchangées, sortie identique, revue de non-régression |
| Anomalie de snapshot qui bloquerait `current` | Protection de l'état courant suspendue | I8 par domaine (S9) ; `sync` ne lit rien sous `snapshots\` ; test dédié |
| Binaire antérieur, ou revert, sur un coffre avec snapshots | Utilisateur bloqué en code 3 | Fail-closed sans écriture ; documenté ; pilote dans un coffre neuf (S14) |
| Receipt ou journal de snapshot lu par `sync` comme le sien | Faux `current-modified`, staging expliqué à tort, règle M3 faussée | Rangement séparé sous `snapshots\` (S3), ensembles distincts, tests |
| `current` modifié pendant la copie | Snapshot faux | Relecture intégrale avant, comparaison de la copie au receipt, identité relue après |
| Crash entre bascule et receipt | Snapshot présent, non approuvé | Points d'arrêt, revérification intégrale, rollback ; aucun retour en arrière après receipt validé |
| Snapshot approuvé supprimé ou modifié à la main | Blocage définitif si fail-closed | Signalé sans blocage (S13) ; jamais réparé |
| Accumulation de snapshots et de `.incomplete` | Disque plein | Préflight d'espace avant toute écriture, idempotence par génération, nombre et taille affichés, documentation |
| Antivirus ou indexeur tenant `I` | Bascule refusée | Retries 32 et 33, erreur 5 immédiate, rollback |
| Chemins plus longs que sous `current` | Outils tiers en échec | `long-path` mesuré sur `I` ; `\\?\` dans Savewarden |
| Confusion entre `retained\` et snapshots | Mauvaise attente de l'utilisateur | Documentation : seuls les snapshots sont approuvés et nommés |
| Package `sync` trop large | Lisibilité | Fichiers par opération, réexamen en B4 |
| Extension vers B4 | Lot trop gros | Non-objectifs §2.2 |
| Mainteneur unique | Revue limitée | Revues indépendantes, Human GO, aucune auto-approbation |

---

## 15. Rapport de restitution attendu

Branche et PR, SHA final, `git diff --stat`, gates et résultats exacts, run CI
et ses 7 checks, ADR créés ou amendés, énumérations de crash et tests de
non-mutation, revues indépendantes, résultat du pilote ou son absence, écarts
au plan, risques résiduels, questions ouvertes, recommandation **sans
auto-approbation**.

## Historique

| Version | Date | Commit | Changements |
| --- | --- | --- | --- |
| 0.1 | 30 octobre 2026 | local, branche `docs/b3-framing` | Brouillon de cadrage : décisions S1 à S14 proposées, protocole de snapshot proposé comme amendement d'ADR 0006, matrice de tests, critères. Revue indépendante en lecture seule intégrée avant remise : explication par nom ou identité, exception A5 et A6, I8 par domaine au lieu de l'union, ensembles de journaux séparés, coût réel de l'option (a), anomalies S13, pilote dans un coffre neuf, verrou jamais créé, structure après le contrôle d'espace, `source_identity`, changements interdits. Aucun GO |

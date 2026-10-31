# Savewarden — Plan B3 : snapshots immuables

> **Statut : v0.2 — VALIDÉ PAR LE PROPRIÉTAIRE le 31 octobre 2026 : arbitrages S1 à S14, précisions Q1 à Q4 et revue indépendante intégrés ; ADR 0004, 0006 §10, 0007 et 0008 acceptés ; GO de développement B3 donné le 31 octobre 2026 ; push, PR, fusion et pilote soumis à des GO distincts**
> Version du plan : `0.2` — 30 octobre 2026 ; remplace le brouillon v0.1 (`5089378`)
> Lot : **B3**, cible `release/v0.1.0-a1`
> Cadre : [plan directeur v0.1](Savewarden_Plan_Bootstrap_Architecture_GitOps_v0.1.md) §7 B3,
> [plan B2 v0.2](Savewarden_Plan_B2_Synchronisation_v0.2.md), [AGENTS.md](../AGENTS.md),
> [modèle de sécurité](../docs/safety-model.md), ADR [0001](../docs/adr/0001-fsd-inspired-go.md),
> [0004](../docs/adr/0004-sync-boundaries.md) à [0007](../docs/adr/0007-sync-contract-receipt-v1.md),
> [ADR 0008](../docs/adr/0008-snapshot-contract-receipt-v1.md).

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
accord explicite.

ADR 0006 reste la **référence de sécurité** : son §1 inclut déjà les
snapshots de B3. Son amendement, le §10, porte le protocole des
snapshots ; il nomme comme tels les deux assouplissements retenus, I8 par
domaine (S9) et le traitement des anomalies d'un snapshot déjà approuvé
(S13).

Ce plan ne vaut ni GO de développement, ni autorisation de push, de tag ou de
release, ni accès aux sauvegardes réelles.

---

## 0. Arbitrages

### 0.1 Décisions S1 à S14

Revue CTO du brouillon v0.1, transmise par le propriétaire le 30 octobre 2026 :
architecture favorable, **S1 à S14 retenues dans leur principe**, sans défaut
bloquant ; quatre précisions demandées avant de figer les ADR (§0.2). Après
une revue indépendante de la v0.2, le propriétaire valide cette v0.2 et
accepte les ADR le 31 octobre 2026. Cette validation ne vaut pas GO de
développement.

| # | Décision retenue | Précision de la revue | Où |
| --- | --- | --- | --- |
| S1 | Le snapshot copie la génération **`current` du coffre**, reconnue par toute sa chaîne de preuve et relue en entier contre son receipt sous verrou | C'est un snapshot de l'état **sauvegardé dans le coffre**, pas nécessairement de l'état actuel du jeu | §2.1, §4.3 |
| S2 | Le code vit **dans `features/sync`**, seconde opération du même moteur ; commande CLI `snapshot` distincte ; extraction réexaminée en B4 | — | §3.1 |
| S3 | **Tout sous `games\<game-id>\snapshots\`** ; `sync` accepte cette entrée et ne lit rien dedans | Isolation indispensable des receipts et journaux | §4.2, §4.9 |
| S4 | Identifiant = **celui de l'opération de snapshot**, distinct de tout identifiant des deux domaines | — | §4.2 |
| S5 | Staging **`snapshots\<snap-id>.incomplete\`**, jamais supprimé ; nombre affiché par le plan, sans parcourir ces dossiers ; taille non mesurée | — | §4.2, §4.7 |
| S6 | Un snapshot est **approuvé à sa publication** | `snapshot --apply` est l'acte explicite de création et d'approbation | §4.7 |
| S7 | **Un snapshot conforme par génération source** | Plusieurs générations, donc plusieurs snapshots, restent possibles | §4.7 |
| S8 | **Nouveau receipt strict `savewarden.snapshot` v1** | — | §4.6 |
| S9 | **Verrou unique par jeu, I8 par domaine** | La récupération d'un snapshot reste possible sans `current` (Q1) | §4.4, §5.6 |
| S10 | `snapshot --manifest --vault [--apply] [--recover]` ; le plan liste les snapshots ; ni libellé, ni `list` | — | §4.1 |
| S11 | **Une PR** `feat/b3-snapshot`, sur le modèle de B2c | Revues intermédiaires **obligatoires** après le refactor du moteur et après le protocole | §8 |
| S12 | **Immutabilité par protocole** | Garantie protocolaire, **pas matérielle de type WORM** (Q4) | §6 |
| S13 | Snapshot approuvé absent, remplacé ou altéré : **signalé, jamais bloquant ni réparé** | Dégradation clairement visible, jamais assimilée à un état sain ; pas de vérification intégrale de tous les snapshots en B3 (Q3) | §4.8, §5.6 |
| S14 | Pilote dans un **coffre neuf** | Protège les preuves et la compatibilité des pilotes B2 | §11 |

Écartés par la revue, et donc hors de B3 : toute API d'écriture nouvelle, les
ACL et attributs lecture seule, une nouvelle séparation de packages, toute
réparation automatique.

### 0.2 Précisions Q1 à Q4

| # | Précision demandée | Intégration |
| --- | --- | --- |
| Q1 | **Récupération indépendante** : `snapshot --recover --apply` ne dépend ni de la présence ni de l'intégrité actuelle de `current`, et une opération B2 en attente ne la bloque pas ; elle ne s'appuie que sur ses preuves et son journal, sous le verrou partagé ; test croisé | Préconditions de création et de récupération séparées (§4.3, §4.4) ; ADR 0006 §10 ; `TestSnapshotAndCurrentRecoveriesAreIndependent` (§9) |
| Q2 | **Provenance en récupération** : relation entre le receipt source du domaine B2, le plan journalisé et le receipt de snapshot, même après rotation, sans lire le contenu actuel de `current` ; comportement explicite d'une preuve indisponible ou contradictoire | Porte de provenance avant toute approbation, cause `source-unverified` ; preuve indisponible ou contradictoire distinguée ; rollback sur preuve contradictoire ; porte non rejouée après un receipt de snapshot validé (§4.5) ; ADR 0006 §10, ADR 0008 |
| Q3 | **S13** : `snapshot-missing` et `snapshot-modified` sont des diagnostics d'intégrité visibles, jamais assimilés à un coffre sain ; B3 ne vérifie pas intégralement tous les anciens snapshots | État des snapshots, ligne `degraded`, avertissement sur stderr, portée du contrôle écrite (§4.8) ; ADR 0008 |
| Q4 | **Immutabilité** : jamais rouvert *en écriture*, déplacé, remplacé ni supprimé après publication ; lectures de contrôle permises ; garantie protocolaire, non WORM | §2.1, §6, critère 5 |

### 0.3 Écarts à la v0.1

- **Message du verrou occupé.** La v0.1 proposait un message commun aux deux
  commandes. Il changerait la sortie de `sync` sur un coffre sans snapshot, ce
  que le §7 interdit : `sync` garde « another sync is running on this game » ;
  `snapshot` emploie « another operation is running on this game ». ADR 0007
  note que le verrou peut désormais être tenu par un snapshot.
- **Création du verrou.** La v0.1 interdisait à `snapshot` de créer
  `sync.lock`. Or `sync` refuse d'agir sur un domaine de `current` ambigu
  avant de prendre le verrou : un verrou absent rendrait alors la récupération
  d'un snapshot dépendante de ce domaine, contre Q1. `snapshot` prend donc le
  verrou par la même primitive que `sync`, qui le crée vide s'il manque ; il
  ne l'écrit ni ne le supprime jamais. C'est sa seule écriture possible hors
  de `snapshots\`.
- **Revue croisée du contrat de récupération**, intégrée : rollback sur
  preuve de provenance contradictoire, au lieu d'un blocage définitif ;
  receipt de snapshot validé tenu pour preuve, sans rejouer la porte ;
  comparaison du receipt de snapshot traitée comme une observation ;
  renommage perdu signalé à part ; grammaire des sous-dossiers ; limite de la
  porte, qui vérifie une cohérence sans authentifier.

---

## 1. Contexte et preuves

| Élément | État vérifié le 30 octobre 2026 |
| --- | --- |
| `release/v0.1.0-a1` | `56cada2`, merge de la PR #10, décisions de clôture de B2c |
| CI | Run 41132916039 sur la tête de la PR #10 et run post-fusion 41133566052 sur `56cada2` : terminés, `success`, 7 checks chacun |
| B2 | Clos : B2a, B2b, B2c fusionnés ; pilotes B2b et B2c exécutés et qualifiés par le propriétaire ; amendements d'ADR 0006 postérieurs au cadrage B2c ratifiés le 30 octobre 2026 |
| Cadrage B3 | Brouillon v0.1 en `5089378`, SHA-256 `5e87d108…06a5b`, branche locale `docs/b3-framing` ; revu par le CTO le 30 octobre 2026 |
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
  récupération de `current`. D'où S9 et Q1.
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
- **Sévérités.** `entities/backup` n'en connaît que deux, `Info` et
  `Blocking`, partagées avec `inspect` ; B3 n'en ajoute pas (§4.8).
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
  build qui n'écrit rien ; `.gitignore` ignore tout dossier `snapshots/`, ce
  qui interdit ce nom à des fixtures versionnées.

---

## 2. Objectif, non-objectifs, permissions

### 2.1 Objectif

```text
savewarden snapshot --manifest <chemin> --vault <dossier> [--apply] [--recover]
```

1. Sans `--apply`, affiche un **plan** sans rien écrire : génération `current`
   reconnue, état des snapshots et leurs dégradations, `.incomplete`
   conservés, ce que `--apply` copierait, espace requis, opérations en
   attente.
2. Avec `--apply`, sous le verrou du jeu, relit `current` en entier contre son
   receipt, le copie dans `snapshots\<snap-id>.incomplete\`, relit la copie
   depuis le disque, la compare au receipt, recoupe la provenance, puis la
   publie par renommage en `snapshots\<snap-id>\` avec un receipt de snapshot.
   Le snapshot fige l'état **sauvegardé dans le coffre** : pour figer l'état
   actuel du jeu, l'utilisateur lance d'abord `sync --apply`.
3. Ne supprime, ne tronque, n'écrase et ne réécrit **rien** ; n'écrit que sous
   `snapshots\`, hormis le fichier de verrou vide que `Lock` crée s'il manque,
   comme pour `sync` ; ne lit le contenu ni de la source du jeu, ni de son
   installation, dont seules les identités servent au contrôle de distinction.
4. Après sa publication, un snapshot n'est **jamais rouvert en écriture,
   déplacé, remplacé ni supprimé**, par aucune commande ; les lectures de
   contrôle restent permises.
5. Une opération interrompue se récupère par `snapshot --recover`,
   indépendamment de l'état de `current`.

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
- API d'écriture nouvelle, attribut lecture seule, ACL, ReFS, coffre copié
  ou déplacé (§13).
- Support fonctionnel Linux et macOS, sortie JSON de la CLI.

### 2.3 Permissions : agent, produit, propriétaire

Inchangées depuis le plan B2 §2.3 :

| Acteur | Peut | Ne peut pas |
| --- | --- | --- |
| **Agent** | Lire et modifier le dépôt ; tests synthétiques en `t.TempDir()` | Explorer ou lire `D:\Ga`, `D:\Savewarden`, `E:\GMS` ou des clés SSH ; lancer Savewarden sur des données réelles sans autorisation explicite et nominative pour cette exécution |
| **Savewarden**, lancé par le propriétaire | Lire le manifeste désigné ; relever sans suivi l'identité de l'installation et des sources ; lire et écrire **seulement** dans le coffre passé à `--vault`, et pour `snapshot` n'écrire que sous `games\<game-id>\snapshots\`, hormis le verrou vide créé comme par `sync` | Lire le contenu de la source, écrire ailleurs, adopter, suivre un lien, supprimer |
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
| **(a) Retenue : opération de `features/sync`** | Fichiers `snapshot*.go` dans `features/sync` ; méthodes de service propres au snapshot ; commande CLI `snapshot` qui les appelle ; moteur paramétré par la disposition et les codes de l'opération, dans le même package | Aucune duplication ; aucun code déplacé ni frontière nouvelle ; verrou, lecture du coffre et reconnaissance de `current` en un seul endroit ; faux et énumérations de crash réutilisés tels quels | Refactor interne du moteur qualifié ; le nom `sync` couvre désormais toutes les publications du coffre d'un jeu ; le package grandit d'environ un tiers |
| (b) `features/snapshot` dupliqué | Copie du moteur et des faux | Package nommé comme la commande ; code de B2 intact | Deux copies du code d'écriture le plus sensible, qui divergeraient ; reconnaissance de `current` dupliquée ; ADR 0004 a rejeté la duplication du parcours pour cette raison |
| (c) Extraction du moteur | Nouveau package de protocole sous `internal/`, consommé par `sync` et `snapshot` | Prépare B4, qui publiera aussi selon ADR 0006 | Même refactor que (a), plus un déplacement de code, une frontière durable nouvelle, son ADR et ses tests d'imports, et des faux de test à déplacer ou dupliquer |

**Retenue : (a) en B3**, et réexamen de (c) au cadrage de B4, premier
consommateur réellement différent puisqu'il écrira hors du coffre.
Amendement d'ADR 0004 : `features/sync` possède toutes les publications dans
le coffre d'un jeu, `current` et snapshots ; aucune règle d'import ne change.

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

### 3.3 ADR

Rédigés sur `docs/b3-framing` avec cette v0.2 et **acceptés par le
propriétaire le 31 octobre 2026**. Aucun code ne les implémente encore. Ils
sont versés comme premiers commits de la PR, avant tout code d'écriture (§8).

| ADR | Objet |
| --- | --- |
| **0006, §10 nouveau** | Protocole des snapshots : disposition, domaines et I8 par domaine, structure, automate et contrat des champs, création, bascule, points d'arrêt, porte de provenance, récupération indépendante et son tableau, anomalies d'un snapshot approuvé, enveloppe ; renvois depuis I8 et le §3 |
| **0008, nouveau** | Contrat de `snapshot` : modes, préconditions de création et de récupération, sortie du plan, état des snapshots, codes 0 à 3, ligne d'état, codes stables et causes, receipt de snapshot v1 et sa relecture stricte |
| **0004, amendement** | `features/sync` possède les publications du coffre d'un jeu, snapshots compris ; la phrase qui annonçait une feature `snapshot` séparée est remplacée |
| **0007, amendement** | `sync` accepte `games\<game-id>\snapshots\` s'il s'agit d'un dossier ordinaire, sans rien y lire ; sortie inchangée sans snapshot ; verrou tenu par l'une ou l'autre commande |

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
- Le mode plan et `--recover` sans `--apply` ne reçoivent pas `Vaults` : ils
  ne peuvent rien écrire, prouvé par des tests binaires sous ACL de refus
  d'écriture.
- `--recover` exclut toute nouvelle copie ; il ne récupère que des opérations
  de snapshot et renvoie à `sync --recover` pour une opération de `current`.
- Windows seulement, comme `sync`.

### 4.2 Disposition du coffre (S3, S4, S5)

```text
<vault>\games\<game-id>\
  sync.lock, current\, retained\, staging\, receipts\, journal\   domaine de current (B2), inchangé
  snapshots\                                                      domaine des snapshots
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
- **Grammaire** du domaine, contrôlée par `snapshot` seul : dans
  `snapshots\`, `receipts`, `journal`, et des dossiers ordinaires `<snap-id>`
  ou `<snap-id>.incomplete` ; dans `snapshots\receipts\`, des fichiers
  `<snap-id>.json` et leurs temporaires `<snap-id>.json.tmp-<k>` ; dans
  `snapshots\journal\`, des dossiers `<snap-id>` au contenu du §3 d'ADR 0006.
  Toute autre entrée, un lien, un fichier à la place d'un dossier, un point
  d'analyse : `unexpected-entry`, code 3, pour `snapshot` seulement.
- **Explication**, comme ADR 0006 §3, par nom **ou** par identité : chaque
  dossier doit porter le nom d'une opération de snapshot journalisée ; son
  identité est exigée dès que `staged` l'a fixée. Un `<snap-id>.incomplete`
  dont le journal s'arrête avant `staged` est accepté s'il est un dossier
  ordinaire sans point d'analyse, jamais lu ni déplacé. Un `I` d'une opération
  close n'est ni relu ni exigé. Un snapshot approuvé absent ou d'une autre
  identité relève de S13 (§4.8).
- `snapshots`, `snapshots\receipts` et `snapshots\journal` sont des dossiers
  de **structure** : créés par `snapshot --apply` sous verrou, **après** le
  second contrôle d'espace et avant tout journal, par création exclusive, dans
  cet ordre et seulement s'ils manquent. Suit le dossier d'opération
  `snapshots\journal\<snap-id>`, puis `001-planned`. Une structure partielle,
  faite de dossiers de structure vides, ou un dossier d'opération vide, est
  un état reconnu : le premier est complété au `--apply` suivant, le second
  relève de la ligne « Aucun » du §5.4.
- Le verrou du jeu reste `sync.lock`, commun aux deux domaines. `snapshot` le
  prend par la même primitive que `sync`, qui le crée vide s'il manque ; il ne
  l'écrit, ne le tronque ni ne le supprime jamais (§0.3).

### 4.3 Création : préconditions et pipeline

Ces préconditions valent pour le plan et pour `--apply` **seulement**. Elles
ne s'appliquent pas à la récupération (§4.4).

| Phase | Écrit ? | Contenu | En cas d'échec |
| --- | --- | --- | --- |
| 1. Préflight | Non | Manifeste strict ; coffre existant, marqué, sans lien ; distinction du coffre, de l'installation et des sources par segments et par identité, sans lire leur contenu ; dossier du jeu présent ; domaine de `current` sans aucune intervention que `sync` relèverait, dont une opération non terminale, un `staging\` inexpliqué ou un coffre copié, sinon code 3 avec la même cause ; aucune opération de snapshot non terminale ; `current` reconnu par toute sa chaîne de preuve, égal à son receipt, identité relue après le contenu ; grammaire et explication de `snapshots\`, receipts et journaux de snapshot relus strictement ; volume et espace | Code 1, 2 ou 3, rien écrit |
| 2. Décision | Non | Snapshot approuvé de la génération `current`, présent sous son identité publiée, relu en entier conforme à son receipt, identité relue après : `up to date` (S7). Sinon, nouveau snapshot ; une dégradation est signalée (§4.8) | Code 0, rien écrit |
| 3. Verrou et structure | Structure | Verrou pris comme `sync` ; état relu, `vault-changed` s'il a changé depuis le plan ; espace recontrôlé ; identifiant tiré et contrôlé ; **puis** structure complétée et dossier d'opération créé | Avant la structure : code 1 ou 2, rien écrit ; après : au plus `metadata only` |
| 4. `planned` | Journal | Plan : génération source, SHA-256 de son receipt, identité de `current`, `sources` du receipt (§4.5) | — |
| 5. Copie | `.incomplete` | Juste avant : `current` relu en entier contre son receipt, identité relue après ; `staging-intent`, création de `I`, copie hachée de chaque fichier, date conservée ; identité de `current` relue après la copie | Contenu différent : `current-modified`, code 2, `failed` ; identité changée : code 3, rien enregistré de plus ; `I` conservé |
| 6. Vérification | Journal | Relecture de `I` depuis le disque, comparée au plan : chemins, tailles, dates, SHA-256, dossiers ; identité de `I` relue après le contenu | Contenu : `verification-mismatch`, `failed`, code 2 ; identité : code 3, rien enregistré de plus ; `I` conservé |
| 7. Publication | Renommage, receipt | Porte de provenance (§4.5) ; `publish-intent` ; bascule `I` → `P` selon l'ordre d'ADR 0006 §4 ; porte de provenance de nouveau ; receipt de snapshot ; `published` | Porte fermée avant `publish-intent` : `failed`, rien publié. Après la bascule : preuve contradictoire, rollback ; preuve indisponible, opération en attente, rien écrit. Code 3, `source-unverified`, dans ces trois cas. Bascule refusée : rollback, code 1 |

- **Espace :** taille complète de la génération source plus max(64 Mio, 5 %),
  disponible pour l'utilisateur appelant, au préflight puis sous verrou avant
  toute écriture, comme V2.
- **Chemins longs :** `long-path`, information, est mesuré sur le chemin le
  plus long écrit, celui de `I`, qui dépasse celui de `current` de
  38 caractères (`snapshots\`, `<snap-id>` et `.incomplete` au lieu de
  `current`) ; le chemin publié `P` le dépasse de 27.
- **Jeu ouvert :** sans effet sur `snapshot`, qui ne lit pas la source.

### 4.4 Récupération : préconditions (Q1)

`snapshot --recover`, avec ou sans `--apply`, ne s'appuie que sur le domaine
des snapshots :

| Exigé | Ignoré |
| --- | --- |
| Manifeste strict, pour l'identifiant du jeu ; coffre marqué ; `<vault>`, `games`, `games\<game-id>`, `snapshots` et `snapshots\journal` dossiers ordinaires sans point d'analyse ; distinction du coffre, de l'installation et des sources, sans lire leur contenu ; journaux et receipts du domaine des snapshots relus strictement ; observation de `P` et `I` de l'opération récupérée ; avec `--apply`, le verrou, pris comme `sync` ; pour les seules transitions qui approuvent, la porte de provenance (§4.5) | Présence, reconnaissance et intégrité de `current` ; opérations non terminales, journaux, receipts et anomalies du domaine de `current` ; `retained\` et `staging\` ; les autres entrées de la racine du coffre, de `games` et de `games\<game-id>`. Rien de tout cela n'est lu, jugé ni touché ; seul le receipt source est lu, à la porte |

- Une opération B2 en attente ne bloque pas la récupération d'un snapshot, et
  inversement : chaque domaine se récupère seul, sous le même verrou, dans
  l'ordre que choisit l'utilisateur.
- La récupération d'un snapshot ne lit **jamais** `current` : elle revérifie
  `P` contre le plan journalisé.
- Sans `snapshots\` ou sans `snapshots\journal`, il n'y a rien à récupérer :
  code 0.
- Le verrou occupé par l'autre commande donne code 1, « another operation is
  running on this game ».

### 4.5 Provenance (Q2)

La provenance d'un snapshot relie trois preuves, sans jamais lire le contenu
actuel de `current` après `planned` :

| Preuve | Domaine | Contenu |
| --- | --- | --- |
| Receipt source `receipts\<source-op>.json` | `current`, immuable par protocole | La génération copiée : ses `sources` |
| Plan journalisé `001-planned` | Snapshots, chaîné et validé | `source_operation`, `source_receipt_sha256`, `source_identity`, `plan` dont `sources`, `plan_sha256` |
| Receipt de snapshot `snapshots\receipts\<snap-id>.json` | Snapshots | `source.operation`, `source.receipt_sha256`, `plan_sha256`, `sources` |

1. **À la planification**, sous verrou : le receipt source est le receipt de
   tête, `current` est reconnu et égal ; `source_receipt_sha256` est le
   SHA-256 de ses octets lus ; `plan.sources` sont ses `sources`, recopiées ;
   l'empreinte de contenu `D` de ces `sources` est celle du receipt source.
2. **À la lecture du journal**, sans I/O : `plan_sha256` est le SHA-256 de
   `plan` ; `copied_sha256` et `verified_sha256` sont égaux à `D`.
3. **Porte de provenance**, avant `publish-intent` et avant l'écriture du
   receipt de snapshot, en exécution comme en récupération : ouvrir
   `receipts\` et `receipts\<source-op>.json` sans suivi de lien ; le receipt
   doit être un fichier ordinaire, valide au sens du receipt v1, d'opération
   `source_operation`, de jeu égal au `<game-id>` du dossier et au jeu du
   plan, d'octets de SHA-256 `source_receipt_sha256`, de `sources`
   d'empreinte `D`. Il n'a pas à être le receipt de tête : après une
   rotation, il ne l'est plus, et la porte reste franchissable. Ni `current`,
   ni `retained\` ne sont lus.
4. **Receipt de snapshot** : `snapshot` égal à l'opération ; `source.*`,
   `plan_sha256` et `sources` égaux à ceux de `planned` ; `published` porte le
   SHA-256 de ses octets. Une fois validé, il tient lieu de preuve de
   provenance : en A5 et A6 comme pour un snapshot publié, la porte n'est pas
   rejouée, puisqu'il n'est écrit qu'après elle.
5. **Porte fermée**, cause `source-unverified`, code 3 :

   | Preuve | Exemples | Avant `publish-intent` | Après la bascule, aucun receipt de snapshot validé |
   | --- | --- | --- | --- |
   | Indisponible | Erreur de lecture autre que l'absence, partage refusé | `failed`, `I` conservé, rien publié | Rien écrit ni déplacé ; l'opération reste en `publish-intent`, une nouvelle exécution réessaie |
   | Contradictoire | Receipt absent, lien ou point d'analyse, invalide, d'une autre opération ou d'un autre jeu, octets ou empreinte différents | Idem | Rollback : `unpublish-intent`, `P` → `I`, `rolled-back` |

   Les transitions de rollback n'approuvent rien et ne franchissent pas la
   porte. Une opération ainsi close ne bloque plus rien ; une opération en
   attente ne bloque que `snapshot --apply`, jamais `sync`.

Le receipt source est un fichier du domaine de `current` que B2 ne réécrit ni
ne supprime : le lire ne crée aucune dépendance à `current` lui-même. Son
absence ou sa modification signale une intervention extérieure sur le coffre,
que `sync` signale de son côté.

**Limite.** La porte vérifie la cohérence de preuves conservées dans le même
coffre, liées par des SHA-256 sans clé : elle refuse une altération
accidentelle ou partielle, pas une réécriture délibérée et coordonnée du
journal et du receipt source, hors modèle, comme en B2c pour le receipt de
`<prev-op>`.

### 4.6 Receipt de snapshot v1 (S8)

Fichier `snapshots\receipts\<snap-id>.json`, validé par temporaire et
renommage, enveloppe E1 propre. JSON canonique, relecture stricte comme le
receipt v1 : champ inconnu, manquant ou en double refusé. Schéma complet dans
ADR 0008.

| Champ | Contenu |
| --- | --- |
| `schema`, `schema_version` | `savewarden.snapshot` et `1` |
| `snapshot` | `<snap-id>` |
| `tool_version`, `recovered_by` | Comme le receipt v1 |
| `game` | Recopié du plan, donc du receipt source |
| `started_at`, `publish_intent_at`, `published_at`, `recovered_at` | Comme le receipt v1, mêmes règles d'ordre et de reconstruction |
| `source.operation` | Opération de la génération copiée |
| `source.receipt_sha256` | SHA-256 des octets de son receipt |
| `plan_sha256` | Empreinte du plan journalisé |
| `sources` | Celles du plan, donc du receipt source, en forme canonique |

Pas de `previous` ni de `changes` : un snapshot ne remplace rien.

### 4.7 Approbation, versions, idempotence (S6, S7)

- `snapshot --apply` est l'acte explicite de création et d'approbation. Un
  snapshot est **approuvé** quand son receipt est validé et que son journal
  se termine par `published`. Avant, il peut encore être renommé en arrière
  par un rollback ; après, plus jamais.
- Les snapshots s'accumulent, du plus ancien au plus récent par identifiant ;
  aucun n'est supprimé. Le plan affiche aussi le **nombre** des `.incomplete`
  conservés, compté sur les seuls noms des entrées de `snapshots\`, sans
  ouvrir ni parcourir ces dossiers. Leur **taille n'est pas mesurée** : dès
  qu'il en existe, la ligne d'état des snapshots l'écrit, `size not
  measured`, plutôt que d'annoncer une mesure. Aucune exception au
  non-parcours de `I` n'est faite pour cet indicateur.
- `--apply` est **idempotent par génération** : si un snapshot approuvé a pour
  `source.operation` l'opération de `current`, pour `source.receipt_sha256`
  l'empreinte de son receipt, et s'il est présent et conforme, rien n'est
  écrit, code `0`, `up to date`, `vault: not modified`. Un `.incomplete` ne
  compte pas.
- Après une rotation de `current`, un nouveau snapshot est possible ; les
  anciens restent intacts.

### 4.8 État des snapshots et dégradation (S13, Q3)

**Ce que contrôle chaque exécution de `snapshot`**, plan comme récupération,
pour chaque opération de snapshot terminale en `published` : receipt de
snapshot relu strictement et égal à son plan ; `P` présent sous l'identité
publiée. Le **contenu** n'est relu que pour le snapshot qui rendrait le plan
`up to date`. B3 ne prétend pas vérifier le contenu de tous les snapshots :
c'est le rôle de `doctor`, en B5.

| Observation | Diagnostic |
| --- | --- |
| `P` absent, `I` absent ou d'une autre identité | `snapshot-missing` |
| `P` absent, `I` présent de l'identité publiée : renommage perdu, signature d'une coupure | `snapshot-rename-lost` ; `I` n'est jamais renommé |
| `P` d'une autre identité, ou flanqué d'un `I` ; receipt de snapshot absent, invalide, contraire au plan ou d'empreinte différente de celle de `published` ; contenu relu différent | `snapshot-modified` |

La comparaison du receipt de snapshot à l'empreinte de `published` est une
observation, pas une cohérence du journal : un écart dégrade le snapshot, il
ne rend pas le journal ambigu.

- Un snapshot ainsi touché est **dégradé** : il ne compte plus pour
  l'idempotence ; un nouveau snapshot de la même génération est alors
  possible. Savewarden ne le recrée, ne le déplace, ne le répare ni ne
  l'adopte.
- La dégradation ne bloque ni `snapshot` ni `sync`, mais elle n'est **jamais
  silencieuse** : le plan affiche `snapshots: N published, K degraded,
  M incomplete`, suivie de `size not measured` quand `M` > 0 (§4.7), et
  nomme chaque snapshot dégradé ; tant que `K` > 0, toute
  exécution de `snapshot` écrit sur stderr `savewarden: warning: K degraded
  snapshot(s)`, avant la ligne d'état. Aucune sortie ne qualifie l'ensemble
  des snapshots de sain ou de vérifié.
- `sync` ne lit rien sous `snapshots\` et ne dit rien de leur état : la
  documentation renvoie au plan de `snapshot`.
- Hors de S13, toute ambiguïté du domaine des snapshots reste fail-closed :
  opération non terminale, journal illisible ou incohérent, entrée
  inexpliquée, receipt de snapshot sans journal qui l'explique ou
  incompatible avec l'état de son opération. Code 3 pour `snapshot`, sans
  effet sur `sync` ; elle bloque la création de snapshots jusqu'à
  l'intervention du propriétaire du coffre, et un nouveau coffre reste
  possible, comme en B2.

Ces codes restent de sévérité `Info` : `entities/backup` n'a que `Info`
et `Blocking`, partagées avec `inspect`. La visibilité passe par la ligne
d'état des snapshots et l'avertissement, pas par une troisième sévérité.

### 4.9 Ce que `snapshot` ne touche jamais

`current`, `retained\`, `staging\`, `receipts\` et `journal\` de `sync`, en
écriture comme en déplacement ; le contenu de `sync.lock`, qu'il ne peut que
créer vide, comme `sync` ; un snapshot publié ; un
`.incomplete` d'une autre opération ; la source, l'installation, le
manifeste ; un coffre non marqué.

### 4.10 Sortie et codes (ADR 0008)

Codes de sortie : même sens qu'ADR 0007. Ligne d'état du coffre en dernière
ligne de stderr avec `--apply` ou `--recover`. États : `not modified` ;
`metadata only; nothing staged or published` ; `snapshot <snap-id> kept
incomplete` ; `published snapshot <snap-id>` ; `rolled back snapshot
<snap-id>` ; `intervention required for <snap-id>: <cause>`.

Plan, forme indicative avec des identifiants fictifs :

```text
game: synthetic-game
vault: F:\Vault
current: 20261105T101500-1a2b3c4d, recognized
snapshots: 2 published, 1 degraded, 0 incomplete
  20261101T090000-0a1b2c3d  from 20261030T120000-11223344  2 files  6 bytes
  20261102T090000-4e5f6a7b  from 20261101T120000-55667788  snapshot-missing
snapshot: new, from 20261105T101500-1a2b3c4d
files: 2  bytes: 6  space needed: 67108870 bytes
findings: 1
  info  snapshot-missing  20261102T090000-4e5f6a7b  published snapshot is absent
```

stderr : `savewarden: warning: 1 degraded snapshot(s)`.

| Code stable | Sévérité | Sens |
| --- | --- | --- |
| `current-modified`, `insufficient-space` | bloquante | Repris de `sync` |
| `no-current` | bloquante | Aucune génération publiée : rien à copier |
| `verification-mismatch` | bloquante, `--apply` | Repris de `sync` |
| `long-path` | information | Repris de `sync`, mesuré sur `I` |
| `snapshot-missing`, `snapshot-rename-lost`, `snapshot-modified` | information, avertissement sur stderr | §4.8 |

Causes d'intervention : `pending-operation`, `unexpected-entry`,
`journal-ambiguous`, et nouvelle, `source-unverified` (§4.5). Diagnostics de
code 1 : `vault-changed` ; « another operation is running on this game ».

### 4.11 Compatibilité avec `sync` et les binaires B2

- `sync` accepte l'entrée `snapshots` de `games\<game-id>` si c'est un dossier
  ordinaire, sans point d'analyse ; un fichier ou un lien sous ce nom reste
  `unexpected-entry`, code 3. Il ne lit **rien** sous `snapshots\` : ni
  grammaire, ni receipts, ni journaux.
- Les journaux de snapshot n'entrent jamais dans les ensembles de `sync` :
  explication de `staging\`, règle M3, receipt de tête, opérations en attente.
  `snapshot` lit l'état de `current` par le lecteur de `sync`, en gardant ses
  propres journaux à part.
- Sur un coffre sans `snapshots`, la sortie de `sync` reste identique octet
  pour octet, message du verrou occupé compris.
- Un binaire antérieur à B3 refuse en code 3, `unexpected-entry`, sans rien
  écrire, un coffre dès que `snapshots\` existe, même vide, c'est-à-dire dès
  qu'un `snapshot --apply` a passé son second contrôle d'espace.
  Fail-closed, documenté.

---

## 5. Protocole : ADR 0006 §10

Résumé ; le texte normatif est celui d'ADR 0006 §10.

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
| 4 | `verified` | `verified_sha256` | Porte de provenance |
| 5 | `publish-intent` | `staging_identity` ; `P` absent attendu | Bascule `I` → `P` ; porte de provenance ; receipt de snapshot |
| 6 | `published` | `receipt_sha256` | Terminal |

`source_identity` porte un nom distinct de `current_identity` : le lecteur de
`sync` donne à ce dernier le sens de l'ancien `current` d'une rotation.
Rollback : `unpublish-intent` (`P` → `I`), puis `rolled-back` ;
`rollback-failed` au plus une fois (E3). Terminaux : `published`, `failed`,
`abandoned`, `rolled-back`.

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

Préconditions du §4.4 ; aucune lecture de `current`. Un receipt de snapshot
validé n'admet que deux lignes, A5 ou A6 et `published` ; toute autre
combinaison, et tout receipt de snapshot sans journal ou d'une opération
close autrement qu'en `published`, est une ambiguïté, code 3.

| Dernier validé | Observation (`P`, `I`) | Action |
| --- | --- | --- |
| Aucun : dossier d'opération vide ou temporaires `001` seuls | (—, —) | `001-abandoned` ; sinon code 3, rien écrit |
| `planned` | (—, —) | `abandoned` |
| `staging-intent` | (—, —), ou (—, dossier ordinaire sans point d'analyse) | `abandoned` ; `I` laissé en place, ni lu ni déplacé |
| `staged`, `verified` | (—, `new`) | `abandoned` ; `I` conservé |
| `publish-intent`, aucun receipt de snapshot | (—, `new`) | Renommage non effectué : `rolled-back` |
| `publish-intent`, aucun receipt de snapshot | (`new`, —) | Renommage effectué : revérification intégrale de `P` contre le plan. Égal, porte franchie : receipt, puis `published`. Égal, preuve contradictoire : `unpublish-intent`, `P` → `I`, `rolled-back`, code 3, `source-unverified`. Égal, preuve indisponible : code 3, rien écrit. Différent : `unpublish-intent`, puis `P` → `I` |
| `publish-intent`, receipt de snapshot validé | (`new`, —) | A5, A6 : receipt relu strictement, égal au plan, `P` revérifié en entier : `published`, sans rejouer la porte ; sinon code 3, rien écrit ni déplacé |
| `unpublish-intent`, aucun receipt de snapshot | (`new`, —) | Déplacement non effectué : l'exécuter, puis `rolled-back` ; échec : `rollback-failed`, code 3 |
| `unpublish-intent` ou `rollback-failed`, aucun receipt de snapshot | (—, `new`) | `rolled-back` |
| `rollback-failed`, aucun receipt de snapshot | (`new`, —) | Nouvel essai ; succès : `rolled-back` ; échec : code 3, sans nouvel enregistrement (E3) |
| `published` | (`new`, —), receipt de snapshot conforme | Rien |
| `published` | Toute autre observation de `P`, de `I` ou du receipt de snapshot | Snapshot dégradé (§4.8) : rien écrit ni déplacé |
| Tout autre cas, dont `P` et `I` présents ensemble hors d'une opération close, une identité `other`, un receipt de snapshot validé sans bascule observée | — | Fail-closed, code 3, rien écrit |

Les amendements ratifiés le 30 octobre 2026 s'appliquent tels quels :
pré-contrôle illisible tenu pour une ambiguïté, identité relue après chaque
contenu, receipt validé sans bascule observée refusé.

### 5.5 Enveloppe

E1, E3 et E4 inchangées. E2 : au plus 11 enregistrements ; le plus long chemin
d'un snapshot en compte 8 (`planned` à `publish-intent`, puis
`unpublish-intent`, `rollback-failed`, `rolled-back`).

### 5.6 Exclusivité et domaines (S9, S13)

- **Verrou :** un seul par jeu, `sync.lock`, pour les deux commandes.
- **I8 par domaine** : le domaine de `current` est `journal\`, celui des
  snapshots `snapshots\journal\`. Deux opérations non terminales d'un même
  domaine sont une ambiguïté, code 3. Une opération non terminale de
  `current` bloque `sync --apply` et `snapshot --apply` ; une opération de
  snapshot non terminale ne bloque que `snapshot --apply`. Une opération non
  terminale dans chaque domaine est un état admis.
- **Récupération :** chaque `--recover` ne récupère que son domaine et nomme
  l'autre commande pour l'autre. La récupération d'un snapshot est
  indépendante de l'**état vivant** de `current` et des opérations de son
  domaine ; sa seule lecture de ce domaine est le receipt source historique,
  à la porte de provenance (§4.4, §4.5). Celle de `current` ne lit rien sous
  `snapshots\`.
- **Exception au §3 d'ADR 0006 pour S13 :** une opération de snapshot dont
  le journal, valide, se termine par `published` n'est jamais une ambiguïté,
  quoi qu'on observe de `P`, de `I` ou de son receipt de snapshot ; elle est
  signalée comme dégradée (§4.8) et rien n'est écrit pour elle. Une opération
  de snapshot non terminale, un journal illisible ou incohérent, une entrée
  inexpliquée restent soumis à toutes les ambiguïtés du §3.

### 5.7 Ce que la publication d'un snapshot ne fait jamais

Écrire, renommer ou supprimer hors de `snapshots\`, hormis la création du
verrou vide comme `sync` ; écrire dans `sync.lock` ; déplacer `P` après
`published` ; renommer un `I` vers `P` hors de la bascule de sa propre
opération ; remplacer une cible ; copier vers un nom
publié ; lire un objet dont l'identité n'est pas celle attendue ; lire
`current` pendant une récupération ; approuver sans porte de provenance ;
supprimer un temporaire ou un `.incomplete` ; recréer ou réparer un snapshot
approuvé.

---

## 6. Immutabilité et garde de lecture seule (S12, Q4)

Après sa publication, un snapshot n'est **jamais rouvert en écriture, ni
déplacé, remplacé ou supprimé**. Les lectures de contrôle restent permises :
idempotence (S7), récupération, `inspect`, `doctor` plus tard. Cette garantie
repose sur :

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

**Garantie protocolaire, non matérielle.** Ce n'est pas une immutabilité de
type WORM : rien n'empêche un autre programme, l'utilisateur ou un
administrateur de modifier ou de supprimer un snapshot. Savewarden ne le
modifie jamais, et détecte une telle intervention dans les limites du §4.8 ;
la vérification du contenu de tous les snapshots relève de `doctor` (B5).

`readonly_test.go`, `architecture_test.go` et `platform/vault` restent
inchangés.

---

## 7. Périmètre des fichiers

| Zone | À créer | À modifier |
| --- | --- | --- |
| Plan et ADR | `docs/adr/0008-snapshot-contract-receipt-v1.md`, accepté avec ce plan | Ce plan, version validée ; ADR 0004, 0006, 0007, amendements acceptés avec ce plan |
| `internal/entities/backup` | Receipt de snapshot v1, sa relecture stricte et ses tests | Contrat des champs et automate du mode snapshot ; classement des observations du §5.4 |
| `internal/features/sync` | `snapshot.go`, `snapshot_apply.go`, `snapshot_recover.go` et leurs tests | Moteur paramétré par la disposition et les codes ; lecture de l'entrée `snapshots` ; I8 par domaine ; lecture du coffre limitée au domaine des snapshots pour la récupération ; documentation du package ; faux coffre |
| `internal/cli` | `snapshot.go` et tests | `root.go`, texte de la commande racine et câblage |
| `internal/app` | — | `app.go` ; `app_test.go`, exemple de commande inconnue |
| `cmd/savewarden` | Tests binaires Windows du snapshot | `main_test.go`, exemples README |
| `docs/` | `commands/snapshot.md` ; page du receipt de snapshot ; `validation/b3.md` | `vault.md`, guide de récupération compris ; `safety-model.md` ; `architecture.md` ; `pilots/odyssey.md` |
| Racine | — | `README.md`, `CHANGELOG.md`, `AGENTS.md` (lot courant) |

Changements interdits :

- toute modification de `internal/platform/vault`,
  `internal/app/readonly_test.go`, `internal/app/architecture_test.go`,
  ADR 0005, `go.mod` et `go.sum` ;
- toute API d'écriture, de suppression ou de remplacement nouvelle ; toute
  écriture de `snapshot` hors de `snapshots\`, hormis le verrou vide créé par
  `Lock` comme pour `sync` ;
- toute sévérité nouvelle dans `entities/backup` ;
- les packages `features/snapshot`, `restore`, `doctor`, et tout dossier vide ;
- toute modification du schéma de manifeste v1, du receipt v1, du marqueur et
  du format d'enregistrement du journal ;
- tout changement de comportement ou de sortie de `sync` sur un coffre sans
  `snapshots` ;
- tout chemin personnel ou propre au pilote dans le code, les tests ou les
  exemples ; toute fixture versionnée dans un dossier nommé `snapshots/` ;
- toute modification des rulesets, des versions d'Actions ou de la CI hors
  ajout de contrôles ;
- tout accès agent à `D:\Ga`, `D:\Savewarden`, `E:\GMS` ou aux clés SSH ;
- tout test lisant ou écrivant hors de `t.TempDir()`.

---

## 8. Plan d'exécution (S11)

Avant tout code : cette v0.2 et ses ADR validés par le propriétaire sur
`docs/b3-framing`, fait le 31 octobre 2026, puis un GO de développement
distinct, non donné. La PR
`feat/b3-snapshot` part de ces commits de cadrage, comme celle de B2c.

| # | Commit | Point de contrôle |
| --- | --- | --- |
| 0 | Plan validé ; ADR 0006, 0004 et 0007 amendés ; ADR 0008, statuts passés à « accepté » : commits de cadrage de `docs/b3-framing`, jusqu'à l'acceptation du 31 octobre 2026 | **Revue du cadrage** par le propriétaire, faite |
| 1 | `entities/backup` : receipt de snapshot, automate et champs, classement, tests purs | — |
| 2 | Moteur de `features/sync` paramétré, sans changement de comportement : suite B2 et sortie de `sync` inchangées | **Revue indépendante de non-régression, obligatoire** avant le commit 3 |
| 3 | `sync` accepte l'entrée `snapshots` ; I8 par domaine | — |
| 4 | Plan de snapshot en lecture seule ; CLI `snapshot` sans `--apply` | — |
| 5 | `--apply` : structure, automate, copie, vérification, porte de provenance, bascule, receipt ; énumération des crashs | — |
| 6 | `--recover`, indépendant de `current` ; énumération des crashs pendant la récupération | **Revue indépendante du protocole, obligatoire** avant le commit 7 |
| 7 | Tests Windows réels et binaires ; documentation | — |
| 8 | Rapport `docs/validation/b3.md` | **Revue de sécurité indépendante**, avant le GO de fusion |

Chaque commit passe la suite complète à son SHA. Push, PR, fusion et pilote
attendent chacun un GO distinct du propriétaire.

---

## 9. Matrice de tests

Noms proposés, fixés au commit de chaque partie. Faux coffre sur les trois OS
sauf mention.

**Entités**

| Test | Preuve |
| --- | --- |
| `TestSnapshotReceiptRoundTrip`, `TestSnapshotReceiptIsStrict` | Forme canonique ; champs inconnus, manquants, en double, dates, reconstruction refusés |
| `TestSnapshotStepFieldsAreStrict`, `TestSnapshotAutomaton` | Contrat de chaque étape ; transitions permises seulement ; étapes et champs de rotation refusés en mode snapshot |
| `TestSnapshotPlanIsCoherent` | `plan_sha256`, `copied_sha256`, `verified_sha256` et empreinte de contenu du plan recoupés à la lecture |
| `TestSnapshotRecoveryRows` (pur) | Chaque ligne du §5.4, exception A5 et A6, porte fermée et défaut fail-closed compris |

**Non-régression et compatibilité de `sync`**

| Test | Preuve |
| --- | --- |
| Suite B2 complète, inchangée | Moteur paramétré sans changement de comportement, énumérations de crash comprises |
| `TestSyncOutputUnchangedWithoutSnapshots` | Sortie identique octet pour octet sur un coffre B2, messages compris |
| `TestSyncAcceptsSnapshotsDirectory` | Plan, `--apply`, rotation et `--recover` inchangés à côté de snapshots |
| `TestSnapshotsAsFileOrLinkIsUnexpectedForSync` | Fichier ou lien nommé `snapshots` : code 3 |
| `TestSnapshotAnomaliesNeverBlockSync` | Journal de snapshot ambigu, entrée hors grammaire, opération de snapshot en attente, snapshot dégradé : `sync` plan, `--apply`, rotation et `--recover` inchangés |
| `TestPendingCurrentOperationBlocksSnapshotApply` | Opération de `current` non terminale : `snapshot --apply` refusé, code 3 |
| `TestSnapshotJournalsStayOutOfSyncSets` | Un `staging\<snap-id>` reste inexpliqué ; la règle M3 ignore un snapshot publié |
| `TestRotationLeavesSnapshotsUntouched` | Empreinte de `snapshots\` inchangée par une rotation et sa récupération |

**Snapshot : création**

| Test | Preuve |
| --- | --- |
| `TestSnapshotPlanWritesNothing` | Aucun `Vaults` reçu ; empreinte du coffre inchangée |
| `TestSnapshotRequiresRecognizedCurrent`, `TestSnapshotWithoutCurrentIsBlocked` | `current-modified`, `no-current`, coffre copié en code 3 |
| `TestSnapshotTakesTheLockLikeSync` | `sync.lock` absent : créé vide par `Lock`, jamais écrit ; aucune autre écriture hors de `snapshots\` |
| `TestSnapshotChecksDistinction` | Coffre dans l'installation ou une source, par nom et par identité : code 1 |
| `TestSnapshotCopiesCurrentExactly` | Entrées, tailles, dates, SHA-256, dossiers ; `sources` du receipt égales à celles du receipt source ; empreintes égales |
| `TestSnapshotWritesOnlyUnderSnapshots` | Trace des appels : toute écriture sous `snapshots\` ; empreinte du reste du coffre inchangée |
| `TestSnapshotIsIdempotentForTheSameGeneration`, `TestNewSnapshotAfterRotation` | S7 ; deux snapshots distincts, tous deux intacts |
| `TestIncompleteBeforeStagedIsExplainedByName` | `I` sans identité journalisée, opération terminale : accepté, jamais lu ni déplacé |
| `TestSnapshotOfCurrentChangedDuringCopy` | Contenu : `current-modified`, `failed` ; identité : code 3 ; `I` conservé, rien publié |
| `TestSnapshotIDCollisionIsRefused` | Identifiant déjà employé dans l'un ou l'autre domaine : code 1, avant tout journal |
| `TestSnapshotStructureCreationIsResumable` | Crash entre les créations de structure : état reconnu, complété au `--apply` suivant |
| `TestSnapshotSpaceIsCheckedBeforeAnyWrite`, `TestSnapshotLongPathIsReported` | Espace insuffisant sous verrou : aucune structure créée ; chemins longs mesurés sur `I` |
| `TestCrashAtEverySnapshotCall` | Crash avant et après chaque appel au coffre, état final exact par point |
| `TestPublishedSnapshotIsNeverMovedOrReopenedForWriting` | Après `published`, aucun appel ne vise `P` en déplacement ou en création, à travers `snapshot`, `sync`, une rotation et chaque récupération |

**Snapshot : provenance (Q2)**

| Test | Preuve |
| --- | --- |
| `TestProvenanceSurvivesRotation` | Opération interrompue après `publish-intent`, puis rotation de `current` : la récupération franchit la porte avec le receipt source, devenu non-tête, et publie sans lire `current` |
| `TestClosedGateBeforePublishFails` | Porte fermée avant `publish-intent` : `failed`, `I` conservé, rien publié, code 3, `source-unverified` |
| `TestContradictoryProvenanceRollsBack` | Après la bascule, receipt source absent, lien, invalide, d'une autre opération ou d'un autre jeu, d'octets ou d'empreinte différents : rollback jusqu'à `rolled-back`, en exécution comme en récupération ; `sync` non bloqué |
| `TestUnavailableProvenanceWritesNothing` | Erreur de lecture du receipt source après la bascule : code 3, rien écrit ni déplacé, opération en `publish-intent` ; une exécution suivante aboutit |
| `TestValidatedSnapshotReceiptNeedsNoGate` | A5 et A6 : receipt source altéré après la validation du receipt de snapshot : `published` sans rejouer la porte |
| `TestRollbackNeedsNoProvenance` | Receipt source absent : `abandoned` et `rolled-back` restent possibles |

**Snapshot : récupération (Q1)**

| Test | Preuve |
| --- | --- |
| `TestSnapshotRecoverPlanWritesNothing` | Aucun `Vaults` reçu ; empreinte du coffre inchangée |
| `TestSnapshotAndCurrentRecoveriesAreIndependent` | Snapshot interrompu, puis rotation interrompue dans la fenêtre où `current` est absent : `snapshot --recover --apply` aboutit sans lire ni toucher le domaine de `current`, puis `sync --recover --apply` aboutit ; même résultat dans l'ordre inverse ; empreinte de l'autre domaine inchangée à chaque étape |
| `TestSnapshotRecoveryIgnoresCurrentDomainAnomalies` | `current` absent, modifié, journal de `current` ambigu, `sync.lock` absent, entrée imprévue hors de `snapshots\` : la récupération du snapshot n'en dépend pas |
| `TestSnapshotRecoveryWithoutJournalHasNothingToDo` | `snapshots\` ou `snapshots\journal` absent : code 0, rien écrit |
| `TestOrphanSnapshotReceiptIsAmbiguous` | Receipt de snapshot sans journal, ou d'une opération close autrement qu'en `published`, ou à côté d'un rollback : code 3, rien écrit |
| `TestSnapshotRecoveryIsInterruptible`, `TestRepeatedSnapshotRecoveryIsStable` | Propriétés du §9 d'ADR 0006 ; trois exécutions sur une ambiguïté : coffre identique octet pour octet, diagnostic identique |
| `TestSnapshotEnvelopeIsBounded` | Dépassement de E1, E2 et E3 : code 3 sans mutation |

**Snapshot : dégradation (Q3)**

| Test | Preuve |
| --- | --- |
| `TestDegradedSnapshotsAreVisible` | `snapshot-missing`, `snapshot-rename-lost`, `snapshot-modified`, dont un receipt de snapshot absent ou d'empreinte différente : ligne `K degraded`, avertissement sur stderr dans chaque mode, rien écrit ni renommé pour eux ; un nouveau snapshot de la même génération est possible |
| `TestSnapshotDomainAmbiguityNeverBlocksSync` | Journal de snapshot incohérent : `snapshot` en code 3 ; `sync` inchangé |
| `TestOnlyTheUpToDateSnapshotIsReadInFull` | Trace des lectures : contenu relu pour le seul snapshot de la génération courante |

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

`docs/commands/snapshot.md` ; `docs/vault.md`, disposition, état des snapshots
et dégradation, guide de récupération du snapshot ; page du receipt de
snapshot ; modèle de sécurité, invariant 7 implémenté et limite non WORM ;
architecture ; README et ses exemples ; CHANGELOG ; ADR ; `AGENTS.md`, lot
courant.

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
2. ADR 0006, 0004 et 0007 amendés et ADR 0008 acceptés avant tout code
   d'écriture ; ADR 0005, `readonly_test.go`, `architecture_test.go` et
   `platform/vault` inchangés.
3. Un snapshot publié est égal, fichier par fichier, au receipt de la
   génération source, relu depuis le disque avant publication ; aucune
   approbation sans porte de provenance franchie ; le receipt de snapshot
   prouve la provenance par `source.receipt_sha256` et l'empreinte de contenu.
4. `snapshot` n'écrit que sous `snapshots\`, hormis le verrou vide que `Lock`
   crée s'il manque, comme `sync` ;
   `current`, `retained\`, `staging\`, `receipts\` et `journal\` restent
   identiques octet pour octet ; le contenu de la source n'est jamais lu.
5. Après sa publication, un snapshot n'est jamais rouvert en écriture,
   déplacé, remplacé, recréé ni supprimé, à travers toutes les commandes et
   récupérations ; les lectures de contrôle restent permises.
6. Chaque point d'arrêt et chaque ligne du §5.4 sont atteints par un test,
   exception A5 et A6 et porte de provenance comprises ; la récupération
   respecte le §9 d'ADR 0006 dans l'enveloppe E1 à E4.
7. Un verrou unique par jeu ; I8 par domaine ; la récupération d'un snapshot
   ne dépend ni de `current` ni d'une opération B2 en attente, et aucune
   anomalie **sous** `snapshots\` ne bloque `sync`, récupération de `current`
   comprise : test croisé. L'entrée `snapshots` elle-même, si ce n'est pas un
   dossier ordinaire, reste une entrée imprévue pour `sync`.
8. La suite B2 passe inchangée ; la sortie de `sync` est identique octet pour
   octet sur un coffre sans snapshot ; `sync` accepte un coffre avec
   snapshots ; un binaire antérieur le refuse sans écrire.
9. Idempotence par génération ; snapshots successifs intacts après rotation ;
   dégradations signalées dans chaque mode, sans blocage ni écriture, jamais
   présentées comme un état sain.
10. Revues indépendantes consignées : non-régression, protocole, sécurité.
11. 7 checks requis verts sur le SHA exact, avec lien du run.
12. Documentation, guide de récupération, CHANGELOG et ADR cohérents ;
    exemples README vérifiés par test ; limite non WORM écrite.
13. Aucun accès agent aux données protégées ; aucun chemin personnel, save ou
    receipt réel dans le diff.
14. Pilote B3 exécuté et qualifié par le propriétaire, ou absence déclarée.

---

## 13. Décisions reportées hors de B3

| Sujet | Comportement en B3 |
| --- | --- |
| Rétention, suppression ou nettoyage de snapshots et de `.incomplete` | Aucune suppression ; accumulation documentée ; le plan affiche le nombre des `.incomplete`, pas leur taille, qu'il ne mesure pas |
| Snapshot d'une génération `retained\` | Non proposé |
| Libellés, notes | Absents ; l'identifiant et la provenance seuls nomment un snapshot |
| Vérification du contenu de tous les snapshots publiés | `doctor`, B5 (§4.8) |
| Réparation d'un snapshot approuvé absent ou altéré, ou d'un renommage perdu | Jamais ; signalé (S13) |
| Ambiguïtés du domaine des snapshots hors de S13 | Fail-closed : elles bloquent la création de snapshots, jamais `sync` ; intervention du propriétaire du coffre ; nouveau coffre possible |
| Authentification forte des preuves de provenance | Hors modèle : la porte vérifie une cohérence, pas une signature |
| Attribut lecture seule, ACL, immutabilité matérielle | Refusés par le garde et par la revue ; immutabilité protocolaire |
| Extraction du moteur de publication | Réexaminée au cadrage de B4 (§3.1) |
| Portabilité d'un coffre copié ou déplacé | Inchangée : code 3, sans rien écrire |
| Import de snapshots créés hors de Savewarden | Jamais adoptés |
| ReFS | Toujours non qualifié |

---

## 14. Risques

| Risque | Effet | Parade |
| --- | --- | --- |
| Régression de `sync` par le paramétrage du moteur | Perte d'une garantie qualifiée de B2 | Commit de refactor distinct, suite B2 et énumérations inchangées, sortie identique, revue de non-régression obligatoire |
| Anomalie de snapshot qui bloquerait `current` | Protection de l'état courant suspendue | I8 par domaine (S9) ; `sync` ne lit rien sous `snapshots\` ; test dédié |
| État de `current` qui bloquerait la récupération d'un snapshot | Snapshot coincé non terminal | Récupération limitée au domaine des snapshots (§4.4) ; test croisé |
| Receipt source modifié ou disparu | Provenance non prouvable | Porte de provenance avant toute approbation ; `source-unverified` ; opération close ou annulée sur preuve contradictoire, jamais bloquée pour toujours |
| Réécriture délibérée et coordonnée du journal et du receipt source | Fausse provenance acceptée | Hors modèle, limite écrite ; SHA-256 sans clé |
| Coupure qui perd le renommage de publication | Snapshot approuvé introuvable sous son nom | `snapshot-rename-lost`, copie conservée sous `.incomplete`, jamais renommée par conjecture |
| Binaire antérieur, ou revert, sur un coffre avec snapshots | Utilisateur bloqué en code 3 | Fail-closed sans écriture ; documenté ; pilote dans un coffre neuf (S14) |
| Receipt ou journal de snapshot lu par `sync` comme le sien | Faux `current-modified`, staging expliqué à tort, règle M3 faussée | Rangement séparé sous `snapshots\` (S3), ensembles distincts, tests |
| `current` modifié pendant la copie | Snapshot faux | Relecture intégrale avant, comparaison de la copie au plan, identité relue après |
| Crash entre bascule et receipt | Snapshot présent, non approuvé | Points d'arrêt, revérification intégrale, rollback ; aucun retour en arrière après receipt validé |
| Snapshot approuvé supprimé ou modifié à la main | Blocage définitif si fail-closed ; ou dégradation ignorée | Signalé sans blocage mais toujours visible (S13, Q3) ; jamais réparé |
| Utilisateur qui croit à une immutabilité matérielle | Fausse sécurité | Limite non WORM écrite dans le plan, ADR et documentation |
| Accumulation de snapshots et de `.incomplete` | Disque plein | Préflight d'espace avant toute écriture, idempotence par génération, nombre des `.incomplete` affiché, taille déclarée non mesurée, documentation |
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
| 0.1 | 30 octobre 2026 | `5089378`, local, branche `docs/b3-framing` | Brouillon de cadrage : décisions S1 à S14 proposées, protocole de snapshot proposé comme amendement d'ADR 0006, matrice de tests, critères ; revue indépendante en lecture seule intégrée avant remise |
| 0.2 | 30 octobre 2026 | local, branche `docs/b3-framing` | Revue CTO : S1 à S14 retenues dans leur principe ; Q1 récupération indépendante de `current` et du domaine B2, préconditions séparées, test croisé ; Q2 porte de provenance et `source-unverified` ; Q3 dégradation visible, portée du contrôle écrite ; Q4 immutabilité en écriture, garantie non WORM ; message du verrou de `sync` inchangé ; projets d'ADR 0004, 0006, 0007 et 0008. Revue croisée indépendante du contrat de récupération intégrée : rollback sur preuve contradictoire, porte non rejouée après un receipt de snapshot validé, verrou pris comme `sync`, comparaison du receipt en observation, `snapshot-rename-lost`, grammaire des sous-dossiers, règle unique du receipt orphelin, limite de la porte. Aucun GO de développement |
| 0.2, revue indépendante | 31 octobre 2026 | local, branche `docs/b3-framing` | Revue indépendante de la v0.2 et des projets d'ADR : aucun P0, S1 à S14 conservées. P1-1 : nombre des `.incomplete` compté sans les parcourir, taille déclarée non mesurée ; P1-2 : dégradation d'un receipt de snapshot limitée à une opération au journal valide et terminal en `published`, receipt orphelin ou incompatible en code 3 (ADR 0008) ; indépendance de la récupération qualifiée par rapport à l'état vivant de `current`, porte de provenance exceptée ; ligne vide finale d'ADR 0004 retirée. Aucun GO de développement |
| 0.2, validation | 31 octobre 2026 | local, branche `docs/b3-framing` | Validée par le propriétaire ; ADR 0004, 0006 §10, 0007 et 0008 passés au statut accepté ; contenu normatif inchangé. GO de développement B3 non donné |
| 0.2, GO de développement | 31 octobre 2026 | local, branche `feat/b3-snapshot` | GO de développement B3 donné par le propriétaire ; contenu normatif inchangé ; push, PR, fusion et pilote restent soumis à des GO distincts |

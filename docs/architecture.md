# Architecture Savewarden

Décision propre à Savewarden : **FSD-inspired, Go-idiomatic**, documentée dans
[ADR 0001](adr/0001-fsd-inspired-go.md). Mosaera est une inspiration de
conception, pas une spécification à laquelle le produit prétend être conforme.

## Existant B4

```text
cmd/savewarden → app ─┬─► cli ─┬─► features/inspect ──► entities/game, entities/backup
                      │        └─► features/sync ─────► entities/game, entities/backup
                      ├─► platform/manifests ──► entities/game, go.yaml.in/yaml/v3
                      ├─► platform/filesystem ─► bibliothèque standard, syscall       (lecture seule)
                      └─► platform/vault ──────► bibliothèque standard, x/sys/windows (seul package qui écrit)
```

`main` fournit contexte, arguments, version et streams. `app.Run` assemble à
chaque invocation les services `inspect` et `sync` avec leurs adaptateurs,
construit l'arborescence CLI et traduit les erreurs en code de sortie, dont
le code 2 d'[ADR 0003](adr/0003-inspect-exit-codes.md) et les codes 2 et 3
d'[ADR 0007](adr/0007-sync-contract-receipt-v1.md). `cli` possède les flags,
le rendu texte, l'`ExitError` et la ligne d'état du coffre. Aucune
découverte de configuration. Seul `platform/vault` écrit, en création seule
([ADR 0005](adr/0005-write-allowed-package.md)) ; `app` ne donne son
`Writer` qu'à `--apply` et `--recover --apply`, jamais au plan. Pour
`restore`, ses sessions s'ouvrent sur la zone de restauration, jamais sur le
coffre ; la ligne d'état nomme alors la destination.

| Package | Rôle |
| --- | --- |
| `entities/game` | Manifeste, jeu, installation, sources ; validation pure et grammaire des chemins et noms Windows, indépendante de l'hôte |
| `entities/backup` | Types partagés : `Entry`, `Directory`, `Finding` ; différence pure ; receipt v1, receipt de snapshot v1, receipt de restauration v1, marqueur du coffre et marqueur de zone stricts ; journaux d'opération, de snapshot et de restauration, leur automate et le classement des observations d'[ADR 0006](adr/0006-publication-recovery-protocol.md) ; aucune I/O |
| `features/inspect` | Inventaire en lecture seule ; ports `ManifestReader`, `FileSystem` et `Scope` ; `Report` typé ; `Directory`, inventaire sans suivi d'un dossier quelconque |
| `features/sync` | Plan, première publication, rotation de `current`, snapshots, restauration isolée et récupérations selon [ADR 0006](adr/0006-publication-recovery-protocol.md) ; moteur paramétré par le domaine, avec une source de lecture et une cible d'écriture distinctes ; politique des retraits et des sources absentes ; ports `ManifestReader`, `Inventories`, `Files`, `Volumes`, et pour écrire `Vaults` et `Clock` ; `Plan`, `Result`, `Recovery` et leurs pendants de snapshot et de restauration typés |
| `platform/manifests` | Lecture stricte du YAML vers `game.Manifest`, [ADR 0002](adr/0002-manifest-v1-strict-yaml.md) |
| `platform/filesystem` | Portée ancrée sans suivi de liens ; implémentation Windows supportée, Unix pour les tests du contrat ; fichiers EFS signalés ; identité de fichier ; un fichier tenu sans partage est décrit, jamais ouvert |
| `platform/vault` | Session de coffre en création seule, Windows seulement : dossiers et fichiers `CREATE_NEW`, flush, date, `MoveFileEx` sans drapeau, identité, verrou, type de volume et espace disponible ; dossiers épinglés sans suivi de lien |

Les ports `Scope` des deux features n'utilisent que des types de la
bibliothèque standard (`fs.FileInfo`, `fs.DirEntry`, `fs.File`) :
`platform/filesystem` les implémente sans importer de feature, et `app` fait
le pont. `sync` n'importe pas `inspect` : son port `Inventories` a la
signature exacte de `inspect.Service.Directory`, et `app` lui passe le service
`inspect` ([ADR 0004](adr/0004-sync-boundaries.md)). Les types d'inventaire
vivent désormais dans `entities/backup`.

`os.Root` assure le confinement des noms mais suit les liens qui restent sous
sa racine. L'adaptateur Windows ajoute donc ses propres garanties : ouverture
de chaque dossier traversé avec `FILE_FLAG_OPEN_REPARSE_POINT`, contrôle des
attributs sur le handle, dossiers gardés ouverts sans partage de suppression
jusqu'à la fin, fichiers refusés si le handle montre un point d'analyse ou un
placeholder. Voir le [modèle de sécurité](safety-model.md).

`dev` est la version par défaut. Une build revue peut injecter une version
avec `-ldflags="-X main.version=..."` ; cela ne crée pas une release.
La CLI ne dépend ni du nom du dossier courant, ni de la date, ni de Git.

## Frontières au fil des lots

```text
cmd/savewarden → app
app → cli + features/* + platform/*
cli → features/* (APIs publiques)
features/* → entities/* + ports ciblés
platform/* → entities/* si nécessaire
entities/* → bibliothèque standard
```

Chaque feature (`inspect`, `sync`, puis `doctor`) possède son orchestration ;
les snapshots et la restauration isolée sont des opérations de `sync`
([ADR 0004](adr/0004-sync-boundaries.md), amendements B3 et B4). Les entités
portent les types métier réellement communs.
Les ports sont des petites interfaces définies par leur consommateur ;
`app` fournit les adaptateurs. Pas d'import feature → feature, feature →
platform/cli/app/Cobra ou platform → feature. Un scénario composé passe par
un service supérieur explicitement conçu et un ADR.

Les sous-packages partagent les frontières de leur couche. Les API exportées
restent limitées ; pas de barrel Go artificiel ni de registre DI. Les imports
sont contrôlés pour le code de production dans `internal/app/architecture_test.go`,
y compris pour les fichiers réservés à un OS :

- dépendances tierces autorisées par couche seulement : Cobra dans `cli`,
  `go.yaml.in/yaml/v3` dans `platform/manifests`, et le seul package
  `golang.org/x/sys/windows` dans `platform/vault` exactement, aucune ailleurs ;
- `syscall` réservé aux adaptateurs `platform/*`.

`internal/app/readonly_test.go` refuse en plus, dans tout le code de
production, les API connues d'écriture, d'exécution, de réseau et de chargement
dynamique, ainsi que les modes d'ouverture en écriture. C'est un garde
statique, pas une preuve : les tests dynamiques de non-mutation le complètent.
[ADR 0005](adr/0005-write-allowed-package.md) l'assouplit pour le seul
package `internal/platform/vault`, ni ses sous-packages ni un nom voisin :
création exclusive `CREATE_NEW`, dossiers créés un par un, flush, date,
`MoveFileEx` appelé avec le littéral `0`. Suppression, remplacement,
troncature et tout drapeau `MOVEFILE_*` restent refusés partout, `vault`
compris.

Ne créer un package que lorsqu'un fichier utile existe ; `features/restore`
n'existe pas, la restauration étant une opération de `sync`, et l'extraction
du moteur est réexaminée selon les critères de l'amendement B4 d'ADR 0004.
Aucun `shared` n'est nécessaire. Les détails propres à un OS sont isolés par
build tags.

## Contrats à stabiliser

- B1, livré : manifeste v1 strict, [documenté](manifest-v1.md), source de
  vérité déclarée par l'utilisateur, portée ancrée sans suivi de liens,
  inventaire déterministe et [contrat de sortie](commands/inspect.md).
- B2 : en trois PR. B2a, livré : plan en lecture seule, [documenté](commands/sync.md),
  [coffre](vault.md), [receipt v1](receipt-v1.md), protocole de publication et
  de récupération d'[ADR 0006](adr/0006-publication-recovery-protocol.md),
  contrat d'[ADR 0007](adr/0007-sync-contract-receipt-v1.md). B2b, livré :
  écritures confinées à `platform/vault` ([ADR 0005](adr/0005-write-allowed-package.md)),
  première publication et sa récupération. B2c, livré : rotation de
  `current` vers `retained`, reconnaissance de la génération précédente par
  son identité publiée, retraits et sources absentes acceptés par drapeau,
  récupération complète.
- B3, livré : snapshots immuables de `current`, [documentés](commands/snapshot.md),
  [receipt de snapshot](snapshot-receipt-v1.md), protocole d'ADR 0006 §10 et
  contrat d'[ADR 0008](adr/0008-snapshot-contract-receipt-v1.md). Les
  snapshots sont une opération de `features/sync`, qui possède toutes les
  publications du coffre d'un jeu (ADR 0004, amendement B3) : son moteur de
  publication est paramétré par le domaine de l'opération, `current` ou
  snapshots, sans changement de comportement pour `current`. Aucun port ni
  frontière nouvelle, aucune API d'écriture nouvelle.
- B4, en développement : restauration d'un snapshot approuvé vers une zone
  de restauration isolée, [documentée](commands/restore.md),
  [receipt de restauration et marqueur de zone](restore-receipt-v1.md),
  protocole d'ADR 0006 §11 et contrat
  d'[ADR 0009](adr/0009-restore-contract-receipt-v1.md). Le plan est le mode
  par défaut, sans écriture ; aucun écrasement, aucune restauration en place,
  aucune écriture dans le coffre. Opération de `features/sync`, sans port ni
  frontière nouvelle ni API d'écriture nouvelle.
- B5 : diagnostic read-only et campagne pilote.

Les contrats encore futurs ne sont pas implémentés : aucune rétention des
générations conservées, et la CLI n'a pas de sortie JSON. Les erreurs futures utilisent contexte et
`errors.Is/As` lorsqu'un consommateur doit les distinguer. Les chemins utilisent
`filepath`, les contenus `io.Reader` et les opérations `context.Context`.

## Références et autorité

Les fichiers Dirloom ont été consultés comme source implémentée de séparation
des responsabilités et de workflow. Les docs Mosaera/Sonora ont été consultées
comme descriptions de conception ; leur lecture ne valide pas le code Sonora.
Les références et SHA de blobs consultés figurent dans
[le dossier B0](validation/b0.md). Les décisions normatives sont celles de ce
dépôt, de ses ADR et de ses tests.

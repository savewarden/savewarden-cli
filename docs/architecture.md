# Architecture Savewarden

Décision propre à Savewarden : **FSD-inspired, Go-idiomatic**, documentée dans
[ADR 0001](adr/0001-fsd-inspired-go.md). Mosaera est une inspiration de
conception, pas une spécification à laquelle le produit prétend être conforme.

## Existant B1

```text
cmd/savewarden → app ─┬─► cli ──────────────► features/inspect ──► entities/game
                      ├─► platform/manifests ──► entities/game, go.yaml.in/yaml/v3
                      └─► platform/filesystem ─► bibliothèque standard, syscall
```

`main` fournit contexte, arguments, version et streams. `app.Run` assemble à
chaque invocation le service `inspect` avec ses deux adaptateurs, construit
l'arborescence CLI et traduit les erreurs en code de sortie, y compris le
code 2 d'[ADR 0003](adr/0003-inspect-exit-codes.md). `cli` possède les flags,
le rendu texte et l'`ExitError`. Aucune découverte de configuration.

| Package | Rôle |
| --- | --- |
| `entities/game` | Manifeste, jeu, installation, sources ; validation pure et grammaire des chemins Windows indépendante de l'hôte |
| `features/inspect` | Orchestration en lecture seule ; ports `ManifestReader`, `FileSystem` et `Scope` ; `Report` typé, indépendant du rendu |
| `platform/manifests` | Lecture stricte du YAML vers `game.Manifest`, [ADR 0002](adr/0002-manifest-v1-strict-yaml.md) |
| `platform/filesystem` | Portée ancrée sans suivi de liens ; implémentation Windows supportée, Unix pour les tests du contrat |

Le port `Scope` n'utilise que des types de la bibliothèque standard
(`fs.FileInfo`, `fs.DirEntry`, `fs.File`) : `platform/filesystem` l'implémente
sans importer la feature, et `app` fait le pont. Les types d'inventaire restent
dans la feature tant que seul `inspect` les utilise ; ils rejoindront
`entities` quand `sync` en aura besoin.

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

Chaque feature (`inspect`, `sync`, `snapshot`, `restore`, `doctor`) possède
son orchestration. Les entités portent les types métier réellement communs.
Les ports sont des petites interfaces définies par leur consommateur ;
`app` fournit les adaptateurs. Pas d'import feature → feature, feature →
platform/cli/app/Cobra ou platform → feature. Un scénario composé passe par
un service supérieur explicitement conçu et un ADR.

Les sous-packages partagent les frontières de leur couche. Les API exportées
restent limitées ; pas de barrel Go artificiel ni de registre DI. Les imports
sont contrôlés pour le code de production dans `internal/app/architecture_test.go`,
y compris pour les fichiers réservés à un OS :

- dépendances tierces autorisées par couche seulement : Cobra dans `cli`,
  `go.yaml.in/yaml/v3` dans `platform/manifests`, aucune ailleurs ;
- `syscall` réservé aux adaptateurs `platform/*`.

`internal/app/readonly_test.go` refuse en plus, dans tout le code de
production, les API connues d'écriture, d'exécution, de réseau et de chargement
dynamique, ainsi que les modes d'ouverture en écriture. C'est un garde
statique, pas une preuve : les tests dynamiques de non-mutation le complètent.
B2 devra l'assouplir explicitement, par package et avec un ADR.

Ne créer `entities/backup`, `features/sync` et les autres packages que lorsqu'un
fichier utile existe. Aucun `shared` n'est nécessaire. Les détails propres à un
OS sont isolés par build tags.

## Contrats à stabiliser

- B1, livré : manifeste v1 strict, [documenté](manifest-v1.md), source de
  vérité déclarée par l'utilisateur, portée ancrée sans suivi de liens,
  inventaire déterministe et [contrat de sortie](commands/inspect.md).
- B2 : destination protégée, plan et staging, paths/tailles/SHA-256,
  détection de changements concomitants, receipt et rollback Windows conçu
  avant toute mise à jour de `current`.
- B3 : identifiants uniques, snapshot `.incomplete` avant validation,
  contenu publié immuable et indépendant de `current`.
- B4 : restauration vers dossier séparé, préflight et `--dry-run`, aucun
  écrasement ni restauration en place.
- B5 : diagnostic read-only et campagne pilote.

Les contrats B2 à B5 sont des exigences futures : aucun moteur de copie ni
format JSON n'est exposé dans B1. Les erreurs futures utilisent contexte et
`errors.Is/As` lorsqu'un consommateur doit les distinguer. Les chemins utilisent
`filepath`, les contenus `io.Reader` et les opérations `context.Context`.

## Références et autorité

Les fichiers Dirloom ont été consultés comme source implémentée de séparation
des responsabilités et de workflow. Les docs Mosaera/Sonora ont été consultées
comme descriptions de conception ; leur lecture ne valide pas le code Sonora.
Les références et SHA de blobs consultés figurent dans
[le dossier B0](validation/b0.md). Les décisions normatives sont celles de ce
dépôt, de ses ADR et de ses tests.

# Architecture Savewarden

Décision propre à Savewarden : **FSD-inspired, Go-idiomatic**, documentée dans
[ADR 0001](adr/0001-fsd-inspired-go.md). Mosaera est une inspiration de
conception, pas une spécification à laquelle le produit prétend être conforme.

## Existant B0

```text
cmd/savewarden → internal/app → internal/cli → Cobra
```

`main` fournit contexte, arguments, version et streams ; `app.Run` assemble
une nouvelle arborescence CLI à chaque invocation et mappe les erreurs vers
le code de sortie. `cli` possède les flags et le texte affiché. Aucun accès
disque, service métier ou découverte de configuration dans ce parcours.

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
sont contrôlés pour le code de production dans `internal/app/architecture_test.go`, y compris pour les
fichiers réservés à un OS. Tout ajout de dépendance externe aux entités doit
changer explicitement le contrat et son ADR.

Ne créer `entities/game`, `entities/backup`, `features/*`, `platform/filesystem`
ou `platform/manifests` que lorsqu'un fichier utile existe. Aucun `shared`
n'est nécessaire dans B0. Les détails Windows seront isolés avec build tags.

## Contrats à stabiliser

- B1 : YAML `schema_version`, parsing strict, champs inconnus et clés
  dupliquées rejetés, source de vérité sélectionnée par l'utilisateur,
  chemins confinés, liens/jonctions refusés par défaut, inventaire déterministe.
- B2 : destination protégée, plan et staging, paths/tailles/SHA-256,
  détection de changements concomitants, receipt et rollback Windows conçu
  avant toute mise à jour de `current`.
- B3 : identifiants uniques, snapshot `.incomplete` avant validation,
  contenu publié immuable et indépendant de `current`.
- B4 : restauration vers dossier séparé, préflight et `--dry-run`, aucun
  écrasement ni restauration en place.
- B5 : diagnostic read-only et campagne pilote.

Ces contrats sont des exigences futures : aucun parser, moteur de copie ou
format JSON n'est exposé dans B0. Les erreurs futures utilisent contexte et
`errors.Is/As` lorsqu'un consommateur doit les distinguer. Les chemins utilisent
`filepath`, les contenus `io.Reader` et les opérations `context.Context`.

## Références et autorité

Les fichiers Dirloom ont été consultés comme source implémentée de séparation
des responsabilités et de workflow. Les docs Mosaera/Sonora ont été consultées
comme descriptions de conception ; leur lecture ne valide pas le code Sonora.
Les références et SHA de blobs consultés figurent dans
[le dossier B0](validation/b0.md). Les décisions normatives sont celles de ce
dépôt, de ses ADR et de ses tests.

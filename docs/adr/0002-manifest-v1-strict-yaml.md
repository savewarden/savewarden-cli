# ADR 0002 — Manifeste v1 : décodage strict et dépendance YAML

Date : 12 octobre 2026. Statut : accepté pour B1, décision D3 du plan B1 v0.2.

## Contexte

`inspect` lit un manifeste écrit à la main pour décider quels dossiers
inventorier. Une lecture permissive transformerait une faute de frappe en
source silencieusement ignorée ou en chemin mal interprété. YAML offre des
constructions ambiguës : documents multiples, ancres et alias, clés de fusion,
tags, booléens YAML 1.1 comme `yes`, conversions implicites de type.

`gopkg.in/yaml.v3` n'est plus maintenu ; l'organisation YAML publie son
successeur sous `go.yaml.in/yaml/v3`. Cette bibliothèque ne détecte les clés
dupliquées qu'en décodant vers une map ou une structure, pas sur l'arbre de
nœuds, et `KnownFields` ne s'applique pas à ce décodage.

## Décision

- Dépendre de `go.yaml.in/yaml/v3` **v3.0.5**, version exacte dans `go.mod`
  et `go.sum`, importée uniquement par `internal/platform/manifests`. Le test
  d'architecture refuse cet import ailleurs.
- N'utiliser la bibliothèque que comme **analyseur** vers `yaml.Node`.
  Décoder les champs à la main, avec un schéma fermé :
  - tous les champs obligatoires, aucun défaut implicite ;
  - champs inconnus et clés dupliquées refusés à tous les niveaux ;
  - types vérifiés sur le tag résolu : chaîne `!!str`, entier `!!int`
    décimal, booléen `!!bool` littéral `true` ou `false` ;
  - ancres, alias, clés de fusion, tags explicites et documents multiples
    refusés avant tout décodage.
- Contrôler l'octet avant YAML : 64 Kio au plus, UTF-8 valide, sans NUL,
  BOM UTF-8 et CRLF tolérés, UTF-16 refusé.
- Valider la sémantique dans l'entité `game`, sans I/O. La grammaire des
  chemins Windows y est écrite sans `path/filepath`, pour un verdict identique
  sur tous les hôtes ; un test Windows la confronte à `filepath`.
- Rapporter la première erreur avec fichier, ligne, colonne et champ.
- Le schéma v1 reste **provisoire jusqu'à `v0.1.0`** : B2 pourra ajouter
  `strategy`, `target` et `exclude` sans changer `schema_version`.

## Alternatives

- `KnownFields(true)` sur une structure Go : refuse les champs inconnus mais
  accepte `yes` pour un booléen et laisse passer alias et fusions.
- `github.com/goccy/go-yaml` en mode strict : viable, mais nouvelle
  dépendance plus large pour un schéma de cinq champs ; le décodage manuel
  reste nécessaire pour les règles de type.
- JSON ou TOML : moins de pièges, mais le document fondateur et les échanges
  existants utilisent YAML.

## Conséquences

Le décodeur fait environ 250 lignes et doit évoluer avec chaque champ. Les
manifestes écrits pour B1 restent lisibles tant que le schéma ne change pas ;
un changement incompatible avant `v0.1.0` sera annoncé dans le CHANGELOG.
Les entrées hostiles du plan B1 §7.1 sont couvertes par
`internal/platform/manifests/decode_test.go`.

# Savewarden — Plan B1 : manifeste strict et `inspect` en lecture seule

> **Statut : REVUE TECHNIQUE FAVORABLE SOUS RÉSERVES — GO humain B1 distinct requis**
> Version du plan : `0.2 — revue` — 12 octobre 2026
> Lot : **B1**, cible `release/v0.1.0-a1`
> Cadre : [plan directeur v0.1](Savewarden_Plan_Bootstrap_Architecture_GitOps_v0.1.md) §7 B1 et §8,
> [AGENTS.md](../AGENTS.md), [modèle de sécurité](../docs/safety-model.md).

Ce plan suit le modèle en dix points du plan directeur (§8), enrichi des
conditions de la revue technique. Les décisions D1–D8 du §11 ont reçu un avis
technique ; le lancement de B1 demeure soumis au **GO explicite du propriétaire**
et à la fusion validée de la PR documentaire B0 #1.

---

## 0. Arbitrages et conditions issus de la revue (12 octobre 2026)

| Décision | Avis technique à retenir pour B1 |
| --- | --- |
| D1 | **Retenue :** code `2` pour un rapport complet avec anomalies, `0` propre, `1` fatal ; ADR 0003 et tests de contrat. |
| D2 | **Retenue :** JSON différé à B2, mais le `Report` interne est typé, indépendant du rendu et prépare une sérialisation explicite. Un receipt de sauvegarde et un rapport d'inspection sont deux objets distincts. |
| D3 | **Retenue sous preuve :** `go.yaml.in/yaml/v3` sur version exacte ; rejet explicite des constructions YAML ambiguës avant décodage. |
| D4 | **Retenue :** UTF-8 avec BOM optionnel, CRLF accepté, taille ≤ 64 Kio. |
| D5 | **Retenue comme limite B1 :** `game-install` uniquement pour Odyssey ; ne pas annoncer un manifeste Windows universel ni figer le contrat définitif. |
| D6 | **Retenue :** aucune lecture de `CPY.ini` en B1. |
| D7 | **Retenue :** une PR B1 avec commits atomiques ; points de contrôle après le manifeste puis le moteur, avant merge. |
| D8 | **Retenue avec compatibilité :** évaluer `errorlint` et `gosec`, activer les règles pertinentes avec exclusions justifiées, sinon documenter précisément le report ; conserver les gates B0. |

### Conditions techniques bloquantes pour valider la future PR B1

1. **Ne pas confondre confinement et non-suivi des liens :** `os.Root`
   confine les chemins à un arbre mais **peut suivre des symlinks internes**.
   La politique `no-follow` de tous les composants (symlink, junction,
   reparse point, placeholder) exige des vérifications spécifiques,
   l'inspection des handles réellement ouverts et des tests de remplacement
   concurrent. Voir [documentation de `os.Root`](https://go.dev/src/os/root.go).
2. **Faire évoluer le port FileSystem** : les seules fonctions sur chemins
   absolus `Lstat`, `ReadDir`, `Open` ne matérialisent pas la garantie de
   confinement. Concevoir une session/portée ouverte sur un ancrage contrôlé,
   avec chemins relatifs et identité vérifiée sur handle. Une ouverture
   ambiguë échoue fermée ; aucun accès hors périmètre autorisé.
3. **Ne pas surestimer le garde AST** : il interdit des APIs mutantes connues,
   mais ne démontre pas l'absence de toute écriture ou effet indirect.
   Prévoir en plus des tests de non-mutation sur données synthétiques, des
   tests d'exécution dans un répertoire non inscriptible lorsque possible,
   et des scénarios d'attaque sur les chemins.
4. **Spécifier les anomalies :** source obligatoire absente → rapport/code 2 ;
   source facultative absente → information/code 0 si aucun autre défaut ;
   fichier inaccessible ou reparse → code 2 ; manifeste invalide/annulation
   → code 1 sans rapport présenté comme complet.
5. **Définir la portabilité honnêtement :** l'accès aux chemins absolus Windows
   et les tests de bout en bout sont Windows-only ; sur Linux/macOS, couvrir
   validation pure, ports/fakes et cas d'intégration applicables. Ne pas
   traiter `filepath` Unix comme validateur de chemins Windows.
6. **Formaliser l'intégrité d'inventaire :** le contrôle taille/mtime/identité
   détecte de nombreux changements mais pas toute mutation conservant ces
   métadonnées ; la garantie doit être annoncée comme best-effort B1,
   non comme preuve atomique d'un instantané cohérent. Prévoir les races
   et l'absence de rapport « complet » en cas d'annulation.

La revue technique de ce document **n'autorise pas** un merge, un tag, une
release ou un accès aux sauvegardes privées sans autorisation dédiée.

---

## 1. Contexte et preuves

| Élément | État au 12 octobre 2026 |
| --- | --- |
| `main` et `release/v0.1.0-a1` | `25d1cc2`, bootstrap B0, CI verte sur 7 checks |
| Protections | Rulesets `main` (24944424) et `release` (24944427) actifs, sans bypass |
| Human GO B0 | Donné le 12 octobre 2026, consigné dans `docs/validation/b0-post-push.md` |
| PR ouverte | [#1](https://github.com/savewarden/savewarden-cli/pull/1), documentaire, vers `release/v0.1.0-a1` |
| Code existant | `cmd/savewarden → internal/app → internal/cli`, aide et version seulement |
| Dépendances | Go 1.27.2, Cobra 1.10.2 |

Acquis réutilisables :

- `internal/app/architecture_test.go` connaît déjà les couches `entities`,
  `features`, `platform` et autorise `platform/manifests` à importer une
  bibliothèque YAML. Aucune frontière nouvelle n'est nécessaire.
- Les tests B0 fixent le style : tableaux de cas, smoke du vrai binaire dans
  un dossier avec espaces et Unicode, sentinelle inchangée, exemples README
  vérifiés.

Points de la revue indépendante reportés vers B1 :

- **Linting :** évaluer `errorlint` et `gosec` avant le premier accès disque.
- **Fixtures :** `.gitignore` masque `saves/`, `current/`, `snapshots/` et
  `*.save` ; les arborescences de test doivent être construites dans le code.

## 2. Objectif observable, non-objectifs

### Objectif

```text
savewarden inspect --manifest <chemin>
```

1. Lit un `manifest.yaml` v1 avec un décodage **strict**.
2. Valide l'identité du jeu, l'installation et chaque source déclarée.
3. Résout chaque source sous l'installation, sans suivre aucun lien ni
   point d'analyse (*reparse point*), y compris dans les dossiers parents.
4. Produit un inventaire **déterministe** : chemin relatif, taille, date de
   modification UTC et SHA-256 de chaque fichier, puis les anomalies.
5. N'effectue **aucune écriture applicative**, création, mutation ou
   exécution. La lecture peut modifier l'horodatage d'accès selon le système
   de fichiers ; les placeholders susceptibles de déclencher des téléchargements
   sont refusés.

Le premier usage réel est le pilote Odyssey : vérifier, en lecture seule,
la source active déclarée et sa stabilité entre deux exécutions.

### Non-objectifs B1

- Aucune copie, synchronisation, snapshot, restauration ou réparation.
- Aucune découverte automatique : pas de lecture de `CPY.ini`, pas de
  recherche dans le registre ou les launchers, pas de catalogue communautaire.
- Aucune comparaison avec `D:\Ga` ni rapprochement de copies historiques.
- Aucune variable d'environnement ni dossier connu Windows
  (`%USERPROFILE%`, `Saved Games`) : prévus pour Cyberpunk 2077, en B5.
- Aucune sortie JSON, si la décision D2 est retenue.
- Aucun fichier de configuration, coffre par défaut, cache ou journal écrit.
- Aucune promesse fonctionnelle Linux/macOS : la CI y teste la logique
  générique, pas le support produit.

## 3. Décisions d'architecture et ADR

### 3.1 Packages créés

```text
cmd/savewarden ──► app ──► cli ──► features/inspect ──► entities/game
                    │                     │
                    │                     └─ ports : ManifestReader, FileSystem
                    ├──► platform/manifests  ──► entities/game, YAML
                    └──► platform/filesystem ──► bibliothèque standard
```

| Package | Rôle | Dépendances |
| --- | --- | --- |
| `internal/entities/game` | `Manifest`, `Game`, `Installation`, `Source`, `Kind`, `Base` ; validation sémantique pure | stdlib |
| `internal/platform/manifests` | Lecture d'un fichier, contrôles d'encodage et de taille, décodage YAML strict vers `game.Manifest` | stdlib, YAML |
| `internal/platform/filesystem` | Implémente une session ancrée, fondée sur `os.Root` pour le confinement, **complétée** de contrôles `no-follow` et reparse/handle propres à Windows ; fichier `_windows.go` sous build tag | stdlib, `syscall` |
| `internal/features/inspect` | Orchestration : charger, valider, résoudre, parcourir, hacher, classer les anomalies ; types `Report`, `Entry`, `Finding` | `entities/game`, stdlib |
| `internal/cli` | Commande `inspect`, flag `--manifest`, rendu texte, codes de sortie | Cobra, `features/inspect` |
| `internal/app` | Assemble adaptateurs et service, mappe les erreurs vers le code de sortie | tous |

Pourquoi ce découpage :

- `game.Manifest` doit vivre dans `entities` : l'adaptateur `platform/manifests`
  le produit et ne peut pas importer une feature.
- Les types d'inventaire restent dans `features/inspect` tant que seul
  `inspect` les utilise. Ils migreront vers `entities/` en B2, quand `sync`
  en aura besoin pour vérifier une copie. Pas d'entité prématurée.
- La politique de sécurité (ne pas suivre, classer, détecter un changement)
  vit dans la feature, derrière un port. Un faux système de fichiers peut
  donc simuler jonctions, verrous et fichiers remplacés **sur les trois OS**.
- Le port ne renvoie que des types stdlib (`fs.FileInfo`, `fs.DirEntry`,
  `io.ReadCloser`) : l'adaptateur n'importe pas la feature.

### 3.2 Port `FileSystem` (forme indicative, à valider par tests)

Le port doit exposer une **portée ancrée**, pas seulement des opérations
indépendantes sur chemins absolus. La forme finale reste à décider en B1 :

```go
type FileSystem interface {
    OpenScope(volumeRoot string) (Scope, error)
}

type Scope interface {
    Lstat(relative string) (fs.FileInfo, error)
    ReadDir(relative string) ([]fs.DirEntry, error)
    OpenRegular(relative string) (File, error)
    SameFile(a, b fs.FileInfo) bool
    Close() error
}
```

L'ancrage (racine du volume ou répertoire préalablement validé) ne dispense
pas d'inspecter **chacun des composants depuis la racine du volume jusqu'à
la source**. Le contrat d'`OpenRegular` doit empêcher la lecture d'une cible
atteinte via symlink/junction/reparse, y compris après un changement entre
la validation des chemins et l'ouverture. Après ouverture, contrôler
l'identité et les attributs du handle avant de lire. Si une garantie `no-follow`
suffisante ne peut pas être établie sur Windows, **ne pas publier B1 comme
conforme** : réduire explicitement l'étendue supportée et soumettre
l'écart à une revue technique.

`os.Root` est un mécanisme de **confinement**, pas une interdiction de suivre
les liens internes : sa documentation précise que ses méthodes suivent des
liens symboliques tant qu'ils restent sous la racine. Les tests doivent donc
établir les garanties additionnelles promises. Sous Windows, la présence de
`FILE_ATTRIBUTE_REPARSE_POINT` est contrôlée indépendamment de
`GODEBUG=winsymlink`. Aucun faux système de fichiers ne remplace les tests
réels Windows avec jonctions, placeholders simulés et remplacements concurrents.

### 3.3 ADR à produire

- **ADR 0002 — Manifeste v1 : décodage strict et dépendance YAML.** Choix de
  la bibliothèque, politique d'encodage, rejet des alias, tags, documents
  multiples, clés dupliquées et booléens YAML 1.1. Statut provisoire du
  schéma v1 jusqu'à `v0.1.0`.
- **ADR 0003 — Contrat de sortie d'`inspect`** (D1 retenue) :
  ajout du code 2 au contrat B0, qui ne connaît que 0 et 1.

Le graphe de couches ne change pas : pas d'ADR de frontière.

## 4. Contrats

### 4.1 Manifeste v1

```yaml
schema_version: 1

game:
  id: assassins-creed-odyssey
  name: "Assassin's Creed Odyssey"

installation:
  path: 'D:\Games\Assassins Creed - Odyssey'

sources:
  - id: saves
    kind: save
    base: game-install
    path: saves
    required: true
```

| Champ | Règle |
| --- | --- |
| `schema_version` | Entier `1` exact ; absent, `"1"`, `1.0` ou `2` rejetés |
| `game.id` | `^[a-z0-9]+(-[a-z0-9]+)*$`, 64 caractères au plus |
| `game.name` | Non vide, 200 caractères au plus, sans caractère de contrôle |
| `installation.path` | Absolu avec lettre de lecteur ; rejet des chemins relatifs, `C:x`, `\x`, UNC, `\\?\`, `\\.\`, `%VAR%` |
| `sources` | Au moins une ; `id` unique, comparaison insensible à la casse |
| `sources[].kind` | `save`, `profile`, `config`, `controls`, `other` |
| `sources[].base` | `game-install` seulement en v1, décision D5 |
| `sources[].path` | Relatif et canonique selon **la grammaire Windows** ; `filepath.IsLocal` ne vaut que sur hôte Windows, jamais comme preuve cross-OS ; rejet de `..`, `.`, vide, flux alternatif `a:b`, segment finissant par un point ou un espace, noms réservés (`CON`, `NUL`, `COM1`, etc.) |
| `sources[].required` | `true` ou `false` littéral ; `yes`, `on`, `1` rejetés |

Règles globales du fichier :

- 64 Kio au plus, UTF-8 valide, sans octet NUL. BOM UTF-8 accepté et CRLF
  accepté, décision D4 ; UTF-16 rejeté avec un message explicite.
- Un seul document YAML. Ancres, alias, clés de fusion `<<` et tags
  explicites rejetés. Champs inconnus et clés dupliquées rejetés à tous les
  niveaux.
- Erreurs actionnables : champ concerné, ligne et colonne quand disponibles.
  Aucune correction silencieuse, par exemple un chemin non canonique n'est
  jamais nettoyé puis accepté.
- Les sources ne doivent ni coïncider ni se contenir. La comparaison se fait
  par identité de fichier (`SameFile`) sur les dossiers existants, ce qui
  couvre la casse et les noms courts 8.3, puis par segments insensibles à la
  casse pour les chemins absents.

Champs reportés : `strategy`, `target`, `exclude` arrivent avec B2 ;
`platform`, variantes et profils arrivent quand un pilote les exige.

### 4.2 Parcours et inventaire

- Chaque composant, de la racine du volume jusqu'à la source, est contrôlé
  dans une portée ancrée. Tout lien ou reparse point refuse la source ;
  la validation doit se prolonger jusqu'au **handle effectivement ouvert**.
  `os.Root` seul ne garantit pas le non-suivi des liens internes.
- Dans la source, seuls les dossiers et fichiers réguliers sont parcourus ou
  lus. Tout autre type est signalé, jamais suivi ni lu. Cela inclut les
  placeholders OneDrive, dont la lecture déclencherait un téléchargement.
- Chaque fichier est haché en flux. Taille et date sont relues avant et
  après lecture ; une différence produit l'anomalie « modifié pendant
  l'inspection ». Après ouverture, identité et attributs du handle sont
  contrôlés. Un changement de contenu qui conserve artificiellement taille,
  date et identité peut rester indétectable : B1 n'est pas un snapshot atomique.
- Un fichier verrouillé ou illisible produit une anomalie ; le parcours
  continue sur les autres fichiers.
- Deux noms qui ne diffèrent que par la casse dans une même source produisent
  une anomalie, car une future copie Windows les ferait entrer en collision.
- Ordre de sortie : chemins relatifs avec séparateur `/`, triés octet par
  octet. Aucun horodatage d'exécution dans la sortie, pour qu'elle reste
  comparable d'une exécution à l'autre.
- `context.Context` est vérifié entre fichiers et pendant les lectures
  longues. Une interruption produit une erreur, jamais un rapport partiel
  présenté comme complet.

### 4.3 Sortie et codes de retour

Sortie texte en anglais, UTF-8, sans couleur, terminée par LF. Les noms
contenant des caractères non imprimables sont échappés.

```text
manifest: <chemin du manifeste>
game: assassins-creed-odyssey (Assassin's Creed Odyssey)
installation: D:\Games\Assassins Creed - Odyssey

source saves (save, required): D:\Games\Assassins Creed - Odyssey\saves
  files: 14  bytes: <total>
  <sha256>  <taille>  <date UTC>  CPY_SAVES/<profil>/UPLAY/5059/1.save
  ...

findings: none
```

| Code | Signification, si D1 est retenue |
| --- | --- |
| `0` | Inventaire complet, aucune anomalie |
| `1` | Erreur d'invocation, manifeste invalide ou erreur fatale ; diagnostic unique sur stderr, rien sur stdout, comme en B0 |
| `2` | Rapport complet sur stdout avec anomalie bloquante : source requise absente, lien refusé, fichier illisible ou modifié ; une ligne de synthèse sur stderr. Source facultative absente seule = information et code `0` |

## 5. Périmètre des fichiers

À créer :

```text
internal/entities/game/{manifest.go, validate.go, validate_test.go}
internal/platform/manifests/{decode.go, decode_test.go, testdata/*.yaml}
internal/platform/filesystem/{filesystem.go, reparse_windows.go,
                              reparse_other.go, filesystem_test.go,
                              filesystem_windows_test.go}
internal/features/inspect/{inspect.go, ports.go, walk.go, report.go,
                           inspect_test.go, fakefs_test.go}
internal/cli/inspect.go
docs/manifest-v1.md
docs/commands/inspect.md
docs/adr/0002-manifest-v1-strict-yaml.md
docs/validation/b1.md
```

À modifier : `internal/cli/root.go`, `internal/app/app.go` et ses tests,
`internal/app/architecture_test.go`, `cmd/savewarden/main_test.go`,
`.golangci.yml`, `go.mod`, `go.sum`, `README.md`, `AGENTS.md`,
`CHANGELOG.md`, `docs/architecture.md`, `docs/safety-model.md`,
`docs/pilots/odyssey.md`.

Changements interdits :

- Toute écriture disque dans le code de production, y compris journaux,
  caches, configuration ou fichiers temporaires.
- `os/exec` ou toute exécution de processus dans le code de production.
- Toute utilisation de `os.Root` comme preuve unique de non-suivi de liens :
  les liens internes peuvent être suivis et demandent un contrôle additionnel.
- Les packages `features/sync`, `snapshot`, `restore`, `doctor` et tout
  dossier vide.
- Toute modification des rulesets, de la CI hors ajout de contrôles, ou des
  versions d'Actions.
- Tout accès agent à `D:\Ga`, `D:\Savewarden`, `E:\GMS` ou aux clés SSH.

## 6. Plan d'exécution

Après **fusion de la PR documentaire #1** et **GO humain explicite sur ce
plan B1 révisé**, créer `feat/b1-inspect` depuis `release/v0.1.0-a1` et une
PR cohérente vers celle-ci, en commits atomiques. Chaque commit compile et passe les tests.

| # | Commit | Contenu | Point de contrôle |
| --- | --- | --- | --- |
| 1 | `test(arch): guard read-only production code` | Test AST : production sans `os/exec`, sans appels mutants (`Create`, `OpenFile`, `WriteFile`, `Mkdir*`, `Remove*`, `Rename`, `Link`, `Symlink`, `Chmod`, `Chtimes`, `Truncate`) ; liste blanche des dépendances externes par couche ; cas négatifs | — |
| 2 | `chore(lint): evaluate errorlint and gosec` | Activation, exclusions justifiées en commentaire, ou report documenté si le bruit est excessif | — |
| 3 | `feat(game): add manifest v1 model and validation` | Entités, validation pure, tableaux de cas | — |
| 4 | `feat(manifests): strict YAML decoding` | Dépendance YAML épinglée, contrôles d'encodage, décodage strict, fixtures hostiles, ADR 0002, `docs/manifest-v1.md` | **Revue du contrat manifeste** |
| 5 | `feat(inspect): read-only inventory service` | Ports, parcours, hachage, anomalies, tests sur faux système de fichiers | — |
| 6 | `feat(filesystem): confined no-follow adapter` | `os.Root`, contrôle des reparse points sous Windows, tests d'intégration en dossier temporaire | — |
| 7 | `feat(cli): add inspect command` | Commande, rendu, codes de sortie, câblage `app`, tests CLI, smoke binaire | **Revue du comportement de bout en bout** |
| 8 | `docs: document B1 inspect` | README, architecture, sécurité, pilote, AGENTS, CHANGELOG, `docs/commands/inspect.md`, ADR 0003 si D1 | — |
| 9 | `docs: add B1 validation report` | `docs/validation/b1.md` : commandes, résultats, CI, limites | Revue finale, puis Human GO |

Le test manuel Odyssey (§9) a lieu après la CI verte et avant le merge.
Le code B1 ne doit accéder aux sauvegardes réelles qu'après une autorisation
distincte du propriétaire et l'exécution prévue sur son environnement.
Le résultat anonymisé complète `docs/validation/b1.md`.

## 7. Matrice de tests

Toutes les données sont synthétiques et construites en `t.TempDir()` ou en
mémoire. Seuls les manifestes YAML vivent en `testdata/`, hors des motifs
ignorés par Git.

### 7.1 Manifeste (unitaires, tous OS)

- **Structure :** fichier vide, commentaires seuls, deux documents, champ
  inconnu à chaque niveau, clé dupliquée à chaque niveau, mauvais types.
- **YAML hostile :** ancres et alias, clé de fusion `<<`, tags `!!binary` et
  `!custom`, booléens `yes`/`on`, tabulations, chaînes très longues.
- **Encodage :** 64 Kio plus un octet, octets non UTF-8, octet NUL, BOM UTF-16,
  BOM UTF-8 et CRLF acceptés.
- **Champs :** chaque règle du tableau §4.1, cas valide et invalide.
- **Chemins Windows :** lecteur relatif, UNC, préfixes de périphérique, flux
  alternatifs, noms réservés, points et espaces finaux. Ces cas tournent sur
  le runner Windows ; la sémantique des chemins dépend de l'OS hôte.
- **Erreurs :** le message cite le champ et la ligne ; une seule erreur par
  invocation côté CLI.

### 7.2 Inspect (unitaires, faux système de fichiers, tous OS)

- Ordre identique quel que soit l'ordre renvoyé par `ReadDir`.
- Lien vers un fichier, lien vers un dossier, entrée irrégulière simulant une
  jonction, périphérique : signalés, jamais ouverts.
- Lien dans un dossier parent de l'installation ou de la source : refus.
- Fichier verrouillé, permission refusée : anomalie, parcours poursuivi.
- Fichier modifié pendant la lecture, fichier remplacé entre `Lstat` et
  `Open` : anomalie.
- Source requise absente, source optionnelle absente, source qui est un
  fichier : comportements distincts.
- Sources qui coïncident ou se contiennent, y compris par casse différente.
- Noms en collision de casse, dossier vide, fichier vide avec le SHA-256
  attendu `e3b0c442…b855`, 10 000 fichiers, arborescence profonde.
- Annulation du contexte en cours de parcours : erreur, aucun rapport complet.
- Échappement des noms non imprimables.

### 7.3 Adaptateur filesystem (intégration, vrais dossiers temporaires)

- Tous OS : fichiers réguliers, espaces et Unicode, lecture seule, lien
  symbolique ignoré si sa création n'est pas autorisée.
- Linux et macOS : fichier en mode `000` illisible, sauf exécution en root.
- Windows :
  - jonction créée par `mklink /J` dans le test, détectée et non suivie ;
  - chemin de plus de 260 caractères lu ;
  - fichier verrouillé sans partage via `syscall.CreateFile`, signalé
    comme illisible ;
  - `SameFile` vrai entre deux casses d'un même dossier.
  - remplacement concurrent par junction/symlink entre `Lstat` et ouverture :
    refus, jamais lecture silencieuse de la cible.
  - symlink pointant **à l'intérieur** de l'ancrage `os.Root` : refus malgré
    le confinement possible ; test qui aurait échoué avec `os.Root` seul.

### 7.4 CLI et binaire

- `inspect` sans `--manifest`, manifeste absent, manifeste invalide,
  argument positionnel en trop : code 1, diagnostic unique, stdout vide.
- Inventaire propre : code 0, sortie exacte.
- Inventaire avec anomalie : code 2 et sortie attendue, si D1 est retenue.
- L'aide racine liste `inspect` et ne mentionne plus « help and version only ».
- Les cas B0 restent verts, dont `sync` en commande inconnue.
- Smoke du vrai binaire :
  - installation synthétique avec espaces et Unicode ;
  - empreinte complète de l'arborescence avant et après : chemins, tailles,
    dates, contenus identiques ;
  - aucun nouveau fichier dans le dossier courant ;
  - deux exécutions successives donnent une sortie identique octet par octet.

### 7.5 Gates

Inchangés depuis `CONTRIBUTING.md`, plus le test AST de lecture seule et les
linters retenus au commit 2. Les 7 checks requis doivent être verts sur le SHA
exact de la PR. `go test -race` reste prouvé par le job Linux.

## 8. Documentation livrée avec le code

| Document | Contenu |
| --- | --- |
| `docs/manifest-v1.md` | Référence du schéma, règles, exemples valides et rejetés, champs reportés, statut provisoire |
| `docs/commands/inspect.md` | Usage, sortie, codes de retour, garanties, limites |
| `README.md` | État B1, commande disponible, exemple vérifié par test |
| `docs/architecture.md` | Section « Existant B1 », packages et ports réels |
| `docs/safety-model.md` | Garanties B1 implémentées et testées, invariants encore futurs |
| `docs/pilots/odyssey.md` | Protocole manuel B1 en lecture seule et résultat anonymisé |
| `AGENTS.md` | Périmètre B1, interdits maintenus, test pilote réservé au propriétaire |
| `CHANGELOG.md` | `Added` : manifeste v1 et `inspect` ; `Limits` mises à jour |
| ADR 0002, ADR 0003 si D1 | Décisions durables |
| `docs/validation/b1.md` | Preuves du lot |

Limites à documenter honnêtement :

- La lecture peut mettre à jour la date de dernier accès selon les réglages
  NTFS ; le contenu n'est jamais modifié.
- La sortie contient des chemins locaux et parfois un nom de profil. Elle doit
  être anonymisée avant tout partage public.
- Un SHA-256 identique prouve l'identité des octets, pas la capacité du jeu
  à charger la partie.

## 9. Rollout, rollback et pilote

- Le code ne rejoint que `release/v0.1.0-a1` par PR. Aucun tag ni release.
- Rollback : revert de la PR sur la release branch ; aucune donnée
  utilisateur n'est concernée, puisque rien n'est écrit.
- **Pilote Odyssey, lecture seule, exécuté par le propriétaire** ou par
  l'agent seulement sur autorisation explicite et nominative :
  1. Fermer le jeu et le launcher.
  2. Écrire le manifeste hors du dépôt, avec le chemin réel de l'installation.
  3. Lancer `inspect` deux fois ; les deux sorties doivent être identiques.
  4. Attendu : code 0, les 14 fichiers `.save` connus au 9 octobre ou plus si
     le jeu a sauvegardé depuis, aucune anomalie.
  5. Optionnel, étape B du pilote : sauvegarder en jeu, relancer, comparer.
  6. Consigner dans `docs/validation/b1.md` le nombre de fichiers, le code de
     sortie et les anomalies, sans hash individuel, nom de profil ni chemin
     personnel.

## 10. Critères d'acceptation

1. `savewarden inspect --manifest <m>` produit l'inventaire décrit en §4 sur
   une installation synthétique, avec une sortie identique entre deux
   exécutions.
2. Chaque règle du §4.1 a au moins un test de rejet et un test d'acceptation.
3. Aucun lien, jonction ou reparse point n'est suivi ni lu, dans la source
   comme dans ses parents ; prouvé par faux système de fichiers tous OS,
   jonction/symlink internes et remplacement concurrent réels sur Windows,
   et inspection post-ouverture du handle. `os.Root` seul est insuffisant.
4. Le test AST échoue si le code de production importe `os/exec` ou appelle
   une fonction mutante ; ses cas négatifs sont testés.
5. Le smoke binaire confirme la **non-mutation applicative** sur ses
   fixtures : contenu, arborescence et dossier courant inchangés ; le garde
   AST, des restrictions d'écriture OS et ces tests se complètent sans
   constituer une preuve formelle universelle.
6. Les 7 checks requis sont verts sur le SHA exact de la PR, avec lien du run.
7. Documentation, CHANGELOG et ADR cohérents avec le code livré, exemples du
   README vérifiés par test.
8. Le pilote Odyssey est exécuté en lecture seule et consigné, ou son
   absence est explicitement déclarée comme limite.
9. Aucun accès agent à `D:\Ga`, `D:\Savewarden` ou `E:\GMS` hors autorisation
   explicite ; aucun save, `CPY.ini` ou chemin personnel dans le diff.

## 11. Décisions à valider par le propriétaire

| # | Question | Recommandation | Alternative |
| --- | --- | --- | --- |
| D1 | Code de sortie quand le rapport contient des anomalies | **RETENU : code 2** pour anomalies bloquantes, rapport stdout et synthèse stderr, ADR 0003 ; information facultative seule = code 0 | Code 1 avec rapport : rompt la règle B0 « rien sur stdout en erreur » |
| D2 | Sortie JSON | **RETENUE : reportée à B2**, conserver un `Report` interne typé indépendant du texte ; le JSON d’inspection et les receipts auront des schémas distincts | JSON dès B1 : deux formats à stabiliser |
| D3 | Bibliothèque YAML | **RETENU sous tests : `go.yaml.in/yaml/v3`**, successeur maintenu de `gopkg.in/yaml.v3`, déjà prévu par le test d'architecture ; version exacte vérifiée à la PR | `github.com/goccy/go-yaml` en mode strict |
| D4 | Encodage du manifeste | **RETENU : UTF-8, BOM toléré, CRLF toléré** : un manifeste écrit au Bloc-notes doit passer | UTF-8 sans BOM seulement |
| D5 | Bases de chemins en v1 | **RETENU pour B1 : `game-install` seulement** | Ajouter dès B1 les dossiers connus Windows |
| D6 | Découverte par `CPY.ini` | **RETENU : hors B1** ; le manifeste déclare le chemin, `inspect` le vérifie | Lire `SavePath` dès B1 |
| D7 | Découpage | **RETENU : une PR pour le lot**, commits atomiques, deux points de contrôle | Deux PR : contrat manifeste, puis `inspect` |
| D8 | Lint | **RETENU après évaluation de compatibilité : `errorlint` + `gosec`** et exclusions ciblées, plus test AST et tests dynamiques de non-mutation | Test AST seul |

## 12. Risques

| Risque | Effet | Parade |
| --- | --- | --- |
| La bibliothèque YAML tolère ce que le contrat refuse (doublons, booléens 1.1) | Manifeste ambigu accepté | Pré-passe sur l'arbre de nœuds et tests écrits avant le décodeur |
| `GODEBUG=winsymlink=0` restaure l'ancienne classification Windows | Jonction vue comme lien ou dossier | Contrôle explicite de l'attribut reparse point dans l'adaptateur |
| Remplacement d'un fichier entre contrôle et ouverture | Lecture non autorisée ou lien suivi | Ancrage `os.Root` + interdiction de suivre les liens internes + vérification des attributs du handle ; fail closed et tests concurrents Windows |
| Placeholder OneDrive lu | Téléchargement déclenché | Entrées irrégulières jamais lues |
| Jeu ou antivirus qui verrouille un fichier | Inventaire incomplet | Anomalie explicite, code 2, jamais de succès annoncé |
| Noms courts 8.3 ou casse différente | Chevauchement de sources non détecté | `SameFile` sur les dossiers existants |
| Schéma v1 modifié par B2 | Manifestes B1 à réécrire | Statut provisoire jusqu'à `v0.1.0` écrit dans ADR 0002 et la doc |
| Données personnelles dans les sorties partagées | Fuite de chemins et de profils | Avertissement doc, aucune écriture de rapport par l'outil |
| Mainteneur unique | Revue limitée | Revue externe et Human GO consignés ; effet du paramètre `require_extra_approval_for_unattributed_changes` observé sur PR #1 |
| Extension de périmètre | Lot trop gros | Non-objectifs §2 opposables en revue |
| Validation Unix d’un chemin Windows | Acceptation d’un chemin invalide ou fake CI trompeur | Contrat Windows explicite, validation OS-aware, fakes sur Unix et intégrations Windows |
| AST présenté comme preuve de non-écriture | Garantie trompeuse | Contrôles d’import + vérification dynamique de non-mutation + limites documentées |

## 13. Rapport de restitution attendu

Branche et PR, SHA final, `git diff --stat`, commandes de gates avec
résultats exacts, lien du run CI et état des 7 checks, ADR créés, résultat du
pilote ou son absence, écarts au plan, risques résiduels, questions ouvertes,
recommandation GO ou NO-GO **sans auto-approbation**.

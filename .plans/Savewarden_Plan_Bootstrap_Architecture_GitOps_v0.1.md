# Savewarden — Plan directeur d'architecture, bootstrap et workflow agent

> **Statut : PROPOSITION À VALIDER — aucun dépôt modifié**  
> Version du plan : `0.1` — 9 octobre 2026  
> Produit : **Savewarden** — *Your progress. Always protected.*  
> Cadre : Windows d'abord, moteur Go indépendant de la CLI, local-first, pas de suppression silencieuse.  
> Chemin de travail : `D:\Forge\projects\savewarden`  
> Données réelles, hors dépôt : `D:\Savewarden` (ne jamais importer ces données dans Git).

## 1. Mandat et définition du succès

**Double objectif indissociable** : (1) résoudre le problème concret de centralisation, vérification et restauration des sauvegardes depuis l'ancien `D:\Ga` sans jamais imposer un import destructeur ; (2) industrialiser un outil utilisable par d'autres joueurs.

Le premier cas de référence est **Assassin's Creed Odyssey**, dont le répertoire actif déclaré dans `CPY.ini` est `E:\GMS\Assassins Creed - Odyssey\saves`. Son état actif de 14 fichiers a été copié et vérifié par SHA-256 vers `D:\Savewarden\games\assassins-creed-odyssey\current\saves`. Un snapshot vérifié existe : `20261009T183208-cacd3cae`. **Ne pas le réécrire ni le supprimer.** Les anciennes copies sous `D:\Ga` restent intactes et ne doivent pas décider de la source de vérité.

Succès de la première version : l'utilisateur peut **inspecter, synchroniser, produire un snapshot, restaurer vers un chemin séparé et diagnostiquer** le jeu à partir d'un manifeste versionné, sans script PowerShell ad hoc, avec tests et documentation. Une restauration en place nécessite des protections supplémentaires et une autorisation explicite : hors de l'incrément initial.

## 2. Sources de référence et niveaux d'autorité

- **Constats pratiques** : sorties PowerShell et `CPY.ini` d'Odyssey, inventaire `D:\Ga`, validation SHA-256 et snapshot.
- **Source inspirante implémentée** : [`dirloom/dirloom`](https://github.com/dirloom/dirloom), notamment `internal/`, `docs/architecture.md`, `CONTRIBUTING.md`, `docs/release-workflow.md`, `.github/workflows/{ci,release,update-packages}.yml`, `.goreleaser.yaml`.
- **Inspiration architecturale non encore normative** : [`Samuelbenny/mosaera`](https://github.com/Samuelbenny/mosaera), particulièrement `docs/mosaera-architecture-brand-foundation.md`, `docs/sonora-fsd-like-dirloom-blueprint.md` et `design/README.md`.
- **Décision pour Savewarden** : *FSD-inspired, Go-idiomatic* ; les contrats publics sont propres à Savewarden et n'affirment pas une conformité à une Mosaera Specification stabilisée.

Ne jamais confondre « décrit dans un blueprint » avec « testé dans le code actuel ».

## 3. Architecture de référence

### 3.1 Principes

1. **Frontières de domaine explicites** : une feature possède son orchestration ; les types métier transverses sont dans des entités partagées et stables.
2. **Dépendances unidirectionnelles** : les features dépendent d'entités et de ports étroits ; jamais de `cli`, de Cobra ou d'implémentations Windows concrètes. `app` assemble les dépendances.
3. **Aucun couplage inter-features par défaut** : pas de `sync` important `snapshot`; orchestration par un service supérieur si nécessaire, avec contrats explicites et ADR.
4. **Interface/adapter** : ports consommateur-définis (filesystem, horloge, hashing, stockage, lecteur de manifestes), adaptateurs plateforme séparés.
5. **Petit noyau, features verticales** : pas de `manager.go` tentaculaire, pas de package `utils` ou `common` fourre-tout, pas de conteneur DI ni de framework interne.
6. **Sans effets de bord implicites** : `inspect` et `doctor` sont read-only ; chaque opération mutante exige une destination explicite et un plan vérifiable.
7. **Go idiomatique** : packages par responsabilités pertinentes, API publique limitée, erreurs typées ou enrichies avec `errors.Is/As`, `context.Context`, `io.Reader`, chemins `filepath`, tests par tableaux.
8. **Portabilité par étapes** : produit Windows-first, compilation sur les trois OS ; les opérations propres à Windows sont isolées et conditionnées par build tags. Pas de prétention à la prise en charge fonctionnelle totale Linux/macOS avant tests.
9. **Mosaera-ready conceptuel** : documenter frontières, dépendances, ADR et invariants sous forme machine-vérifiable quand rentable ; aucune migration automatique anticipée.

### 3.2 Arborescence cible *évolutive* (ne pas créer de dossiers vides)

```text
savewarden/
├── .github/
│   ├── workflows/
│   │   ├── ci.yml                 # CI initiale
│   │   └── release.yml            # Quand la première release est prête
│   └── pull_request_template.md
├── cmd/savewarden/main.go
├── internal/
│   ├── app/                       # composition explicite, assemblage des adaptateurs
│   ├── cli/                       # Cobra, sorties, codes de retour
│   ├── entities/
│   │   ├── game/                  # identité de jeu, sources, variantes
│   │   └── backup/                # types snapshot / intégrité (au moment utile)
│   ├── features/
│   │   ├── inspect/               # incrément 1
│   │   ├── sync/                  # incrément 2
│   │   ├── snapshot/              # incrément 3
│   │   ├── restore/               # incrément 4
│   │   └── doctor/                # incrément 5
│   └── platform/
│       ├── filesystem/            # implémentation des ports d'accès disque
│       └── manifests/             # lecture stricte des YAML
├── docs/
│   ├── architecture.md
│   ├── gitops.md
│   ├── safety-model.md
│   ├── adr/                      # créé au premier ADR
│   └── pilots/odyssey.md
├── testdata/                      # uniquement des fixtures synthétiques
├── README.md
├── CONTRIBUTING.md
├── SECURITY.md
├── CHANGELOG.md
├── LICENSE                        # licence open source à choisir explicitement
├── .gitignore
├── .golangci.yml
├── .goreleaser.yaml                # lors de l'activation release
├── go.mod
└── go.sum                        # quand requis
```

**Important** : cet arbre est une carte des responsabilités, **pas** une liste de dossiers à générer immédiatement. L'agent crée les packages seulement lorsqu'il introduit un fichier utile. `internal/shared` n'est pas nécessaire tant qu'il n'existe aucun besoin transversal réel.

### 3.3 Graphe de dépendances

```text
cmd/savewarden ──► app ──► cli
                       └─► features/{inspect,sync,snapshot,restore,doctor}
                       └─► platform/{filesystem,manifests}
cli ─────────────────► features/* (API publiques)
features/* ──────────► entities/* + ports ciblés (interfaces)
platform/* ──────────► entities/* (si nécessaire)
entities/* ──────────► bibliothèque standard uniquement (par défaut)
```

Le graphe est indicatif : pas d'import cyclique, pas d'import direct `platform` dans une feature. La couche `app` assemble les dépendances, sans devenir un service métier géant.

### 3.4 Contrats à produire progressivement

- `manifest.yaml` versionné (`schema_version`) ; décodage strict, rejet des champs inconnus, des clés dupliquées et des entrées ambiguës ; aucun code exécuté depuis les manifestes.
- Sources explicites avec `base: game-install` ou chemin résolu/autorisé ; **source de vérité choisie par l'utilisateur**, jamais calculée à partir de vieilles copies.
- Résultats CLI prévisibles, mode lisible puis option JSON versionnée quand la commande est stabilisée.
- Journaux d'opérations/receipts : source, destination, identifiant, statut, horodatage, chemins relatifs, tailles, SHA-256, nombre de fichiers ; pas d'informations personnelles superflues.
- Décisions futures `junction`, `symlink`, relocation et changement de `SavePath` **hors MVP de sécurité de base** ; support ensuite uniquement par pilote et consentement explicite.

## 4. Modèle de sécurité du moteur (priorité absolue)

**Règles bloquantes, à tester avant toute fonctionnalité avancée :**

- Vérifier et canonicaliser les chemins ; interdire recouvrement source/destination/snapshots et évasion par `..`.
- Détecter les liens symboliques/jonctions et **ne pas les suivre dans les sources par défaut**. Toute politique de suivi exige une décision séparée.
- Ne jamais déduire qu'un fichier ancien est « obsolète » à partir du seul nom ou de la date.
- Ne jamais exécuter les fichiers inspectés ; ne jamais supprimer automatiquement le contenu source ou les copies de `Ga`.
- Pour les écritures : préparer une destination nouvelle/staging, copier, comparer les SHA-256 et les ensembles de chemins, puis publier seulement après succès ; traiter les changements de source pendant la copie comme erreur/non-publication.
- Les opérations de remplacement du `current` doivent être conçues **avant implémentation** avec rollback et garanties spécifiques Windows ; ne jamais supposer qu'un `os.Rename` remplace atomiquement une cible non vide sous Windows.
- Un snapshot validé est en lecture seule logique, versionné et immuable ; jamais de réécriture d'un snapshot approuvé.
- Une synchronisation supprimant des fichiers ne peut pas être déclenchée implicitement (pas de `/MIR` par défaut).
- Les opérations destinées aux **vraies** sauvegardes doivent être pilotées séparément des tests : en CI, données exclusivement synthétiques ; sur machine utilisateur, mode de lecture/dry-run avant écriture.
- Documenter clairement : une copie sur le même disque physique n'est **pas** une sauvegarde hors-disque.

## 5. GitOps : politique Dirloom adaptée à Savewarden

### 5.1 Branches, développement, validation

- `main` : branche d'intégration protégée ; pas de push direct après initialisation du dépôt. PR obligatoire, checks requis, historique lisible.
- **Une branche de release par incrément publiable** : `release/v0.1.0-a1`, puis `release/v0.1.0-a2`, `release/v0.1.0-rc1`, `release/v0.1.0` si ces jalons sont effectivement publiés. Les noms représentent des versions planifiées, pas une obligation de créer tous ces tags.
- Les `feat/*`, `fix/*`, `docs/*`, `test/*` de cette version partent de sa release branch et ouvrent leurs PR **vers cette branche**.
- Freeze : no new features; corrections ciblées, code/tests/docs/notes cohérents ; preuve CI et snapshot GoReleaser sur le commit précis.
- PR finale `release/v…` → `main` avec **merge commit** (pas de squash sur le PR final) ; tag annoté sur le vrai merge SHA de `main`, puis pipeline GitHub Release.
- Tags publiés et archives immuables ; nouvelle correction = nouvelle version. Le changelog GitHub est généré **depuis `CHANGELOG.md` uniquement**.
- Périmètre release indépendant de la distribution. Publication GitHub validée ≠ installation Scoop/Homebrew/Winget validée.

**Cas du dépôt vierge** : autoriser seulement la création du commit de bootstrap initial sur `main` (aucun historique alternatif existant), puis protéger `main` et créer `release/v0.1.0-a1` avant toute feature de la première version. Si le dépôt contient déjà des commits, **ne rien écraser** et adapter le plan à son état réel.

### 5.2 CI dès le bootstrap

CI principale (PR et push main/release) :

1. `go mod verify` ; `go mod tidy` avec diff nul de `go.mod` et `go.sum` ; `gofmt` et si choisi `goimports`.
2. `go vet ./...` ; `go test ./...` ; builds Windows/Linux/macOS ; compilation ciblée de `cmd/savewarden`.
3. `go test -race ./...` sur Linux ; `golangci-lint` ; `govulncheck`.
4. GitHub Actions épinglées par **SHA complet** ; scripts d'installation et outils à version/empreinte fixées ; job de vérification des pins.
5. Tests de documentation exécutable/CLI quand les exemples existent.
6. Le GoReleaser `check` et `snapshot --skip=publish`, puis smoke des archives, entrent dans les gates **au moment de l'activation du pipeline de release** ; ne pas créer une fausse validation 13-artefacts avant d'avoir l'outil de génération/SBOM.
7. Ne pas recopier les jobs Dirloom spécialisés Mermaid, Graphviz et D2 si Savewarden ne produit pas ces formats.

Les runners critiques de release doivent être **pinnés par version OS vérifiée** (le dépôt Dirloom observé utilise actuellement `ubuntu-26.04` pour certains jobs) ; ne pas réutiliser aveuglément les versions/outils de Dirloom sans preuve de compatibilité. CI générale peut fonctionner avec une matrice explicite et versionnée.

### 5.3 Release pipeline quand prêt

- 6 artefacts potentiels, **si les builds sont effectivement fonctionnels** : Windows/Linux/macOS × amd64/arm64 ; Windows-first peut conduire à limiter la publication initiale plutôt que distribuer des binaires non validés fonctionnellement. Décision à prendre avant le premier tag.
- GoReleaser : release GitHub en **draft**, aucun publish automatique.
- SHA-256 des archives, SBOM par archive, attestations des artefacts, vérification et preuves conservées ; 13 sujets uniquement **si** six archives + six SBOM + checksums sont vraiment activés.
- Human GO après audit des artefacts et CI du tag exact ; publication manuelle ; aucun rewrite d'un tag public.
- Scoop/Homebrew/Winget : automatisation **post-publication stable**, dans des dépôts/canaux officiels, seulement après bootstrap des recettes et tests d'installation. Les prereleases n'actualisent pas les canaux stables.
- Le robot de publication dispose d'autorisations minimales, secrets en environnement protégé ; les workflows de PR ne reçoivent jamais des tokens de publication.

**Protection GitHub cible** : branch ruleset pour `main` et `release/*`, PR obligatoire, checks requis et commentaires/reviews selon effectif disponible, interdiction force-push/delete, protection tags, dépendances aux forks minimisées. Ne pas déclarer « deux reviewers » effectifs si un seul mainteneur existe : documenter ce contrôle manquant et ne pas simuler une approbation.

## 6. Qualité non négociable : code + tests + docs

Chaque incrément comprend **simultanément** :

| Axe | Minimum attendu |
| --- | --- |
| Code | APIs limitées, comportements déterministes, erreurs actionnables, lisibilité, absence d'import cycles |
| Tests | unitaires, intégration tempfile, régressions des cas dangereux, CLI smoke ; tests Windows pour les chemins/liens |
| Docs | README ou guide de commande, contrats, limitations, procédures de récupération, changelog et ADR si décision durable |
| Preuves | commandes exécutées + sorties (locales ou GitHub Actions), diff des fichiers, résultats échoués explicités |
| Sécurité | lecture seule par défaut, absence de suppression implicite, aucune écriture dans le `D:\Ga` historique |

Les nouveaux chemins/manifestes sont testés sur : espaces, Unicode, chemins absents, permissions insuffisantes, chemins identiques ou chevauchants, destination non vide, collision, changements en cours de copie, fichiers verrouillés, échec partiel, symlink/junction, grandes listes et fichier vide. Ne pas forcer 100 % de couverture numérique ; prioriser exhaustivement les branches de sécurité et scénarios critiques.

## 7. Lots ordonnés (une PR cohérente par lot ; Human GO entre lots)

### B0 — Bootstrap documentaire + squelette Go + GitOps minimum **(première mission de l'agent)**

**Objectif** : dépôt propre, builds et commandes d'aide/version fonctionnels, garde-fous CI et contrat architectural initial. **Aucune commande de copie/restore sur données réelles.**

Livrables :

- Inspecter d'abord le statut GitHub/local. Si dépôt absent, demander/recevoir la destination `owner/name` avant de créer l'URL du module Go ; **ne pas inventer le propriétaire**. Ne pas créer de dépôt distant ni activer la visibilité publique sans décision explicite.
- Initialiser `go.mod`, `cmd/savewarden`, composition/CLI minimale, `savewarden --help`, `savewarden version` ; version de développement explicite.
- Ecrire `README.md`, `CONTRIBUTING.md`, `SECURITY.md`, `CHANGELOG.md`, `docs/architecture.md`, `docs/safety-model.md`, `docs/gitops.md`, `docs/pilots/odyssey.md` et un ADR initial sur FSD-inspired/Go quand la justification est stabilisée.
- Documenter les sources historiques utilisées en tant que **preuves** (paths, 14 fichiers, snapshot), mais aucune donnée privée du jeu, aucune valeur de compte sensible, aucune copie de saves dans le repo.
- Ajouter `go test` smoke CLI et tests d'architecture simples (la sortie `--help`/`version` ; éventuellement les règles d'import si elles sont robustes).
- CI tri-OS + race + lint + vulnérabilités + pin-check ; les jobs doivent tourner en réel, avec outputs reproductibles.
- Préparer la politique GitOps et les instructions agents (`AGENTS.md`), la PR template, `.gitignore` excluant `D:\Savewarden`, `/dist`, artefacts de build, rapports locaux et secrets. Choix explicite de la licence, pas de texte de licence fictif.
- Pas de release/tag, pas de migration `Ga`, pas de création de junction/symlink.

**Critères de GO B0** : le repo compile, CI affiche des exécutions vertes sur les OS retenus, l'aide et la version sont déterministes, les docs décrivent les contrats, aucun package inutile/dossier vide, aucun fichier de sauvegarde committé, aucun push/merge sans revue.

### B1 — Manifeste strict + `inspect` en lecture seule

- `manifest.yaml` v1 de référence pour Odyssey, avec chemin relatif à l'installation et `SavePath` vérifié en entrée, sans heuristique silencieuse.
- Tests unitaires du parser strict (inputs hostiles/malformés), de la résolution de chemin et du non-suivi des liens.
- Inventaire déterministe ; formats lisible et, une fois fixé, JSON versionné. Aucun write dans `D:\Savewarden` ni l'installation.
- Docs du schéma/manifeste + fixtures synthétiques ; test manuel opt-in sur Odyssey.

### B2 — `sync` sans suppression + receipt vérifiable

- Source de vérité explicite et destination protégée ; staging/plan de copie, SHA-256, vérification, publication contrôlée, erreurs de changement concomitant.
- Protection de l'ancien `current` en cas de conflit ; politique de mise à jour explicitée avant code. Aucun `/MIR` implicite.
- Tests de failure injection, crash, fichiers verrouillés, recouvrement et destination non vide.

### B3 — `snapshot` immuable

- Snapshots indépendants du `current`, identifiant unique, `.incomplete` tant que non vérifié, publication uniquement après SHA-256 complet.
- Tests versioning, idempotence, crash et impossibilité de réécrire un snapshot approuvé ; test sur Odyssey après accord explicite.

### B4 — `restore` vers dossier isolé

- Sélection explicite snapshot → dossier destination **séparé** ; préflight, copie vérifiée, pas d'écrasement implicite ; `--dry-run`.
- Aucun restore dans le dossier du jeu dans B4 ; cela viendra avec un ADR de sécurité et protection de l'état actif.

### B5 — `doctor`, validation pilote et release candidate

- Diagnostic de l'état des manifestes, chemins, hash/receipts/snapshots ; résultats actionnables, read-only.
- Campagne complète Odyssey, tests Windows, régressions, package snapshot et preuve de restauration isolée.
- Puis élargir le modèle avec **Cyberpunk 2077** (source sous `C:`) et **Metal Gear Solid V** (profil différent) avant généralisation.

## 8. Modèle de plan à fournir à l'agent **avant chaque lot**

Tout nouveau plan devra contenir :

1. **Contexte et preuves** (état des commits/branches, bugs, logs, résultats connus).
2. **Objectif observable**, non-objectifs et risques.
3. **Décisions d'architecture et ADR** affectés ; graphe de dépendances si changement de frontière.
4. **Scope** : fichiers/packages à créer ou modifier ; changements interdits.
5. **Plan d'exécution atomisé**, checkpoints et ordre des PR.
6. **Matrice tests** : unitaires, intégration, safety, OS spécifiques, smoke, CI.
7. **Documentation à livrer** simultanément et changelog.
8. **Critères d'acceptation vérifiables** + preuves demandées.
9. **Rollout/rollback**, aucune mutation non approuvée sur les données utilisateur.
10. **Rapport de restitution** : commit SHA, branche/PR, `git diff --stat`, tests exacts, résultats CI, nouveaux ADR, risques résiduels, questions ouvertes.

**Formule de validation** : Plan accepté → Agent exécute → Diff + preuves transmis → Revue ChatGPT → Human GO → Merge. Un agent n'est pas autorisé à s'auto-approuver ou à publier une release.

## 9. Brief copiable immédiatement à Codex / Cursor — B0 seulement

> **MISSION : Savewarden B0 — bootstrap prod-grade.**
>
> Tu agis comme un ingénieur Go senior avec responsabilité d'architecture, de tests, de documentation et de supply-chain. Lis intégralement ce plan avant toute écriture. Étudie la structure de `dirloom/dirloom` et ses conventions GitOps sans copier les domaines non pertinents. Consulte la documentation de conception Mosaera comme **inspiration non normative**. Conçois Savewarden en FSD-inspired Go idiomatique, frontières de slices nettes et dépendances dirigées. **Ne développe ni `sync`, ni `snapshot`, ni `restore` dans B0.**
>
> **Préflight obligatoire** : inspecter répertoire, remote, default branch, droits et worktree ; ne jamais écraser l'existant. Si le repo distant/owner, licence, ou stratégie d'initialisation ne sont pas déterminés, signale les points bloquants et propose une réponse sans inventer la décision. N'initialise aucune publication ni package manager.
>
> **Travail B0** : initialiser Go/Cobra, aide/version, organisation de packages minimale et évolutive, README/CONTRIBUTING/SECURITY/CHANGELOG, architecture/safety/GitOps/pilote Odyssey/ADR, AGENTS.md, template PR, .gitignore et CI tri-OS + lint/race/vuln/pin. Mettre les dépendances à jour de façon reproductible et protéger les données utilisateurs hors repo. Écrire les tests de chaque comportement implémenté et la documentation correspondant aux commandes réellement disponibles. N'invente pas de feature implémentée.
>
> **Gates** : exécuter format, tidy propre, mod verify, vet, tests, race, build, lint, vulnérabilité, tests CLI et CI réelle. Si un gate est bloqué par l'environnement, rendre compte, ne pas prétendre au succès. Suivre `release/vX.Y.Z-*` → PR → freeze → `main` de Dirloom pour tout lot versionné après le bootstrap initial. SHA-pin des Actions, permissions minimales.
>
> **Interdictions** : ne toucher sous aucun prétexte à `D:\Ga`, `D:\Savewarden`, `E:\GMS` ou aux clés SSH ; ne pas importer de sauvegardes dans Git, ne pas activer d'écriture automatique ou de lien de fichiers ; ne pas créer de dossiers vides, d'abstraction/framework prématuré, de release/tag, ni de PR auto-mergée ; ne pas pousser vers `main` une fois la protection en place.
>
> **Rendu final obligatoire** : arbre réel via Dirloom si disponible, décisions et ADR, diff/commits/PR, résultat de chaque test avec commandes et lien de run CI, limitations, écarts au plan, questions en suspens et recommandation GO/NO-GO **sans auto-approbation**.

## 10. Décisions bloquantes avant exécution B0

- **Nom et emplacement du remote GitHub** : `owner/savewarden` ; choisir l'owner exact et la visibilité du dépôt avant création (privé pendant le bootstrap recommandé, sans action immédiate).
- **Licence open source** : MIT ou Apache-2.0 à arbitrer ; ne pas créer une licence arbitraire dans le code.
- **Cible compilée et publiée** : tri-OS en CI, mais décision distincte sur les OS réellement supportés fonctionnellement et sur les archives distribuées.
- **Protection de branche** : configurer et documenter les reviewers/checks réellement disponibles, sans fausse validation de plusieurs humains.

Aucun de ces points ne doit bloquer la discussion d'architecture ; ils bloquent uniquement les opérations effectives qui en dépendent.

---

## Références inspectées

- Dirloom CI : https://github.com/dirloom/dirloom/blob/main/.github/workflows/ci.yml
- Dirloom release : https://github.com/dirloom/dirloom/blob/main/.github/workflows/release.yml
- Dirloom GitOps : https://github.com/dirloom/dirloom/blob/main/docs/release-workflow.md
- Dirloom architecture : https://github.com/dirloom/dirloom/blob/main/docs/architecture.md
- Dirloom GoReleaser : https://github.com/dirloom/dirloom/blob/main/.goreleaser.yaml
- Mosaera design : https://github.com/Samuelbenny/mosaera/blob/main/design/README.md (privé)
- Mosaera FSD-like Sonora blueprint : https://github.com/Samuelbenny/mosaera/blob/main/docs/sonora-fsd-like-dirloom-blueprint.md (privé)

*Fin du plan B0 et de la roadmap technique de Savewarden.*

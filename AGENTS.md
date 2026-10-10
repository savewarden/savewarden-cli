# Instructions pour les agents Savewarden

## Périmètre et preuves

- Lire le plan demandé intégralement et inspecter statut, branche, remote et
  fichiers existants avant toute écriture. Ne pas écraser du travail existant.
- Bootstrap initial : **B0 seulement**, aide/version, docs, tests et CI.
  Human GO et plan du lot suivant requis avant B1–B5.
- Ne jamais déclarer une CI verte sans run terminé sur le commit concerné.
  Une compilation croisée prouve seulement la compilation.
- Tenir code, tests, docs et CHANGELOG.md cohérents. Rapport final : branche,
  SHA ou absence de commit, diff, commandes/résultats, CI, ADR, limites.

## Frontières

- `cmd/savewarden` délègue à `app` ; `app` compose ; `cli` gère Cobra et les
  sorties. Les features possèdent leur orchestration métier.
- Features : imports d'entités et ports étroits consommateur-définis ; jamais
  `cli`, Cobra, `app`, adaptateurs `platform` ou une autre feature.
- Entités : bibliothèque standard seulement par défaut. Adaptateurs séparés.
- Pas de framework DI, global service locator, `manager.go` géant, `utils`,
  `common`, barrels ou dossiers créés uniquement pour remplir un arbre.
- Mosaera est une inspiration non normative. Une nouvelle frontière durable
  requiert un ADR et la mise à jour des tests d'import.

## Données protégées

- **Ne toucher sous aucun prétexte à `D:\Ga`, `D:\Savewarden`, `E:\GMS`
  ou aux clés SSH dans B0.** Ne pas importer de saves ou comptes dans Git.
- Tests exclusivement synthétiques/tempfiles. Les paths historiques dans
  les docs sont des preuves rapportées par le plan, pas des données à explorer.
- Pas d'exécution de fichiers inspectés, suppression source, `/MIR`, suivi
  implicite de symlink/junction, relocation ou restauration en place.
- Futurs writes : préflight, source explicite, destination distincte, staging,
  comparaison des paths/tailles/SHA-256, détection de changement source et
  publication après succès. Concevoir rollback Windows avant remplacement
  de `current`. Aucun snapshot approuvé réécrit.

## GitOps et contrôles

- Le dépôt vierge autorise un unique commit initial local B0 sur `main`.
  Sa publication attend la revue humaine du résultat. Toute suite part de
  `release/v0.1.0-a1` après Human GO B0.
- Branches de travail depuis `release/vX.Y.Z-*`, PR vers cette release branch.
  PR finale vers `main` avec merge commit ; jamais auto-merge/auto-approbation.
- Aucun push direct après bootstrap, force-push, tag, release ou publication
  de package sans validation humaine explicite dans le périmètre approprié.
- Exécuter les gates de CONTRIBUTING.md. SHA-pin complet des Actions, outils
  exacts, `contents: read` dans CI. Pas de secrets de publication dans les PR.
- Documenter les protections réellement actives et le nombre réel de
  reviewers ; ne pas simuler une approbation indépendante.

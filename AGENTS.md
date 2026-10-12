# Instructions pour les agents Savewarden

## Périmètre et preuves

- Lire le plan demandé intégralement et inspecter statut, branche, remote et
  fichiers existants avant toute écriture. Ne pas écraser du travail existant.
- Lot courant : **B1**, manifeste strict et `inspect` en lecture seule,
  selon `.plans/Savewarden_Plan_B1_Manifeste_Inspect_v0.2.md`, plan révisé
  par le propriétaire ; son nom porte désormais sa version.
  Human GO et plan accepté requis avant chaque lot suivant, B2 à B5.
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
  ou aux clés SSH.** Seule exception : une lecture `inspect` du pilote, sur
  autorisation explicite et nominative du propriétaire pour cette exécution.
  Ne pas importer de saves ou comptes dans Git.
- Tests exclusivement synthétiques/tempfiles. Les paths historiques dans
  les docs sont des preuves rapportées par le plan, pas des données à explorer.
- Pas d'exécution de fichiers inspectés, suppression source, `/MIR`, suivi
  implicite de symlink/junction, relocation ou restauration en place.
- Le code de production reste en lecture seule tant qu'un lot n'autorise pas
  d'écriture : `internal/app/readonly_test.go` ne s'assouplit que par package,
  avec un ADR. `os.Root` confine mais suit les liens internes ; ne jamais en
  faire l'unique garantie de non-suivi.
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
- Après fusion dans la release branch, supprimer la branche de travail, sur le
  remote et en local. Les release branches restent protégées contre la
  suppression par leur ruleset.
- Aucun push direct après bootstrap, force-push, tag, release ou publication
  de package sans validation humaine explicite dans le périmètre approprié.
- Exécuter les gates de CONTRIBUTING.md. SHA-pin complet des Actions, outils
  exacts, `contents: read` dans CI. Pas de secrets de publication dans les PR.
- Documenter les protections réellement actives et le nombre réel de
  reviewers ; ne pas simuler une approbation indépendante.

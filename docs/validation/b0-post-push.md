# B0 — Compte rendu post-push

Date des constats : **12 octobre 2026**. Ce document complète le
[rapport local B0](b0.md) daté du 10 octobre, sans le réécrire : ce dernier
décrit fidèlement l'état **avant** remote, push et CI. Il ne constitue pas
une auto-approbation ; la décision humaine est consignée en fin de document.

## Commit et CI distants

| Élément | Constat |
| --- | --- |
| Dépôt | [savewarden/savewarden-cli](https://github.com/savewarden/savewarden-cli), public |
| Commit | `25d1cc206b2e686b32825c1343f298f1ee5cfa8f`, sans parent, seul commit de `main` |
| Run CI | [38288590229](https://github.com/savewarden/savewarden-cli/actions/runs/38288590229), `push` sur `main`, terminé le 2026-10-12 à 18:05 UTC |
| Conclusion | `success`, 7 check runs émis par l'app `github-actions` (id 15368) |

| Check | Conclusion |
| --- | --- |
| `Verify (ubuntu-24.04)` | success |
| `Verify (windows-2025)` | success |
| `Verify (macos-15)` | success |
| `Race detector` | success |
| `Lint` | success |
| `Vulnerability scan` | success |
| `Pins` | success |

La CI prouve le bootstrap aide/version sur ces trois runners et la course
de données sur Linux. Elle ne valide aucune future opération sur des
sauvegardes. Le blocage local `-race requires cgo` reste compensé par le job
Linux pour B0.

Contrôle local répété le 12 octobre sur Windows (`go1.27.2 windows/amd64`) :
`go mod verify`, `go vet`, `go test -count=1` des packages et du checker de
pins, `go run ./.github/scripts` et `gofmt -l` ont réussi ; arbre de travail
inchangé.

## Protections constatées avant application

Lecture API avec un compte **administrateur** du dépôt :

| Requête | Réponse |
| --- | --- |
| `GET /repos/savewarden/savewarden-cli/rulesets` | `[]` |
| `GET /repos/savewarden/savewarden-cli/rules/branches/main` | `[]`, règles d'organisation applicables comprises |
| `GET /repos/savewarden/savewarden-cli/branches/main/protection` | `404 Branch not protected` |
| `GET /repos/savewarden/savewarden-cli/branches` | `main`, `protected: false` |
| Tags, releases, PR | aucun |

**Conclusion : `main` n'est pas protégée.** La revue indépendante avait obtenu
un `403` sur la protection classique, faute de droits suffisants ; la lecture
administrateur lève cette incertitude.

Réglages complémentaires constatés :

- Actions : permissions par défaut `read`, approbation de PR par Actions
  désactivée, actions autorisées `all`, pinning SHA imposé par GitHub désactivé.
- Fusion au niveau dépôt : merge, squash et rebase autorisés ; auto-merge
  désactivé. Les rulesets doivent donc restreindre les méthodes par branche.
- Secret scanning et push protection : désactivés avant application.
  Alertes Dependabot activées, mises à jour de sécurité désactivées.
- Signalement privé de vulnérabilités : désactivé ; [SECURITY.md](../../SECURITY.md)
  prévoit déjà ce cas.

## Revue indépendante du 12 octobre 2026

Verdict : **GO technique pour le contenu B0**, **NO-GO pour démarrer B1**
avant protections vérifiées, compte rendu post-push et Human GO explicite.

| Point | Suite donnée |
| --- | --- |
| P1 — protections `main` et `release/*` (bloquant) | Confirmé ci-dessus, puis appliqué ; voir la section suivante |
| P2 — compte rendu post-push | Ce document |
| P3 — linters `gosec`/`errorlint` avant le filesystem | Reporté au plan B1, avec évaluation des faux positifs |
| P3 — `.gitignore` masque `saves/`, `current/`, `snapshots/`, `*.save` | Reporté au plan B1 ; fixtures construites dans les tests |

La revue n'a pas exécuté le code ; le contrôle local ci-dessus complète ce
point pour le seul périmètre B0.

## Protections appliquées

Appliquées le **12 octobre 2026 vers 18:16 UTC** sur GO du propriétaire,
conformément à [GitOps](../gitops.md), puis relues par l'API.

| Ruleset | Id | Cible | Enforcement |
| --- | --- | --- | --- |
| `main` | 24944424 | `~DEFAULT_BRANCH` | `active` |
| `release` | 24944427 | `refs/heads/release/*` | `active` |

Règles effectives lues sur `rules/branches/main` et
`rules/branches/release%2Fv0.1.0-a1` :

| Règle | `main` | `release/*` |
| --- | --- | --- |
| Suppression interdite | oui | oui |
| Force-push interdit | oui | oui |
| PR obligatoire, zéro approbation | oui | oui |
| Conversations résolues | oui | oui |
| Méthodes de fusion | merge | merge, squash |
| 7 checks requis, app 15368 | oui | oui |
| Branche à jour exigée | oui | oui |
| Checks exigés à la création | oui | non |
| Bypass | aucun, `never` pour l'administrateur | idem |

`GET /repos/savewarden/savewarden-cli/branches/main` indique désormais
`protected: true`. Une branche hors motif, par exemple `feat/x`, ne reçoit
aucune règle : les branches de travail restent libres jusqu'à leur PR.

Secret scanning et push protection sont **activés** depuis la même date.
Les autres réglages constatés plus haut sont inchangés.

Limites :

- GitHub a ajouté `require_extra_approval_for_unattributed_changes: true`
  à la règle PR, sans demande explicite. Son effet pour un mainteneur unique
  n'est pas vérifié ; à observer lors de la première PR vers la release branch.
- L'enforcement est constaté par l'API, pas par une tentative de push direct
  sur `main`. La première PR réelle en fournira la preuve d'usage.

## Décision humaine

- [x] **Human GO de clôture B0** donné le 12 octobre 2026 par le propriétaire
  du projet, après revue indépendante du commit `25d1cc2`, vérification des
  7 checks et confirmation des protections `main` et `release/*`.
- [x] `release/v0.1.0-a1` créée le 12 octobre 2026 vers 18:22 UTC depuis
  `25d1cc206b2e686b32825c1343f298f1ee5cfa8f`. L'API et `git ls-remote`
  confirment ce SHA ; la branche est signalée `protected: true`.

Portée du GO : créer la release branch, intégrer ce compte rendu par une PR
documentaire vers celle-ci, puis préparer le plan B1 pour validation séparée.
Il n'autorise ni l'implémentation B1, ni tag, release, force-push ou
contournement des rulesets. `require_extra_approval_for_unattributed_changes`
reste inchangé par décision du propriétaire.

Un seul mainteneur, Samuelbenny ; aucune seconde revue indépendante n'est
simulée. La revue du 12 octobre est un avis externe, pas une approbation de PR.

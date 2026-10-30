# B2c — Rapport de validation

Rotation de `current` et récupération complète de `sync`, selon le plan B2
v0.2 §0.8, §8.3 et §9.3, et ADR 0006 amendé, référence de sécurité. Ce
rapport consigne les preuves locales, les revues indépendantes et ce qui reste
à établir. À sa rédaction, aucun push, aucune PR, aucune fusion ni aucun
pilote n'avait eu lieu ; la PR, sa CI, la fusion et le pilote sont consignés
ensuite, sur GO distincts. Aucun tag ni release.

## Cadre

| Élément | Valeur |
| --- | --- |
| GO humain | « Go pour la suite » du propriétaire, le 29 octobre 2026, après la clôture de la revue de conception sur `a7bc69c` ; GO de développement, local, sans publication Git |
| Branche | `feat/b2c-rotation`, locale, créée depuis `docs/b2c-framing` en `a7bc69c`, elle-même sur `54b1c02`, tête de `release/v0.1.0-a1` ; les neuf commits de cadrage sont donc en tête de la branche ; Go 1.27.3 |
| PR | Non ouverte à la rédaction ; puis, sur GO distincts, [savewarden/savewarden-cli#8](https://github.com/savewarden/savewarden-cli/pull/8), fusionnée dans `release/v0.1.0-a1` par le merge commit `d69ff0c` ; branche supprimée, sur le remote et en local |
| Écritures | Seul `internal/platform/vault` écrit, ADR 0005 ; B2c n'ajoute ni API d'écriture, ni port, ni méthode ; `git diff --stat 54b1c02..` sur `internal/app`, `internal/platform`, ADR 0005, `go.mod`, `go.sum` et `.github` ne montre qu'un cas du test de contrat `app_test.go` |
| Revues indépendantes | Revue du remplacement, point de contrôle du §8.3 ; revue de sécurité, préalable au rapport ; vérification indépendante de leurs corrections sur `f5ba069`. Trois agents en lecture seule, distincts de l'auteur ; ce ne sont pas des approbations humaines. Consignées ci-dessous |
| Pilote B2c | Non exécuté à la fusion ; **exécuté par le propriétaire après fusion, le 30 octobre 2026**, selon le [protocole B2c](../pilots/odyssey.md#protocole-b2c), voir [Pilote B2c](#pilote-b2c) ; qualification fonctionnelle à prononcer par le propriétaire |

## Commits

| SHA | Commit | §8.3 |
| --- | --- | --- |
| `ed4bb83` à `a7bc69c` | Cadrage : ADR 0006 amendé, plan §0.8, §4.5, §8.3, §9.3, priorité d'ADR 0006 sur le §5 ; neuf commits revus et certifiés par le propriétaire | 0 — **revue du cadrage** |
| `53df1df` | `current_identity` de `planned`, identité publiée, chaîne de reconnaissance, règle M3 pure | 1 |
| `362044c` | Reconnaissance au plan et sous verrou, rotation M1 et M2, relectures, restauration vérifiée, rollback | 2 |
| `2be4f22` | `--allow-removals` et `--accept-missing-source` dans la feature | 3 |
| `58e3791` | Récupération d'une rotation, table fermée, règle M3, énumération des crashs | 4 — **revue du remplacement** |
| `844688f` | Drapeaux et rendu de la CLI, ADR 0007 amendé (R5), complétude des codes | 5 |
| `c8deaa9` | Documentation B2c | 6 |
| `2c918e5`, `d597a8a` | Tests nommés du §9.3 restés hors des commits précédents : Windows réel, table pure, coffre copié ; exigence `--recover --apply` inscrite au §9.3 | Complément de 5 et 4 |
| `9591ad7` | Corrections de la revue du remplacement | 4, suite |
| `f5ba069` | Corrections de la revue de sécurité | 7, préalable |
| `b59835f` | Corrections de la vérification indépendante des deux commits précédents | 7, préalable |
| `bf306fd` | Rapport de validation | 7 |
| `d5d2037` | Deux constats du premier lint de la CI, sur `bf306fd` : `%w` dans `checkRetired`, loi de De Morgan dans un test | 7, suite |

Écart au découpage, admis par A5 et expliqué : six commits s'ajoutent aux
sept prévus. Deux portent des tests nommés par la matrice et oubliés par les
commits 4 et 5 ; trois portent les corrections des revues, gardées séparées
pour que chaque revue reste traçable à son SHA ; le dernier corrige les deux
constats du premier lint de la CI. Chacun des commits 1 à 4 passe
la suite complète à son propre SHA, vérifié dans un worktree temporaire.

## Décisions du propriétaire intégrées

- **A1** `current_identity` de `planned`, obligatoire pour une rotation,
  interdit pour une première publication, repris sans substitution ;
  **A2** relectures sous verrou et juste avant le retrait ; **A3** les deux
  drapeaux dans les seules conditions de D5 et V1 ; **A4** aucune suppression,
  accumulation documentée ; **A5** découpage ci-dessus.
- **M3** et **R1** : abandon d'une opération sans enregistrement seulement
  sans génération publiée attendue et sans `current`, ou à côté de la
  génération précédente reconnue.
- **R2** : génération précédente reconnue par toute sa chaîne de preuve,
  identité NTFS publiée comprise ; un dossier `unidentified` n'est jamais
  `old`.
- **R3** et **R4** : observation exacte et relecture de `retained` entre M1 et
  M2, restaurations vérifiées, fenêtres résiduelles déclarées.
- **R5** : codes B2b ajoutés à ADR 0007, `current-exists` gardé dans sa
  définition historique et non émis.
- Effets arbitrés : coffre copié ou déplacé refusé en code 3 sans écriture ni
  adoption, sans présumer de corruption ; M3 strict ; `current` réapparu entre
  M1 et M2 en code 3, sans promotion ni restauration ; (`other`, `old`,
  `new`) sans ligne applicable.
- Exigence de développement : la récupération répétée à côté d'un `current`
  inattendu exerce `--recover --apply`, trois fois, code 3, diagnostic stable,
  aucune mutation.

## Critères d'acceptation B2c (plan §12)

| # | Critère | Preuve |
| --- | --- | --- |
| 16 | Rotation réelle avec rollback, renommage refusé par un fichier ouvert ; génération conservée identique ; chaque ligne du §5.5 atteinte ; crash avant et après M1 à M4 et pendant chaque validation ; §5.9 ; ancien `current` toujours intact | Windows réel : `TestRotationEndToEnd`, `retained` relu par `inspect` aux SHA-256 de son receipt ; `TestRotationRefusedByAnOpenFileOnWindows`, erreur 5, `rolled back`, code 1 ; `TestSyncKilledDuringRotationThenRecovered`. Faux coffre : `TestCrashAtEveryRotationCall`, crash avant et après chaque appel de trois chemins, nominal, restauration par M3, `rollback-failed`, avec l'état final exact de chaque point ; `TestRotationRecoveryRows`, une ligne par cas ; `TestRotationRecoveryIsInterruptible` et `TestRepeatedInterruptionsBetweenUnpromoteAndRestore`, crash à chaque appel de la récupération, M4 compris ; `TestRotationEnvelope`, E1 à E3 et le chemin de onze enregistrements ; `TestRotationAmbiguityIsStable` ; `rotationTerminal` exige l'ancienne génération intacte, dans `current` ou `retained`. Après fusion, pilote réel : rotation en code `0`, génération conservée égale à son receipt, voir [Pilote B2c](#pilote-b2c) |
| 17 | Aucune disparition de fichiers sans drapeau, dans les seules conditions de V1 ; source requise absente et source retirée toujours bloquantes ; `current-modified` bloque sans rien déplacer | `TestAllowRemovalsCoversRemovedFilesOnly`, `TestAcceptMissingSourceCoversVanishedSourceOnly`, `TestRequiredOrRemovedSourcesStayBlocked`, `TestInvalidAcceptMissingSourceIsCode1`, `TestTwoAcceptedMissingSources` ; binaire `TestRotationFlagsEndToEnd` ; `TestCurrentWithAlteredContentIsNotRecognized`, `TestRotationRefusesCurrentChangedBeforeTheLock`, `TestRotationFailsWhenCurrentChangesBeforeRetirement`, et le binaire de `TestRotationEndToEnd`, `current` modifié à la main : code 2, coffre inchangé |
| 18 | Revue de sécurité consignée ; `readonly_test.go` inchangé depuis B2b | Revue ci-dessous ; `readonly_test.go`, `architecture_test.go` et `internal/platform` inchangés depuis `54b1c02` |
| 19 | 7 checks verts sur le SHA exact | Non établi à la rédaction ; puis 7 checks verts sur `d5d2037`, tête de la PR, et sur `d69ff0c` après fusion, voir [CI](#ci) |
| 20 | Docs, guide de récupération, CHANGELOG et ADR cohérents ; exemples vérifiés | Guide de commande, guide du coffre, modèle de sécurité, architecture, README, AGENTS, CONTRIBUTING, CHANGELOG, ADR 0004, 0006 et 0007 ; `TestStableCodesAreDocumented` ; `TestReadmeCommandExamples` |
| 21 | Aucun accès aux données protégées, aucun chemin personnel | Tests en mémoire ou en `t.TempDir()`, chemins fictifs ; aucun accès à `D:\Ga`, `D:\Savewarden`, `E:\GMS` ni aux clés SSH ; la vérification du retour arrière vers B2b a utilisé un coffre synthétique en dossier temporaire |

## Matrice du §9.3

La matrice nomme 66 tests. Ils existent tous, sauf les deux tests B2b que la
matrice dit transformés en tests de rotation : `TestExistingCurrentIsRefused`
devient `TestExistingCurrentIsRotated`, `TestSyncApplyRefusesToReplaceCurrent`
devient `TestRotationEndToEnd`. Hors matrice, le test CLI
`TestRemovalFlagsAreNotAvailable` devient `TestPolicyFlagsReachTheRequest`
et `TestRecoverRefusesPolicyFlags`. B2c ajoute 76 fonctions de test et en
retire ces trois. `TestRepeatedRecoveryWithAnUnexpectedCurrentAfterRetirement`
exerce `--recover --apply` trois fois, comme exigé.

## CI

**À la rédaction : aucun run.** La branche n'était pas poussée.

**Après le GO de push :** le premier run, sur `bf306fd`, échoue au seul job
Lint, sur deux constats que golangci-lint n'avait pas relevés faute d'avoir
tourné localement ; ils sont corrigés en `d5d2037`. Les sept checks requis
sont ensuite verts sur le SHA exact de la PR, puis sur le commit de fusion.
Ils couvrent aussi ce qui n'avait pas pu tourner localement : govulncheck et
le détecteur de courses.

| Commit | Run | Résultat |
| --- | --- | --- |
| `bf306fd` | [40973584785](https://github.com/savewarden/savewarden-cli/actions/runs/40973584785) | Lint en échec, `errorlint` et `QF1001` ; six autres checks verts |
| `d5d2037`, tête de la PR #8 | [40974262994](https://github.com/savewarden/savewarden-cli/actions/runs/40974262994) | 7/7 : Verify Ubuntu, Windows et macOS, Race detector, Lint, Vulnerability scan, Pins |
| `d69ff0c`, fusion dans `release/v0.1.0-a1` | [41037795818](https://github.com/savewarden/savewarden-cli/actions/runs/41037795818) | 7/7, mêmes jobs |

## Revues indépendantes

**Revue du remplacement**, sur les commits 1 à 4, par un agent en lecture
seule : sept constats, tous vérifiés et corrigés en `9591ad7`.

| # | Constat | Correction |
| --- | --- | --- |
| F1, important | Un pré-contrôle de M1 qui ne pouvait lire une identité, `current` disparu ou devenu jonction, menait à `rolled-back` « current unchanged » | Tout pré-contrôle en échec ou illisible est une ambiguïté, code 3, rien enregistré ; un renommage échoué n'est un non-renommage confirmé que si la source et la cible le confirment ; durcit aussi la bascule de B2b |
| F2 | M3 et M4 dépensaient leur `rollback-failed` sur un déplacement jamais tenté | Même correction |
| F3, important | La restauration vérifiée après M4 n'avait pas de test | `TestUnpromoteThenAlteredRetainedStops` |
| F4 | Un `retained\<prev-op>` déjà présent n'était refusé qu'après le journal | Refusé par le plan, code 3, avant tout journal |
| F5 | Sous verrou, codes et ligne d'état inexacts dans quelques cas | Ordre corrigé ; ligne d'état au nom du coffre avant tout dossier d'opération |
| F6 | Le plan annonçait « current published » pour un objet étranger | Seulement quand `current` est le staging enregistré |
| F7 | Matrice : crashs sans état final exact, chemin de M3 non énuméré | Trois chemins énumérés, état final exact, temporaires de `rollback-failed` |

**Revue de sécurité**, sur `2c918e5` : aucun constat bloquant ; un important,
sept mineurs. Le confinement est confirmé : aucune API d'écriture hors de
`platform/vault`, liste du garde inchangée, aucun port nouveau.

| # | Constat | Suite |
| --- | --- | --- |
| 1, important | `retained\<head-op>` refusé tard | Déjà corrigé par F4 |
| 2 | Bascule close sans observation confirmée | Déjà corrigé par F1 |
| 3 | La récupération vérifiait `retained` contre un receipt non authentifié | Receipt de `<prev-op>` authentifié par son journal publié et l'identité fixée par `planned`, en `f5ba069` |
| 4 | Un receipt validé sans bascule observée pouvait mener à `rolled-back` et à un receipt orphelin | Contradiction, code 3, rien écrit, toute ligne ; ADR 0006 amendé |
| 5 | Fenêtres non déclarées ; identité lue avant le contenu | Identité relue après chaque contenu ; fenêtre du staging nommée au §5 |
| 6 | Formulation « ôté » | « déplacé hors du coffre » |
| 7 | Les diagnostics montrent des identités NTFS | Notes de confidentialité complétées |
| 8 | Dossier `retained` vide transformable en point de montage | Hors modèle, déjà déclaré ; refusé par la relecture avant la promotion |

Question ouverte de la revue, vérifiée : un binaire construit depuis `54b1c02`
refuse un coffre synthétique qui a connu une rotation, `journal-ambiguous` sur
`current_identity`, code 3, plan comme `--apply`, coffre inchangé.

**Vérification des corrections**, sur `f5ba069`, par un troisième agent :
chaque correction tient pour ses cinq appelants de la bascule ; le refus d'un
receipt validé sans bascule et le recoupement de la génération remplacée ne
bloquent aucun état que le protocole produit, A5 et A6 compris, ni un receipt
reconstruit. Un constat important et quatre mineurs, corrigés en `b59835f` :

| # | Constat | Correction |
| --- | --- | --- |
| 1, important | La vérification de `current` par la récupération ne relisait pas l'identité après le contenu : une substitution pendant la lecture pouvait faire écrire `unpromote-intent`, `unpublish-intent` ou `published` | Identité relue après le contenu, code 3, rien écrit ; testé en première publication et en rotation |
| 2 | Même lacune pour la règle M3 | Identité relue ; le plan, en lecture seule, garde sa relecture sous verrou |
| 3 | Deux relectures sans test qui échoue si on les retire | Substitution vers un contenu différent pendant la relecture sous verrou, de `current` au point 3 et de `retained` après M1 |
| 4 | Un `retained` devenu jonction avant M1 était clos `rolled-back` | Ambiguïté, code 3, rien enregistré |
| 5 | Branches de recoupement et lignes de temporaires préparés non testées | Journal de la génération remplacée absent ; rotation interrompue sur un receipt reconstruit ; deux lignes de `rollback-failed` préparé |

Les deux nouvelles gardes ont été désactivées le temps de vérifier que leurs
tests échouent.

## Pilote B2c

**À la fusion :** non exécuté. L'agent n'a lu aucune des zones protégées par
`AGENTS.md` et n'a pas exécuté le pilote ensuite. Il a préparé le binaire et
les scripts de contrôle du propriétaire, et ne les a exercés que sur des
coffres synthétiques.

**Après fusion, 30 octobre 2026 :** exécuté par le propriétaire, sur
Assassin's Creed Odyssey, selon [le protocole B2c](../pilots/odyssey.md#protocole-b2c) :
préflight en lecture seule, première rotation, vérification de la génération
conservée, puis contrôle d'idempotence. Chaque étape n'a été lancée qu'après
la réussite de la précédente.

| Élément | Résultat |
| --- | --- |
| Binaire | `go build -trimpath` depuis un worktree détaché au commit de fusion `d69ff0c`, propre ; `vcs.revision` et `vcs.modified=false` contrôlés ; compilation reproductible, même SHA-256 depuis un second dossier |
| Scripts du propriétaire | Hors dépôt, empreintes vérifiées avant exécution ; ils s'arrêtent avant tout appel à Savewarden si le jeu tourne, si le worktree, le binaire, la racine du coffre ou le dossier des rapports ne sont pas ceux attendus, ou si le coffre contient un point d'analyse ; seul celui d'idempotence lance `--apply`, une fois, après un plan `up to date` ; éprouvés sur coffres synthétiques, cas nominaux et négatifs : aucun appel à Savewarden après une précondition en échec, aucun `--apply` après un plan qui n'est pas à jour |
| Poste | Windows 11 |
| Coffre | Celui du pilote B2b, `current` publié par ce pilote |
| Préflight, plan seul | 27 contrôles sur 27, deux fois ; code `0`, deux plans identiques, `plan: changes` ; `current` reconnu, égal à l'opération du pilote B2b ; rien en attente, rien à récupérer ; coffre inchangé |
| Changements planifiés | 14 fichiers, 9 566 606 octets : 8 modifiés, 6 inchangés, aucun ajout ni retrait ; aucune anomalie ; aucun drapeau |
| Rotation | `sync --apply`, code `0` ; 14 fichiers et 9 566 606 octets vérifiés par SHA-256 ; ligne d'état `published` ; la ligne `retained:` nomme la génération du pilote B2b |
| Vérification | 31 contrôles sur 31 : plan suivant `up to date`, 14 fichiers inchangés ; nouveau receipt dont `previous` est l'opération B2b ; `retained` ne contient que cette génération |
| Génération conservée | Relue par `inspect` : 14 fichiers, 9 520 705 octets, la taille publiée par le pilote B2b ; fichiers, tailles, SHA-256 et dossiers égaux à son receipt |
| Génération courante | Relue par `inspect` : égale à son receipt |
| Second `sync --apply` | 17 contrôles sur 17 : code `0`, `up to date`, `operation: none`, dernière ligne de stderr `savewarden: vault: not modified` ; plan identique avant et après |
| Coffre | Empreinte d'inventaire et de contenu inchangée pendant chaque contrôle et autour du second `--apply` |
| Verdicts | `PreflightPassed`, `VerificationPassed` et `IdempotencePassed` à `True` |

Un premier lancement du contrôle d'idempotence, avec l'identifiant d'exemple
de la commande au lieu de celui de l'opération, s'est arrêté sur sa
précondition sans appeler Savewarden.

**Qualification fonctionnelle :** à prononcer par le propriétaire.

Limites du pilote :

- Il qualifie la rotation nominale. Le rollback, les crashs et la
  récupération restent prouvés par les tests synthétiques, les tests Windows
  réels et la CI ; le pilote ne simule ni refus de renommage ni coupure.
- Un seul jeu, une seule source, un seul poste, une seule rotation.
- L'empreinte du coffre couvre les chemins, attributs, dates, tailles et
  SHA-256 de chaque entrée ; ni les ACL, ni les identités NTFS, ni les flux
  alternatifs. Les scripts relisent les métadonnées sur chaque entrée : la
  copie qu'en garde l'index du dossier parent est mise à jour en différé, ce
  qui a produit un faux écart sur un coffre synthétique.
- Ne sont versionnés ni les empreintes individuelles, ni les receipts réels,
  ni les identifiants d'opération, ni le nom de profil, ni aucun chemin
  personnel, ni la sortie complète, ni les scripts du propriétaire.

## Amendements d'ADR 0006 après la certification du cadrage

Deux entrées s'ajoutent après le cadrage certifié par le propriétaire, sans
changer d'architecture : la troisième fenêtre résiduelle, entre la relecture
d'une restauration vérifiée et M3, et la fenêtre du staging entre `verified`
et M2 ; le refus d'un receipt validé sans bascule observée, et
l'authentification du receipt de `<prev-op>` avant une restauration. Elles
appliquent le fail-closed du §3 à des cas que les revues ont démontrés ;
elles relèvent de la décision du propriétaire. À la fusion de la PR #8, leur
ratification explicite n'était pas consignée ; elle reste à prononcer.

## Contrôles locaux

| Commande | Résultat |
| --- | --- |
| `gofmt -l .` | Aucune sortie |
| `go mod tidy -diff`, `go mod verify` | Aucun écart ; `all modules verified` |
| `go vet ./...`, Windows et `GOOS=linux` ; scripts | Succès |
| `go test -count=1 ./...` | `ok` pour les 10 packages ; `cmd/savewarden` 83 s, tests Windows réels compris |
| `go test ./.github/scripts`, `go run ./.github/scripts` | Succès |
| `go build -trimpath`, Windows et Linux | Succès, compilation seulement pour Linux |
| Commits 1 à 4, chacun à son SHA | Suite complète verte |
| golangci-lint 2.14.0, `run ./... ./.github/scripts`, Windows et `GOOS=linux` | `0 issues` après le commit de correction ; non exécuté avant `bf306fd`, dont la CI a relevé deux constats |
| govulncheck | **Non exécuté** : outil absent sur ce poste ; prouvé ensuite par la CI, voir [CI](#ci) |
| `go test -race` | **Non exécuté** : pas de cgo local ; prouvé ensuite par le job Linux de la CI, voir [CI](#ci) |

## Limites et suites

- CI et fusion de la PR B2c : faites sur GO distincts, voir [CI](#ci) ; la
  PR n'a reçu aucune approbation, le dépôt n'en exige pas.
- Points ouverts relevés par les revues, laissés à la décision du
  propriétaire : la ligne `unpromote-intent` d'ADR 0006 fait M4 avant de
  vérifier `retained` ; si `retained` est altéré, le coffre reste sans
  `current` jusqu'à une intervention, conforme à l'ADR, l'ordre inverse
  exigerait un amendement. Le recoupement du receipt de `<prev-op>` refuse
  une altération accidentelle ou partielle, pas une réécriture délibérée du
  receipt et du journal, hors modèle. Hérité de B2b : `RenderApply` affiche
  `operation: <id>` pour un refus de code 1 ou 2 levé avant le dossier
  d'opération.
- Un `retained\<X>` autre que celui de la génération courante n'est expliqué
  que par le nom de son receipt ; aucune action de B2c ne le touche.
- Le pilote B2c a été exécuté par le propriétaire après la fusion, voir
  [Pilote B2c](#pilote-b2c) ; sa qualification fonctionnelle, et donc la
  clôture de B2c, lui reviennent.
- Les fenêtres résiduelles du §5 et l'accumulation des générations sont des
  limites documentées, pas des défauts.
- Un coffre copié ou déplacé n'est pas pris en charge ; sa portabilité relève
  d'une décision ultérieure (plan §13).

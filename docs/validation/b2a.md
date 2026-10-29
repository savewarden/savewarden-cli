# B2a — Rapport de validation

Date : **29 octobre 2026**. Lot B2a, plan de `sync` en lecture seule, selon le
[plan B2 v0.2](../../.plans/Savewarden_Plan_B2_Synchronisation_v0.2.md) §8.1.
Ce rapport accompagne la future PR ; il ne constitue pas une auto-approbation.

## Cadre

| Élément | Valeur |
| --- | --- |
| GO humain | Plan B2 v0.2 validé par le propriétaire après revue intégrale, puis GO B2a, le 29 octobre 2026 |
| Branche | `feat/b2a-sync-plan`, créée depuis `release/v0.1.0-a1` en `979c9a8` |
| PR | Pas encore ouverte : la branche n'est pas poussée, en attente de l'accord du propriétaire |
| Écritures | Aucune : `internal/app/readonly_test.go` est inchangé, tout le code de production reste en lecture seule |
| Revue indépendante | Revue en lecture seule du code, sans accès à mes conclusions : neuf constats, tous corrigés en `bd04b46` |

Commits, dans l'ordre du §8.1 :

| SHA | Commit |
| --- | --- |
| `081ea6a` | Plan B2 v0.2 validé ; seuls ses passages de statut diffèrent du fichier relu (`4a57683`, SHA-256 `81c9b631…c02783`) |
| `775208f` | ADR 0004, frontières de `sync` |
| `3f98b83` | ADR 0006, protocole de publication et de récupération, contenu du §5 du plan — **point de contrôle de conception** |
| `59e91df` | ADR 0007, contrat de `sync` et receipt v1 |
| `e335d89` | Types d'inventaire déplacés vers `entities/backup`, sortie `inspect` inchangée |
| `17c595d` | Modèles purs : différence, receipt et marqueur stricts, journal chaîné, automate, classement |
| `a3a4359` | `inspect.Service.Directory`, inventaire sans suivi d'un dossier quelconque ; EFS signalé |
| `b9bc5d9` | Feature `sync` : préflight et plan en lecture seule |
| `06bbcd1` | Commande `sync` en mode plan, codes 0/1/2/3 — **point de contrôle de bout en bout** |
| `1e95e4d` | Documentation B2a |
| `bd04b46` | Corrections de la revue indépendante |

Diff total depuis `979c9a8` : 54 fichiers, 8 053 insertions, 220 suppressions,
dont environ la moitié de tests.

## Critères d'acceptation B2a (plan §12)

| # | Critère | Preuve |
| --- | --- | --- |
| 1 | Le mode plan n'écrit rien | `TestSyncPlanDoesNotWrite` : binaire réel lancé deux fois, écriture refusée par ACL sur tout l'arbre (`write access denied during the run: true`), empreinte complète identique, sorties identiques ; `TestSyncPlanEndToEnd` : le coffre n'existe toujours pas après le plan |
| 2 | ADR 0004, 0006 et 0007 ; ADR 0006 contient tout le §5 | `docs/adr/0006-…` reprend le §5 validé à l'identique, seuls les renvois de section adaptés |
| 3 | Validation pure du journal et classement suivent les tableaux | `TestJournalAmbiguities`, `TestRecordBeyondTheEnvelope`, `TestFirstPublicationAndRotationDoNotMix` ; `TestRecoveryTablesAreClosed` essaie les 125 observations pour chaque dernière étape, première publication et rotation : seules les lignes d'ADR 0006 passent ; `TestPreparedTemporariesMustMatchTheObservation`, `TestRecoveryEnvelopeE1`, `E2AndE3` ; `TestClassifyVaultRoot` pour l'initialisation |
| 4 | Plan conforme aux §4.4 et §4.5 ; `inspect` inchangé ; garde inchangé | `TestRemovalsAndMissingSources`, `TestCurrentIsCheckedAgainstItsReceipt` ; tests de sortie exacte d'`inspect` passés sans modification de leurs attendus ; `readonly_test.go` non modifié |
| 5 | Receipt v1 avec provenance | `TestReceiptRoundTrip`, `TestReconstructedReceiptProvenance`, `TestReceiptStrictness`, `TestDocumentedReceiptExample` |
| 19 | 7 checks verts sur le SHA exact | **Non établi** : branche non poussée, aucun run CI (voir CI) |
| 20 | Docs cohérentes, exemples vérifiés | Plan de `docs/commands/sync.md` et receipt de `docs/receipt-v1.md` vérifiés par test ; `TestReadmeCommandExamples` couvre `sync` |
| 21 | Aucun accès agent aux données protégées, aucun chemin personnel | Tests en `t.TempDir()` ou en mémoire, chemins fictifs ; aucun accès à `D:\Ga`, `D:\Savewarden`, `E:\GMS` |

## Contrôles locaux

Poste Windows 11, `go1.27.2 windows/amd64`, sur `bd04b46`.

| Commande | Résultat |
| --- | --- |
| `gofmt -l cmd internal .github/scripts` | Aucune sortie |
| `go mod tidy`, puis `git diff --exit-code -- go.mod go.sum` | Aucun écart ; aucune dépendance ajoutée |
| `go mod verify` | `all modules verified` |
| `go vet ./...`, `GOOS` windows, linux, darwin, freebsd, plan9 ; `go vet ./.github/scripts` | Succès |
| `go test -count=1 ./...` | `ok` pour les 9 packages |
| `go test -count=1 ./.github/scripts` ; `go run ./.github/scripts` | `ok` ; `Workflow Action, Go tool and OS pins verified` |
| `go build -trimpath -o bin/savewarden ./cmd/savewarden` ; `CGO_ENABLED=0` pour windows, linux, darwin × amd64, arm64 | Succès, compilation seulement hors Windows |
| golangci-lint v2.14.0, compilé pour l'hôte, `GOOS` windows, linux, darwin | `0 issues.` trois fois |
| govulncheck v1.6.0 | `No vulnerabilities found` dans le code ; voir les limites |
| `go test -race ./...` | Non exécuté : pas de cgo ni de toolchain C locale ; prouvé par le job Linux de la CI |

Tests réels Windows de ce lot : chiffrement EFS par `cipher /e` détecté
(`TestEncryptedFilesAreReported`, exécuté pour de vrai sur ce poste) ; jonction
dans la source, dossier étranger, coffre dans l'installation, initialisation
interrompue, avec leurs codes 2, 1, 1 et 3 (`TestSyncPlanExitCodes`).

## CI

**Aucun run.** La branche n'est pas poussée : aucune exécution GitHub Actions
ne valide ce lot. Les sept checks requis devront être verts sur le SHA exact de
la PR, lien du run joint, avant tout Human GO de fusion. Une compilation
croisée locale ne prouve que la compilation.

## Revue indépendante

Revue en lecture seule de tout le code de B2a, menée par un agent séparé à
partir des ADR et du plan, sans mes conclusions. Verdict : rien de bloquant à
l'exécution pour B2a, puisque rien n'écrit ; neuf constats, dont un bloquant
pour B2b. Tous corrigés et testés en `bd04b46` :

| # | Constat | Correction |
| --- | --- | --- |
| 1 | Le classement ignorait l'état pour lequel un temporaire avait été préparé | Un temporaire préparé exige l'état d'avant son intention, sinon ambiguïté |
| 2 | Clés JSON comparées avec la casse, alors qu'`encoding/json` les fait correspondre sans elle, et avec repli Unicode | Clés exigées en ASCII minuscule |
| 3 | Sources qui se chevauchent par nom court acceptées par `sync`, refusées par `inspect` | Refus en code 1, comme `inspect` |
| 4 | Dossier de jeu ne différant que par la casse ignoré | Intervention `unexpected-entry` |
| 5 | Première publication et rotation pouvaient se mêler ; `previous` non validé | Automate distinct, `previous` validé |
| 6 | Chaîne vide lue comme `null` ; receipt d'un autre jeu accepté | Refusés |
| 7 | En-tête `current: none`, `pending: none` sur une intervention | `not checked, intervention required`, `unknown, ambiguous journal` |
| 8 | Journal illisible en code 1 au lieu d'une ambiguïté | Intervention `journal-ambiguous`, code 3 |
| 9 | Test de la limite de 11 enregistrements inopérant | Douzième enregistrement réel refusé |

La revue n'a pu comparer `inspect` octet pour octet avec `979c9a8` : cette
preuve vient des tests de sortie exacte, inchangés et verts.

## Écarts au plan

- Le port de lecture de `sync` s'appelle `Files`, de portée `Scope`, au lieu
  de `Sources` : en B2a il lit le coffre et compare des identités, pas les
  fichiers source. ADR 0004 le nomme ainsi ; B2b ajoutera ses ports.
- `platform/filesystem` modifié, alors que le §7 ne le prévoyait pas pour B2a :
  détection EFS exigée par le §4.3, et message Unix qui ne nomme plus
  `inspect` seul.
- `entities/game` exporte `CheckWindowsName` pour juger les noms trouvés sur
  disque, sans dupliquer la grammaire.
- Les opérations en attente sont listées et bloquent en code 3, mais ne sont
  pas encore classées par identité : l'observation des identités de fichier
  arrive avec `platform/vault` en B2b. Le classement pur existe et est testé ;
  la phrase « pending recovery: current published, receipt missing » d'ADR 0006
  viendra avec lui.
- Pour une opération terminale `published` dont `current` ne correspond plus,
  le plan applique le §4.5, `current-modified` en code 2 ; ADR 0006 le précise
  dans ses conséquences.
- L'enveloppe E1 est comptée **par numéro d'enregistrement**, la lecture la
  plus stricte des deux formulations d'ADR 0006.
- Les champs propres à une étape sont rangés sous la clé `fields` de
  l'enregistrement ; l'exemple d'ADR 0006 les montre par `…`.
- L'en-tête du plan ajoute `volume`, `current`, `pending` et `plan` à l'exemple
  du §4.8 ; ADR 0007 et le guide les décrivent.
- Un commit supplémentaire, `bd04b46`, pour les corrections de revue.

## Risques résiduels et limites

- Pas d'instantané atomique, comme `inspect`.
- Type de volume du coffre non contrôlé avant B2b ; `subst` non détecté.
- govulncheck signale au niveau module GO-2026-6809, `crypto/x509` de la
  bibliothèque standard Go 1.27.2, corrigée en 1.27.3, que le code n'appelle
  pas. La mise à jour de la toolchain épinglée relève d'une décision distincte.
- Le garde statique ne couvre que les API connues ; les tests dynamiques de
  non-mutation le complètent.
- Mainteneur unique : la revue indépendante est un avis, pas une approbation ;
  le Human GO reste le contrôle final.

## Questions pour le propriétaire

1. Confirmer la lecture stricte de l'enveloppe E1, par numéro d'enregistrement.
2. Confirmer le rangement des champs d'étape sous `fields` avant que B2b
   n'écrive de journal ; sinon amender ADR 0006.
3. Autoriser le push de `feat/b2a-sync-plan` et l'ouverture de la PR vers
   `release/v0.1.0-a1`.

## Recommandation

**GO technique pour revue de la PR**, sous réserve des sept checks verts sur le
SHA exact : périmètre B2a livré, entièrement en lecture seule, critères 1 à 5,
20 et 21 couverts, revue indépendante intégrée. **Pas d'auto-approbation** :
push, PR, fusion et GO B2b attendent chacun une décision du propriétaire.
Aucun tag ni release.

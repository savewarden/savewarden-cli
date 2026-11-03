# Savewarden — Plan B4 : restauration vers un dossier isolé

> **Statut : v0.2 — VALIDÉ PAR LE PROPRIÉTAIRE le 3 novembre 2026, sur `342167f` certifié par le CTO : R1 à R15, X1 à X4 et arbitrages Q7 à Q9 ratifiés ; corrections C1 à C4, revue croisée indépendante et ajustements V1 à V3 intégrés ; ADR 0004 (amendement B4), 0006 §11 et 0009 acceptés ; GO de développement B4 non donné ; push, PR, fusion et pilote soumis à des GO distincts**
> Version du plan : `0.2` — 1er novembre 2026 ; remplace le brouillon v0.1 (`1bb33d7`)
> Lot : **B4**, cible `release/v0.1.0-a1`
> Cadre : [plan directeur v0.1](Savewarden_Plan_Bootstrap_Architecture_GitOps_v0.1.md) §7 B4,
> [plan B3 v0.2](Savewarden_Plan_B3_Snapshots_v0.2.md), [AGENTS.md](../AGENTS.md),
> [modèle de sécurité](../docs/safety-model.md), ADR [0001](../docs/adr/0001-fsd-inspired-go.md),
> [0004](../docs/adr/0004-sync-boundaries.md) à [0008](../docs/adr/0008-snapshot-contract-receipt-v1.md).

B3 produit des snapshots : des copies vérifiées de `current`, approuvées à
leur publication et jamais réécrites ensuite. B4 permet de les **récupérer** :
copier un snapshot approuvé, vérifié de bout en bout, vers un dossier que
l'utilisateur désigne, **hors du jeu et hors du coffre**, sans jamais rien
remplacer ni supprimer. L'utilisateur reprend ensuite lui-même les fichiers
dont il a besoin ; la restauration en place, dans le dossier du jeu, reste
hors de B4.

Le plan directeur fixe le cap : sélection explicite d'un snapshot, dossier de
destination séparé, préflight, copie vérifiée, aucun écrasement implicite,
mode sans écriture ; aucune restauration dans le dossier du jeu, qui exigera
un ADR de sécurité et la protection de l'état actif.

ADR 0006 reste la **référence de sécurité** : son §1 range déjà « la
restauration vers un dossier isolé en B4, pour son staging et sa
publication » dans sa portée. Son amendement, le §11, rédigé avec cette
v0.2 et accepté avec elle le 3 novembre 2026, porte le protocole de la
restauration et nomme ses écarts X1 à X4.

Ce plan ne vaut ni GO de développement, ni autorisation de push, de tag ou de
release, ni accès aux sauvegardes réelles. Ses ADR ont été acceptés par une
décision distincte du propriétaire, le 3 novembre 2026.

---

## 0. Décisions

### 0.1 Décisions R1 à R15

Revue CTO de la v0.1, transmise au propriétaire le 1er novembre 2026 :
architecture favorable, aucun P0 ; **R1 à R15 et X1 à X4 recommandés tels
quels**, sous réserve de quatre corrections normatives, intégrées (§0.5). Le
propriétaire les a ratifiés le 3 novembre 2026, avec l'acceptation des ADR,
après la certification par le CTO du paquet `342167f`. La première question
de fond était celle de
l'emplacement du journal : B4 est la première opération qui écrit hors du
coffre (R3 à R5).

| # | Décision proposée | Raison principale | Où |
| --- | --- | --- | --- |
| R1 | **Source : un snapshot approuvé et intact**, désigné par `--snapshot <snap-id>` exact. Ni `current`, ni `retained\`, ni `.incomplete`, ni snapshot dégradé, ni dossier externe ; aucun alias « dernier » | Seuls les snapshots sont approuvés et immuables ; sélection explicite exigée par le plan directeur | §4.3 |
| R2 | **Authentification complète et ciblée** avant tout journal : journal du snapshot terminal en `published`, receipt de snapshot strict et d'empreinte journalisée, identité publiée, absence de `.incomplete`, contenu relu en entier, identité relue après. Seul ce snapshot est lu. Une lecture impossible est distinguée d'une altération démontrée ; un snapshot encore en cours ou ambigu côté coffre n'est « pas restaurable maintenant », code 1 | Ne restaurer que ce que la chaîne de preuve de B3 établit ; applique à B4 la recommandation P2-4 (§0.3) | §4.3 |
| R3 | **Destination : une zone de restauration marquée**, créée par `restore` dans un dossier absent ou vide, ou zone existante du même jeu. Jamais d'écriture dans un dossier existant non marqué | ADR 0006 §8 interdit d'écrire dans un dossier non marqué ; aucun mélange avec des fichiers de l'utilisateur | §4.2 |
| R4 | **Coffre en lecture seule** : `restore` n'y écrit rien, ni journal, ni receipt, ni verrou ; il ne prend pas `sync.lock` et n'exclut ni `sync` ni `snapshot` | Un snapshot publié n'est plus jamais modifié par Savewarden ; le coffre reste intact, compatible avec les binaires B3, utilisable même protégé en écriture | §4.4, §5.7 |
| R5 | **Journal, receipt et verrou dans la zone** ; la récupération ne lit que la zone et ne dépend pas du coffre | Une restauration interrompue se termine même si le coffre est débranché ou altéré entre-temps | §4.5 |
| R6 | **Zone d'un seul jeu, restaurations successives**, chacune publiée sous `<dest>\<op>\` ; verrou et I8 par zone | Un échec ne condamne pas la zone ; même grammaire que le domaine des snapshots, déjà qualifiée | §4.2, §5.7 |
| R7 | **Pas d'idempotence par snapshot** : chaque `--apply` publie une nouvelle copie vérifiée ; le plan signale les restaurations antérieures du même snapshot dans la zone | Une copie restaurée est faite pour être utilisée et modifiée ; en obtenir une neuve est l'effet attendu | §4.7 |
| R8 | **Remise à l'utilisateur** : une restauration publiée n'est jamais rouverte en écriture, déplacée ni supprimée par Savewarden, et n'est plus vérifiée ; sa présence est signalée en information, jamais bloquante | Le receipt atteste l'état à la publication ; la suite appartient à l'utilisateur | §4.7 |
| R9 | **Aucune porte de provenance rejouée sur le coffre après `planned`** : la copie et sa relecture sont comparées au plan journalisé | Le plan provient du snapshot authentifié ; R5 exige une récupération sans le coffre | §4.4, §5.5 |
| R10 | **Code dans `features/sync`**, troisième opération du moteur, avec une **couture interne explicite** : source de lecture et cible d'écriture distinctes, non exportées ; commande CLI `restore` distincte ; extraction du moteur **non retenue pour B4**, critères de réexamen écrits | `restore` a besoin du moteur et du lecteur du domaine des snapshots, tous deux dans `features/sync` ; la couture prépare une extraction future sans frontière nouvelle | §3.1 |
| R11 | **Deux schémas stricts nouveaux** : receipt de restauration `savewarden.restore` v1 et marqueur de zone `savewarden.restore-area` v1 | Même discipline que le receipt v1 et le marqueur du coffre | §4.6 |
| R12 | **Distinctions** : zone sans recouvrement avec le coffre, l'installation ou une source, par segments et par identité **quelle que soit la lettre de lecteur** ; aucun coffre ni zone parmi ses ancêtres ; volume NTFS local, fixe ou amovible ; contrôles refaits par la récupération ; **une identité impossible à observer ne prouve jamais la séparation** (C3) | Pas de restauration en place, ni dans le coffre, ni dans une autre zone, même par un alias `subst` ou une zone déplacée | §4.2, §4.5 |
| R13 | **Commande `restore`**, plan par défaut, `--apply`, `--recover` ; `--recover` refuse `--vault` et `--snapshot` | Mêmes conventions que `sync` et `snapshot` ; le plan sans `--apply` est le `--dry-run` du plan directeur | §4.1 |
| R14 | **Pilote** par le propriétaire, depuis le coffre du pilote B3 **lu seulement**, vers une zone neuve hors du jeu ; empreinte du coffre identique avant et après | Prouve R4 sur données réelles sans créer de coffre | §11 |
| R15 | **Coffre copié vers un autre volume** : identités changées, snapshots non reconnus, restauration refusée sans lire le contenu ni présumer une corruption ; portabilité renvoyée à un lot dédié. Un disque externe simplement déplacé n'est pas concerné, sous l'hypothèse NTFS écrite au §4.3 | Cohérent avec l'arbitrage ratifié de B2c : aucune identité adoptée | §4.3, §13 |

Écartés d'emblée, hors de B4 : restauration en place, restauration partielle,
API d'écriture nouvelle, attributs ou ACL, suppression ou nettoyage, toute
écriture dans le coffre.

### 0.2 Écarts à ADR 0006, nommés comme tels

Comme B3 nommait I8 par domaine et S13, B4 nomme ses assouplissements ; chacun
est écrit dans ADR 0006 §11 avec sa raison.

| # | Écart | Raison |
| --- | --- | --- |
| X1 | **Ni verrou ni I8 du coffre** pour `restore`, qui ne sonde pas non plus `sync.lock` | Prémisse, écrite comme invariant du §11 : aucune opération ne renomme un snapshot publié ni ses ancêtres, ni ne les ouvre sans partage en lecture. Sonder `sync.lock`, comme `lockBusy`, l'ouvrirait un instant et ferait échouer le `Lock` d'un `sync` concurrent, contre R4 |
| X2 | **Snapshot remplacé pendant la copie** : `failed`, code 2, au lieu de l'ambiguïté du §5 d'ADR 0006 | Le dossier remplacé est dans le coffre, que `restore` n'écrit pas ; la zone reste cohérente, rien n'est publié. Le diagnostic nomme le remplacement, sans le prendre pour un changement de contenu |
| X3 | **Lecture ciblée** : les anomalies du coffre hors du snapshot désigné sont ignorées, dont un identifiant d'opération partagé avec le domaine de `current`, que `snapshot` tient pour ambigu | L'authenticité du snapshot désigné repose sur sa propre chaîne de preuve, relue en entier ; P2-3 recommande aussi de lire moins |
| X4 | **État du snapshot côté coffre non terminal ou ambigu** : code 1, « not restorable », au lieu d'un code 3 | `restore` ne peut ni distinguer une opération vivante d'une opération interrompue sans sonder le verrou (X1), ni intervenir sur le coffre ; le code 3 reste réservé aux ambiguïtés de la zone, que `restore` récupère |

### 0.3 Points ouverts de B3

Le rapport [`docs/validation/b3.md`](../docs/validation/b3.md) consigne des
points soumis au propriétaire, dont l'arbitrage devait précéder ce cadrage. Le
1er novembre 2026, le propriétaire a transmis, avec le GO de cadrage B4, les
recommandations du CTO ci-dessous ; leur confirmation formelle accompagne la
validation de ce plan.

| Point | Recommandation transmise | Effet sur B4 |
| --- | --- | --- |
| P2-4 de la revue de sécurité, lecture d'un snapshot assimilée à une modification | Corriger plus tard : distinguer indisponibilité et altération | **Appliqué aux seules lectures de `restore`** : un snapshot illisible donne code 1, « not verifiable », jamais `snapshot-modified` (R2). La classification de `snapshot` reste celle de B3, inchangée |
| P2-3, budget global de lecture | À planifier, sans retarder B4 | `restore` ne lit que le snapshot désigné et sa zone, aux bornes par fichier existantes ; le budget global reste ouvert |
| `.gitignore` des artefacts du coffre | Petit lot de durcissement | Hors de B4 |
| N2, volume momentanément indisponible | Documenter, puis améliorer la classification | Hors de B4 pour l'adaptateur : un volume non prêt peut encore faire apparaître le snapshot absent au préflight, refus sûr au diagnostic inexact, documenté. Pendant la copie, `restore` contrôle de nouveau la racine et le marqueur du coffre avant de conclure (§4.4) |
| Profondeur JSON | Accepter la protection de Go 1.27.3 | Mêmes décodeurs |
| Tolérances héritées de B2 | Conserver le suivi | Inchangées |

### 0.4 Questions au propriétaire et avis du CTO

| # | Question de la v0.1 | Avis de la revue CTO |
| --- | --- | --- |
| 1 | **Zone ou journal dans le coffre (R3 à R5).** La zone garde le coffre en lecture seule mais publie sous `<dest>\<op>\<source-id>\` ; l'alternative écrirait le coffre, le ferait refuser par les binaires B3 et écrirait un staging dans un dossier non marqué (§3.1) | Valider R3 à R5 : la récupération ne doit pas exiger de modifier le coffre |
| 2 | **Code 1 pour un snapshot en cours ou ambigu côté coffre (X4)** | Valider X4 : `restore` ne peut pas réparer ce domaine ; le code 3 reste réservé à sa zone |
| 3 | **Zones sur lecteur amovible**, acceptées comme le coffre | Valider R12, sous les mêmes contrôles |
| 4 | **`P` renommé dans la zone** : entrée inexpliquée, code 3 pour la zone | Valider avec précision : nom inconnu, entrée inexpliquée ; copie remise dégradée sous son nom attendu, information (C1) |
| 5 | **Pas d'idempotence par snapshot (R7)** | Valider R7 : une restauration est une remise, pas un snapshot de plus |
| 6 | **Option (a) avec couture interne (R10)** | Valider R10 (a), couture en commit isolé soumis à la revue de non-régression avant tout code de `restore` |

Questions ouvertes par la revue croisée de la v0.2 (§0.6), arbitrées par la
revue CTO de la v0.2 (§0.7) :

| # | Question | Proposition de ce plan | Avis de la revue CTO de la v0.2 |
| --- | --- | --- | --- |
| 7 | **Chemin de référence sur une lettre sans volume** : disque du jeu perdu ou débranché, alias `subst` retiré | **Refus**, code 1, « separation cannot be proven » : la lettre a pu désigner un alias ou un volume monté aujourd'hui sous une autre lettre, et aucune règle par les noms ne le démontre. Pas de restauration tant qu'une lettre de référence n'a pas de volume | **Accepté**, avec une limitation fonctionnelle explicite : B4 ne garantit pas la restauration lorsque le disque d'origine est indisponible, et ne se présente pas comme une reprise après sinistre complète. Rattacher un volume de remplacement rend les identités observables sans prouver l'identité historique de l'ancien disque. Réexamen dans un lot dédié |
| 8 | **Receipt d'une restauration remise perdu** par une coupure | Ambiguïté, code 3, qui ferme la zone aux nouvelles restaurations ; la copie remise reste en place ; une nouvelle zone reste possible | **Accepté** : aucune reconstruction conjecturale ; copie conservée, examinable à la main ; nouvelle zone possible |
| 9 | **Chemins réellement sauvegardés** : ceux des `sources` du receipt de snapshot | Ajoutés aux chemins de référence, à la création comme à la récupération, sans exiger qu'ils égalent ceux du manifeste | **Accepté, ajout obligatoire** : références historiques des données sauvegardées, complétant le manifeste actuel, qui a pu évoluer |

### 0.5 Corrections de la revue CTO de la v0.1

| # | Constat | Correction intégrée | Où |
| --- | --- | --- | --- |
| C1 | La grammaire de la zone tenait un fichier ou un lien à la place d'un dossier pour une entrée imprévue, code 3, alors que la remise rend non bloquante toute observation d'une restauration publiée | Grammaire en deux classes : les noms de structure et les objets non remis restent stricts ; un objet qui porte le nom d'une restauration remise, `<op>` ou `<op>.incomplete`, est classé par le protocole de remise **quel que soit son type**, dossier, fichier, lien, jonction ou point d'analyse, observé sans suivi, jamais ouvert, lu, déplacé ni réutilisé | §4.2, §4.7, §5.5 ; tests |
| C2 | « Zone marquée sans `journal\` : rien à récupérer » acceptait une zone portant des `P`, des `.incomplete` ou des receipts sans journal | Code 0 seulement pour une zone sans aucun objet d'opération ; tout objet d'opération qu'aucun journal n'explique est une ambiguïté, code 3 | §4.5, §5.5 ; tests |
| C3 | Distinctions refaites par la récupération sans règle pour une installation ou une source absente, déplacée, ou sur un volume indisponible | Règle d'observabilité unique, à la création comme à la récupération : décisive quand la chaîne de `X` s'observe ; **refus**, code 1, quand elle ne s'observe pas ou que la lettre de `X` n'a pas de volume, après la revue croisée (§0.6, P0-1). Distinction écrite entre l'indépendance au coffre et la dépendance au manifeste | §4.2, §4.5 ; tests |
| C4 | Le recouvrement entre lettres n'était prouvé que sur le faux | Test Windows réel avec un alias `subst` sur une arborescence synthétique temporaire, exécuté par la CI Windows, où l'échec de l'alias est un échec du test | §9, §12 |

### 0.6 Revue croisée indépendante de la v0.2 et des projets d'ADR

Faite en lecture seule sur les textes de cette v0.2, comme le CTO le
demandait, sur les crashs, les objets remis et les distinctions de
destination. Deux P0 et neuf P1, tous intégrés ; P2 intégrés.

| # | Constat | Correction | Où |
| --- | --- | --- | --- |
| P0-1 | La ligne « lettre sans volume » de C3 concluait la séparation par les noms sous la racine de la lettre ; un alias `subst` retiré ou un volume remonté sous une autre lettre la mettait en défaut, contre le principe de C3 | Refus, code 1 ; l'information `reference-unavailable` est retirée ; question 7 | §4.2, ADR 0006 §11, ADR 0009 |
| P0-2 | La récupération prenait `restore.lock`, que `Lock` crée, dans un dossier encore non marqué : une reprise de Z3 interrompue rendait la zone irrécupérable, et un `<dest>` vide devenait non vide | « Rien à récupérer » n'ouvre aucune session et n'écrit rien ; la reprise de Z3 valide le marqueur sans verrou ; aucun verrou dans un dossier non marqué | §4.2, §4.5, ADR 0006 §11 |
| P1-1 | Chaînes d'identités sans la racine du volume, qui est l'identité d'un alias `subst` | Chaînes depuis la racine, racine comprise | §4.2 |
| P1-2 | Lignes de la règle d'observabilité qui se recouvraient | Ordre de décision, refus en tête ; comparaison dans les deux sens | §4.2 |
| P1-3 | `X` absent sur une lettre reprise par un autre disque amovible | Comparaison complémentaire des noms sous la racine de la lettre ; décisif pour `X` tel qu'il se résout aujourd'hui ; limite écrite | §4.2 |
| P1-4 | Chemins de référence liés au snapshot par le seul identifiant du jeu | Chemins des `sources` du receipt de snapshot ajoutés, lus dans le journal de la zone ; question 9 | §4.2 |
| P1-5 | `I` d'une opération close contrôlé par son type seulement | Identité `new` exigée dès `staged` | §4.2, §5.5 |
| P1-6 | Tableau « destination sans opération journalisée » incomplet | Entrée étrangère dans une zone marquée, code 3 ; `<dest>` non dossier, code 1 ; ordre de décision | §4.5 |
| P1-7 | E1 à E4 désignaient à la fois l'enveloppe et les écarts | Écarts renommés X1 à X4 | Partout |
| P1-8 | ADR 0009 contredisait ADR 0006 §11 sur une destination absente en récupération | Code 0 écrit dans les deux | ADR 0009 |
| P1-9 | Une copie remise supprimée entre le plan et Z6 bloquait une nouvelle restauration | Observations des restaurations remises exclues de la comparaison de Z6 | §4.2, ADR 0009 |

### 0.7 Revue CTO de la v0.2

Transmise par le propriétaire le 2 novembre 2026 : empreintes des quatre
documents conformes ; **avis favorable, aucun nouveau P0 démontré** ; les
deux P0 de la revue croisée correctement traités ; Q7 à Q9 arbitrées (§0.4) ;
R1 à R15, X1 à X4 et les frontières proposées conservés ; pas d'extraction du
moteur dans ce lot. Ce n'est pas l'acceptation formelle des ADR, qui reste au
propriétaire, ni un GO de développement. Trois ajustements, intégrés :

| # | Constat | Correction intégrée | Où |
| --- | --- | --- | --- |
| V1 | Deux reprises `--recover --apply` de Z3, sans verrou, peuvent viser la même zone : issue du perdant non écrite | Concurrence écrite : un seul renommage du marqueur réussit ; le perdant n'écrit plus rien et sort en code 1, `destination-changed`, son temporaire restant un essai abandonné ; E1 borne les essais. Test Windows réel de concurrence, et test déterministe sur le faux | §4.2, §9, ADR 0006 §11, ADR 0009 |
| V2 | Le statut du §10 d'ADR 0006 disait encore qu'aucun code ne l'implémentait | Correction documentaire dans un commit séparé des changements normatifs, avec sa propre entrée d'amendement ; aucune règle de B3 ne change | ADR 0006 |
| V3 | Les anciennes phrases d'ADR 0004 présentaient encore `snapshot` et `restore` comme de futures features | Phrases conservées, marquées historiques avec leur renvoi ; priorité de l'amendement écrite | ADR 0004 |

---

## 1. Contexte et preuves

| Élément | État vérifié le 1er novembre 2026 |
| --- | --- |
| `release/v0.1.0-a1` | `f3074d0`, merge de la PR #12, clôture documentaire de B3 ; parents `08ec0c2` et `97233e0` |
| CI | Run 41321636100 sur la tête de la PR #12 et run post-fusion 41322204062 sur `f3074d0` : 7/7, `success` |
| B3 | Clos dans son périmètre : PR #11 fusionnée (`08ec0c2`), pilote Odyssey exécuté et qualifié par le propriétaire, clôture vérifiée par le CTO ; aucune release |
| Cadrage B4 | Ouvert sur le GO du propriétaire du 1er novembre 2026 ; branche locale `docs/b4-framing`, créée depuis `f3074d0` |
| Toolchain | Go 1.27.3, golangci-lint 2.14.0 |
| Coffre du pilote B3 | Contient un snapshot publié et qualifié ; jamais lu par l'agent. Source naturelle du pilote B4, en lecture seule (R14) |

Constats du code, relus à `f3074d0` :

- **Session d'écriture.** `Vaults.Open(path)` (`platform/vault`) accepte tout
  chemin absolu et propre sous une lettre de lecteur, épingle ses ancêtres
  sans les suivre, et accepte un dossier final absent si son parent existe.
  Une zone de restauration peut donc être ouverte comme une session, **sans
  API nouvelle** ; ADR 0005 et `readonly_test.go` n'ont pas à changer.
- **Moteur.** `operation` (`apply.go`, `publish.go`, `copy.go`) construit ses
  noms comme `o.game + "/" + …` et prend `o.game + "/sync.lock"`. Depuis B3,
  `domain` (`domain.go`) paramètre les journaux, stagings, noms publiés,
  constructeur d'enregistrement, code de changement et message du verrou ; ni
  la **racine** de session ni le **verrou** ne le sont. Surtout, les
  relectures et le résultat passent par `o.plan.vaultVolume` et
  `o.plan.vaultSegments` (`copy.go`, `verify` ; `snapshot_apply.go` ;
  `snapshot_recover.go` ; `apply.go`, `result`), et les diagnostics par le
  préfixe `vault <chemin>:` (`errVault`, `vault.go`) : le moteur suppose que
  l'on lit et écrit dans **une même racine**, le coffre. `restore` est la
  première opération qui lit dans une racine et écrit dans une autre.
- **Lecture des snapshots.** `readSnapshots` (`snapshot.go`) lit tout le
  domaine des snapshots ; la récupération l'appelle déjà sans le domaine de
  `current` (`v` nul). Mais le receipt illisible d'un snapshot quelconque
  interrompt toute la lecture (`readReceipts`, `errVault`), et un identifiant
  partagé avec `current` y est une ambiguïté : B4 a besoin d'un lecteur
  **ciblé** d'un seul snapshot. `entities/backup` fournit déjà
  `ParseSnapshotJournal`, `CheckSnapshotReceipt` et
  `PublishedSnapshotCondition`, sans I/O.
- **Recouvrements.** `distinctByName` et `overlapping` (`vault.go`), et
  leurs appels (`sync.go`, `snapshot.go`), ne comparent chemins et identités
  que si les lettres de lecteur sont égales : un alias `subst` d'une source
  passe ces contrôles, limite déjà écrite pour `inspect`. B4 doit comparer
  les identités quelle que soit la lettre (R12).
- **Copie depuis le coffre.** `copyFile` (`copy.go`) traduit un fichier
  source absent en changement, code 2 : un coffre débranché pendant la copie
  y serait pris pour un snapshot modifié (§4.4).
- **Copie.** `copyContent` (`snapshot_apply.go`) copie un dossier du coffre
  décrit par les `sources` d'un receipt vers un `.incomplete` ; c'est
  exactement le besoin de B4, la session de destination près.
- **Marqueur.** Le marqueur du coffre est un JSON strict `savewarden.vault`
  v1 : `schema`, `schema_version`, `created_at`, `tool_version`, validé par
  temporaire et renommage selon I3 et I4 d'ADR 0006 §4.
- **Receipts.** Les `sources` du receipt v1 portent le chemin absolu de chaque
  source, celui du manifeste. Le receipt de snapshot les recopie ; le receipt
  de restauration les recopiera aussi : ce sont des données de l'utilisateur,
  jamais versionnées.
- **Tests.** Les faux du package `sync_test` (`fakeFS`, `fakeVaults`,
  `crashBefore`, `crashAfter`, empreintes, énumérations des crashs) et les
  helpers binaires de `cmd/savewarden` servent tels quels à une opération de
  plus du même package.

---

## 2. Objectif, non-objectifs, permissions

### 2.1 Objectif

```text
savewarden restore --manifest <m> --vault <v> --snapshot <snap-id> --to <dest> [--apply]
savewarden restore --manifest <m> --to <dest> --recover [--apply]
```

1. Sans `--apply`, affiche un **plan** sans rien écrire : snapshot authentifié
   ou motif du refus, état de la destination, ce que `--apply` copierait,
   espace requis, restaurations antérieures et opérations en attente de la
   zone.
2. Avec `--apply`, authentifie le snapshot, prépare la zone, copie le
   snapshot dans `<dest>\<op>.incomplete\`, relit la copie depuis le disque,
   la compare au plan, puis la publie par renommage en `<dest>\<op>\` avec un
   receipt de restauration.
3. N'écrit **que dans la zone** : jamais dans le coffre, jamais dans le
   dossier du jeu ou d'une source, jamais dans un dossier existant non marqué,
   hormis un dossier vide désigné par `--to`, qu'il marque aussitôt. Ne
   supprime, ne tronque, n'écrase et ne réécrit rien.
4. Une restauration interrompue se récupère par `restore --recover`, sans le
   coffre.
5. La restauration publiée est **remise** à l'utilisateur (R8).

### 2.2 Non-objectifs B4, opposables en revue

- **Restauration en place**, dans l'installation, le dossier de sauvegarde
  du jeu ou toute source du manifeste : lot futur, avec son ADR de sécurité.
- **Restauration de `current`, de `retained\`, d'un `.incomplete`**, d'un
  snapshot dégradé, non publié ou créé hors de Savewarden.
- **Restauration partielle** : une source, un fichier, un filtre.
- **Restauration depuis un coffre copié** vers un autre volume (R15).
- **Suppression, nettoyage, rétention** de zones, de restaurations ou de
  `.incomplete`.
- **Écriture dans le coffre**, sous quelque forme que ce soit, verrou compris.
- Export, archive, compression, chiffrement ; volumes autres que NTFS local,
  fixe ou amovible ; attributs, ACL, flux alternatifs.
- Vérification ultérieure des copies remises : `doctor`, B5, s'il le faut.
- Support fonctionnel Linux et macOS, sortie JSON de la CLI.

### 2.3 Permissions : agent, produit, propriétaire

| Acteur | Peut | Ne peut pas |
| --- | --- | --- |
| **Agent** | Lire et modifier le dépôt ; tests synthétiques en `t.TempDir()` | Explorer ou lire `D:\Ga`, `D:\Savewarden`, `E:\GMS` ou des clés SSH ; lancer Savewarden sur des données réelles sans autorisation explicite et nominative pour cette exécution |
| **Savewarden**, lancé par le propriétaire | Lire le manifeste désigné ; relever sans suivi l'identité de l'installation et des sources ; **lire seulement** le coffre passé à `--vault`, et dans celui-ci le seul snapshot désigné ; écrire **seulement** dans la zone passée à `--to` | Écrire dans le coffre, lire le contenu de la source ou de l'installation, écrire ailleurs, adopter, suivre un lien, supprimer |
| **Propriétaire** | Exécuter les pilotes, autoriser nominativement une exécution par l'agent | — |

---

## 3. Architecture et ADR

### 3.1 Où vit le code (R10) : réexamen de l'extraction du moteur

B3 avait retenu l'option (a) et renvoyé au cadrage de B4 le réexamen de
l'extraction, B4 étant « le premier consommateur réellement différent
puisqu'il écrira hors du coffre ». Ce réexamen tient à deux constats :

1. **`restore` a besoin de deux choses que possède `features/sync`** : le
   moteur d'écriture (enregistrements, enveloppe E1, retries, bascule, copie
   hachée, relecture, récupération) et le **lecteur du domaine des
   snapshots**, pour authentifier sa source. ADR 0001 interdit qu'une feature
   en importe une autre.
2. **Avec une zone (R3), l'écriture hors du coffre suit le protocole du
   domaine des snapshots.** La zone reprend sa grammaire, son automate et son
   tableau de récupération, sans porte de provenance (R9). Ce qui change pour
   le moteur est la séparation de **deux racines** : la source de lecture, le
   coffre, et la cible d'écriture, la zone, avec son verrou et le libellé de
   ses diagnostics (§1, constat « Moteur »).

| Option | Contenu | Pour | Contre |
| --- | --- | --- | --- |
| **(a) Proposée : troisième opération de `features/sync`, avec couture interne** | Fichiers `restore*.go` ; méthodes de service propres ; commande CLI `restore` ; types non exportés de **source de lecture** et de **cible d'écriture** (chemin, volume, segments, verrou, libellé des diagnostics) qui remplacent `o.game`, `lockFile` et `o.plan.vault*` dans le moteur, `operation` cessant d'embarquer le `Plan` de `sync` | Aucun déplacement de code qualifié ; aucune frontière nouvelle ; lecteur des snapshots et moteur réutilisés sur place ; faux et énumérations de crash réutilisés tels quels ; la couture est exactement ce qu'une extraction future détacherait | Le package `sync` porte trois commandes et environ deux mille lignes de plus, tests compris ; son nom ne dit plus tout ce qu'il fait ; la couture touche le moteur qualifié |
| (b) `features/restore` dupliquée | Copie du moteur, du lecteur et des faux | Package nommé comme la commande | Deux copies du code le plus sensible, qui divergeraient ; rejetée par ADR 0004 pour le parcours, et par B3 |
| (c) Moteur extrait et `features/restore` | Nouveau package de protocole, consommé par `sync` et `restore` ; authentification servie à `restore` par un port consommateur-défini que `sync.Service` satisfait, câblé par `app` comme `Inventories` (ADR 0004) | Frontières nommées par responsabilité ; `sync` allégé ; prépare d'autres opérations d'écriture | Déplacement de la plus grande partie du code qualifié par trois pilotes ; découpage de `operation`, qui mêle moteur, plan et rendu ; frontière durable nouvelle, son ADR et ses tests d'imports ; faux de test à déplacer ou dupliquer ; risque concentré sur le moteur au moment où une troisième opération s'y ajoute |

**Proposé : (a) en B4.** Le gain de (c) est de nommage et de lisibilité ; son
coût porte sur le code qualifié, sans bénéfice pour la sûreté de B4. La
documentation du package `sync` est réécrite : il est la feature des
**publications selon ADR 0006**, dans le coffre et dans une zone de
restauration. Amendement d'ADR 0004 ; aucune règle d'import ne change.

**Critères de réexamen de (c)**, consignés dans ADR 0004 : une quatrième
opération d'écriture, restauration en place ou requalification d'un coffre
copié notamment ; une opération qui aurait besoin du moteur sans le lecteur du
coffre ; une opération qui renommerait un snapshot publié ou ses ancêtres, ou
les tiendrait sans partage, rétention ou `doctor` par exemple, puisqu'elle
lèverait la prémisse de X1 ; ou une revue qui juge le package illisible. Si le propriétaire
préfère (c) dès B4, le commit 2 du §8 devient l'extraction, avec la même
revue de non-régression obligatoire, et ce plan passe en v0.3 avec un ADR de
frontière.

Fichiers prévus dans `features/sync`, sans `manager.go` ni fourre-tout :
`restore.go` (plan, authentification ciblée, lecture de la zone),
`restore_apply.go` (initialisation de la zone et automate),
`restore_recover.go` (récupération). La couture du moteur est un commit
distinct, sans changement de comportement pour `current` ni pour les
snapshots ; la revue de non-régression vérifie qu'aucun usage de la racine
de session ne passe plus par `o.plan.vault*`.

### 3.2 Ports

Aucun port nouveau. Le plan de restauration reçoit `ManifestReader`, `Files`,
`Inventories` et `Volumes` ; `--apply` et `--recover --apply` reçoivent en plus
`Vaults`, `Clock` et l'aléa. `Vaults.Open` n'est appelé **que sur la zone**,
jamais sur le coffre : propriété testée sur le faux, qui consigne chaque
chemin ouvert (§9). Le code de `platform/vault`, ADR 0005,
`readonly_test.go` et `architecture_test.go` sont inchangés. Seuls des
commentaires changent : ceux de `platform/vault` et de `ports.go` qui
parlent d'un « vault directory » ou de « sync.lock » diront qu'une session
s'ouvre sur un coffre ou sur une zone, avec son verrou.

### 3.3 ADR du lot

Rédigés sur `docs/b4-framing` avec cette v0.2, comme pour B3, et **acceptés
par le propriétaire le 3 novembre 2026**. Aucun code ne les implémente
encore. Ils sont versés comme premiers commits de la PR, avant tout code
d'écriture (§8).

| ADR | Objet |
| --- | --- |
| **0006, §11 nouveau** | Protocole de la restauration isolée : zone et son initialisation, grammaire, automate et contrat des champs, bascule, points d'arrêt, récupération indépendante du coffre et son tableau, remise, enveloppe ; **écarts X1 à X4** et leurs raisons ; **invariant** des snapshots publiés, prémisse de X1 ; une session d'écriture s'ouvre sur un coffre ou sur une zone. Amendements : §1, portée nommant les zones ; §2, I8 précisé par zone ; §8, « dossier marqué » couvrant coffre et zone |
| **0009, nouveau** | Contrat de `restore` : modes, préconditions d'authentification, de destination et de récupération, sortie du plan, codes 0 à 3, ligne d'état de la destination, codes stables et causes ; receipt de restauration v1 et marqueur de zone v1, relectures strictes |
| **0004, amendement** | `features/sync` possède aussi la restauration isolée ; couture interne entre source de lecture et cible d'écriture ; critères de réexamen de l'extraction. Les deux phrases qui laissaient `restore` « feature à cadrer », dans les Conséquences et dans l'amendement B3, sont remplacées |

ADR 0005, 0007 et 0008 ne sont pas amendés : aucune API d'écriture, aucun
changement de `sync` ni de `snapshot`. ADR 0008 annonçait déjà que B4
relirait le receipt de snapshot.

---

## 4. Contrats

### 4.1 Commande et modes (R13)

```text
savewarden restore --manifest <m> --vault <v> --snapshot <s> --to <d>           plan seul, rien n'est écrit
savewarden restore --manifest <m> --vault <v> --snapshot <s> --to <d> --apply   copie, vérifie, publie
savewarden restore --manifest <m> --to <d> --recover                            plan de récupération, rien n'est écrit
savewarden restore --manifest <m> --to <d> --recover --apply                    exécute la récupération
```

- `--manifest`, `--to` toujours obligatoires ; `--vault` et `--snapshot`
  obligatoires hors de `--recover` et **refusés** avec lui, code 1 : la
  récupération ne lit pas le coffre (R5). Aucun défaut, aucune variable
  d'environnement, aucun fichier de configuration.
- Le manifeste reste exigé par la récupération : identifiant du jeu, comparé
  à celui du marqueur, et distinctions refaites (§4.5).
- `--to` suit la grammaire de `--vault` : chemin absolu et propre sous une
  lettre de lecteur, noms Windows contrôlés. `--snapshot` suit
  `ValidOperationID`.
- Le manifeste fournit l'identifiant du jeu, l'installation et les sources
  pour les distinctions (R12). Le contenu des sources n'est pas lu.
- Le plan et `--recover` sans `--apply` ne reçoivent pas `Vaults` : ils ne
  peuvent rien écrire, prouvé par des tests binaires sous ACL de refus.
- Windows seulement, comme `sync` et `snapshot`.

### 4.2 Zone de restauration (R3, R6, R12)

```text
<dest>\
  savewarden-restore.json            marqueur de zone v1, et ses temporaires .tmp-<k>
  restore.lock                       verrou de la zone, créé vide, jamais écrit
  receipts\<op>.json                 receipt de restauration v1, et ses temporaires .tmp-<k>
  journal\<op>\NNN-<étape>.rec       journal de chaque opération de restauration
  <op>.incomplete\<source-id>\...    I : copie en cours, échouée ou abandonnée ; jamais supprimée
  <op>\<source-id>\...               P : restauration publiée, remise à l'utilisateur
```

- `<op>` est l'identifiant de l'opération de restauration, de forme
  `ValidOperationID` ; il ne peut valoir aucun nom de structure. Un
  identifiant déjà employé dans la zone, ou égal à celui du snapshot
  restauré, est refusé avant tout journal, code 1 ; une nouvelle exécution en
  tire un autre.
- **Marqueur** `savewarden-restore.json` : `schema`
  (`savewarden.restore-area`), `schema_version` (`1`), `created_at`,
  `tool_version`, `game` (identifiant du jeu). Strict, comme le marqueur du
  coffre. Une zone ne sert qu'à un jeu : un marqueur d'un autre jeu donne
  code 1.
- **Grammaire**, contrôlée par `restore`, chaque entrée observée par `Lstat`
  sans suivi, **en deux classes** (C1) :

  | Classe | Entrées | Règle |
  | --- | --- | --- |
  | Structure, jamais remise | Le marqueur et ses temporaires, `restore.lock` : fichiers ordinaires ; `receipts` et `journal` : dossiers ordinaires ; dans `receipts\`, des fichiers `<op>.json` et leurs temporaires ; dans `journal\`, des dossiers `<op>` au contenu du §3 d'ADR 0006 | Stricte, toujours : toute autre entrée, un lien, un fichier à la place d'un dossier ou l'inverse, un point d'analyse : `unexpected-entry`, code 3 |
  | Objets d'opération | `<op>` et `<op>.incomplete` à la racine | Selon l'état du journal de `<op>`, ci-dessous |

- **Objets d'opération**, comme ADR 0006 §3 et §10 :
  - **nom qu'aucune opération journalisée de la zone n'explique** :
    `unexpected-entry`, code 3, quel que soit son type ;
  - **opération non terminale** : `P` et `I` doivent être des dossiers
    ordinaires sans point d'analyse ; l'identité de `I` est exigée dès que
    `staged` l'a fixée ; un `I` dont le journal s'arrête avant `staged` est
    accepté, jamais lu ni déplacé ; tout autre type ou identité : ambiguïté,
    code 3 ;
  - **opération close autrement qu'en `published`** : un `<op>` présent, de
    quelque type que ce soit, est une ambiguïté, code 3 ; son `I` n'est ni
    relu ni exigé, mais un `I` présent doit être un dossier ordinaire sans
    point d'analyse, de l'identité `new` si `staged` l'a fixée, sinon
    ambiguïté, code 3 ;
  - **restauration remise**, journal terminal en `published` : `<op>` et
    `<op>.incomplete` sont classés par le protocole de remise (§4.7), **quel
    que soit leur type** : dossier, fichier, lien, jonction ou autre point
    d'analyse. Ils ne sont observés que par `Lstat` et, pour un dossier
    ordinaire, par leur identité, lue par un handle sans suivi aussitôt
    refermé ; jamais ouverts pour leur contenu, listés, épinglés, lus,
    suivis, déplacés ni réutilisés ; aucune de ces observations n'est une
    ambiguïté, et une entrée qu'on ne peut décrire sous ces noms est classée
    `not observable` au lieu d'arrêter la lecture ;
  - les noms d'opération sont comparés sans égard à la casse, comme Windows
    les résout.
- **Destination acceptée par `--apply`** : `<dest>` absent avec un parent
  existant, dossier ordinaire sans point d'analyse ; ou `<dest>` dossier
  ordinaire vide ; ou zone marquée du même jeu, sans opération non terminale
  ni ambiguïté. Zone du même jeu avec une opération en attente, une
  initialisation interrompue ou une ambiguïté : code 3, renvoi à `restore
  --recover`. Tout autre `<dest>`, dont un dossier non vide sans marqueur, un
  fichier, un lien, un coffre ou une zone d'un autre jeu : code 1, rien
  écrit, **jamais adopté**.
- **Distinctions (R12)**, sans lire de contenu :
  - `<dest>` n'est ni égal, ni intérieur, ni ancêtre d'aucun **chemin de
    référence** : le coffre, à la création seulement, l'installation et
    chaque source du manifeste, et les chemins des `sources` du plan,
    recopiés du receipt de snapshot, ceux réellement sauvegardés, que la
    récupération relit dans le journal de la zone sans le coffre ; par
    segments sans égard à la casse et **par
    identité quelle que soit la lettre de lecteur**, pour qu'un alias
    `subst` ou un point de montage ne contourne pas le contrôle, selon la
    règle d'observabilité ci-dessous ;
  - aucun ancêtre de `<dest>` ne contient `savewarden-vault.json` ni
    `savewarden-restore.json`, contrôle par nom sans suivi ;
  - aucun ancêtre n'est un lien ou un point d'analyse : un dossier redirigé
    ou synchronisé par un service de nuage qui en serait un est refusé ;
  - volume NTFS sur un lecteur local fixe ou amovible, comme le coffre.

  Échec : code 1, rien écrit.

**Règle d'observabilité (C3)**, la même à la création et à la récupération ;
le texte normatif est celui d'ADR 0006 §11. Principe : une identité
impossible à observer ne prouve jamais la séparation. Les chaînes
s'entendent **depuis la racine du volume, racine comprise**, et les
identités se comparent dans les deux sens. Sur une même lettre, la
comparaison des segments sans égard à la casse précède et refuse tout
recouvrement. Pour chaque chemin de référence `X`, la première ligne qui
s'applique décide :

| # | Cas de `X` | Contrôle | Issue |
| --- | --- | --- | --- |
| 1 | Chaîne existante non observable, quelle que soit la lettre : accès refusé, erreur de lecture, identité illisible, lien ou point d'analyse | Aucune preuve | **Refus**, code 1, rien écrit |
| 2 | Lettre sans volume : racine rapportée absente, lettre non attribuée, alias `subst` retiré, lecteur sans média | Aucune preuve : la lettre a pu désigner un alias ou un volume monté aujourd'hui sous une autre lettre (P0-1) | **Refus**, code 1, « separation from `X` cannot be proven » |
| 3 | `X` présent | Identité de `X` dans la chaîne de `<dest>` ; identité de `<dest>` dans la chaîne de `X` ; `<dest>` absent : seule la première recherche, sur la chaîne de son parent | Décisif |
| 4 | `X` absent, volume lisible : source retirée, installation déplacée ou désinstallée, lettre reprise par un autre disque | Chaîne existante jusqu'à l'ancêtre présent le plus profond : `X` ne peut contenir `<dest>` ; il y serait si l'identité de `<dest>` y figure. En outre, chemins comparés sous la racine de leur lettre, segments sans égard à la casse, contre une permutation de lecteurs amovibles : égaux ou imbriqués, refus | Décisif pour `X` tel qu'il se résout aujourd'hui |

- **Indépendance et dépendances de la récupération** : elle est
  indépendante du coffre, qu'elle n'ouvre jamais ; elle dépend du manifeste,
  exigé, et de ces contrôles de sûreté de la destination, refaits à chaque
  exécution qui peut écrire.
- **Limite** : les distinctions portent sur les chemins de référence tels
  qu'ils se résolvent aujourd'hui. Des données déplacées sans que ces chemins
  le reflètent ne sont protégées que si un cas décisif les atteint ; la
  documentation demande de tenir le manifeste à jour avant de restaurer.
- **Conséquence de la ligne 2**, question 7, acceptée : tant qu'une lettre de
  référence n'a pas de volume, disque du jeu perdu ou débranché, la
  restauration est refusée. **B4 ne garantit pas la restauration lorsque le
  disque d'origine est indisponible**, et ne se présente pas comme une reprise
  après sinistre complète. Rattacher un volume à cette lettre rend les
  identités observables sans prouver l'identité historique de l'ancien
  disque : la décision porte alors sur le chemin tel qu'il se résout. Un lot
  dédié réexaminera la restauration avec des références hors ligne.

**Initialisation de la zone**, sur le modèle d'I1 à I9 d'ADR 0006 §4, faite
uniquement de créations exclusives :

| # | Étape | État laissé par un arrêt juste après | Traitement au lancement suivant |
| --- | --- | --- | --- |
| Z1 | Préflight : destination acceptée, distinctions, espace | Rien | — |
| Z2 | Session ouverte sur `<dest>`, son parent épinglé ; `CreateDirectory(<dest>)` s'il manque ; s'il apparaît entre-temps, retour au préflight | `<dest>` vide | Destination vide, acceptée |
| Z3 | `<dest>` épinglé et **relu**, il doit être vide, sinon code 1, rien écrit ; puis temporaire du marqueur en `CREATE_NEW`, écrit, flushé, fermé | `<dest>` ne contient que des temporaires du marqueur | Initialisation interrompue : signalée par le plan, refusée par `--apply`, code 3 ; seul `--recover --apply` valide un nouveau marqueur, au `k` libre suivant, distinctions refaites, après la même relecture et **sans verrou** : aucun verrou n'est créé dans un dossier non marqué (P0-2) |
| Z4 | Validation par `MoveFileEx(temporaire, savewarden-restore.json, 0)` | Zone marquée, sans verrou | Normal |
| Z5 | `restore.lock` par `Lock` : créé vide s'il manque, ouvert sans partage | Verrou vide | Normal |
| Z6 | Sous verrou : zone relue, `destination-changed` code 1 si elle a changé depuis le plan, hors des observations des restaurations remises, qui ne la changent pas (P1-9) ; espace recontrôlé | Métadonnées seules | Code 1 ou 2, `destination: metadata only; nothing staged or published` |
| Z7 | `receipts`, puis `journal`, créés s'ils manquent | Structure, éventuellement partielle | Structure faite de dossiers vides : complétée au `--apply` suivant ; `receipts` absent à côté d'une opération journalisée : ambiguïté, code 3 |
| Z8 | `journal\<op>`, exclusif | Dossier d'opération vide, ou ne contenant que des temporaires `001-…` | Opération non commencée : bloque `--apply` ; `--recover --apply` valide `001-abandoned` |
| Z9 | Validation de `001-planned` | Opération commencée | Automate du §5.2 |

Refus sans aucune mutation, comme pour le coffre : temporaires du marqueur et
toute autre entrée, code 1 ; marqueur illisible ou de schéma inconnu, code 1,
jamais réécrit ; neuf temporaires du marqueur, enveloppe E1 épuisée, code 3.

**Fenêtre résiduelle de Z3.** La relecture épinglée réduit, sans la fermer, la
fenêtre où un autre programme ajouterait un fichier dans un `<dest>` vide
juste avant le marqueur ; `<dest>` épinglé ne peut plus être renommé ni
remplacé. Un ajout dans cette fenêtre laisse marqueur et verrou à côté de ce
fichier ; Z6 le voit, sort en code 1 et n'écrit rien de plus ; les exécutions
suivantes y voient une entrée imprévue de la zone, code 3. Z2 à Z4 ne
concernent qu'une zone nouvelle ; une zone marquée commence en Z5.

**Concurrence de la validation du marqueur (V1).** La reprise de Z3 ne prend
pas de verrou : deux `--recover --apply`, ou une reprise et une création,
peuvent viser la même `<dest>`. Chacun prend le plus petit `k` libre par
`CREATE_NEW`, qui ne peut être pris deux fois ; `MoveFileEx` sans drapeau ne
remplaçant jamais, **un seul marqueur est validé**. Le perdant ne réessaie
pas, n'écrit plus rien, relit `<dest>` et sort en code 1,
`destination-changed`, ligne d'état `metadata only; nothing staged or
published` ; son temporaire reste, essai abandonné accepté à côté du
marqueur. Un processus qui ne relit `<dest>` qu'après la validation y voit
une zone marquée sans objet d'opération. Neuf temporaires épuisent E1,
code 3. L'état laissé est toujours reconnu : temporaires seuls, ou zone
marquée avec des temporaires abandonnés. Le texte normatif est celui d'ADR
0006 §11.

### 4.3 Authentification du snapshot (R1, R2, R15)

Lecture **ciblée**, sans verrou du coffre, par une portée en lecture seule
sans suivi :

| # | Contrôle | Échec |
| --- | --- | --- |
| 1 | Coffre marqué ; `<vault>`, `games`, `games\<game-id>`, `snapshots`, `snapshots\journal` et `snapshots\journal\<snap-id>` dossiers ordinaires sans point d'analyse | Coffre invalide ou lien : code 1 ; dossier du journal absent : `snapshot <snap-id> not found`, code 1 |
| 2 | Journal relu strictement par `ParseSnapshotJournal`, automate de snapshot ; `sync.lock` n'est **ni ouvert, ni sondé, ni observé par `Lstat`**, et `games\<game-id>` n'est pas listé (X1) | Non terminal ou journal ambigu : `snapshot <snap-id> is not restorable: <état>`, code 1, le détail disant si l'opération est en attente ou le journal ambigu et renvoyant à `snapshot` et à `snapshot --recover` (X4). Terminal autre que `published` : `snapshot <snap-id> was not published`, code 1 |
| 3 | `snapshots\receipts` dossier ordinaire ; receipt `snapshots\receipts\<snap-id>.json`, fichier ordinaire, relu strictement par `CheckSnapshotReceipt` : SHA-256 égal à `receipt_sha256` de `published`, contenu égal au plan ; un receipt reconstruit par la récupération de B3, `published_at` nul, est valide | Dossier ou receipt absent, invalide ou différent : `snapshot-modified`, code 2 ; lien ou point d'analyse : code 1, comme partout dans la chaîne |
| 4 | `S` = `snapshots\<snap-id>` de l'identité `staging_identity` ; `snapshots\<snap-id>.incomplete` absent | Classés par `PublishedSnapshotCondition` : `snapshot-missing`, `snapshot-rename-lost` ou `snapshot-modified`, code 2 |
| 5 | Contenu de `S` inventorié et relu en entier contre les `sources` du receipt : chemins, tailles, dates, SHA-256, dossiers ; identité de `S` relue après le contenu | Différence : `snapshot-modified`, code 2 ; identité changée pendant la lecture : `snapshot-modified`, code 2 |

- **Illisible n'est pas altéré** (P2-4) : partage refusé, verrou, erreur de
  lecture autre que l'absence donnent `snapshot <snap-id> not verifiable:
  <détail>`, code 1, sans aucun code de dégradation. Limite : un volume non
  prêt que l'adaptateur de lecture rapporte absent donne encore
  `snapshot-missing` au préflight (N2, §0.3) ; c'est l'exception, écrite, à
  cette distinction.
- **Rien d'autre n'est lu** dans le coffre : ni `current`, ni `retained\`,
  ni `staging\`, ni les receipts et journaux du domaine de `current`, ni les
  autres snapshots, journaux, receipts ou `.incomplete`. Une anomalie
  ailleurs dans le coffre ne bloque pas la restauration d'un snapshot intact.
- La provenance n'est pas recoupée avec le domaine de `current` : le receipt
  de snapshot validé en tient lieu (ADR 0006 §10). Le receipt de restauration
  en recopie `source.*`.
- **Coffre copié (R15).** Après une copie du coffre vers un autre volume, les
  identités changent : chaque snapshot est classé `snapshot-modified`, la
  restauration est refusée, code 2, sans adoption d'identité. Le contenu n'en
  est pas lu, ADR 0006 §8 interdisant de lire un objet d'une identité
  inattendue ; le diagnostic dit que l'identité diffère de celle publiée,
  sans présumer une corruption, comme ADR 0006 §4 pour un coffre copié. Un disque
  externe déplacé vers un autre ordinateur conserve son numéro de série de
  volume et ses index de fichier NTFS : il n'est pas concerné. C'est une
  hypothèse sur NTFS, non testée par la CI, écrite comme telle dans la
  documentation.

### 4.4 Création : pipeline

| Phase | Écrit ? | Contenu | En cas d'échec |
| --- | --- | --- | --- |
| 1. Préflight | Non | Manifeste strict ; authentification complète (§4.3) ; destination acceptée et distinctions (§4.2) ; volume et espace | Code 1, 2 ou 3, rien écrit |
| 2. Zone | Zone | Z2 à Z5 | Avant le marqueur : au plus `<dest>` vide ; après : au plus `metadata only` |
| 3. Sous verrou | Structure | Z6 ; authentification refaite sans relire le contenu : journal, receipt, identité de `S` ; identifiant tiré et contrôlé ; Z7, Z8 | Code 1 ou 2, au plus `metadata only` |
| 4. `planned` | Journal | Plan : snapshot, SHA-256 de son receipt, identité de `S`, provenance recopiée, `sources` du receipt de snapshot | — |
| 5. Copie | `I` | `staging-intent` ; création exclusive de `I` ; copie hachée de chaque fichier de `S` contre le plan, date conservée, chaque fichier flushé ; identité de `S` relue après la copie | Contenu différent : `changed-during-restore`, `failed`, code 2 ; identité de `S` changée : `changed-during-restore`, détail « snapshot replaced », `failed`, code 2 (X2) ; fichier de `S` absent : racine et marqueur du coffre contrôlés de nouveau, coffre disparu ou illisible : `not verifiable`, `failed`, code 1, coffre présent : `changed-during-restore`, code 2 ; autre erreur de lecture : `failed`, code 1 ; `I` conservé dans tous les cas |
| 6. Vérification | Journal | `staged` ; relecture de `I` depuis le disque contre le plan ; identité de `I` relue après | Contenu : `verification-mismatch`, `failed`, code 2 ; identité de `I` changée : code 3, rien enregistré de plus ; `I` conservé |
| 7. Publication | Renommage, receipt | `verified` ; `publish-intent` ; bascule `I` → `P` selon ADR 0006 §4 ; receipt de restauration ; `published` | Bascule refusée et confirmée non faite : `rolled-back`, code 1 ; toute autre issue : récupération |

- **Pas de porte de provenance après `planned` (R9).** Le plan provient du
  snapshot authentifié en phases 1 et 3 ; la copie est hachée contre lui, et
  sa relecture lui est comparée. Le coffre n'est plus consulté après la
  copie : un snapshot altéré ensuite ne change rien à ce qui est publié.
- **Une altération du coffre pendant la copie** n'est jamais une ambiguïté
  de la zone : la zone reste cohérente, l'opération se clôt en `failed`
  (X2).
- **Rapport impossible à écrire** après une publication : code 1, avec la
  ligne `destination: published restore <op>` ; le receipt fait foi, comme
  pour `sync` et `snapshot`.
- **Avant la ligne `published restore <op>`**, `<dest>\<op>` ne doit pas être
  utilisé : entre A3 et A6, la récupération peut encore le remettre en `I`,
  ou bloquer la zone s'il a été touché. La documentation le dit.
- **Espace :** taille complète du snapshot plus max(64 Mio, 5 %), disponible
  pour l'utilisateur sur le volume de la zone, au préflight puis sous
  verrou avant toute structure ou journal.
- **Chemins longs :** `long-path`, information, mesuré sur `I`,
  `<dest>\<op>.incomplete\<source-id>\…`.
- **Dates et attributs :** la date de modification de chaque fichier est
  celle du receipt ; aucun attribut ni ACL n'est reproduit, comme pour les
  snapshots ; les fichiers créés héritent des droits de la zone.

### 4.5 Récupération : préconditions (R5)

La récupération est **indépendante du coffre**, jamais ouvert, lu ni nommé,
et **dépendante du manifeste** et des contrôles de sûreté de la destination.
Ordre de décision (P0-2, P1-6) :

1. `<dest>` est classé par le tableau « destination sans opération
   journalisée » ci-dessous ; un état sans rien à récupérer conclut aussitôt,
   code 0, **sans ouvrir de session ni rien écrire**, pas même un
   `restore.lock` absent ;
2. pour tout état où la récupération peut écrire : manifeste strict, marqueur
   du même jeu, distinctions refaites selon la règle d'observabilité du §4.2,
   chemins des `sources` du plan compris, volume recontrôlé ; échec : code 1,
   avant toute écriture ;
3. grammaire de la zone, journaux et receipts relus strictement, observation
   de `P` et `I` de l'opération récupérée ; avec `--apply`, le verrou de la
   zone, pris seulement dans une zone marquée ; puis la ligne du §5.5.

Le contenu de l'installation et des sources n'est jamais lu ; seules leurs
identités servent.

- Une zone déplacée sur le même volume garde ses identités et ses noms
  relatifs : elle peut avoir été glissée sous un coffre ou une source. Les
  distinctions refaites l'interdisent ; échec : code 1, rien écrit.
- `<dest>` ne contenant que des temporaires du marqueur : seul cas où la
  récupération marque un dossier ; `<dest>` est épinglé et relu juste avant,
  comme en Z3, et aucun verrou n'est pris avant que le marqueur soit validé.
- **Destination sans opération journalisée (C2)** : « rien à récupérer »
  n'est conclu que d'une zone sans aucun objet d'opération.

  | `<dest>` observé | Issue |
  | --- | --- |
  | Absent, son parent présent ou non, ou dossier ordinaire vide | Rien à récupérer, code 0, sans session |
  | Fichier, lien, jonction ou autre point d'analyse | Pas une zone : code 1 |
  | Ne contenant que des temporaires du marqueur | Initialisation interrompue : reprise par `--recover --apply` (Z3), sans verrou |
  | Non vide, sans marqueur, autre chose que ces temporaires | Pas une zone : code 1, jamais adopté |
  | Zone marquée dont les seules entrées sont le marqueur, ses temporaires, `restore.lock`, et `receipts` et `journal` vides ou absents | Structure seule : rien à récupérer, code 0 |
  | Zone marquée portant un `<op>`, un `<op>.incomplete`, un receipt ou un temporaire de receipt, sans dossier `journal\<op>` qui l'explique | Objet d'opération inexpliqué : ambiguïté, code 3, `journal-ambiguous`, rien écrit |
  | Zone marquée avec des opérations journalisées sans `receipts` | Structure partielle : ambiguïté, code 3, comme Z7 |
  | Zone marquée portant toute autre entrée hors de la grammaire, dont un fichier ajouté dans la fenêtre résiduelle de Z3 | `unexpected-entry`, code 3, rien écrit |
- La récupération ne revérifie `P` ou `I` que contre le plan journalisé dans
  la zone.
- Le plan, avec ou sans `--recover`, sonde `restore.lock` en lecture seule
  avant de déclarer une opération de la zone en attente : verrou tenu,
  code 1, « another restore is running in this area ». Ce sondage ne touche
  que la zone, jamais le coffre.

### 4.6 Receipt de restauration v1 (R11)

Fichier `<dest>\receipts\<op>.json`, validé par temporaire et renommage,
enveloppe E1 propre, JSON canonique, relecture stricte : champ inconnu,
manquant ou en double refusé. Schéma complet dans ADR 0009.

| Champ | Contenu |
| --- | --- |
| `schema`, `schema_version` | `savewarden.restore` et `1` |
| `restore` | `<op>` |
| `tool_version`, `recovered_by` | Comme le receipt v1 |
| `game` | Recopié du plan |
| `started_at`, `publish_intent_at`, `published_at`, `recovered_at` | Comme le receipt v1, mêmes règles d'ordre et de reconstruction |
| `snapshot.id`, `snapshot.receipt_sha256` | Snapshot restauré et SHA-256 des octets de son receipt |
| `source.operation`, `source.receipt_sha256` | Recopiés du receipt de snapshot : génération d'origine |
| `plan_sha256` | Empreinte du plan journalisé |
| `sources` | Celles du receipt de snapshot, en forme canonique |

La chaîne restauration → snapshot → génération se lit ainsi sans le coffre.

### 4.7 Restaurations successives et remise (R7, R8)

- Chaque `--apply` publie une **nouvelle** restauration sous un nouvel
  `<op>` ; rien n'est jamais remplacé. Le plan signale, en information
  `snapshot-already-restored`, les restaurations publiées du même snapshot
  dans la zone.
- Une restauration est **publiée** quand son journal se termine par
  `published`, ce qui suppose son receipt validé ; un receipt absent ensuite,
  même perdu par une coupure, est une ambiguïté de la zone, code 3, qui la
  ferme aux nouvelles restaurations, la copie restant en place (question 8).
  Elle est alors **remise** :
  Savewarden ne la rouvre jamais en écriture, ne la déplace ni ne la
  supprime, et ne la vérifie plus. Son receipt atteste l'état à la
  publication, pas l'état présent.
- Le plan liste les restaurations publiées de la zone et dit, sans lire leur
  contenu, si `P` est présent sous l'identité publiée. Un écart est une
  information, jamais bloquante, jamais réparée ; le contenu n'étant pas lu,
  une modification à l'intérieur de `P` n'est pas signalée.
- **Protocole de remise (C1).** `P` et `I` d'une restauration remise sont
  observés par `Lstat` sans suivi ; l'identité n'est relevée que pour un
  dossier ordinaire sans point d'analyse. Un fichier, un lien, une jonction
  ou un autre point d'analyse sous l'un de ces noms est classé, jamais
  ouvert, lu, suivi, déplacé ni réutilisé ; Savewarden n'y écrit jamais.

  | Observation de `P` | Observation de `I` | Information |
  | --- | --- | --- |
  | Dossier ordinaire de l'identité publiée | Absent | Aucune : `present` |
  | Absent | Absent, ou objet observé autre qu'un dossier de l'identité publiée | `restore-missing` |
  | Absent | Type ou identité illisible | Aucune conclusion : `not observable` |
  | Absent | Dossier ordinaire de l'identité publiée : renommage perdu | `restore-rename-lost` ; `I` n'est jamais renommé |
  | Dossier d'une autre identité, fichier, lien, jonction ou point d'analyse | Quelconque | `restore-replaced` |
  | Dossier ordinaire de l'identité publiée | Tout objet présent | `restore-replaced` |
  | Identité ou type illisible | Quelconque | Aucune conclusion : `not observable` dans la ligne de la restauration, aucune information de dégradation |

  Le receipt d'une restauration publiée n'est pas remis : absent, invalide ou
  d'une autre empreinte que `receipt_sha256`, c'est une ambiguïté de la zone,
  code 3.
- La documentation demande de **copier** hors de la zone ce dont on a besoin,
  sans renommer ni ajouter d'entrée dans la zone : un `P` renommé devient une
  entrée inexpliquée, code 3 pour toute nouvelle opération de cette zone,
  jamais pour le coffre. Une nouvelle zone reste toujours possible.

### 4.8 Ce que `restore` ne touche jamais

Le coffre, en écriture comme en déplacement, `sync.lock` compris ; dans le
coffre, tout ce qui n'est pas le snapshot désigné, en lecture ; l'installation
et les sources, dont seules les identités servent aux distinctions ; un
dossier non marqué autre que `<dest>` absent ou vide ; une restauration
publiée ; un `.incomplete` d'une autre opération ; un coffre ou une zone
imbriqués.

### 4.9 Sortie et codes (ADR 0009)

Codes de sortie : même sens qu'ADR 0007. Avec `--apply` ou `--recover`, la
dernière ligne de stderr est la **ligne d'état de la destination**, préfixée
`savewarden: destination: ` : `not modified` ; `metadata only; nothing staged
or published` ; `restore <op> kept incomplete` ; `published restore <op>` ;
`rolled back restore <op>` ; `intervention required for <op>: <cause>`. Le
coffre n'étant jamais écrit, aucune ligne d'état du coffre n'est émise. Tout
code 3 nomme sa cause, et il ne naît que de la zone (X4).

Plan, forme indicative avec des identifiants fictifs :

```text
game: synthetic-game
vault: F:\Vault
snapshot: 20261102T090000-4e5f6a7b, from 20261101T120000-55667788, verified
destination: G:\Restores\synthetic, restore area, 1 published, 0 incomplete
  20261103T100000-9a8b7c6d  of 20261102T090000-4e5f6a7b  present
restore: new, to G:\Restores\synthetic\<restore-id>
files: 2  bytes: 6  space needed: 67108870 bytes
findings: 1
  info  snapshot-already-restored  20261103T100000-9a8b7c6d  this snapshot was already restored in this area
```

| Code | Résultat |
| --- | --- |
| `0` | Plan sans anomalie bloquante ; restauration publiée et vérifiée ; récupération achevée ou sans objet |
| `1` | Invocation, manifeste, coffre ou destination invalides ; snapshot introuvable, non publié, non vérifiable ou non restaurable (X4) ; recouvrement ou séparation non prouvée ; zone d'un autre jeu ; destination non vide sans marqueur ; verrou de la zone occupé ; `destination-changed` ; identifiant déjà employé ; erreur de lecture ; interruption ; bascule refusée puis annulée ; rapport impossible à écrire |
| `2` | Anomalie bloquante, rien publié : snapshot dégradé, espace insuffisant ; avec `--apply` : `changed-during-restore`, `verification-mismatch` |
| `3` | Intervention requise **dans la zone** : opération en attente, initialisation interrompue, entrée imprévue, journal ou receipt ambigus |

| Code stable | Sévérité | Sens |
| --- | --- | --- |
| `snapshot-missing`, `snapshot-rename-lost`, `snapshot-modified` | **bloquante** pour `restore` | Classement d'ADR 0008, appliqué au snapshot désigné ; information pour `snapshot`, inchangé |
| `insufficient-space` | bloquante | Volume de la zone |
| `changed-during-restore` | bloquante, `--apply` | Snapshot différent du plan, ou remplacé, pendant la copie |
| `verification-mismatch` | bloquante, `--apply` | Repris de `sync` |
| `long-path` | information | Mesuré sur `I` |
| `snapshot-already-restored` | information | Restauration publiée du même snapshot dans la zone |
| `restore-missing`, `restore-rename-lost`, `restore-replaced` | information | Restauration remise observée sans son nom, restée sous `I`, ou remplacée par un autre objet (§4.7) |

Causes d'intervention : `pending-operation`, `initialization-interrupted`,
`unexpected-entry`, `journal-ambiguous`. Diagnostics de code 1 :
`destination-changed` ; « another restore is running in this area » ;
`snapshot <snap-id> not found`, `was not published`, `not verifiable`, `is
not restorable` ; « separation from `X` cannot be proven ». Une copie
revérifiée différente du plan en récupération donne `rolled back restore
<op>`, code 2, `verification-mismatch`. Pendant A3 à A6, le plan annonce
« operation `<op>` pending recovery: copy published, receipt missing; do not
use `<dest>\<op>` yet ». ADR 0009 fait foi pour ces tableaux.

### 4.10 Compatibilité

- Le coffre n'étant jamais écrit, **`sync` et `snapshot` sont inchangés**,
  comportement et sortie, et un binaire B3 ou antérieur relit un coffre après
  une restauration exactement comme avant.
- Une zone n'est lue par aucune autre commande. `inspect` peut lire une
  restauration publiée avec un manifeste dont la source pointe sur
  `<dest>\<op>\<source-id>`.

---

## 5. Protocole proposé : ADR 0006 §11

Résumé ; le texte normatif est celui d'ADR 0006 §11, accepté avec cette
v0.2, qui prévaut en cas d'écart.

### 5.1 Notation

`S` : `snapshots\<snap-id>` dans le coffre, lu seulement ; `P` : `<dest>\<op>` ;
`I` : `<dest>\<op>.incomplete`. `new` est l'identité de `I` relevée dans
`staged` ; `—` un nom absent ; toute autre identité, `other`, est une
ambiguïté.

### 5.2 Automate et contrat des champs

Journal `<dest>\journal\<op>\`, même format d'enregistrement que le §3,
mêmes noms d'étapes que la première publication, **mode restauration**
distinct : un journal de snapshot n'est jamais lu comme un journal de
restauration, ni l'inverse.

| # | Enregistrement validé | Champs (`fields`) | Action qui suit |
| --- | --- | --- | --- |
| 1 | `planned` | `snapshot` ; `snapshot_receipt_sha256` ; `snapshot_identity`, identité de `S` ; `plan` : objet strict, version de l'outil, jeu, `started_at`, `source` recopiée du receipt de snapshot, `sources` ; `plan_sha256` | — |
| 2 | `staging-intent` | `staging` : `<op>.incomplete` | Créer `I`, copier, flusher |
| 3 | `staged` | `staging_identity`, `copied_sha256` | Relecture de `I` |
| 4 | `verified` | `verified_sha256` | — |
| 5 | `publish-intent` | `staging_identity` ; `P` absent attendu | Bascule `I` → `P` ; receipt de restauration |
| 6 | `published` | `receipt_sha256` | Terminal |

Cohérence à la lecture : `plan_sha256` est le SHA-256 de `plan` ;
`snapshot` et `plan.source.operation` au format du §3, distincts entre eux et
de l'opération ; jeu du plan égal à celui du marqueur ; `copied_sha256` et `verified_sha256` égaux à l'empreinte
de contenu de `plan.sources`. Rollback : `unpublish-intent`
(`staging_identity`), seul déplacement `P` → `I`, puis `rolled-back` ;
`rollback-failed` (`error`) au plus une fois par intention. Terminaux :
`published`, `failed`, `abandoned`, `rolled-back`. Les étapes et champs de
rotation et de snapshot sont refusés dans ce mode.

### 5.3 Bascule et points d'arrêt

- Ordre d'ADR 0006 §4 : fermer toute portée ouverte sur `I` et `P` ; ne
  garder épinglés que les ancêtres de `<dest>`, `<dest>` et
  `journal\<op>` ; pré-contrôle de `I`, nom `P` absent ; `MoveFileEx` sans
  drapeau, retries du §7 ; post-contrôle : `P` de l'identité attendue, `I`
  absent. La portée de lecture du coffre est fermée dès la fin de la copie.
- Points d'arrêt A1 à A8 de la première publication, `P` et `I` à la place
  de `current` et `staging\<op>`. En A3 et A4, `P` est revérifié en entier
  contre le plan, sans lire le coffre. En A5 et A6, un receipt de
  restauration validé interdit tout retour en arrière.

### 5.4 Initialisation

Z1 à Z9 du §4.2, protocole d'I11 appliqué à la zone : créations exclusives,
marqueur validé par renommage, chaque état partiel reconnu ou refusé sans
mutation, jamais adopté implicitement.

### 5.5 Récupération `--recover --apply`

Préconditions du §4.5 ; aucune lecture du coffre. Un receipt de restauration
validé n'admet que deux lignes, A5 ou A6 et `published` ; toute autre
combinaison, et tout receipt sans journal ou d'une opération close autrement
qu'en `published`, est une ambiguïté, code 3.

| Dernier validé | Observation (`P`, `I`) | Action |
| --- | --- | --- |
| Aucun : dossier d'opération vide ou temporaires `001` seuls | (—, —) | `001-abandoned` ; sinon code 3, rien écrit |
| `planned` | (—, —) | `abandoned` |
| `staging-intent` | (—, —), ou (—, dossier ordinaire sans point d'analyse) | `abandoned` ; `I` laissé en place, ni lu ni déplacé |
| `staged`, `verified` | (—, `new`) | `abandoned` ; `I` conservé |
| `publish-intent`, aucun receipt | (—, `new`) | Renommage non effectué : `rolled-back` |
| `publish-intent`, aucun receipt | (`new`, —) | Renommage effectué : revérification intégrale de `P` contre le plan ; égal : receipt, puis `published` ; différent : `unpublish-intent`, puis `P` → `I`, `rolled-back`, code 2, `verification-mismatch` |
| `publish-intent`, receipt validé | (`new`, —) | A5, A6 : receipt relu strictement, égal au plan, `P` revérifié : `published` ; sinon code 3, rien écrit ni déplacé |
| `unpublish-intent`, aucun receipt | (`new`, —) | L'exécuter, puis `rolled-back` ; échec : `rollback-failed`, code 3 |
| `unpublish-intent` ou `rollback-failed`, aucun receipt | (—, `new`) | `rolled-back` |
| `rollback-failed`, aucun receipt | (`new`, —) | Nouvel essai ; succès : `rolled-back` ; échec : code 3, sans nouvel enregistrement (E3) |
| `published` | Tout objet de tout type sous `P` et sous `I` | Rien : restauration remise, protocole de remise du §4.7, sans suivi ni lecture ; `restore-missing`, `restore-rename-lost` ou `restore-replaced` en information ; un receipt absent, invalide ou d'empreinte différente de `receipt_sha256` reste une ambiguïté, code 3, puisque le receipt n'est pas remis |
| `failed`, `abandoned`, `rolled-back` | `P` absent ; `I` absent, ou dossier ordinaire sans point d'analyse, de l'identité `new` si `staged` l'a fixée | Rien |
| `failed`, `abandoned`, `rolled-back` | `P` présent, de tout type ; ou `I` d'un autre type ou d'une autre identité | Ambiguïté, code 3, rien écrit |
| Aucun journal pour l'objet | `P`, `I` ou receipt présents | Ambiguïté, code 3, rien écrit (C2, §4.5) |
| Tout autre cas, dont `P` et `I` présents ensemble avant `published`, une identité `other`, un receipt validé sans bascule observée | — | Fail-closed, code 3, rien écrit |

Initialisation interrompue (Z3) : `<dest>` ne contenant que des temporaires
du marqueur, distinctions refaites, `<dest>` épinglé et relu : un nouveau
marqueur est validé au `k` libre suivant, temporaires conservés ; toute autre
entrée, code 1, rien écrit.

Propriétés du §9 : explicite, idempotente, interruptible, convergente et
bornée dans l'enveloppe, monotone, non aggravante.

### 5.6 Enveloppe

E1, E3 et E4 inchangées ; E1 vaut pour chaque numéro d'enregistrement, chaque
receipt de restauration et le marqueur de zone. E2 : au plus 11
enregistrements ; le plus long chemin en compte 8, comme un snapshot.

### 5.7 Exclusivité et domaines (R4, R6)

- **Verrou de zone** `restore.lock`, pris par la même primitive `Lock` que
  `sync.lock`. **I8 par zone** : deux opérations non terminales dans une zone
  sont une ambiguïté, code 3 ; une opération non terminale bloque toute
  nouvelle restauration dans cette zone.
- **Aucun verrou du coffre (X1).** `restore` lit un snapshot publié, que
  nulle opération de Savewarden ne déplace ni n'écrit plus : il peut
  s'exécuter pendant un `sync` ou un `snapshot` du même jeu. Constats du
  code : la classification d'une opération de snapshot terminale ne déplace
  rien ; les portées de lecture et les sessions d'écriture partagent la
  lecture et l'écriture, sans la suppression, et ne se bloquent donc pas ;
  une épingle sur un parent n'empêche pas le renommage d'un enfant
  (`TestParentPinsDoNotBlockChildRename`) ; `restore` n'ouvre jamais
  `sync.lock`. Un test Windows réel le confirme, le faux ne pouvant prouver
  les modes de partage (§9).
- **Invariant**, écrit dans ADR 0006 §11 : aucune opération de Savewarden ne
  renomme un snapshot publié ni l'un de ses ancêtres, ni ne les ouvre sans
  partage en lecture. Une opération future qui le lèverait rouvre X1 et
  l'extraction du moteur (§3.1).
- **Fenêtre résiduelle :** un autre programme qui modifie `S` pendant la
  copie est détecté par le hachage contre le plan, `changed-during-restore` ;
  après la copie, il est sans effet sur la restauration.

### 5.8 Ce qu'une restauration ne fait jamais

Écrire, renommer, verrouiller ou supprimer quoi que ce soit dans le coffre ;
ouvrir ou sonder `sync.lock` ; lire dans le coffre autre chose que le
snapshot désigné ; lire le coffre
pendant une récupération ; écrire hors de la zone, hormis la création de
`<dest>` absent ; écrire dans un dossier non marqué, hormis Z2 à Z4 ;
remplacer une cible ; copier vers un nom publié ; déplacer `P` après
`published` ; supprimer un temporaire ou un `I` ; restaurer un snapshot non
publié, dégradé ou non vérifiable ; restaurer dans l'installation, une
source, un coffre ou une autre zone.

---

## 6. Non-écrasement et limites

La garantie de non-écrasement repose sur :

1. une destination absente, vide ou marquée, jamais adoptée sinon (§4.2) ;
2. `NewFile` en `CREATE_NEW` et `Move` sans remplacement, imposés par le garde
   d'ADR 0005, inchangé ;
3. la zone confinée par les distinctions et le contrôle des ancêtres ;
4. aucun chemin de code ne vise le coffre en écriture : `Vaults.Open` n'y est
   jamais appelé, propriété testée et prouvée par un binaire sous ACL de
   refus d'écriture sur le coffre.

Limites, écrites dans la documentation :

- **Remise sans suivi** : après sa publication, une restauration peut être
  modifiée par l'utilisateur ou un autre programme ; Savewarden ne le détecte
  pas et n'y prétend pas.
- **Authenticité sans clé** : un snapshot réécrit de façon délibérée et
  coordonnée avec son receipt et son journal serait restauré fidèlement ;
  comme la porte de provenance de B3, la vérification refuse une altération
  accidentelle ou partielle, pas une falsification cohérente.
- **Fichiers ouverts** dans `I` par un antivirus ou l'Explorateur : bascule
  refusée en erreur 5, rollback, code 1.
- **Coffre copié** vers un autre volume : refus (R15).

---

## 7. Périmètre des fichiers

| Zone | À créer | À modifier |
| --- | --- | --- |
| Plan et ADR | `docs/adr/0009-restore-contract-receipt-v1.md` | Ce plan, version validée ; ADR 0004 et 0006, amendements |
| `internal/entities/backup` | Receipt de restauration v1, marqueur de zone v1, leurs relectures strictes et tests | Mode restauration de l'automate et contrat de ses champs ; classement des observations du §5.5 |
| `internal/features/sync` | `restore.go`, `restore_apply.go`, `restore_recover.go` et leurs tests | Couture du moteur : source de lecture et cible d'écriture ; lecteur ciblé d'un snapshot ; recouvrement par identité quelle que soit la lettre ; documentation du package et commentaires de `ports.go` ; faux coffre, qui consigne les chemins ouverts |
| `internal/platform/vault` | — | Commentaires seulement : une session s'ouvre sur un coffre ou sur une zone ; aucune ligne de code |
| `internal/cli` | `restore.go` et tests | `root.go`, texte de la commande racine et câblage |
| `internal/app` | — | `app.go` |
| `cmd/savewarden` | Tests binaires Windows de la restauration | `main_test.go`, exemples README |
| `docs/` | `commands/restore.md` ; page du receipt de restauration et du marqueur de zone ; `validation/b4.md` | `vault.md`, renvoi à la zone ; `safety-model.md` ; `architecture.md` ; `pilots/odyssey.md` |
| Racine | — | `README.md`, `CHANGELOG.md`, `AGENTS.md` (lot courant) |

Changements interdits :

- toute modification du code de `internal/platform/vault`, ses commentaires
  exceptés, de `internal/platform/filesystem`, de
  `internal/app/readonly_test.go`, de
  `internal/app/architecture_test.go`, d'ADR 0005, 0007 et 0008, de `go.mod`
  et `go.sum` ;
- toute API d'écriture, de suppression ou de remplacement nouvelle ; toute
  écriture, tout verrou ou toute session d'écriture sur le coffre ;
- toute sévérité nouvelle dans `entities/backup` ;
- les packages `features/restore`, `doctor`, et tout dossier vide, sauf
  décision (c) du §3.1 ;
- toute modification du manifeste v1, du receipt v1, du receipt de snapshot
  v1, du marqueur du coffre et du format d'enregistrement du journal ;
- tout changement de comportement ou de sortie de `sync` et de `snapshot` ;
- tout chemin personnel ou propre au pilote dans le code, les tests ou les
  exemples ;
- toute modification des rulesets, des versions d'Actions ou de la CI hors
  ajout de contrôles ;
- tout accès agent à `D:\Ga`, `D:\Savewarden`, `E:\GMS` ou aux clés SSH ;
- tout test lisant ou écrivant hors de `t.TempDir()`.

---

## 8. Plan d'exécution

Avant tout code : cette v0.2 validée et ses ADR acceptés par le propriétaire
sur `docs/b4-framing`, fait le 3 novembre 2026, puis un GO de développement
distinct, non donné. La PR `feat/b4-restore` part de ces commits de
cadrage, comme celles de B2c et de B3.

| # | Commit | Point de contrôle |
| --- | --- | --- |
| 0 | Plan validé ; ADR 0006 et 0004 amendés ; ADR 0009 ; statuts passés à « accepté » : commits de cadrage de `docs/b4-framing`, jusqu'à l'acceptation du 3 novembre 2026 | **Revue du cadrage** par le propriétaire, faite |
| 1 | `entities/backup` : receipt de restauration, marqueur de zone, mode restauration de l'automate et ses champs, classement, tests purs | — |
| 2 | Couture du moteur de `features/sync` : source de lecture et cible d'écriture explicites, `operation` sans le `Plan` de `sync`, sans changement de comportement : suites B2 et B3 inchangées, sorties de `sync` et `snapshot` identiques octet pour octet | **Revue indépendante de non-régression, obligatoire** avant le commit 3 : aucun usage de la racine de session par `o.plan.vault*` |
| 3 | Lecteur ciblé d'un snapshot ; lecture de la zone ; plan de restauration en lecture seule ; CLI `restore` sans `--apply` | — |
| 4 | `--apply` : zone Z1 à Z9, automate, copie, vérification, bascule, receipt ; énumération des crashs | — |
| 5 | `--recover`, indépendant du coffre ; énumération des crashs pendant la récupération | **Revue indépendante du protocole, obligatoire** avant le commit 6 |
| 6 | Tests Windows réels et binaires ; documentation | — |
| 7 | Rapport `docs/validation/b4.md` | **Revue de sécurité indépendante**, avant le GO de fusion |

**Erratum proposé, 3 novembre 2026, en attente de ratification par le
propriétaire**, avec celui de l'amendement B4 d'ADR 0004, qui le détaille :

- au §3.1, option (a), « `operation` cessant d'embarquer le `Plan` de
  `sync` », et au commit 2 ci-dessus, « `operation` sans le `Plan` de
  `sync` », se lisent : le moteur ne tire plus aucune racine du plan ; la
  cible d'écriture et la source de lecture sont construites à l'ouverture
  depuis le chemin du coffre et l'identifiant du jeu ; seule l'opération de
  `sync` porte son plan, pour ses propres phases, que le moteur, les
  opérations de snapshot et de restauration et les récupérations ne lisent ni
  ne renseignent, ce que vérifie `TestOnlySyncReadsItsPlan` ;
- la couture tient dans le commit 2 et dans les commits qui corrigent ses
  revues, tous avant le commit 3 ;
- « suites B2 et B3 inchangées » vaut pour leurs assertions : leur faux
  coffre partagé est devenu plus strict, refusant les noms que refuse la
  vraie session.

Le point de contrôle est inchangé : plus aucun usage de `o.plan.vault*`.

Chaque commit passe la suite complète à son SHA, dans un worktree propre.
Push, PR, fusion et pilote attendent chacun un GO distinct du propriétaire.

---

## 9. Matrice de tests

Noms proposés, fixés au commit de chaque partie. Faux coffre sur les trois OS
sauf mention.

**Entités**

| Test | Preuve |
| --- | --- |
| `TestRestoreReceiptRoundTrip`, `TestRestoreReceiptIsStrict` | Forme canonique ; champs inconnus, manquants, en double, dates, reconstruction refusés |
| `TestRestoreAreaMarkerIsStrict` | Schéma, version, jeu ; marqueur du coffre refusé comme marqueur de zone, et l'inverse |
| `TestRestoreStepFieldsAreStrict`, `TestRestoreAutomaton` | Contrat de chaque étape ; transitions permises seulement ; étapes et champs de rotation et de snapshot refusés ; journal de snapshot refusé en mode restauration, et l'inverse |
| `TestRestorePlanIsCoherent` | `plan_sha256`, `copied_sha256`, `verified_sha256` et empreinte de contenu recoupés à la lecture |
| `TestRestoreRecoveryRows` (pur) | Chaque ligne du §5.5, A5, A6 et défaut fail-closed compris |

**Non-régression**

| Test | Preuve |
| --- | --- |
| Suites B2 et B3 complètes, inchangées | Moteur paramétré sans changement de comportement, énumérations de crash comprises |
| `TestSyncAndSnapshotOutputsUnchanged` | Sorties identiques octet pour octet à celles de `f3074d0` sur les scénarios différentiels de B3 |
| `TestRestoreNeverOpensTheVaultForWriting` | Trace du faux : `Vaults.Open` jamais appelé sur le coffre ; empreinte complète du coffre identique dans chaque mode, crash et récupération compris |
| `TestRestoreRunsBesideSyncAndSnapshot` | `sync.lock` tenu, opération de `current` ou de snapshot en attente : la restauration d'un snapshot intact aboutit, sans toucher ni bloquer l'autre opération |

**Authentification**

| Test | Preuve |
| --- | --- |
| `TestRestoreRequiresAPublishedSnapshot` | Inconnu, terminal non publié : code 1 ; non terminal, que `sync.lock` soit tenu ou non, et journal ambigu : code 1, `not restorable`, renvoi à `snapshot --recover` ; `sync.lock` jamais ouvert ; rien écrit |
| `TestRestoreAcceptsARebuiltSnapshotReceipt` | Receipt de snapshot reconstruit par la récupération, `published_at` nul : restauration possible |
| `TestRestoreRefusesADegradedSnapshot` | `snapshot-missing`, `snapshot-rename-lost`, `snapshot-modified` par identité, receipt absent, invalide ou d'empreinte différente, `.incomplete` à côté, contenu différent : code 2, rien écrit |
| `TestUnreadableSnapshotIsNotDegraded` | Partage refusé, erreur de lecture : code 1, `not verifiable`, aucun code de dégradation |
| `TestRestoreReadsOnlyTheSelectedSnapshot` | Trace des lectures : rien sous `current`, `retained\`, `staging\`, `receipts\`, `journal\`, ni d'un autre snapshot ; `games\<game-id>` jamais listé ; `sync.lock` jamais observé, même par `Lstat` |
| `TestRestoreIgnoresOtherVaultAnomalies` | `current` absent ou modifié, journal de `current` ambigu, autre snapshot dégradé, entrée imprévue ailleurs : restauration d'un snapshot intact inchangée |

**Destination**

| Test | Preuve |
| --- | --- |
| `TestRestoreDestinationIsNewEmptyOrArea` | Dossier non vide sans marqueur, fichier, lien, jonction, coffre, zone d'un autre jeu : code 1, rien écrit |
| `TestRestoreDistinction` | Zone dans ou autour du coffre, de l'installation, d'une source, par nom et par identité ; ancêtre marqué coffre ou zone : code 1 |
| `TestRestoreDistinctionAcrossDriveLetters` | Faux à deux lettres désignant les mêmes nœuds, comme un alias `subst` : zone dans une source ou dans le coffre refusée, code 1 |
| `TestRestoreDistinctionWithUnavailableReferences` | Chaque ligne de la règle d'observabilité, à la création et à la récupération : chaîne non observable ou à point d'analyse, et lettre sans volume : code 1, rien écrit ; `X` présent : décision dans les deux sens, racine comprise ; source retirée, installation déplacée ou désinstallée, lettre reprise par un autre disque : décision par la chaîne existante et par les noms sous la racine |
| `TestReferencePathsIncludeTheSnapshotSources` | Manifeste dont les sources diffèrent des chemins sauvegardés : zone dans un chemin du receipt de snapshot refusée, à la création comme à la récupération, ce chemin relu dans le journal de la zone sans le coffre |
| `TestRestoreDestinationFilledBeforeMarking` | Fichier ajouté dans un `<dest>` vide entre le préflight et Z3 : code 1, aucun marqueur écrit |
| `TestRestoreToAnotherVolume` | Coffre et zone sur deux volumes du faux : restauration publiée, rien écrit sur le volume du coffre |
| `TestRestoreAreaInitializationIsResumable` | Crash à chaque étape Z2 à Z9, et pendant la reprise de Z3 elle-même : état reconnu ; temporaires du marqueur seuls : `--recover --apply` les reprend sans verrou, distinctions refaites ; `restore.lock` n'apparaît jamais dans un dossier non marqué ; structure complétée |
| `TestNothingToRecoverOpensNoSession` | `<dest>` absent, vide, ou zone marquée de structure seule, même sans `restore.lock` : code 0, aucun `Vaults.Open`, rien écrit |
| `TestMarkerValidationRaceHasOneWinner` (V1) | Faux déterministe : marqueur validé par un autre processus entre le temporaire et le renommage, et `k` pris entre le choix et la création, en reprise comme en création : un seul marqueur ; perdant en code 1, `destination-changed`, sans autre écriture ; temporaire abandonné accepté ensuite ; neuf temporaires : code 3 |
| `TestRestoreSpaceIsCheckedBeforeAnyWrite`, `TestRestoreLongPathIsReported` | Espace insuffisant : aucune structure ni journal ; chemins longs mesurés sur `I` |

**Restauration**

| Test | Preuve |
| --- | --- |
| `TestRestoreCopiesTheSnapshotExactly` | Entrées, tailles, dates, SHA-256, dossiers égaux au receipt de snapshot ; receipt de restauration chaîné au snapshot et à la génération |
| `TestRestoreWritesOnlyInsideTheArea` | Trace des appels : toute écriture sous `<dest>` ; aucune hors de la zone |
| `TestSnapshotChangedDuringRestore` | Contenu ou identité de `S` changés pendant la copie : `changed-during-restore`, `failed`, code 2, `I` conservé, rien publié |
| `TestVaultUnavailableDuringRestore` | Coffre retiré pendant la copie : `not verifiable`, `failed`, code 1, jamais `changed-during-restore` ; `I` conservé |
| `TestRestoreIDEqualToTheSnapshotIsRefused` | Identifiant tiré égal à celui du snapshot : refusé avant tout journal |
| `TestSuccessiveRestoresAccumulate` | Deux restaurations du même snapshot : deux `P`, le premier intact ; `snapshot-already-restored` au plan |
| `TestRestoreIDCollisionIsRefused` | Identifiant déjà employé dans la zone : code 1, avant tout journal |
| `TestCrashAtEveryRestoreCall` | Crash avant et après chaque appel au faux, état final exact par point ; coffre identique |
| `TestPublishedRestoreIsNeverTargetedAgain` | Après `published`, aucun appel ne vise `P` en déplacement ou en création, à travers toute restauration et récupération suivantes |

**Récupération**

| Test | Preuve |
| --- | --- |
| `TestRestoreRecoverPlanWritesNothing` | Aucun `Vaults` reçu ; empreinte de la zone inchangée |
| `TestRestoreRecoveryNeedsNoVault` | Coffre absent, renommé, illisible ou altéré : la récupération aboutit, A3 compris, sans ouvrir le coffre |
| `TestRestoreRecoveryAfterEveryCrash`, `TestRestoreRecoveryIsInterruptible`, `TestRepeatedRestoreRecoveryIsStable` | Propriétés du §9 d'ADR 0006 ; trois exécutions sur une ambiguïté : zone identique octet pour octet, diagnostic identique |
| `TestOrphanRestoreReceiptIsAmbiguous` | Receipt sans journal, ou d'une opération close autrement qu'en `published`, ou receipt d'une restauration publiée supprimé : code 3 |
| `TestHandedOverRestoreObservations` | `P` supprimé, laissé sous `I`, remplacé, ou flanqué d'un `I` : `restore-missing`, `restore-rename-lost`, `restore-replaced`, jamais bloquants ; `P` renommé : entrée inexpliquée, code 3 pour la zone seulement ; contenu jamais lu |
| `TestHandedOverObjectsOfAnyType` | Après `published`, `P` puis `I` devenus fichier, lien et point d'analyse : classés par la remise, jamais bloquants ; trace : identité lue par un handle sans suivi aussitôt refermé, ni ouverture pour le contenu, ni liste, ni épingle, ni lecture, ni suivi, ni déplacement ; `I` illisible à côté d'un `P` absent : `not observable` ; les mêmes objets sous le nom d'une opération non remise, et un `I` d'une autre identité pour une opération close : code 3 ; copie remise supprimée entre le plan et Z6 : nouvelle restauration non bloquée |
| `TestAreaWithoutJournalIsEmptyOrAmbiguous` | Chaque ligne du tableau « destination sans opération journalisée » du §4.5 : structure seule, code 0 ; `<op>`, `.incomplete`, receipt ou temporaire de receipt sans journal, code 3 ; jamais rien écrit |
| `TestRestoreRecoveryRedoesTheDistinctions` | Zone déplacée sous un marqueur de coffre ou dans une source : code 1, rien écrit |
| `TestRunningRestoreIsNotInterrupted` | `restore.lock` tenu : le plan de récupération donne code 1, jamais une opération en attente |
| `TestRestoreEnvelopeIsBounded` | Dépassement de E1, E2 et E3 : code 3 sans mutation |

**Windows réel et binaire**

| Test | Preuve |
| --- | --- |
| `TestRestoreEndToEnd` | `sync --apply`, `snapshot --apply`, `restore --apply`, `inspect` de la restauration égal au receipt de snapshot ; empreinte du coffre identique ; plans de `sync` et `snapshot` inchangés |
| `TestRestoreFromAWriteDeniedVault` | Coffre sous ACL de refus d'écriture : plan, `--apply` et récupération aboutissent |
| `TestRestoreBesideRealSyncAndSnapshot` | Portée de lecture de la restauration ouverte, épingles et fichier de `S` tenus, pendant qu'une rotation `sync --apply` et un `snapshot --apply` réels aboutissent ; puis l'inverse |
| `TestRestoreRefusesASubstAlias` (C4) | Arborescence synthétique en `t.TempDir()` ; lettre libre attribuée par `subst` à un dossier source, puis au coffre ; précondition vérifiée : même identité par les deux lettres ; `restore --apply` vers une zone sous l'alias, et l'inverse, zone par le chemin réel et source par l'alias : code 1, aucune écriture, empreinte de l'arborescence inchangée ; même refus pour `--recover --apply` d'une zone glissée sous l'alias ; puis alias retiré alors que le manifeste le nomme encore : refus, code 1, « separation cannot be proven » ; alias retiré en fin de test dans tous les cas. Exécuté par la CI Windows : l'impossibilité d'attribuer l'alias y est un échec, jamais un saut |
| `TestConcurrentMarkerRecovery` (V1) | `<dest>` ne contenant que des temporaires du marqueur, sur des arborescences synthétiques neuves, à de nombreuses reprises : deux `restore --recover --apply` lancés ensemble, puis une reprise et une création ensemble ; à chaque fois un seul marqueur validé, chaque processus en code 0 ou 1, jamais 3 ni de second marqueur, aucun `restore.lock` dans un dossier non marqué ; la zone obtenue est acceptée par le plan et par `--apply` suivants |
| `TestHandedOverJunctionIsNeverFollowed` | Après `published`, `P` remplacé par une jonction vers un dossier témoin : `restore-replaced`, empreinte du dossier témoin inchangée, aucune ouverture à travers la jonction |
| `TestRestorePlanUnderDeniedWrites`, `TestRestoreRecoverPlanUnderDeniedWrites` | Plans sous ACL de refus d'écriture : rien écrit |
| `TestRestoreRenameRefusedByAnOpenFile` | Fichier ouvert dans `I` : erreur 5, `rolled back`, code 1 |
| `TestRestoreKilledDuringCopyThenRecovered` | Processus tué pendant la copie ; récupération : `abandoned`, `I` conservé |

**CLI** : aide, flags, `--recover` qui refuse `--vault` et `--snapshot`,
codes documentés (`TestStableCodesAreDocumented` étendu à ADR 0009),
exemples README.

Vérification manuelle consignée au rapport : un binaire construit depuis
`f3074d0` donne, sur un coffre synthétique après restauration, des sorties de
`sync` et de `snapshot` identiques à celles d'avant.

---

## 10. Documentation livrée avec le code

`docs/commands/restore.md` ; page du receipt de restauration et du marqueur de
zone, avec le guide de récupération d'une zone ; `docs/vault.md`, renvoi :
le coffre n'est jamais écrit par `restore` ; modèle de sécurité,
non-écrasement et remise ; architecture ; README et ses exemples ; CHANGELOG ;
ADR ; `AGENTS.md`, lot courant. La documentation dit comment reprendre les
fichiers d'une restauration vers le jeu, à la main et à ses risques, sans
commande Savewarden.

---

## 11. Rollout, rollback et pilote

- Le code ne rejoint `release/v0.1.0-a1` que par PR ; aucun tag ni release.
- **Rollback du code** : revert de la PR, **neutre pour les coffres**, que B4
  n'écrit jamais. Les zones restent des dossiers ordinaires, lisibles par
  `inspect`.

**Pilote B4 (R14)**, exécuté par le propriétaire après fusion, avec un binaire
du commit de fusion, depuis le coffre du pilote B3, **lu seulement**, vers une
zone neuve hors du jeu, du coffre et de toute source :

1. Empreinte complète du coffre : inventaire et contenu.
2. `snapshot`, plan seul : le snapshot du pilote B3 est intact.
3. `restore`, plan seul : code `0`, snapshot `verified`, 14 fichiers attendus,
   aucune anomalie ; destination inchangée.
4. `restore --apply` : code `0`, `destination: published restore <op>`.
5. `inspect` d'un manifeste écrit hors du dépôt qui pointe sur la
   restauration : fichiers, tailles et SHA-256 égaux au receipt du snapshot.
6. `restore --recover` : rien à récupérer, code `0` ; nouveau plan : la
   restauration précédente est signalée en information.
7. Empreinte du coffre identique à l'étape 1 ; plans de `sync` et de
   `snapshot` inchangés.
8. Consigner codes, nombres et verdicts, sans hash individuel, identifiant
   d'opération, receipt réel, nom de profil ni chemin personnel.

La reprise manuelle des fichiers dans le jeu reste hors du pilote.

---

## 12. Critères d'acceptation

1. Le plan et `--recover` sans `--apply` n'écrivent rien : binaire sous ACL de
   refus, empreintes inchangées.
2. ADR 0006 et 0004 amendés et ADR 0009 acceptés avant tout code d'écriture ;
   ADR 0005, 0007, 0008, `readonly_test.go`, `architecture_test.go` et le
   code de `platform/vault` inchangés.
3. Une restauration publiée est égale, fichier par fichier, au receipt du
   snapshot, relue depuis le disque avant publication ; son receipt chaîne le
   snapshot et la génération d'origine.
4. `restore` n'écrit jamais dans le coffre, verrou compris : empreinte
   complète identique dans chaque mode, crash et récupération compris ;
   restauration possible depuis un coffre protégé en écriture.
5. Rien n'est écrasé, remplacé ni supprimé ; une restauration publiée n'est
   jamais rouverte en écriture ni déplacée ; aucune écriture hors de la zone.
6. Chaque point d'arrêt et chaque ligne du §5.5 sont atteints par un test ;
   la récupération respecte le §9 d'ADR 0006 dans l'enveloppe E1 à E4.
7. La récupération ne lit pas le coffre ; la restauration ne lit du coffre
   que le snapshot désigné et ne dépend ni de `current` ni des autres
   opérations.
8. Les suites B2 et B3 passent inchangées ; les sorties de `sync` et de
   `snapshot` sont identiques octet pour octet.
9. Un snapshot non publié, en cours, dégradé ou non vérifiable n'est jamais
   restauré, même en partie ; illisible et altéré sont distingués, hormis le
   volume non prêt au préflight (N2), documenté ; un coffre retiré pendant la
   copie n'est jamais pris pour un snapshot modifié.
10. Destinations non marquées, recouvrements, y compris par une autre lettre
    de lecteur, et zones imbriquées refusés sans écriture, à la création comme
    à la récupération ; une identité impossible à observer n'est jamais tenue
    pour une séparation ; le refus par alias `subst` est prouvé sur Windows
    réel par la CI.
11. Un objet de tout type portant le nom d'une restauration remise n'est
    jamais suivi, lu, déplacé ni bloquant ; une zone sans journal n'est
    déclarée sans objet que si elle ne porte aucun objet d'opération ; deux
    validations concurrentes du marqueur n'en valident qu'un, sur le faux et
    sur Windows réel, et le perdant n'écrit plus rien.
12. Revues indépendantes consignées : non-régression, protocole, sécurité.
13. 7 checks requis verts sur le SHA exact, avec lien du run.
14. Documentation, guide de récupération de la zone, CHANGELOG et ADR
    cohérents ; exemples README vérifiés par test ; limites écrites.
15. Aucun accès agent aux données protégées ; aucun chemin personnel, save ou
    receipt réel dans le diff.
16. Pilote B4 exécuté et qualifié par le propriétaire, ou absence déclarée.

---

## 13. Décisions reportées hors de B4

| Sujet | Comportement en B4 |
| --- | --- |
| Restauration en place | Refusée par les distinctions ; lot futur, ADR de sécurité et protection de l'état actif |
| Restauration de `current` ou de `retained\` | Non proposée : créer d'abord un snapshot |
| Restauration partielle | Non proposée : le snapshot entier |
| Coffre copié vers un autre volume | Refusé (R15) ; requalification ou import contrôlé dans un lot dédié |
| Nettoyage de zones, de restaurations ou de `.incomplete` | Jamais par Savewarden ; documenté |
| Vérification ultérieure des restaurations remises | Non ; `doctor` (B5) décidera s'il les lit |
| Classification P2-4 de `snapshot` | Inchangée ; seul `restore` distingue illisible et altéré |
| Budget global de lecture (P2-3) | Ouvert ; lectures de `restore` ciblées |
| Volume indisponible (N2) | Documenté ; classification future dans `platform/filesystem` |
| Extraction du moteur | Non retenue ; critères de réexamen (§3.1) |
| Attributs, ACL, flux alternatifs | Non reproduits |
| Volumes réseau, optiques, RAM, ou d'un autre système que NTFS | Refusés, comme pour le coffre, qui n'accepte que NTFS sur un lecteur local fixe ou amovible |
| ReFS | Toujours non qualifié |
| Destination dans un dossier redirigé ou synchronisé par un service de nuage | Refusée si un ancêtre est un lien ou un point d'analyse ; sinon non qualifiée, documentée |
| Chemin de référence sur une lettre sans volume, disque du jeu perdu ou débranché | Restauration refusée, code 1, tant qu'un volume n'est pas rattaché à cette lettre (question 7, acceptée) ; un volume de remplacement ne prouve pas l'identité historique de l'ancien disque. **B4 ne garantit pas la restauration lorsque le disque d'origine est indisponible** ; lot dédié pour la restauration avec des références hors ligne |
| Installation ou source déplacée sans mise à jour du manifeste | Protégée seulement dans un cas décisif de la règle d'observabilité ; documentation : mettre le manifeste à jour avant de restaurer |

---

## 14. Risques

| Risque | Effet | Parade |
| --- | --- | --- |
| Régression de `sync` ou `snapshot` par le paramétrage du moteur | Perte d'une garantie qualifiée | Commit distinct, suites B2 et B3 inchangées, sorties identiques, revue de non-régression obligatoire |
| Écriture dans le coffre par erreur de racine | Coffre modifié par une restauration | `Vaults.Open` jamais appelé sur le coffre, testé ; binaire depuis un coffre protégé en écriture ; empreinte du coffre au pilote |
| Restauration dans le dossier du jeu | Écrasement de sauvegardes actives | Distinctions avec l'installation et les sources, par identité quelle que soit la lettre de lecteur ; destination absente, vide ou marquée ; jamais de remplacement |
| Alias `subst` ou zone déplacée sous une source ou un coffre | Contournement des distinctions | Identités comparées entre lettres ; distinctions refaites par la récupération ; test Windows réel par `subst` en CI (C4) |
| Source ou installation absente, ou sur un volume indisponible | Séparation présumée sans preuve ; ou restauration impossible quand le disque du jeu est perdu | Règle d'observabilité (C3) : décision par la chaîne existante ; lettre sans volume ou chaîne non observable : refus (P0-1) ; recours documenté, rattacher un volume à la lettre ; question 7 |
| Objet quelconque sous le nom d'une restauration remise | Suivi d'un lien, ou zone bloquée à tort | Protocole de remise par `Lstat`, sans suivi ni lecture, jamais bloquant (C1) |
| Écriture dans un dossier de l'utilisateur | Mélange ou perte de fichiers | Dossier non vide sans marqueur refusé, jamais adopté ; `<dest>` épinglé et relu juste avant le marqueur, fenêtre résiduelle écrite |
| Opération future qui renommerait un snapshot publié ou le tiendrait sans partage | Prémisse de X1 levée | Invariant d'ADR 0006 §11 ; critère de réexamen (§3.1) |
| Snapshot altéré, restauré comme sain | Données fausses remises | Authentification complète avant `planned`, copie hachée contre le plan, relecture de `I` |
| Snapshot illisible pris pour altéré | Doute injustifié | `not verifiable`, code 1, sans dégradation (P2-4) |
| Coffre débranché pendant une restauration | Opération inachevée | `failed` en exécution ; récupération sans le coffre (R5) |
| Coupure qui perd le renommage | Restauration introuvable sous son nom | Récupération ; `restore-rename-lost` après `published`, `I` jamais renommé par conjecture |
| Utilisateur qui renomme ou ajoute dans la zone | Zone bloquée en code 3 | Documentation : copier hors de la zone ; nouvelle zone toujours possible ; coffre jamais concerné |
| Accumulation de restaurations et de `.incomplete` | Disque plein | Espace contrôlé avant toute écriture ; restaurations antérieures signalées ; documentation |
| Coffre copié vers un autre volume | Aucune restauration possible | Refus documenté (R15) ; lot de portabilité |
| Falsification coordonnée d'un snapshot | Données forgées restaurées | Hors modèle, limite écrite |
| Antivirus ou indexeur tenant `I` | Bascule refusée | Retries 32 et 33, erreur 5 immédiate, rollback |
| Package `sync` trop large | Lisibilité | Fichiers par opération, critères de réexamen de l'extraction |
| Extension vers la restauration en place | Lot trop gros, risque sur l'état actif | Non-objectifs §2.2 |
| Mainteneur unique | Revue limitée | Revues indépendantes, Human GO, aucune auto-approbation |

---

## 15. Rapport de restitution attendu

Branche et PR, SHA final, `git diff --stat`, gates et résultats exacts, run CI
et ses 7 checks, ADR créés ou amendés, énumérations de crash et tests de
non-mutation du coffre, revues indépendantes, résultat du pilote ou son
absence, écarts au plan, risques résiduels, questions ouvertes,
recommandation **sans auto-approbation**.

## Historique

| Version | Date | Commit | Changements |
| --- | --- | --- | --- |
| 0.1 | 1er novembre 2026 | local, branche `docs/b4-framing` | Brouillon de cadrage sur le GO du propriétaire : décisions R1 à R15 proposées, protocole de restauration proposé comme ADR 0006 §11, réexamen de l'extraction du moteur, points ouverts de B3 et leur effet sur B4, matrice de tests, critères. Revue indépendante en lecture seule intégrée avant remise : aucun P0 ; huit P1, soit état du snapshot côté coffre en code 1 sans sonder `sync.lock` (E4), reprise d'une initialisation interrompue, distinctions refaites par la récupération, recouvrement par identité entre lettres de lecteur, relecture de `<dest>` avant le marqueur, écarts E1 à E4 nommés, coffre retiré pendant la copie, couture source et cible du moteur et lecteurs amovibles acceptés comme pour le coffre ; P2 intégrés. Aucun ADR rédigé, aucun GO de développement |
| 0.2 | 1er novembre 2026 | local, branche `docs/b4-framing` ; fichier renommé depuis la v0.1 | Revue CTO de la v0.1 : architecture favorable, aucun P0 ; R1 à R15 et X1 à X4 recommandés, ratification par le propriétaire attendue. C1, grammaire de la zone en deux classes et protocole de remise quel que soit le type de l'objet ; C2, zone sans journal sans objet d'opération seulement ; C3, règle d'observabilité des chemins de référence, à la création comme à la récupération ; C4, test Windows réel par `subst` en CI. Projets d'ADR 0004, 0006 §11 et 0009, non acceptés. Revue croisée indépendante des textes, intégrée avant remise (§0.6) : P0-1, refus pour une lettre de référence sans volume ; P0-2, aucune session ni verrou avant le marqueur en récupération ; P1-1 à P1-9, dont les écarts renommés X1 à X4 ; questions 7 à 9 ouvertes. Aucun GO de développement |
| 0.2, revue CTO | 2 novembre 2026 | local, branche `docs/b4-framing` | Revue CTO de la v0.2 : avis favorable, aucun nouveau P0 ; Q7 acceptée avec la limitation de reprise après sinistre écrite, Q8 et Q9 acceptées ; V1, concurrence de la validation du marqueur et ses tests ; V2, statut du §10 d'ADR 0006 corrigé dans un commit séparé ; V3, phrases historiques d'ADR 0004 marquées et priorité de l'amendement écrite. R1 à R15 et X1 à X4 inchangés. ADR non acceptés ; aucun GO de développement |
| 0.2, validation | 3 novembre 2026 | local, branche `docs/b4-framing` | Paquet `342167f` certifié par le CTO ; plan validé par le propriétaire ; R1 à R15, X1 à X4 et Q7 à Q9 ratifiés ; ADR 0004 (amendement B4), 0006 §11 et 0009 passés au statut accepté ; contenu normatif inchangé. GO de développement B4 non donné |
| 0.2, correction éditoriale | 3 novembre 2026 | local, branche `docs/b4-framing` | Deux P2 de la revue CTO du commit d'acceptation : l'avertissement d'en-tête ne dit plus que le plan ne vaut pas acceptation d'ADR, acceptés par décision distincte ; §3.3 renommé « ADR du lot ». Contenu normatif inchangé |
| 0.2, erratum proposé | 3 novembre 2026 | local, branche `feat/b4-restore` | Revue de non-régression du commit 2 et vérification de ses corrections : erratum proposé au §3.1, au §8 et à l'amendement B4 d'ADR 0004 sur le plan de `sync` porté par la seule opération de `sync`, la couture répartie sur le commit 2 et ses corrections, et le faux coffre devenu plus strict ; en attente de ratification par le propriétaire. Aucune autre règle ne change |

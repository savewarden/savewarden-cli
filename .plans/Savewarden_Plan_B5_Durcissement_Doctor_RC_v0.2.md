# Savewarden — Plan B5 : durcissement S-01 et S-02, diagnostic `doctor` et release candidate

> **Statut : v0.2 — CADRAGE RÉVISÉ, soumis à une revue ciblée puis à l'acceptation du propriétaire : arbitrages Q1 à Q9 du 6 novembre 2026 intégrés ; revue indépendante r2 et revue CTO intégrées ; S-02 intégré à B5a ; aucun ADR rédigé ni accepté ; aucun GO de développement**
> Version du plan : `0.2` — 6 novembre 2026
> Lot : **B5**, cible `release/v0.1.0-a1`
> Cadre : [plan directeur v0.1](Savewarden_Plan_Bootstrap_Architecture_GitOps_v0.1.md) §7 B5,
> [plan B4 v0.2](Savewarden_Plan_B4_Restauration_v0.2.md), [AGENTS.md](../AGENTS.md),
> [modèle de sécurité](../docs/safety-model.md), chantiers
> [S-01](../docs/safety-model.md#chantiers-de-sécurité-ouverts) et S-02 (consigné par la
> PR #15, ouverte à cette date), ADR
> [0004](../docs/adr/0004-sync-boundaries.md) à [0009](../docs/adr/0009-restore-contract-receipt-v1.md),
> [rapport B4](../docs/validation/b4.md), [rapport B3](../docs/validation/b3.md).

Le plan directeur range trois choses dans B5. D'abord la commande `doctor`, un
diagnostic en lecture seule des manifestes, chemins, hash, receipts, snapshots
et restaurations remises. Ensuite la campagne pilote complète sur Odyssey,
avec des tests de régression Windows et la preuve de restauration isolée.
Enfin, une fois ces deux points acquis, l'extension du modèle à Cyberpunk 2077
puis à Metal Gear Solid V.

Le propriétaire a fixé les points de cadrage suivants.

- **5 novembre 2026.**
  - **S-01 entre en tête de B5.** Les écritures sont durcies contre une
    lettre de lecteur réattribuée pendant une session, avant `doctor` et la
    campagne. La campagne qualifie ainsi les primitives durcies, et non un
    code qui changerait après elle.
  - **Les quatre points ouverts de B4 entrent dans le cadrage** :
    commentaires de `platform/vault`, lecture du coffre par `sync` et
    `snapshot`, budgets de lecture avec les autres points ouverts de B3,
    alignement des six noms de tests de la matrice B4.
- **6 novembre 2026.**
  - **S-02 est corrigé dans B5a, avec S-01.** La revue indépendante r2 a
    trouvé qu'une lettre de lecteur qui désigne un dossier contourne les
    distinctions. Le défaut a été prouvé sur Windows réel le même jour.
  - Il est consigné comme chantier ouvert, avec une interdiction
    opérationnelle et les garanties écrites corrigées, par la PR #15.
  - **Les questions Q1 à Q9 sont arbitrées** (§0.3).

B5 vise la release candidate. Ce plan prépare cette candidate sans la
publier : aucun tag, aucune release, aucune publication de package. La
diffusion publique, au sens de T22, reste bloquée tant que S-01 et S-02 ne
sont pas fermés, chacun par son gate de sécurité.

B4 est clos sur le Human GO du propriétaire du 6 novembre 2026. Ce plan ne
vaut ni GO de développement, ni acceptation d'ADR, ni autorisation de push,
de tag ou de release, ni accès aux sauvegardes réelles. Aucun code de B5 ne
démarre avant l'acceptation de ce plan, son entrée dans la release branch
(T21) et un GO de développement distinct.

---

## 0. Décisions

### 0.1 Décisions T1 à T22

« Arbitrée » : tranchée par le propriétaire le 6 novembre 2026 (§0.3).
« Proposée » : prise avec l'acceptation du plan. La question de fond reste
celle de T2 et T20 : une session d'écriture ne doit dépendre d'aucune lettre
de lecteur une fois ouverte, et aucune lettre ne doit masquer un dossier.

| # | Décision | Raison principale | Statut | Où |
| --- | --- | --- | --- | --- |
| T1 | **Quatre PR successives** vers `release/v0.1.0-a1` : **B5a** S-01 et S-02 ; **B5b** lecteurs durcis et points ouverts ; **B5c** `doctor` ; **B5d** campagne pilote et préparation de la RC. Chacune a sa revue, sa CI et son GO de fusion | `platform/vault` est le package le plus sensible : il se relit seul. `doctor` repose sur les lecteurs durcis par B5b. Précédent : B2 en trois PR. Découpage retenu par la revue CTO | Proposée | §8 |
| T2 | **S-01, option A, sous conditions : la session écrit par un chemin de volume tiré de ses épingles.** Elle ouvre en deux branches, cible existante ou absente. Elle contrôle la cohérence de sa chaîne, relit le chemin final de chaque nouveau handle et s'invalide au premier défaut de périphérique. Ses erreurs ne nomment jamais sa base. Aucune API d'écriture nouvelle | Les épingles empêchent déjà de renommer les dossiers du chemin ; seule la lettre restait résolue à chaque appel. L'option B demanderait `NtCreateFile`, `SetFileInformationByHandle` et `unsafe`, interdits partout par ADR 0005 ; elle serait un nouvel arbitrage, jamais un repli | Arbitrée (Q1) | §3.1, §4.1 |
| T3 | **Hypothèses Windows confrontées à des tests réels avant le code** : HW1 à HW12 pour B5a, HD1 et HD2 pour B5c (§5), chacune dans un commit de tests seuls. Si une hypothèse tombe : arrêt, compte rendu, arbitrage du propriétaire. Aucun repli silencieux | Même discipline qu'ADR 0006 : les hypothèses Windows sont prouvées avant le code qui en dépend | Proposée | §5, §8 |
| T4 | **Contrôles sous session sur les chaînes tenues.** `sync`, `snapshot`, `restore` et chacune de leurs récupérations comparent la chaîne tenue par la session aux chaînes de référence, dans les deux sens, **après `Open` et avant toute écriture, verrou compris**. L'ordre est fixe. Une observation impossible donne le code 1, sans rien écrire. La `Session` gagne des requêtes en lecture seule, changement d'interface déclaré | La comparaison de la seule racine ne prouve pas l'absence de recouvrement. La fenêtre de `sync` n'est pas fermée par la relecture du marqueur : des écritures la précèdent (`apply.go:350-388`) | Proposée | §4.3 |
| T5 | **Requalification de B2, B3 et B4**, exigée par la sortie de S-01 et de S-02. Les suites complètes restent inchangées. Le différentiel binaire des sorties de `sync`, `snapshot`, `restore` et `inspect` se fait contre le binaire de `a59d787`. Il est normalisé (identifiants d'opération, dates), couvre les chemins de succès et d'erreur et exclut les seuls changements nommés au §7.2. Tests réels de réattribution pendant une session | Les primitives changent de chemin, pas de comportement, hors des refus nommés : la preuve doit le montrer | Proposée | §9, §12 |
| T6 | **Commentaires de `platform/vault`** (point B4), écrits dans B5a : une session s'ouvre sur un coffre ou sur une zone, et écrit par le chemin de volume de ses épingles | Le package est rouvert par S-01 ; c'est le seul moment où le toucher ne coûte pas une revue de plus | Proposée | §7 |
| T7 | **Lecture stable du coffre** (point B4). Reproduire sur faux la fenêtre de `readProbing` pour le domaine courant, le domaine des snapshots et les plans de récupération. Si elle est reproduite : lecture stable, puis sonde du verrou indépendante de la liste, comme pour la zone. Sinon, consigner la preuve et ne rien changer | Le défaut est probable, pas démontré. Il n'écrit rien et donne un faux code 3, sûr mais trompeur. `doctor` lira le coffre avec ce lecteur | Proposée | §4.4 |
| T8 | **Budgets de lecture** (P2-3 de B3, point B4). Ils portent sur les métadonnées seulement, par lecture, jamais sur un contenu vérifié contre son receipt. Celui-ci est haché en flux, borné par les tailles et les nombres de son receipt. Les valeurs sont dérivées d'un dimensionnement écrit (§4.4) ; celle des octets est mesurée en M1. Un budget épuisé donne le code 1 et un message distinct dont l'issue est un coffre neuf | Un coffre forgé ne doit pas épuiser la mémoire. Un budget sur le contenu ou par exécution refuserait `--verify` et des coffres légitimes | Arbitrée (Q9) | §4.4 |
| T9 | **Points ouverts de B3** traités dans B5b selon le §0.2 : P2-4 avec un amendement d'ADR 0008 accepté avant son code ; N2 par une méthode marqueur ; plafond de profondeur JSON ; verrou de la récupération de `current` signalé ; `.gitignore`. Tolérance de B2 reportée | Décision du propriétaire du 5 novembre 2026. `doctor` dépend de P2-4 : il doit distinguer « illisible » de « modifié » | Proposée ; report arbitré (Q5) | §0.2 |
| T10 | **Noms de la matrice B4** : les six noms du plan deviennent des tests nommés ou des sous-tests, sans changement d'assertion ; la correspondance est consignée | Décision du propriétaire du 5 novembre 2026 ; commit de tests seuls | Décidée le 5 novembre | §8 |
| T11 | **`doctor` est une feature, `features/doctor`.** Elle possède son orchestration et son rapport et définit ses ports. `sync` lui fournit des **relevés** en lecture seule, rendus dans de nouveaux types d'observation de `entities/backup`. `app` passe un adaptateur étroit. Nouvelle frontière : **ADR 0010** et amendement d'ADR 0004 | Les trois options sont comparées avec leurs coûts au §3.4. La feature isole du code de lecture à côté des moteurs d'écriture, et les tests d'imports la contrôlent | Proposée | §3.4 |
| T12 | **`doctor` n'écrit jamais et ne gêne aucune écriture.** `app` ne lui donne ni `Writer`, ni session, ni verrou. Il ne sonde aucun verrou et n'ouvre jamais `sync.lock` ni `restore.lock`, qu'il classe par l'entrée de répertoire. Il n'épingle rien et partage lecture, écriture et suppression à chaque ouverture | Sonder le verrou, ou seulement le lister par `Lstat`, l'ouvrirait : `os.Root.Lstat` ouvre l'entrée, et le `Lock` sans partage d'un `sync` concurrent échouerait (X1 de B4) | Proposée | §4.5 |
| T13 | **Portée de `doctor`** : manifeste strict ; lettres (S-02) ; chemins, distinctions et liens ; coffre (marqueur, `current`, générations conservées, journaux, receipts, snapshots, enveloppe) ; zones désignées par `--area`, répétable, **un constat par zone**. Le coffre ne connaît pas les zones : `doctor` ne les cherche pas | Le diagnostic porte sur ce que l'utilisateur désigne, comme toutes les commandes. Aucune découverte | Proposée | §4.5 |
| T14 | **Deux niveaux.** Par défaut, un diagnostic **structurel**, sans lire le contenu. Avec `--verify`, `doctor` relit en flux `current`, chaque génération conservée et chaque snapshot publié, et les compare à leur receipt. **Les restaurations remises ne sont jamais ouvertes pour leur contenu, listées ni lues**, même avec `--verify` : présence, type et identité seulement | Le défaut reste rapide ; `--verify` comble la limite de B3. ADR 0006 §11 et ADR 0009 restent inchangés ; vérifier une copie remise passe par un `inspect` explicite | Arbitrée (Q3) | §4.5 |
| T15 | **Codes de `doctor`** : `0`, `2`, `3`, `1`, avec une **table normative par domaine** fixée par ADR 0010 (§4.5). Règle : jamais moins sévère que la commande qui agirait. Les issues normales sont des informations. Une opération en attente non départagée est un constat distinct, code 3, avec pour action `--recover` sans `--apply`. Le code 1 est réservé à l'absence de rapport | Mêmes sens qu'ADR 0003, 0007 et 0009 : `0` fiable, `2` à examiner, `3` à récupérer, `1` à corriger. Une règle uniforme contredirait l'existant | Arbitrée (Q4) | §4.5 |
| T16 | **Campagne pilote Odyssey obligatoire**, rotation comprise (jouer et sauvegarder entre deux `--apply`). Elle est exécutée par le propriétaire avec un binaire qu'il reconstruit par le GoReleaser épinglé, sur le commit candidat de B5d. `doctor` est passé, en lecture seule, sur les coffres B2c et B3 et sur la zone B4 ; le coffre historique est exclu | S-01 réécrit justement les créations et les renommages que la campagne doit qualifier. Aucune « absence déclarée » n'est admise pour clore B5 | Proposée | §11 |
| T17 | **Préparation de la RC** (B5d) : build de contrôle en snapshot par `go run` de GoReleaser à version exacte, Windows amd64 seul, sans hook, sans artefact, `release: disable: true`. Le vérificateur de pins refuse `upload-artifact` et les permissions d'écriture. CONTRIBUTING et `gitops.md` distinguent ce build de contrôle du pipeline de release. Aucun changement du Go de production en B5d | Plan directeur §5.2 point 6 et §5.3. Sur un dépôt public, un artefact de workflow est un binaire distribué. `platform/vault` renvoie `ErrUnsupported` hors Windows | Arbitrée (Q8) | §4.6 |
| T18 | **Extension à Cyberpunk 2077 puis à Metal Gear Solid V : hors de B5**, dans un lot B6 cadré à part, consigné comme écart au plan directeur §7. La RC porte sur Odyssey seul, écrit dans le README, le CHANGELOG et le guide du pilote | Le manifeste v1 refuse les variables et dossiers connus, « prévus avec le pilote Cyberpunk 2077 » | Arbitrée (Q6) | §2.2, §13 |
| T19 | **B5 reste sur `release/v0.1.0-a1`.** Le libellé de la première version publiée est fixé au gate de publication, après la fermeture de S-01 et de S-02 | AGENTS.md : toute suite part de `release/v0.1.0-a1`. Rien n'est publié en B5 | Proposée | §11 |
| T20 | **S-02 : en B5, toute lettre qui n'est pas une racine de volume est refusée** pour le coffre, la zone et les chemins du manifeste, ainsi que pour les chemins sauvegardés que `restore` compare. Les chaînes sont construites de toute façon depuis la vraie racine du volume. Le raccourci « même lettre » est supprimé : `sync` et `snapshot` comparent par identité quelle que soit la lettre. L'acceptation des alias est reportée après preuves | Un alias vers un sous-dossier fait écrire Savewarden dans l'installation, une source ou le coffre (prouvé sur Windows réel). Refuser est fail-closed. Reconstruire depuis la vraie racine ferme la même classe de défaut pour les chaînes tenues | Arbitrée (Q2) | §3.2, §4.2 |
| T21 | **Entrée du plan et gouvernance.** Le plan accepté rejoint la release branch par une PR documentaire depuis `docs/b5-framing`. La branche est mise à jour par un merge de `release/v0.1.0-a1`, sans rebase ni force-push, une fois connu le sort de la PR #15. La PR porte aussi `AGENTS.md` (lot courant B5), `docs/architecture.md` et `docs/validation/b4.md`. La fusion se fait par merge commit sur Human GO, puis la branche est supprimée sur le remote et en local. B5a part de ce merge commit | Le ruleset exige une branche à jour ; AGENTS.md doit nommer le lot courant avant tout code ; les GO de B4 doivent être consignés | Proposée | §8 |
| T22 | **Diffusion publique**, bloquée tant que S-01 ou S-02 est ouvert : tout tag, toute release ou brouillon de release, tout artefact de workflow, tout GitHub Package ou image, tout binaire joint ailleurs, tout canal de paquets, toute annonce. Un tag publie de façon irréversible sur `proxy.golang.org` : un ruleset de tag précède le premier. La visibilité du code en est exclue. Aucun binaire n'est présenté comme publiquement qualifié avant ces deux gates | Aligne le terme sur « diffuser une version publique » du modèle de sécurité ; complété par la revue r2 et la revue CTO | Arbitrée (Q7) | §11, §13 |

Écartés d'emblée, hors de B5 : restauration en place, réparation par
`doctor`, nettoyage, rétention, portabilité d'un coffre copié, API
d'écriture nouvelle, sortie JSON, support fonctionnel de Linux et macOS,
acceptation des alias de lettre.

### 0.2 Points ouverts de B4 et de B3

| Point | Origine | Décision v0.2 | Sous-lot |
| --- | --- | --- | --- |
| Commentaires de `platform/vault` | B4, plan §7 | Écrits (T6) | B5a |
| Lecture non atomique du coffre par `sync` et `snapshot` | B4 | Reproduire pour les deux domaines et les plans de récupération ; corriger ou consigner (T7) | B5b |
| Tests de course de B4 en CI réelle | B4 | **Fait.** Exécution **inférée** du code et du job, consignée à la clôture de B4 (`docs/validation/b4.md`, l.186 et l.455-465). Pour B5, les tests de course et de différentiel échouent en CI s'ils ne sont pas exercés (§9) | — |
| Six noms de tests | B4, matrice §9 | Alignés (T10) | B5b |
| Budget de lecture de la zone | B4 | T8 | B5b |
| Budget de lecture du coffre (P2-3) | B3 | T8 | B5b |
| Lecture d'un snapshot approuvé (P2-4 de la revue de sécurité de B3) | B3 | « Non vérifiable » distinct de `snapshot-modified`. Portée : `snapshot` seul, `sync` ne lisant pas le domaine des snapshots. Plan : code 1, cause nommée, rien conclu. `--apply` : code 1, sans dégradation ni second snapshot. Issue d'un snapshot illisible en permanence (ACL) : rendre le fichier lisible, ce que nomment le message et `doctor` ; aucun contournement. Amendement d'ADR 0008, accepté avant son code | B5b |
| Volume indisponible à la porte (N2) | B3 | Une erreur qui reste `errors.Is(fs.ErrNotExist)`, avec une méthode marqueur comme `Busy()`, consommée par la seule porte de provenance (`snapshot_apply.go`). `inspect`, `restore` et les autres consommateurs sont inchangés. Le classement de `platform/vault` est aligné ou sa différence justifiée | B5b |
| Profondeur JSON (P2-2) | B3 | Plafond explicite dans `strictValue`, indépendant de la chaîne d'outils ; le message garde le mot « depth », attendu par le test existant | B5b |
| Verrou créé par la récupération de `current` | B3, hérité de B2 | Signalé comme une écriture, comme M1 de B3 l'a fait pour un snapshot | B5b |
| `.gitignore` (M7) | B3, B4 | Ajouter `**/journal/`, `*.rec`, `**/staging/`, `**/retained/`, `savewarden-vault.json`, `savewarden-restore.json`, `sync.lock`, `restore.lock`, `**/*.incomplete/` | B5b |
| Tolérance de B2 (`preparedFor`) | B3 | **Report arbitré (Q5).** Limite datée, écrite dans le modèle de sécurité ; lot cible : B6, ou le premier lot qui rouvre la récupération de la rotation. `doctor` la signale ; tests de non-régression inchangés ; aucune adoption automatique | — |
| `snapshots\receipts` supprimé | B3 | Code 3 inchangé dans **`snapshot`**, seule commande qui lit ce domaine. `doctor` nomme le cas et la procédure, sans suppression : recréer le dossier vide | B5c |
| Concurrence réelle du marqueur | B4 | Inchangée : la logique est prouvée de façon déterministe sur faux | — |

### 0.3 Arbitrages du propriétaire du 6 novembre 2026

Source : réponses du propriétaire en session de cadrage, le 6 novembre 2026.
« Oui, toutes, telles qu'écrites » vaut pour Q1 et Q3 à Q9, sur les
recommandations convergentes de la revue r2 et de la revue CTO. Q2 a reçu une
réponse propre. Les GO ne sont pas encore consignés dans le dépôt : la PR
documentaire de T21 les consigne.

1. **Q1, option A sous conditions (T2).** Conditions : les hypothèses HW
   (§5), P1-01 et P1-02 de la revue CTO (§4.1, §4.3), S5-P1-01 et S5-P1-03 à
   S5-P1-06 de la revue r2 (§4.1). L'option B serait un nouvel arbitrage,
   avec un amendement préalable d'ADR 0005, jamais un repli implicite.
2. **Q2, S-02 (T20).** « Refuser en B5 toute lettre qui n'est pas une racine
   de volume (coffre, zone, chemins du manifeste), et construire de toute
   façon les chaînes depuis la vraie racine du volume ; acceptation des
   alias reportée après preuves. »
3. **Q3, présence seulement (T14).** ADR 0006 §11 et ADR 0009 restent
   inchangés. Vérifier une restauration remise passe par un `inspect`
   explicite, avec un manifeste qui la désigne, comme au pilote B4.
4. **Q4, codes 0, 1, 2 et 3 (T15)**, avec une table normative par domaine
   dans ADR 0010. Une opération en attente non départagée est un constat
   distinct, en code 3, avec pour action `--recover` sans `--apply`.
5. **Q5, report de la tolérance de B2** : limite datée, lot cible nommé,
   signalée par `doctor` (§0.2).
6. **Q6, B6 (T18)**, consigné comme écart au plan directeur §7 ; « Odyssey
   seul » écrit dans le README, le CHANGELOG et le guide du pilote.
7. **Q7, définition de la diffusion publique complétée (T22).**
8. **Q8, Windows amd64 seul (T17).** La contradiction avec CONTRIBUTING est
   levée dans B5d. SBOM et attestations sont décidés avant toute publication,
   donc avant le premier tag.
9. **Q9, portée des budgets fixée avant les valeurs (T8).** Les valeurs de
   comptage sont dérivées au §4.4. **Écart à signaler** : la valeur en octets
   ne peut pas être mesurée sans code. Elle est mesurée par le commit M1 de
   B5b, sur un coffre synthétique, puis consignée par un erratum de ce plan
   et acceptée par le propriétaire. Cela se fait avant le code des budgets et
   avant ADR 0010.

Avis transmis à la revue CTO : le propriétaire lui adresse S-02 et la revue
r2. La v0.2 avance en parallèle et intègre cet avis à sa revue ciblée.

### 0.4 Revues intégrées et traçabilité

Revue r2 du 6 novembre 2026 : rapport consolidé et deux rapports bruts,
identifiants `S5-*` pour B5a et B5b, `D5-*` pour B5c, B5d et la gouvernance.
Revue CTO du 5 novembre 2026 sur `646d11f` : `P1-01` à `P1-03`, P2 numérotés.

| Constat | Traitement dans la v0.2 |
| --- | --- |
| S5-P0-01, S-02 : un alias de lettre contourne les distinctions | T20 ; §3.2 ; §4.2 ; HW11, HW12 ; tests réels des deux côtés (§9) |
| CTO P1-01, S5-P1-06 : cible absente, hypothèses manquantes | §4.1, ouverture en deux branches ; HW3, HW9 à HW11 |
| CTO P1-02, S5-P1-02 : T4 insuffisant | T4 ; §4.3 ; commit séparé (§8) |
| CTO P1-03, D5-P1-06 : `--verify` sur les restaurations remises | T14 (Q3) |
| S5-P1-01 : espace de noms local de `Volume{GUID}` | §4.1, chemin final de chaque handle ; HW8 ; §6 |
| S5-P1-03 : erreurs qui portent la base | §4.1 ; test d'erreur forcée ; différentiel des erreurs |
| S5-P1-04 : `Describe` sous session | §4.1, requête par handle ; HW10 |
| S5-P1-05 : volume retiré puis réinséré | §4.1, invalidation ; HW7 scindé ; §6 corrigé |
| S5-P1-07 : portée de N2 | §0.2 |
| S5-P1-08, D5-P1-05, CTO P2-5 : budgets | T8 ; §4.4 |
| D5-P1-01, D5-P2-09 : `doctor` gêne `sync` | T12 ; §4.5 ; HD1, HD2 ; test de concurrence |
| D5-P1-02, D5-P2-01, D5-P3-05 : frontière de `doctor` | T11 ; §3.4 |
| D5-P1-03, D5-P2-10 : points de contrôle, bases des différentiels | §8 |
| D5-P1-04, CTO P2-1, D5-P3-06 : codes de `doctor` | T15 ; §4.5 |
| D5-P1-07, D5-P1-08, D5-P1-09, D5-P2-08 : outillage de la RC | T17 ; §4.6 ; critère 11 |
| D5-P1-10 : revue du job | §8 ; critère 12 |
| D5-P1-11, D5-P2-02, D5-P2-03 : campagne | T16 ; §11 |
| CTO P2-2, S5-P2-06 : HW7 et la CI | HW7 ; critère 1 |
| CTO P2-3 : politique `subst` | T20 |
| CTO P2-4, S5-P2-04 : lectures encore faites par lettre | §4.3, lectures sous session ; §6 |
| CTO P2-6, D5-P2-05, D5-P2-06, périmés | §1 ; T21 |
| CTO, Q7 : binaire présenté comme qualifié | T22 |
| S5-P2-01 : T7 incomplète | T7 ; §4.4 |
| S5-P2-02 : alternative `os.Root` | §3.1 |
| S5-P2-03 : catégorie de l'amendement d'ADR 0005 | §3.5 ; §4.1, instrumentation |
| S5-P2-05 : portée de P2-4 | §0.2 |
| S5-P2-07 : sorties non nommées | §7.2 ; §4.4, noms hors grammaire |
| S5-P2-08 : commit 3 trop large ; regard de sécurité | §8 |
| D5-P2-04 : exception aux rulesets | §7.3 |
| D5-P2-07 : un constat par zone | T13 ; §4.5 |
| S5-P3-01 à S5-P3-05 | HW2 ; §0.2 ; T5 |
| D5-P3-01 à D5-P3-04, D5-P3-07 | T12 ; §4.5 ; §0.2 ; §13 ; §2.3 |

---

## 1. Contexte et preuves

| Élément | État vérifié le 6 novembre 2026 |
| --- | --- |
| `release/v0.1.0-a1` | `a59d787`, merge de la PR #14 (clôture de B4, depuis `docs/b4-closure`). Code identique à `4197317` : la PR #14 ne touche que trois fichiers de documentation |
| CI | Run 42155814194 sur `a59d787` : 7/7, `success` |
| `main` | `25d1cc2`, bootstrap B0 seul |
| B4 | **Clos.** Code : PR #13, `4197317`. Pilote réel exécuté par le propriétaire le 5 novembre 2026, 101 contrôles sur 101, qualifié le 6. Clôture : PR #14, `a59d787`. Human GO de clôture du 6 novembre 2026, donné en session ; `validation/b4.md` le liste encore comme restant, ce que corrige T21 |
| S-02 | Trouvé par la revue r2 du 6 novembre 2026, prouvé sur Windows réel le même jour, hérité de B2 et de B4. Consigné comme chantier ouvert par la PR #15 (`docs/s02-letter-alias`, `e6bb4cf`, CI 7/7, run 42232254323), **ouverte, non fusionnée**. Interdiction opérationnelle : aucun alias de lettre, ni pour `--vault` ou `--to`, ni dans le manifeste |
| Cadrage B5 | `docs/b5-framing` : v0.1 en `646d11f`, parent `4197317`, présente sur le remote. L'autorisation de ce push n'est consignée ni dans le dépôt ni dans les rapports : elle reste à citer par le propriétaire. v0.2 : commit local sur cette branche, sans push |
| Revues | Revue r2 du 6 novembre 2026 : deux examinateurs, un P0, dix-huit P1, constats porteurs revérifiés. Revue CTO du 5 novembre 2026 : 0 P0, 3 P1, 6 P2. Elle est antérieure à S-02, qui dépasse son « 0 P0 » et la prémisse de son P1-02 (« règle déjà acquise en B4 ») |
| Visibilité | Dépôt **public** |
| Toolchain | Go 1.27.3, golangci-lint 2.14.0, `golang.org/x/sys` v0.49.0 |
| Release | Aucune configuration GoReleaser ; un seul workflow, `ci.yml` ; aucun tag ; aucun ruleset de tag, selon la revue r2 |

Constats du code, relus à `a59d787` :

- **Session d'écriture.** `Session` garde `base`, le chemin à lettre passé à
  `Open` (`platform/vault/vault_windows.go`).
  - Chaque nom est résolu par `absolute`, puis préfixé `\\?\` par `longPath`.
  - `CreateDirectory`, `CreateFile` (`CREATE_NEW`), `MoveFileEx` (drapeaux
    `0`), `Identity`, `Hold` et `Lock` ouvrent tous par chemin complet, donc
    par lettre.
  - Les épingles, sans `FILE_SHARE_DELETE`, partent de la racine de la
    **lettre**, qui n'est pas celle du volume pour un alias. `Open` résout de
    nouveau la lettre pour chaque ancêtre, sans contrôler la cohérence de la
    chaîne.
  - Une cible absente est acceptée : seul son parent est tenu, et un parent
    absent est refusé (`:45-79`).
- **Distinctions, S-02.**
  - `restore` construit ses chaînes depuis la racine de la lettre
    (`restore_distinct.go:74-101`, `:139-159`, `:178-190`).
  - `sync` et `snapshot` ne comparent coffre et installation que sur une même
    lettre (`sync.go:117`, `snapshot.go:158`, `snapshot_recover.go:105`,
    `vault.go:573-581`).
  - ADR 0006 §11 exige des chaînes « depuis la racine du volume » : le code
    de `restore` n'y est pas conforme. La distinction du coffre de `sync`
    vient du plan B2 ; aucun ADR ne la fixe.
- **Distinctions sous session.**
  - `restore` refait ses distinctions sous session, par lettre, en mêlant
    `os.Root` et chemins complets.
  - `sync` et `snapshot` n'en refont aucune. Leurs écritures I2, I3, I5 et
    I6 précèdent la relecture du marqueur sous verrou (`apply.go:350-388`).
- **Erreurs.**
  - `os.NewFile(handle, path)` (`:185`) met le chemin complet dans les
    erreurs. Elles vont sur stderr et sont inscrites par l'enregistrement
    `failed` du journal (`copy.go:126-128`, `apply.go:615`).
  - Les messages de `Close` et de `Describe` nomment aussi la base (`:328`,
    `:349`, `:357`).
- **`Describe`.** Fonction de package, chemin à lettre seulement
  (`:337-359`) ; le port `Volumes` prend un chemin (`ports.go:47-51`).
- **API.**
  - `x/sys/windows` v0.49.0 fournit `GetFinalPathNameByHandle` (son wrapper
    ne signale pas un tampon trop petit), `GetVolumeNameForVolumeMountPoint`
    et `GetVolumeInformationByHandle`. Aucune constante `VOLUME_NAME_*` n'y
    figure.
  - `x/sys` est confiné à `platform/vault` (`architecture_test.go:30-32`).
    `platform/filesystem` n'a droit qu'à `syscall`, qui n'expose aucune
    requête de chemin de volume.
  - `unsafe` et `NtCreateFile` sont interdits partout.
- **Lecture du coffre.**
  - `readProbing` ne sonde le verrou qu'à la fin. La sonde dépend de la
    liste (`v.lock`) et des seules opérations en attente (`vault.go:150`).
  - L'ordre de lecture diffère : journaux puis receipts dans le coffre,
    l'inverse dans la zone. `readSnapshots` et les plans de récupération ont
    la même forme.
- **Ouverture par `Lstat`.** Sous Windows, `os.Root.Lstat` ouvre l'entrée
  avec `FILE_GENERIC_READ` (sources de Go 1.27.3). Lister `games\<id>` ouvre
  donc `sync.lock` un instant. Le `Lock` sans partage d'un `sync` concurrent
  (`vault_windows.go:303`) échoue pendant cet instant.
- **Budgets.**
  - Chaque fichier lu est borné à 64 Mio.
  - Un journal est borné par l'enveloppe E2 : 11 enregistrements, huit
    essais au plus à côté d'un enregistrement (`journal.go:20-22`).
  - Ni le nombre d'entrées listées ni le nombre de journaux et de receipts
    ne sont bornés.
- **Manifeste et architecture.** Inchangés depuis la v0.1. Le manifeste v1
  n'accepte qu'une base `game-install`. `docs/architecture.md` prévoit une
  feature `doctor`. Une feature n'importe pas une autre feature.

---

## 2. Objectif, non-objectifs, permissions

### 2.1 Objectif

À la fin de B5 :

1. **S-01 fermé** : une fois une session ouverte, aucune création, aucun
   renommage, aucune épingle, aucune identité ni aucun verrou ne résout plus
   de lettre de lecteur.
   - Les décisions prises sous session portent sur les objets tenus.
   - Le résidu de l'espace de noms local est écrit, avec la phrase de modèle
     de menace.
   - Des tests Windows réels réattribuent une lettre pendant une session.
2. **S-02 fermé.**
   - Une lettre qui n'est pas une racine de volume est refusée pour le
     coffre, la zone, les chemins du manifeste et les chemins sauvegardés
     comparés.
   - Les chaînes partent de la vraie racine du volume, et le raccourci
     « même lettre » a disparu.
   - Les tests réels couvrent `sync`, `snapshot`, `restore` et leurs
     récupérations.
   - B2, B3 et B4 sont requalifiés, et le gate de sécurité de S-01 et S-02
     est passé.
3. **Lecteurs durcis** : la lecture du coffre est stable ou la preuve est
   consignée ; les budgets sont appliqués ; les points ouverts de B3 sont
   corrigés ou arbitrés.
4. **Diagnostic** :

   ```text
   savewarden doctor --manifest <m> --vault <v> [--area <dest>]... [--verify]
   ```

   Le rapport, en lecture seule, donne pour chaque constat son objet, son
   code stable et l'action à mener. Il ne répare rien et ne gêne aucune
   écriture.
5. **Campagne et build** : Odyssey requalifié de bout en bout sur le code
   candidat par le propriétaire, rotation comprise ; build de contrôle en
   snapshot vérifié en CI, sans artefact ni publication.

### 2.2 Non-objectifs B5, opposables en revue

- **Réparation, nettoyage ou adoption par `doctor`** : il diagnostique et
  nomme l'action, jamais ne l'exécute.
- **Contenu des restaurations remises** : jamais ouvert, listé ni lu par
  `doctor` (Q3).
- **Acceptation des alias de lettre** : reportée après preuves (Q2).
- **Protection contre un processus hostile du même compte** : hors du modèle
  de menace (§6).
- **Restauration en place**, restauration partielle, rétention, suppression.
- **Cyberpunk 2077 et Metal Gear Solid V** : lot B6 (T18).
- **Portabilité d'un coffre copié** vers un autre volume (R15 de B4).
- **Toute API d'écriture nouvelle**, tout drapeau de remplacement.
- **Tag, release, publication**, canaux Scoop, Homebrew ou Winget, artefact
  de workflow.
- Sortie JSON de la CLI ; support fonctionnel de Linux et macOS.
- Durcissement des **lectures** contre une lettre réattribuée pendant la
  lecture : limite écrite au §6.

### 2.3 Permissions : agent, produit, propriétaire

| Acteur | Peut | Ne peut pas |
| --- | --- | --- |
| **Agent** | Lire et modifier le dépôt ; tests synthétiques en `t.TempDir()` ; lettres `subst` temporaires et noms DOS locaux de test, créés et retirés par les tests, vers des dossiers temporaires ; HW8 en CI seulement | Lire `D:\Ga`, `D:\Savewarden`, `E:\GMS`, un coffre ou une zone de pilote, ou des clés SSH ; lancer Savewarden sur des données réelles sans autorisation explicite et nominative pour cette exécution ; modifier une lettre ou un nom DOS qu'un test n'a pas créé ; exécuter HW7 |
| **Savewarden**, lancé par le propriétaire | `doctor` : lire le manifeste, le coffre désigné, les zones désignées et les identités de l'installation et des sources, sans suivi de lien ; les autres commandes, comme en B4 | `doctor` : écrire, verrouiller, sonder ou ouvrir `sync.lock` et `restore.lock`, épingler, ouvrir une session, réparer, ouvrir une restauration remise pour son contenu |
| **Propriétaire** | Exécuter la campagne et HW7 ; autoriser nominativement une exécution par l'agent ; ajouter un check requis au ruleset ; donner chaque GO | — |

---

## 3. Architecture et ADR

### 3.1 S-01 : où cesser de résoudre une lettre (T2)

| Option | Mécanisme | Pour | Contre |
| --- | --- | --- | --- |
| **A, retenue sous conditions** | Chemin de volume tiré des épingles par `GetFinalPathNameByHandle(VOLUME_NAME_GUID)`, utilisé pour tout appel après `Open` ; complété par le contrôle du chemin final de chaque nouveau handle | Aucune API d'écriture nouvelle ; les mêmes `CreateDirectory`, `CreateFile` et `MoveFileEx` ; des requêtes en lecture ajoutées au paragraphe x/sys d'ADR 0005 | Repose sur les épingles pour figer les dossiers du volume, ce qu'elles font déjà ; `\??\Volume{GUID}` passe par l'espace de noms DOS local (HW8) ; HW1 à HW12 à prouver |
| B | Créations relatives au handle du parent (`NtCreateFile`, `RootDirectory`, `FILE_CREATE`) ; renommage par handle sans remplacement | Plus aucun chemin, même dans le volume ; ferme l'espace de noms | `NtCreateFile`, `SetFileInformationByHandle` et `unsafe` interdits partout. Un garde fondé sur les noms ne sait pas restreindre `SetFileInformationByHandle`, qui renomme aussi bien qu'il supprime. `FILE_RENAME_INFO` est de taille variable, donc `unsafe` |
| C | Contrôler après chaque écriture que la cible est sous la racine tenue | Aucun changement de chemin | Détecte sans empêcher ; ne satisfait pas la sortie de S-01. Retenue seulement comme défense en profondeur de A |
| `os.Root` | Créations relatives à un handle (`Mkdirat`), sans `unsafe` dans Savewarden | Aucun chemin après l'ouverture | Son `Rename` remplace la cible ; `os.Root` suit les liens internes (AGENTS.md) : écartée |
| `VOLUME_NAME_NT` | Chemin `\Device\HarddiskVolumeN` | — | Passe encore par `\??\GLOBALROOT` ; stabilité de `HarddiskVolumeN` non documentée : rien à y gagner |

Avec A, la session reste ouverte par un chemin à lettre, seule forme que
l'utilisateur donne. Le détail est au §4.1. Les diagnostics continuent de
nommer les entrées relativement au coffre ou à la zone ; aucun GUID
n'apparaît dans une sortie ni dans un journal.

### 3.2 S-02 : aucune lettre qui masque un dossier (T20)

- **Règle.** Une lettre `X:` n'est acceptée que si sa racine est la racine
  d'un volume.
- **Détection, à prouver par HW12.**
  - On ouvre un handle sur `X:\`, sans suivi.
  - Son chemin final en `VOLUME_NAME_GUID` doit valoir exactement
    `\\?\Volume{GUID}\`, sans reste.
  - Contre-épreuve : `GetVolumeNameForVolumeMountPoint("X:\")` doit réussir
    et rendre le même GUID.
  - Un volume sans nom GUID est refusé : c'est un changement de sortie nommé.
- **Où.**
  - Une requête en lecture seule dans `platform/vault`, seul package où
    x/sys est permis.
  - Elle est exposée à `features/sync` par une méthode nouvelle du port
    `Volumes`, changement de port déclaré, et composée par `app` comme
    `Describe`.
  - `platform/filesystem` ne gagne aucune dépendance. Aucune frontière
    nouvelle.
- **Quoi.**
  - Au plan de `sync`, `snapshot` et `restore`, et de leurs récupérations,
    avant toute distinction : la lettre du coffre, de la zone, de
    l'installation, de chaque source et, pour `restore`, de chaque chemin
    sauvegardé lu dans le receipt du snapshot.
  - Une lettre absente garde le comportement actuel.
- **Sous session.** `Open` refait le contrôle sur sa première épingle, la
  racine de la lettre, avant toute autre. La fenêtre entre le plan et
  l'ouverture est fermée pour la cible.
- **Chaînes depuis la vraie racine.**
  - La chaîne de la cible est celle des épingles, dont la première est
    prouvée racine de volume.
  - Chaque chaîne de référence commence à une racine de lettre dont
    l'identité égale celle de `\\?\Volume{GUID}\` (HW12).
- **Raccourci « même lettre » supprimé.**
  - `sync` et `snapshot` comparent le coffre à l'installation et aux sources
    par chaînes d'identités, quelle que soit la lettre.
  - La règle des trois commandes est écrite dans l'amendement d'ADR 0006
    §11.
- **`inspect`.** Il ne fait que lire, et il est hors de la règle. Son
  contrôle de liens part de la racine de la lettre : limite écrite (§6).
- **`doctor`.** Il signale une lettre refusée par un constat (§4.5).
- **Interdiction opérationnelle.** Elle reste en vigueur jusqu'au gate de
  S-02.

### 3.3 Lecteurs (T7, T8, T9)

Aucune frontière ne change. `features/sync` reçoit la lecture stable du
coffre et les budgets. `entities/backup` reçoit le plafond de profondeur
JSON. `platform/filesystem` reçoit la méthode marqueur de N2, en lecture
seule.

### 3.4 `doctor` : une feature et un ADR (T11)

| Option | Mécanisme | Coûts |
| --- | --- | --- |
| **D1, retenue** : feature `features/doctor` | Ports définis par `doctor` ; relevés en lecture seule de `sync`, rendus dans de nouveaux types d'observation de `entities/backup` ; adaptateur étroit dans `app`, comme `syncCommands` | ADR 0010 et amendement d'ADR 0004 ; types nouveaux dans `entities/backup` ; méthodes publiques nouvelles de `sync`, donc un point de contrôle de non-régression ; cas d'imports explicites |
| D2 : opération de `features/sync` | `sync.Service.Doctor`, sur les lecteurs non exportés de `sync` | Le plus court, aucun type nouveau. Mais une cinquième responsabilité dans le package le plus chargé, du code de diagnostic à côté des moteurs d'écriture. Le plan B4 citait `doctor` parmi les critères de réexamen d'ADR 0004 |
| D3 : orchestration dans `app` | `app` compose `sync` et `inspect` | Interdit : `app` compose, il ne possède pas d'orchestration métier (AGENTS.md) |

```text
cmd/savewarden → app ─┬─► cli ─┬─► features/inspect ──► entities/*
                      │        ├─► features/sync ─────► entities/*
                      │        └─► features/doctor ───► entities/*   (ports définis par doctor)
                      ├─► platform/manifests, platform/filesystem
                      └─► platform/vault                              (jamais donné à doctor)
```

- **Types.** Les nouveaux types d'observation vivent dans `entities/backup`,
  sans I/O. Les types existants de `sync` (`Cause`, `Intervention`,
  `Pending`, `VaultState`, `HandedOverRestore`, `PublishedSnapshot`) ne sont
  pas déplacés : les sorties restent intactes par construction.
- **Relevés.** `sync` gagne des méthodes de relevé, en lecture seule. Elles
  exposent ce que ses plans lisent déjà, distinctions et contrôle S-02
  compris, calculés dans `sync` et jamais dupliqués. Aucun moteur d'écriture
  n'est exposé.
- **`inspect`.** `doctor` n'a pas de port vers `inspect` en B5 : les liens et
  les distinctions de l'installation et des sources sont relevés par `sync`,
  qui les calcule déjà.
- **Imports.** Aucune règle d'import ne change : `doctor → sync` est déjà
  refusé. Des cas explicites sont ajoutés : `doctor` vers `sync`, `inspect`,
  `platform/*` et `cli` refusés ; `cli` vers `doctor` permis.
- **Gardes.** `readonly_test.go` couvre la feature sans changement, comme
  tout package. `TestOnlySyncReadsItsPlan` s'étend aux relevés.

### 3.5 ADR à rédiger

| ADR | Objet | Sous-lot |
| --- | --- | --- |
| 0005, amendement | **Paragraphe x/sys**, à côté de `GetVolumeInformation`, et non la table des API d'écriture permises. Il ajoute trois requêtes en lecture seule (`GetFinalPathNameByHandle`, `GetVolumeNameForVolumeMountPoint`, `GetVolumeInformationByHandle`) et une constante locale `0x1` (`VOLUME_NAME_GUID`, `FILE_NAME_NORMALIZED`). Règle : après `Open`, aucun chemin à lettre, vérifiée par une instrumentation interne au package. Une phrase de modèle de menace (§6) | B5a, accepté avant le code |
| 0006 §11, amendement | **S-01** : fenêtre fermée pour les écritures face à une lettre ou un alias réattribué, pas face à un processus hostile du même compte ; ouverture en deux branches ; contrôles sous session sur les chaînes tenues, dans l'ordre du §4.3 ; invalidation au premier défaut de périphérique. **S-02** : lettres qui ne sont pas des racines de volume refusées ; chaînes depuis la vraie racine ; distinction du coffre de `sync` et `snapshot` écrite ici, par identité quelle que soit la lettre. Acceptation provisoire du 4 novembre 2026 levée | B5a, accepté avant le code |
| 0008, amendement | P2-4 : « non vérifiable », portée `snapshot`, codes du plan et de `--apply`, issue | B5b, accepté avant son code |
| 0010, nouveau | `doctor` : frontière, ports, ce qu'il ouvre, niveaux, sortie, table normative des codes par domaine, constats ; deux espaces de noms distincts, codes de constat et causes d'intervention | B5c, accepté avant le code |
| 0004, amendement | Relevés en lecture seule exposés par `sync` ; comparaison D1, D2, D3 ; critère de réexamen inchangé (§13) | B5c, accepté avant le code |
| 0009 | Inchangé (Q3) | — |

---

## 4. Contrats

### 4.1 Session de `platform/vault` après S-01

**Ouverture**, sur le chemin à lettre donné par l'utilisateur :

1. **Racine de la lettre.** Épingle de `X:\`. Son chemin final GUID doit être
   sans reste (S-02, HW12) ; sinon refus, rien écrit.
2. **Ancêtres.** Épinglés comme aujourd'hui, segment par segment, sans
   suivi. Le chemin final de chaque épingle doit égaler celui de la
   précédente suivi du segment (HW3). Une lettre réattribuée pendant `Open`
   donne une chaîne incohérente : refus.
3. **Cible existante.** La cible est épinglée ; la base est son chemin
   final.
4. **Cible absente.** Le parent est épinglé ; un parent absent reste refusé.
   La base est le chemin final du parent suivi du dernier segment, validé
   comme nom Windows et qui n'existe pas encore (HW9).
5. **Tampon et volume.** Le tampon est dimensionné par une première requête,
   et un résultat tronqué est refusé (HW1). Drapeaux : `0x1`. Un volume sans
   chemin GUID est refusé, avec un message nommé (§7.2).

**Création d'une racine absente.** Première écriture, après les contrôles
du §4.3. Elle passe par la base, en création exclusive. La racine est aussitôt
épinglée, son chemin final relu égal à la base, son identité lue par handle,
puis ajoutée à la chaîne tenue.

**Écritures.**

- `CreateDir`, `NewFile`, `Move`, `Hold`, `Identity` et `Lock` joignent la
  base à un nom relatif ; `longPath` ne préfixe plus un chemin qui l'est
  déjà. Aucune lettre n'est résolue après `Open`, ce qu'une instrumentation
  interne au package vérifie en test.
- **Chemin final de chaque nouveau handle** (défense en profondeur, HW8) :
  après `NewFile`, après l'épingle qui suit `CreateDir`, et après `Lock`, le
  chemin final GUID du handle doit égaler la base suivie du nom. Sinon la
  session est invalidée et l'opération refusée. L'empreinte d'un masquage se
  réduit alors à un objet vide à l'endroit masqué (§6).

**Requêtes en lecture seule ajoutées à `Session`.** C'est un changement
d'interface, déclaré, de `Session` et du port `Vault` de `sync`.

- **Chaîne tenue** : les identités des épingles, de la racine du volume à la
  cible ou à son parent, lues par handle.
- **Volume tenu**, en remplacement de `Describe` sous session :
  - système de fichiers par `GetVolumeInformationByHandle` sur l'épingle
    racine ;
  - type de lecteur et espace libre par `\\?\Volume{GUID}\` (HW10).

  `qualify` porte sur ce volume. `Describe` reste appelée au plan, sur la
  lettre, après le contrôle S-02.
- **Lectures de décision**, par la base et sans suivi (liste au §4.3).

**Erreurs.** `os.NewFile` reçoit le nom relatif. Aucune erreur de
`platform/vault` ne contient la base ni un GUID. Les messages de `Close` et
de `Describe` nomment un chemin relatif, ou le chemin à lettre de
l'utilisateur au plan.

**Invalidation.**

- La première erreur de périphérique invalide la session : volume
  indisponible, au sens des codes classés aujourd'hui et de ceux qu'observe
  HW7. Toute écriture suivante est alors refusée, classée « volume
  indisponible ».
- Avant chaque `Move` et chaque enregistrement validé du journal, la
  vivacité de l'épingle racine est contrôlée : même identité, lue par
  handle.
- Un volume retiré puis réinséré laisse des handles morts. La session
  s'invalide alors, sans écrire sur le nouveau montage (HW7b).

**Inchangé** : les API d'écriture, les drapeaux, le classement `BusyError`,
`NotFollowedError` et `fs.ErrExist`.

### 4.2 S-02 : refus et chaînes

- **Refus.** Coffre, zone, installation, source ou chemin sauvegardé dont la
  lettre n'est pas une racine de volume : code 1, rien écrit, avant toute
  autre distinction. Le message nomme l'objet et la lettre, et dit de
  désigner le volume par sa propre lettre. Les textes exacts sont fixés au
  commit et listés au §7.2.
- **Recouvrements entre lettres.** Les cas que le raccourci « même lettre »
  laissait passer sont refusés comme recouvrements : coffre et installation
  ou source sur deux lettres d'un même arbre.
- **Récupérations.** `sync --recover`, `snapshot --recover` et
  `restore --recover` appliquent les mêmes refus, au plan et sous session.

### 4.3 Contrôles sous session (T4)

Pour `sync --apply`, `snapshot --apply` et `restore --apply`, et pour chaque
`--recover --apply`, reprise du marqueur comprise : après `Open`, **avant
toute écriture, création du verrou comprise**, dans cet ordre.

1. **Racine de volume** : fait par `Open` (§4.1).
2. **Chaîne tenue égale à la chaîne du plan.**
   - Cible existante : son identité égale celle du plan.
   - Cible absente : l'identité du parent égale celle du plan.
   - Une cible absente au plan et présente sous session donne le code 1,
     sauf les reprises que les règles actuelles prévoient (Z3 de B4).
3. **Chaînes de référence relues.** Installation, sources et, pour
   `restore`, coffre et chemins sauvegardés. Elles sont relues après le
   contrôle S-02 de leur lettre et doivent égaler celles du plan. Cela
   détecte une lettre de référence réattribuée entre le plan et la session.
4. **Recouvrements, dans les deux sens.**
   - Aucune identité de référence ne figure dans la chaîne tenue : la cible
     n'est pas dans une référence.
   - L'identité de la cible, ou celle du parent tenu si la cible est
     absente, ne figure dans aucune chaîne de référence : aucune référence
     n'est dans la cible.
   - Références de `sync` et `snapshot` : installation et sources. Celles de
     `restore` : coffre, installation, sources actuelles et chemins
     sauvegardés.
5. **Lectures de décision sur les objets tenus**, par la base de la session
   et sans suivi :
   - marqueurs des ancêtres ;
   - Z3, « encore vide », lu sur l'objet tenu ;
   - relecture du marqueur du coffre ou de la zone.
6. **Observation impossible** : code 1, rien écrit.

Le plan garde les chaînes en mémoire ; le format du journal ne change pas.
Les contrôles de Z2 à Z6 de `restore` passent sur les chaînes tenues.

### 4.4 Lecteurs

- **Lecture stable (T7).** Si la fenêtre est reproduite, elle s'applique au
  domaine courant, au domaine des snapshots et aux plans de récupération.
  - Le plan lit jusqu'à deux lectures identiques, trois au plus. Un coffre
    qui change encore : code 1, « another run may be writing this vault »,
    rien conclu.
  - Le verrou est sondé par une sonde indépendante de la liste, comme pour
    la zone, dès qu'une opération en attente ou une intervention est vue.
  - `--apply` est inchangé, sous son propre verrou.
- **Budgets (T8).**
  - **Portée.** Métadonnées chargées en mémoire : noms listés,
    enregistrements, receipts, marqueurs. Le budget vaut par lecture :
    chacune des trois lectures stables a le sien.
  - **Hors budget.** Un contenu vérifié contre son receipt (`--verify`,
    relecture de `current`) n'est jamais compté. Il est haché en flux, borné
    par les tailles et les nombres de son receipt ; un contenu plus grand
    est une différence.
  - **Dimensionnement.** Au moins dix fois une décennie d'usage quotidien,
    soit 36 500 opérations par domaine. Chacune laisse au plus un journal, un
    receipt, une génération conservée et un staging, plus des essais.

  | Budget | Valeur | Dérivation |
  | --- | --- | --- |
  | Entrées d'un dossier listé | 262 144 | Plus de sept fois les 36 500 entrées d'un dossier qui reçoit une entrée par opération ; les essais restent marginaux, un essai ne subsistant qu'après une interruption |
  | Enregistrements d'un journal | Inchangé | Déjà borné par l'enveloppe E2 : 11 enregistrements, huit essais au plus à côté de chacun |
  | Octets lus par fichier | Inchangé | 64 Mio par enregistrement ou receipt |
  | Octets de métadonnées par lecture | **Mesurée en M1** | Coffre synthétique de 36 500 opérations, mémoire et temps mesurés ; valeur consignée par erratum et acceptée avant le code des budgets et avant ADR 0010 (écart du §0.3) |

  - **Épuisement.** Code 1 pour les commandes, message distinct : « vault
    too large to read within the budget; create a new vault ». Rien n'est
    conclu, récupération comprise. Pour `doctor`, c'est un constat du
    domaine (§4.5).
  - **Noms hors grammaire.** Jamais lus. Un nom hors grammaire dans un
    journal reste `journal-ambiguous`, sans être lu : aucune cause ne change.
- **P2-4 et N2** : selon le §0.2.

### 4.5 `doctor`

```text
savewarden doctor --manifest <m> --vault <v> [--area <dest>]... [--verify]
```

| Objet | Structurel, par défaut | Avec `--verify` |
| --- | --- | --- |
| Manifeste | Lecture stricte, ADR 0002 | — |
| Lettres | Celles du coffre, des zones, de l'installation et des sources sont des racines de volume (S-02) | — |
| Installation et sources | Existence, liens, distinctions par chaînes d'identités depuis la racine du volume, relevées par `sync` | — |
| Coffre | Marqueur ; journaux et automate ; receipts stricts et leurs chaînes ; identités publiées ; `staging`, `.incomplete`, enveloppe | `current`, chaque génération conservée et chaque snapshot publié relus en flux et comparés à leur receipt |
| Zone, par `--area` | Marqueur strict du même jeu ; journaux ; receipts ; `.incomplete` ; restaurations remises observées par nom et par identité, sans suivi, l'identité lue par un handle refermé aussitôt (ADR 0009) | Rien de plus : une restauration remise n'est jamais ouverte pour son contenu, listée ni lue (Q3) |

**Ce que `doctor` ouvre** (ADR 0010) :

- **listes** du coffre, de ses domaines et des zones, par `ReadDir`, qui
  rend nom et type sans ouvrir les entrées (HD1) ;
- **noms de verrou** (`sync.lock`, `restore.lock`) : classés par l'entrée de
  répertoire, jamais ouverts ni passés à `Lstat`. Si leur type ne peut être
  connu sans ouverture, c'est un constat, sans ouverture. Les relevés de
  `sync` suivent la même règle, sans changer de sortie ;
- **marqueurs, enregistrements et receipts** aux noms conformes : ouverts en
  lecture, partage lecture, écriture et suppression, refermés aussitôt ;
- **identités** : un handle sans suivi, même partage, refermé aussitôt ;
- **contenu sous `--verify`** : même partage, lu en flux ;
- **jamais** : une épingle tenue d'une lecture à l'autre, une session, un
  verrou, une sonde.

Si HD2 montre qu'une ouverture en partage complet fait encore échouer le
renommage de `current` par une rotation concurrente, le résidu est écrit. La
documentation dit de lancer `doctor --verify` hors de toute écriture. La
rotation échoue alors en `rolled back`, code 1, sans dommage.

**Table normative des codes**, par domaine, fixée par ADR 0010.

- **Ordre de sévérité** : `0` < `2` < `3`. `doctor` n'est jamais moins
  sévère que la commande qui agirait sur le même état.
- **Refus d'usage.** Un refus d'usage de cette commande (son code 1) devient
  un constat de code 2.
- **Code 1 de `doctor`.** Réservé à l'absence de rapport : erreur d'usage,
  manifeste illisible, coffre absent, non marqué ou illisible à sa racine.
- **Rapport.** stdout pour le rapport, stderr pour les erreurs, selon ADR
  0003 : jamais de rapport partiel avec le code `0`.

| Domaine | Constat | Code | Commande de référence |
| --- | --- | --- | --- |
| Coffre, `current` | Sain ; staging d'une opération close | 0, information | `sync` : 0 |
| Coffre, `current` | Opération en attente, non départagée | 3 ; action : `sync --recover`, sans `--apply`, qui sonde le verrou. Un code 3 peut donc signifier « en cours » | `sync` : 3, ou 1 si le verrou est tenu |
| Coffre, `current` | `unexpected-entry`, `journal-ambiguous`, enveloppe épuisée | 3 | `sync` : 3 |
| Coffre, `current` | `current` ou génération conservée différents de leur receipt (`--verify`) | 2 ; 3 si une opération en attente la concerne | Récupération : 3 |
| Coffre, `current` | Cas de la tolérance de B2 (Q5) | 2, limite nommée | — |
| Snapshots | Snapshot sain ; dégradation déjà consignée | 0, information ; « non restaurable » dit pour un snapshot dégradé | `snapshot` : information |
| Snapshots | Différence nouvelle au receipt (`--verify`) | 2 | `restore` de ce snapshot : refus |
| Snapshots | `.incomplete` d'une opération close | 0, information | — |
| Snapshots | `.incomplete` inexpliqué ; `snapshots\receipts` absent à côté d'opérations | 3 ; action : recréer le dossier vide, pour le second | `snapshot` : 3 |
| Snapshots | « Non vérifiable » (P2-4) | 2 ; action : rendre le fichier lisible | `snapshot` : 1 |
| Zone | Marqueur d'un autre jeu | 2, constat de cette zone ; le rapport continue | `restore` : 1 |
| Zone | Receipt d'une restauration publiée absent ; journal invalide ; `.incomplete` inexpliqué | 3 | `restore` : 3 |
| Zone | `.incomplete` d'une opération close ; restaurations remises présentes, absentes ou d'une autre identité | 0, information | — |
| Chemins | Lettre refusée (S-02) ; recouvrement ; lien | 2 | Commandes d'écriture : 1 |
| Tout domaine | Budget épuisé | 2 pour ce domaine, rien conclu sur lui | Commandes : 1 |

- **Constats.** Chaque constat porte un code stable documenté, l'objet
  relatif au coffre ou à la zone, et l'action à mener ; jamais une
  suppression.
- **Causes d'intervention.** Celles de `sync` (`journal-ambiguous`…) sont
  reprises comme causes, dans leur propre espace de noms, à côté du code du
  constat.
- **Limite.** `doctor` voit l'état au moment de sa lecture ; la lecture
  stable le borne, sans le figer.

### 4.6 Build de contrôle en snapshot (T17)

- **Outil.**
  - `go run github.com/goreleaser/goreleaser/v2@vX.Y.Z`, à version exacte,
    vérifiée par la base de checksums de Go et déjà acceptée par le
    vérificateur de pins.
  - La compatibilité avec Go 1.27.3 et `GOTOOLCHAIN=local` est prouvée dans
    le commit.
  - À défaut : action épinglée par SHA complet, version exacte, vérification
    de checksum et extension du vérificateur, dans la même PR, sur arbitrage
    du propriétaire.
- **`.goreleaser.yaml`.**
  - Build : `windows/amd64` seul, `CGO_ENABLED=0`, `-trimpath`, ldflags
    limités à `-X main.version={{.Version}}`,
    `mod_timestamp: "{{ .CommitTimestamp }}"`.
  - Sorties : archive zip et `checksums.txt`.
  - `release: disable: true` ; changelog désactivé ; aucun hook.
- **Job « Release snapshot ».**
  - Sur `windows-2025`, qui exécute le smoke. `contents: read`, aucun
    secret, **aucun artefact**.
  - Commandes : `goreleaser check`, puis `goreleaser release --snapshot
    --clean`. `--snapshot` implique déjà `--skip=announce,publish,validate`.
  - Le job imprime les checksums dans son journal.
- **Smoke.**
  - Il extrait l'archive. `savewarden version` doit rendre la version de
    snapshot attendue, au gabarit fixé au commit, et `savewarden --help`
    être déterministe.
  - `go version -m` doit montrer `-trimpath`, `CGO_ENABLED=0`,
    `vcs.modified=false` et les seuls ldflags permis. Les checksums sont
    vérifiés.
  - Le script est versionné sous `.github/scripts`.
- **Vérificateur de pins étendu.** Il refuse `upload-artifact`, toutes
  versions, ainsi que `contents: write`, `packages: write`,
  `id-token: write`, `attestations: write` et les hooks GoReleaser. Des
  fixtures négatives sont testées.
- **Documentation.** `CONTRIBUTING.md` et `gitops.md` distinguent deux
  choses :
  - le **build de contrôle en snapshot** : sans artefact, sans SBOM, sans
    attestation ;
  - le **pipeline de release** : artefacts, SBOM et attestations, au gate de
    publication.
- **Go de production.** Aucun changement en B5d, prouvé par le diff contre
  la fusion de B5c.
- **Check requis.** Ajouté par le propriétaire après un run vert : exception
  écrite au §7.3, `gitops.md` mis à jour.

### 4.7 Compatibilité

Aucun format ne change : receipt v1, receipt de snapshot v1, receipt de
restauration v1, marqueurs v1, enregistrements de journal. Un binaire de
`a59d787` lit un coffre et une zone écrits par B5 avec les mêmes sorties. Le
différentiel binaire le vérifie, dans les deux sens.

---

## 5. Hypothèses Windows, à prouver avant le code (T3)

### 5.1 B5a : HW1 à HW12

Tests réels dans `platform/vault`, sur `windows-2025` en CI et sur le poste
de développement, dans des dossiers temporaires. Lettres `subst` et noms DOS
créés et retirés par le test seulement. Exceptions : HW8 tourne en CI
seulement, HW7 est manuel.

| # | Hypothèse | Preuve attendue |
| --- | --- | --- |
| HW1 | `GetFinalPathNameByHandle(0x1)` sur un dossier épinglé rend `\\?\Volume{GUID}\…` sur un volume fixe ; par une lettre `subst`, le chemin dans le volume réel. Un tampon trop petit est détecté par la requête de taille | Chemin obtenu ; rouvert, même identité ; troncature refusée |
| HW2 | `CreateDirectory`, `CreateFile` en `CREATE_NEW`, `MoveFileEx` avec `0` et `GetFileInformationByHandle` acceptent ces chemins avec la même sémantique | Erreur 183 pour `CreateDirectory` et `MoveFileEx` sur cible existante, erreur 80 pour `CreateFile` ; erreur 17 entre volumes, testée hors de `Session` ; état inchangé |
| HW3 | Chaque préfixe du chemin de volume, rouvert sans suivi, désigne l'identité de l'épingle correspondante ; une lettre réattribuée pendant `Open` est détectée | Identités égales ; chaîne incohérente refusée |
| HW4 | Une lettre `subst` retirée puis réattribuée vers un autre dossier pendant une session ouverte : les créations et renommages suivants restent dans la cible tenue | Empreinte du témoin identique ; objets créés dans la cible |
| HW5 | Les épingles prises par chemin de volume interdisent toujours le renommage d'un dossier tenu ; un handle ouvert par lettre et un handle ouvert par chemin de volume portent sur le même objet, avec les mêmes conflits de partage | Renommage refusé ; conflits identiques |
| HW6 | Chemins longs et noms Unicode derrière ce préfixe | Création au-delà de `MAX_PATH` |
| HW7 | (a) Volume amovible retiré pendant une session : appels en échec, classés « volume indisponible », jamais redirigés. (b) Retiré puis réinséré : handles morts, session invalidée, rien écrit sur le nouveau montage. (c) Chemin GUID rouvert pendant l'indisponibilité : échec classé | Codes observés consignés. **Test manuel du propriétaire**, autorisé à part et nominativement, sur données synthétiques, sans coffre de pilote ; jamais présenté comme passé en CI. Un VHD sur runner élevé est étudié, non exigé |
| HW8 | Un nom DOS local `Volume{GUID}`, défini par `DefineDosDevice` dans la session de connexion du test, masque-t-il le nom global pour `\\?\Volume{GUID}\…` ? | **CI seulement.** Masquage possible : résidu écrit (§6), et le contrôle du chemin final refuse. Impossible : consigné, contrôle gardé en défense |
| HW9 | Cible absente : base égale au chemin du parent tenu suivi du segment ; racine créée en exclusif par cette base, épinglée, chemin final égal à la base, identité lue par handle | Première publication B2b, coffre sans `games`, Z2, reprise Z3, zone existante, récupérations |
| HW10 | `GetVolumeInformationByHandle` sur l'épingle racine rend le système de fichiers ; `GetDriveType` et `GetDiskFreeSpaceEx` acceptent `\\?\Volume{GUID}\` | Mêmes valeurs que par la lettre |
| HW11 | Un point de montage de volume parmi les ancêtres est refusé comme point d'analyse, par lettre et par chemin de volume | Refus `NotFollowedError` ; en CI si le runner permet de créer le montage, sinon limite écrite |
| HW12 | Détection S-02 : le chemin final GUID du handle de `X:\` est sans reste pour une vraie lettre de volume, et avec reste pour un alias `subst` et pour un alias `DefineDosDevice` créé par le test. `GetVolumeNameForVolumeMountPoint` échoue pour ces deux alias. L'identité de `X:\` égale celle de `\\?\Volume{GUID}\` pour une vraie lettre | Les trois cas ; contre-épreuve concordante |

La réattribution d'une vraie lettre de volume (`mountvol`) demande des
droits d'administrateur et un volume de test : elle n'est pas proposée en
CI. HW4 la représente par `subst`, la voie démontrée par la revue de sécurité
de B4. `subst` agit sur toute la session de connexion : aucun test qui crée
une lettre ne tourne pendant la campagne (§11).

### 5.2 B5c : HD1 et HD2

| # | Hypothèse | Preuve attendue |
| --- | --- | --- |
| HD1 | `ReadDir` rend nom et type des entrées sans les ouvrir : un `Lock` sans partage pris en boucle par un tiers sur `sync.lock` réussit pendant que `doctor` liste `games\<id>` | Aucun échec de `Lock` sur mille itérations |
| HD2 | Une ouverture en partage lecture, écriture et suppression, d'une identité ou d'un fichier de `current`, ne fait échouer ni `Hold`, ni le `Move` de `current` d'une rotation concurrente | Établi, ou résidu écrit (§4.5) |

---

## 6. Non-écrasement et limites

Inchangés : créations exclusives, renommage sans remplacement, aucune
suppression, aucun lien suivi, journal avant chaque action.

Limites qui restent après B5 :

- **Espace de noms local** (selon HW8).
  - `\\?\Volume{GUID}` est résolu par `\??\`, qui cherche d'abord les noms
    DOS de la session de connexion. Un processus du même compte peut définir
    un nom local `Volume{GUID}` sans privilège.
  - **Modèle de menace** : Savewarden protège contre une lettre ou un alias
    réattribué par accident ou par un outil bénin, `subst` ne sachant
    définir qu'une lettre. Il ne protège pas contre un processus hostile du
    même compte, qui peut de toute façon écrire dans le coffre.
  - Le contrôle du chemin final de chaque nouveau handle réduit l'effet d'un
    masquage à un objet vide à l'endroit masqué.
- **Lectures.**
  - Les lectures de `platform/filesystem` résolvent encore des lettres :
    installation, sources, coffre et zone au plan, relectures sous verrou,
    relecture de vérification, porte de provenance. Une lettre réattribuée
    pendant une lecture fait lire un autre contenu.
  - Les gardes existants devraient refuser un contenu divergent : inventaire
    fait deux fois avec contrôle des identités, copie hachée contre le plan,
    relecture du snapshot. Mais un mauvais objet lu, un résultat incohérent
    ou une indisponibilité peuvent produire un autre diagnostic.
  - **Confidentialité.** Une lecture involontaire lit et hache le contenu
    d'un autre dossier. Une copie peut le placer dans un staging du coffre
    avant le refus, où il reste, rien n'étant supprimé.
  - Le confinement des écritures n'implique pas celui des lectures. Les
    décisions prises sous session portent sur les objets tenus (§4.3).
- **Volume entier.**
  - Le chemin GUID suit le volume, pas la lettre.
  - Un volume retiré puis réinséré pendant une session laisse des handles
    morts : la session s'invalide, rien n'est écrit sur le nouveau montage
    (§4.1, HW7).
  - Entre deux exécutions, un volume remonté ailleurs est le même volume
    pour le plan suivant, qui refait tous ses contrôles.
- **Alias.** Refusés en B5 ; leur acceptation est reportée (§13). Le
  contrôle de liens d'`inspect`, qui ne fait que lire, part de la racine de
  la lettre : les dossiers au-dessus de la cible d'un alias ne sont pas
  contrôlés.
- **`doctor`** lit un état qui peut changer pendant sa lecture. La lecture
  stable le borne sans le figer ; résidu de HD2 s'il est établi.
- **Tolérance de B2** : limite datée (Q5).
- Toutes les limites de B4 restent : restauration remise non suivie, coffre
  copié refusé, falsification coordonnée hors modèle.

---

## 7. Périmètre des fichiers

### 7.1 Par sous-lot

| Zone | B5a | B5b | B5c | B5d |
| --- | --- | --- | --- | --- |
| `internal/platform/vault` | Requête S-02 ; base de volume ; ouverture en deux branches et contrôle de chaîne ; chemin final des handles ; invalidation ; chaîne tenue, volume tenu et lectures de décision ; erreurs sans base ; commentaires (T6) ; tests HW1 à HW12 ; instrumentation | — | — | — |
| `internal/app` | Composition de la méthode nouvelle de `Volumes`. `readonly_test.go` : ses sélecteurs d'écriture restent inchangés, les requêtes ajoutées n'étant pas des écritures ; un droit d'accès en lecture nouveau, s'il en faut un pour ouvrir `X:\`, n'est ajouté qu'avec l'amendement d'ADR 0005 | — | `app.go`, composition de `doctor` par un adaptateur étroit ; `architecture_test.go`, cas explicites | — |
| `internal/features/sync` | Contrôles sous session (T4) ; refus S-02 et suppression du raccourci ; chaînes ; ports `Volumes` et `Vault` | Lecture stable, budgets, P2-4, verrou de récupération signalé ; noms de la matrice B4 | Relevés en lecture seule ; noms de verrou classés sans ouverture | — |
| `internal/features/doctor` | — | — | Feature, ports, rapport, tests | — |
| `internal/entities/backup` | — | Plafond de profondeur JSON | Types d'observation | — |
| `internal/platform/filesystem` | — | Méthode marqueur de N2 | — | — |
| `internal/cli` | — | — | `doctor.go`, tests ; `root.go` | — |
| `cmd/savewarden` | Tests binaires réels S-02 et de réattribution ; différentiel, erreurs comprises | Différentiel | Tests binaires Windows de `doctor`, concurrence comprise | — |
| `.github` | — | — | — | Job « Release snapshot » ; vérificateur étendu et fixtures ; script de smoke |
| Racine | `CHANGELOG.md` | `.gitignore`, `CHANGELOG.md` | `README.md`, `CHANGELOG.md` | `.goreleaser.yaml`, `CONTRIBUTING.md`, `CHANGELOG.md` |
| `docs/` | ADR 0005 et 0006 ; `safety-model.md`, S-01 et S-02 fermés ; `commands/sync.md`, `snapshot.md`, `restore.md`, `inspect.md` ; `validation/b5a.md` | ADR 0008 ; `vault.md` ; `validation/b5b.md` | ADR 0010, amendement 0004 ; `commands/doctor.md` ; `architecture.md` ; `validation/b5c.md` | `gitops.md` ; `pilots/odyssey.md` ; `validation/b5d.md` |

PR documentaire de T21, avant B5a : ce plan ; `AGENTS.md`, lot courant
B5 ; `docs/architecture.md`, qui dit encore « B4, en développement » ;
`docs/validation/b4.md`, dont « Points soumis au propriétaire » et les
limites listent encore la PR de clôture et le Human GO comme restants.

### 7.2 Changements de sortie nommés

Seuls changements de sortie admis, exclus du différentiel. Tout autre écart
fait échouer T5.

- **B5a.**
  - Refus d'une lettre qui n'est pas une racine de volume : coffre, zone,
    installation, source, chemin sauvegardé.
  - Refus des recouvrements entre lettres.
  - Refus d'un volume sans chemin GUID.
  - Refus à l'ouverture : chaîne incohérente, racine de lettre devenue
    alias.
  - Nouveaux refus sous session de `sync` et `snapshot` (T4).
  - Refus après un chemin final hors de la base.
  - Session invalidée après un défaut de périphérique.
  - Erreurs de `platform/vault` qui nomment un chemin relatif au lieu du
    chemin complet.
- **B5b.**
  - Lecture stable : « another run may be writing this vault ».
  - Budget épuisé.
  - « Non vérifiable » de P2-4.
  - Verrou de la récupération de `current` signalé.
- **B5c.** La commande `doctor`, nouvelle.

### 7.3 Changements interdits dans tout B5

- Toute API d'écriture, de suppression ou de remplacement nouvelle ; tout
  `MOVEFILE_*` ; `NtCreateFile`, `SetFileInformationByHandle` et `unsafe`,
  sauf arbitrage de l'option B par le propriétaire et amendement préalable.
- Toute acceptation d'un alias de lettre.
- Pour `doctor` : toute écriture, session, épingle, verrou ou sonde de
  verrou, toute ouverture de `sync.lock` ou `restore.lock`, toute lecture du
  contenu d'une restauration remise.
- Tout changement de format : manifeste v1, receipts, marqueurs, journal.
- Tout changement de sortie hors du §7.2.
- Tout changement du Go de production en B5d.
- Tout chemin personnel ou propre au pilote dans le code, les tests, les
  exemples ou les rapports.
- Toute modification des rulesets, sauf l'ajout du check « Release
  snapshot » par le propriétaire après un run vert.
- Tout tag, toute release, toute publication, tout artefact de workflow,
  tout hook GoReleaser ; tout secret, et toute permission d'écriture, dans
  un workflow.
- Tout accès agent à `D:\Ga`, `D:\Savewarden`, `E:\GMS`, à un coffre ou une
  zone de pilote, ou aux clés SSH.
- Tout test lisant ou écrivant hors de `t.TempDir()`, hormis les lettres et
  noms DOS que le test crée et retire.

---

## 8. Plan d'exécution

Avant tout code :

1. revue ciblée de cette v0.2 par le propriétaire et le CTO, avis sur S-02
   compris, et acceptation, en v0.3 si des corrections le demandent ;
2. PR documentaire de T21 ;
3. GO de développement distinct.

Les amendements d'ADR s'appuient sur les résultats HW : ils sont rédigés
après eux et acceptés avant tout code de production. Chaque sous-lot part
du commit de fusion du précédent.

Bases des différentiels :

- B5a contre le binaire de `a59d787` ;
- B5b contre la fusion de B5a ;
- le point de contrôle du commit 17 contre la fusion de B5b ;
- B5d par le diff du Go de production, qui doit être vide.

| # | Sous-lot | Commit | Point de contrôle |
| --- | --- | --- | --- |
| 0a | — | v0.2 revue et acceptée | **Acceptation par le propriétaire** |
| 0b | — | PR documentaire (T21) | **Human GO de fusion**, puis GO de développement |
| 1 | B5a | Tests réels HW1 à HW12, sans code de production ; protocole de HW7 pour le propriétaire | **Résultats HW revus** ; une hypothèse tombée arrête B5a |
| 2 | B5a | Amendements d'ADR 0005 et 0006 §11, rédigés avec les résultats HW | **Acceptation par le propriétaire** avant le commit 3 |
| 3 | B5a | T4 seul : chaîne tenue et lectures de décision en lecture seule ; chaînes du plan en mémoire ; contrôles sous session avant toute écriture, pour `sync`, `snapshot`, `restore` et toutes les récupérations ; tests sur faux et réels | — |
| 4 | B5a | S-02 : requête de racine de volume ; refus ; chaînes depuis la vraie racine ; raccourci « même lettre » supprimé ; tests sur faux et réels des deux côtés, pour les trois commandes et leurs récupérations | — |
| 5 | B5a | Base de volume : ouverture en deux branches, contrôle de chaîne, chemin final des handles, invalidation, volume tenu, erreurs sans base ; commentaires (T6) | — |
| 6 | B5a | Tests réels de réattribution pendant une session ; requalification de B2 à B4 ; différentiel contre `a59d787`, erreurs comprises | — |
| 7 | B5a | Rapport `docs/validation/b5a.md` ; S-01 et S-02 fermés dans le modèle de sécurité ; documentation des commandes | **Revue de sécurité indépendante, obligatoire**, puis gate de S-01 et S-02 et GO de fusion |
| 8 | B5b | Noms de la matrice B4 (T10), tests seuls | — |
| 9 | B5b | M1 : mesure sur un coffre synthétique, benchmark hors CI ; valeur consignée par erratum | **Valeur acceptée** par le propriétaire |
| 10 | B5b | Amendement d'ADR 0008 | **Acceptation** avant le commit 13 |
| 11 | B5b | T7 : fenêtre reproduite sur faux pour les deux domaines et les plans de récupération ; correction ou preuve consignée | — |
| 12 | B5b | Budgets (T8) | — |
| 13 | B5b | P2-4 et N2 | — |
| 14 | B5b | Profondeur JSON, verrou de récupération signalé, `.gitignore` ; rapport `b5b.md` | **Revue indépendante de non-régression**, avec un **regard de sécurité sur N2 et P2-4**, puis GO de fusion |
| 15 | B5c | ADR 0010 et amendement d'ADR 0004 | **Acceptation par le propriétaire** |
| 16 | B5c | Tests réels HD1 et HD2, sans code de production | **Résultats revus** |
| 17 | B5c | Types d'observation ; relevés en lecture seule de `sync` ; noms de verrou classés sans ouverture | **Revue de non-régression** et différentiel de `sync`, `snapshot`, `restore` et `inspect` contre la fusion de B5b |
| 18 | B5c | Feature `doctor`, structurel | — |
| 19 | B5c | `--verify` ; zones ; CLI et composition ; tests d'imports | — |
| 20 | B5c | Tests Windows réels et binaires, concurrence comprise ; documentation ; rapport `b5c.md` | **Revue indépendante** de la lecture seule, de la non-interférence et des constats, puis GO de fusion |
| 21 | B5d | Vérificateur étendu et fixtures ; `.goreleaser.yaml` ; job « Release snapshot » ; smoke ; CONTRIBUTING et `gitops.md` | **Revue indépendante CI et chaîne d'approvisionnement** |
| 22 | B5d | Campagne pilote par le propriétaire (§11) ; rapport `b5d.md` ; CHANGELOG | GO de fusion, puis Human GO de clôture de B5 |

Chaque commit passe la suite complète à son SHA, dans une extraction propre.
Push, PR, fusion, campagne, HW7 et tout changement de ruleset attendent
chacun un GO distinct du propriétaire.

---

## 9. Matrice de tests

Noms proposés, fixés au commit de chaque partie. Faux système de fichiers et
faux coffre sur les trois OS, sauf mention « Windows réel ». En CI, les tests
de différentiel et de course échouent s'ils ne sont pas exercés
(`GITHUB_ACTIONS=true`), comme `notExercised` le fait en B4.

**B5a, S-01 et S-02**

| Test | Preuve |
| --- | --- |
| `TestLetterRootIsAVolumeRoot` (Windows réel) | HW12 |
| `TestAliasLetterIsRefused` (Windows réel, binaire) | S-02. Alias `subst` d'un sous-dossier strict de l'installation, d'une source, du coffre et du dossier du jeu dans le coffre. Il est employé comme `--vault` ou `--to`, ou dans le manifeste (installation par un alias de son parent, source). `sync`, `snapshot`, `restore` et chaque `--recover --apply` refusent, code 1, rien créé, empreintes des témoins identiques. Témoins : vraies lettres acceptées |
| `TestVaultOnAnotherLetterIsComparedByIdentity` | Raccourci supprimé : coffre et installation désignés par deux lettres d'un même arbre, refusé |
| `TestChainsStartAtTheVolumeRoot` (faux et Windows réel) | Première identité de chaque chaîne : la racine du volume |
| `TestSessionWritesThroughTheVolumePath` (Windows réel) | HW1 à HW3 ; l'instrumentation ne voit aucun chemin à lettre après `Open` |
| `TestVolumePathSemantics` (Windows réel) | HW2 : 183, 80 ; 17 hors de `Session` |
| `TestOpenRefusesAnIncoherentChain` (Windows réel) | HW3 : lettre réattribuée pendant `Open`, refus |
| `TestAbsentTargetIsCreatedUnderTheHeldParent` (faux et Windows réel) | HW9 : première publication B2b, coffre sans `games`, Z2, reprise Z3, zone existante, récupérations |
| `TestLetterRemappedDuringTheSession` (Windows réel) | HW4 |
| `TestPinsHoldThroughTheVolumePath` (Windows réel) | HW5 |
| `TestVolumePathLongAndUnicodeNames` (Windows réel) | HW6 |
| `TestLocalVolumeNameShadowing` (Windows réel, CI seulement) | HW8 |
| `TestNewHandleOutsideTheBaseIsRefused` | Chemin final hors de la base : refus, session invalidée |
| `TestSessionIsInvalidatedAfterADeviceError` | Invalidation et vivacité de l'épingle racine ; HW7 manuel |
| `TestSessionVolumeIsDescribedByHandle` (Windows réel) | HW10 |
| `TestMountPointAmongAncestorsIsRefused` (Windows réel) | HW11, si le runner le permet |
| `TestSessionErrorsNameNoVolumePath` (Windows réel, binaire) | Erreur d'écriture forcée : ni stderr ni l'enregistrement `failed` ne contiennent `\\?\Volume{` ou la base |
| `TestChecksUnderTheSessionUseTheHeldChain` | T4 : recouvrements parent et enfant dans les deux sens, références historiques, zone déplacée, observation impossible ; code 1, rien écrit ; pour `sync`, `snapshot`, `restore` et les récupérations |
| `TestRestoreRefusesALetterRemappedAfterTheSession` (Windows réel, binaire) | Aucune écriture hors de la zone tenue |
| `TestSyncRefusesALetterRemappedBeforeTheSession`, `TestSnapshotRefusesALetterRemappedBeforeTheSession` (Windows réel, binaire) | T4 : lettre du coffre réattribuée entre le plan et l'ouverture ; code 1, **rien écrit, verrou compris**, témoin identique |
| `TestOutputsUnchangedSinceB4` | Différentiel normalisé contre un golden enregistré depuis `a59d787`, succès et erreurs, hors §7.2 |

**B5b, lecteurs**

| Test | Preuve |
| --- | --- |
| `TestVaultIsReadUntilStable`, `TestSnapshotDomainIsReadUntilStable`, `TestRecoveryPlansAreReadUntilStable` | Fenêtre reproduite sur faux avant correction, fermée après ; vérifiée par mutation |
| `TestVaultThatKeepsChangingIsRefused` | Trois lectures différentes : code 1, rien conclu |
| `TestLockProbeDoesNotDependOnTheListing` | Sonde faite même si la liste n'a pas vu le verrou |
| `TestReadBudgetIsPerRead` | Chaque lecture stable a son budget ; au-delà : code 1, rien lu au-delà |
| `TestVerifiedContentIsNeverBudgeted` | Relecture de `current` d'une taille au-delà du budget de métadonnées : acceptée |
| `TestNonConformingJournalNamesStayAmbiguous` | Nom hors grammaire dans un journal : `journal-ambiguous`, jamais ouvert |
| `TestUnreadableSnapshotIsNotModified` | P2-4 : « non vérifiable », code 1 au plan et à `--apply`, aucune dégradation, aucun second snapshot |
| `TestVolumeNotReadyOnlyChangesTheGate` | N2 : la porte classe la preuve comme indisponible ; `inspect` et `restore` inchangés |
| `TestJSONDepthIsCapped` | Plafond indépendant du décodeur ; message avec « depth » |
| `TestCurrentRecoveryReportsItsLock` | Verrou créé par la récupération de `current`, signalé |
| `BenchmarkVaultReadAtScale` (hors CI) | M1 : mémoire et temps sur 36 500 opérations |
| Six tests nommés selon la matrice B4 | T10 : mêmes assertions |

**B5c, `doctor`**

| Test | Preuve |
| --- | --- |
| `TestDoctorWritesNothing` (Windows réel, binaire) | Deux exécutions sous ACL de refus d'écriture sur tout l'arbre : sorties et empreintes identiques |
| `TestDoctorNeverOpensLockFiles` (Windows réel) | HD1 ; aucune ouverture de `sync.lock` ni de `restore.lock` dans la trace des lectures |
| `TestDoctorRunsBesideWriters` (Windows réel, binaire) | `sync --apply`, première publication et rotation, et `restore --apply` pendant que `doctor` tourne en boucle, structurel puis `--verify`. Les écritures réussissent, ou le résidu HD2 est observé en `rolled back`, sans dommage |
| `TestDoctorHealthyVault` | Coffre synthétique sain, structurel et `--verify` : code 0 |
| `TestDoctorCodesFollowTheTable` | Chaque ligne de la table du §4.5 : constat, code et action ; jamais moins sévère que la commande de référence |
| `TestDoctorPendingOperationIsUndecided` | Constat distinct, code 3, action `--recover` sans `--apply`, aucune sonde |
| `TestDoctorOneFindingPerArea` | Zone d'un autre jeu : constat de cette zone, rapport complet |
| `TestDoctorVerifyStreamsAgainstTheReceipt` | `--verify` hache en flux, borné par le receipt ; contenu plus grand : constat |
| `TestDoctorNeverOpensHandedOverRestores` | Trace : identité par handle refermé aussitôt, aucune liste ni lecture, `--verify` compris |
| `TestDoctorReportsRefusedLetters` | S-02 : constat de code 2 |
| `TestDoctorDoesNotFollowLinks` (Windows réel) | Jonction sous une zone ou un coffre : non suivie |
| `TestDoctorStableCodesAreDocumented` | Chaque code de constat figure dans `commands/doctor.md` |
| `TestOnlySyncReadsItsPlan`, étendu | Les relevés n'exposent aucun moteur d'écriture |
| Imports | Cas explicites du §3.4 |

**B5d**

| Test | Preuve |
| --- | --- |
| `TestPinsRefuseArtifactsAndWritePermissions` | Fixtures négatives : `upload-artifact`, `contents: write`, `packages: write`, `id-token: write`, `attestations: write`, hooks GoReleaser |
| Smoke d'archive | §4.6 |
| Campagne | §11 |

---

## 10. Documentation livrée avec le code

- ADR amendés et ADR 0010.
- Modèle de sécurité : S-01 et S-02 fermés, avec leurs résidus et la phrase
  de modèle de menace ; limite des lectures réécrite.
- `commands/doctor.md`, avec chaque code de constat et son action ;
  `commands/sync.md`, `snapshot.md`, `restore.md` et `inspect.md` pour les
  lettres ; `vault.md`.
- Architecture et graphe.
- README et exemples vérifiés par test, « Odyssey seul » compris.
- `gitops.md` et `CONTRIBUTING.md` : build de contrôle et pipeline de
  release.
- Guide du pilote.
- CHANGELOG par sous-lot.
- Un rapport de validation par sous-lot.
- `AGENTS.md`, par la PR de T21.

---

## 11. Rollout, rollback et pilote

- Le code ne rejoint `release/v0.1.0-a1` que par PR ; aucun tag ni release.
- **Rollback de B5a** : revert de la PR. S-01 et S-02 redeviennent ouverts,
  l'interdiction opérationnelle s'applique de nouveau. Le revert est neutre
  pour les coffres et les zones, aucun format ne changeant.
- **Rollback de B5b et B5c** : revert, sans effet sur les données ; `doctor`
  n'écrit rien.

**Campagne pilote B5d (T16), obligatoire** pour clore B5 et avant toute RC.

**Binaire.** Le propriétaire le reconstruit par le GoReleaser épinglé, sur
le commit candidat de B5d. Le Go de production est identique à la fusion de
B5c. Les checksums sont comparés à ceux du journal du job de CI sur ce
commit ; un écart arrête la campagne.

**Préflight**, sur le modèle du kit de B4 :

- empreintes ;
- worktree propre et `vcs.modified=false` ;
- coffre neuf et zone neuve absents ;
- `subst`, sans argument, ne liste aucune lettre ;
- aucun test qui crée une lettre ne tourne.

L'agent n'accède à aucun coffre ni à aucune zone.

1. `doctor` en lecture seule, structurel puis `--verify`, sur le coffre de
   B2c (générations conservées), le coffre du pilote B3 et la zone du pilote
   B4.
   - Code `0` attendu, ou chaque constat expliqué.
   - Empreintes identiques avant et après, hors date de dernier accès.
   - Le coffre historique est exclu, ni lu ni importé.
2. Coffre neuf dédié à B5.
   - `sync --apply`, première publication, code `0`.
   - Jouer et sauvegarder.
   - `sync --apply`, **rotation obligatoire**, code `0`.
3. `snapshot --apply`, code `0`. `restore --apply` vers une zone neuve, code
   `0`. `inspect` de la restauration, égale au receipt du snapshot.
4. `doctor --verify` sur le coffre neuf et la zone neuve : code `0`,
   génération conservée vérifiée.
5. Plans de `sync`, `snapshot` et `restore` du binaire de `a59d787` sur le
   coffre neuf : mêmes sorties, aucune anomalie.
6. Consigner codes, nombres et verdicts. Ni hash individuel, ni
   identifiant d'opération, ni receipt réel, ni nom de profil, ni chemin
   personnel.

---

## 12. Critères d'acceptation

1. **S-01.** HW1 à HW6 et HW8 à HW12 prouvés. HW7 est prouvé manuellement
   par le propriétaire, ou son absence est déclarée et écrite comme limite.
   Les tests réels de réattribution pendant une session ne montrent aucune
   écriture hors de la cible tenue.
2. **S-02.** Lettres qui ne sont pas des racines de volume refusées partout
   où T20 le dit. Chaînes depuis la vraie racine ; raccourci « même lettre »
   supprimé. Tests réels des deux côtés pour `sync`, `snapshot`, `restore`
   et leurs récupérations.
3. Aucune API d'écriture nouvelle ; amendements d'ADR 0005 et 0006 acceptés
   avant le code ; instrumentation de la règle « aucune lettre après
   `Open` ».
4. Suites B2, B3 et B4 inchangées. Sorties identiques au binaire de
   `a59d787` hors du §7.2, erreurs comprises. Tests de différentiel et de
   course exercés sur le SHA exact.
5. S-01 et S-02 fermés par une revue de sécurité indépendante et un gate du
   propriétaire, consignés.
6. Lecture du coffre stable, ou preuve d'absence de fenêtre consignée.
   Budgets appliqués, valeur M1 acceptée. Points de B3 corrigés ou
   arbitrés ; ADR 0008 accepté avant son code.
7. `doctor` n'écrit rien, n'ouvre aucun verrou et ne tient aucune épingle :
   test sous ACL de refus et test de concurrence.
8. Chaque constat suit la table normative ; aucune action ne supprime.
9. `--verify` relit en flux `current`, les générations conservées et les
   snapshots publiés. Les restaurations remises ne sont jamais ouvertes
   pour leur contenu.
10. Frontière de `doctor` contrôlée par les tests d'imports ; ADR 0010 et
    amendement d'ADR 0004 acceptés.
11. Build de contrôle verte en CI : aucun secret, **aucun artefact sur le
    run**, vérifié par l'API ; aucun hook ; `release: disable: true`. Le
    vérificateur refuse les artefacts et les permissions d'écriture.
12. Revues indépendantes consignées : sécurité de B5a ; non-régression et
    regard de sécurité sur N2 et P2-4 en B5b ; point de contrôle du commit
    17 ; lecture seule et non-interférence de B5c ; CI et chaîne
    d'approvisionnement de B5d.
13. Checks requis verts sur le SHA exact de chaque PR et de chaque fusion,
    avec le lien du run.
14. Documentation, CHANGELOG et ADR cohérents ; exemples README vérifiés par
    test ; limites écrites ; « Odyssey seul » écrit.
15. Aucun accès agent aux données protégées ; aucun chemin personnel, save
    ou receipt réel dans le diff.
16. **Campagne B5d exécutée et qualifiée par le propriétaire, rotation
    comprise.** Aucune absence n'est admise.

---

## 13. Décisions reportées hors de B5

| Sujet | Comportement en B5 |
| --- | --- |
| Cyberpunk 2077, Metal Gear Solid V | Lot B6 : dossiers connus, profils, manifeste et ADR 0002 ; écart au plan directeur §7 consigné |
| Acceptation des alias de lettre | Refusés (T20) ; reconsidérée après preuves, chaînes de référence comprises |
| Vérification du contenu d'une restauration remise par `doctor` | Exclue (Q3) ; demanderait un amendement coordonné d'ADR 0006 §11 et d'ADR 0009 |
| Processus hostile du même compte | Hors du modèle de menace (§6) |
| Lectures indépendantes des lettres | Limite écrite (§6) |
| Contrôle de liens d'`inspect` au-dessus d'un alias | Limite écrite (§6) |
| Tolérance de B2 (`preparedFor`) | Reportée (Q5) ; signalée par `doctor` |
| Restauration en place | Lot futur, ADR de sécurité et protection de l'état actif |
| Portabilité d'un coffre copié | Lot dédié |
| Sortie JSON | Après stabilisation des commandes |
| Tag, release, canaux de paquets, SBOM, attestations | Gate de publication, après S-01 et S-02 fermés ; ruleset de tag avant le premier tag |
| Windows arm64, Linux, macOS | Non distribués (Q8) |
| Rétention et nettoyage | Jamais par Savewarden sans lot dédié |
| Extraction du moteur de `sync` | Critère de réexamen d'ADR 0004 inchangé. Ce critère est l'invariant de non-renommage et de partage en lecture (ADR 0006, l.1102-1105) ; `doctor` le respecte (T12, HD2) |

---

## 14. Risques

| Risque | Effet | Parade |
| --- | --- | --- |
| Hypothèse Windows fausse sur les chemins de volume | Primitives cassées ou garantie fausse | HW1 à HW12 prouvés avant le code ; arrêt et arbitrage, jamais de repli silencieux |
| Masquage de `Volume{GUID}` par un nom local | Écriture ailleurs | Contrôle du chemin final de chaque handle ; HW8 ; résidu et modèle de menace écrits |
| Détection S-02 imparfaite | Alias accepté, ou vraie lettre refusée | HW12 et sa contre-épreuve ; témoins réels des deux côtés ; refus nommés |
| Nouveaux refus pour une configuration légitime | Utilisateur bloqué | Messages qui nomment l'issue, désigner le volume par sa lettre ; documentés |
| Régression de `sync`, `snapshot` ou `restore` | Perte d'une garantie qualifiée | Suites complètes ; différentiel normalisé, erreurs comprises ; revue de sécurité ; campagne obligatoire |
| GUID affiché ou consigné | Sorties changées, données de machine exposées | Erreurs sans base ; test d'erreur forcée ; différentiel |
| Volume réinséré pendant une session | Écriture sur un montage neuf | Invalidation, vivacité de l'épingle racine ; HW7b |
| Faux code 3 de la lecture du coffre | Doute injustifié | T7 : reproduction, puis lecture stable |
| Budget trop bas | Coffre légitime refusé | Dimensionnement écrit ; M1 ; message distinct et issue documentée |
| `doctor` pris pour un outil de réparation | Gestes manuels risqués | Actions nommées, jamais de suppression |
| `doctor` qui gêne une écriture | `Lock` ou rotation refusés | Noms de verrou jamais ouverts ; partage complet ; HD1, HD2 ; test de concurrence |
| Frontière `doctor` qui fuit vers `sync` | Couplage entre features | Ports définis par `doctor` ; types dans `entities/backup` ; adaptateur étroit ; point de contrôle du commit 17 ; ADR 0010 |
| Artefact ou secret dans le job de release | Binaire distribué, publication involontaire | Aucun artefact, vérifié ; vérificateur étendu ; `release: disable: true` ; revue CI et chaîne d'approvisionnement |
| Binaire de campagne différent de la RC | Qualification d'un autre binaire | Reconstruction par le GoReleaser épinglé ; checksums comparés à la CI ; aucun Go de production en B5d |
| RC jugée suffisante sans Cyberpunk ni MGS V | Généralisation prématurée | « Odyssey seul » écrit ; B6 avant toute promesse de généralité |
| Dépôt public pendant que S-01 et S-02 sont ouverts | Attente d'une protection non fournie | T22 ; interdiction opérationnelle ; modèle de sécurité explicite |
| Mainteneur unique | Revue limitée | Revues indépendantes, Human GO, aucune auto-approbation |

---

## 15. Rapport de restitution attendu

Pour chaque sous-lot :

- branche et PR, SHA final, `git diff --stat` ;
- gates et résultats exacts, runs CI et leurs checks ;
- ADR créés ou amendés ;
- résultats HW1 à HW12, ou HD1 et HD2, valeur M1 ;
- différentiel et changements du §7.2 observés ;
- revues indépendantes ;
- résultat de la campagne ;
- écarts au plan, risques résiduels, questions ouvertes ;
- recommandation **sans auto-approbation**.

## Historique

| Version | Date | Commit | Changements |
| --- | --- | --- | --- |
| 0.1 | 5 novembre 2026 | `646d11f`, branche `docs/b5-framing` | Brouillon de cadrage sur le GO du propriétaire du 5 novembre 2026, en documentation seule : S-01 en tête de B5, points ouverts de B4 intégrés. Décisions T1 à T19 proposées ; option A de S-01 et hypothèses HW1 à HW7 ; `doctor` en feature avec ADR 0010 ; points ouverts de B3 ; découpage en quatre PR ; campagne et build de release en snapshot. Aucune revue intégrée, aucun ADR rédigé, aucun GO de développement |
| 0.2 | 6 novembre 2026 | local, branche `docs/b5-framing` | **Arbitrages du propriétaire du 6 novembre 2026 sur Q1 à Q9** (§0.3). Intégration de la revue indépendante r2 et de la revue CTO (§0.4). **S-02 intégré à B5a** (T20) : refus des lettres qui ne sont pas des racines de volume, chaînes depuis la vraie racine, raccourci « même lettre » supprimé. T2 précisé : ouverture en deux branches, cohérence de chaîne, chemin final des handles, invalidation, erreurs sans base, volume tenu. T4 réécrit sur les chaînes tenues, dans les deux sens, avant toute écriture. Hypothèses HW1 à HW12, HD1 et HD2. Budgets sur les métadonnées par lecture, valeurs dérivées, octets mesurés en M1 (écart signalé). `doctor` : ce qu'il ouvre, table normative des codes, présence seule des restaurations remises, un constat par zone, trois options de frontière comparées. Build de contrôle sans artefact ni hook, vérificateur étendu, aucun Go de production en B5d. Campagne et rotation obligatoires. Nouvelles décisions T20 à T22. Contexte mis à jour : B4 clos, release à `a59d787`, PR #15. Aucun ADR rédigé, aucun GO de développement |

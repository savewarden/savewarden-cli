# Savewarden — Plan B5 : durcissement S-01 et S-02, diagnostic `doctor` et release candidate

> **Statut : v0.5 PROPOSÉE, révision r3 — complément ciblé à la v0.4 `3c16d23`, préparé hors du dépôt, sans commit ni push, soumis à l'acceptation explicite du propriétaire : arbitrages CTO D1 à D4 du 7 novembre 2026 cités (§0.3) et intégrés (§0.5) ; corrections de la revue qualité du 7 novembre 2026 (§0.5) ; précisions P1 à P3 de la revue CTO de la r2 levées, politique d'alias arbitrée par le propriétaire (décisions 23 et 24) ; v0.5 r2 non ratifiée ; v0.4 non ratifiée tant que son texte exact n'est pas vérifié ; aucun ADR rédigé ni accepté ; aucun GO de développement**
> Version du plan : `0.5` proposée, révision r3 — 7 novembre 2026
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
  - **La revue CTO ciblée de `1bdeb01`** confirme le P0 S-02 et demande une
    v0.3 ciblée : quatre P1, C1 à C4, et deux formulations trop fortes. Le
    propriétaire tranche C2 et C4 et confirme trois décisions prises après
    `1bdeb01` (§0.3, §0.5).
- **7 novembre 2026.** La revue CTO de `fceaa38` lève C1 à C4 en
  conception, retient les six compléments et accepte les deux extensions.
  Elle demande deux précisions ciblées, A et B, sans élargir le périmètre
  (§0.3, §0.5).
- **7 novembre 2026, arbitrages sur `3c16d23`.** Le CTO accepte la
  précision A, et la précision B avec deux tests ajoutés au gate de B5a. Il
  demande de consigner l'écart de procédure du push de `fceaa38`, sans
  antidater d'autorisation, et de préparer uniquement le diff ciblé des deux
  compléments de tests, sans nouvelle extension du périmètre (§0.3, §0.5).
- **7 novembre 2026, revue CTO de la v0.5 r2.** HW13 est retenu comme
  hypothèse de faisabilité et l'erratum E1 accepté dans son principe. Trois
  précisions sont demandées : politique d'alias, banc de HW13, preuves
  d'E1. Le propriétaire admet l'équivalent strict d'une racine de volume
  (décisions 23 et 24).

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
| T2 | **S-01, option A, sous conditions : la session écrit par un chemin de volume tiré de ses épingles.** Elle ouvre en deux branches, cible existante ou absente. Elle contrôle la cohérence de sa chaîne, relit le chemin final de chaque nouveau handle, et de la source et de la destination de chaque `Move`, et s'invalide au premier défaut de périphérique. Ses erreurs ne nomment jamais sa base. Aucune API d'écriture nouvelle | Les épingles empêchent déjà de renommer les dossiers du chemin ; seule la lettre restait résolue à chaque appel. L'option B demanderait `NtCreateFile`, `SetFileInformationByHandle` et `unsafe`, interdits partout par ADR 0005 ; elle serait un nouvel arbitrage, jamais un repli | Arbitrée (Q1) | §3.1, §4.1 |
| T3 | **Hypothèses Windows confrontées à des tests réels avant le code** : HW1 à HW13 pour B5a, HD1 et HD2 pour B5c (§5), chacune dans un commit de tests seuls. Si une hypothèse tombe : arrêt, compte rendu, arbitrage du propriétaire. Aucun repli silencieux | Même discipline qu'ADR 0006 : les hypothèses Windows sont prouvées avant le code qui en dépend | Proposée | §5, §8 |
| T4 | **Contrôles sous session sur les chaînes tenues.** `sync`, `snapshot`, `restore` et chacune de leurs récupérations comparent la chaîne tenue par la session aux chaînes de référence, dans les deux sens, **après `Open` et avant la première écriture**. L'ordre est fixe : `Open` ; contrôles ; le cas échéant, création exclusive d'une racine absente, **seule mutation préparatoire** permise avant la reprise des contrôles, et contrôles repris ; étapes d'initialisation d'ADR 0006, inchangées ; verrou ; mutations de l'opération. Une reprise qui échoue arrête tout, sans autre objet ni suppression (§4.3). Le parent d'une cible absente n'ancre que la création ; il n'est jamais cherché dans une chaîne de référence (C1). Une observation impossible donne le code 1, sans rien écrire au-delà d'une racine vide déjà créée. La `Session` gagne des requêtes en lecture seule, changement d'interface déclaré | La comparaison de la seule racine ne prouve pas l'absence de recouvrement. La fenêtre de `sync` n'est pas fermée par la relecture du marqueur : des écritures la précèdent (`apply.go:350-388`) | Proposée | §4.3 |
| T5 | **Requalification de B2, B3 et B4**, exigée par la sortie de S-01 et de S-02. Les suites complètes restent inchangées, hors des tests de B4 que S-02 change, nommés au §7.2 (erratum E1, accepté dans son principe, décision 23). Le différentiel binaire des sorties de `sync`, `snapshot`, `restore` et `inspect` se fait contre le binaire de `a59d787`. Il est normalisé (identifiants d'opération, dates), couvre les chemins de succès et d'erreur et exclut les seuls changements nommés au §7.2. Tests réels de réattribution pendant une session | Les primitives changent de chemin, pas de comportement, hors des refus nommés : la preuve doit le montrer | Proposée | §9, §12 |
| T6 | **Commentaires de `platform/vault`** (point B4), écrits dans B5a : une session s'ouvre sur un coffre ou sur une zone, et écrit par le chemin de volume de ses épingles | Le package est rouvert par S-01 ; c'est le seul moment où le toucher ne coûte pas une revue de plus | Proposée | §7 |
| T7 | **Lecture stable du coffre** (point B4). Reproduire sur faux la fenêtre de `readProbing` pour le domaine courant, le domaine des snapshots et les plans de récupération. Si elle est reproduite : lecture stable, puis sonde du verrou indépendante de la liste, comme pour la zone. Sinon, consigner la preuve et ne rien changer | Le défaut est probable, pas démontré. Il n'écrit rien et donne un faux code 3, sûr mais trompeur. `doctor` lira le coffre avec ce lecteur | Proposée | §4.4 |
| T8 | **Budgets de lecture** (P2-3 de B3, point B4). Ils portent sur les métadonnées seulement, par lecture, jamais sur un contenu vérifié contre son receipt. Celui-ci est haché en flux, borné par les tailles et les nombres de son receipt. Les valeurs de comptage sont des hypothèses de dimensionnement écrites (§4.4), confirmées par M1 ; celle des octets est mesurée en M1. Un budget épuisé donne le code 1 et un message distinct dont l'issue est un coffre neuf | Un coffre forgé ne doit pas épuiser la mémoire. Un budget sur le contenu ou par exécution refuserait `--verify` et des coffres légitimes | Arbitrée (Q9) | §4.4 |
| T9 | **Points ouverts de B3** traités dans B5b selon le §0.2 : P2-4 avec un amendement d'ADR 0008 accepté avant son code ; N2 par une méthode marqueur ; plafond de profondeur JSON ; verrou de la récupération de `current` signalé ; `.gitignore`. Tolérance de B2 reportée | Décision du propriétaire du 5 novembre 2026. `doctor` dépend de P2-4 : il doit distinguer « illisible » de « modifié » | Proposée ; report arbitré (Q5) | §0.2 |
| T10 | **Noms de la matrice B4** : les six noms du plan deviennent des tests nommés ou des sous-tests, sans changement d'assertion ; la correspondance est consignée | Décision du propriétaire du 5 novembre 2026 ; commit de tests seuls | Décidée le 5 novembre | §8 |
| T11 | **`doctor` est une feature, `features/doctor`.** Elle possède son orchestration et son rapport et définit ses ports. `sync` lui fournit des **relevés** en lecture seule, rendus dans de nouveaux types d'observation de `entities/backup`. `app` passe un adaptateur étroit. Nouvelle frontière : **ADR 0010** et amendement d'ADR 0004 | Les trois options sont comparées avec leurs coûts au §3.4. La feature isole du code de lecture à côté des moteurs d'écriture, et les tests d'imports la contrôlent | Proposée | §3.4 |
| T12 | **`doctor` n'écrit jamais et ne gêne aucune écriture : gate de B5c.** Aucune écriture concurrente n'échoue de son fait, en `rolled back` ni autrement. `app` ne lui donne ni `Writer`, ni session, ni verrou. Il ne sonde aucun verrou et n'ouvre jamais `sync.lock` ni `restore.lock`, qu'il classe par l'entrée de répertoire. Il n'épingle rien et partage lecture, écriture et suppression à chaque ouverture. Une ouverture que HD2 ne prouve pas inoffensive n'est jamais faite (§4.5) | Sonder le verrou, ou seulement le lister par `Lstat`, l'ouvrirait : `os.Root.Lstat` ouvre l'entrée, et le `Lock` sans partage d'un `sync` concurrent échouerait (X1 de B4). Une exécution sans dommage ne prouve pas la non-interférence (C4) | Arbitrée (C4) | §4.5 |
| T13 | **Portée de `doctor`** : manifeste strict ; lettres (S-02) ; chemins, distinctions et liens ; coffre (marqueur, `current`, générations conservées, journaux, receipts, snapshots, enveloppe) ; zones désignées par `--area`, répétable, **un constat par zone**. Le coffre ne connaît pas les zones : `doctor` ne les cherche pas | Le diagnostic porte sur ce que l'utilisateur désigne, comme toutes les commandes. Aucune découverte | Proposée | §4.5 |
| T14 | **Deux niveaux.** Par défaut, un diagnostic **structurel**, sans lire le contenu. Avec `--verify`, `doctor` relit en flux `current`, chaque génération conservée et chaque snapshot publié, et les compare à leur receipt ; si HD2 exclut la lecture de `current` ou des générations conservées, seuls les snapshots publiés sont relus (§4.5). **Les restaurations remises ne sont jamais ouvertes pour leur contenu, listées ni lues**, même avec `--verify` : présence, type et identité seulement | Le défaut reste rapide ; `--verify` comble la limite de B3. ADR 0006 §11 et ADR 0009 restent inchangés ; vérifier une copie remise passe par un `inspect` explicite | Arbitrée (Q3) | §4.5 |
| T15 | **Codes de `doctor`** : `0`, `2`, `3`, `1`, avec une **matrice normative par domaine** fixée par ADR 0010 (§4.5). `1` : aucun rapport fiable. `3` : tout état qu'une commande de référence classe 3, et tout état indécidable. `2` : anomalie classée 2 par une commande de référence, refus persistant de celle-ci, ou domaine sur lequel rien n'est conclu. `0` : information. La commande de référence est nommée ligne par ligne ; le code du rapport est le plus haut de ses constats. Une opération en attente non départagée est un constat distinct, code 3, avec pour action `--recover` sans `--apply` | Mêmes sens qu'ADR 0003, 0007 et 0009 : `0` fiable, `2` à examiner, `3` à récupérer, `1` à corriger. Une règle uniforme contredirait l'existant ; un ordre global sur des commandes de sémantiques différentes aussi (C3) | Arbitrée (Q4) ; matrice précisée (C3) | §4.5 |
| T16 | **Campagne pilote Odyssey obligatoire**, rotation comprise (jouer et sauvegarder entre deux `--apply`). Elle est exécutée par le propriétaire avec un binaire qu'il reconstruit par le GoReleaser épinglé, sur le commit candidat de B5d. `doctor` est passé, en lecture seule, sur les coffres B2c et B3 et sur la zone B4 ; le coffre historique est exclu | S-01 réécrit justement les créations et les renommages que la campagne doit qualifier. Aucune « absence déclarée » n'est admise pour clore B5 | Proposée | §11 |
| T17 | **Préparation de la RC** (B5d) : build de contrôle en snapshot par `go run` de GoReleaser à une version stable exacte, choisie à v2.20.0 ou au-delà et épinglée après preuve de compatibilité avec Go 1.27.3 sous Windows, jamais flottante, Windows amd64 seul, sans hook, sans artefact, `release: disable: true`. Le vérificateur de pins refuse `upload-artifact` et les permissions d'écriture. CONTRIBUTING et `gitops.md` distinguent ce build de contrôle du pipeline de release. Aucun changement du Go de production en B5d | Plan directeur §5.2 point 6 et §5.3. Sur un dépôt public, un artefact de workflow est un binaire distribué. `platform/vault` renvoie `ErrUnsupported` hors Windows | Arbitrée (Q8) | §4.6 |
| T18 | **Extension à Cyberpunk 2077 puis à Metal Gear Solid V : hors de B5**, dans un lot B6 cadré à part, consigné comme écart au plan directeur §7. La RC porte sur Odyssey seul, écrit dans le README, le CHANGELOG et le guide du pilote | Le manifeste v1 refuse les variables et dossiers connus, « prévus avec le pilote Cyberpunk 2077 » | Arbitrée (Q6) | §2.2, §13 |
| T19 | **B5 reste sur `release/v0.1.0-a1`.** Le libellé de la première version publiée est fixé au gate de publication, après la fermeture de S-01 et de S-02 | AGENTS.md : toute suite part de `release/v0.1.0-a1`. Rien n'est publié en B5 | Proposée | §11 |
| T20 | **S-02 : en B5, toute lettre qui n'est pas une racine de volume est refusée** pour le coffre, la zone et les chemins du manifeste, ainsi que pour les chemins sauvegardés que `restore` compare, lus dans le receipt du snapshot ou, pour `restore --recover`, dans les plans journalisés de la zone, sans coffre. Les chaînes sont construites de toute façon depuis la vraie racine du volume. Le raccourci « même lettre » est supprimé : `sync` et `snapshot` comparent par identité quelle que soit la lettre. L'acceptation des alias qui désignent un sous-dossier est reportée après preuves. Une lettre strictement équivalente à une racine de volume, réelle, `subst` ou `DefineDosDevice`, est admise si elle remplit les trois conditions de la détection, après preuve par HW12 et HW13 (décision 24). HW11 et HW12 sont des préconditions : si l'une tombe, arrêt et arbitrage (T3) | Un alias vers un sous-dossier fait écrire Savewarden dans l'installation, une source ou le coffre. Prouvé sur Windows réel pour `restore` et `sync` ; `snapshot` partage le raccourci, établi par la lecture du code et couvert par les tests de B5a. Refuser est fail-closed. Reconstruire depuis la vraie racine ferme la même classe de défaut pour les chaînes tenues | Arbitrée (Q2) ; chemins sauvegardés confirmés le 6 novembre ; équivalent strict d'une racine arbitré le 7 novembre (décision 24) | §3.2, §4.2 |
| T21 | **Entrée du plan et gouvernance.** Jusqu'à son acceptation explicite, l'agent ne pousse pas le plan (décision 12 ; état du remote au §1). Le plan accepté rejoint la release branch par une PR documentaire depuis `docs/b5-framing`, sur GO distinct. La branche est mise à jour par un merge de `release/v0.1.0-a1`, sans rebase ni force-push, une fois connu le sort de la PR #15. La PR porte aussi `AGENTS.md` (lot courant B5), `docs/architecture.md` et `docs/validation/b4.md`. La fusion se fait par merge commit sur Human GO, puis la branche est supprimée sur le remote et en local. B5a part de ce merge commit | Le ruleset exige une branche à jour ; AGENTS.md doit nommer le lot courant avant tout code ; les GO de B4 doivent être consignés | Proposée | §8 |
| T22 | **Diffusion publique**, bloquée tant que S-01 ou S-02 est ouvert : tout tag, toute release ou brouillon de release, tout artefact de workflow, tout GitHub Package ou image, tout binaire joint ailleurs, tout canal de paquets, toute annonce. Un tag publie de façon irréversible sur `proxy.golang.org` : un ruleset de tag précède le premier. La visibilité du code en est exclue. Aucun binaire n'est présenté comme publiquement qualifié avant ces deux gates | Aligne le terme sur « diffuser une version publique » du modèle de sécurité ; complété par la revue r2 et la revue CTO | Arbitrée (Q7) | §11, §13 |

Écartés d'emblée, hors de B5 : restauration en place, réparation par
`doctor`, nettoyage, rétention, portabilité d'un coffre copié, API
d'écriture nouvelle, sortie JSON, support fonctionnel de Linux et macOS,
acceptation des alias de lettre qui désignent un sous-dossier.

### 0.2 Points ouverts de B4 et de B3

| Point | Origine | Décision | Sous-lot |
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
   comptage sont dérivées au §4.4. **Écart accepté** (décision 10) : la
   valeur en octets ne peut pas être mesurée sans code. Elle est mesurée par
   le commit M1 de B5b, sur un coffre synthétique, puis consignée par un
   erratum de ce plan et acceptée par le propriétaire. Cela se fait avant le
   code des budgets et avant ADR 0010.

Décisions postérieures à `1bdeb01`, prises en session le 6 novembre 2026
sur les questions de la v0.2, et absentes de ce commit :

10. **M1 et erratum.** « Oui : mesure M1 en B5b, erratum accepté avant le
    code des budgets et avant ADR 0010. »
11. **Chemins sauvegardés.** « Oui, lecture fail-closed : refuser aussi ces
    chemins. » Le refus S-02 porte sur les chemins sauvegardés que
    `restore` compare. Pour `restore --recover`, qui n'ouvre jamais le
    coffre, ce sont ceux des plans journalisés dans la zone (§3.2).
12. **Plan local.** « Garder local : la revue ciblée passe par le paquet ;
    push et PR documentaire (T21) après l'acceptation. »

Choix du propriétaire sur la revue CTO ciblée, le 6 novembre 2026 :

13. **C2 : HW7 obligatoire.** Il est prouvé par le propriétaire sur un vrai
    volume amovible, et en CI par un VHD détaché puis rattaché si le runner
    le permet. Sans HW7 prouvé, S-01 n'est pas fermé et B5a ne fusionne pas.
    Aucun volume n'est exclu.
14. **C4 : gate strict.** T12 reste absolu. HD2 est prouvé avant le code de
    `doctor`. S'il échoue, `--verify` ne relit que les snapshots publiés,
    ni `current` ni les générations conservées, dont la vérification est
    reportée. Aucun `rolled back` n'est admis.

Décisions de la revue CTO de `fceaa38`, le 7 novembre 2026 :

15. **Les six compléments du §0.5 sont retenus**, deux avec précision.
    - L'ordre des appels distingue la création initiale contrôlée d'une
      racine absente des écritures ordinaires soumises au verrou
      (précision A).
    - GoReleaser est épinglé à une version stable exacte après preuve de
      compatibilité avec Go 1.27.3 sous Windows. « v2.20.0 au moins » borne
      le choix et ne devient jamais une mise à jour flottante.
16. **HW11 obligatoire, accepté.** Si le runner GitHub ne permet pas le
    montage de test, le propriétaire exécute une preuve reproductible sur un
    VHD de test. Sans cette preuve, B5a ne franchit pas son gate.
17. **HD2 couvre les lectures d'identité du mode structurel, accepté.** Une
    ouverture non prouvée inoffensive est exclue de `doctor`, même sans
    `--verify`. Le diagnostic correspondant est déclaré non vérifié.
18. **Deux précisions avant ratification**, sans refonte ni élargissement.
    A : la création d'une racine absente est la seule mutation préparatoire,
    sous conditions, sans poursuite ni suppression si la reprise échoue
    (§4.1, §4.3). B : la fermeture de S-01 distingue les scénarios
    neutralisés et prouvés, les effets constatés par HW8 et les scénarios
    hors modèle de menace (§6, critères 1 et 5).

Arbitrages CTO sur la v0.4 `3c16d23`, le 7 novembre 2026. Chaque décision
cite le CTO mot pour mot, entre guillemets : sa directive D1 à D4 et, quand
elle l'éclaire, l'analyse qui la précède (§2 à §4 de l'arbitrage, joint au
paquet de revue avec son empreinte). Suit la transposition de l'agent, qui
n'est pas une parole du CTO.

19. **D1, précision A.** « D1 — Précision A : acceptée. Ta correction est
    justifiée. La création exclusive d'une racine absente doit être l'unique
    mutation autorisée avant sa revalidation. Une fois celle-ci réussie, les
    étapes d'ADR 0006 restent applicables dans leur ordre actuel, y compris
    leurs écritures pré-verrou. Conserver les tests démontrant qu'une
    revalidation échouée ne crée ni marqueur, ni `games`, ni verrou, ni
    journal et ne supprime pas automatiquement la racine vide. »
    - Transposition : aucun changement de texte, la v0.4 l'écrit déjà (T4,
      §4.1, §4.3). `TestCreatedRootIsCheckedBeforeTheLock` et
      `TestSessionOrderIsOpenChecksLockWrites` sont gardés (§9).
20. **D2, précision B.** « D2 — Précision B : acceptée avec deux
    compléments. Conserver les trois classes de fermeture S-01 et les
    limites de HW8. Ajouter au gate B5a : Un test Windows réel de refus
    opérationnel des alias `DefineDosDevice`, au-delà de leur seule
    détection par HW12. Un test dédié de réattribution d'une lettre de
    référence entre le plan et la session, couvrant notamment les
    références historiques utiles à la séparation. Ces deux scénarios ne
    pourront pas être déclarés neutralisés avant leurs preuves. Le masquage
    administratif via `mountvol` peut rester hors du modèle de menace, avec
    une limite explicite. »
    - Tableau des classes, §2 de l'arbitrage : « Réattributions et alias
      neutralisés » : « Garder, sous réserve d'exécution réussie des tests
      correspondants » ; « Masquage local du nom de volume » : « Garder,
      avec les effets réels de HW8 et les limites de confinement » ; « Hors
      modèle de menace ou non exercé » : « Garder, mais distinguer
      explicitement ces deux catégories ».
    - §2 de l'arbitrage : « Il n'est pas nécessaire de réaliser ces tests
      pendant le cadrage. En revanche, ils doivent figurer dans la matrice
      de validation B5a et être exécutés avant la clôture des chantiers. »
    - §2 de l'arbitrage : « Pour `mountvol`, le maintien hors du modèle de
      menace actuel est acceptable, à condition de ne pas revendiquer une
      protection universelle contre toute réattribution administrative de
      volume. »
    - Transposition : `TestDefineDosDeviceAliasIsRefused` et
      `TestReferenceLetterRemappedBeforeTheSessionIsRefused` (§9), exigés
      au gate de B5a (§8, commit 7). Classe 1 au statut « À démontrer » au
      cadrage. Classe 3 en deux catégories distinctes : 3a, hors modèle de
      menace, où figure `mountvol` avec sa limite ; 3b, non exercé. « Hors
      du périmètre de S-01 » est un sous-classement de l'agent, à
      l'intérieur de 3b (§6).
    - Hors de la lettre de D2, soumis au CTO : l'hypothèse HW13,
      précondition du test de référence (§5.1), et l'erratum E1 des tests
      de B4 que S-02 change (§7.2).
21. **D3, push de `fceaa38`.** « D3 — Push de `fceaa38`. Le commit est
    effectivement sur `origin/docs/b5-framing`, mais aucun GO de push n'est
    consigné dans nos décisions. Ne pas antidater une autorisation ni
    présumer de son auteur. Consigner l'écart de procédure. Conserver la
    branche distante en l'état, sans force-push, suppression ni nouvelle
    PR. »
    - §3 de l'arbitrage : « Je recommande de conserver provisoirement la
      branche distante telle quelle, de consigner l'écart de procédure et de
      ne faire aucun push supplémentaire. Pas de `force-push` ni de
      suppression corrective sans décision distincte. Cette régularisation
      documentaire ne constitue pas une autorisation rétroactive. »
    - Transposition : écart consigné au §1, avec les seuls faits du reflog.
      Ni auteur ni intention ne sont attribués ; aucune autorisation
      rétroactive ; aucun push supplémentaire.
22. **D4, acceptation du plan.** « D4 — Acceptation du plan. La lecture
    technique des précisions A et B est favorable, mais le texte exact de
    la v0.4 et son patch ne sont pas encore vérifiés indépendamment. Ne pas
    considérer `3c16d23` comme ratifié avant cette dernière vérification.
    Aucun GO de push, de fusion de la PR #15, de rédaction normative des
    ADR ou de développement B5. Préparer uniquement le diff ciblé des deux
    compléments de tests, sans nouvelle extension du périmètre. »
    - §4 de l'arbitrage : « Je ne demande pas une refonte du plan. Les
      ajustements nécessaires sont limités aux deux scénarios de test et à
      la qualification des garanties futures. »
    - Transposition : la v0.4 n'est pas ratifiée (statut). Le complément
      porte les deux compléments de tests de D2 et les consignations de D1
      et D3. Deux points dépassent la lettre de D4 : HW13 et l'erratum E1.
      Ils sont soumis explicitement au CTO, qui peut les écarter (§0.5).

Revue CTO du complément v0.5 r2, le 7 novembre 2026, et choix du
propriétaire qui en découle. Même convention : citation, puis transposition.

23. **Revue CTO de la v0.5 r2.** « HW13 est retenu comme hypothèse de
    faisabilité, non comme garantie déjà prouvée. » « L'erratum E1 sur les
    quatre tests B4 est accepté dans son principe. » Trois précisions sont
    demandées avant ratification :
    - « Politique d'alias. Réconcilier la règle S-02 du §3.2 avec
      l'interdiction générale des alias au §7.3 et avec les lettres
      équivalentes à des racines de volume envisagées dans HW13. Ne pas
      admettre implicitement un nouveau type d'alias. Soumettre toute
      modification normative du contrat au propriétaire. »
    - « Sécurité HW13. Borner le montage Windows à une lettre libre
      exclusivement créée pour le test, dans un contexte isolé, avec
      constat de l'état initial, nettoyage exact et vérification finale.
      Aucun changement des lettres existantes ni expérimentation sur les
      coffres ou la session interactive du pilote. Si la faisabilité
      échoue, appliquer T3 sans test sauté ni repli implicite. »
    - « Conservation des preuves E1. Adapter les fixtures dépendant de
      `sync` et `snapshot` initialement réussis via `subst`. Maintenir la
      preuve spécifique du contrôle de séparation sous session, distincte
      d'un refus S-02 au plan. Pour les récupérations, utiliser de
      véritables opérations récupérables et des références historiques
      valides. »
    - « Je ne demande aucune refonte du plan, aucun nouveau sous-lot et
      aucun changement de l'architecture retenue. » « Préparer uniquement
      un diff documentaire ciblé et une courte table de levée des trois
      observations, toujours hors du dépôt. » « La v0.5 r2 n'est pas encore
      ratifiée. »
    - Transposition : P1 par la décision 24 (T20, §2.2, §3.2, §6, §7.3,
      §13, critère 2) ; P2 dans HW13, le §2.3 et le §5.1 ; P3 au §7.2 et
      au §9. Table de levée au §0.5.
24. **Politique d'alias, choix du propriétaire** (P1). Question posée en
    session le 7 novembre 2026, avec deux options formulées par l'agent :
    l'équivalent strict d'une racine admis, ou tous les alias refusés.
    Réponse : « Équivalent strict admis (Recommandé) », la mention
    « Recommandé » portant l'avis de l'agent. Contenu de l'option retenue :
    - tout alias d'un sous-dossier reste refusé ;
    - une lettre strictement équivalente à une racine de volume est
      admise, qu'elle soit réelle, `subst` ou `DefineDosDevice`, si elle
      remplit les trois conditions de la détection (chemin final GUID sans
      reste, même GUID par `GetVolumeNameForVolumeMountPoint`, même
      identité), et seulement après preuve par HW12 et HW13 ;
    - les formulations absolues du §2.2, du §6, du §7.3 et du §13 sont
      reformulées en conséquence.

    Transposition : cette décision précise Q2 (décision 2), l'acceptation
    reportée étant celle des alias de sous-dossier. Les chaînes partent de
    la vraie racine du volume, et une session écrit par le chemin de volume
    de ses épingles : un équivalent strict ne contourne aucune distinction.
    L'interdiction opérationnelle reste en vigueur jusqu'au gate de S-02.

Le propriétaire tient le rôle de CTO : les revues CTO du 5 au 7 novembre
2026 sont les siennes. Elles sont indépendantes de l'auteur du plan, mais non
du propriétaire qui l'accepte, et sont consignées comme telles.

### 0.4 Revues intégrées et traçabilité

Revue r2 du 6 novembre 2026 : rapport consolidé et deux rapports bruts,
identifiants `S5-*` pour B5a et B5b, `D5-*` pour B5c, B5d et la gouvernance.
Revue CTO du 5 novembre 2026 sur `646d11f` : `P1-01` à `P1-03`, P2 numérotés.
Revue CTO ciblée du 6 novembre 2026 sur `1bdeb01` : C1 à C4 et six avis,
traités au §0.5. Revue CTO du 7 novembre 2026 sur `fceaa38` : précisions A
et B, traitées au §0.5. Arbitrages CTO du 7 novembre 2026 sur `3c16d23` :
D1 à D4, traités au §0.5. Revue qualité du complément v0.5 r1, le 7 novembre
2026, relayée par le propriétaire : deux bloquants, quatre importants, six
mineurs, traités au §0.5. Revue CTO de la v0.5 r2, le 7 novembre 2026 :
précisions P1 à P3, levées au §0.5. Cette table garde le traitement fait en
v0.2.

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

### 0.5 Levée des constats des revues CTO (v0.3 à v0.5)

Verdict de la revue : P0 S-02 confirmé, T20 et l'architecture favorables,
quatre P1, v0.2 non acceptée. Les anciens P1 sont levés au niveau de
l'intention ou du contrat : P1-01 sous réserve de HW9, P1-02 par C1, P1-03.

| Constat | Correction v0.3 | Tests | Critères | Où |
| --- | --- | --- | --- | --- |
| **C1**, fausse collision d'une cible absente : le parent tenu figure dans la chaîne d'une référence sœur | Le parent d'une cible absente ne sert qu'à prouver la stabilité de l'ancrage et l'absence persistante de la cible. « Aucune référence dans la cible » ne porte que sur une cible existante. Après la création, la chaîne tenue est reconstruite avec la racine créée, et les règles sont reprises avant le verrou | `TestAbsentTargetBesideAReferenceIsAccepted` ; `TestAbsentTargetUnderAReferenceIsRefused` ; `TestExistingTargetContainingAReferenceIsRefused` ; `TestCreatedRootIsCheckedBeforeTheLock` | 2 | T4 ; §4.3 |
| **C2**, HW7 facultatif alors que S-01 se ferme | HW7 obligatoire (décision 13). Il est prouvé par le propriétaire sur un vrai volume amovible, et par un VHD en CI si le runner le permet. Sans preuve, S-01 n'est pas fermé et B5a ne fusionne pas. HW11 devient obligatoire lui aussi, au titre des garanties non testées (ligne ci-dessous), accepté (décision 16) | `TestVolumeDetachedDuringTheSession` ; `TestMountPointAmongAncestorsIsRefused` sans repli documentaire | 1 | §2.3 ; §5.1 ; §8 |
| **C3**, codes de `doctor` contradictoires : 2 contre « récupération : 3 » | Matrice sémantique de 0, 1, 2 et 3. Commande de référence nommée ligne par ligne. Cas de `current` et des générations conservées rendus exhaustifs, avec et sans opération en attente. Erreur d'usage de `doctor` distincte d'un refus de la commande de référence. Code du rapport : le plus haut constat. Même correction pour un snapshot dégradé, que `restore` classe 2 et que la v0.2 donnait en 0 | `TestDoctorAgreesWithItsReferenceCommand` ; `TestDoctorUsageErrorIsNotAFinding` ; `TestDoctorReportCodeIsTheHighestFinding` | 8 | T15 ; §4.5 |
| **C4**, non-interférence promise alors qu'un `rolled back` est admis | Gate strict (décision 14). HD2 couvre chaque classe d'ouverture d'un objet qu'une écriture renomme. Une classe non prouvée inoffensive n'est jamais ouverte, et l'erratum qui la retire est accepté avant le code de `doctor`. Pour `--verify`, le repli se limite aux snapshots publiés. La consigne « hors de toute écriture » est supprimée. La même règle s'applique aux identités ouvertes en mode structurel, sans quoi le gate ne serait pas strict : extension acceptée (décision 17) | `TestDoctorRunsBesideWriters` sans aucun `rolled back` ; `TestDoctorOpensOnlyWhatHD2Proved` | 7, 9 | T12 ; T14 ; §4.5 ; §5.2 |
| **Formulation « objet vide »** | Supprimée. HW8 constate séparément les effets de `CreateDir`, `NewFile` et `Move`. Aucun effet maximal n'est promis avant HW8. `Move` reçoit une postcondition propre | `TestLocalVolumeNameShadowing`, HW8a à HW8c ; `TestMoveChecksItsSourceAndDestination` | 1 | §4.1 ; §5.1 ; §6 |
| **Garanties non testées** | Plus aucune garantie n'est remplacée par une limite écrite. Cela vaut pour HW7 (C2) et le résidu de HD2 (C4). Cela vaut aussi pour HW11, précondition de T20 selon la revue, que la v0.2 laissait en « limite écrite » si le runner ne permettait pas le montage : accepté (décision 16) | Ceux de C2 et C4 | 1, 7 | §5 ; §6 |

| Avis | Intégration v0.3 | Où |
| --- | --- | --- |
| 1, modèle de menace | Accepté, HW8 gardé ; voir la ligne « objet vide » | §4.1 ; §6 |
| 2, S-02 dans `platform/vault` | Une requête unique, composée dans `Volumes`, appliquée à toutes les références et à toutes les récupérations. `restore --recover` prend ses chemins sauvegardés dans les plans de la zone, jamais dans le coffre | §3.2 ; §4.2 ; §4.3 ; `TestRestoreRecoverChecksSavedPathsWithoutTheVault` |
| 3, interface `Session` | Changement assumé, sans API d'écriture. La revue de sécurité de B5a relit le port `Vault` et les fausses sessions. L'ordre est imposé et testé | §4.1 ; §8 ; `TestSessionOrderIsOpenChecksLockWrites` ; critère 3 |
| 4, codes | C3 | §4.5 |
| 5, budgets et M1 | Les valeurs de comptage sont des hypothèses de dimensionnement. M1 mesure aussi les grands receipts, les noms nombreux et longs, et la profondeur | T8 ; §4.4 ; `BenchmarkVaultReadAtScale` |
| 6, GoReleaser | Version stable exacte, choisie à v2.20.0 ou au-delà, épinglée après preuve de compatibilité avec Go 1.27.3 sous Windows, jamais flottante (décision 15). L'absence d'artefact est vérifiée par l'API | T17 ; §4.6 ; critère 11 |

**Revue CTO de `fceaa38`, 7 novembre 2026.** C1 à C4 sont levés en
conception, et non certifiés : HW7, HW9, HD2 et les autres hypothèses
restent à démontrer. Les six compléments sont retenus et les deux extensions
acceptées (décisions 15 à 17).

| Précision | Correction v0.4 | Tests | Critères | Où |
| --- | --- | --- | --- | --- |
| **A**, création d'une racine absente avant le verrou | La création exclusive d'une racine absente est la seule mutation préparatoire permise avant la reprise des contrôles. Elle n'a lieu qu'après les contrôles sur le parent tenu, pour une destination prouvée neuve. Si la reprise échoue, l'opération s'arrête : aucun autre objet, aucune suppression, la racine vide reste. Les formules « avant toute écriture » sont corrigées. **Écart à la lettre de la demande** : avant le verrou, ADR 0006 fait déjà d'autres étapes d'initialisation (marqueur, `games` et dossier du jeu). Elles restent inchangées et, après une création, ne suivent qu'une reprise réussie. Les supprimer exigerait d'amender ADR 0006 et de déplacer le verrou, ce qui élargirait B5 | `TestCreatedRootIsCheckedBeforeTheLock` précisé ; `TestSessionOrderIsOpenChecksLockWrites` précisé | 3 | T4 ; §4.1 ; §4.3 |
| **B**, portée de la fermeture de S-01 | Trois classes, chacune avec ses preuves : scénarios neutralisés et prouvés ; effets de masquage local constatés par HW8 ; scénarios hors modèle de menace ou non exercés, jamais présentés comme sécurisés. Aucun confinement n'est promis contre le masquage local du nom de volume. Complétée par D2 (décision 20) | Ceux de HW3, HW4, HW7, HW8, HW11, HW12 et de T4, inchangés | 1, 5 | §2.1 ; §3.5 ; §6 |

**Arbitrages CTO sur `3c16d23`, 7 novembre 2026.** Le complément v0.5
traite ces quatre points. D4 le borne : « Préparer uniquement le diff ciblé
des deux compléments de tests, sans nouvelle extension du périmètre ». Le §4
de l'arbitrage aussi : « Les ajustements nécessaires sont limités aux deux
scénarios de test et à la qualification des garanties futures ».

| Arbitrage | Complément v0.5 | Tests | Critères | Où |
| --- | --- | --- | --- | --- |
| **D1**, précision A | Aucun changement de texte : la v0.4 l'écrit déjà. Décision 19 consignée | `TestCreatedRootIsCheckedBeforeTheLock`, `TestSessionOrderIsOpenChecksLockWrites`, gardés | 3 | §0.3 |
| **D2**, deux tests au gate de B5a | Alias `DefineDosDevice` refusé de bout en bout ; lettre de référence réattribuée entre le plan et la session, références historiques comprises. Classe 1 « à démontrer » au cadrage. Classe 3 en deux catégories distinctes : 3a, hors modèle de menace, où figure `mountvol` avec sa limite ; 3b, non exercé | `TestDefineDosDeviceAliasIsRefused`, `TestReferenceLetterRemappedBeforeTheSessionIsRefused` | 1, 2, 5 | §2.1 ; §5.1 ; §6 ; §8 ; §9 |
| **D3**, push de `fceaa38` | Écart de procédure consigné avec les seuls faits du reflog, sans auteur ni intention attribués ; branche distante conservée en l'état ; pas d'autorisation rétroactive | — | — | §0.3 ; §1 |
| **D4**, acceptation | v0.4 non ratifiée avant vérification de son texte exact ; diff ciblé ; les deux points qui dépassent sa lettre sont signalés ci-dessous | — | — | Statut ; §0.3 |

**Revue qualité du complément v0.5 r1, 7 novembre 2026**, relayée par le
propriétaire : « non ratifiable en l'état ». Corrections de la révision r2.
Les deux lignes marquées « soumis au CTO » dépassent la lettre de D4 : le
CTO peut les retenir ou les écarter.

| Constat | Correction r2 | Où |
| --- | --- | --- |
| **B1**, décisions 20 et 22 infidèles à l'arbitrage | D1 à D4 cités mot pour mot, transposition de l'agent séparée. « Non exercé » rétabli comme catégorie distincte de « hors modèle de menace ». « Hors du périmètre de S-01 » désigné comme sous-classement de l'agent. La décision 22 cite D4. « Hors du dépôt, sans commit » porté au statut et au §1, comme des faits | §0.3 ; §0.5 ; §6 ; statut ; §1 |
| **B2**, §1 qui disculpe l'agent | « L'agent n'en a lancé aucun » retiré. Restent l'entrée du reflog et « auteur non établi » | §1 |
| **I1**, test de référence sur un point d'interposition non décrit | Mécanisme réel nommé : un double du port `Vaults` qui réattribue la lettre dans `Open`, comme `remapVaults` en B4 (`internal/app/restore_remap_windows_test.go:25-43`). Le test est en processus, avec les adaptateurs réels : le binaire n'a aucun point d'insertion entre `Plan` et `begin` (`apply.go:193`, `:208`), et aucun crochet n'est ajouté au code de production. Même correction pour les deux tests de réattribution du coffre avant la session. Précondition écrite en **HW13**, arrêt T3 si elle tombe ; risque au §14. La lettre de HW13 vise la racine du volume de `t.TempDir()` : exception bornée au §2.3. **Soumis au CTO** | §2.3 ; §5.1 ; §6 ; §9 ; §14 |
| **I2**, tests de B4 que S-02 change, contraire à T5 | **Erratum E1** : quatre tests inventoriés, changements nommés, sans refonte ; T5 et critère 4 ajustés. **Soumis au CTO** | T5 ; §7.2 ; critère 4 |
| **I3**, « jamais déclassé », règle ajoutée | Retiré. Un scénario de la classe 1 n'est déclaré neutralisé qu'avec son essai passé ; un essai qui ne peut être exercé relève de T3 | §6 ; §8 |
| **I4**, refus non déclarés au §7.2 | Nouveaux refus sous session de `restore` et `restore --recover` déclarés, référence nommée comprise | §7.2 |
| **M1** à **M6** | Date du §1. Statuts des classes 2 et 3. Couverture : chemin sauvegardé du receipt dans le test `DefineDosDevice`, `sync --recover` et `snapshot --recover` dans le test de référence. Légende du §9. Latitude du CTO citée au lieu d'un constat. Ligne recoupée. Renommage tenté dans le dépôt, consigné au §1 | §1 ; §0.3 ; §5.1 ; §6 ; §9 |

**Revue CTO de la v0.5 r2, 7 novembre 2026.** HW13 retenu comme hypothèse,
E1 accepté dans son principe (décision 23). Levée des trois précisions par
la révision r3, sans refonte, sans nouveau sous-lot ni changement
d'architecture.

| Précision | Levée r3 | Où |
| --- | --- | --- |
| **P1**, politique d'alias | Contrat arbitré par le propriétaire : équivalent strict d'une racine de volume admis aux trois conditions de la détection, après HW12 et HW13 ; tout alias d'un sous-dossier refusé (décision 24). Formulations absolues reprises. Interdiction opérationnelle inchangée jusqu'au gate de S-02 | T20 ; §0.1 ; §2.2 ; §3.2 ; §6 ; §7.3 ; §13 ; critère 2 ; §14 |
| **P2**, sécurité du banc HW13 | CI seulement, contexte isolé ; lettre libre réservée, jamais une lettre préexistante ni définie au démarrage ; état initial relevé, nettoyage exact, vérification finale ; jamais sur le poste, la session interactive, la campagne, un coffre ou une zone de pilote. T3 si HW13 tombe, sans test sauté ni repli vers `mountvol`, un VHD ou des privilèges | HW13 ; §2.3 ; §5.1 ; §9 |
| **P3**, preuves d'E1 | Fixture de snapshot construite sans alias. Refus attribué à son garde, un refus au plan ne requalifiant aucun contrôle sous session. Garde de séparation prouvé seul sur faux, avec mutation consignée. Récupérations sur des opérations réellement récupérables, avec témoin positif. Références historiques produites par le code sous test, jamais forgées | §7.2 ; §8, commit 7 ; §9 |

---

## 1. Contexte et preuves

| Élément | État vérifié le 7 novembre 2026 |
| --- | --- |
| `release/v0.1.0-a1` | `a59d787`, merge de la PR #14 (clôture de B4, depuis `docs/b4-closure`). Code identique à `4197317` : la PR #14 ne touche que trois fichiers de documentation |
| CI | Run 42155814194 sur `a59d787` : 7/7, `success` |
| `main` | `25d1cc2`, bootstrap B0 seul |
| B4 | **Clos.** Code : PR #13, `4197317`. Pilote réel exécuté par le propriétaire le 5 novembre 2026, 101 contrôles sur 101, qualifié le 6. Clôture : PR #14, `a59d787`. Human GO de clôture du 6 novembre 2026, donné en session ; `validation/b4.md` le liste encore comme restant, ce que corrige T21 |
| S-02 | Trouvé par la revue r2 du 6 novembre 2026, prouvé sur Windows réel le même jour, hérité de B2 et de B4. Prouvé pour `restore` et `sync`, et pour une installation nommée par un alias dans le manifeste ; `snapshot` partage le raccourci, ce qu'établit la lecture du code, sans essai. Consigné comme chantier ouvert par la PR #15 (`docs/s02-letter-alias`, `e6bb4cf`, CI 7/7, run 42232254323), **ouverte, non fusionnée**. Interdiction opérationnelle : aucun alias de lettre, ni pour `--vault` ou `--to`, ni dans le manifeste |
| Cadrage B5 | `docs/b5-framing` : v0.1 en `646d11f`, parent `4197317`, présente sur le remote. L'autorisation de ce push n'est consignée ni dans le dépôt ni dans les rapports : elle reste à citer par le propriétaire. v0.2 : commit `1bdeb01`, non accepté. v0.3 : commit `fceaa38`, non ratifié. **`origin/docs/b5-framing` pointe sur `fceaa38`.** Le reflog du suivi distant, dans ce clone, porte l'entrée `origin/docs/b5-framing@{2026-11-06 19:30:52 +0100}: update by push`. L'auteur n'est pas établi ; aucune PR n'est ouverte. **Écart de procédure consigné** (décision 21) : opération distante sans autorisation consignée, contraire à la décision 12, sans intention attribuée. La branche distante est conservée en l'état, sans push supplémentaire, force-push, suppression ni nouvelle PR (décisions 21 et 22). Cette consignation ne vaut pas autorisation rétroactive. v0.4 : commit local `3c16d23`, non ratifié. v0.5 : complément proposé hors du dépôt, sans commit ni push. Le renommage de la v0.4 en v0.5 dans le dépôt, tenté par l'agent, a été arrêté par la revue automatique avant tout commit ; le complément a ensuite été préparé hors du dépôt |
| Revues | Revue r2 du 6 novembre 2026 : deux examinateurs, un P0, dix-huit P1, constats porteurs revérifiés. Revue CTO du 5 novembre 2026 : 0 P0, 3 P1, 6 P2. Elle est antérieure à S-02, qui dépasse son « 0 P0 » et la prémisse de son P1-02 (« règle déjà acquise en B4 »). Revue CTO ciblée du 6 novembre 2026 sur `1bdeb01`, faite sur le paquet de revue, sans relancer les sondes : P0 S-02 confirmé, 4 P1 (C1 à C4), v0.2 non acceptée. Revue CTO du 7 novembre 2026 sur `fceaa38` : intégrité et rejeu du patch conformes, C1 à C4 levés en conception, précisions A et B demandées, v0.3 non ratifiée. Arbitrages CTO du 7 novembre 2026 sur `3c16d23` : A acceptée, B acceptée avec deux tests ajoutés au gate de B5a, consignation de l'écart du push de `fceaa38` demandée, v0.4 en attente de vérification de son texte exact. Revue qualité du complément v0.5 r1, le 7 novembre 2026, relayée par le propriétaire : non ratifiable en l'état, deux bloquants et quatre importants, corrigés en r2 (§0.5). Revue CTO de la v0.5 r2, le 7 novembre 2026 : intégrité et rejeu conformes, HW13 retenu comme hypothèse, E1 accepté dans son principe, trois précisions levées en r3, r2 non ratifiée (§0.5). Le rôle de CTO est tenu par le propriétaire (§0.3) |
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
   - La fermeture est déclarée dans sa portée, en trois classes : scénarios
     neutralisés, chacun une fois son essai passé ; effets de masquage
     constatés par HW8 ; scénarios hors modèle de menace, distingués des
     scénarios non exercés (§6).
   - Des tests Windows réels réattribuent une lettre pendant une session.
   - HW7 est prouvé : un volume retiré, ou retiré puis réinséré, pendant
     une session.
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
- **Acceptation des alias de lettre qui désignent un sous-dossier** :
  reportée après preuves (Q2). Seul l'équivalent strict d'une racine de
  volume est admis (décision 24, §3.2).
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
| **Agent** | Lire et modifier le dépôt ; tests synthétiques en `t.TempDir()` ; lettres `subst` temporaires et noms DOS locaux de test, créés et retirés par les tests, vers des dossiers temporaires ; pour HW13 et les tests qui emploient sa voie, en CI seulement : une lettre libre réservée au test vers la racine du volume qui porte `t.TempDir()`, état initial relevé, nettoyage exact et vérification finale (§5.1), sans lecture ni écriture hors de `t.TempDir()` ; HW8 en CI seulement ; VHD de test créés, attachés, détachés et retirés par les tests de HW7 et HW11, dans un dossier temporaire, en CI seulement | Lire `D:\Ga`, `D:\Savewarden`, `E:\GMS`, un coffre ou une zone de pilote, ou des clés SSH ; lancer Savewarden sur des données réelles sans autorisation explicite et nominative pour cette exécution ; modifier une lettre, un nom DOS ou un volume qu'un test n'a pas créé ; lancer HW13, ou un test qui emploie sa voie, hors de la CI, notamment sur le poste ou dans la session interactive du propriétaire ; exécuter le protocole manuel de HW7 ou de HW11 |
| **Savewarden**, lancé par le propriétaire | `doctor` : lire le manifeste, le coffre désigné, les zones désignées et les identités de l'installation et des sources, sans suivi de lien ; les autres commandes, comme en B4 | `doctor` : écrire, verrouiller, sonder ou ouvrir `sync.lock` et `restore.lock`, épingler, ouvrir une session, réparer, ouvrir une restauration remise pour son contenu |
| **Propriétaire** | Exécuter la campagne, HW7 et, s'il n'est pas prouvé en CI, HW11 ; autoriser nominativement une exécution par l'agent ; ajouter un check requis au ruleset ; donner chaque GO | — |

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
- **Équivalent strict d'une racine** (décision 24). La règle porte sur ce
  que la lettre désigne, pas sur sa nature. Une lettre réelle, un `subst`
  ou un nom défini par `DefineDosDevice` est admis s'il remplit les trois
  conditions de la détection ci-dessous ; tout autre alias est refusé.
  - C'est la détection qui décide : une lettre qui échoue à l'une des
    conditions est refusée, même si elle désigne une racine. Aucun
    relâchement n'en découle.
  - Cette admission n'est acquise qu'après HW12 et HW13. Si HW13 montre
    qu'aucun équivalent strict ne remplit les conditions, la règle reste
    celle de la détection et les tests qui en dépendent relèvent de T3.
- **Détection, à prouver par HW12.** Trois conditions, toutes requises :
  - On ouvre un handle sur `X:\`, sans suivi. Son chemin final en
    `VOLUME_NAME_GUID` doit valoir exactement `\\?\Volume{GUID}\`, sans
    reste.
  - Contre-épreuve : `GetVolumeNameForVolumeMountPoint("X:\")` doit réussir
    et rendre le même GUID.
  - L'identité de `X:\` égale celle de `\\?\Volume{GUID}\`.
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
  - Pour `restore --recover`, qui n'ouvre jamais le coffre : la lettre de la
    zone, de l'installation et des sources du manifeste, et de chaque chemin
    sauvegardé des plans journalisés dans la zone (`RestorePlan.Sources`,
    `restore_recover.go:164-170`). Le coffre n'y est ni lu ni nommé, et la
    récupération ne gagne aucune dépendance au coffre.
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
  S-02, pour tout alias, équivalent strict d'une racine compris.

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
| 0006 §11, amendement | **S-01** : fenêtre fermée pour les écritures face à une lettre ou un alias réattribué, pas face à un processus hostile du même compte ; portée en trois classes du §6, le masquage local limité aux effets constatés par HW8 ; création d'une racine absente, seule mutation préparatoire avant la reprise des contrôles (§4.3) ; ouverture en deux branches ; contrôles sous session sur les chaînes tenues, dans l'ordre du §4.3 ; invalidation au premier défaut de périphérique. **S-02** : lettres qui ne sont pas des racines de volume refusées ; chaînes depuis la vraie racine ; distinction du coffre de `sync` et `snapshot` écrite ici, par identité quelle que soit la lettre. Acceptation provisoire du 4 novembre 2026 levée | B5a, accepté avant le code |
| 0008, amendement | P2-4 : « non vérifiable », portée `snapshot`, codes du plan et de `--apply`, issue | B5b, accepté avant son code |
| 0010, nouveau | `doctor` : frontière, ports, ce qu'il ouvre et ce que HD2 en retire, niveaux, sortie, matrice normative des codes par domaine (C3), constats ; deux espaces de noms distincts, codes de constat et causes d'intervention | B5c, accepté avant le code |
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

**Création d'une racine absente : seule mutation préparatoire** (précision
A). C'est l'étape I2 d'ADR 0006 §4 pour un coffre absent, Z2 de §11 pour une
zone neuve.

- **Conditions.** Elle n'a lieu qu'après les contrôles du §4.3 sur le parent
  tenu, et pour une destination prouvée neuve : absente au plan et absente
  sous session. Elle passe par la base, en création exclusive, et n'adopte
  jamais un objet apparu entre-temps.
- **Épingle.** La racine est aussitôt épinglée, son chemin final relu égal à
  la base, son identité lue par handle, puis ajoutée à la chaîne tenue.
- **Reprise.** Les contrôles du §4.3 sont repris sur cette chaîne (étape 4).
  Aucune autre mutation n'a lieu entre la création et la fin de la reprise.
- **Échec de la reprise.** L'opération s'arrête en code 1. Aucun autre objet
  n'est créé : ni marqueur, ni `games`, ni verrou, ni journal. Aucune
  suppression n'est tentée. La racine vide reste, et le message la nomme.
- **Étapes ordinaires.** Elles suivent les contrôles réussis et, si une
  racine a été créée, la reprise réussie de ces contrôles. Ce sont
  les autres étapes d'initialisation d'ADR 0006, inchangées : marqueur,
  `games` et dossier du jeu pour `sync` (I3 à I5), marqueur pour `restore`
  (Z4). Chacune est une création exclusive ou un renommage sans
  remplacement, contrôlée comme toute écriture de la session. Viennent
  ensuite le verrou (I6, Z5) et les mutations de l'opération, sous ce
  verrou.

**Écritures.**

- `CreateDir`, `NewFile`, `Move`, `Hold`, `Identity` et `Lock` joignent la
  base à un nom relatif ; `longPath` ne préfixe plus un chemin qui l'est
  déjà. Aucune lettre n'est résolue après `Open`, ce qu'une instrumentation
  interne au package vérifie en test.
- **Chemin final de chaque nouveau handle** (défense en profondeur, HW8) :
  après `NewFile`, après l'épingle qui suit `CreateDir`, et après `Lock`, le
  chemin final GUID du handle doit égaler la base suivie du nom. Sinon la
  session est invalidée et l'opération refusée.
- **Postcondition de `Move`**, à prouver par HW8c. Avant le renommage, la
  source est ouverte par la base, sans suivi et en partage complet, puis
  refermée. Son chemin final doit égaler la base suivie du nom, et son
  identité est lue. Après le renommage, la destination est contrôlée de même
  et doit porter cette identité. Sinon la session est invalidée.
  - Les basculements de `switchTo` comparent déjà les identités avant et
    après (`publish.go`).
  - Le renommage d'un temporaire vers son nom final n'en compare aucune
    (`apply.go:574`, `restore_apply.go:350`).
- **Effets d'un masquage.** Aucun effet maximal n'est promis avant HW8.
  `CreateDir` écrit avant le contrôle de l'épingle qui le suit. `NewFile`
  écrit avant celui de son handle. `Move` n'ouvre aucun nouveau handle.
  HW8 constate séparément l'effet de chacune, et le résidu écrit au §6 est
  celui qu'il observe.

**Requêtes en lecture seule ajoutées à `Session`.** C'est un changement
d'interface, déclaré, de `Session` et du port `Vault` de `sync`. Il
n'ajoute aucune API d'écriture.

- La revue de sécurité de B5a relit explicitement le port `Vault` et les
  fausses sessions de test (§8, commit 7).
- L'ordre est imposé et testé pour chaque commande et chaque récupération :
  1. `Open` ;
  2. contrôles du §4.3 ;
  3. le cas échéant, création exclusive d'une racine absente, seule
     mutation préparatoire, puis chaîne reconstruite et contrôles repris ;
  4. étapes d'initialisation d'ADR 0006, inchangées, ou reprise d'une
     initialisation interrompue par `--recover --apply` ;
  5. verrou ;
  6. mutations de l'opération, sous le verrou.

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
  Pour `restore --recover`, ces refus portent sur les chemins sauvegardés
  des plans journalisés dans la zone, sans lire le coffre (§3.2).

### 4.3 Contrôles sous session (T4)

Pour `sync --apply`, `snapshot --apply` et `restore --apply`, et pour chaque
`--recover --apply`, reprise du marqueur comprise : après `Open` et **avant
la première écriture**, dans cet ordre. Si la cible est une racine absente,
sa création exclusive est la seule mutation préparatoire permise avant la
reprise des contrôles (§4.1). Dans ce cas, les autres étapes
d'initialisation d'ADR 0006 et le verrou ne suivent qu'une reprise réussie.

1. **Racine de volume** : fait par `Open` (§4.1).
2. **Chaîne tenue égale à la chaîne du plan.**
   - Cible existante : son identité égale celle du plan.
   - Cible absente : l'identité du parent égale celle du plan.
   - Une cible absente au plan et présente sous session donne le code 1,
     sauf les reprises que les règles actuelles prévoient (Z3 de B4).
3. **Chaînes de référence relues.** Installation, sources et, pour
   `restore`, coffre et chemins sauvegardés ; pour `restore --recover`,
   chemins sauvegardés des plans de la zone, sans coffre. Elles sont relues
   après le contrôle S-02 de leur lettre et doivent égaler celles du plan.
   Cela détecte une lettre de référence réattribuée entre le plan et la
   session.
4. **Recouvrements, dans les deux sens.**
   - **La cible n'est dans aucune référence.** L'identité d'aucun objet de
     référence ne figure dans la chaîne tenue. Cette chaîne va de la racine
     du volume à la cible si elle existe, à son parent sinon.
   - **Aucune référence n'est dans la cible**, pour une cible existante
     seulement : son identité ne figure dans aucune chaîne de référence.
   - **Parent d'une cible absente.** Il ne sert qu'à prouver la stabilité
     de l'ancrage (étape 2) et l'absence persistante de la cible (étape 5).
     Il n'est jamais cherché dans une chaîne de référence : il est l'ancêtre
     commun de références sœurs, jusqu'à la racine du volume. Une cible
     absente, dont l'absence est prouvée sous session, ne contient aucune
     référence existante.
   - **Après la création** d'une racine absente (§4.1), la chaîne tenue est
     reconstruite avec la racine créée. Les chaînes de référence sont
     relues, et les deux règles sont reprises sur la nouvelle identité, avant
     toute autre mutation : ni marqueur, ni `games`, ni verrou, ni journal.
     Un refus arrête l'opération en code 1. La racine vide créée reste,
     sans suppression ni autre objet (§4.1).
   - Références de `sync` et `snapshot` : installation et sources. Celles de
     `restore` : coffre, installation, sources actuelles et chemins
     sauvegardés. Celles de `restore --recover` : installation, sources et
     chemins sauvegardés des plans de la zone.
5. **Lectures de décision sur les objets tenus**, par la base de la session
   et sans suivi :
   - marqueurs des ancêtres ;
   - absence d'une cible absente, lue sur le parent tenu ;
   - Z3, « encore vide », lu sur l'objet tenu ;
   - relecture du marqueur du coffre ou de la zone.
6. **Observation impossible** : code 1. Avant la création d'une racine
   absente, rien n'est écrit. Pendant la reprise qui la suit, seule la
   racine vide créée reste, sans suppression.

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
    Les valeurs de comptage ci-dessous sont des **hypothèses de
    dimensionnement**. Le volume d'opérations ne couvre pas tous les cas : M1
    mesure aussi mémoire et temps sur de grands receipts, des noms nombreux et
    longs et une arborescence profonde. M1 confirme ou corrige chaque valeur
    dans le même erratum.

  | Budget | Valeur | Dérivation |
  | --- | --- | --- |
  | Entrées d'un dossier listé | 262 144 | Plus de sept fois les 36 500 entrées d'un dossier qui reçoit une entrée par opération ; les essais restent marginaux, un essai ne subsistant qu'après une interruption |
  | Enregistrements d'un journal | Inchangé | Déjà borné par l'enveloppe E2 : 11 enregistrements, huit essais au plus à côté de chacun |
  | Octets lus par fichier | Inchangé | 64 Mio par enregistrement ou receipt |
  | Octets de métadonnées par lecture | **Mesurée en M1** | Coffre synthétique de 36 500 opérations, mémoire et temps mesurés ; valeur consignée par erratum et acceptée avant le code des budgets et avant ADR 0010 (écart accepté, décision 10) |

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
| Coffre | Marqueur ; journaux et automate ; receipts stricts et leurs chaînes ; identités publiées ; `staging`, `.incomplete`, enveloppe | `current`, chaque génération conservée et chaque snapshot publié relus en flux et comparés à leur receipt ; les seuls snapshots publiés si HD2 exclut les deux premiers (non-interférence, ci-dessous) |
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

**Non-interférence, gate de B5c (T12, C4).** Aucune écriture concurrente
n'échoue du fait de `doctor`, ni en `rolled back`, ni en `busy`, ni
autrement.

- **Ce que qualifie HD2.** HD2 qualifie, classe par classe, chaque ouverture
  par `doctor` d'un objet qu'une écriture épingle ou renomme. Ce sont
  `current`, les générations conservées, les dossiers de staging et les
  `.incomplete`. Les classes sont l'identité, la liste et, sous `--verify`,
  les fichiers lus en flux.
- **Classe non prouvée.** Une classe que HD2 ne prouve pas inoffensive n'est
  jamais ouverte, et le constat qui en dépend est retiré.
  - Pour `--verify`, seuls les snapshots publiés sont relus, au sens d'un
    journal terminal en `published` et d'un receipt validé. L'invariant des
    snapshots publiés d'ADR 0006 interdit de les renommer. La vérification
    de `current` et des générations conservées est reportée (§13).
  - Pour une identité, le constat repose sur l'entrée de répertoire et le
    receipt, et la comparaison d'identité est reportée.
- **Erratum.** Chaque retrait est consigné par un erratum accepté par le
  propriétaire avant le code de `doctor`. `commands/doctor.md` dit ce qui
  n'est pas vérifié.
- **Aucun résidu d'interférence** n'est admis ni remplacé par une consigne
  d'usage.

**Matrice normative des codes**, par domaine, fixée par ADR 0010 (C3).

- **Sens des codes.**
  - `1` : aucun rapport fiable. C'est une erreur d'usage de `doctor`, un
    manifeste illisible, ou un coffre absent, non marqué ou illisible à sa
    racine. Ce n'est jamais un constat, ni la traduction d'un refus d'une
    autre commande.
  - `3` : à récupérer ou indécidable. C'est tout état qu'une commande de
    référence classe 3, sa récupération comprise quand une opération en
    attente concerne l'objet, et tout état que `doctor` ne sait pas
    départager.
  - `2` : à examiner, rien à récupérer. C'est une anomalie qu'une commande
    de référence classe 2, un état qu'elle refuse en code 1 et qui persiste
    sans action de l'utilisateur, ou un domaine sur lequel rien n'a pu être
    conclu.
  - `0` : information seulement.
- **Commande de référence.** C'est celle qui agirait ensuite sur l'objet dans
  cet état, nommée ligne par ligne. Dès qu'une opération en attente concerne
  l'objet, c'est sa récupération. Sinon, c'est la prochaine écriture ou la
  restauration qui le lirait. Une ligne sans commande de référence le dit et
  justifie son code.
- **Cohérence, ligne par ligne.** Le code de `doctor` est au moins celui de
  la commande de référence de la ligne, un refus en code 1 comptant pour 2.
  Aucun ordre global entre commandes n'est employé.
- **Code du rapport.** C'est le plus haut de ses constats, `0` < `2` < `3`.
  `1` n'est rendu que sans rapport. stdout porte le rapport et stderr les
  erreurs, selon ADR 0003 : jamais de rapport partiel avec le code `0`.
- **Lignes liées à HD2.** Une ligne qui repose sur une ouverture retirée par
  HD2 est retirée avec elle, par le même erratum.

| Domaine | État | Code de `doctor` | Commande de référence, et son code |
| --- | --- | --- | --- |
| Coffre, `current` | Sain ; staging d'une opération close | 0, information | `sync` : 0 |
| Coffre, `current` | Opération en attente, non départagée | 3 ; action : `sync --recover`, sans `--apply`, qui sonde le verrou. Un code 3 peut donc signifier « en cours » | `sync` : 3, ou 1 si le verrou est tenu |
| Coffre, `current` | Cause d'intervention relevée par `sync` (`unexpected-entry`, `journal-ambiguous`, enveloppe épuisée, autres causes de `sync.md`) | 3 | `sync` : 3 |
| Coffre, `current` | `current` d'une autre identité que celle de son receipt, sans opération en attente | 3 | Prochaine rotation, `sync --apply` : 3, identité changée |
| Coffre, `current` | Contenu de `current` différent de son receipt (`--verify`), sans opération en attente | 2, `current-modified` | Prochaine rotation : 2, `current-modified` |
| Coffre, `current` | `current` différent de son receipt, en identité ou en contenu, quand une opération en attente le concerne | 3 | Récupération : 3 |
| Coffre, générations conservées | Génération différente de son receipt, en identité ou en contenu, sans opération en attente | 2 | Aucune : hors récupération, aucune commande ne la relit. Anomalie à examiner, rien à récupérer |
| Coffre, générations conservées | Génération différente de son receipt, quand une rotation en attente la relit | 3 | Récupération de la rotation : 3, au moindre écart |
| Coffre, `current` | Cas de la tolérance de B2 (Q5) | 2, limite nommée | `sync` : toléré |
| Snapshots | Snapshot sain ; `.incomplete` d'une opération close | 0, information | `snapshot` : 0 |
| Snapshots | Snapshot dégradé : `snapshot-missing`, `snapshot-rename-lost`, `snapshot-modified`, ou différence au receipt sous `--verify` | 2 ; « non restaurable » | `restore` de ce snapshot : 2, ou 1 s'il est introuvable. `snapshot` avertit sans bloquer |
| Snapshots | Opération de snapshot en attente, non départagée | 3 ; action : `snapshot --recover`, sans `--apply` | `snapshot` : 3, ou 1 si le verrou est tenu |
| Snapshots | `.incomplete` inexpliqué ; `snapshots\receipts` absent à côté d'opérations | 3 ; action, pour le second : recréer le dossier vide | `snapshot` : 3 |
| Snapshots | « Non vérifiable » (P2-4) | 2 ; action : rendre le fichier lisible | `snapshot` : 1, refus persistant |
| Zone | Marqueur d'un autre jeu | 2, constat de cette zone ; le rapport continue | `restore` : 1, refus persistant |
| Zone | Opération en attente, non départagée | 3 ; action : `restore --recover`, sans `--apply` | `restore` : 3, ou 1 si le verrou est tenu |
| Zone | Receipt d'une restauration publiée absent ; journal invalide ; `.incomplete` inexpliqué | 3 | `restore` : 3 |
| Zone | `.incomplete` d'une opération close ; restaurations remises présentes, absentes ou d'une autre identité | 0, information | `restore` : 0, les restaurations remises n'étant pas suivies (ADR 0006 §11) |
| Chemins | Lettre refusée (S-02) ; recouvrement ; lien | 2 | Commandes d'écriture : 1, refus persistant |
| Tout domaine | Budget épuisé | 2 pour ce domaine, rien conclu sur lui | Commandes : 1, refus persistant |
| Tout domaine | Lecture instable : le domaine change encore | 2 pour ce domaine, rien conclu sur lui ; action : relancer `doctor` | Commandes : 1, « another run may be writing this vault » |

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
  - Version stable exacte, **choisie à v2.20.0 ou au-delà**, épinglée
    après la preuve ci-dessous et jamais flottante : ni `@latest`, ni plage,
    ni mise à jour automatique (décision 15). Selon la revue CTO, v2.20.0
    corrige
    une fuite de clés SSH dans `dist/artifacts.json`, bien que ce pipeline
    ne porte ni clé ni publication. Le proxy de modules la donne comme
    dernière version, publiée le 4 novembre 2026, vérifié le 6.
  - Le binaire réellement épinglé est construit et exécuté avec Go 1.27.3 et
    `GOTOOLCHAIN=local`, sur Windows, dans le commit : `goreleaser check`,
    snapshot et smoke.
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

### 5.1 B5a : HW1 à HW13

Tests réels dans `platform/vault`, sur `windows-2025` en CI et sur le poste
de développement, dans des dossiers temporaires. Lettres `subst` et noms DOS
créés et retirés par le test seulement. Exceptions : HW8 et HW13, ainsi
que tout test qui emploie la voie de HW13, tournent en CI seulement. HW7 est prouvé par le propriétaire, et en CI par un VHD si le
runner le permet. HW11 est prouvé en CI par un VHD, ou à défaut par le
propriétaire. **Aucune hypothèse n'est remplacée par une limite écrite** :
sans preuve, le chantier concerné n'est pas fermé.

| # | Hypothèse | Preuve attendue |
| --- | --- | --- |
| HW1 | `GetFinalPathNameByHandle(0x1)` sur un dossier épinglé rend `\\?\Volume{GUID}\…` sur un volume fixe ; par une lettre `subst`, le chemin dans le volume réel. Un tampon trop petit est détecté par la requête de taille | Chemin obtenu ; rouvert, même identité ; troncature refusée |
| HW2 | `CreateDirectory`, `CreateFile` en `CREATE_NEW`, `MoveFileEx` avec `0` et `GetFileInformationByHandle` acceptent ces chemins avec la même sémantique | Erreur 183 pour `CreateDirectory` et `MoveFileEx` sur cible existante, erreur 80 pour `CreateFile` ; erreur 17 entre volumes, testée hors de `Session` ; état inchangé |
| HW3 | Chaque préfixe du chemin de volume, rouvert sans suivi, désigne l'identité de l'épingle correspondante ; une lettre réattribuée pendant `Open` est détectée | Identités égales ; chaîne incohérente refusée |
| HW4 | Une lettre `subst` retirée puis réattribuée vers un autre dossier pendant une session ouverte : les créations et renommages suivants restent dans la cible tenue | Empreinte du témoin identique ; objets créés dans la cible |
| HW5 | Les épingles prises par chemin de volume interdisent toujours le renommage d'un dossier tenu ; un handle ouvert par lettre et un handle ouvert par chemin de volume portent sur le même objet, avec les mêmes conflits de partage | Renommage refusé ; conflits identiques |
| HW6 | Chemins longs et noms Unicode derrière ce préfixe | Création au-delà de `MAX_PATH` |
| HW7 | (a) Volume amovible retiré pendant une session : appels en échec, classés « volume indisponible », jamais redirigés. (b) Retiré puis réinséré : handles morts, session invalidée, rien écrit sur le nouveau montage. (c) Chemin GUID rouvert pendant l'indisponibilité : échec classé | **Obligatoire** (C2). Codes observés consignés. **Test manuel du propriétaire sur un vrai volume amovible**, autorisé à part et nominativement, sur données synthétiques, sans coffre de pilote. En CI, si le runner le permet, un VHD créé, attaché, détaché et rattaché par le test : il prouve le cas d'un volume fixe démonté, jamais celui d'un volume amovible. Le résultat manuel n'est jamais présenté comme passé en CI. Sans HW7 prouvé, S-01 n'est pas fermé et B5a ne fusionne pas |
| HW8 | Un nom DOS local `Volume{GUID}`, défini par `DefineDosDevice` dans la session de connexion du test, masque-t-il le nom global pour `\\?\Volume{GUID}\…` ? Si oui, quel effet a chaque primitive : (a) `CreateDir`, (b) `NewFile`, (c) `Move`, masquage défini avant l'appel et, pour `Move`, entre son contrôle et le renommage, par un crochet de test interne au package | **CI seulement.** Masquage possible : effet de chaque primitive constaté et consigné à part, objet par objet ; refus et invalidation observés ; postcondition de `Move` prouvée ; résidu du §6 écrit d'après ces constats seuls. Impossible : consigné, contrôles gardés en défense |
| HW9 | Cible absente : base égale au chemin du parent tenu suivi du segment ; racine créée en exclusif par cette base, épinglée, chemin final égal à la base, identité lue par handle | Première publication B2b, coffre sans `games`, Z2, reprise Z3, zone existante, récupérations |
| HW10 | `GetVolumeInformationByHandle` sur l'épingle racine rend le système de fichiers ; `GetDriveType` et `GetDiskFreeSpaceEx` acceptent `\\?\Volume{GUID}\` | Mêmes valeurs que par la lettre |
| HW11 | Un point de montage de volume parmi les ancêtres est refusé comme point d'analyse, par lettre et par chemin de volume | Refus `NotFollowedError`. **Obligatoire**, précondition de T20 : en CI par un VHD monté dans un dossier temporaire si le runner le permet, sinon par le propriétaire sur un VHD de test ; aucune limite écrite n'en tient lieu |
| HW12 | Détection S-02 : le chemin final GUID du handle de `X:\` est sans reste pour une vraie lettre de volume, et avec reste pour un alias `subst` et pour un alias `DefineDosDevice` créé par le test. `GetVolumeNameForVolumeMountPoint` échoue pour ces deux alias. L'identité de `X:\` égale celle de `\\?\Volume{GUID}\` pour une vraie lettre | Les trois cas ; contre-épreuve concordante |
| HW13 | Hypothèse de faisabilité, non garantie (décision 23). Précondition, après S-02, de tout test qui réattribue une lettre que S-02 accepte (§6, classe 1 ; §9). Une lettre strictement équivalente à une racine de volume (décision 24) est créée, réattribuée et retirée par le test, sans droits d'administrateur. Voies candidates : `subst` d'une racine de volume ; nom défini par `DefineDosDevice` vers le volume. Pour chacune, on constate les trois conditions de la détection (§3.2), non vérifiées à ce jour. **Banc borné** (P2) : CI seulement, sur le runner, contexte isolé et jetable ; jamais sur le poste ni dans la session interactive du propriétaire, jamais pendant la campagne ni près d'un coffre ou d'une zone de pilote. Un nom DOS local est visible des autres processus de la même session de connexion : c'est pourquoi le banc ne tourne que là. Lettre libre réservée au test : absente de `GetLogicalDrives`, inconnue de `QueryDosDevice` ; aucune lettre préexistante n'est modifiée, aucune lettre définie au démarrage n'est visée. État initial relevé : les 26 lettres et leur cible `QueryDosDevice`. Nettoyage exact : seule la définition créée est retirée, par correspondance exacte. Vérification finale : l'état relevé égale l'état initial, sinon le test échoue. Volume visé : celui de `t.TempDir()`, sans lecture ni écriture hors de `t.TempDir()` | Une voie au moins remplit les trois conditions, puis est réattribuée par le test, état initial rétabli. **Sinon, HW13 tombe** : arrêt, compte rendu et arbitrage du propriétaire (T3). Les tests qui en dépendent ne sont ni sautés ni déclarés neutralisés. Aucun repli vers `mountvol`, un VHD ou des privilèges sans GO distinct du propriétaire |

La réattribution d'une vraie lettre de volume (`mountvol`) demande des
droits d'administrateur et un volume de test : elle n'est pas proposée en
CI. Elle reste hors du modèle de menace (décision 20), sans protection
revendiquée (§6). HW4 exerce `subst`, la voie démontrée par la revue de
sécurité de B4. HW13 cherche, pour les tests qui suivent S-02, une voie
sans droits d'administrateur. S'il n'en trouve aucune, recourir dans ces
tests à `mountvol` ou à un VHD serait un nouvel arbitrage du propriétaire
(T3), jamais un choix implicite. `subst` agit sur toute la session de
connexion : aucun test qui crée une lettre ne tourne pendant la campagne
(§11).

### 5.2 B5c : HD1 et HD2

| # | Hypothèse | Preuve attendue |
| --- | --- | --- |
| HD1 | `ReadDir` rend nom et type des entrées sans les ouvrir : un `Lock` sans partage pris en boucle par un tiers sur `sync.lock` réussit pendant que `doctor` liste `games\<id>` | Aucun échec de `Lock` sur mille itérations |
| HD2 | Chaque classe d'ouverture de `doctor`, en partage lecture, écriture et suppression, ne fait échouer aucune écriture concurrente : ni `Hold`, ni `Lock`, ni le `Move` d'un basculement, ni ses contrôles d'identité. Les classes sont l'identité, la liste et la lecture en flux. Les objets sont `current`, une génération conservée, un staging et un `.incomplete` | Établi classe par classe, sans aucun `rolled back` ni `busy` sur mille itérations. Sinon, la classe est retirée de `doctor` par erratum (§4.5). **Aucun résidu admis** (C4) |

---

## 6. Non-écrasement et limites

Inchangés : créations exclusives, renommage sans remplacement, aucune
suppression, aucun lien suivi, journal avant chaque action.

**Portée de la fermeture de S-01** (précision B, décision 20). Elle porte
sur les écritures et se déclare en trois classes.

- **Au cadrage, aucune garantie de ce tableau n'est prouvée.** Les
  scénarios de la classe 1 sont à démontrer en B5a : leur statut ne passe à
  « prouvé » qu'au rapport B5a, avec l'essai passé, le run et le SHA. Les
  effets de la classe 2 sont à constater par HW8. La classe 3 ne porte que
  des limites écrites, sans garantie.
- Une réattribution ou un alias n'est déclaré neutralisé que sous réserve
  d'exécution réussie de son test (décision 20). Un essai qui ne peut pas
  être exercé, notamment si HW13 tombe, relève de T3 : arrêt, compte rendu
  et arbitrage du propriétaire. Aucune issue n'est fixée d'avance.
- Classe 3, deux catégories distinctes (décision 20). **3a, hors modèle de
  menace** : Savewarden ne prétend pas s'en protéger. **3b, non exercé** :
  aucun essai ne le couvre, et rien n'est garanti. « Hors du périmètre de
  S-01 » est un sous-classement de l'agent, à l'intérieur de 3b, pour ce
  que S-01, limité aux écritures, ne vise pas.
- La décision de fermeture, le rapport B5a et le modèle de sécurité écrit
  dans ADR 0006 §11 reprennent ces classes scénario par scénario, avec leurs
  preuves. Aucun scénario des classes 2 et 3 n'y est dit sécurisé ni
  confiné.

| Classe | Scénario | Preuve exigée | Ce qui sera affirmé | Statut au cadrage |
| --- | --- | --- | --- | --- |
| 1. Neutralisé, une fois prouvé | Lettre du coffre ou de la zone réattribuée pendant une session ouverte, sans droits d'administrateur | HW4, sur HW1, HW2 et HW5 ; après S-02, voie de HW13 ; `TestLetterRemappedDuringTheSession`, `TestRestoreRefusesALetterRemappedAfterTheSession` | Les écritures restent dans la cible tenue ; témoin intact | À démontrer |
| 1. Neutralisé, une fois prouvé | Lettre réattribuée pendant `Open` | HW3 ; après S-02, voie de HW13 ; `TestOpenRefusesAnIncoherentChain` | Chaîne incohérente refusée, rien écrit | À démontrer |
| 1. Neutralisé, une fois prouvé | Lettre du coffre réattribuée entre le plan et l'ouverture | Voie de HW13 ; `TestSyncRefusesALetterRemappedBeforeTheSession`, `TestSnapshotRefusesALetterRemappedBeforeTheSession` | Code 1, rien écrit, témoin intact | À démontrer |
| 1. Neutralisé, une fois prouvé | Lettre d'une référence réattribuée entre le plan et la session : installation, source, coffre pour `restore`, chemin sauvegardé | Voie de HW13 ; `TestReferenceLetterRemappedBeforeTheSessionIsRefused` (décision 20) | Code 1, rien écrit, témoins intacts, référence nommée | À démontrer, test au gate de B5a |
| 1. Neutralisé, une fois prouvé | Alias `subst` d'un sous-dossier (S-02) | `TestAliasLetterIsRefused`, de bout en bout | Refusé partout avant toute écriture | À démontrer |
| 1. Neutralisé, une fois prouvé | Alias `DefineDosDevice` d'un sous-dossier (S-02) | Détection par HW12 ; refus de bout en bout par `TestDefineDosDeviceAliasIsRefused` (décision 20) | Refusé partout avant toute écriture. La détection seule ne suffit pas | À démontrer, test au gate de B5a |
| 1. Neutralisé, une fois prouvé | Volume retiré, puis réinséré, pendant une session | HW7 : VHD en CI pour un volume fixe démonté, protocole du propriétaire pour un vrai volume amovible | Session invalidée ; rien écrit sur le nouveau montage | À démontrer |
| 1. Neutralisé, une fois prouvé | Point de montage parmi les ancêtres | HW11, en CI ou par le propriétaire | Refusé | À démontrer |
| 2. Masquage local, effets constatés seulement | Nom DOS local `Volume{GUID}` défini dans la session de connexion, avant `CreateDir` ou `NewFile`, ou entre le contrôle et le renommage d'un `Move` | HW8a à HW8c, `TestLocalVolumeNameShadowing`, CI seulement | L'effet constaté de chaque primitive, et lui seul. Le contrôle du chemin final et la postcondition de `Move` détectent après coup et invalident la session ; ils ne confinent pas, la primitive ayant pu agir avant. Si HW8 montre le masquage impossible, c'est consigné, et les contrôles restent en défense | À constater |
| 3a. Hors modèle de menace | Processus hostile du même compte ; action d'administrateur, de pilote ou du noyau | Aucune | Rien. Un tel processus peut de toute façon écrire dans le coffre | Limite écrite |
| 3a. Hors modèle de menace | Réattribution administrative d'un volume pendant une session : `mountvol`, gestion des disques | Aucune ; non exercée (§5.1) | **Aucune protection universelle n'est revendiquée** contre une réattribution administrative de volume. Le chemin GUID ne dépend pas de la lettre, mais ce cas n'est ni exercé ni garanti | Limite écrite |
| 3b. Non exercé | Lectures par lettre de `platform/filesystem`. Sous-classement de l'agent : hors du périmètre de S-01 | Aucune | Limite ci-dessous ; S-01 ne porte que sur les écritures | Limite écrite |

Limites qui restent après B5 :

- **Espace de noms local** : classe 2 ci-dessus.
  - `\\?\Volume{GUID}` est résolu par `\??\`, qui cherche d'abord les noms
    DOS de la session de connexion. Un processus du même compte peut définir
    un nom local `Volume{GUID}` sans privilège.
  - **Modèle de menace** : Savewarden protège contre une lettre ou un alias
    réattribué par accident ou par un outil bénin, `subst` ne sachant
    définir qu'une lettre. Il ne protège pas contre un processus hostile du
    même compte, qui peut de toute façon écrire dans le coffre.
  - **Effet d'un masquage** : celui que HW8 constate, séparément pour
    `CreateDir`, `NewFile` et `Move`. Il est écrit ici avec les résultats ;
    aucun effet maximal ni aucun confinement n'est promis, avant comme après
    eux.
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
    (§4.1). Garanti seulement par HW7, obligatoire avant la fermeture de
    S-01.
  - Entre deux exécutions, un volume remonté ailleurs est le même volume
    pour le plan suivant, qui refait tous ses contrôles.
- **Alias.** Les alias de sous-dossier sont refusés en B5 ; leur
  acceptation est reportée (§13). L'équivalent strict d'une racine de
  volume est admis (décision 24). Le contrôle de liens d'`inspect`, qui ne fait que lire, part de la racine de
  la lettre : les dossiers au-dessus de la cible d'un alias ne sont pas
  contrôlés.
- **`doctor`** lit un état qui peut changer pendant sa lecture. La lecture
  stable le borne sans le figer. Aucun résidu d'interférence n'est admis
  (T12) ; une vérification retirée par HD2 est écrite comme non faite.
- **Tolérance de B2** : limite datée (Q5).
- Toutes les limites de B4 restent : restauration remise non suivie, coffre
  copié refusé, falsification coordonnée hors modèle.

---

## 7. Périmètre des fichiers

### 7.1 Par sous-lot

| Zone | B5a | B5b | B5c | B5d |
| --- | --- | --- | --- | --- |
| `internal/platform/vault` | Requête S-02 ; base de volume ; ouverture en deux branches et contrôle de chaîne ; chemin final des handles ; invalidation ; chaîne tenue, volume tenu et lectures de décision ; erreurs sans base ; commentaires (T6) ; tests HW1 à HW13 ; instrumentation | — | — | — |
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
  - Nouveaux refus sous session de `sync`, `snapshot` et de leurs
    récupérations (T4).
  - Nouveaux refus sous session de `restore` et `restore --recover` (T4) :
    chaîne de référence relue qui diffère de celle du plan, coffre et
    chemins sauvegardés compris, ou, pour `restore --recover`, chemins
    sauvegardés des plans de la zone ; recouvrements sur la chaîne tenue
    (§4.3, étape 4) ; reprise après la création d'une racine. Les refus que
    B4 émettait déjà gardent leur message ; les nouveaux nomment la
    référence en cause.
  - Refus après un chemin final hors de la base : nouveau handle, source ou
    destination d'un `Move`.
  - Session invalidée après un défaut de périphérique.
  - Erreurs de `platform/vault` qui nomment un chemin relatif au lieu du
    chemin complet.
  - **Erratum E1, tests de B4 que S-02 change**, accepté dans son principe
    par le CTO (décision 23), sans refonte. Inventaire fait sur `a59d787`,
    par recherche de `subst` et des lettres de faux qui désignent un
    sous-dossier. Chaque cas garde sa valeur probante (P3) : fixture
    construite sans alias, refus attribué à son garde, opération de
    récupération réellement récupérable (§9).
    1. `TestRestoreRefusesASubstAlias`
       (`cmd/savewarden/restore_windows_test.go:382-432`). En B4 : alias
       `subst` d'une source et du coffre ; installation nommée par un alias
       dans le manifeste. Il attend « overlaps the » et « lies in a
       Savewarden vault or restore area », et le code 0 de `sync` et
       `snapshot` à travers l'alias du manifeste, dont il réutilise le
       snapshot. Après S-02, en quatre temps :
       - **Fixture d'abord.** Un snapshot valide est publié par `sync` et
         `snapshot` avec le manifeste et les lettres réels. La suite s'appuie
         sur lui, jamais sur une publication à travers un alias.
       - **Alias de sous-dossier.** Chaque commande est testée à part :
         `sync` et `snapshot` avec le manifeste aliasé ; `restore` dont la
         zone ou le coffre passe par l'alias ; `restore --recover` sur une
         zone par l'alias. Toutes sont refusées par S-02 au plan, code 1,
         coffre et sources inchangés. La zone de `--recover` porte une
         opération réellement récupérable ; son témoin positif, par le vrai
         chemin et après le refus, la récupère en code 0.
       - **Seules les identités départagent.** Le cas d'origine est gardé
         avec une lettre strictement équivalente à une racine (voie de
         HW13, décision 24, CI seulement). Le manifeste nomme l'installation
         par cette lettre ; `sync` et `snapshot` sont acceptés ; `restore`
         vers `saves\Restauration`, nommée par le vrai chemin, est refusé
         par recouvrement (« overlaps the »).
       - **Lettre retirée.** Le manifeste nomme l'installation par une
         lettre retirée : « cannot be proven », inchangé.
    2. `TestRestoreRefusesALetterRemappedBeforeTheSession`
       (`internal/app/restore_remap_windows_test.go:93-178`). En B4 : lettre
       `subst` d'un dossier (`freeAlias`), réattribuée par `remapVaults`
       vers la source `saves` ; il attend « overlaps the source saves » sous
       session. Après S-02, la lettre est un équivalent strict d'une racine,
       accepté au plan (voie de HW13, CI seulement). `remapVaults` la
       réattribue vers `saves` avant `Open`.
       - Le refus vient sous session, de la reprise S-02 de `Open`, avant
         toute écriture. Témoin : le plan a réussi, le double a atteint
         `Open`, l'erreur n'est pas celle du plan, la source est identique.
       - Sur Windows réel, ce test prouve le refus sous session, pas le
         garde de séparation seul : une réattribution vers un sous-dossier
         est arrêtée d'abord par la reprise S-02. Le garde seul est prouvé
         par le cas 4. L'atteindre sur Windows réel demanderait une
         réattribution entre deux racines de volume, avec des fixtures sur
         un second volume, hors de `t.TempDir()` : non proposé.
       - Sous-cas de reprise du marqueur : opération réellement
         récupérable, avec témoin positif.
    3. `TestRestoreDistinctionAcrossDriveLetters`
       (`internal/features/sync/restore_plan_test.go:572-585`, sur faux).
       En B4 : `H:` et `V:` désignent l'installation et le coffre, des
       sous-dossiers du faux, attendus en recouvrement. Après S-02, deux cas
       distincts. Une lettre de sous-dossier reçoit le refus S-02 au plan.
       Le cas d'origine, deux lettres vers les mêmes nœuds, est gardé par des
       équivalents stricts de racines du faux, `H:` pour `D:\` et `V:` pour
       `F:\` : les mêmes destinations, nommées depuis ces lettres, sont
       refusées par recouvrement de l'installation et comme coffre. `W:`,
       équivalent strict de `F:\`, garde son message.
    4. `TestSeparationIsCheckedUnderTheSession`
       (`internal/features/sync/restore_recover_test.go:389-432`, sur
       faux). En B4 : `H:` désigne `G:\Neutre`, un sous-dossier, puis la
       source avant `Open`. Après S-02, c'est **la preuve spécifique du garde
       de séparation sous session**.
       - `H:` est déclarée racine de volume du faux au plan, puis
         réattribuée avant `Open` vers la source, que le faux déclare aussi
         racine de volume. Ni S-02 au plan ni sa reprise dans `Open` ne
         refusent : seul le garde de séparation refuse (« overlaps the
         source saves »). Le faux simule ici une réattribution que Windows
         réel ne permet pas sans second volume.
       - Témoin : `Open` atteint, aucun appel d'écriture. Mutation : le garde
         de séparation retiré dans une copie jetable, le test échoue ; le
         rapport B5a la consigne.
       - Sous-cas de reprise du marqueur : opération réellement
         récupérable, avec témoin positif.

    Assertions gardées : code 1, rien écrit, sources et témoins identiques.
    Chaque refus est attribué à son garde, par son étape et son message :
    plan, reprise S-02 de `Open`, contrôles sous session. Un refus S-02 au
    plan ne requalifie aucun test de contrôle sous session. Les tests
    d'`inspect` ne changent pas : il est hors de la règle (§3.2).
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
- Toute acceptation d'un alias de lettre qui désigne un sous-dossier, et
  de toute lettre qui ne remplit pas les trois conditions de la détection
  S-02 (§3.2). Seul l'équivalent strict d'une racine de volume est admis
  (décision 24).
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

1. acceptation explicite de cette v0.5 par le propriétaire, après une revue
   différentielle courte ;
2. push et PR documentaire de T21, chacun sur GO distinct ;
3. GO de développement distinct, propre à B5a.

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
| 0a | — | v0.5 acceptée | **Acceptation explicite par le propriétaire** |
| 0b | — | PR documentaire (T21) | **Human GO de fusion**, puis GO de développement |
| 1 | B5a | Tests réels HW1 à HW13, sans code de production ; tests VHD de HW7 et HW11 ; protocoles de HW7, et de HW11 s'il n'est pas prouvé en CI, pour le propriétaire | **Résultats HW revus**, HW7 et HW11 compris, avant le commit 2 ; une hypothèse tombée ou non prouvée arrête B5a |
| 2 | B5a | Amendements d'ADR 0005 et 0006 §11, rédigés avec les résultats HW | **Acceptation par le propriétaire** avant le commit 3 |
| 3 | B5a | T4 seul : chaîne tenue et lectures de décision en lecture seule ; chaînes du plan en mémoire ; contrôles sous session avant la première écriture, pour `sync`, `snapshot`, `restore` et toutes les récupérations, cible absente selon C1, création d'une racine absente selon la précision A, ordre imposé du §4.1 ; tests sur faux et réels | — |
| 4 | B5a | S-02 : requête de racine de volume ; refus ; chaînes depuis la vraie racine ; raccourci « même lettre » supprimé ; tests sur faux et réels des deux côtés, pour les trois commandes et leurs récupérations ; refus de bout en bout des alias `subst` et `DefineDosDevice` (décision 20) ; tests de B4 de l'erratum E1 ajustés, preuves conservées (§7.2, décision 23) | — |
| 5 | B5a | Base de volume : ouverture en deux branches, contrôle de chaîne, chemin final des handles, postcondition de `Move`, invalidation, volume tenu, erreurs sans base ; commentaires (T6) | — |
| 6 | B5a | Tests réels de réattribution pendant une session, et d'une lettre de référence entre le plan et la session, références historiques comprises (décision 20), par la voie de HW13 ; requalification de B2 à B4 ; différentiel contre `a59d787`, erreurs comprises | — |
| 7 | B5a | Rapport `docs/validation/b5a.md` ; S-01 et S-02 fermés dans le modèle de sécurité ; documentation des commandes | **Revue de sécurité indépendante, obligatoire**, port `Vault`, fausses sessions et ordre des appels compris, puis gate de S-01 et S-02 et GO de fusion. Le gate exige les deux tests de la décision 20 passés. Un scénario de la classe 1 du §6 n'y est déclaré neutralisé qu'avec son essai passé ; un essai qui ne peut être exercé relève de T3. Le rapport consigne les mutations des gardes prouvés seuls sur faux, séparation et comparaison des chaînes (§9) |
| 8 | B5b | Noms de la matrice B4 (T10), tests seuls | — |
| 9 | B5b | M1 : mesure sur un coffre synthétique, benchmark hors CI ; grands receipts, noms nombreux et longs, profondeur ; valeur en octets et valeurs de comptage consignées par erratum | **Valeurs acceptées** par le propriétaire |
| 10 | B5b | Amendement d'ADR 0008 | **Acceptation** avant le commit 13 |
| 11 | B5b | T7 : fenêtre reproduite sur faux pour les deux domaines et les plans de récupération ; correction ou preuve consignée | — |
| 12 | B5b | Budgets (T8) | — |
| 13 | B5b | P2-4 et N2 | — |
| 14 | B5b | Profondeur JSON, verrou de récupération signalé, `.gitignore` ; rapport `b5b.md` | **Revue indépendante de non-régression**, avec un **regard de sécurité sur N2 et P2-4**, puis GO de fusion |
| 15 | B5c | ADR 0010 et amendement d'ADR 0004 | **Acceptation par le propriétaire** |
| 16 | B5c | Tests réels HD1 et HD2, classe par classe, sans code de production | **Résultats revus** ; toute classe non prouvée est retirée par un erratum accepté avant le commit 18 |
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

Mentions : « binaire », le binaire construit, lancé comme un processus ;
« en processus », le service et les adaptateurs réels, dans le processus du
test ; « VHD », un disque virtuel créé et retiré par le test ; « CI », exercé
sur le SHA en CI et en échec s'il n'y est pas exercé, jamais sauté ; « CI
seulement », de même, et jamais lancé sur le poste de développement ; « CI si le runner
le permet », complété par un protocole du propriétaire sinon. Tout test qui
emploie la voie de HW13 est « CI seulement », sur son banc borné (§5.1).

**Règles des tests de réattribution et des récupérations** (P3, décision
23) :

- **Réattribution entre le plan et la session.** Elle part d'une lettre
  acceptée au plan. Son refus est attribué à un garde sous session, par son
  étape et son message ; un refus S-02 au plan ne la requalifie pas, et
  fait échouer le test. Témoin : le double du port `Vaults` a atteint
  `Open`.
- **Gardes prouvés seuls.** Sur Windows réel, une réattribution vers un
  sous-dossier est arrêtée d'abord par la reprise S-02 sous session. Les
  gardes qui la suivent, séparation et comparaison des chaînes de
  référence, sont prouvés seuls sur faux, chacun avec une mutation que le
  rapport B5a consigne.
- **Opération réellement récupérable.** Un test de `--recover --apply`
  part d'une opération produite et interrompue par le code sous test : sur
  faux, par un crochet de la fausse session ; sur Windows réel, par l'arrêt
  du binaire pendant la copie ou le staging, comme
  `TestRestoreKilledDuringCopyThenRecovered` et
  `TestSyncKilledDuringStagingThenRecovered` en B4. Témoin positif : la
  même fixture, sans l'alias ni la réattribution, est récupérée en code 0.
  Une zone absente ou vide, qui conclut « rien à récupérer », ne prouve
  aucun refus.
- **Références historiques valides.** Un receipt ou un journal qui porte
  un alias n'est jamais forgé. Il est produit par le code sous test, à
  travers une lettre strictement équivalente à une racine (voie de HW13,
  CI seulement), que le test redéfinit ensuite vers un sous-dossier : le
  chemin sauvegardé est valide, et sa lettre désigne désormais un
  sous-dossier.

**B5a, S-01 et S-02**

| Test | Preuve |
| --- | --- |
| `TestLetterRootIsAVolumeRoot` (Windows réel) | HW12 |
| `TestAliasLetterIsRefused` (Windows réel, binaire ; témoin d'équivalent strict en CI seulement) | S-02. Alias `subst` d'un sous-dossier strict de l'installation, d'une source, du coffre et du dossier du jeu dans le coffre. Il est employé comme `--vault` ou `--to`, ou dans le manifeste (installation par un alias de son parent, source). `sync`, `snapshot`, `restore` et chaque `--recover --apply` refusent, code 1, rien créé, empreintes des témoins identiques. Témoins : vraies lettres acceptées ; équivalent strict d'une racine accepté (décision 24, voie de HW13, CI seulement) |
| `TestDefineDosDeviceAliasIsRefused` (Windows réel, binaire, CI seulement) | S-02, décision 20, **obligatoire au gate de B5a**. Alias créé par `DefineDosDevice` sous la forme de HW12, dans la session de connexion du test, et retiré au nettoyage. Il vise un sous-dossier strict de l'installation, d'une source, du coffre et du dossier du jeu, et sert de `--vault` ou de `--to`, ou figure dans le manifeste. `sync`, `snapshot`, `restore` et chaque `--recover --apply` le refusent avant toute écriture : code 1, rien créé, empreintes des témoins identiques. Pour `restore` et `restore --recover`, l'alias figure aussi dans un chemin sauvegardé : receipt du snapshot, ou plan journalisé d'une opération réellement récupérable de la zone, sans ouverture du coffre. Ces références historiques sont produites selon les règles ci-dessus. Le nom DOS suit le banc borné de HW13 : lettre libre réservée, état initial relevé, nettoyage exact, vérification finale. La détection de HW12 n'en tient pas lieu |
| `TestVaultOnAnotherLetterIsComparedByIdentity` | Raccourci supprimé : coffre et installation désignés par deux lettres d'un même arbre, refusé |
| `TestChainsStartAtTheVolumeRoot` (faux et Windows réel) | Première identité de chaque chaîne : la racine du volume |
| `TestSessionWritesThroughTheVolumePath` (Windows réel) | HW1 à HW3 ; l'instrumentation ne voit aucun chemin à lettre après `Open` |
| `TestVolumePathSemantics` (Windows réel) | HW2 : 183, 80 ; 17 hors de `Session` |
| `TestOpenRefusesAnIncoherentChain` (Windows réel) | HW3 : lettre réattribuée pendant `Open`, refus |
| `TestAbsentTargetIsCreatedUnderTheHeldParent` (faux et Windows réel) | HW9 : première publication B2b, coffre sans `games`, Z2, reprise Z3, zone existante, récupérations |
| `TestLetterRemappedDuringTheSession` (Windows réel) | HW4 |
| `TestPinsHoldThroughTheVolumePath` (Windows réel) | HW5 |
| `TestVolumePathLongAndUnicodeNames` (Windows réel) | HW6 |
| `TestLocalVolumeNameShadowing` (Windows réel, CI seulement) | HW8a à HW8c : effet constaté séparément pour `CreateDir`, `NewFile` et `Move` |
| `TestNewHandleOutsideTheBaseIsRefused` | Chemin final hors de la base : refus, session invalidée |
| `TestMoveChecksItsSourceAndDestination` (faux et Windows réel) | Postcondition de `Move` : source hors de la base, rien renommé ; destination hors de la base ou d'une autre identité, session invalidée ; renommage d'un temporaire compris |
| `TestSessionIsInvalidatedAfterADeviceError` | Invalidation et vivacité de l'épingle racine, sur faux |
| `TestVolumeDetachedDuringTheSession` (Windows réel, VHD, CI si le runner le permet) | HW7a à HW7c sur un volume fixe démonté ; complété par le protocole du propriétaire sur un vrai volume amovible, obligatoire |
| `TestSessionVolumeIsDescribedByHandle` (Windows réel) | HW10 |
| `TestMountPointAmongAncestorsIsRefused` (Windows réel, VHD) | HW11, obligatoire : en CI si le runner le permet, sinon protocole du propriétaire |
| `TestAbsentTargetBesideAReferenceIsAccepted` (faux et Windows réel) | C1, cas positifs. Zone absente sœur d'une source, coffre neuf frère de l'installation, cible absente dont le parent est l'ancêtre commun des références ; racine du volume comme ancêtre commun, sur faux. Acceptés, publication normale |
| `TestAbsentTargetUnderAReferenceIsRefused` (faux et Windows réel) | C1, cas négatifs. Zone absente sous une source, sous l'installation ou sous le coffre : code 1, rien écrit |
| `TestExistingTargetContainingAReferenceIsRefused` (faux et Windows réel) | C1. Zone existante qui contient une source ou l'installation : code 1, rien écrit |
| `TestCreatedRootIsCheckedBeforeTheLock` (faux) | C1 et précision A. Une référence apparue sous la racine créée, ou une observation impossible pendant la reprise, est refusée en code 1. Seule la racine vide reste : ni marqueur, ni `games`, ni verrou, ni journal ; aucune suppression tentée |
| `TestSessionOrderIsOpenChecksLockWrites` (faux) | Avis 3 et précision A. La fausse session enregistre l'ordre des appels. Le test échoue sur toute écriture avant la fin des contrôles, toute mutation autre que la création de la racine avant la fin de leur reprise, et tout verrou avant les étapes d'initialisation d'ADR 0006. Pour chaque commande et chaque récupération |
| `TestRestoreRecoverChecksSavedPathsWithoutTheVault` (Windows réel, binaire, CI seulement) | Chemin sauvegardé alias dans le plan journalisé d'une opération réellement récupérable de la zone, produite selon les règles ci-dessus : refus, code 1, rien écrit. Témoin positif : lettre rétablie, l'opération est récupérée en code 0. La trace ne montre aucune ouverture du coffre |
| `TestSessionErrorsNameNoVolumePath` (Windows réel, binaire) | Erreur d'écriture forcée : ni stderr ni l'enregistrement `failed` ne contiennent `\\?\Volume{` ou la base |
| `TestChecksUnderTheSessionUseTheHeldChain` | T4 : recouvrements parent et enfant dans les deux sens, références historiques, zone déplacée, observation impossible ; code 1, rien écrit ; pour `sync`, `snapshot`, `restore` et les récupérations |
| `TestRestoreRefusesALetterRemappedAfterTheSession` (Windows réel, binaire) | Aucune écriture hors de la zone tenue |
| `TestSyncRefusesALetterRemappedBeforeTheSession`, `TestSnapshotRefusesALetterRemappedBeforeTheSession` (Windows réel, en processus, CI seulement) | T4 : lettre du coffre, acceptée au plan, réattribuée entre le plan et l'ouverture, par la voie de HW13 et le double du port `Vaults` décrit ci-dessous ; refus sous session, code 1, **rien écrit, verrou compris**, témoin identique |
| `TestReferenceLetterRemappedBeforeTheSessionIsRefused` (Windows réel, en processus, CI seulement) | T4, étape 3, décision 20, **obligatoire au gate de B5a**. **Mécanisme** : un double du port `Vaults` réattribue la lettre dans `Open`, puis ouvre la vraie session, comme `remapVaults` en B4 (`internal/app/restore_remap_windows_test.go:25-43`). Le test appelle le service avec les adaptateurs réels : le binaire n'a aucun point d'insertion entre `Plan` et `begin` (`apply.go:193`, `:208`), et aucun crochet n'est ajouté au code de production. **Précondition, HW13** : au plan, la lettre de la référence est acceptée par S-02 comme racine de volume, sans quoi elle est refusée avant tout contrôle de chaîne ; si HW13 tombe, T3. Références exercées : installation et source pour `sync`, `snapshot`, `sync --recover` et `snapshot --recover` ; installation, source, coffre et chemin sauvegardé d'un receipt pour `restore` ; chemin sauvegardé d'un plan de la zone pour `restore --recover`. Les récupérations partent d'opérations réellement récupérables, et les chemins sauvegardés sont des références historiques valides, selon les règles ci-dessus. Attendu : code 1, rien écrit, témoins identiques, référence nommée par le message (§7.2). Témoin : plan accepté, double atteint, refus sous session ; un refus au plan fait échouer le test. Lettres créées et retirées par le test seulement, sur le banc borné de HW13 |
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
| `BenchmarkVaultReadAtScale` (hors CI) | M1 : mémoire et temps sur 36 500 opérations par domaine, grands receipts, noms nombreux et longs, arborescence profonde ; valeurs de comptage confirmées ou corrigées |
| Six tests nommés selon la matrice B4 | T10 : mêmes assertions |

**B5c, `doctor`**

| Test | Preuve |
| --- | --- |
| `TestDoctorWritesNothing` (Windows réel, binaire) | Deux exécutions sous ACL de refus d'écriture sur tout l'arbre : sorties et empreintes identiques |
| `TestDoctorNeverOpensLockFiles` (Windows réel) | HD1 ; aucune ouverture de `sync.lock` ni de `restore.lock` dans la trace des lectures |
| `TestDoctorRunsBesideWriters` (Windows réel, binaire) | `sync --apply`, première publication et rotation, `snapshot --apply` et `restore --apply`, pendant que `doctor` tourne en boucle, structurel puis `--verify`. **Toutes les écritures réussissent, code 0 : aucun `rolled back`, aucun `busy`** |
| `TestDoctorOpensOnlyWhatHD2Proved` (Windows réel) | Trace des ouvertures : chacune relève d'une classe prouvée par HD2 ; aucune ouverture d'une classe retirée |
| `TestDoctorHealthyVault` | Coffre synthétique sain, structurel et `--verify` : code 0 |
| `TestDoctorCodesFollowTheTable` | Chaque ligne de la matrice du §4.5 : constat, code et action |
| `TestDoctorAgreesWithItsReferenceCommand` | C3, cas contradictoires, chacun construit deux fois à l'identique : `doctor` sur l'un, la commande de référence sur l'autre. Couvre `current` d'une autre identité ou d'un autre contenu, avec et sans opération en attente ; une génération conservée, avec et sans rotation en attente ; un snapshot dégradé. Le code de `doctor` est celui de la matrice et jamais sous celui de la référence, un refus comptant pour 2 |
| `TestDoctorUsageErrorIsNotAFinding` | Erreur d'usage, manifeste illisible, coffre non marqué : code 1, aucun rapport. Refus d'une commande de référence : constat de code 2, rapport complet |
| `TestDoctorReportCodeIsTheHighestFinding` | Constats 0, 2 et 3 mêlés : code 3 ; 0 et 2 : code 2 |
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
| Smoke d'archive | §4.6 ; binaire de GoReleaser épinglé à sa version exacte, choisie à v2.20.0 ou au-delà, exécuté avec Go 1.27.3 sur Windows |
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

1. **S-01.** HW1 à HW13 prouvés, **HW7 et HW11 compris, sans exception**.
   HW7 est prouvé par le propriétaire sur un vrai volume amovible, et par
   un VHD en CI si le runner le permet. HW11 est prouvé en CI par un VHD, ou
   par le propriétaire. Aucune absence n'est admise : sans ces preuves,
   S-01 n'est pas fermé et B5a ne fusionne pas. HW8 constate séparément les
   effets de `CreateDir`, `NewFile` et `Move`, et le résidu écrit est celui
   qu'il observe. Les tests réels de réattribution pendant une session ne
   montrent aucune écriture hors de la cible tenue. La fermeture est
   déclarée dans les trois classes du §6 : scénarios neutralisés, chacun
   avec son essai passé, dont la réattribution d'une lettre de référence
   entre le plan et la session (décision 20) ; effets de masquage local
   constatés par HW8, sans promesse de confinement ; scénarios hors modèle
   de menace et scénarios non exercés, distingués, jamais présentés comme
   sécurisés.
2. **S-02.** Lettres qui ne sont pas des racines de volume refusées partout
   où T20 le dit, chemins sauvegardés compris. Pour `restore --recover`,
   ce sont ceux des plans de la zone, sans lire le coffre. Chaînes depuis la
   vraie racine ; raccourci « même lettre » supprimé. Tests réels des deux
   côtés pour `sync`, `snapshot`, `restore` et leurs récupérations. Alias
   `subst` et `DefineDosDevice` d'un sous-dossier refusés de bout en bout,
   avant toute écriture ; la détection seule ne ferme pas S-02 (décision
   20). Une lettre strictement équivalente à une racine de volume n'est
   admise qu'aux trois conditions de la détection (décision 24). Une
   cible absente sœur d'une référence est acceptée, et une cible sous une
   référence ou qui en contient une est refusée (C1).
3. Aucune API d'écriture nouvelle ; amendements d'ADR 0005 et 0006 acceptés
   avant le code ; instrumentation de la règle « aucune lettre après
   `Open` ». L'ordre du §4.1 est testé : `Open`, contrôles, création d'une
   racine absente et reprise des contrôles, initialisation d'ADR 0006,
   verrou, mutations. Une reprise qui échoue ne laisse que la racine vide,
   sans autre objet ni suppression. Le port `Vault` et les fausses sessions sont relus par la revue de
   sécurité.
4. Suites B2, B3 et B4 inchangées, hors des tests de l'erratum E1, accepté
   dans son principe (§7.2, décision 23), dont la valeur probante est
   conservée. Sorties identiques au binaire de
   `a59d787` hors du §7.2, erreurs comprises. Tests de différentiel et de
   course exercés sur le SHA exact.
5. S-01 et S-02 fermés par une revue de sécurité indépendante et un gate du
   propriétaire, consignés. La décision consignée reprend les trois classes
   du §6 avec leurs preuves ; la revue vérifie qu'aucun scénario des
   classes 2 et 3 n'est présenté comme sécurisé, et qu'aucun scénario de la
   classe 1 n'est déclaré neutralisé sans son essai passé.
6. Lecture du coffre stable, ou preuve d'absence de fenêtre consignée.
   Budgets appliqués. Valeurs M1 acceptées, octets et comptages, grands
   receipts, noms et profondeur compris. Points de B3 corrigés ou arbitrés ;
   ADR 0008 accepté avant son code.
7. `doctor` n'écrit rien, n'ouvre aucun verrou, ne tient aucune épingle et
   ne fait échouer aucune écriture concurrente. Preuves : test sous ACL de
   refus, et test de concurrence **sans aucun `rolled back` ni `busy`**.
   Chaque ouverture relève d'une classe prouvée par HD2.
8. Chaque constat suit la matrice normative, cas contradictoires testés
   contre leur commande de référence. Le code 1 est réservé à l'absence de
   rapport. Aucune action ne supprime.
9. `--verify` relit en flux `current`, les générations conservées et les
   snapshots publiés. Si HD2 l'a exclu, il ne relit que les snapshots
   publiés, par un erratum accepté avant le code de `doctor`. Les
   restaurations remises ne sont jamais ouvertes pour leur contenu.
10. Frontière de `doctor` contrôlée par les tests d'imports ; ADR 0010 et
    amendement d'ADR 0004 acceptés.
11. Build de contrôle verte en CI : aucun secret, **aucun artefact sur le
    run**, vérifié par l'API ; aucun hook ; `release: disable: true`. Le
    vérificateur refuse les artefacts et les permissions d'écriture.
    GoReleaser est épinglé à une version stable exacte, choisie à v2.20.0
    ou au-delà, après preuve de compatibilité avec Go 1.27.3 sur Windows ;
    aucune version flottante.
12. Revues indépendantes de l'auteur consignées, chacune avec son auteur ou
    son rôle. Une revue du propriétaire, rôle de CTO compris, n'est jamais
    présentée comme une approbation indépendante de lui. Revues attendues :
    sécurité de B5a, port `Vault` et fausses sessions compris ; non-régression et
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
| Acceptation des alias de lettre qui désignent un sous-dossier | Refusés (T20) ; reconsidérée après preuves, chaînes de référence comprises. L'équivalent strict d'une racine de volume est admis dès B5 (décision 24) |
| Vérification du contenu d'une restauration remise par `doctor` | Exclue (Q3) ; demanderait un amendement coordonné d'ADR 0006 §11 et d'ADR 0009 |
| Vérification de `current` et des générations conservées par `doctor`, si HD2 l'exclut | Reportée par erratum ; `--verify` ne relit alors que les snapshots publiés (T12, §4.5) |
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
| Hypothèse Windows fausse sur les chemins de volume | Primitives cassées ou garantie fausse | HW1 à HW13 prouvés avant le code ; arrêt et arbitrage, jamais de repli silencieux |
| Masquage de `Volume{GUID}` par un nom local | Écriture ailleurs | Contrôle du chemin final de chaque handle ; postcondition de `Move` ; HW8a à HW8c ; résidu constaté et modèle de menace écrits |
| Cible absente sœur d'une référence refusée à tort | Zone ou coffre neuf légitime bloqué | Parent jamais cherché dans une chaîne de référence ; tests positifs de frères (C1) |
| Détection S-02 imparfaite | Alias de sous-dossier accepté, pris pour un équivalent strict d'une racine, ou vraie lettre refusée | Trois conditions requises ensemble ; HW12, HW13 et la contre-épreuve ; témoins réels des deux côtés ; refus nommés |
| Aucune lettre acceptée par S-02 n'est réattribuable sans droits d'administrateur (HW13) | Tests de réattribution de la classe 1, dont le test de référence de la décision 20, inexécutables après S-02 ; gate de S-01 bloqué | HW13 prouvé avant le code ; s'il tombe, arrêt et arbitrage du propriétaire (T3), jamais de test sauté ni de recours implicite à `mountvol` ou à un VHD |
| Nouveaux refus pour une configuration légitime | Utilisateur bloqué | Messages qui nomment l'issue, désigner le volume par sa lettre ; documentés |
| Régression de `sync`, `snapshot` ou `restore` | Perte d'une garantie qualifiée | Suites complètes ; différentiel normalisé, erreurs comprises ; revue de sécurité ; campagne obligatoire |
| GUID affiché ou consigné | Sorties changées, données de machine exposées | Erreurs sans base ; test d'erreur forcée ; différentiel |
| Volume réinséré pendant une session | Écriture sur un montage neuf | Invalidation, vivacité de l'épingle racine ; HW7 obligatoire, sur un vrai volume amovible |
| Faux code 3 de la lecture du coffre | Doute injustifié | T7 : reproduction, puis lecture stable |
| Budget trop bas | Coffre légitime refusé | Dimensionnement écrit ; M1 ; message distinct et issue documentée |
| `doctor` pris pour un outil de réparation | Gestes manuels risqués | Actions nommées, jamais de suppression |
| `doctor` qui gêne une écriture | `Lock` ou rotation refusés | Gate strict de T12 ; noms de verrou jamais ouverts ; partage complet ; HD1, HD2 classe par classe ; classe non prouvée retirée ; test de concurrence sans aucun `rolled back` |
| Codes de `doctor` contradictoires | Action mal choisie | Matrice sémantique ; commande de référence par ligne ; cas contradictoires testés (C3) |
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
- résultats HW1 à HW13, ou HD1 et HD2, valeur M1 ;
- différentiel et changements du §7.2 observés ;
- revues indépendantes ;
- résultat de la campagne ;
- écarts au plan, risques résiduels, questions ouvertes ;
- recommandation **sans auto-approbation**.

## Historique

| Version | Date | Commit | Changements |
| --- | --- | --- | --- |
| 0.1 | 5 novembre 2026 | `646d11f`, branche `docs/b5-framing` | Brouillon de cadrage sur le GO du propriétaire du 5 novembre 2026, en documentation seule : S-01 en tête de B5, points ouverts de B4 intégrés. Décisions T1 à T19 proposées ; option A de S-01 et hypothèses HW1 à HW7 ; `doctor` en feature avec ADR 0010 ; points ouverts de B3 ; découpage en quatre PR ; campagne et build de release en snapshot. Aucune revue intégrée, aucun ADR rédigé, aucun GO de développement |
| 0.2 | 6 novembre 2026 | `1bdeb01`, local, branche `docs/b5-framing` | **Arbitrages du propriétaire du 6 novembre 2026 sur Q1 à Q9** (§0.3). Intégration de la revue indépendante r2 et de la revue CTO (§0.4). **S-02 intégré à B5a** (T20) : refus des lettres qui ne sont pas des racines de volume, chaînes depuis la vraie racine, raccourci « même lettre » supprimé. T2 précisé : ouverture en deux branches, cohérence de chaîne, chemin final des handles, invalidation, erreurs sans base, volume tenu. T4 réécrit sur les chaînes tenues, dans les deux sens, avant toute écriture. Hypothèses HW1 à HW12, HD1 et HD2. Budgets sur les métadonnées par lecture, valeurs dérivées, octets mesurés en M1 (écart signalé). `doctor` : ce qu'il ouvre, table normative des codes, présence seule des restaurations remises, un constat par zone, trois options de frontière comparées. Build de contrôle sans artefact ni hook, vérificateur étendu, aucun Go de production en B5d. Campagne et rotation obligatoires. Nouvelles décisions T20 à T22. Contexte mis à jour : B4 clos, release à `a59d787`, PR #15. Aucun ADR rédigé, aucun GO de développement |
| 0.3 | 6 novembre 2026 | `fceaa38`, branche `docs/b5-framing`, présente sur le remote (§1) | **Révision ciblée sur la revue CTO de `1bdeb01`** (§0.5), sans réécriture. C1 : parent d'une cible absente réservé à l'ancrage, chaîne reconstruite après création, tests de frères. C2 : HW7 obligatoire (décision 13) ; HW11 obligatoire aussi, proposé au titre des garanties non testées. C3 : matrice sémantique des codes, commande de référence par ligne, cas de `current` et des générations conservées exhaustifs, snapshot dégradé en 2. C4 : gate strict de T12, HD2 classe par classe, repli de `--verify` sur les snapshots publiés (décision 14). « Objet vide » supprimé ; HW8 en trois constats ; postcondition de `Move`. Décisions 10 à 12 postérieures à `1bdeb01` ; `restore --recover` sans coffre ; ordre imposé et revue du port `Vault` ; M1 élargi ; GoReleaser v2.20.0 au moins. Rôle de CTO tenu par le propriétaire, consigné. Aucun ADR rédigé, aucun GO de push, de fusion ni de développement |
| 0.4 | 7 novembre 2026 | `3c16d23`, local, branche `docs/b5-framing`, non ratifiée | **Précisions A et B de la revue CTO de `fceaa38`** (§0.5), sans refonte. Décisions 15 à 18 : six compléments retenus, HW11 obligatoire et HD2 structurel acceptés. A : création exclusive d'une racine absente, seule mutation préparatoire avant la reprise des contrôles ; échec sans autre objet ni suppression ; étapes d'initialisation d'ADR 0006 inchangées après la reprise (écart à la lettre signalé) ; formules « avant toute écriture » corrigées (T4, §4.1, §4.3, §8, critère 3, tests). B : portée de la fermeture de S-01 en trois classes (§6, §2.1, §3.5, critères 1 et 5). GoReleaser jamais flottant. Push de `fceaa38` constaté et consigné (§1). Aucun ADR rédigé, aucun GO de push, de fusion ni de développement |
| 0.5 proposée, r3 | 7 novembre 2026 | aucun : complément préparé hors du dépôt, sans commit ni push, à partir de `3c16d23` | **Arbitrages CTO D1 à D4 sur `3c16d23`** (§0.5). Décisions 19 à 22, qui citent le CTO mot pour mot et séparent la transposition de l'agent. D1 : texte inchangé. D2 : `TestDefineDosDeviceAliasIsRefused` et `TestReferenceLetterRemappedBeforeTheSessionIsRefused` au gate de B5a ; classe 1 « à démontrer » au cadrage ; classe 3 en deux catégories, 3a hors modèle de menace, où figure `mountvol` avec sa limite, et 3b non exercé (§6, §5.1, §8, §9, critères 1, 2 et 5). D3 : écart de procédure du push de `fceaa38` consigné avec les seuls faits du reflog, sans autorisation rétroactive (§1). D4 : v0.4 non ratifiée avant vérification. **r2, corrections de la revue qualité de r1** : test de référence en processus sur un double du port `Vaults`, précondition HW13 et son risque ; erratum E1 des tests de B4 que S-02 change ; « jamais déclassé » retiré ; refus de `restore` et `restore --recover` au §7.2 ; couverture, légende du §9 et mineurs. HW13 et E1 dépassent la lettre de D4 et sont soumis au CTO. **r3, précisions P1 à P3 de la revue CTO de la r2** (décisions 23 et 24) : équivalent strict d'une racine de volume admis aux trois conditions de la détection, alias de sous-dossier refusés ; banc de HW13 borné, en CI seulement ; preuves d'E1 conservées : fixture sans alias, refus attribué à son garde, mutation du garde de séparation, opérations réellement récupérables, références historiques jamais forgées. Aucun ADR rédigé, aucun GO de push, de fusion ni de développement |

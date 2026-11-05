# Savewarden — Plan B5 : durcissement S-01, diagnostic `doctor` et release candidate

> **Statut : v0.1 — BROUILLON DE CADRAGE, soumis au propriétaire : décisions T1 à T19 proposées, aucune prise ; ni revue indépendante ni revue CTO faites ; aucun ADR rédigé ni accepté ; aucun GO de développement**
> Version du plan : `0.1` — 5 novembre 2026
> Lot : **B5**, cible `release/v0.1.0-a1`
> Cadre : [plan directeur v0.1](Savewarden_Plan_Bootstrap_Architecture_GitOps_v0.1.md) §7 B5,
> [plan B4 v0.2](Savewarden_Plan_B4_Restauration_v0.2.md), [AGENTS.md](../AGENTS.md),
> [modèle de sécurité](../docs/safety-model.md), chantier
> [S-01](../docs/safety-model.md#chantiers-de-sécurité-ouverts), ADR
> [0004](../docs/adr/0004-sync-boundaries.md) à [0009](../docs/adr/0009-restore-contract-receipt-v1.md),
> [rapport B4](../docs/validation/b4.md), [rapport B3](../docs/validation/b3.md).

Le plan directeur range trois choses dans B5. D'abord la commande `doctor`, un
diagnostic en lecture seule des manifestes, chemins, hash, receipts, snapshots
et restaurations remises. Ensuite la campagne pilote complète sur Odyssey,
avec des tests de régression Windows et la preuve de restauration isolée.
Enfin, une fois ces deux points acquis, l'extension du modèle à Cyberpunk 2077
puis à Metal Gear Solid V.

Le 5 novembre 2026, le propriétaire a fixé deux points de cadrage :

- **S-01 entre en tête de B5.** Le durcissement des écritures contre une
  lettre de lecteur réattribuée pendant une session est fait avant `doctor`
  et la campagne. La campagne qualifie alors les primitives durcies, et non
  un code qui changerait après elle.
- **Les quatre points ouverts de B4 entrent dans le cadrage** : les
  commentaires de `platform/vault`, la lecture du coffre par `sync` et
  `snapshot`, les budgets de lecture avec les autres points ouverts de B3, et
  l'alignement des six noms de tests de la matrice B4.

B5 vise la release candidate. Ce plan prépare cette candidate sans la
publier : aucun tag, aucune release, aucune publication de package. La
diffusion publique reste bloquée tant que S-01 n'est pas fermé par son
propre gate de sécurité.

Ce plan ne vaut ni GO de développement, ni acceptation d'ADR, ni autorisation
de push, de tag ou de release, ni accès aux sauvegardes réelles. Il ne
remplace pas la clôture de B4 : selon [AGENTS.md](../AGENTS.md), aucun
développement de B5 ne démarre avant le Human GO de clôture de B4.

---

## 0. Décisions proposées

### 0.1 Décisions T1 à T19

Toutes sont **proposées** ; aucune n'est prise. La question de fond est
celle de T2 : comment une session d'écriture cesse de dépendre d'une lettre
de lecteur une fois ouverte.

| # | Décision proposée | Raison principale | Où |
| --- | --- | --- | --- |
| T1 | **Quatre PR successives** vers `release/v0.1.0-a1` : **B5a** S-01 ; **B5b** lecteurs durcis et points ouverts ; **B5c** `doctor` ; **B5d** campagne pilote et préparation de la RC. Chacune a sa revue, sa CI et son GO de fusion | `platform/vault` est le package le plus sensible : il se relit seul. `doctor` repose sur les lecteurs durcis par B5b. Précédent : B2 en trois PR | §8 |
| T2 | **S-01, option A : la session écrit par un chemin de volume tiré de son épingle.** À l'ouverture, la session épingle ses dossiers comme aujourd'hui. Elle tire ensuite de l'épingle la plus profonde un chemin `\\?\Volume{GUID}\…` par `GetFinalPathNameByHandle`. Toutes les créations, renommages, épingles, identités et le verrou passent ensuite par ce chemin : plus aucune lettre n'est résolue. Aucune API d'écriture nouvelle | Les épingles empêchent déjà de renommer les dossiers du chemin ; seule la lettre restait résolue à chaque appel. L'option B demanderait `NtCreateFile` et `unsafe`, interdits partout par ADR 0005 (§3.1) | §3.1, §4.1 |
| T3 | **Hypothèses Windows confrontées à des tests réels avant le code** : HW1 à HW7 (§5), dans un commit de tests seuls. Si une hypothèse tombe, on s'arrête, on rend compte et le propriétaire arbitre l'option B. Aucun repli silencieux | Même discipline qu'ADR 0006 : les hypothèses Windows sont prouvées avant le code qui en dépend | §5, §8 |
| T4 | **Contrôles sous session sur l'identité tenue.** Les distinctions que `restore` refait une fois la session ouverte, de Z2 à Z6 et en récupération, comparent l'identité de la racine tenue par la session, relue par son chemin de volume ; elles ne résolvent plus de lettre. `sync` et `snapshot`, qui n'ont pas ce contrôle, l'acquièrent : l'identité tenue est comparée à celle du coffre relevée au plan, sinon code 1 | Sinon, une lettre réattribuée entre l'épingle et le contrôle ferait contrôler un autre endroit que celui où l'on écrit. Pour `sync` et `snapshot`, la fenêtre entre le plan et l'ouverture n'est aujourd'hui fermée que par la relecture du marqueur, sans preuve dédiée | §4.1 |
| T5 | **Requalification de B2, B3 et B4**, exigée par la sortie de S-01. Suites complètes inchangées ; différentiel binaire des sorties de `sync`, `snapshot` et `restore` contre le binaire de `4197317`, identiques octet pour octet ; nouveau test Windows réel qui réattribue une lettre `subst` pendant une session ouverte | Les primitives changent de chemin, pas de comportement : la preuve doit le montrer | §9, §12 |
| T6 | **Commentaires de `platform/vault`** (point B4), écrits dans B5a : une session s'ouvre sur un coffre ou sur une zone, et écrit par le chemin de volume de son épingle | Le package est rouvert par S-01 ; c'est le seul moment où le toucher ne coûte pas une revue de plus | §7 |
| T7 | **Lecture du coffre par `sync` et `snapshot`** (point B4). D'abord reproduire sur faux, dans `readProbing`, la fenêtre corrigée pour la zone en `285b370`. Si elle est reproduite, appliquer la même parade : lire jusqu'à deux lectures identiques, trois au plus, sinon code 1 ; sonder le verrou dès qu'une opération en attente est vue. Si elle ne l'est pas, consigner la preuve et ne rien changer | Le défaut est probable, pas démontré. Il n'écrit rien : il donne un faux code 3, sûr mais trompeur. `doctor` lira le coffre avec ce lecteur | §3.2, §4.2 |
| T8 | **Budgets de lecture du coffre et de la zone** (P2-3 de B3, point B4). Une exécution a un budget : nombre d'entrées listées par dossier, nombre de journaux et de receipts, octets lus. Le contenu n'est lu que pour les noms conformes à la grammaire ; un nom non conforme reste signalé comme entrée inattendue. Budget épuisé : code 1, rien conclu, rien écrit. Les valeurs sont proposées en v0.2, après mesure | Un coffre forgé ne doit pas épuiser la mémoire. `doctor` lit tout le coffre : il en a plus besoin que les plans | §4.2 |
| T9 | **Points ouverts de B3** traités dans B5b, chacun selon le §0.2 : P2-4 corrigé avec un amendement d'ADR 0008 ; N2 classé dans `platform/filesystem` ; plafond de profondeur JSON ; verrou créé par la récupération de `current` signalé comme écriture ; `.gitignore`. La tolérance de B2 et `snapshots\receipts` supprimé sont arbitrés au §0.3 | Décision du propriétaire du 5 novembre 2026. `doctor` dépend de P2-4 : il doit distinguer « illisible » de « modifié » | §0.2 |
| T10 | **Noms de la matrice B4** : les six noms du plan deviennent des tests nommés ou des sous-tests, sans changement d'assertion ; la correspondance est consignée | Décision du propriétaire du 5 novembre 2026 ; commit de tests seuls | §8 |
| T11 | **`doctor` est une feature, `features/doctor`.** Elle possède son orchestration et son rapport. Elle consomme des ports étroits qu'elle définit : les méthodes en lecture seule de `features/sync` et `features/inspect` les satisfont, et `app` les compose, comme le port `Inventories` d'ADR 0004. Nouvelle frontière : **ADR 0010** et mise à jour des tests d'imports | `doctor` n'écrit rien et compose plusieurs features : c'est le service supérieur que prévoit l'architecture. En faire une opération de `sync` élargirait encore ce package | §3.3 |
| T12 | **`doctor` n'écrit jamais.** `app` ne lui donne ni `Writer`, ni session, ni verrou. Il ne sonde pas `sync.lock` : une opération en attente est signalée sans être départagée, avec l'action à mener | Sonder le verrou l'ouvrirait un instant et ferait échouer le `Lock` d'un `sync` concurrent (E1 de B4) | §4.3 |
| T13 | **Portée de `doctor`** : manifeste strict ; chemins, distinctions et liens ; coffre (marqueur, `current`, générations conservées, journaux, receipts, snapshots, enveloppe) ; zones de restauration désignées par `--area`, répétable. Le coffre ne connaît pas les zones : `doctor` ne les cherche pas | Le diagnostic porte sur ce que l'utilisateur désigne, comme toutes les commandes. Aucune découverte | §4.3 |
| T14 | **Deux niveaux.** Par défaut, un diagnostic **structurel** : relectures strictes, chaînes de receipts, identités, tailles, sans lire le contenu. Avec `--verify`, `doctor` relit en entier `current`, chaque génération conservée et chaque snapshot publié, et les compare à leur receipt | Le défaut reste rapide. `--verify` comble la limite de B3 : un snapshot n'est plus relu après une rotation | §4.3 |
| T15 | **Codes de `doctor`**, fixés par ADR 0010 : `0` aucun constat au-delà de l'information ; `2` constat à examiner (dégradation, receipt invalide, `.incomplete`, génération modifiée) ; `3` intervention requise (ambiguïté, opération en attente) ; `1` erreur d'usage, lecture impossible ou budget épuisé. Sortie texte ; pas de JSON en B5 | Mêmes sens qu'ADR 0003, 0007 et 0009 : `0` fiable, `2` à examiner, `3` à récupérer, `1` à corriger | §4.3 |
| T16 | **Campagne pilote Odyssey** (B5d) exécutée par le propriétaire, avec le binaire du commit de fusion de B5c, après S-01. Elle requalifie `sync` (première publication, rotation si la source a changé), `snapshot`, `restore` et `doctor` (structurel puis `--verify`), dans un coffre neuf. Les coffres et zones des pilotes précédents ne sont que lus. Les résultats sont consignés sans donnée personnelle | Preuve de bout en bout sur données réelles, sur le code candidat | §11 |
| T17 | **Préparation de la RC** (B5d) : configuration GoReleaser, job CI `check` puis `release --snapshot --skip=publish`, smoke des archives. Seule cible fonctionnelle : Windows amd64. Aucun tag, aucune release, aucun secret. Le job devient un check requis sur décision du propriétaire | Plan directeur §5.2 point 6 et §5.3. `platform/vault` renvoie `ErrUnsupported` hors Windows : distribuer ces binaires tromperait | §4.4 |
| T18 | **Extension à Cyberpunk 2077 puis à Metal Gear Solid V : hors de B5**, dans un lot B6 cadré à part. Elle demande des dossiers connus (`Saved Games`, profil), donc un changement du manifeste v1 et d'ADR 0002, et des lectures nouvelles | Le manifeste v1 refuse les variables et dossiers connus, « prévus avec le pilote Cyberpunk 2077 ». La RC peut porter sur Odyssey seul, à condition de le documenter | §2.2, §13 |
| T19 | **B5 reste sur `release/v0.1.0-a1`.** Le libellé de la première version publiée est fixé au gate de publication, après la fermeture de S-01 | AGENTS.md : toute suite part de `release/v0.1.0-a1`. Rien n'est publié en B5 | §11 |

Écartés d'emblée, hors de B5 : restauration en place, réparation par
`doctor`, nettoyage, rétention, portabilité d'un coffre copié, API
d'écriture nouvelle, sortie JSON, support fonctionnel de Linux et macOS.

### 0.2 Points ouverts de B4 et de B3

| Point | Origine | Proposition | Sous-lot |
| --- | --- | --- | --- |
| Commentaires de `platform/vault` | B4, plan §7 | Écrits (T6) | B5a |
| Lecture non atomique du coffre par `sync` et `snapshot` | B4 | Reproduire, puis corriger ou consigner (T7) | B5b |
| Tests de course de B4 en CI réelle | B4 | **Établi, à consigner à la clôture de B4.** Sur `windows-2025`, `notExercised` fait échouer un test non exercé quand `GITHUB_ACTIONS=true`. Le workflow ne redéfinit pas cette variable. Verify (windows-2025) est vert sur `4197317` : les quatre tests ont donc tourné. C'est une inférence tirée du code et du job ; le journal ne nomme pas les tests, la CI lançant `go test` sans `-v` | Clôture B4 |
| Six noms de tests | B4, matrice §9 | Alignés (T10) | B5b |
| Budget de lecture de la zone | B4 | Budget par exécution (T8) | B5b |
| Budget de lecture du coffre (P2-3) | B3 | Budget par exécution (T8) | B5b |
| Lecture d'un snapshot approuvé (P2-4) | B3 | « Non vérifiable » distinct de `snapshot-modified`, code 1 pour `--apply`, sans dégradation, comme `restore` le fait déjà. Amendement d'ADR 0008 (S13) | B5b |
| Volume indisponible à la porte (N2) | B3 | Type d'erreur « volume non disponible » dans `platform/filesystem`, distinct d'« absent » ; la porte de provenance le classe comme preuve indisponible et non contradictoire | B5b |
| Profondeur JSON (P2-2) | B3 | Plafond explicite dans `strictValue`, indépendant de la chaîne d'outils | B5b |
| Verrou créé par la récupération de `current` | B3, hérité de B2 | Signalé comme une écriture, comme M1 l'a fait pour un snapshot | B5b |
| `.gitignore` (M7) | B3 | Ajouter `**/journal/`, `*.rec`, `**/staging/`, `**/retained/`, `savewarden-vault.json` | B5b |
| Tolérance de B2 (`preparedFor`) | B3 | **Report proposé**, documenté : l'impact est faible, et toucher la récupération de la rotation juste avant la RC coûte une requalification de B2c. `doctor` signale ce cas (§0.3, Q5) | — |
| `snapshots\receipts` supprimé | B3 | Code 3 inchangé dans `sync` ; `doctor` nomme le cas et la procédure sans suppression : recréer le dossier vide | B5c |
| Concurrence réelle du marqueur | B4 | Inchangée : la logique est prouvée de façon déterministe sur faux | — |

### 0.3 Questions au propriétaire

1. **Option A ou B pour S-01 (T2).** A écrit par le chemin de volume de
   l'épingle, sans API d'écriture nouvelle. B crée et renomme relativement
   aux handles : il faudrait autoriser `NtCreateFile`, un renommage par
   handle et `unsafe`, que l'ADR 0005 interdit partout aujourd'hui.
2. **Lettres `subst` comme coffre ou zone.** Avec A, une session ouverte sur
   une lettre `subst` écrit dans le volume réel, quoi qu'il arrive ensuite à
   la lettre. On propose de les accepter, sous les distinctions par identité.
   Le comportement actuel envers un coffre ouvert par une lettre `subst` est
   à confirmer par un test avant la v0.2.
3. **`doctor` et les restaurations remises.** R8 de B4 : une restauration
   remise « n'est plus vérifiée ». On propose qu'avec `--verify`, `doctor` la
   relise par la portée sans suivi d'`inspect` et la compare à son receipt,
   **en information seulement**, jamais bloquante, l'utilisateur pouvant
   l'avoir modifiée. Cela demande une note dans ADR 0009. L'alternative :
   présence seulement, comme aujourd'hui.
4. **Codes de `doctor` (T15)**, et notamment `3` pour une opération en
   attente que `doctor` ne départage pas (T12).
5. **Tolérance de B2** : report documenté (§0.2), ou correction dans B5b avec
   amendement d'ADR 0006 §3 et requalification de B2c.
6. **Cyberpunk 2077 et Metal Gear Solid V en B6 (T18)**, la RC portant sur
   Odyssey seul.
7. **Définition de la diffusion publique.** Le dépôt
   `savewarden/savewarden-cli` est **public** sur GitHub : le code, le modèle
   de sécurité et la description de S-01 y sont déjà lisibles. On propose
   que la diffusion bloquée par S-01 soit : tout tag, toute release, tout
   binaire distribué, tout canal de paquets et toute annonce. La visibilité
   du code n'en fait pas partie.
8. **Cibles de la RC (T17)** : Windows amd64 seul ; Windows arm64 non
   proposé tant qu'aucun runner arm64 ne le teste. Linux et macOS ne sont pas
   distribués. SBOM et attestations : à décider avant le premier tag.
9. **Valeurs des budgets (T8)**, proposées en v0.2 après mesure sur un coffre
   synthétique de grande taille.

---

## 1. Contexte et preuves

| Élément | État vérifié le 5 novembre 2026 |
| --- | --- |
| `release/v0.1.0-a1` | `4197317`, merge de la PR #13, B4 ; tête du remote |
| CI | Run 41832056655 sur `4197317` : 7/7, `success`, relu par `gh run view` |
| `main` | `25d1cc2`, bootstrap B0 seul |
| Clôture de B4 | Branche locale `docs/b4-closure-o1u3ft` sur `7ccf7c7`, un commit au-dessus de `4197317`, non poussée ; pilote B4 à exécuter par le propriétaire, puis à consigner ; une seule PR de clôture, sur décision du propriétaire du 5 novembre 2026 |
| Cadrage B5 | Ouvert sur le GO du propriétaire du 5 novembre 2026, en documentation seule ; branche locale `docs/b5-framing`, créée depuis `4197317` ; sans push |
| Visibilité | Dépôt **public** (`gh repo view`) |
| Toolchain | Go 1.27.3, golangci-lint 2.14.0, `golang.org/x/sys` v0.49.0 |
| Release | Aucune configuration GoReleaser ; un seul workflow, `ci.yml` ; aucun tag |

Constats du code, relus à `4197317` :

- **Session d'écriture.** `Session` garde `base`, le chemin à lettre passé à
  `Open` (`platform/vault/vault_windows.go`). Chaque nom est résolu par
  `absolute`, qui joint `base` et le nom, puis préfixé `\\?\` par
  `longPath`. `CreateDirectory`, `CreateFile` (`CREATE_NEW`), `MoveFileEx`
  (drapeaux `0`), `Identity`, `Hold` et `Lock` ouvrent tous **par chemin
  complet, donc par lettre**. Les épingles (`openDirectory`, partage sans
  `FILE_SHARE_DELETE`) empêchent de renommer ou supprimer les dossiers tenus,
  ancêtres compris depuis la racine du volume. Elles n'empêchent pas qu'une
  lettre désigne un autre volume ou un autre dossier entre deux appels :
  c'est S-01.
- **API disponibles.** `x/sys/windows` v0.49.0 fournit `GetFinalPathNameByHandle`
  et `GetVolumeNameForVolumeMountPoint`, deux requêtes en lecture seule. La
  constante `VOLUME_NAME_GUID` (valeur `0x1` dans l'API Win32) n'y est pas
  définie : `platform/vault` devrait la déclarer. `NtCreateFile` y figure, mais
  le champ `Length` de son `OBJECT_ATTRIBUTES` ne se remplit qu'avec
  `unsafe.Sizeof` : `x/sys` lui-même ne le fait que dans ses tests. `unsafe` et
  `NtCreateFile` sont interdits partout par ADR 0005 et `readonly_test.go`.
- **Distinctions sous session.** Depuis `5184d51`, `restore` refait ses
  distinctions une fois la session ouverte. Il passe par les lecteurs de
  `platform/filesystem`, donc par lettre. `TestRestoreRefusesALetterRemappedBeforeTheSession`
  prouve la fermeture de la fenêtre qui précède l'ouverture, pas celle qui la
  suit.
- **Lecture du coffre.** `readProbing` (`features/sync/vault.go`) liste la
  racine, lit le marqueur, le dossier du jeu, ses journaux et ses receipts, et
  ne sonde le verrou qu'**à la fin**. Avant `285b370`, la lecture de la zone
  avait cette forme et donnait un faux code 3 quand une publication
  concurrente se terminait entre deux lectures. `readStableArea`
  (`features/sync/restore_area.go`) en est la parade. `--apply` tient son
  propre verrou et ne sonde pas : la fenêtre ne concerne que les plans, et
  demain `doctor`.
- **Budgets.** Chaque fichier lu est borné, 64 Mio pour un enregistrement ou
  un receipt, mais ni le nombre d'entrées ni le total ne le sont.
- **Manifeste.** Le manifeste v1 n'accepte qu'une base `game-install` et
  refuse les variables et dossiers connus, « prévus avec le pilote Cyberpunk
  2077 » (`docs/manifest-v1.md`).
- **Architecture.** `docs/architecture.md` prévoit déjà une feature
  `doctor` qui possède son orchestration. Une feature n'importe pas une autre
  feature : un scénario composé passe par un service supérieur explicitement
  conçu et un ADR.
- **Tests de course de B4.** Voir le §0.2 : leur exécution en CI réelle est
  établie par inférence sur `4197317`.

---

## 2. Objectif, non-objectifs, permissions

### 2.1 Objectif

À la fin de B5 :

1. **S-01 fermé** : une fois une session ouverte, aucune création, aucun
   renommage, aucune épingle, aucune identité ni aucun verrou ne résout plus
   de lettre de lecteur. Un test Windows réel réattribue une lettre pendant
   une session ouverte et montre qu'aucune écriture ne sort de la cible
   tenue. B2, B3 et B4 sont requalifiés et un gate de sécurité est passé.
2. **Lecteurs durcis** : la lecture du coffre est stable ou la preuve est
   consignée ; les budgets de lecture sont appliqués ; les points ouverts de
   B3 sont corrigés ou arbitrés.
3. **Diagnostic** :

   ```text
   savewarden doctor --manifest <m> --vault <v> [--area <dest>]... [--verify]
   ```

   Le rapport, en lecture seule, donne pour chaque constat son objet, son
   code stable et l'action à mener. Il ne répare rien.
4. **Campagne** : Odyssey requalifié de bout en bout sur le code candidat,
   par le propriétaire ; build de release en snapshot vérifiée en CI, sans
   publication.

### 2.2 Non-objectifs B5, opposables en revue

- **Réparation, nettoyage ou adoption par `doctor`** : il diagnostique et
  nomme l'action, jamais ne l'exécute.
- **Restauration en place**, restauration partielle, rétention, suppression.
- **Cyberpunk 2077 et Metal Gear Solid V** : lot B6 proposé (T18), avec le
  changement du manifeste.
- **Portabilité d'un coffre copié** vers un autre volume (R15 de B4).
- **Toute API d'écriture nouvelle**, tout drapeau de remplacement.
- **Tag, release, publication**, canaux Scoop, Homebrew ou Winget.
- Sortie JSON de la CLI ; support fonctionnel de Linux et macOS.
- Durcissement des **lectures** contre une lettre réattribuée : hors de S-01,
  limite écrite au §6.

### 2.3 Permissions : agent, produit, propriétaire

| Acteur | Peut | Ne peut pas |
| --- | --- | --- |
| **Agent** | Lire et modifier le dépôt ; tests synthétiques en `t.TempDir()` ; lettres `subst` temporaires créées et retirées par les tests, vers des dossiers temporaires | Lire `D:\Ga`, `D:\Savewarden`, `E:\GMS`, un coffre ou une zone de pilote, ou des clés SSH ; lancer Savewarden sur des données réelles sans autorisation explicite et nominative pour cette exécution ; modifier une lettre de lecteur qui n'a pas été créée par un test |
| **Savewarden**, lancé par le propriétaire | `doctor` : lire le manifeste, le coffre désigné et les zones désignées, sans suivi de lien ; les autres commandes, comme en B4 | `doctor` : écrire, verrouiller, sonder `sync.lock`, ouvrir une session, réparer |
| **Propriétaire** | Exécuter la campagne ; autoriser nominativement une exécution par l'agent ; donner chaque GO | — |

---

## 3. Architecture et ADR

### 3.1 S-01 : où cesser de résoudre une lettre (T2)

| Option | Mécanisme | Pour | Contre |
| --- | --- | --- | --- |
| **A, retenue** | Chemin de volume tiré de l'épingle la plus profonde par `GetFinalPathNameByHandle(VOLUME_NAME_GUID)`, utilisé ensuite pour tout appel | Aucune API d'écriture nouvelle ; les mêmes `CreateDirectory`, `CreateFile` et `MoveFileEx` ; une requête en lecture ajoutée à ADR 0005 ; `subst` désamorcé, le chemin désignant le volume réel | Repose sur les épingles pour figer les dossiers à l'intérieur du volume, ce qu'elles font déjà ; HW1 à HW7 à prouver |
| B | Créations relatives au handle du dossier parent (`NtCreateFile` avec `RootDirectory`, `FILE_CREATE`) ; renommage par handle sans remplacement | Plus aucun chemin, même dans le volume | `NtCreateFile`, `SetFileInformationByHandle` et `unsafe` interdits partout ; la même API de renommage supprime par `FileDispositionInfo` ; élargit la surface d'écriture contre l'esprit d'ADR 0005 |
| C | Contrôler après chaque écriture que la cible est sous la racine tenue | Aucun changement de chemin | Détecte sans empêcher : l'écriture a déjà eu lieu ailleurs ; ne satisfait pas la sortie de S-01 |

Avec A, la session reste ouverte par un chemin à lettre, seule forme que
l'utilisateur donne. Elle épingle ses dossiers comme aujourd'hui. Elle tire
de l'épingle la plus profonde, le coffre ou son parent s'il n'existe pas
encore, le chemin de volume. Elle contrôle que ce chemin désigne bien la même
identité que l'épingle (HW3), puis l'adopte comme base. Les diagnostics
continuent de nommer les entrées relativement au coffre ou à la zone : aucune
sortie ne change, aucun GUID n'apparaît.

### 3.2 Lecteurs (T7, T8, T9)

Aucune frontière ne change. `features/sync` reçoit la lecture stable du
coffre, sur le modèle de `readStableArea`, et les budgets dans `list`,
`readJournals` et `readReceipts`. `entities/backup` reçoit le plafond de
profondeur JSON. `platform/filesystem` reçoit le type d'erreur du volume
indisponible (N2), en lecture seule.

### 3.3 `doctor` : une feature et un ADR (T11)

```text
cmd/savewarden → app ─┬─► cli ─┬─► features/inspect ──► entities/*
                      │        ├─► features/sync ─────► entities/*
                      │        └─► features/doctor ───► entities/*   (ports définis par doctor)
                      ├─► platform/manifests, platform/filesystem
                      └─► platform/vault                              (jamais donné à doctor)
```

- `features/doctor` définit ses ports : `ManifestReader`, un port de lecture
  du coffre et de la zone, un port d'inventaire sans suivi. `app` les remplit
  avec des méthodes en lecture seule de `sync.Service` et
  d'`inspect.Service`, aux signatures exactes, comme `Inventories` pour
  `sync`.
- Les types qui traversent la frontière, observations du coffre, de la zone
  et des générations, vivent dans `entities/backup`, sans I/O. `doctor`
  n'importe ni `sync` ni `inspect`.
- `sync` gagne des méthodes de **relevé**, en lecture seule, qui exposent ce
  que ses plans lisent déjà. Il n'expose aucun moteur d'écriture.
- ADR 0010 fixe la frontière, le contrat de sortie, les codes et les codes
  stables des constats. `architecture_test.go` apprend la feature ;
  `readonly_test.go` la couvre sans changement, comme tout package.

Alternative écartée : `doctor` comme opération de `features/sync`. Ce serait
plus court, mais le package porterait une cinquième responsabilité, qui
n'écrit rien et lit aussi les manifestes et les sources.

### 3.4 ADR à rédiger

| ADR | Objet | Sous-lot |
| --- | --- | --- |
| 0005, amendement | `GetFinalPathNameByHandle`, requête en lecture seule, et la constante `VOLUME_NAME_GUID` déclarée dans `platform/vault` ; règle : après `Open`, plus aucun chemin à lettre ; liste du garde mise à jour, listes attendues indépendantes comprises | B5a |
| 0006 §11, amendement | Fenêtre des lettres de lecteur **fermée pour les écritures** ; contrôles sous session sur l'identité tenue ; acceptation provisoire du 4 novembre 2026 levée | B5a |
| 0008, amendement | P2-4 : « non vérifiable » distinct de `snapshot-modified` (S13) | B5b |
| 0010, nouveau | `doctor` : frontière, ports, niveaux, sortie, codes, constats | B5c |
| 0009, note | Restaurations remises relues par `doctor --verify`, en information, si Q3 est retenue | B5c |
| 0004, note | `sync` expose des relevés en lecture seule à `doctor` ; critères de réexamen inchangés | B5c |

---

## 4. Contrats

### 4.1 Primitives de `platform/vault` après S-01

- **Inchangé** : la surface de `Session` (`Open`, `CreateDir`, `Hold`,
  `Release`, `NewFile`, `Move`, `Identity`, `Lock`, `Close`), les API
  d'écriture, les drapeaux, les erreurs et leur classement (`BusyError`,
  `NotFollowedError`, `fs.ErrExist`, volume indisponible).
- **Changé** : `base` devient le chemin de volume tiré de l'épingle ;
  `absolute` le joint aux noms ; `longPath` ne préfixe plus un chemin qui
  l'est déjà. Aucune lettre n'est résolue après `Open`.
- **Contrôle à l'ouverture** : le chemin de volume, rouvert sans suivi,
  désigne la même identité, volume et index, que l'épingle ; sinon refus,
  sans écriture.
- **`Describe`**, requête de type de volume et d'espace libre : appelée
  sur la base de la session quand une session existe, sur la lettre sinon
  (plan).
- **Contrôles sous session (T4)** : la feature compare l'identité de la
  racine tenue, `Identity(".")` relue par le chemin de volume, aux identités
  de référence relevées au plan : pour `restore`, celles du coffre, de
  l'installation et des sources ; pour `sync` et `snapshot`, celle du coffre.
  Une différence : code 1, comme une lettre réattribuée avant l'ouverture.
  Le plan de `sync` et de `snapshot` relève donc l'identité du coffre quand
  il existe ; le format du journal ne change pas, l'identité restant en
  mémoire pendant l'exécution.

### 4.2 Lecteurs

- **Lecture stable du coffre (T7)** : si la fenêtre est reproduite, le plan
  lit le coffre jusqu'à deux lectures identiques, trois au plus. Un coffre
  qui change encore : code 1, « another run may be writing this vault », rien
  conclu. Le verrou est sondé dès qu'une opération en attente est vue, et non
  plus à la fin. `--apply` est inchangé, sous son propre verrou.
- **Budgets (T8)** : nombre d'entrées par dossier listé, nombre de journaux
  et de receipts lus, octets lus au total par exécution. Épuisé : code 1,
  « vault too large to read within the budget », rien conclu. Les noms non
  conformes à la grammaire ne sont jamais lus.
- **P2-4** : une identité illisible ou un fichier tenu par un autre programme
  rend le snapshot candidat « non vérifiable » : code 1 pour `--apply`, sans
  dégradation ni second snapshot. Une différence observée reste
  `snapshot-modified`.
- **N2** : un volume non prêt n'est plus traduit en « absent » ; la porte de
  provenance le classe comme preuve indisponible, rollback inchangé.

### 4.3 `doctor`

```text
savewarden doctor --manifest <m> --vault <v> [--area <dest>]... [--verify]
```

| Objet | Structurel, par défaut | Avec `--verify` |
| --- | --- | --- |
| Manifeste | Lecture stricte, ADR 0002 | — |
| Installation et sources | Existence, liens, distinctions par segments et par identité | — |
| Coffre | Marqueur ; journaux et automate ; receipts stricts et leurs chaînes ; identités publiées ; `staging`, `.incomplete`, enveloppe | `current`, chaque génération conservée et chaque snapshot publié relus en entier et comparés à leur receipt |
| Zone, par `--area` | Marqueur strict du même jeu ; journaux ; receipts ; `.incomplete` ; restaurations remises présentes, par `Lstat` | Restaurations remises relues et comparées à leur receipt, en information (Q3) |

- **Constats** : chaque constat porte un code stable documenté, l'objet
  relatif au coffre ou à la zone, et l'action à mener. Par exemple :
  `sync --recover --apply`, recréer `snapshots\receipts` vide, créer un coffre
  neuf. Jamais une suppression. Les codes existants (`snapshot-modified`,
  `journal-ambiguous`…) sont repris tels quels ; les nouveaux sont fixés par
  ADR 0010.
- **Codes (T15)** : `0`, `2`, `3`, `1`, selon le §0.1. Le rapport va sur
  stdout, les erreurs sur stderr, selon les règles d'ADR 0003 : jamais de
  rapport partiel avec le code `0`.
- **Garanties (T12)** : aucun `Writer`, aucune session, aucun verrou, aucune
  sonde de `sync.lock`. Lecture stable (T7) et budgets (T8). Aucun lien suivi.
  Restaurations remises jamais ouvertes sans `--verify`.
- **Limites** : `doctor` voit l'état au moment de sa lecture. Une opération
  en attente est signalée sans être départagée entre « en cours » et
  « interrompue » ; l'action dit de relancer `doctor` si un `sync` tourne.

### 4.4 Build de release en snapshot (T17)

- `.goreleaser.yaml` : `windows/amd64`, version injectée par
  `-ldflags="-X main.version=…"`, archive zip et `checksums.txt`.
- Job CI « Release snapshot » : GoReleaser à version exacte, action épinglée
  par SHA complet, `contents: read`, aucun secret ; `goreleaser check`, puis
  `goreleaser release --snapshot --skip=publish --clean`, puis smoke de
  l'archive : `savewarden version` et `savewarden --help` déterministes.
- Aucun tag, aucune release, aucun brouillon de release. L'ajout du job aux
  checks requis du ruleset est une décision du propriétaire.

### 4.5 Compatibilité

Aucun format ne change : receipt v1, receipt de snapshot v1, receipt de
restauration v1, marqueurs v1, enregistrements de journal. Un binaire de
`4197317` lit un coffre et une zone écrits par B5 avec les mêmes sorties. Le
différentiel binaire le vérifie, dans les deux sens.

---

## 5. Hypothèses Windows de S-01, à prouver avant le code (T3)

Tests réels dans `platform/vault`, sur `windows-2025` en CI et sur le poste
de développement, dans des dossiers temporaires. Lettres `subst` créées et
retirées par le test seulement.

| # | Hypothèse | Preuve attendue |
| --- | --- | --- |
| HW1 | `GetFinalPathNameByHandle(VOLUME_NAME_GUID)` sur un dossier épinglé rend `\\?\Volume{GUID}\…`, sur un volume fixe ; par une lettre `subst`, il rend le chemin dans le volume réel | Chemin obtenu ; rouvert, même identité |
| HW2 | `CreateDirectory`, `CreateFile` en `CREATE_NEW`, `MoveFileEx` avec `0` et `GetFileInformationByHandle` acceptent ces chemins avec la même sémantique : erreur 183 sur cible existante, erreur 17 entre volumes | Codes d'erreur observés, état inchangé |
| HW3 | Un chemin de volume rouvert sans suivi désigne l'identité de l'épingle | Volume et index égaux |
| HW4 | Une lettre `subst` retirée puis réattribuée vers un autre dossier pendant qu'une session est ouverte : les créations et renommages suivants restent dans la cible tenue ; rien n'apparaît dans l'autre dossier | Empreinte de l'autre dossier identique ; objets créés dans la cible |
| HW5 | Les épingles prises par chemin de volume interdisent toujours le renommage d'un dossier tenu | Renommage refusé tant que l'épingle est tenue |
| HW6 | Chemins longs et noms Unicode fonctionnent derrière ce préfixe | Création au-delà de `MAX_PATH` |
| HW7 | Un volume amovible retiré pendant une session fait échouer les appels, classés « volume indisponible », sans jamais les rediriger | Échec classé, aucune écriture ailleurs ; test manuel du propriétaire si la CI ne peut pas retirer un volume |

La réattribution d'une vraie lettre de volume (`mountvol`) demande des
droits d'administrateur et un volume de test : elle n'est pas proposée en CI.
HW4 la représente par `subst`, qui est la voie démontrée par la revue de
sécurité de B4.

---

## 6. Non-écrasement et limites

Inchangés : créations exclusives, renommage sans remplacement, aucune
suppression, aucun lien suivi, journal avant chaque action.

Limites qui restent après B5 :

- **Lectures.** Les lectures de `platform/filesystem` résolvent encore des
  lettres. Une lettre réattribuée pendant une lecture fait lire un autre
  contenu. Les gardes existants le détectent sans le garantir : inventaire
  fait deux fois avec contrôle des identités, copie hachée contre le plan,
  relecture du snapshot. L'opération échoue alors, sans rien publier. Cette
  limite est écrite ; elle ne bloque pas la diffusion, aucune écriture ne
  pouvant en sortir de la cible.
- **Volume entier.** Le chemin de volume suit le volume, pas la lettre. Un
  volume démonté puis remonté ailleurs reste le même volume : c'est voulu.
- `doctor` lit un état qui peut changer pendant sa lecture ; la lecture
  stable le borne, sans le figer.
- Toutes les limites de B4 restent : restauration remise non suivie, coffre
  copié refusé, falsification coordonnée hors modèle.

---

## 7. Périmètre des fichiers

| Zone | B5a | B5b | B5c | B5d |
| --- | --- | --- | --- | --- |
| `internal/platform/vault` | Base de volume, contrôle à l'ouverture, `Describe`, commentaires ; tests HW1 à HW7 | — | — | — |
| `internal/app` | `readonly_test.go` : requête en lecture ajoutée, listes attendues indépendantes | — | `app.go`, composition de `doctor` ; `architecture_test.go` | — |
| `internal/features/sync` | Contrôles sous session sur l'identité tenue | Lecture stable, budgets, P2-4, verrou de récupération signalé ; noms de la matrice B4 | Méthodes de relevé en lecture seule | — |
| `internal/features/doctor` | — | — | Feature, ports, rapport, tests | — |
| `internal/entities/backup` | — | Plafond de profondeur JSON | Types d'observation partagés | — |
| `internal/platform/filesystem` | — | Erreur de volume indisponible (N2) | — | — |
| `internal/cli` | — | — | `doctor.go`, tests ; `root.go` | — |
| `cmd/savewarden` | Test binaire de non-régression ; différentiel | Différentiel | Tests binaires Windows de `doctor` | Smoke d'archive |
| `.github` | — | — | — | Job « Release snapshot » |
| Racine | `CHANGELOG.md`, `AGENTS.md` | `.gitignore`, `CHANGELOG.md` | `README.md`, `CHANGELOG.md` | `.goreleaser.yaml`, `CHANGELOG.md` |
| `docs/` | ADR 0005 et 0006 ; `safety-model.md`, S-01 fermé ; `validation/b5a.md` | ADR 0008 ; `vault.md` ; `validation/b5b.md` | ADR 0010, notes 0004 et 0009 ; `commands/doctor.md` ; `architecture.md` ; `validation/b5c.md` | `gitops.md` ; `pilots/odyssey.md` ; `validation/b5d.md` |

Changements interdits dans tout B5 :

- toute API d'écriture, de suppression ou de remplacement nouvelle ; tout
  `MOVEFILE_*` ; `NtCreateFile`, `SetFileInformationByHandle`, `unsafe`, sauf
  arbitrage de l'option B par le propriétaire et amendement préalable ;
- toute écriture, session, verrou ou sonde de verrou par `doctor` ;
- tout changement de format : manifeste v1, receipts, marqueurs, journal ;
- tout changement de sortie de `sync`, `snapshot` et `restore` hors des cas
  nommés : lecture stable, budgets, P2-4, N2, verrou signalé ;
- tout chemin personnel ou propre au pilote dans le code, les tests, les
  exemples ou les rapports ;
- toute modification des rulesets ; tout tag, toute release, toute
  publication ; tout secret dans un workflow ;
- tout accès agent à `D:\Ga`, `D:\Savewarden`, `E:\GMS`, à un coffre ou une
  zone de pilote, ou aux clés SSH ;
- tout test lisant ou écrivant hors de `t.TempDir()`, hormis les lettres
  `subst` que le test crée et retire.

---

## 8. Plan d'exécution

Avant tout code : une v0.2 de ce plan intégrant la revue indépendante, la
revue CTO et les arbitrages, acceptée sur `docs/b5-framing` ; un GO de
développement distinct. Les amendements d'ADR de B5a s'appuient sur les
résultats HW : ils sont rédigés après eux et acceptés avant tout code de
production. Chaque sous-lot part du commit de fusion du précédent.

| # | Sous-lot | Commit | Point de contrôle |
| --- | --- | --- | --- |
| 0 | — | Plan validé | **Revue du cadrage** par le propriétaire |
| 1 | B5a | Tests réels HW1 à HW7, sans code de production ; le garde ne couvre pas les tests | **Résultats HW revus** ; une hypothèse tombée arrête B5a |
| 2 | B5a | Amendements d'ADR 0005 et 0006 §11, rédigés avec les résultats HW | **Acceptation par le propriétaire** avant le commit 3 |
| 3 | B5a | Base de volume, contrôle à l'ouverture, `Describe` ; commentaires (T6) ; garde mis à jour ; contrôles sous session sur l'identité tenue (T4) | — |
| 4 | B5a | Test réel de réattribution pendant une session ; requalification ; différentiel contre `4197317` | — |
| 5 | B5a | Rapport `docs/validation/b5a.md` ; S-01 fermé dans le modèle de sécurité | **Revue de sécurité indépendante, obligatoire**, puis gate S-01 et GO de fusion |
| 6 | B5b | Noms de la matrice B4 (T10), tests seuls | — |
| 7 | B5b | Fenêtre de lecture du coffre reproduite sur faux ; correction ou preuve consignée (T7) | — |
| 8 | B5b | Budgets (T8) | — |
| 9 | B5b | P2-4 et amendement d'ADR 0008 ; N2 | — |
| 10 | B5b | Profondeur JSON, verrou de récupération signalé, `.gitignore` ; rapport `b5b.md` | **Revue indépendante de non-régression**, puis GO de fusion |
| 11 | B5c | ADR 0010 et notes acceptés | Revue du propriétaire |
| 12 | B5c | Types partagés ; relevés en lecture seule de `sync` ; feature `doctor`, structurel | — |
| 13 | B5c | `--verify` ; zones ; CLI et composition ; tests d'imports | — |
| 14 | B5c | Tests Windows réels et binaires ; documentation ; rapport `b5c.md` | **Revue indépendante** de la lecture seule et des constats, puis GO de fusion |
| 15 | B5d | GoReleaser et job « Release snapshot » ; smoke | — |
| 16 | B5d | Campagne pilote par le propriétaire (§11) ; rapport `b5d.md` ; CHANGELOG | GO de fusion, puis clôture de B5 |

Chaque commit passe la suite complète à son SHA, dans une extraction propre.
Push, PR, fusion, campagne et tout changement de ruleset attendent chacun un
GO distinct du propriétaire.

---

## 9. Matrice de tests

Noms proposés, fixés au commit de chaque partie. Faux système de fichiers et
faux coffre sur les trois OS, sauf mention « Windows réel ».

**B5a, S-01**

| Test | Preuve |
| --- | --- |
| `TestSessionWritesThroughTheVolumePath` (Windows réel) | HW1 à HW3 : base tirée de l'épingle, même identité, aucune lettre dans les chemins utilisés |
| `TestVolumePathSemantics` (Windows réel) | HW2 : erreur 183 sur cible existante, erreur 17 entre volumes, état inchangé |
| `TestLetterRemappedDuringTheSession` (Windows réel) | HW4 : lettre `subst` réattribuée vers un dossier témoin pendant la session ; `CreateDir`, `NewFile`, `Move` restent dans la cible ; témoin identique |
| `TestPinsHoldThroughTheVolumePath` (Windows réel) | HW5 |
| `TestVolumePathLongAndUnicodeNames` (Windows réel) | HW6 |
| `TestSessionRefusesAVolumePathOfAnotherIdentity` | Contrôle à l'ouverture : identité différente, refus sans écriture |
| `TestChecksUnderTheSessionUseTheHeldIdentity` | T4 sur faux : la lettre réattribuée après l'épingle ne change pas l'objet contrôlé |
| `TestRestoreRefusesALetterRemappedAfterTheSession` (Windows réel, binaire) | Pendant de `TestRestoreRefusesALetterRemappedBeforeTheSession` : aucune écriture hors de la zone tenue |
| `TestSyncRefusesALetterRemappedBeforeTheSession`, `TestSnapshotRefusesALetterRemappedBeforeTheSession` (Windows réel, binaire) | T4 pour `sync` et `snapshot` : lettre `subst` du coffre réattribuée vers un dossier témoin entre le plan et l'ouverture ; code 1, témoin identique |
| `TestOutputsUnchangedSinceB4` | Différentiel : sorties de `sync`, `snapshot`, `restore`, `inspect` identiques à un golden enregistré depuis `4197317` |
| Gardes | `readonly_test.go` : la requête ajoutée n'est permise que dans `platform/vault` ; listes attendues indépendantes |

**B5b, lecteurs**

| Test | Preuve |
| --- | --- |
| `TestVaultIsReadUntilStable` | Fenêtre de `readProbing` reproduite sur faux avant correction, fermée après ; vérifiée par mutation |
| `TestVaultThatKeepsChangingIsRefused` | Trois lectures différentes : code 1, rien conclu |
| `TestReadBudgetIsEnforced` | Entrées, enregistrements et octets au-delà du budget : code 1, rien lu au-delà |
| `TestNonConformingNamesAreNeverRead` | Noms hors grammaire signalés, jamais ouverts |
| `TestUnreadableSnapshotIsNotModified` | P2-4 pour `snapshot` : identité illisible ou fichier tenu, « non vérifiable », aucune dégradation, aucun second snapshot |
| `TestVolumeNotReadyIsNotAbsent` | N2 : la porte classe la preuve comme indisponible |
| `TestJSONDepthIsCapped` | Plafond indépendant du décodeur |
| `TestCurrentRecoveryReportsItsLock` | Verrou créé par la récupération de `current`, signalé |
| Six tests nommés selon la matrice B4 | T10 : mêmes assertions |

**B5c, `doctor`**

| Test | Preuve |
| --- | --- |
| `TestDoctorWritesNothing` (Windows réel, binaire) | Deux exécutions sous ACL de refus d'écriture sur tout l'arbre : sorties et empreintes identiques |
| `TestDoctorNeverProbesTheLock` | Aucune ouverture de `sync.lock` dans la trace des lectures ; un `sync` concurrent prend son verrou |
| `TestDoctorHealthyVault` | Coffre synthétique sain, structurel et `--verify` : code 0 |
| `TestDoctorFindings` | Une ligne par constat : receipt invalide, snapshot altéré, génération altérée, `.incomplete`, entrée inattendue, `snapshots\receipts` supprimé, enveloppe épuisée ; code et action attendus |
| `TestDoctorPendingOperation` | Opération en attente : code 3, action nommée, aucune sonde |
| `TestDoctorVerifyRereadsEverySnapshot` | `--verify` lit tout le contenu et seulement lui |
| `TestDoctorAreas` | Zones par `--area` : marqueur d'un autre jeu, `.incomplete`, restaurations remises présentes |
| `TestDoctorHandedOverRestoresAreInformational` | Si Q3 retenue : restauration remise modifiée, information, code 0 |
| `TestDoctorDoesNotFollowLinks` (Windows réel) | Jonction sous une zone ou un coffre : non suivie |
| `TestDoctorStableCodesAreDocumented` | Chaque code de constat figure dans `commands/doctor.md` |
| Imports | `features/doctor` n'importe ni `sync`, ni `inspect`, ni `platform`, ni `cli` |

**B5d** : smoke des archives ; campagne pilote (§11).

---

## 10. Documentation livrée avec le code

ADR amendés et ADR 0010 ; modèle de sécurité, S-01 fermé et limite des
lectures écrite ; `commands/doctor.md` avec chaque code de constat et son
action ; `vault.md` ; architecture et graphe ; README et exemples vérifiés
par test ; `gitops.md`, build de release en snapshot ; guide du pilote ;
CHANGELOG par sous-lot ; `AGENTS.md`, lot courant ; un rapport de validation
par sous-lot.

---

## 11. Rollout, rollback et pilote

- Le code ne rejoint `release/v0.1.0-a1` que par PR ; aucun tag ni release.
- **Rollback de B5a** : revert de la PR, retour aux chemins à lettre et à la
  fenêtre de S-01. Il est neutre pour les coffres et les zones, aucun format
  ne changeant ; S-01 redevient ouvert.
- **Rollback de B5b et B5c** : revert, sans effet sur les données ; `doctor`
  n'écrit rien.

**Campagne pilote B5d (T16)**, exécutée par le propriétaire après la fusion
de B5c, avec un binaire compilé depuis un worktree détaché à ce commit, S-01
étant fermé. L'agent n'accède à aucun coffre ni à aucune zone. Aucun test
créant une lettre `subst` ne tourne sur le poste pendant la campagne, ni
pendant le pilote B4 : les suites Windows réelles de B4 en créent.

1. `doctor` sur le coffre du pilote B3 et la zone du pilote B4, lus
   seulement : structurel puis `--verify`, code `0` attendu ; empreinte du
   coffre et de la zone identiques avant et après.
2. Coffre neuf dédié à B5 : `sync --apply`, code `0` ; si la source a changé
   depuis, second `sync --apply` et rotation ; sinon `up to date`.
3. `snapshot --apply`, code `0` ; `restore --apply` vers une zone neuve,
   code `0` ; `inspect` de la restauration, égale au receipt du snapshot.
4. `doctor --verify` sur le coffre neuf et la zone neuve : code `0`.
5. Plans de `sync`, `snapshot` et `restore` du binaire de `4197317` sur le
   coffre neuf : mêmes sorties, aucune anomalie.
6. Consigner codes, nombres et verdicts, sans hash individuel, identifiant
   d'opération, receipt réel, nom de profil ni chemin personnel.

---

## 12. Critères d'acceptation

1. Après l'ouverture d'une session, aucun chemin à lettre n'est utilisé :
   HW1 à HW7 prouvés ; test réel de réattribution pendant une session, aucune
   écriture hors de la cible tenue.
2. Aucune API d'écriture nouvelle ; amendements d'ADR 0005 et 0006 acceptés
   avant le code ; gardes à jour, listes attendues indépendantes.
3. Suites B2, B3 et B4 inchangées ; sorties identiques au binaire de
   `4197317` hors des cas nommés au §7.
4. S-01 fermé par une revue de sécurité indépendante et un gate du
   propriétaire, consignés.
5. Lecture du coffre stable ou preuve d'absence de fenêtre consignée ;
   budgets appliqués ; points de B3 corrigés ou arbitrés.
6. `doctor` n'écrit rien, ne verrouille rien, ne sonde aucun verrou : binaire
   sous ACL de refus, empreintes identiques.
7. Chaque constat de `doctor` a un code stable documenté et une action qui
   ne supprime rien ; codes de sortie selon ADR 0010.
8. `--verify` relit `current`, les générations conservées et les snapshots
   publiés et les compare à leur receipt.
9. Frontière de `doctor` contrôlée par les tests d'imports ; ADR 0010 accepté.
10. Build de release en snapshot verte en CI, sans publication ni secret.
11. Revues indépendantes consignées : sécurité de B5a, non-régression de
    B5b, lecture seule de B5c.
12. Checks requis verts sur le SHA exact de chaque PR et de chaque fusion,
    avec lien du run.
13. Documentation, CHANGELOG et ADR cohérents ; exemples README vérifiés par
    test ; limites écrites.
14. Aucun accès agent aux données protégées ; aucun chemin personnel, save ou
    receipt réel dans le diff.
15. Campagne B5d exécutée et qualifiée par le propriétaire, ou absence
    déclarée.

---

## 13. Décisions reportées hors de B5

| Sujet | Comportement en B5 |
| --- | --- |
| Cyberpunk 2077, Metal Gear Solid V | Lot B6 proposé : dossiers connus, profils, manifeste et ADR 0002 |
| Lectures indépendantes des lettres | Limite écrite (§6) |
| Tolérance de B2 (`preparedFor`) | Report proposé (Q5) ; signalée par `doctor` |
| Restauration en place | Lot futur, ADR de sécurité et protection de l'état actif |
| Portabilité d'un coffre copié | Lot dédié |
| Sortie JSON | Après stabilisation des commandes |
| Tag, release, canaux de paquets | Gate de publication, après S-01 fermé |
| Windows arm64, Linux, macOS | Non distribués (Q8) |
| Rétention et nettoyage | Jamais par Savewarden sans lot dédié |
| Extraction du moteur de `sync` | Critères de réexamen d'ADR 0004 inchangés ; `doctor` ne les déclenche pas, n'écrivant rien |

---

## 14. Risques

| Risque | Effet | Parade |
| --- | --- | --- |
| Hypothèse Windows fausse sur les chemins de volume | Primitives cassées ou garantie fausse | HW1 à HW7 prouvés avant le code ; arrêt et arbitrage, jamais de repli silencieux |
| Régression de `sync`, `snapshot` ou `restore` par S-01 | Perte d'une garantie qualifiée | Surface inchangée ; suites complètes ; différentiel contre `4197317` ; revue de sécurité |
| GUID affiché à l'utilisateur ou consigné | Sorties changées, données de machine exposées | Diagnostics relatifs ; différentiel octet pour octet |
| Faux code 3 de la lecture du coffre | Doute injustifié, intervention inutile | T7 : reproduction, puis lecture stable |
| Budget trop bas | Coffre légitime refusé | Valeurs mesurées en v0.2 ; refus en code 1 sans conclusion ; documenté |
| `doctor` pris pour un outil de réparation | Attente déçue, gestes manuels risqués | Actions nommées, jamais de suppression ; documentation |
| `doctor` qui perturbe un `sync` | Verrou refusé au `sync` | Aucune sonde de verrou (T12), testée |
| `--verify` long sur un gros coffre | Temps d'exécution | Défaut structurel ; `--verify` explicite ; budget |
| Frontière `doctor` qui fuit vers `sync` | Couplage entre features | Ports définis par `doctor` ; types dans `entities/backup` ; tests d'imports ; ADR 0010 |
| Job de release qui reçoit un secret | Publication involontaire | `contents: read`, `--skip=publish`, aucun secret, revue du workflow |
| RC jugée suffisante sans Cyberpunk ni MGS V | Généralisation prématurée | Portée de la RC écrite : Odyssey seul ; B6 avant toute promesse de généralité |
| Dépôt public pendant que S-01 est ouvert | Attente d'une protection non fournie | Q7 : aucune diffusion au sens défini ; modèle de sécurité explicite |
| Mainteneur unique | Revue limitée | Revues indépendantes, Human GO, aucune auto-approbation |

---

## 15. Rapport de restitution attendu

Pour chaque sous-lot : branche et PR, SHA final, `git diff --stat`, gates et
résultats exacts, runs CI et leurs checks, ADR créés ou amendés, résultats
HW1 à HW7, différentiel, revues indépendantes, résultat de la campagne ou son
absence, écarts au plan, risques résiduels, questions ouvertes,
recommandation **sans auto-approbation**.

## Historique

| Version | Date | Commit | Changements |
| --- | --- | --- | --- |
| 0.1 | 5 novembre 2026 | local, branche `docs/b5-framing` | Brouillon de cadrage sur le GO du propriétaire du 5 novembre 2026, en documentation seule : S-01 en tête de B5, points ouverts de B4 intégrés. Décisions T1 à T19 proposées ; option A de S-01 et hypothèses HW1 à HW7 ; `doctor` en feature avec ADR 0010 ; points ouverts de B3 ; découpage en quatre PR ; campagne et build de release en snapshot. Aucune revue intégrée, aucun ADR rédigé, aucun GO de développement |

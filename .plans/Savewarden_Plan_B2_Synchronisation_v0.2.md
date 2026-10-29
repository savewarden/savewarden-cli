# Savewarden — Plan B2 : synchronisation sécurisée, `sync` sans suppression

> **Statut : v0.2 — VALIDÉ PAR LE PROPRIÉTAIRE le 29 octobre 2026 — B2a et B2b livrés ; arbitrages B2c A1–A5 et M3, puis revue du cadrage, intégrés (§0.8) ; GO de développement B2c non donné**
> Version du plan : `0.2` — 29 octobre 2026, validée après revue intégrale ; remplace le brouillon v0.1 du 24 octobre
> Lot : **B2**, en trois PR successives B2a, B2b, B2c, cible `release/v0.1.0-a1` (D15)
> Cadre : [plan directeur v0.1](Savewarden_Plan_Bootstrap_Architecture_GitOps_v0.1.md) §4, §7 B2 et §8,
> [plan B1 v0.2](Savewarden_Plan_B1_Manifeste_Inspect_v0.2.md), [AGENTS.md](../AGENTS.md),
> [modèle de sécurité](../docs/safety-model.md), [ADR 0001](../docs/adr/0001-fsd-inspired-go.md).

B2 fait passer Savewarden de l'observation à la protection : pour la première
fois, le code de production **écrit**, dans son propre coffre. Ce document
reste le **seul plan directeur de B2**, décliné en trois PR successives, chacune
validée indépendamment. Il intègre les arbitrages rendus par le propriétaire
le 29 octobre 2026 après revue indépendante du brouillon v0.1, puis ses
arbitrages finaux V1 à V4 du même jour. Le propriétaire l'a validé après revue
intégrale et amendements, le 29 octobre 2026.

L'ADR 0006 qu'il prévoit devient la **référence de sécurité** de toutes les
opérations de publication et de récupération de Savewarden, en B2 comme dans
les lots suivants. Son contenu normatif est fixé au §5.

GO B2a donné par le propriétaire le 29 octobre 2026 : travail en lecture seule
selon le §8.1. Ce plan ne vaut ni GO B2b ou B2c, ni autorisation de tag ou de
release, ni accès aux sauvegardes réelles.

---

## 0. Arbitrages du propriétaire (29 octobre 2026)

### 0.1 Décisions D1–D17

| # | Arbitrage | Effet dans ce plan |
| --- | --- | --- |
| D1 | **Modifiée : trois PR.** B2a conception et plan, B2b première publication, B2c rotation et récupération complète | §8 ; GO distinct avant chaque PR |
| D2 | **Retenue.** `--vault` obligatoire ; le manifeste v1 reste indépendant de la destination | §4.1 |
| D3 | **Retenue.** Plan sans écriture par défaut ; `--apply` obligatoire pour toute mutation | §4.1 |
| D4 | **Modifiée.** Remplacement de `current` dans le périmètre de B2, **réservé à B2c**, après qualification de la première publication B2b | §4.1, §5.5, §8.3 |
| D5 | **Retenue avec garde-fou.** `--allow-removals` obligatoire pour des fichiers retirés ; une source facultative devenue absente ne fait jamais disparaître ses fichiers de la nouvelle génération sans politique explicite | §4.4 |
| D6 | **Retenue.** `--recover` explicite, rollback par défaut avant publication achevée, finalisation seulement si le nouvel état est vérifié | §5.4, §5.5 |
| D7 | **Retenue.** Aucune suppression automatique en B2, stagings échoués compris | §6.1 |
| D8 | **Retenue.** `golang.org/x/sys/windows`, version fixée, **exclusivement** dans `platform/vault` | §3.1, §6.2 |
| D9 | **Retenue avec correction.** Codes 0/1/2/3, 3 = intervention requise ; le code décrit le résultat de la commande, pas l'état publié | §4.8 |
| D10 | **Retenue.** Receipt JSON v1 ; aucun format JSON public de la CLI | §4.7 |
| D11 | **Modifiée.** **NTFS seul** au départ ; ReFS seulement après tests d'intégration réels ; FAT32 et exFAT refusés | §4.2 |
| D12 | **Retenue.** Même volume : information explicite, non bloquante ; jamais qualifiée de sauvegarde hors disque | §4.2 |
| D13 | **Retenue.** Partage en lecture comme B1, contrôles avant et après ; jeu fermé requis | §4.3 |
| D14 | **Retenue.** Fichiers EFS refusés | §4.3 |
| D15 | **Retenue.** `release/v0.1.0-a1`, tant qu'aucun gel ni publication de cette alpha n'est autorisé | §8 |
| D16 | **Retenue.** Pilotes exécutés par le propriétaire, coffre neuf, idéalement sur un autre disque physique | §11 |
| D17 | **Retenue et précisée.** L'ancien coffre du pilote est exclu comme destination B2, sans chemin personnel codé en dur dans le produit | §2.3, §4.2 |

### 0.2 Questions ouvertes de la v0.1

| Question | Réponse du propriétaire | Effet |
| --- | --- | --- |
| `current` modifié hors de Savewarden | **Blocage strict** : diagnostiquer l'écart, refuser toute publication, ne rien déplacer dans `retained\`, exiger une décision ultérieure | §4.5 |
| Noms | **`retained`** ; **`vault`** dans le code, « coffre » dans la documentation française | §4.2 |
| Retries des renommages | **5 secondes au plus**, tentatives bornées, backoff, erreurs réellement transitoires seulement, jamais de boucle infinie | §5.7 |
| Revue indépendante de `platform/vault` | **Obligatoire** : écritures Windows, accès concurrents, journaux, récupération | §8.2, §8.3 |

### 0.3 Corrections obligatoires

| # | Correction exigée | Où |
| --- | --- | --- |
| C1 (P0) | Journal vérifiable résistant aux interruptions, marqueurs incomplets et états ambigus, ordre des opérations, fail-closed | §5.3 |
| C2 (P0) | Reconnaître la fenêtre où `current` est absent entre deux renommages : récupération transactionnelle par protocole, pas échange atomique | §5.5 |
| C3 (P0) | Récupération d'une première publication interrompue dès B2b, y compris après renommage du staging et avant le receipt | §5.4, §8.2 |
| C4 (P0) | Distinguer le résultat de la commande et l'état réellement publié ; le code 1 ne garantit pas `current` inchangé | §4.8 |
| C5 (P1) | Aucune suppression implicite liée à une source facultative devenue absente | §4.4 |
| C6 (P1) | Limiter les garanties annoncées pour `MOVEFILE_WRITE_THROUGH` | §5.6 |
| C7 (P1) | Distinguer permissions de l'agent et exécution du pilote par le propriétaire | §2.3, §11 |
| C8 (P1) | Corriger les exemples du §4.8, l'état des branches, PR et de l'historique Git | §1, §4.8 |

### 0.4 Arbitrages finaux V1–V4

| # | Arbitrage | Où |
| --- | --- | --- |
| V1 | `--accept-missing-source <id>` répétable, **B2c seulement**, pour une source `required: false`, physiquement absente et présente dans le receipt précédent. Source requise absente : toujours bloquante. Retrait d'une source du manifeste : décision distincte, jamais couverte par ce flag. Choix conservateur : une source manquante peut être un incident de configuration ou un volume absent, pas une intention | §4.1, §4.4 |
| V2 | Réserve = **taille complète de la nouvelle génération** en staging + max(64 Mio, 5 % de cette taille) ; espace **disponible pour l'utilisateur appelant**, quotas compris ; contrôle au préflight et sous verrou juste avant le staging ; estimation, jamais garantie contre un disque plein | §4.2, §4.3, §4.6 |
| V3 | Plan en **premier commit de B2a** ; pas de PR documentaire séparée ; la v0.2 est la seule version dans l'arbre ; les brouillons restent dans l'historique Git local | §8 |
| V4 | Retries **uniquement** sur `ERROR_SHARING_VIOLATION` (32) et `ERROR_LOCK_VIOLATION` (33), backoff borné à 5 secondes au total ; `ERROR_ACCESS_DENIED` (5) reste un échec immédiat | §5.7 |

### 0.5 Points de contrôle demandés pour la revue finale

| Exigence du propriétaire | Où le plan y répond |
| --- | --- |
| Distinguer un enregistrement préparé d'une opération effectivement réalisée | §5.3, « Ce que prouve un enregistrement » |
| Ne jamais déduire un état sûr d'un journal incomplet | §5.3, ambiguïtés ; I4, I5 |
| Une récupération répétée ne peut ni écraser une génération ni aggraver une ambiguïté | §5.9 ; tests §9.2 et §9.3 ; critères 12 et 16 |
| Interruption juste après `staging → current`, avant le receipt : coffre identifiable et récupérable sans ambiguïté | §5.4, points d'arrêt A1 à A8 |
| `MOVEFILE_WRITE_THROUGH` sans garantie générale de durabilité | §5.6 |

### 0.6 Amendements de la revue finale (29 octobre 2026)

Revue intégrale du fichier `561ed7a`, SHA-256 `a0c6b401…6041e2c` : architecture
validée sur le fond, D1–D17 et V1–V4 maintenus, trois corrections bloquantes et
cinq précisions, intégrées ici sans changer le nom du plan.

| # | Constat de la revue | Correction intégrée | Où |
| --- | --- | --- | --- |
| P0-1 | L'initialisation d'un coffre neuf précède son premier journal, sans protocole de récupération | Protocole I1 à I9, créations exclusives, marqueur validé par renommage ; états partiels reconnus ou refusés sans mutation ; reprise seulement par `--recover --apply` ; invariant I11 | §5.2, §5.4, §4.2, §9.2 |
| P0-2 | Convergence illimitée promise malgré neuf temporaires non supprimables | Enveloppe E1 à E4 déclarée ; convergence garantie dans l'enveloppe seulement ; au-delà, code 3 sans mutation ; invariant I12 | §5.3, §5.9, §9.2 |
| P0-3 | Rollback de rotation à deux renommages résumé par « selon les déplacements observés » | Quatre déplacements M1 à M4, chacun avec son intention et ses préconditions d'identité ; tableau de récupération fermé | §5.5, §9.3 |
| P1-1 | Abandon avant publication trop général | Observation admise pour chaque étape `planned`, `staging-intent`, `staged`, `verified` ; staging inexpliqué = ambiguïté | §5.4 |
| P1-2 | « Rien écrit » inexact après le second contrôle d'espace | « Aucun staging ni publication ; des métadonnées d'initialisation peuvent exister » ; état `metadata only` | §4.2, §4.3, §4.6, §4.8 |
| P1-3 | Fermeture des handles et contrôle d'identité autour du renommage non explicites | Ordre de bascule en six points, pré- et post-contrôle, résultat d'une course, test Windows réel | §5.4, §9.2 |
| P1-4 | Provenance des champs d'un receipt reconstruit | Provenance de chaque champ ; `published_at` nul et `recovered_at` renseigné pour un receipt reconstruit | §4.7, §9.2 |
| P1-5 | Un temporaire ne prouve pas l'absence d'action après une coupure | Invariant de protocole en exécution normale seulement ; observation contraire = code 3 | §5.3, §5.6 |

### 0.7 Conditions techniques bloquantes pour valider les PR B2

1. **Rien n'est jamais écrasé, tronqué ni supprimé.** Créations en
   `CREATE_NEW` ; renommages par `MoveFileEx` sans aucun drapeau, qui refusent
   une cible existante et toute copie entre volumes.
2. **ADR 0006 est accepté avant tout code d'écriture** (fusionné avec B2a) ;
   tout écart découvert ensuite l'amende avant le code concerné.
3. **Le journal n'est jamais cru seul** : la récupération n'agit que si le
   dernier enregistrement valide et l'identité observée des dossiers
   concordent ; toute ambiguïté est fail-closed, code 3, sans aucun
   déplacement ni écriture.
4. **`os.Root` n'est pas une garantie de non-suivi**, côté source comme côté
   coffre ; ancêtres épinglés et contrôles sur handle comme en B1.
5. **Publication seulement après succès complet** : plan sans anomalie
   bloquante, copie, relecture depuis le disque, second inventaire identique.
6. **Chaque transition est récupérable dans l'enveloppe déclarée** et prouvée
   par énumération des points de crash : initialisation du coffre et première
   publication en B2b, rotation en B2c ; au-delà de l'enveloppe, code 3 sans
   mutation.
7. **La source n'est jamais écrite** : empreinte complète avant et après, avec
   écriture refusée par ACL sur la source pendant `--apply`.
8. **Le garde de lecture seule ne s'assouplit que pour `platform/vault`**, en
   B2b, avec ADR 0005 ; B2c n'ajoute aucune API permise.
9. **Revues indépendantes obligatoires** : `platform/vault` avant le GO de
   fusion de B2b, revue de sécurité avant celui de B2c.

### 0.8 Arbitrages B2c A1 à A5 et M3 (29 octobre 2026)

Après la qualification de B2b (fusion `8f9f5e1`, pilote réel validé), l'examen
du plan B2c confronté au code fusionné a montré un défaut : la table de
récupération d'une rotation attend `current = old` dès `planned`, alors que
l'identité de l'ancien `current` n'était enregistrée qu'à `retire-intent`.
Une rotation interrompue pendant sa préparation aurait été ambiguë, code 3.
Le propriétaire a validé les cinq arbitrages proposés et tranché M3. Ce sont
des décisions de cadrage : **elles ne valent pas GO de développement B2c**.

| # | Décision | Exigence | Où |
| --- | --- | --- | --- |
| A1 | Identité persistante de l'ancien `current` | `planned.fields.current_identity` obligatoire pour une rotation, interdit pour une première publication ; `retire-intent` et `restore-intent` la reprennent exactement, jamais remplacée par une identité nouvellement observée ; journaux B2b, `previous: null`, inchangés | ADR 0006 §3 |
| A2 | Vérification sous verrou | `current` reconnu comme génération précédente (R2 ci-dessous) avant `planned`, son identité fixée ; nouvelle relecture, même identité, immédiatement avant le retrait ; observation exacte relue entre M1 et M2 (R3) ; tout écart bloque avant déplacement ; pré- et post-contrôles et fermeture des portées inchangés | ADR 0006 §5 |
| A3 | Retraits et sources absentes | D5 et V1 sans extension : `--allow-removals` pour les fichiers retirés d'une source présente seulement ; `--accept-missing-source <id>`, répétable, pour une source facultative absente et connue du receipt précédent seulement ; source requise absente et source retirée du manifeste toujours bloquantes ; anciens fichiers dans `retained\` ; receipt `missing` et `accepted: true` distingués ; aucun flag n'accorde la permission de l'autre | §4.4 |
| A4 | Conservation | Aucune suppression ni rétention automatique en B2 ; accumulation des générations conservées et son coût en espace documentés ; contrôles de capacité, quotas et marge maintenus | §4.2 |
| A5 | Découpage | Les commits du §8.3 restent la référence ; un regroupement techniquement indispensable est admis s'il est expliqué et vérifiable | §8.3 |
| M3 | Opération non commencée | `001-abandoned` admis seulement sans génération publiée attendue et sans `current`, ou avec la génération précédente reconnue (R1, R2) ; sinon code 3, rien écrit ; cas distinct d'une rotation dont `planned` existe | ADR 0006 §4 |

**Revue du cadrage (29 octobre 2026).** Le propriétaire a validé ce cadrage
sous conditions. Ses corrections sont intégrées à ADR 0006 et prévalent sur
les §4.5 et §5.5 ; elles ne valent pas davantage GO de développement :

| # | Correction | Exigence | Où |
| --- | --- | --- | --- |
| R1 | M3 sans `current` (P0-1) | Une génération publiée est attendue dès qu'existe un receipt, valide ou non, ou un journal terminal `published` ; attendue et `current` absent : contradiction, code 3, rien écrit ; abandon seulement sans génération attendue et sans `current` ; `current` présent : abandon seulement s'il est reconnu (R2) | ADR 0006 §4 |
| R2 | Génération précédente reconnue (P0-2) | Chaîne entière : receipt de tête valide ; journal précédent terminal `published`, `receipt_sha256` égal au SHA-256 du receipt ; identité observée égale à celle que ce journal a publiée, `staging_identity` de `staged` reprise par `publish-intent` ou `promote-intent` ; contenu intégralement conforme. Un dossier `unidentified` n'est jamais `old`. Valable pour une publication B2b et une rotation B2c, journaux B2b inchangés | ADR 0006 §4, §5 point 1 |
| R3 | Fenêtre M1–M2 (P1) | Après le post-contrôle de M1 et avant `promote-intent`, observation exacte (—, `old`, `new`) et `retained\<prev-op>` relu intégralement : tout écart, code 3, rien enregistré ni déplacé ; après un échec de M2, nouvelle observation avant toute restauration ; toute restauration M3 précédée de la même relecture ; récupération écrite ligne par ligne, rien déduit | ADR 0006 §5, point 5, restaurations vérifiées, tableau |
| R4 | Fenêtres résiduelles (P1) | `sync.lock` n'exclut qu'une autre opération Savewarden ; une écriture externe entre une relecture et le déplacement qui la suit n'est pas détectée, et la documentation le dit ; anomalie observée après M1 : blocage, sans rollback conjectural | ADR 0006 §5 |
| R5 | ADR 0007 (P1) | Codes B2b manquants ajoutés ; définition historique de `current-exists` conservée | Ci-dessous |

Conséquence de R2, acceptée : une copie du coffre, ou d'un `current`, change
les identités ; la génération n'est plus reconnue, code 3, et un nouveau
coffre reste la voie documentée. Le mode plan applique la même
reconnaissance que `--apply` : il annonce `unexpected-entry`, code 3, pour un
`current` dont l'identité n'est pas celle que son journal a publiée, quel que
soit son contenu ; un receipt absent ou invalide garde `current-modified`,
code 2, comme en B2b. C'est le seul changement de comportement du mode plan
pour un coffre écrit par B2b.

**Impacts sur ADR 0004.** Aucune frontière nouvelle, aucun port nouveau :
la rotation et sa récupération n'emploient que les ports existants, `Vault`
pour écrire, `Files` et `Inventories` pour lire, `Volumes` et `Clock`. ADR 0005
est inchangé : B2c n'ajoute aucune API d'écriture. Une correction est due :
l'amendement B2b d'ADR 0004 ne mentionne pas `Scope.Identity`, la lecture
d'identité ajoutée en B2b à la portée de lecture, ni la description d'un
fichier tenu sans partage ; à consigner au commit de documentation de B2c.

**Impacts sur ADR 0007.** Le schéma du receipt v1 ne change pas : `previous`,
`status: "missing"` et `accepted` existent déjà. À amender, au commit CLI de
B2c :

- codes émis par B2b et absents de la table des codes stables, manque B2b à
  corriger (R5), avec le sens du code fusionné :

  | Code | Sortie | Sens |
  | --- | --- | --- |
  | `current-exists` | 2, rien publié | `current` existant et différent du plan, refusé avant tout staging par une version sans rotation |
  | `changed-during-sync` | 2, rien publié | Source modifiée, apparue ou disparue entre le plan et la copie, pendant la copie ou avant le second inventaire |
  | `verification-mismatch` | 2, rien publié, staging conservé | Relecture du staging différente des empreintes copiées |
  | `insufficient-space` | 2, bloquante au plan | Espace disponible pour l'utilisateur inférieur à la génération complète plus sa marge |

  et deux diagnostics de code 1, sans code stable : `vault-changed`, le
  coffre a changé depuis le plan ou le plan de récupération ; `another sync
  is running on this game`, verrou tenu par une autre opération ;
- `current-exists` garde sa **définition historique** : B2c ne l'émet plus
  dans son chemin nominal, où la génération précédente reconnue est
  remplacée ; un `current` au contenu modifié reste `current-modified`,
  code 2, un `current` d'une autre identité que celle publiée est
  `unexpected-entry`, code 3 ;
- `--accept-missing-source` est répétable, une source par occurrence ; tout
  usage hors du §4.4 est une invocation invalide, code 1, rien écrit ;
- rendu d'un retrait ou d'une absence acceptés par flag : l'anomalie reste
  listée, en information, avec le flag qui l'accepte ;
- ligne d'état d'un retrait ou d'une promotion échoués puis annulés :
  `rolled back <op>; current unchanged`, code 1 ; d'un rollback impossible,
  ou d'une anomalie observée après M1 : `intervention required`, code 3 ;
- un `current` reconnu exige désormais son identité publiée : le sens de
  `current-modified` est inchangé, l'écart d'identité relève de
  `unexpected-entry`.

---

## 1. Contexte et preuves

| Élément | État vérifié le 29 octobre 2026 |
| --- | --- |
| `release/v0.1.0-a1` | `979c9a8`, merge de la PR #3 (`docs/b1-pilot-odyssey`), fusionnée le 24 octobre 2026 ; branche de la PR supprimée du remote |
| CI post-merge | [Run 40044543099](https://github.com/savewarden/savewarden-cli/actions/runs/40044543099), `push` sur `release/v0.1.0-a1`, SHA `979c9a8`, terminé, `success` ; 7 checks `success` : `Verify (ubuntu-24.04)`, `Verify (windows-2025)`, `Verify (macos-15)`, `Race detector`, `Lint`, `Vulnerability scan`, `Pins` |
| `main` | `25d1cc2`, bootstrap B0 |
| Protections | Rulesets `main` (24944424) et `release` (24944427), `enforcement: active`, relus par l'API GitHub ; détail des règles selon `docs/validation/b0-post-push.md` |
| Pilote B1 | Exécuté par le propriétaire sous Windows 11, binaire `3fe7d92` : deux `inspect`, codes `0`/`0`, rapports identiques, 14 fichiers, `findings: none`, `PilotPassed = True` |
| Qualification B1 | Validée par le propriétaire le 24 octobre 2026 (`cc78d77`) ; elle ne valide ni synchronisation, ni snapshot, ni restauration |
| Branche du plan | `docs/b2-sync-plan`, locale, non publiée, rebasée sur `979c9a8` ; brouillon v0.1 en `e1932a2`, contenu identique octet pour octet à l'ancien commit `eeafa90` ; v0.2 en `d056f3c`, puis intégration de V1–V4. La v0.2 remplace la v0.1 dans `.plans/` ; Git l'enregistre comme suppression et ajout, le contenu ayant changé de plus de moitié |
| Dépendances | Go 1.27.2, Cobra 1.10.2, `go.yaml.in/yaml/v3` v3.0.5 |

Acquis B1 réutilisables :

- `platform/filesystem` : portée ancrée sans suivi de liens, dossiers épinglés
  sans `FILE_SHARE_DELETE`, contrôle des attributs sur handle, refus des
  placeholders, chemins `\\?\`, `NotFollowedError`.
- `features/inspect` : parcours trié, classement des liens, hachage en flux,
  détection identité/taille/date autour de chaque lecture, collisions de
  casse, annulation sans rapport partiel.
- `entities/game` : grammaire canonique des chemins Windows indépendante de
  l'hôte (`ParseWindowsAbsolute`, `ParseWindowsRelative`).
- Tests : faux système de fichiers tous OS, jonctions `mklink /J`, refus
  d'écriture par `icacls`, tests de liens obligatoires en CI, smoke binaire.
- Contrat de sortie : `ExitError`, `checkedWriter`, ADR 0003.

Constats vérifiés pour ce plan :

- Go 1.27.2 : `os.Rename` appelle `MoveFileEx(MOVEFILE_REPLACE_EXISTING)` et
  `Root.Rename` passe par `FILE_RENAME_REPLACE_IF_EXISTS` : les deux
  remplacent une cible existante. `syscall.Rename` appelle `MoveFile`.
- `syscall` Windows fournit `CreateFile`, `CreateDirectory`,
  `FlushFileBuffers`, `SetFileTime`, `MoveFile`, mais ni `MoveFileEx`, ni
  `GetDiskFreeSpaceEx`, ni `GetVolumeInformation` ; `golang.org/x/sys/windows`
  expose `MoveFileEx`, vérifié par le propriétaire (D8).
- [Documentation `MoveFileEx`](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-movefileexw),
  relue le 29 octobre 2026 : un dossier ne peut être déplacé que sur le même
  lecteur ; `MOVEFILE_REPLACE_EXISTING` signale une erreur si la cible est un
  dossier existant ; `MOVEFILE_COPY_ALLOWED` simule le déplacement d'un
  fichier entre volumes par copie puis suppression ; la garantie de
  `MOVEFILE_WRITE_THROUGH` vise un déplacement « performed as a copy and
  delete operation ». Aucune garantie d'atomicité n'y est énoncée.
  `MoveFile` équivaut à autoriser la copie entre volumes pour un fichier : il
  reste donc interdit, au profit de `MoveFileEx` sans drapeau.
- `main` arrête le contexte sur `os.Interrupt` seulement ; la fermeture de la
  console ou un arrêt brutal tuent le processus : cas traité comme un crash.
- `SECURITY.md` décrit encore l'état B0 ; actualisé avec B2a.
- Le plan directeur rapporte un coffre historique du pilote Odyssey, créé hors
  de Savewarden, avec un `current` et le snapshot vérifié
  `20261009T183208-cacd3cae`. Constat rapporté, non revérifié, **non exploré**
  par l'agent. Il n'est jamais une destination B2 (§2.3).

## 2. Objectif, non-objectifs, permissions

### 2.1 Objectif

```text
savewarden sync --manifest <chemin> --vault <dossier> [--apply] [--recover]
```

1. Sans `--apply`, calcule et affiche un **plan** sans rien écrire : fichiers
   ajoutés, modifiés, retirés et inchangés par rapport au `current` du coffre,
   anomalies, destination exacte, opérations en attente.
2. Avec `--apply`, copie l'état des sources déclarées vers un staging du
   coffre, vérifie la copie relue depuis le disque, contrôle que la source n'a
   pas changé, puis publie ce staging comme `current` du jeu.
3. Ne supprime, ne tronque, n'écrase et ne réécrit **rien** : en B2c, l'ancien
   `current` devient une génération conservée dans `retained\`.
4. Écrit un **receipt JSON v1** par publication et un **journal vérifiable**
   qui permet de terminer ou d'annuler une opération interrompue avec
   `--recover`.
5. Reste en lecture seule sur la source, l'installation et le manifeste.

Livraison progressive :

| PR | Comportement observable |
| --- | --- |
| B2a | `sync` en mode plan seulement ; aucune écriture possible ; ADR 0004, 0006, 0007 |
| B2b | `--apply` publie un **premier** `current` dans un coffre neuf ; `--recover` récupère une première publication interrompue ; un `current` existant n'est jamais remplacé |
| B2c | `--apply` remplace `current` par rotation vers `retained\`, avec rollback et récupération à chaque transition |

### 2.2 Non-objectifs B2, opposables en revue

- **Restauration** sous toutes ses formes, en place ou ailleurs : B4.
- **Relocation**, changement de `SavePath`, modification de `CPY.ini`,
  création de lien, jonction ou symlink.
- **Suppression de la source** ou de quoi que ce soit dans la source.
- **`/MIR` ou miroir supprimant** : un fichier absent de la source n'est
  jamais effacé ; en B2c il reste dans la génération conservée.
- **Snapshots** immuables nommés et leur cycle de vie : B3. Les générations
  `retained\` ne sont pas des snapshots approuvés.
- **Rétention ou nettoyage** : ni générations conservées, ni stagings échoués,
  ni fichiers temporaires du journal ne sont supprimés (D7).
- **Adoption, migration ou intervention** sur un coffre non créé par
  Savewarden, dont l'ancien coffre du pilote (D17).
- Fusion de contenus, synchronisation bidirectionnelle, sélection partielle,
  `exclude`, compression, chiffrement, copie distante ou réseau.
- Instantané atomique de la source par VSS : élévation et COM, hors périmètre.
- NTFS transactionnel (`MoveFileTransacted`, TxF) : Microsoft en déconseille
  l'usage ; rejeté.
- Coffres ReFS, FAT32, exFAT, réseau (D11).
- Sortie JSON de la CLI (D10).
- Support fonctionnel Linux/macOS : `sync` refuse explicitement ces hôtes.

### 2.3 Permissions : agent, produit, propriétaire

| Acteur | Peut | Ne peut pas |
| --- | --- | --- |
| **Agent** de développement | Lire et modifier le dépôt ; tests sur données synthétiques en `t.TempDir()` | Explorer, lire ou écrire `D:\Ga`, `D:\Savewarden`, `E:\GMS` ou des clés SSH ; lancer Savewarden sur des données réelles, sauf autorisation explicite et nominative du propriétaire pour cette exécution |
| **Savewarden** exécuté par le propriétaire | Lire le manifeste désigné, où qu'il soit, par exemple sous l'ancien coffre ; lire les sources qu'il déclare, par exemple `E:\GMS` pour Odyssey ; lire et écrire **seulement** dans le coffre passé à `--vault` | Écrire ailleurs ; adopter un dossier non vide sans marqueur ; suivre un lien ; supprimer |
| **Propriétaire** | Choisir manifeste et coffre, exécuter les pilotes, autoriser nominativement une exécution par l'agent | — |

- La lecture de `E:\GMS` par Savewarden pendant le pilote est **attendue** :
  c'est la source déclarée. Ce n'est pas une violation des règles de l'agent.
- Le produit ne contient **aucun chemin personnel codé en dur**, ni liste de
  chemins interdits propre au pilote. Sa sécurité générale repose sur des
  règles génériques : validation du coffre, refus d'un dossier non vide sans
  marqueur, isolation des chemins, refus d'adoption implicite, destination
  explicite affichée par le plan et confirmée par `--apply`.
- Exemples, fixtures et documentation utilisent des chemins fictifs neutres,
  comme `D:\Games\Synthetic Game` ou `F:\Vault`.

## 3. Décisions d'architecture et ADR

### 3.1 Packages

```text
cmd/savewarden ──► app ─┬─► cli ─┬─► features/inspect ──► entities/game, entities/backup
                        │        └─► features/sync ─────► entities/game, entities/backup
                        ├─► platform/manifests ──► entities/game, YAML
                        ├─► platform/filesystem ─► stdlib, syscall           (lecture seule)
                        └─► platform/vault ──────► stdlib, syscall, x/sys    (seul package qui écrit)
```

| Package | Rôle | Introduit |
| --- | --- | --- |
| `internal/entities/backup` | Types partagés : `Entry`, inventaire d'un dossier, `Finding`, `Severity`, `Code` ; différence pure ; receipt v1 ; enregistrement de journal et sa validation pure | B2a |
| `internal/features/inspect` | Inchangé fonctionnellement ; expose l'inventaire sans suivi de liens d'un dossier quelconque, utilisé aussi par son `Inspect` | B2a |
| `internal/features/sync` | Orchestration : préflight, plan, staging, copie, vérification, publication, rotation, rollback, récupération ; ports définis par lui-même | B2a, étendu en B2b et B2c |
| `internal/platform/vault` | Session de coffre en **création seule** : verrou, marqueur, dossiers et fichiers `CREATE_NEW`, flush, date, enregistrements de journal validés par renommage, `MoveFileEx` sans drapeau, identité de fichier, type de volume et espace libre | B2b |
| `internal/cli` | Commande `sync`, flags, rendu, codes de sortie, ligne d'état du coffre | B2a |
| `internal/app` | Composition ; ponts `inspect → Inventories`, `filesystem → Sources`, `vault → Vaults` et `Volumes` | B2a, B2b |

Pourquoi ce découpage :

- **Pas de duplication du parcours.** Le parcours sans suivi de liens est le
  code le plus sensible de B1. `sync` en a besoin pour inventorier la source
  avant et après copie, vérifier le staging et contrôler `current` contre son
  receipt. ADR 0001 interdit `sync → inspect` ; `sync` définit un port
  `Inventories` que `app` satisfait avec le service `inspect` : orchestration
  supérieure explicite, réduite à un pont de composition, actée par ADR 0004.
- **Types communs dans `entities/backup`.** Le plan B1 l'annonçait. Le
  déplacement est un refactor sans changement de comportement, prouvé par une
  sortie `inspect` identique octet par octet. Le modèle de journal y vit aussi :
  ADR 0006 le rend réutilisable par B3 et B4.
- **Un seul package qui écrit.** `platform/filesystem` reste en lecture seule ;
  `features/sync` n'appelle aucune API d'écriture et passe par ses ports.
- **Windows seulement pour `platform/vault`.** POSIX `rename(2)` remplace la
  cible ; un fichier `_other.go` renvoie `errors.ErrUnsupported`. La logique
  est testée sur faux coffre sur les trois OS.
- **`x/sys/windows` limité à `platform/vault`** par le test d'imports (D8).

### 3.2 Ports de `features/sync` (forme indicative, à valider par tests)

```go
// Inventories walks one directory without following anything, as inspect does.
type Inventories interface {
    Directory(ctx context.Context, volume string, segments []string) (backup.Directory, error)
}

// Sources opens files for reading: read-only, no-follow. Also used to read
// the vault's marker, receipts and journal in plan mode.
type Sources interface {
    OpenScope(volume string) (SourceScope, error)
}
type SourceScope interface {
    Lstat(name string) (fs.FileInfo, error)
    ReadDir(name string) ([]fs.DirEntry, error)
    OpenRegular(name string) (fs.File, error)
    SameFile(a, b fs.FileInfo) bool
    Close() error
}

// Volumes answers read-only questions about a destination volume.
type Volumes interface {
    Describe(root string) (backup.Volume, error) // file system name, drive type, free bytes
}

// Vaults opens a create-only session for one game, for --apply and --recover
// only: it runs the vault initialization protocol of ADR 0006 when needed
// (steps I2 to I6), then locks.
type Vaults interface {
    Open(root, gameID string) (Vault, error)
}
type Vault interface {
    CreateDir(rel string) error                                   // fails if it exists
    CreateFile(rel string, modTime time.Time) (io.WriteCloser, error) // CREATE_NEW; Close flushes, sets mtime
    Commit(rel string, body []byte) error                         // temp CREATE_NEW, write, flush, rename without replace
    Move(fromRel, toRel string) error                             // MoveFileEx, no flag: never replaces, never copies
    Identity(rel string) (backup.FileID, error)                   // volume serial + file index, no-follow
    Close() error                                                 // releases pins and lock
}

type Clock interface {
    Now() time.Time
    Sleep(ctx context.Context, d time.Duration) error // bounded retries only
}
```

Les types des ports viennent de la bibliothèque standard ou de
`entities/backup` : les adaptateurs n'importent pas la feature. Les noms
relatifs suivent la forme `io/fs`. Le mode plan ne reçoit que `Inventories`,
`Sources` et `Volumes` ; `app` ne lui fournit pas `Vaults`. L'aléa de
l'identifiant d'opération vient d'un `io.Reader` injecté, `crypto/rand` en
production. `Volumes` arrive avec `platform/vault` en B2b ; en B2a, le plan
signale « volume not checked by this build ».

### 3.3 ADR à produire

| ADR | Objet | Livré avec |
| --- | --- | --- |
| **0004 — Frontières de `sync`** | `features/sync`, `entities/backup`, port `Inventories` satisfait par `inspect` via `app`, `platform/vault` seul adaptateur d'écriture, `x/sys/windows` limité à ce package ; tests d'imports mis à jour | B2a |
| **0005 — Écritures autorisées** | Assouplissement du garde pour `internal/platform/vault` seulement ; API permises nommées ; interdits renforcés ; règle « `MoveFileEx` sans drapeau » | B2b, premier commit |
| **0006 — Protocole de publication et de récupération** | **Référence de sécurité** de toute publication et de toute récupération : invariants, journal vérifiable, initialisation du coffre, première publication, ordre de bascule, rotation, fenêtre d'absence, durabilité, retries, enveloppe de récupération, fail-closed (§5) | **B2a**, avant tout code d'écriture ; amendé avant le code si B2b ou B2c révèlent un écart |
| **0007 — Contrat de `sync` et receipt v1** | Modes et flags par PR, codes 0/1/2/3, ligne d'état du coffre, format du plan, schéma du receipt, politique des retraits et sources absentes | B2a |

## 4. Contrats

### 4.1 Commande, modes et disponibilité par PR

```text
savewarden sync --manifest <m> --vault <v>                     plan seul, rien n'est écrit
savewarden sync --manifest <m> --vault <v> --apply             copie, vérifie, publie
savewarden sync --manifest <m> --vault <v> --recover           plan de récupération, rien n'est écrit
savewarden sync --manifest <m> --vault <v> --recover --apply   exécute la récupération
```

| Mode ou flag | B2a | B2b | B2c |
| --- | --- | --- | --- |
| Plan, par défaut | Oui | Oui | Oui |
| `--apply` | Absent : flag inconnu, code 1 | Première publication ; `current` existant et différent du plan : `current-exists`, code 2 | Première publication et rotation |
| `--recover`, `--recover --apply` | Absents | Première publication interrompue | Toute opération interrompue |
| `--allow-removals` | Absent | Absent | Fichiers retirés d'une source **présente** |
| `--accept-missing-source <id>` | Absent | Absent | Source `required: false`, physiquement absente et présente dans le receipt précédent, nommée une par une (§4.4) |

- `--manifest` et `--vault` obligatoires, aucun défaut, aucune variable
  d'environnement, aucun fichier de configuration (D2).
- `--vault` suit la grammaire `ParseWindowsAbsolute` de `entities/game` ;
  UNC, `\\?\`, `\\.\`, relatif et `%VAR%` refusés ; aucun nettoyage silencieux.
- Le mode plan est **garanti sans écriture** : il ne reçoit pas `Vaults`, et
  un test binaire le lance avec écriture refusée par ACL sur tout l'arbre.
- `--recover` exclut toute nouvelle copie et refuse `--allow-removals` et
  `--accept-missing-source`.
- Toutes les sources du manifeste sont copiées ensemble, en une génération.

### 4.2 Coffre

```text
<vault>\
  savewarden-vault.json               marqueur : schéma, version de l'outil, date de création
  games\<game-id>\
    sync.lock                         verrou : handle exclusif pendant l'opération, jamais supprimé
    current\<source-id>\...           génération publiée et vérifiée
    retained\<op-id>\<source-id>\...  générations remplacées (B2c), jamais modifiées ni supprimées
    staging\<op-id>\<source-id>\...   opération en cours, échouée ou abandonnée, identifiée comme telle
    receipts\<op-id>.json             receipt de chaque génération publiée
    journal\<op-id>\NNN-<étape>.rec   enregistrements validés, jamais réécrits (§5.3)
```

- Nommage : `vault` dans le code et les sorties, « coffre » dans la
  documentation française ; `retained` pour les générations conservées.
- `<op-id>` : `AAAAMMJJTHHMMSS-xxxxxxxx`, heure **UTC** et 8 chiffres
  hexadécimaux aléatoires. `retained\<op-id>` porte l'identifiant de
  l'opération qui l'a produite et correspond à `receipts\<op-id>.json`.
- `snapshots\` est réservé à B3 ; B2 n'y crée rien.
- **États acceptés** au préflight, initialisés selon le protocole I1 à I9 du
  §5.4 :
  - absent, parent existant : créé à la première `--apply`, avec son marqueur ;
  - dossier vide : marqueur créé à la première `--apply` ;
  - coffre Savewarden, marqueur valide de schéma connu, aucune entrée
    imprévue : utilisé ;
  - dossier ne contenant **que** des temporaires de marqueur : initialisation
    interrompue, reconnue ; `--apply` refuse en code 3 ; seul `--recover
    --apply` la reprend.
- **Refusés, code 1 :** fichier ; dossier non vide sans marqueur, temporaires
  de marqueur mêlés à toute autre entrée compris ; marqueur invalide ou de
  schéma inconnu ; lien, jonction, point d'analyse ou placeholder dans le
  chemin du coffre jusqu'à `games\<game-id>` compris. Cette règle générique
  suffit à exclure l'ancien coffre du pilote, non vide et sans marqueur, sans
  que le produit nomme aucun chemin.
- **Refusé, code 3 :** coffre marqué contenant une entrée imprévue à sa racine,
  dans `games` ou dans `games\<game-id>`.
- **Distinction stricte :** le coffre ne coïncide pas avec l'installation ou
  une source, ne les contient pas et n'y est pas contenu. Contrôle par segments
  insensibles à la casse, puis par identité (`SameFile`) de chaque dossier
  existant de la chaîne, ce qui couvre les noms courts 8.3.
- **Volume du coffre (D11), à partir de B2b :**
  - système de fichiers **NTFS** seulement ; ReFS refusé comme « non
    qualifié », y compris un Dev Drive, jusqu'à des tests d'intégration réels
    et un amendement d'ADR 0006 ; FAT32, exFAT et tout autre refusés ;
  - lecteur local fixe ou amovible seulement ; lecteur réseau, optique, RAM
    disk ou de type inconnu refusés ;
  - un lecteur `subst` n'est pas détecté comme redirection : limite héritée
    de B1, documentée.
- **Même volume que la source (D12)** : information `same-volume`, non
  bloquante, avec le texte « not an off-disk backup ». Deux volumes d'un même
  disque physique ne sont pas détectés.
- **Espace libre, à partir de B2b (V2)** :
  - besoin = **taille complète de la nouvelle génération**, c'est-à-dire tous
    les fichiers copiés en staging et pas seulement les modifiés, plus
    max(64 Mio, 5 % de cette taille) ; la marge couvre aussi journal et receipt ;
  - espace mesuré par `GetDiskFreeSpaceEx` en **octets disponibles pour
    l'utilisateur appelant**, qui tiennent compte des quotas ;
  - calcul en entiers non signés 64 bits avec détection de dépassement : un
    total qui déborderait est refusé ;
  - contrôlé au préflight, où un échec n'écrit rien, puis **sous verrou juste
    avant la création du journal et du staging** (étape I7 du §5.4) ; un échec
    à ce stade donne `insufficient-space`, bloquant : aucun staging, aucune
    copie, aucune publication, mais les métadonnées d'initialisation et le
    verrou peuvent déjà exister ;
  - cette estimation n'est **pas une garantie** : un disque plein ou un quota
    atteint pendant la copie reste traité comme une erreur d'écriture (§4.6).

### 4.3 Pipeline d'une opération

| Phase | Écrit ? | Contenu | En cas d'échec |
| --- | --- | --- | --- |
| 1. Préflight | Non | Manifeste strict ; résolution sans suivi des sources, de l'installation et du coffre ; distinction ; état du coffre et de son journal ; volume et espace dès B2b ; longueur des chemins finaux | Code 1 ou 2, rien écrit |
| 2. Inventaire initial | Non | Inventaire complet de chaque source via `Inventories` ; aucune anomalie bloquante tolérée ; noms d'entrées valides sous Win32 ; EFS refusé | Code 2, rien écrit |
| 3. Plan | Non | Contrôle strict de `current` contre son receipt (§4.5) ; différence pure : ajoutés, modifiés, retirés, inchangés ; règles des sources absentes (§4.4) | Mode plan : fin, code 0 ou 2 |
| 4. Staging | Oui | Initialisation I2 à I6 du §5.4 si nécessaire ; verrou ; état du coffre relu sous verrou et identique au plan, sinon `vault-changed`, code 1 ; espace libre revérifié (V2), sinon `insufficient-space`, code 2, sans staging ni publication mais avec d'éventuelles métadonnées d'initialisation ; dossier d'opération, puis enregistrements `planned` puis `staging-intent` ; `staging\<op-id>` créé ; chaque fichier copié d'un handle sans suivi vers un fichier `CREATE_NEW`, SHA-256 des octets lus, flush, date reportée ; dossiers vides recréés ; `staged` | `failed` si possible ; staging conservé ; `current` intact ; code 1 sur erreur d'écriture, 2 sur changement de la source |
| 5. Contrôle source | Non | Second inventaire complet, identique au plan : chemins, tailles, dates, SHA-256 | `changed-during-sync`, `failed`, code 2 |
| 6. Vérification | Non | Staging relu depuis le disque par une portée de lecture sans suivi : ensemble exact des chemins et dossiers, tailles, SHA-256 ; aucune entrée en trop ; portée fermée ; `verified` | `verification-mismatch`, `failed`, code 2 |
| 7. Publication | Oui | Première publication §5.4 (B2b) ou rotation §5.5 (B2c) ; receipt ; `published` | §5 |

Détection d'un changement de la source, cumulative :

- à l'ouverture de chaque fichier : identité, taille et date égales au plan ;
- après lecture : taille et date inchangées, octets lus égaux à la taille ;
- SHA-256 calculé pendant la copie égal à celui du plan ;
- second inventaire complet égal au premier, ce qui couvre les fichiers
  ajoutés, retirés ou modifiés ailleurs pendant la fenêtre.

Limite assumée : pas d'instantané atomique. Chaque fichier publié est
exactement celui qui a été haché et aucun changement n'a été observé pendant
la fenêtre ; un changement puis un retour à l'identique entre deux lectures,
ou une incohérence entre fichiers écrits par le jeu, reste possible. Jeu et
launcher doivent être fermés. Les fichiers source sont ouverts avec le même
partage que B1, lecture et écriture, pour ne jamais faire échouer une
sauvegarde du jeu (D13).

Ce qui est copié : contenu et date de modification des fichiers réguliers,
arborescence des dossiers. Ce qui ne l'est pas : attributs, ACL, flux
alternatifs, dates de création et de dossier, compression. Un fichier source
chiffré par EFS est refusé, `encrypted-source`, bloquant (D14).

Identique : si le plan ne contient aucun changement et que `current` est
conforme à son receipt, `--apply` n'écrit rien, rapporte « up to date » et
sort en code 0, en B2b comme en B2c.

### 4.4 Sources absentes et retraits

Une absence n'est jamais interprétée comme une suppression volontaire : une
source manquante peut être un incident de configuration ou un volume
temporairement absent (V1).

| Situation, par rapport au receipt de `current` | Règle | Disponible |
| --- | --- | --- |
| Source `required: true` absente | **Bloquant, toujours** ; aucun flag ne le lève | B2a |
| Source `required: false` absente, absente aussi du receipt précédent ou sans receipt | Information ; receipt `status: "missing"`, sans entrées | B2a |
| Source `required: false` physiquement absente, **présente dans le receipt précédent** | **Bloquant** `source-vanished` ; publication seulement si `--accept-missing-source <id>` la nomme ; ses fichiers restent dans `retained\` ; nouveau receipt `status: "missing"`, `accepted: true` | B2c |
| Source présente dans le receipt précédent mais **retirée du manifeste** | **Bloquant** `source-removed-from-manifest` ; **aucun flag en B2** : décision distincte du propriétaire, hors B2 (§13) | B2a |
| Fichiers retirés d'une source présente | **Bloquant** `files-removed` sans `--allow-removals` ; fichiers conservés dans `retained\` | B2c |
| Source présente mais vide, `current` en contenait des fichiers | Cas précédent : `--allow-removals` exigé | B2c |

- `--allow-removals` ne couvre **jamais** une source absente ou retirée du
  manifeste.
- `--accept-missing-source <id>` ne couvre que la troisième ligne. Nommer une
  source requise, présente, inconnue du manifeste, absente du receipt
  précédent ou retirée du manifeste est une invocation invalide : code 1,
  rien écrit.
- En B2b, ces cas ne peuvent pas aboutir : un `current` existant n'est jamais
  remplacé. Le mode plan les signale toutefois dès B2a.

### 4.5 `current` contrôlé contre son receipt

Avant tout plan dont le coffre contient un `current`, celui-ci est
réinventorié sans suivi de liens et comparé **exactement** à son receipt :
ensemble des chemins et dossiers, tailles, dates, SHA-256, aucune entrée en
trop, aucun lien.

- Écart : **blocage strict** `current-modified`, code 2, aucune publication,
  rien n'est déplacé vers `retained\`, aucun flag de contournement en B2.
- Le rapport liste les écarts ; le guide du coffre explique comment les
  examiner. La politique de résolution relève d'une décision ultérieure du
  propriétaire, hors B2 ; d'ici là, un nouveau coffre reste possible.
- `current` sans receipt valide, ou receipt sans `current`, en l'absence
  d'opération en attente : même blocage.
- En B2c, le contrôle porte aussi sur l'identité : `current` doit être la
  génération précédente reconnue (ADR 0006 §4) ; un `current` d'une autre
  identité que celle publiée par son journal est `unexpected-entry`, code 3,
  quel que soit son contenu (§0.8, R2).
- Les générations `retained\` ne sont pas recontrôlées à chaque `sync` ; ce
  sera le rôle de `doctor` en B5. Seule exception : `retained\<prev-op>`,
  relu par la rotation qui le crée, entre M1 et M2 et avant toute
  restauration (§0.8, R3).

### 4.6 Verrous, fichiers ouverts, chemins longs, volumes

| Situation | Comportement |
| --- | --- |
| Fichier source verrouillé par le jeu | `unreadable`, bloquant ; rien publié |
| Deux `sync --apply` simultanés sur un jeu | `sync.lock` ouvert sans partage : le second échoue, code 1, « another sync is running » ; libéré par l'OS si le processus meurt |
| `current` ou staging tenu par l'Explorateur, l'antivirus ou l'indexeur | Retries bornés (§5.7) sur erreurs transitoires seulement ; sinon échec sans bascule, ou rollback, avec la cause |
| Chemin final au-delà de 260 caractères | Écriture par `\\?\` ; information `long-path` : l'Explorateur et certains jeux peuvent échouer lors d'une copie manuelle |
| Nom d'entrée que Win32 altérerait : point ou espace final, nom réservé | `unportable-name`, bloquant, grammaire de `entities/game` |
| Collision de casse dans la source | Bloquant, comme en B1 |
| Disque plein ou quota atteint pendant la copie, malgré le préflight | Erreur d'écriture : `failed`, staging incomplet conservé, code 1, `vault: current unchanged; operation <op> kept in staging` |
| Espace libre qui diminue entre préflight et `--apply` | Second contrôle sous verrou (V2) ; insuffisant : `insufficient-space`, code 2, aucun staging ni publication, `vault: metadata only; nothing staged or published` si l'initialisation a écrit |
| Coffre ou parent devenu point d'analyse, placeholder OneDrive | Refus au préflight ; ancêtres épinglés pendant l'opération |
| Accès contrôlé aux dossiers de Windows | Échec propre, code 1 ; ne jamais désactiver la protection |
| Volume non NTFS ou non local | Refus au préflight, code 1 |

### 4.7 Vérification d'intégrité et receipt v1

Vérification avant publication, phase 6 : relecture de chaque fichier du
staging par la portée de lecture B1, ensemble exact des chemins et dossiers,
tailles, SHA-256. La relecture peut provenir du cache du système : elle
prouve ce que le système de fichiers restitue, pas la durabilité du support.
Après le renommage de publication, l'identité du dossier publié est
contrôlée. Le contrôle de `current` contre son receipt (§4.5) a lieu à chaque
`sync` ; une récupération qui finalise une publication re-hache tout `current`.

Receipt `receipts\<op-id>.json`, forme indicative à figer dans ADR 0007 :

```json
{
  "schema": "savewarden.receipt",
  "schema_version": 1,
  "operation": "20261105T101500-1a2b3c4d",
  "tool_version": "dev",
  "recovered_by": null,
  "game": {"id": "synthetic-game", "name": "Synthetic Game"},
  "started_at": "2026-11-05T10:15:00Z",
  "publish_intent_at": "2026-11-05T10:15:03Z",
  "published_at": "2026-11-05T10:15:04Z",
  "recovered_at": null,
  "previous": null,
  "plan_sha256": "…",
  "changes": {"added": 1, "modified": 0, "removed": 0, "unchanged": 0},
  "sources": [
    {
      "id": "saves", "kind": "save", "required": true, "status": "copied",
      "path": "D:\\Games\\Synthetic Game\\saves",
      "files": 1, "bytes": 3,
      "directories": ["Slot 1"],
      "entries": [
        {"path": "Slot 1/1.save", "size": 3, "mod_time": "2026-10-09T16:32:08.1234567Z", "sha256": "…"}
      ]
    }
  ]
}
```

- `encoding/json` de la bibliothèque standard ; entrées triées octet par
  octet ; dates UTC RFC 3339 ; champs inconnus refusés à la relecture.
- Écrit comme un enregistrement de journal : fichier temporaire, flush,
  renommage sans remplacement (§5.3). Un receipt n'est jamais réécrit.
- `previous` : identifiant de la génération remplacée en B2c, `null` sinon.
- Distinct du rapport d'`inspect` ; contient des chemins locaux et le nom du
  jeu : à anonymiser avant tout partage.

**Provenance des champs (P1-4).** Chaque champ a une source déterministe ;
un receipt reconstruit par `--recover` ne prétend jamais connaître ce qu'il
ignore :

| Champ | Source en exécution normale | Source dans un receipt reconstruit |
| --- | --- | --- |
| `operation`, `game`, `previous`, `plan_sha256`, `changes`, `sources`, `directories`, `entries` | `001-planned`, contrôlés contre `current` | Les mêmes, après revérification intégrale de `current` contre le plan |
| `tool_version` | Version qui a validé `001-planned` | La même, lue dans `001-planned` |
| `recovered_by` | `null` | Version de l'outil qui reconstruit |
| `started_at` | `at` de `001-planned` | Le même |
| `publish_intent_at` | `at` de `publish-intent` ou `promote-intent` : la bascule a eu lieu **après** cet instant | Le même |
| `published_at` | Horloge au post-contrôle d'identité qui suit la bascule | **`null`** : l'instant réel du renommage est inconnu |
| `recovered_at` | `null` | Horloge de la récupération qui reconstruit |

Un receipt reconstruit est vérifiable comme tout autre : le `sync` suivant
compare `current` à ses entrées (§4.5). Un consommateur qui a besoin d'une
date de publication lit `published_at`, ou à défaut la borne inférieure
`publish_intent_at`, jamais `recovered_at`.

### 4.8 Sortie, codes de retour et état du coffre

Texte anglais UTF-8, sans couleur, LF, noms échappés comme `inspect`. Exemple
en mode plan, chemins fictifs, coffre sur le volume de la source :

```text
manifest: D:\Manifests\synthetic-game.yaml
game: synthetic-game (Synthetic Game)
vault: D:\Vault (will be created)
destination: D:\Vault\games\synthetic-game\current
mode: plan only, nothing written

source saves (save, required): D:\Games\Synthetic Game\saves
  status: planned
  files: 2  bytes: 6  added: 2  modified: 0  removed: 0  unchanged: 0
  + 7692c3ad3540bb803c020b3aee66cd8887123234ea0c6e7143c0add73ff431ed  3  2026-10-09T16:32:08.1234567Z  1.save
  + 3fc4ccfe745870e2c0d99f71f30ff0656c8dedd41cc1d7d3d376b0dbe685e2f3  3  2026-10-09T16:32:08.1234567Z  Slot 1/2.save

findings: 1 info
  info  same-volume  saves  D:\  vault and source share a volume; not an off-disk backup
```

Avec `--apply`, stdout ajoute `operation`, `verification`, `published` et
`receipt`, puis stderr porte la **ligne d'état du coffre** :

```text
savewarden: vault: published 20261105T101500-1a2b3c4d
```

Deux axes distincts, fixés par ADR 0007 :

- **Le code de sortie décrit le résultat de la commande.**
- **L'état publié est celui du coffre**, établi par le journal et le receipt.
  Chaque exécution `--apply` ou `--recover` l'annonce par une dernière ligne
  `savewarden: vault: <état>` sur stderr ; le mode plan l'affiche aussi.

| Code | Résultat de la commande | stdout | stderr |
| --- | --- | --- | --- |
| `0` | Succès complet : plan sans anomalie bloquante ; ou publication vérifiée, receipt et `published` validés, rapport écrit ; ou déjà à jour | Rapport | Ligne d'état si `--apply` |
| `1` | Échec : invocation, manifeste ou coffre invalide, verrou pris, erreur d'écriture, interruption, rapport impossible à écrire en entier. **Ne dit rien à lui seul de `current`** : la ligne d'état le dit | Vide, rapport, ou préfixe tronqué | Diagnostic, puis ligne d'état si `--apply` ou `--recover` |
| `2` | Rapport complet avec anomalie bloquante ; **cette exécution n'a rien publié** | Rapport | Synthèse, puis ligne d'état si `--apply` |
| `3` | Intervention requise : opération en attente, initialisation interrompue, journal ambigu, observation contradictoire, rollback impossible, enveloppe de récupération épuisée | Rapport si possible | Diagnostic avec emplacements exacts, puis ligne d'état |

| État annoncé du coffre | Combinaisons possibles |
| --- | --- |
| `not modified` | 0 en plan ou à jour ; 1 avant toute écriture ; 2 au préflight ou au plan |
| `metadata only; nothing staged or published` | 1 ou 2 quand seule l'initialisation I2 à I6 a écrit, par exemple `insufficient-space` sous verrou |
| `current unchanged; operation <op> kept in staging` | 1 après erreur d'écriture ou interruption ; 2 sur changement de la source ou vérification échouée |
| `published <op>` | 0 ; **1 si la publication a réussi mais que stdout n'a pas pu être écrit** |
| `rolled back <op>; current unchanged` | 1 après bascule échouée et rollback réussi, B2c |
| `intervention required for <op>: <cause>` | 3 seulement |

- Une interruption demandée pendant la section critique est différée ; si la
  publication aboutit, le résultat est rapporté normalement, avec la mention
  « interrupt deferred until publication completed ».
- Si stderr lui-même ne peut pas être écrit, le journal et le receipt font
  foi ; le mode plan affiche l'état publié et toute opération en attente.
- Un consommateur se fie au code pour le résultat de la commande, et à la
  ligne d'état, au receipt ou au mode plan pour l'état du coffre.

## 5. ADR 0006 — protocole de publication et de récupération (contenu normatif)

### 5.1 Portée

ADR 0006 s'applique à **toute opération qui rend visible un ensemble de
fichiers sous un nom stable** dans un coffre Savewarden et à **toute
récupération** : `current` en B2, snapshots en B3, restauration vers un dossier
isolé en B4 pour son staging et sa publication. Une future opération qui ne
peut pas s'y conformer exige un amendement de l'ADR, revu, avant son code.

### 5.2 Invariants

| # | Invariant |
| --- | --- |
| I1 | Rien n'est écrasé, tronqué ni supprimé : créations exclusives, renommages sans remplacement et sans copie |
| I2 | Publier, c'est renommer un dossier entièrement vérifié, sur le même volume ; jamais copier vers le nom publié |
| I3 | Write-ahead : chaque création de staging et chaque renommage d'un dossier d'opération, `staging\<op>`, `current` ou `retained\`, rollback compris, est précédé d'un enregistrement d'intention validé qui décrit l'état d'avant et d'après par identité de fichier |
| I4 | Le journal n'est jamais cru seul : une récupération n'agit que si le dernier enregistrement valide et l'observation concordent |
| I5 | Fail-closed : toute ambiguïté arrête tout, sans déplacement ni écriture, code 3, diagnostic |
| I6 | Rollback par défaut avant publication achevée ; finalisation seulement après revérification intégrale du nouvel état contre le plan |
| I7 | Une génération publiée ou conservée, un receipt, un enregistrement ne sont jamais rouverts en écriture |
| I8 | Une seule opération par jeu à la fois ; une opération non terminale bloque toute nouvelle `--apply` |
| I9 | La publication est **transactionnelle par protocole, pas atomique** : la fenêtre où `current` est absent est reconnue et récupérable |
| I10 | Seules les garanties documentées de Windows sont revendiquées ; le reste est une hypothèse couverte par une récupération fondée sur l'observation |
| I11 | L'initialisation du coffre, qui précède tout journal, n'est faite que de créations exclusives et d'un marqueur validé par renommage ; chaque état partiel est reconnu par le tableau du §5.4 ou refusé sans mutation, jamais adopté implicitement |
| I12 | La récupération est garantie **dans une enveloppe bornée et déclarée** (§5.9) ; au-delà, fail-closed |

### 5.3 Journal vérifiable

Emplacement : `games\<game-id>\journal\<op-id>\`, un dossier par opération.

**Format d'un enregistrement** `NNN-<étape>.rec`. L'automate étant acyclique,
une opération compte au plus 11 enregistrements validés (enveloppe E2,
§5.9) : `NNN` va de `001` à `011`, et un numéro supérieur est une ambiguïté.

```text
{"at":"2026-11-05T10:15:04Z","op":"20261105T101500-1a2b3c4d","prev":"<sha256 de l'enregistrement précédent>","schema":"savewarden.journal","schema_version":1,"seq":5,"step":"publish-intent", …}
sha256 <SHA-256 hexadécimal des octets de la première ligne, LF compris>
```

- Première ligne : JSON canonique, clés triées, UTF-8 sans BOM, une seule ligne.
- `prev` : SHA-256 du fichier de l'enregistrement précédent ; `null` pour `001`.
  La chaîne fixe l'ordre et interdit toute substitution.
- Champs propres à l'étape : identités volume + index de fichier attendues
  avant et après, chemins relatifs, empreinte du plan ; `001-planned` contient
  le plan complet et la version de l'outil.

**Validation d'un enregistrement, dans cet ordre :**

1. créer `NNN-<étape>.rec.tmp-<k>` en `CREATE_NEW`, avec le plus petit `k`
   libre de 1 à 9 (enveloppe E1, §5.9), pour qu'un temporaire laissé par un
   crash n'empêche pas une nouvelle tentative ; si les neuf noms existent,
   aucun essai : code 3, enveloppe épuisée ;
2. écrire les deux lignes en entier ;
3. `FlushFileBuffers`, puis fermer ;
4. renommer vers `NNN-<étape>.rec` par `MoveFileEx` sans drapeau, avec les
   retries du §5.7 ;
5. l'enregistrement n'est **validé** que si le nom final existe, que sa somme
   de contrôle et sa chaîne sont correctes.

Aucune action protégée par un enregistrement ne commence avant sa validation.
Un temporaire n'est jamais supprimé, jamais réutilisé, jamais lu comme
enregistrement. Receipts et marqueur suivent le même procédé.

**Ce que prouve un enregistrement.** Trois niveaux, jamais confondus :

| Niveau | Ce qui existe | Ce que cela indique | Ce que cela ne prouve pas |
| --- | --- | --- | --- |
| Préparé | Temporaire `*.rec.tmp-<k>` seulement | Une intention a été rédigée ; **en exécution normale**, son action n'a pas commencé. C'est un invariant de protocole, pas une certitude après une coupure de courant (§5.6) | Rien sur le coffre : l'observation doit montrer l'état d'avant ; toute observation contraire est une contradiction, code 3, sans récupération conjecturale |
| Validé | Enregistrement final correct | L'action est **autorisée** et a pu être effectuée, en tout ou partie, ou pas du tout | Que l'action a eu lieu ; seule l'observation des identités le dit |
| Réalisé | Observation conforme à l'état d'après de l'intention validée, ou enregistrement terminal **et** observation conforme | L'action a eu lieu | — |

Un journal incomplet n'autorise jamais à conclure. L'absence d'un
enregistrement ne prouve pas qu'une action n'a pas eu lieu, et sa présence ne
prouve pas qu'elle a eu lieu. Après une coupure, le journal peut être en
retard comme en avance sur le coffre : un journal absent ou incomplet n'est
pas la preuve qu'aucune mutation n'a eu lieu. Seule une observation prévue par
les tableaux des §5.4 et §5.5 permet de décider ; toute autre est une
ambiguïté.

**Lecture du journal d'une opération** : enregistrements finaux de numéros
consécutifs depuis `001`, sommes et chaîne correctes, `op` égal au dossier,
étape connue du schéma, transition permise par l'automate de §5.4 ou §5.5.
Un ou plusieurs temporaires du numéro **suivant le dernier validé** signalent
une intention préparée : l'observation doit montrer l'état d'avant cette
intention, sinon ambiguïté.

**Ambiguïtés, toutes fail-closed, code 3, rien déplacé, rien écrit :**

| Cas | Pourquoi |
| --- | --- |
| Enregistrement final illisible, somme fausse ou JSON tronqué | Impossible par le protocole sauf corruption : jamais ignoré |
| Numéro manquant ou dupliqué, chaîne rompue, numéro supérieur à `011` | Ordre ou contenu non prouvé, enveloppe E2 dépassée |
| Étape inconnue, schéma inconnu, transition interdite | Automate non reconnu |
| Temporaire d'un autre numéro que le suivant ; neuf temporaires du numéro suivant | Historique incohérent ; enveloppe E1 épuisée |
| Second `rollback-failed` pour la même intention | Enveloppe E3 dépassée |
| Enregistrement d'une autre opération dans le dossier | Mélange d'opérations |
| Deux opérations non terminales pour un jeu | Exclusivité violée |
| Enregistrement après un état terminal | Opération close modifiée |
| Observation contraire au dernier enregistrement, dans un sens ou dans l'autre : `published` sans `current` de l'identité attendue, ou `current` publié alors que `publish-intent` manque | Durabilité ou intervention extérieure non prouvées |
| Identité de fichier indisponible, dossier du journal illisible | Observation impossible |
| `staging\<op>`, `current` ou `retained\<op>` qu'aucun journal n'explique, par nom ou par identité | Objet d'origine inconnue |
| Temporaire du numéro suivant alors que l'observation montre l'état d'après | Action réalisée sans intention validée : protocole violé ou ordre perdu par une coupure |

Le diagnostic nomme chaque fichier en cause et renvoie au guide de
récupération. Hors des cas prévus par les tableaux de récupération des §5.4
et §5.5, Savewarden ne tente aucune réparation.

### 5.4 Initialisation du coffre et première publication (B2b)

#### Initialisation du coffre, avant tout journal

Le journal vit dans `games\<game-id>\journal\` : il ne peut pas protéger la
création du coffre qui le contient. L'initialisation suit donc un protocole
propre, sans journal, fait uniquement de créations exclusives (I11), dans cet
ordre exact :

| # | Étape | État laissé par un arrêt juste après | Traitement au lancement suivant |
| --- | --- | --- | --- |
| I1 | Préflight : `<vault>` absent avec parent existant, ou dossier vide, sur volume NTFS local | Rien | — |
| I2 | `CreateDirectory(<vault>)` s'il est absent ; s'il apparaît entre-temps, retour au préflight | `<vault>` vide | État « vide », accepté : l'initialisation reprend en I3 |
| I3 | `savewarden-vault.json.tmp-<k>` en `CREATE_NEW`, écrit, flushé, fermé | `<vault>` ne contient **que** des temporaires de marqueur, fichiers ordinaires | Forme reconnue « initialisation interrompue » : signalée par le mode plan, refusée par `--apply`, code 3 ; seul `--recover --apply` reprend en I3 avec le `k` libre suivant |
| I4 | Validation : `MoveFileEx(temporaire, savewarden-vault.json, 0)` | Marqueur valide, temporaires éventuels | Coffre initialisé, sans jeu |
| I5 | `CreateDirectory(games)`, puis `CreateDirectory(games\<game-id>)`, chacun seulement s'il manque | Dossiers vides | Normal ; un dossier existant est contrôlé sans suivi : dossier ordinaire, aucun point d'analyse |
| I6 | `sync.lock` : `CREATE_NEW`, ou ouverture de l'existant, sans partage | Verrou vide ; le handle est libéré par l'OS | Normal |
| I7 | Sous verrou : état du coffre relu, espace revérifié (V2) | Métadonnées seules | En cas d'échec ici : code 1 ou 2, `vault: metadata only; nothing staged or published` |
| I8 | `CreateDirectory(journal)` s'il manque, puis `CreateDirectory(journal\<op>)`, exclusif | Dossier d'opération vide, ou ne contenant que des temporaires `001-…` | « Opération non commencée » : bloque `--apply` (I8) ; `--recover --apply` valide `001-abandoned` si aucun `staging\<op>` n'existe |
| I9 | Validation de `001-planned` | Opération commencée | Automate ci-dessous |

Refus sans aucune mutation :

- `<vault>` qui contient des temporaires de marqueur **et** toute autre entrée :
  dossier non vide sans marqueur, code 1, jamais adopté ;
- marqueur final illisible ou de schéma inconnu, par exemple après une
  coupure : code 1 ; jamais réécrit, remplacé ni complété ;
- marqueur valide mais entrée imprévue à la racine du coffre, dans `games`
  ou dans `games\<game-id>` : code 3 ;
- lien, jonction ou point d'analyse à n'importe quel niveau : code 1 ;
- neuf temporaires de marqueur déjà présents : enveloppe E1 épuisée, code 3.

Un coffre incomplet n'est **jamais requalifié automatiquement** en coffre
sain. La seule reprise d'une initialisation interrompue est `--recover
--apply`, explicite ; elle se limite à valider un nouveau marqueur dans un
dossier qui ne contient que des temporaires de marqueur, eux-mêmes conservés.

#### Automate de première publication

Sous verrou, aucun `current` existant :

| # | Enregistrement validé | Action qui suit |
| --- | --- | --- |
| 1 | `planned` : plan complet | — |
| 2 | `staging-intent` : chemin du staging, absent attendu | Créer `staging\<op>`, copier, flusher |
| 3 | `staged` : identité du staging, empreintes copiées | Second inventaire de la source, relecture du staging |
| 4 | `verified` : empreinte de la relecture | — |
| 5 | `publish-intent` : identité du staging, `current` absent attendu | Bascule `staging\<op>` → `current` (ordre ci-dessous) ; receipt validé par renommage |
| 6 | `published` : SHA-256 du receipt | Terminal |

Terminaux : `published`, `failed`, `abandoned`, `rolled-back`. Le seul
déplacement de rollback d'une première publication est `current` →
`staging\<op>`, précédé d'un `unpublish-intent` validé (I3). `rollback-failed`
n'est pas terminal ; il en existe au plus un par intention (E3).

**Notation des observations.** Les tableaux de récupération notent l'identité
trouvée sous chaque nom : `C` pour `current`, `S` pour `staging\<op>`, `R` pour
`retained\<prev-op>`. `new` est l'identité du staging relevée dans `staged` ;
`old` celle de `current` relevée dans `retire-intent` ; `—` un nom absent.
Toute autre identité sous l'un de ces noms est une ambiguïté.

Récupération `--recover --apply` d'une première publication :

| Dernier validé | Observation (`C`, `S`) | Action |
| --- | --- | --- |
| Aucun : dossier d'opération vide ou temporaires `001` seuls | (—, —) | `001-abandoned` |
| `planned` | (—, —) | `abandoned` |
| `staging-intent` | (—, —), ou (—, dossier ordinaire sans point d'analyse) | `abandoned` ; staging laissé en place, ni lu ni déplacé |
| `staged`, `verified` | (—, `new`) | `abandoned` ; staging laissé en place |
| `publish-intent` | (—, `new`) | Renommage non effectué : `rolled-back` |
| `publish-intent` | (`new`, —) | Renommage effectué : revérification intégrale de `current` contre le plan ; égal : receipt (§4.7) puis `published` ; différent : `unpublish-intent`, puis `current` → `staging\<op>` |
| `unpublish-intent` | (`new`, —) | Déplacement non effectué : l'exécuter, puis `rolled-back` ; s'il échoue : `rollback-failed`, code 3 |
| `unpublish-intent` ou `rollback-failed` | (—, `new`) | Déplacement effectué : `rolled-back` |
| `rollback-failed` | (`new`, —) | Nouvel essai ; succès : `rolled-back` ; échec : code 3, sans nouvel enregistrement (E3) |
| `published` | (`new`, —), receipt conforme | Rien |
| Tout autre cas, dont un `staging\<op>` présent avant `staging-intent`, un `S` absent après `staged`, ou `C` et `S` tous deux présents | — | Fail-closed, code 3 |

Un `current` existant et différent du plan est refusé avant tout staging
(`current-exists`, code 2).

#### Ordre d'une bascule, handles et identités

Appliqué à chaque renommage d'un dossier d'opération : publication,
rotation, rollback.

1. **Fermer** toutes les portées de lecture ouvertes sur le dossier à déplacer
   et sur sa descendance, dont la portée de vérification du staging et celle
   du contrôle de `current` : leurs épingles, ouvertes sans
   `FILE_SHARE_DELETE` comme en B1, bloqueraient le renommage.
2. Ne garder épinglés que `<vault>`, `games`, `games\<game-id>` et
   `journal\<op>`. Hypothèse : un handle sur un dossier parent n'empêche pas
   de renommer un enfant ; elle est vérifiée par un test Windows réel avant
   tout code de bascule (§9.2).
3. **Pré-contrôle** : ouvrir le dossier source sans suivi
   (`FILE_FLAG_OPEN_REPARSE_POINT`, `FILE_FLAG_BACKUP_SEMANTICS`), avec partage
   lecture, écriture et suppression ; identité égale à celle de l'intention ;
   aucun point d'analyse ; nom cible absent ; refermer le handle aussitôt.
4. `MoveFileEx(source, cible, 0)`, avec les retries du §5.7.
5. **Post-contrôle** : ouvrir la cible sans suivi ; identité égale à celle de
   l'intention ; nom source absent.
6. **Course** : `MoveFileEx` opère sur des noms ; un remplacement du dossier
   source entre 3 et 4 n'est pas empêché. Le post-contrôle le détecte :
   identité inattendue, aucun autre déplacement, l'objet inconnu n'est ni lu
   ni déplacé, code 3. Cette fenêtre relève du processus hostile, hors modèle
   (§6.1). Le renommage par handle, `FileRenameInfo`, la supprimerait, mais il
   exige `SetFileInformationByHandle`, interdit par le garde parce que la même
   API supprime aussi par `FileDispositionInfo` : écarté en B2.

#### Points d'arrêt de la bascule `staging → current`

Le dossier publié est identifié par son identité volume + index de fichier,
relevée sur le staging épinglé, inscrite dans `staged` et `publish-intent`,
contrôlée avant et après le renommage, et conservée par celui-ci ; jamais par
son seul nom.

| # | Arrêt du processus | Journal observé | Coffre observé | Récupération |
| --- | --- | --- | --- | --- |
| A1 | Avant validation de `publish-intent` | `verified` ; éventuels temporaires `005-…tmp-<k>` | (—, `new`) | Intention seulement préparée : `abandoned`, staging conservé |
| A2 | Après validation, avant renommage | `publish-intent` validé | (—, `new`) | Renommage non effectué : `rolled-back` |
| A3 | Pendant ou juste après le renommage, retour reçu ou non | `publish-intent` validé | (`new`, —) ; aucun receipt | Revérification intégrale de `current` contre le plan ; égal : receipt reconstruit (§4.7) puis `published` ; différent : `unpublish-intent`, `current` → `staging\<op>`, `rolled-back` |
| A4 | Après le post-contrôle, pendant l'écriture du receipt | Idem | Idem, plus `receipts\<op>.json.tmp-<k>` | Temporaires ignorés, ni lus comme receipt, ni réutilisés, ni supprimés ; nouveau temporaire au `k` libre suivant ; puis comme A3 |
| A5 | Après validation du receipt, avant `published` | Idem | Receipt final présent | Receipt relu : schéma, opération et entrées égaux au plan, `current` revérifié : `published` ; sinon fail-closed |
| A6 | Pendant l'écriture de `published` | Idem, plus temporaire `006-…tmp-<k>` | Idem | Comme A5, avec un nouveau temporaire |
| A7 | Après `published` | Terminal | Conforme | Rien |
| A8 | État non prévu, par exemple `current` d'une autre identité, `C` et `S` tous deux présents, `published` sans `current` conforme, `current` publié sans `publish-intent` | — | — | Fail-closed, code 3, rien déplacé ni écrit |

Pendant A3 à A6, le mode plan annonce « operation `<op>` pending recovery:
current published, receipt missing » : il ne confond jamais cet état avec un
`current` modifié hors de Savewarden (§4.5).

### 5.5 Rotation de `current` (B2c)

Automate, `current` existant et conforme à son receipt `<prev-op>`. Quatre
déplacements possibles, chacun précédé de sa propre intention validée :

| Déplacement | Intention | Préconditions observées (`C`, `R`, `S`) | Après |
| --- | --- | --- | --- |
| M1 retrait : `current` → `retained\<prev-op>` | `retire-intent` | (`old`, —, `new`) | (—, `old`, `new`) |
| M2 promotion : `staging\<op>` → `current` | `promote-intent` | (—, `old`, `new`) | (`new`, `old`, —) |
| M4 dépromotion : `current` → `staging\<op>` | `unpromote-intent` | (`new`, `old`, —) | (—, `old`, `new`) |
| M3 restauration : `retained\<prev-op>` → `current` | `restore-intent` | (—, `old`, `new`) | (`old`, —, `new`) |

| # | Enregistrement validé | Action qui suit |
| --- | --- | --- |
| 1–4 | `planned`, `staging-intent`, `staged`, `verified` | Comme §5.4 |
| 5 | `retire-intent` | M1, bascule selon l'ordre du §5.4 |
| 6 | `promote-intent` | M2 ; post-contrôle ; receipt |
| 7 | `published` | Terminal |

Rollback : M4 puis M3 si le nouveau `current` doit être retiré, M3 seul si la
promotion n'a pas eu lieu. `rollback-failed` : au plus un par intention (E3),
non terminal.

En cours d'exécution :

- M1 échoue : l'observation confirme (`old`, —, `new`) : `rolled-back`, code 1,
  `vault: rolled back <op>; current unchanged` ;
- M2 échoue : `restore-intent`, M3, post-contrôle, `rolled-back`, code 1 ;
  si M3 échoue : `rollback-failed`, code 3, diagnostic nommant les deux
  emplacements, tous deux intacts ;
- post-contrôle de M2 contraire à l'intention : code 3, aucun autre
  déplacement.

**Fenêtre d'absence (correction C2).** Entre M1 et M2, ou entre M4 et M3, et
jusqu'à la récupération en cas de crash dans cet intervalle, **`current`
n'existe pas**. Ce n'est pas un échange atomique : la sûreté vient du journal,
des identités et de la récupération. Un jeu, l'Explorateur ou `inspect` qui
regarde le coffre à cet instant ne voit pas de `current`. La documentation
demande de ne pas accéder au coffre pendant un `sync` ; le mode plan annonce
toute opération en attente.

Récupération, tableau fermé :

| Dernier validé | Observation (`C`, `R`, `S`) | Action |
| --- | --- | --- |
| `planned` à `verified` | (`old`, —, `S` selon les règles du §5.4) | `abandoned` |
| `retire-intent` | (`old`, —, `new`) | M1 non effectué : `rolled-back` |
| `retire-intent` | (—, `old`, `new`) | M1 effectué : `restore-intent`, puis M3, puis `rolled-back` |
| `promote-intent` | (—, `old`, `new`) | M2 non effectué : `restore-intent`, puis M3, puis `rolled-back` |
| `promote-intent` | (`new`, `old`, —) | M2 effectué : revérification intégrale ; égal : receipt, `published` ; différent : `unpromote-intent`, puis M4 |
| `unpromote-intent` | (`new`, `old`, —) | M4 non effectué : M4, puis `restore-intent`, M3, `rolled-back` |
| `unpromote-intent` | (—, `old`, `new`) | M4 effectué : `restore-intent`, puis M3, puis `rolled-back` |
| `restore-intent` | (—, `old`, `new`) | M3 non effectué : M3, puis `rolled-back` |
| `restore-intent` | (`old`, —, `new`) | M3 effectué : `rolled-back` |
| `rollback-failed` | L'une des observations prévues pour l'intention qui le précède | Comme la ligne de cette intention ; nouvel échec du déplacement : code 3, sans nouvel enregistrement (E3) |
| `published` | (`new`, `old`, —), receipt conforme | Rien |
| Tout autre cas | — | Fail-closed, code 3 |

Chaque ligne est atteinte par un test : crash avant et après chacun des
déplacements M1 à M4, et pendant la validation de chaque enregistrement, puis
récupération répétée (§9.3).

### 5.6 Durabilité : ce qui est garanti, ce qui ne l'est pas

- `FlushFileBuffers` est appelé sur chaque fichier copié, enregistrement,
  receipt et marqueur avant son renommage de validation.
- **`MOVEFILE_WRITE_THROUGH` n'est pas utilisé** et n'est pas présenté comme
  une garantie : sa documentation ne promet le vidage sur disque que pour un
  déplacement réalisé par copie puis suppression, que Savewarden refuse.
- Aucune garantie documentée n'assure qu'un renommage de dossier survit à une
  coupure de courant, ni que l'ordre des opérations de métadonnées est
  préservé. Le journal de NTFS rend le système de fichiers cohérent après un
  crash ; il ne promet pas que la dernière opération a persisté. Ce sont des
  **hypothèses**, inscrites comme telles.
- Conséquence : après une coupure, l'état observé peut correspondre à
  n'importe quel point antérieur du protocole, et le journal peut être en
  retard ou en avance sur le coffre. La récupération repose sur l'observation ;
  une combinaison non prévue par les tableaux est une contradiction, donc
  fail-closed (I4, I5), sans récupération conjecturale.
- Caches d'écriture matériels qui mentiraient sur un flush : hors modèle.
- ReFS : non qualifié tant que ces comportements n'y sont pas testés.

### 5.7 Retries bornés

- Uniquement sur les renommages de dossiers et de validation, et
  **exclusivement** sur `ERROR_SHARING_VIOLATION` (32) et
  `ERROR_LOCK_VIOLATION` (33) (V4).
- `ERROR_ACCESS_DENIED` (5) reste un **échec immédiat**, même si un antivirus
  ou l'Explorateur en est la cause probable : le diagnostic invite à fermer
  les programmes qui utilisent le coffre. Toute autre erreur : échec immédiat.
- La liste 32 et 33 est fermée pour B2 ; l'étendre exigerait une décision du
  propriétaire et un amendement d'ADR 0006.
- Backoff exponentiel par horloge injectée, nombre de tentatives borné,
  **5 secondes au plus** au total par renommage ; jamais de boucle infinie.
- Pendant la section critique, de `publish-intent` ou `retire-intent` jusqu'à
  l'état terminal, l'annulation est différée jusqu'à la fin de la bascule ou
  du rollback.

### 5.8 Ce que publication et récupération ne font jamais

Supprimer ; remplacer ; copier vers un nom publié ; déplacer entre volumes ;
suivre un lien ; déplacer ou lire un objet dont l'identité n'est pas celle
attendue ; agir sur un journal ambigu ; écrire dans un dossier non marqué,
hormis l'initialisation I2 à I4 d'un dossier absent, vide ou ne contenant que
des temporaires de marqueur ; supprimer un temporaire pour rouvrir
l'enveloppe ; toucher une génération `retained\` autre que celle retirée par
la même opération, lors de son propre rollback.

### 5.9 Propriétés de la récupération

| Propriété | Règle | Preuve exigée |
| --- | --- | --- |
| Explicite | Jamais automatique ; `--recover` sans `--apply` affiche l'action exacte sans rien écrire | Test sous ACL refusant l'écriture |
| Idempotente | Sur une opération terminale : rien à faire, code 0, coffre inchangé ; relancer `--recover --apply` après succès ne change rien | Empreinte complète du coffre identique |
| Interruptible | Chaque déplacement de récupération est précédé de son intention validée (`unpublish-intent`, `unpromote-intent`, `restore-intent`) ; receipts et enregistrements passent par un temporaire ; un crash pendant la récupération laisse un état prévu par les tableaux | Énumération des crashs **pendant** la récupération, puis nouvelle récupération |
| Convergente, **dans l'enveloppe** | Pour un état observé donné, la récupération aboutit toujours au même état terminal, quel que soit le nombre de ses propres interruptions tant que l'enveloppe E1–E4 n'est pas dépassée | Même état final pour toutes les séquences de crash testées |
| Bornée | Hors de l'enveloppe : code 3, `vault: intervention required for <op>: recovery envelope exhausted`, aucune écriture ni déplacement | Test de dépassement de chaque limite E1 à E3 |
| Monotone | La récupération n'ajoute que des enregistrements et n'effectue que des renommages sans remplacement vers des noms absents ; elle ne supprime, n'écrase et ne réécrit jamais | Garde statique ; cible existante : échec, état inchangé |
| Non aggravante | Face à une ambiguïté, chaque exécution s'arrête avant toute écriture, avec le même code 3 et le même diagnostic ; une ambiguïté ne devient jamais un état « réparé » par répétition | Trois exécutions successives : coffre identique octet pour octet, diagnostic identique |

**Enveloppe de récupération.**

| # | Limite | Justification |
| --- | --- | --- |
| E1 | Au plus 9 temporaires par fichier validé par renommage : enregistrement, receipt, marqueur | Une relance n'en consomme un que si elle est interrompue pendant l'écriture de ce fichier ; aucun temporaire n'est supprimé |
| E2 | Au plus 11 enregistrements validés par opération | L'automate est acyclique ; son plus long chemin est la rotation dépromue puis restaurée, avec un `rollback-failed` par intention : `001` à `011` |
| E3 | Au plus un `rollback-failed` par intention de rollback | Un nouvel échec du même déplacement n'ajoute rien au journal et sort en code 3 |
| E4 | Retries de renommage sur 32 et 33 seulement, 5 secondes au plus | §5.7 |

Au-delà de l'enveloppe, Savewarden ne promet aucune convergence. L'état reste
tel quel, décrit par le diagnostic ; le guide du coffre explique comment
l'examiner, sans commande de suppression. Savewarden ne supprime jamais un
temporaire pour rouvrir l'enveloppe : toute intervention sur le coffre
relève de son propriétaire.

## 6. Non-destruction et garde de lecture seule

### 6.1 Politique

- Aucun appel de suppression, troncature, remplacement ou réécriture dans le
  code de production, y compris pour un staging échoué, un temporaire de
  journal ou le verrou (D7).
- Créations en `CREATE_NEW`, dossiers créés un par un, échec s'ils existent ;
  renommages par `MoveFileEx` sans drapeau.
- Aucun snapshot approuvé n'est utilisé comme destination, réécrit ou
  déplacé : la règle générique du §4.2 refuse tout coffre non créé par
  Savewarden.
- Aucun suivi implicite de lien : côté source par la portée B1 ; côté coffre
  par ancêtres épinglés, ouverture `FILE_FLAG_OPEN_REPARSE_POINT` et contrôle
  sur handle de chaque fichier et dossier créé. `os.Root` n'est jamais
  l'unique garantie.
- Aucune exécution de fichier, aucun processus lancé, aucun réseau, aucun
  chargement dynamique : interdits maintenus partout.
- Modèle de menace : accidents, outils concurrents bénins, crash et coupure.
  Un processus hostile du même compte, capable d'écrire dans le coffre, reste
  hors modèle ; les contrôles d'identité le signalent sans le prévenir.

### 6.2 Assouplissement de `internal/app/readonly_test.go` (B2b)

| Règle | Portée |
| --- | --- |
| Imports interdits `os/exec`, `net`, `net/http`, `plugin` | Inchangés, partout |
| Interdits renforcés partout, `platform/vault` compris | `Remove`, `RemoveAll`, `DeleteFile`, `RemoveDirectory`, `Rename`, `MoveFile`, `MoveFileWithProgress`, `MoveFileTransacted`, `ReplaceFile`, `MOVEFILE_REPLACE_EXISTING`, `MOVEFILE_COPY_ALLOWED`, `MOVEFILE_DELAY_UNTIL_REBOOT`, `MOVEFILE_CREATE_HARDLINK`, `MOVEFILE_WRITE_THROUGH`, `Truncate`, `SetEndOfFile`, `O_TRUNC`, `O_APPEND`, `CREATE_ALWAYS`, `OPEN_ALWAYS`, `TRUNCATE_EXISTING`, `FILE_FLAG_DELETE_ON_CLOSE`, `SetFileInformationByHandle`, `Link`, `Symlink`, `CreateSymbolicLink`, `CreateHardLink`, `DeviceIoControl`, `SetFileAttributes`, `Chmod`, `Chown`, exécution, chargement dynamique, appels système bruts |
| Permis **seulement** dans `internal/platform/vault` | `GENERIC_WRITE`, `FILE_WRITE_ATTRIBUTES`, `CREATE_NEW`, `CreateDirectory`, `FlushFileBuffers`, `SetFileTime`, `MoveFileEx` ; chaque entrée justifiée en commentaire |
| Requêtes en lecture seule | `GetVolumeInformation`, `GetDriveType`, `GetDiskFreeSpaceEx` viennent de `x/sys/windows`, confiné à `platform/vault` par le test d'imports |
| Règle d'appel | Dans `platform/vault`, chaque appel à `MoveFileEx` passe le littéral `0` comme drapeaux ; vérifié par le garde AST |
| Tous les autres packages de production | Garde B1 inchangé |
| B2c | Aucune API permise ajoutée : `readonly_test.go` inchangé depuis B2b |

Tests du garde : chaque API permise est refusée hors de `platform/vault` ;
chaque interdit renforcé est refusé dans `platform/vault` ; un `MoveFileEx`
avec un drapeau non nul est refusé ; un nom voisin, par exemple
`internal/platform/vaultx`, n'hérite pas de l'exception. Le test d'imports
refuse `golang.org/x/sys/windows` hors de `platform/vault`. Le garde reste un
filet statique fondé sur les noms : il est complété par les tests dynamiques
de non-mutation de la source, d'absence d'écriture du mode plan et de création
exclusive.

## 7. Périmètre des fichiers

| PR | À créer | À modifier |
| --- | --- | --- |
| B2a | `internal/entities/backup/*` ; `internal/features/sync/*` (préflight, plan, rapport, lecture du journal) ; `internal/cli/sync.go` et tests ; `docs/adr/0004-sync-boundaries.md`, `0006-publication-recovery-protocol.md`, `0007-sync-contract-receipt-v1.md` ; `docs/commands/sync.md` ; `docs/vault.md` ; `docs/receipt-v1.md` ; `docs/validation/b2a.md` | `internal/features/inspect/*`, `internal/cli/root.go` et tests, `internal/app/app.go` et tests, `internal/app/architecture_test.go`, `cmd/savewarden/main_test.go`, `README.md`, `AGENTS.md`, `SECURITY.md`, `CHANGELOG.md`, `docs/architecture.md`, `docs/safety-model.md` |
| B2b | `internal/platform/vault/{vault.go, vault_windows.go, vault_other.go, *_test.go}` ; `internal/features/sync/{copy.go, publish.go, recover.go}` et faux coffre ; `internal/app/sync_windows_test.go` ; `docs/adr/0005-write-allowed-package.md` ; `docs/validation/b2b.md` | `internal/app/readonly_test.go`, `internal/app/architecture_test.go`, `go.mod`, `go.sum`, `cmd/savewarden/main_test.go`, docs de B2a, `CONTRIBUTING.md`, `CHANGELOG.md`, `AGENTS.md`, `docs/pilots/odyssey.md` |
| B2c | `internal/features/sync/rotate.go` et tests ; `docs/validation/b2c.md` | `internal/features/sync/*`, `internal/cli/sync.go`, docs, ADR 0006 et 0007 si amendés, `CHANGELOG.md`, `docs/pilots/odyssey.md` |

Changements interdits dans les trois PR :

- toute écriture hors de `internal/platform/vault`, toute suppression ou tout
  remplacement de fichier, partout ;
- les packages `features/snapshot`, `restore`, `doctor`, et tout dossier vide ;
- toute modification du schéma de manifeste v1 ;
- tout chemin personnel ou propre au pilote dans le code, les tests ou les
  exemples ;
- toute modification des rulesets, des versions d'Actions ou de la CI hors
  ajout de contrôles ;
- tout accès agent à `D:\Ga`, `D:\Savewarden`, `E:\GMS` ou aux clés SSH ;
- tout test lisant ou écrivant hors de `t.TempDir()`.

## 8. Plan d'exécution

Chaînage, chaque étape attendant la précédente :

```text
validation finale du plan v0.2 → GO B2a → PR B2a → revue → Human GO → merge
  → GO B2b → PR B2b → revue indépendante platform/vault → Human GO → merge
  → pilote B2b, première publication, qualifié par le propriétaire
  → GO B2c → PR B2c → revue de sécurité indépendante → Human GO → merge
  → pilote B2c, rotation
```

Chaque PR cible `release/v0.1.0-a1` (D15), part de sa tête à jour, commits
atomiques qui compilent et passent les tests, 7 checks verts sur le SHA exact,
branche supprimée après fusion. Aucun gel, tag ni release dans B2.

Publication du plan (V3) : après le GO du propriétaire sur le plan complet,
`feat/b2a-sync-plan` part de la tête à jour de `release/v0.1.0-a1` et son
**premier commit** ajoute le plan validé, sous le nom `…_v0.2.md`. Aucune PR
documentaire séparée. La v0.2 est la seule version présente dans l'arbre ; le
brouillon v0.1 et les révisions de revue restent dans l'historique local de
`docs/b2-sync-plan`, jamais publiés. Les amendements de la revue finale sont
intégrés à la v0.2 elle-même, à la demande du propriétaire.

### 8.1 PR B2a — `feat/b2a-sync-plan` : conception et plan, sans aucune écriture

| # | Commit | Contenu | Point de contrôle |
| --- | --- | --- | --- |
| 1 | `docs(plan): add the reviewed B2 plan v0.2` | Plan validé, seule version de l'arbre (V3) | — |
| 2 | `docs(adr): sync boundaries` | ADR 0004 | — |
| 3 | `docs(adr): publication and recovery protocol` | **ADR 0006**, contenu du §5 | **Revue de sécurité de la conception** |
| 4 | `docs(adr): sync contract and receipt v1` | ADR 0007 | — |
| 5 | `refactor(backup): move inventory types to entities/backup` | Sortie `inspect` identique octet par octet | — |
| 6 | `feat(backup): diff, receipt and journal models` | Différence, receipt v1, enregistrement de journal, validation pure fail-closed ; tests par tableaux | — |
| 7 | `feat(inspect): expose no-follow directory inventory` | Aucun changement de sortie | — |
| 8 | `feat(sync): read-only preflight and plan` | Ports, distinction, état du coffre et du journal, §4.4, §4.5 ; faux systèmes | — |
| 9 | `feat(cli): add sync plan mode` | Flags absents de B2a = code 1 ; codes 0/1/2/3 en lecture ; smoke binaire sous ACL refusant l'écriture | **Revue du plan de bout en bout** |
| 10 | `docs: document B2a` | Commande, coffre, guide de récupération initial, architecture, sécurité, SECURITY, AGENTS, CHANGELOG | — |
| 11 | `docs: add B2a validation report` | Commandes, résultats, CI, limites | Revue finale, Human GO |

`readonly_test.go` reste **inchangé** : B2a est entièrement en lecture seule.

### 8.2 PR B2b — `feat/b2b-first-publication` : première publication sécurisée

| # | Commit | Contenu | Point de contrôle |
| --- | --- | --- | --- |
| 1 | `test(arch): allow create-only writes in platform/vault` | ADR 0005 ; garde assoupli pour un package ; règle `MoveFileEx` à drapeau `0` ; `x/sys/windows` en version exacte, limité par le test d'imports | **Revue de l'assouplissement** |
| 2 | `feat(vault): create-only vault adapter` | Verrou, marqueur, dossiers et fichiers `CREATE_NEW`, flush, date, validation par renommage, `MoveFileEx`, identité, volume NTFS local, espace libre pour l'appelant ; test préalable des handles parents (§5.4) ; tests Windows réels | — |
| 3 | `feat(sync): vault initialization protocol` | Étapes I1 à I9, états partiels reconnus ou refusés, crash après chaque étape | — |
| 4 | `feat(sync): staging copy and verification` | Copie hachée, détection de changement, second inventaire, relecture ; injection de fautes | — |
| 5 | `feat(sync): first publication and receipt` | Automate §5.4, ordre de bascule, receipt et sa provenance ; `current-exists` pour un `current` existant et différent | — |
| 6 | `feat(sync): recover an interrupted first publication` | `--recover`, plan puis `--apply` ; initialisation interrompue ; points A1 à A8 ; enveloppe E1 à E4 ; énumération des crashs, récupération comprise | **Revue de récupération** |
| 7 | `feat(cli): sync --apply and --recover` | Câblage, ligne d'état du coffre, smoke binaire avec source en écriture refusée | — |
| 8 | `docs: document B2b` | Commande, guide de récupération, pilote, CONTRIBUTING, AGENTS, CHANGELOG | — |
| 9 | `docs: add B2b validation report` | Preuves, revue indépendante consignée | Revue indépendante `platform/vault`, Human GO |

**Revue indépendante obligatoire** de `platform/vault` avant le Human GO :
écritures Windows, accès concurrents, journal, récupération. Elle est
consignée dans `docs/validation/b2b.md` ; elle ne vaut pas approbation de PR
et le mainteneur unique ne s'auto-approuve pas.

### 8.3 PR B2c — `feat/b2c-rotation` : rotation et récupération complète

Préalable : B2b fusionnée **et** première publication qualifiée par le
propriétaire (D4), puis GO B2c.

Amendé par les arbitrages du §0.8. L'ancien commit 1, amendement d'ADR 0006
« seulement si la qualification révèle un écart », est fait par le cadrage :
l'écart était dans le protocole de rotation, non dans la qualification. Il
précède la branche, dans une PR documentaire ou en tête de la branche B2c,
au choix du propriétaire. La PR B2c garde sept commits :

| # | Commit | Contenu | Point de contrôle |
| --- | --- | --- | --- |
| 0 | `docs(adr): amend ADR 0006 for the B2c framing` | Fait au cadrage : A1, A2, M3, protection du receipt en rotation ; puis R1 à R4 de la revue | **Revue du cadrage**, préalable au GO |
| 1 | `feat(backup): persistent identity of the replaced current` | `current_identity` de `planned` obligatoire ou interdit selon le mode ; `Journal.Old` fixé par `planned` ; cohérence de `retire-intent` et `restore-intent` ; identité publiée d'une opération terminée, lue dans `staged`, `publish-intent` ou `promote-intent` ; chaîne de la génération précédente reconnue (R2) et table M3 (R1), pures ; compatibilité des journaux B2b | — |
| 2 | `feat(sync): rotate current into retained with rollback` | Génération reconnue sous verrou avant `planned`, mode plan compris ; relecture juste avant le retrait ; automate §5.5 ; M1, observation exacte et relecture de `retained` avant M2 (R3), M2, rollback par restauration vérifiée, retries ; le refus `current-exists` remplacé par la rotation, le code conservé | — |
| 3 | `feat(sync): removals and missing sources policy` | §4.4, D5, V1, A3 : `--allow-removals`, `--accept-missing-source` répétable | — |
| 4 | `feat(sync): recover an interrupted rotation` | Tableau §5.5, restaurations vérifiées comprises ; règle M3, première publication comprise ; énumération des crashs à chaque transition et pendant la récupération | **Revue du remplacement** |
| 5 | `feat(cli): rotation flags and output` | Flags, rendu, `rolled back`, ADR 0007 amendé (§0.8, R5) ; test de complétude des codes documentés ; smoke binaire de rotation | — |
| 6 | `docs: document B2c` | Guide de récupération complet, accumulation des générations (A4), sécurité, pilote, ADR 0004 corrigé, CHANGELOG | — |
| 7 | `docs: add B2c validation report` | Preuves, revue de sécurité consignée | Revue de sécurité indépendante, Human GO |

Le commit 1 ajoute au découpage d'origine la part « entités » du contrat
A1 et M3, pour qu'elle soit revue seule avant la rotation ; c'est le seul
écart au §8.3, compensé par le commit 0 sorti de la PR.

## 9. Matrice de tests

Données exclusivement synthétiques, construites en `t.TempDir()` ou en
mémoire. Aucune arborescence dont le nom figure dans `.gitignore`, comme
`current` ou `saves`, n'est versionnée : elles sont construites dans le code.
Chemins d'exemple fictifs uniquement.

### 9.1 B2a — entités, plan, CLI en lecture seule (tous OS sauf mention)

- **Différence** : ajouté, modifié par taille, date ou SHA-256 seuls, retiré,
  inchangé, dossiers vides, entrées non triées, sources ajoutées ou retirées.
- **Receipt** : aller-retour exact ; champ ou schéma inconnu, JSON tronqué,
  date non UTC, SHA-256 mal formé : refus.
- **Journal, validation pure** : chaîne valide ; somme fausse ; JSON tronqué
  dans un final ; numéro manquant, dupliqué ; chaîne rompue ; étape ou
  transition inconnue ; temporaire du numéro suivant, d'un autre numéro, en
  plusieurs tentatives ; enregistrement d'une autre opération ; deux
  opérations non terminales ; enregistrement après terminal ; numéro `012` ;
  neuf temporaires du même numéro ; second `rollback-failed` pour une même
  intention ; chaque transition des automates des §5.4 et §5.5, permise ou
  interdite. Chaque ambiguïté produit fail-closed, jamais un état deviné.
- **Classement pur des observations** : pour chaque ligne des tableaux de
  récupération des §5.4 et §5.5, l'observation prévue donne l'action prévue ;
  toute autre combinaison de (`C`, `R`, `S`) donne code 3. Classement de
  l'initialisation : chaque état laissé par I1 à I8, plus les formes refusées.
- **Receipt reconstruit** : provenance de chaque champ selon §4.7,
  `published_at` nul, `recovered_at` et `recovered_by` renseignés.
- **Préflight** : coffre absent, vide, géré, fichier, non vide sans marqueur,
  marqueur invalide ; lien dans le chemin du coffre ; coffre égal, parent ou
  enfant de l'installation ou d'une source, par casse et par identité seule ;
  même volume ; opération en attente affichée.
- **Plan** : premier `sync`, inchangé, ajouts, modifications, retraits ;
  toutes les lignes du §4.4 ; `current-modified` sur chaque type d'écart du
  §4.5 ; `unportable-name`, `encrypted-source`, `long-path`.
- **CLI** : flags de B2b et B2c refusés en code 1 ; aide racine avec `sync` ;
  le test B0/B1 qui attendait `sync` inconnu est remplacé explicitement.
- **Binaire**, Windows : mode plan sous ACL refusant toute écriture sur tout
  l'arbre, empreinte inchangée, aucun fichier dans le dossier courant.
- **Refactor** : sortie `inspect` octet pour octet identique sur les
  fixtures B1.

### 9.2 B2b — première publication (faux coffre tous OS, réel Windows)

- **Copie** : fichier vide (SHA-256 `e3b0c442…b855`), 10 000 fichiers,
  arborescence profonde, Unicode, dossiers vides ; source modifiée entre plan
  et ouverture, pendant et après la lecture ; fichier ajouté ou retiré pendant
  la fenêtre ; remplacé entre `Lstat` et ouverture ; verrouillé ; lecture qui
  échoue ; écriture refusée, disque plein au fichier N, flush qui échoue,
  cible de staging existante.
- **Vérification** : octet altéré, fichier tronqué, en trop, manquant, entrée
  remplacée par un lien dans le staging.
- **Initialisation du coffre (P0-1)** : crash juste après chacune des étapes
  I2 à I8, puis nouveau lancement : chaque état du tableau du §5.4 traité comme
  prévu ; `<vault>` vide repris ; temporaires de marqueur seuls : code 3 en
  `--apply`, reprise par `--recover --apply` avec le `k` suivant, anciens
  temporaires conservés ; temporaires de marqueur plus un fichier étranger :
  code 1, aucune mutation ; marqueur tronqué injecté : code 1, jamais réécrit ;
  entrée imprévue dans un coffre marqué : code 3 ; neuf temporaires de
  marqueur : code 3 ; dossier d'opération vide ou à temporaires `001` seuls :
  `001-abandoned` par `--recover --apply`. Empreinte complète du coffre avant
  et après chaque refus : identique.
- **Espace (V2)** : octets disponibles pour l'appelant inférieurs à la taille
  complète de la génération plus la marge au préflight : `insufficient-space`,
  rien écrit ; quota inférieur à l'espace libre du volume : c'est le quota qui
  compte ; espace suffisant au préflight puis insuffisant sous verrou : refus
  avant journal et staging, `vault: metadata only; nothing staged or
  published` ; disque plein ou quota atteint au fichier N : `failed`, code 1,
  staging conservé ; somme de tailles qui déborderait 64 bits : refus.
- **Publication** : renommage refusé par l'erreur 32 ou 33, transitoire puis
  réussi dans la limite, ou permanent ; erreur 5 et toute autre erreur jamais
  retentées ; durée totale des retries au plus 5 s sur horloge injectée ;
  `current` apparu entre plan et publication : `vault-changed` ou échec du
  renommage, rien écrasé ; receipt impossible à écrire.
- **Crash, énumération** : pour chaque appel au coffre `k` de la première
  publication, crash juste avant et juste après `k`, puis `--recover`.
  Invariants : source inchangée ; aucun fichier existant modifié ; un
  `current` présent est toujours vérifié contre le plan ; après récupération,
  opération terminale et nouveau `sync` possible ; ambiguïté = code 3 sans
  aucun déplacement. Chaque point d'arrêt A1 à A8 du §5.4 est atteint par un
  test nommé, dont A3 : crash après renommage du staging et avant receipt.
- **Récupération répétée (§5.9)** : crash à chaque étape **de la
  récupération** elle-même, puis nouvelle récupération ; même état terminal
  pour toutes les séquences dans l'enveloppe ; `--recover --apply` relancé sur
  un état terminal : code 0, empreinte du coffre identique ; état ambigu :
  trois exécutions successives, code 3, diagnostic identique, coffre identique
  octet pour octet ; `--recover` sans `--apply` sous ACL refusant l'écriture.
- **Enveloppe (P0-2)** : neuf interruptions successives pendant l'écriture du
  même enregistrement, du receipt ou du marqueur, puis dixième lancement :
  code 3 `recovery envelope exhausted`, aucune écriture ; huit interruptions
  puis succès au neuvième essai : convergence ; second échec d'un même
  déplacement de rollback : code 3 sans nouvel enregistrement.
- **Journal réel** : temporaire laissé par un crash puis nouvelle tentative
  `tmp-2` ; final tronqué injecté à la main : fail-closed ; temporaire du
  numéro suivant avec observation de l'état d'après : fail-closed ; staging
  sans journal : fail-closed ; coffre en avance sur le journal, `current`
  publié alors que `publish-intent` manque : fail-closed (P1-5).
- **Avant publication (P1-1)** : pour `planned`, `staging-intent`, `staged`
  et `verified`, chaque observation admise du §5.4 mène à `abandoned`, et un
  `staging\<op>` présent avant `staging-intent` ou absent après `staged` mène
  au code 3.
- **Receipt reconstruit (P1-4)** : crash en A3, puis `--recover --apply` :
  receipt valide, `published_at` nul, `recovered_at` renseigné,
  `publish_intent_at` égal au journal ; `sync` suivant : `current` conforme,
  « up to date ».
- **Codes et état** : chaque ligne du tableau d'états du §4.8 atteinte, dont
  `published` avec code 1 quand stdout échoue après publication.
- **Adaptateur `platform/vault`, Windows réel** : `CREATE_NEW` refuse un
  fichier existant ; création de dossier refusée s'il existe ; `MoveFileEx` à
  drapeau `0` refuse une cible existante, fichier ou dossier ; renommage de
  dossier refusé tant qu'un fichier est ouvert, code d'erreur observé
  consigné pour ADR 0006 ; jonction et symlink plantés dans le chemin du
  coffre, `games\<id>`, le staging ou substitués après contrôle : refus, aucune
  écriture hors du coffre, avec un test qu'`os.Root` seul n'aurait pas fait
  échouer ; chemins de plus de 260 caractères ; date reportée à la résolution
  NTFS de 100 ns ; verrou exclusif, libéré après mort d'un processus enfant ;
  identité stable à travers un renommage ; volume NTFS accepté ; lecteur
  réseau, FAT32 et exFAT refusés par faux `Volumes`, et par volume réel quand
  la CI peut en monter un.
- **Handles et bascule, Windows réel (P1-3)** : un parent épinglé sans
  `FILE_SHARE_DELETE` n'empêche pas de renommer un enfant, hypothèse prouvée
  ou plan amendé avant tout code de bascule ; une portée de lecture restée
  ouverte sur le dossier à déplacer bloque le renommage, d'où sa fermeture
  préalable, vérifiée ; pré-contrôle et post-contrôle d'identité autour de
  `MoveFileEx` ; course simulée par un test du package, qui substitue le
  dossier source entre pré-contrôle et renommage : post-contrôle en échec,
  code 3, aucun autre déplacement.
- **Binaire** : `--apply` avec écriture refusée sur la source et
  l'installation, empreinte de la source identique ; second `--apply` : « up
  to date » ; source modifiée puis `--apply` : `current-exists`, code 2,
  coffre inchangé ; `inspect` d'un manifeste synthétique pointant sur
  `current` : SHA-256 identiques au receipt ; processus tué pendant le
  staging d'une grande source synthétique, puis `--recover`.
- Tests de liens obligatoires en CI, comme B1.

### 9.3 B2c — rotation et récupération complète

- **Rotation** : `current` → `retained\<prev-op>`, staging → `current`,
  identités ; génération conservée identique octet par octet à son receipt.
- **Échecs** : retrait refusé, promotion refusée puis rollback réussi,
  rollback refusé : `rollback-failed`, code 3 ; `current` réapparu entre les
  deux renommages : rien écrasé ; fichier ouvert dans `current` pendant la
  rotation.
- **Fenêtre M1–M2** : `retained\<prev-op>` modifié ou remplacé après le
  retrait : ni promotion ni restauration, code 3, rien écrit ; récupération
  identique, jamais de restauration d'une génération altérée.
- **Crash, énumération à chaque transition (P0-3)** : crash avant et après
  chacun des déplacements M1 à M4, et pendant la validation de `retire-intent`,
  `promote-intent`, `unpromote-intent`, `restore-intent`, `rollback-failed`,
  du receipt et de `published`, puis `--recover`. Chaque ligne du tableau de
  récupération du §5.5 est atteinte par un test nommé ; interruptions
  successives entre M4 et M3, puis relances. Invariants du §9.2, plus :
  l'ancien `current` est toujours intact, à sa place ou dans
  `retained\<prev-op>`. Propriétés du §5.9 vérifiées sur la rotation, crash
  pendant la récupération compris.
- **Sources et retraits** : chaque ligne du §4.4, avec et sans les flags ;
  `--allow-removals` sans effet sur une source absente ; source retirée du
  manifeste toujours bloquée ; `--accept-missing-source` refusé en code 1 pour
  une source requise, présente, inconnue, absente du receipt précédent ou
  retirée du manifeste ; répété pour deux sources ; nouveau receipt
  `status: "missing"`, `accepted: true`.
- **Annulation** dans la section critique : bascule ou rollback menés à terme.
- **Binaire** : modification de la source puis rotation ; `retained\` vérifié
  par `inspect` ; `current-modified` injecté : blocage, rien déplacé.

#### Matrice des états B2c, amendée au §0.8

Chaque ligne est couverte par le test nommé, à créer en B2c ; un même test
peut couvrir plusieurs lignes par sous-tests. « Rien » signifie aucune
écriture ni aucun déplacement, coffre identique octet pour octet. Les
invariants du §9.2 s'appliquent partout, plus : l'ancien `current` est
toujours intact, à sa place ou dans `retained\<prev-op>`, identique à son
receipt, ou bien l'exécution s'arrête en code 3 sans le déplacer. Tous les
tests sont synthétiques, sur dossiers temporaires.

**Contrat du journal (A1)**, `internal/entities/backup`

| Cas | Attendu | Test |
| --- | --- | --- |
| `planned` de rotation avec `current_identity` | Lu ; `Journal.Old` égal à ce champ | `TestRotationPlannedCarriesCurrentIdentity` |
| `planned` de rotation sans `current_identity` | Ambiguïté, code 3 | `TestRotationPlannedRequiresCurrentIdentity` |
| `planned` de première publication avec `current_identity` | Ambiguïté, code 3 | `TestFirstPublicationRefusesCurrentIdentity` |
| `retire-intent` dont l'identité diffère de `planned` | Ambiguïté, code 3 | `TestRetireIntentKeepsThePlannedIdentity` |
| `restore-intent` dont l'identité diffère de `planned` | Ambiguïté, code 3 | `TestRestoreIntentKeepsThePlannedIdentity` |
| Journaux écrits par B2b, première publication, gabarits synthétiques | Lus sans changement, mêmes décisions qu'en B2b | `TestB2bJournalsStillParse` |
| Identité publiée d'une opération terminée | `staging_identity` de `staged`, égale à celle de `publish-intent` ou de `promote-intent` ; divergence : ambiguïté | `TestPublishedIdentityOfAnOperation` |

**Génération précédente reconnue (R2)**, `internal/entities/backup` pour la
chaîne, `internal/features/sync` pour le contenu

| `current` observé | Attendu | Test |
| --- | --- | --- |
| Publié par une première publication B2b : receipt de tête, journal `published`, identité de `staged`, contenu conforme | Reconnu, `old` | `TestPreviousGenerationIsRecognized` |
| Publié par une rotation : identité de `staged` reprise par `promote-intent` | Reconnu | `TestPreviousGenerationIsRecognizedAfterRotation` |
| Receipt reconstruit par la récupération, `recovered_by` renseigné | Reconnu | `TestPreviousGenerationFromReconstructedReceipt` |
| Dossier ordinaire dont aucune identité publiée n'est connue | `unidentified`, jamais `old` | `TestUnidentifiedCurrentIsNeverOld` |
| Contenu conforme, autre identité : dossier recopié, coffre copié vers un autre volume | Non reconnu : `unexpected-entry`, code 3 | `TestCurrentWithAnotherIdentityIsNotRecognized` |
| Même identité, contenu altéré | Non reconnu : `current-modified`, code 2 | `TestCurrentWithAlteredContentIsNotRecognized` |
| Receipt de tête invalide, journal non terminal ou non `published`, `receipt_sha256` différent | Non reconnu, jamais rotation ; causes B2b inchangées | `TestPreviousGenerationNeedsItsWholeProofChain` |
| Mode plan, `current` d'une autre identité que celle publiée, contenu conforme ou non | `unexpected-entry`, code 3, rien écrit ; receipt absent ou invalide : `current-modified`, code 2, comme en B2b | `TestPlanReportsACurrentNotPublishedByItsOperation` |

**Récupération avant `planned` (M3, R1)**, `internal/features/sync`,
première publication comprise. Chaque cas est joué deux fois : dossier
d'opération vide, puis temporaires `001-…` seuls ; le test B2b
`TestRecoveryOfAnUnstartedOperation` est étendu, non remplacé.

| Génération publiée attendue | Observation | Attendu | Test |
| --- | --- | --- | --- |
| Non | `current` absent | `001-abandoned` | `TestUnstartedOperationWithoutAnyGenerationIsAbandoned` |
| Oui | `current` reconnu | `001-abandoned` | `TestUnstartedOperationBesideTheRecognizedGenerationIsAbandoned` |
| Oui : receipt valide, receipt corrompu seul, ou journal `published` seul | `current` absent | Contradiction, code 3, rien | `TestUnstartedOperationWithPublishedGenerationButNoCurrentIsAmbiguous` |
| Non | `current` présent | Code 3, rien | `TestUnstartedOperationWithCurrentButNoPublishedGenerationIsAmbiguous` |
| Oui | `current` d'une autre identité, au contenu altéré, ou dont receipt ou journal sont en défaut | Code 3, rien | `TestUnstartedOperationBesideAnUnrecognizedCurrentIsAmbiguous` |
| Indifférent | `staging\<op>` présent | Code 3, rien | `TestUnstartedOperationWithAStagingIsAmbiguous` |
| Indifférent | `retained\<…>` qu'aucun receipt n'explique | Code 3, rien | `TestUnstartedOperationWithAnUnexplainedRetainedIsAmbiguous` |

**Vérifications sous verrou (A2)**, `internal/features/sync`

| Moment | Écart injecté | Attendu | Test |
| --- | --- | --- | --- |
| Entre le plan et le verrou | Contenu de `current` modifié | `current-modified`, code 2, aucun journal ; `metadata only` au plus | `TestRotationRefusesCurrentChangedBeforeTheLock` |
| Sous verrou, avant `planned` | `current` remplacé par une jonction, d'une autre identité, ou identité illisible | Code 3, aucun journal | `TestRotationRefusesAnUnrecognizedCurrentUnderLock` |
| Après `verified`, avant `retire-intent` | Contenu modifié, même identité | `failed`, `current-modified`, code 2, staging conservé, rien déplacé | `TestRotationFailsWhenCurrentChangesBeforeRetirement` |
| Après `verified`, avant `retire-intent` | `current` remplacé, autre identité | Code 3, rien enregistré ni déplacé | `TestRotationStopsWhenCurrentIsReplacedBeforeRetirement` |
| Entre `retire-intent` et M1 | `current` remplacé | Pré-contrôle en échec, code 3, rien déplacé | `TestRetirementPrecheckCatchesASwap` |
| Avant M1 | `retained\<prev-op>` déjà présent | Code 3, rien déplacé | `TestRotationRefusesAnExistingRetained` |

**Fenêtre M1–M2 et restaurations vérifiées (R3, R4)**, `internal/features/sync`

| Moment | Écart injecté | Attendu | Test |
| --- | --- | --- | --- |
| Après le post-contrôle de M1, avant `promote-intent` | Fichier de `retained\<prev-op>` modifié, ajouté ou retiré, même identité | Code 3, rien enregistré ni déplacé : journal à `retire-intent`, ni promotion ni restauration | `TestPromotionRefusedWhenRetainedChangedAfterRetirement` |
| Idem | `retained\<prev-op>` remplacé, autre identité | Idem | `TestPromotionRefusedWhenRetainedReplacedAfterRetirement` |
| Idem | `current` réapparu | Idem, rien écrasé | `TestPromotionRefusedWhenCurrentReappearsAfterRetirement` |
| Après un refus de M2 | `retained\<prev-op>` altéré depuis la relecture du point 5 | Nouvelle observation : code 3, aucun `restore-intent`, rien déplacé | `TestFailedPromotionDoesNotRestoreAnAlteredRetained` |
| `--recover` de `retire-intent`, `promote-intent`, `unpromote-intent` ou `restore-intent`, observation (—, `old`, `new`) | `retained\<prev-op>` altéré | Code 3, rien écrit ; trois exécutions, même diagnostic | `TestRecoveryDoesNotRestoreAnAlteredRetained` |
| Idem | Aucun, `retained\<prev-op>` conforme | `restore-intent` s'il manque, M3, `rolled-back` | `TestRecoveryRestoresAVerifiedRetained` |
| Entre la relecture du point 5 et M2 | Écriture externe dans `retained\<prev-op>` | Non détectée, limite documentée (R4) ; aucun test ne prétend la détecter | — |

**Exécution**, `internal/features/sync`, et Windows réel pour les erreurs du
système

| Événement | Attendu | Test |
| --- | --- | --- |
| M1 refusé, erreur 32 ou 33 puis succès dans la limite | Rotation publiée | `TestRetirementRetriesSharingViolations` |
| M1 refusé, erreur 5, fichier ouvert dans `current` | `rolled-back` aussitôt, code 1, `current` inchangé | `TestRetirementRefusedByAnOpenFileRollsBack` |
| M2 refusé, observation (—, `old`, `new`) confirmée | Restauration vérifiée, `restore-intent`, M3, `rolled-back`, code 1 | `TestPromotionRefusedRestoresCurrent` |
| M2 refusé puis M3 refusé | `rollback-failed`, code 3, les deux emplacements intacts | `TestRestorationRefusedIsRecordedOnce` |
| `current` réapparu après `promote-intent`, avant M2 | M2 refusé sans écrasement ; nouvelle observation : code 3, aucune restauration, rien d'autre déplacé | `TestCurrentReappearingBetweenMovesIsNeverOverwritten` |
| Post-contrôle de M2 contraire | Code 3, aucun autre déplacement | `TestPromotionPostcheckMismatchStops` |
| Annulation demandée pendant la section critique | Bascule ou rollback menés à terme | `TestCancellationDuringRotationIsDeferred` |

**Récupération d'une rotation, tableau du §5.5**, `internal/entities/backup`
et `internal/features/sync`

| Couverture | Test |
| --- | --- |
| Classement pur des 125 observations par dernier enregistrement, comme en B2a : chaque observation a une ligne, toute autre finit en code 3 | `TestRotationRecoveryTableIsClosed` |
| Une ligne par couple dernier enregistrement, observation : `planned` à `verified`, `retire-intent`, `promote-intent`, avec et sans receipt déjà validé, `unpromote-intent`, `restore-intent`, `rollback-failed` après chacune des intentions, `published` | `TestRotationRecoveryRows` |

**Crashs et récupération répétée**, `internal/features/sync`

| Couverture | Test |
| --- | --- |
| Crash avant et après chaque appel au coffre d'une rotation : initialisation sous verrou, `planned` à `verified`, relectures, `retire-intent`, M1, relecture de `retained`, `promote-intent`, M2, receipt, `published`, chemins de rollback par M3 ; puis `--recover --apply`, qui finit à la ligne attendue | `TestCrashAtEveryRotationCall` |
| Crash à chaque appel de la récupération elle-même, pour chaque action : M3 seul, M4 puis M3, revérification puis receipt ; même état terminal pour toutes les séquences | `TestRotationRecoveryIsInterruptible` |
| Interruptions successives entre M4 et M3, puis relances | `TestRepeatedInterruptionsBetweenUnpromoteAndRestore` |
| Enveloppe : neuf temporaires d'un enregistrement de rotation ou d'un receipt, code 3 sans écriture ; huit puis succès ; plus long chemin de onze enregistrements ; second `rollback-failed` pour une même intention refusé (E3) | `TestRotationEnvelope` |
| Ambiguïté de rotation : trois exécutions, même diagnostic, coffre identique | `TestRotationAmbiguityIsStable` |

**Retraits et sources absentes (A3)**, `internal/features/sync` et
`internal/cli`

| Cas | Sans flag | Avec le flag prévu | Avec l'autre flag | Test |
| --- | --- | --- | --- | --- |
| Fichiers retirés d'une source présente | `files-removed`, code 2 | `--allow-removals` : publié, fichiers dans `retained\` | `--accept-missing-source` : toujours code 2 | `TestAllowRemovalsCoversRemovedFilesOnly` |
| Source facultative absente, connue du receipt précédent | `source-vanished`, code 2 | `--accept-missing-source <id>` : publié, `missing`, `accepted: true` | `--allow-removals` : toujours code 2 | `TestAcceptMissingSourceCoversVanishedSourceOnly` |
| Source requise absente | Code 2 | Aucun flag ne lève | — | `TestRequiredOrRemovedSourcesStayBlocked` |
| Source retirée du manifeste | Code 2 | Aucun flag ne lève | — | `TestRequiredOrRemovedSourcesStayBlocked` |
| `--accept-missing-source` nommant une source requise, présente, inconnue, absente du receipt précédent ou retirée du manifeste | — | Code 1, rien écrit | — | `TestInvalidAcceptMissingSourceIsCode1` |
| Deux sources acceptées, flag répété | — | Les deux `accepted: true` | — | `TestTwoAcceptedMissingSources` |

**Espace et conservation (A4)** : besoin de la génération complète plus la
marge, l'ancien `current` n'étant que renommé,
`TestRotationSpaceNeedsTheWholeGeneration` ; générations conservées
cumulées sans suppression, `TestRetainedGenerationsAccumulate`, et
documentées.

**Contrat de sortie (R5)**, `internal/cli` : chaque code stable déclaré par
les features figure dans la table d'ADR 0007, `current-exists` compris,
`TestStableCodesAreDocumented`.

**Windows réel, binaire et pilote**, `cmd/savewarden` et
`internal/platform/vault` : rotation réelle, `retained\` relu par `inspect`
égal à son receipt, second `--apply` à jour, `TestRotationEndToEnd` ; M1
refusé par un fichier ouvert dans `current`,
`TestRotationRefusedByAnOpenFileOnWindows` ; processus tué pendant le
staging d'une rotation puis `--recover`,
`TestSyncKilledDuringRotationThenRecovered`. Les tests B2b
`TestExistingCurrentIsRefused` et `TestSyncApplyRefusesToReplaceCurrent`
deviennent des tests de rotation : `current-exists` n'est plus attendu dans
le chemin nominal. Pilote B2c exécuté par le propriétaire après la fusion :
sauvegarde modifiée en jeu, rotation, `retained\` conforme, puis
idempotence.

### 9.4 Gates

Inchangés depuis `CONTRIBUTING.md` : `gofmt`, `go mod tidy` sans diff,
`go mod verify`, `go vet` du module et des scripts, `go test ./...` et des
scripts, `go run ./.github/scripts`, build `-trimpath`, golangci-lint 2.14.0,
govulncheck 1.6.0, `go test -race` sur Linux ; plus le garde et ses cas
négatifs. Les 7 checks requis verts sur le SHA exact de chaque PR, avec lien
du run. Une compilation croisée ne prouve que la compilation ; les tests
Windows réels ne sont prouvés que par `Verify (windows-2025)`. `govulncheck`
couvre `x/sys` ; le contrôle des pins ne change pas, faute d'outil binaire
téléchargé.

## 10. Documentation livrée avec le code

| Document | Contenu | PR |
| --- | --- | --- |
| ADR 0004, 0006, 0007 | Frontières, protocole de référence, contrat | B2a |
| ADR 0005 | Écritures autorisées | B2b |
| `docs/commands/sync.md` | Usage, modes et flags par version, sortie, codes, ligne d'état, garanties, limites | B2a, complété |
| `docs/vault.md` | Disposition du coffre, `retained\`, espace disque, fenêtre d'absence, **guide de récupération** pour chaque cause du code 3, sans aucune commande de suppression | B2a, complété |
| `docs/receipt-v1.md` | Schéma, exemple vérifié par test, confidentialité | B2a |
| `README.md` | État courant, exemple `sync` vérifié par test, avertissement même disque | Chaque PR |
| `docs/architecture.md`, `docs/safety-model.md` | Existant réel, invariants implémentés et testés, limites | Chaque PR |
| `SECURITY.md` | État réel au lieu de B0 | B2a |
| `AGENTS.md` | Lot courant, exception d'écriture limitée à `platform/vault`, permissions §2.3 | Chaque PR |
| `CONTRIBUTING.md` | Tests de `sync`, énumération des crashs | B2b |
| `docs/pilots/odyssey.md` | Protocoles B2b et B2c, résultats anonymisés | B2b, B2c |
| `CHANGELOG.md` | `Added` et `Limits` à chaque PR | Chaque PR |
| `docs/validation/b2a.md`, `b2b.md`, `b2c.md` | Preuves de chaque PR, revues consignées | Chaque PR |

Limites à documenter honnêtement : pas d'instantané atomique, jeu fermé ;
même disque physique ≠ sauvegarde hors disque ; fenêtre d'absence de
`current` pendant une rotation ; coupure de courant : état récupérable,
dernière étape non garantie ; générations, stagings et temporaires non
nettoyés ; attributs, ACL, flux alternatifs et dates de création non copiés ;
NTFS seul ; `subst` non détecté ; SHA-256 identique ≠ partie chargeable ;
receipts et sorties contiennent des chemins locaux.

## 11. Rollout, rollback et pilotes

- Le code ne rejoint `release/v0.1.0-a1` que par PR. Aucun gel, tag ni release.
- Rollback du code : revert de la PR sur la release branch. Les coffres créés
  restent lisibles par `inspect`, avec un manifeste dont l'installation est
  `<vault>\games\<id>` et la source `current\<source-id>` ; aucune source n'a
  été modifiée, rien n'est à défaire chez l'utilisateur.
- **Permissions (§2.3)** : les pilotes sont exécutés par le propriétaire.
  Savewarden y lit le manifeste choisi, éventuellement rangé sous l'ancien
  coffre, et la source `E:\GMS` ; il n'écrit que dans le coffre neuf. L'agent
  n'explore aucun de ces chemins et ne lance rien sur données réelles sans
  autorisation nominative pour cette exécution.

**Pilote B2b — première publication**, après CI verte et avant ou après merge
selon le choix du propriétaire ; il conditionne le GO B2c :

1. Binaire du SHA exact ; jeu et launcher fermés ; coffre **neuf**, absent ou
   vide, sur un volume NTFS local, distinct de l'ancien coffre, de `D:\Ga`, de
   la source et de l'installation, idéalement sur un autre disque physique.
2. `inspect` deux fois : codes 0, rapports identiques.
3. `sync` sans `--apply` : 14 ajouts au moins, aucun retrait, aucune anomalie
   bloquante, code 0 ; relire la destination affichée.
4. `sync --apply` : code 0, `vault: published <op>`, receipt présent.
5. `inspect` de la source : identique à l'étape 2.
6. `inspect` d'un manifeste écrit hors du dépôt qui pointe sur `current` :
   mêmes SHA-256 que le receipt.
7. Second `sync --apply` : « up to date », code 0, coffre inchangé.
8. Consigner nombre de fichiers, codes, lignes d'état et anomalies, sans
   SHA-256 individuel, nom de profil, chemin personnel ni receipt réel.

**Pilote B2c — rotation**, après merge de B2c :

1. Sauvegarder en jeu, fermer ; `sync` : modifications listées, code 0.
2. `sync --apply` : `vault: published <op2>`, `retained\<op1>` présent.
3. `inspect` de `retained\<op1>` : identique au receipt `op1` ; `inspect` de
   `current` : identique au receipt `op2`.
4. Consigner comme au pilote B2b.

## 12. Critères d'acceptation

**B2a**

1. Le mode plan n'écrit rien : binaire sous ACL refusant toute écriture,
   empreinte complète inchangée.
2. ADR 0004, 0006 et 0007 acceptés ; ADR 0006 contient tout le §5.
3. La validation pure du journal rejette chaque ambiguïté du §5.3, enveloppe
   comprise, et le classement pur des observations suit exactement les
   tableaux de l'initialisation, de la première publication et de la rotation.
4. Le plan applique §4.4 et §4.5 ; sortie `inspect` inchangée octet pour
   octet ; `readonly_test.go` inchangé.
5. Le schéma du receipt v1 porte les champs de provenance du §4.7 ; un receipt
   reconstruit se distingue d'un receipt normal par ses seuls champs.

**B2b**

6. `--apply` publie un premier `current` dont chaque fichier a la taille et le
   SHA-256 du plan, relus depuis le disque avant publication, avec receipt v1.
7. La source n'est jamais écrite : ACL de refus pendant `--apply`, empreinte
   identique.
8. Un changement de la source injecté à chaque phase empêche la publication.
9. Aucun fichier existant n'est écrasé, tronqué, supprimé ou remplacé ; un
   `current` existant n'est jamais remplacé.
10. Initialisation du coffre : un crash après chacune des étapes I2 à I8 laisse
    un état reconnu par le §5.4 ou refusé sans aucune mutation ; un coffre
    incomplet n'est jamais requalifié automatiquement ; seule `--recover
    --apply` reprend une initialisation interrompue.
11. L'énumération des crashs de la première publication prouve les invariants ;
    chaque point d'arrêt A1 à A8 a son test nommé, dont A3 entre renommage et
    receipt ; l'ordre de bascule ferme les portées, contrôle l'identité avant
    et après chaque renommage, et une course détectée donne code 3 sans autre
    déplacement.
12. La récupération respecte chaque propriété du §5.9 dans l'enveloppe E1 à
    E4 : crash pendant la récupération puis relance, convergence, idempotence,
    coffre identique octet pour octet après des exécutions répétées sur un état
    ambigu ; chaque dépassement d'enveloppe donne code 3 sans mutation ; un
    receipt reconstruit suit la provenance du §4.7.
13. Espace libre contrôlé selon V2, au préflight et sous verrou ; un échec sous
    verrou n'écrit ni journal ni staging ; disque plein pendant la copie traité
    comme erreur d'écriture, `current` intact.
14. Garde assoupli pour `platform/vault` seulement, ADR 0005, `MoveFileEx` à
    drapeau `0` ; `x/sys/windows` limité par le test d'imports ; retries
    limités aux erreurs 32 et 33.
15. Revue indépendante de `platform/vault` consignée ; pilote B2b exécuté et
    qualifié par le propriétaire, ou absence déclarée, ce qui bloque B2c.

**B2c**

16. Rotation réelle sous Windows avec rollback, y compris un renommage refusé
    par un fichier ouvert ; génération conservée identique à son receipt ;
    chaque ligne du tableau de récupération du §5.5 atteinte par un test, crash
    avant et après M1 à M4 et pendant chaque validation, fenêtre d'absence
    comprise, avec les propriétés du §5.9 ; l'ancien `current` est toujours
    intact.
17. Aucune disparition de fichiers sans `--allow-removals` ou
    `--accept-missing-source` dans les seules conditions de V1 ; source requise
    absente et source retirée du manifeste toujours bloquantes ;
    `current-modified` bloque sans rien déplacer.
18. Revue de sécurité indépendante consignée ; `readonly_test.go` inchangé
    depuis B2b.

**Toutes**

19. 7 checks requis verts sur le SHA exact, avec lien du run.
20. Documentation, guide de récupération, CHANGELOG et ADR cohérents avec le
    code livré ; exemples README vérifiés par test.
21. Aucun accès agent à `D:\Ga`, `D:\Savewarden` ou `E:\GMS` ; aucun chemin
    personnel dans le code ; aucun save, receipt réel ou chemin personnel dans
    le diff.

## 13. Décisions reportées hors de B2

Les points V1 à V4 sont tranchés (§0.4) : aucun point ouvert ne reste dans ce
plan. Les sujets suivants relèvent d'une décision distincte du propriétaire,
dans un lot ultérieur ; B2 les bloque sans les résoudre :

| Sujet | Comportement en B2 |
| --- | --- |
| Résolution d'un `current` modifié hors de Savewarden | `current-modified`, blocage strict, aucun contournement (§4.5) |
| Retrait d'une source du manifeste alors que le receipt précédent la contient | `source-removed-from-manifest`, blocage, aucun flag (§4.4) |
| Qualification de ReFS | Refus « non qualifié » (§4.2) |
| Adoption, migration ou intervention sur un coffre non créé par Savewarden | Refus générique (§4.2, D17) |
| Rétention et nettoyage des générations, stagings et temporaires | Aucune suppression (D7) |
| Extension de la liste des erreurs retentées | Liste fermée 32 et 33 (V4) |

## 14. Risques

| Risque | Effet | Parade |
| --- | --- | --- |
| Écrasement par `os.Rename`, `Root.Rename` ou `MoveFile` entre volumes | Perte de données | Interdits partout, `MoveFileEx` à drapeau `0`, garde AST, test réel |
| Crash entre deux renommages | `current` absent | Fenêtre reconnue, journal write-ahead, identités, récupération, énumération |
| Journal corrompu ou ambigu | Utilisateur bloqué en code 3 | Fail-closed assumé, guide de récupération, aucune action devinée |
| Coupure de courant | Dernière étape perdue | Récupération par observation ; contradiction = fail-closed |
| Rollback impossible | Intervention manuelle | Code 3, deux états intacts nommés, guide sans suppression |
| Antivirus, indexeur ou Explorateur tenant un dossier | Bascule refusée | Retries ≤ 5 s sur erreurs transitoires, échec propre |
| Source modifiée pendant la copie | `current` incohérent | Hachage pendant copie, second inventaire, aucune publication |
| Source facultative disparue ou vidée | Génération appauvrie | Blocage, flags explicites dans les seules conditions de V1, génération précédente conservée |
| Source retirée du manifeste | Coffre bloqué pour ce jeu | Décision distincte hors B2 (§13) ; un nouveau coffre reste possible |
| Récupération relancée ou interrompue | Aggravation d'un état ambigu | Propriétés du §5.9 : monotone, convergente, non aggravante ; tests de répétition |
| Crash pendant l'initialisation d'un coffre neuf | Coffre partiel, inutilisable ou adopté à tort | Protocole I1 à I9, états reconnus ou refusés sans mutation, reprise explicite seulement |
| Enveloppe de récupération épuisée | Opération bloquée en code 3 | Limites E1 à E4 déclarées, testées ; diagnostic ; aucune suppression pour rouvrir l'enveloppe |
| Dossier substitué entre pré-contrôle et renommage | Mauvais objet déplacé | Post-contrôle d'identité, code 3, aucun autre déplacement ; processus hostile hors modèle |
| Espace qui diminue entre préflight et copie | Échec en cours de copie | Second contrôle sous verrou ; disque plein = erreur d'écriture, `current` intact |
| `current` modifié hors de Savewarden | Blocage durable de l'utilisateur | Diagnostic précis, guide, décision ultérieure, nouveau coffre possible |
| Jonction plantée dans le coffre | Écriture hors du coffre | Ancêtres épinglés, ouverture sans suivi, contrôle sur handle |
| Coffre sur ReFS, Dev Drive, FAT ou réseau | Refus pour l'utilisateur | Message explicite ; ReFS qualifiable plus tard par tests |
| Accumulation de générations, stagings, temporaires | Disque plein | Préflight d'espace, documentation, nettoyage manuel guidé |
| Assouplissement du garde trop large | Écriture ailleurs | Un package, API nommées, règle de drapeau, cas négatifs, revue |
| Couplage implicite `sync` → `inspect` | Frontière érodée | Port défini par `sync`, pont dans `app`, ADR 0004 |
| Adoption accidentelle de l'ancien coffre | Réécriture du snapshot pilote | Refus générique des dossiers non vides sans marqueur |
| Chemin personnel codé en dur | Produit non portable, fuite | Interdit au §7, revue du diff |
| Dépendance `x/sys` | Surface supply chain | Version exacte, `go.sum`, govulncheck, limitée à `platform/vault` |
| Mainteneur unique | Revue limitée sur le premier lot qui écrit | Revues indépendantes obligatoires, Human GO, aucune auto-approbation |
| Extension de périmètre vers B3/B4 | Lot trop gros | Non-objectifs §2.2, trois PR |

## 15. Rapport de restitution attendu

Pour chaque PR : branche et PR, SHA final, `git diff --stat`, commandes de
gates avec résultats exacts, lien du run CI et état des 7 checks, ADR créés ou
amendés, résultats des tests de crash et de non-mutation, revues
indépendantes, résultat du pilote ou son absence, écarts au plan, risques
résiduels, questions ouvertes, recommandation GO ou NO-GO **sans
auto-approbation**.

## Historique

| Version | Date | Commit | Changements |
| --- | --- | --- | --- |
| 0.1 | 24 octobre 2026 | `e1932a2`, local, rebasé depuis `eeafa90`, contenu identique | Brouillon initial, décisions D1–D17 proposées |
| 0.2 | 29 octobre 2026 | `d056f3c`, local | Arbitrages du propriétaire : trois PR, NTFS seul, `current` contrôlé strictement, sources absentes protégées ; journal vérifiable et fail-closed ; récupération de première publication dès B2b ; fenêtre d'absence reconnue ; codes et état du coffre distingués ; garanties de durabilité limitées ; permissions agent/produit/propriétaire ; exemples et état Git corrigés ; ADR 0006 érigé en référence de sécurité |
| 0.2, arbitrages V1–V4 | 29 octobre 2026 | `561ed7a`, local | Arbitrages finaux V1–V4 ; niveaux « préparé, validé, réalisé » d'un enregistrement ; points d'arrêt A1 à A8 de la première publication ; propriétés de la récupération répétée ; §13 réduit aux décisions reportées hors B2 |
| 0.2, amendements de revue finale | 29 octobre 2026 | commit local suivant `561ed7a` | Revue intégrale de `561ed7a` : protocole d'initialisation I1 à I9 (P0-1) ; enveloppe de récupération E1 à E4 (P0-2) ; rollback de rotation M1 à M4 en tableau fermé (P0-3) ; observations admises avant publication, état `metadata only`, ordre des handles autour de la bascule, provenance des champs d'un receipt reconstruit, nuance après coupure (P1-1 à P1-5) |
| 0.2, cadrage B2c | 29 octobre 2026 | local, branche `docs/b2c-framing` | Arbitrages A1 à A5 et M3 du propriétaire (§0.8) ; ADR 0006 amendé ; §8.3 et §9.3 amendés ; impacts sur ADR 0004 et 0007 recensés ; pas de GO de développement B2c |
| 0.2, revue du cadrage B2c | 29 octobre 2026 | local, branche `docs/b2c-framing` | Corrections R1 à R5 du propriétaire (§0.8) : M3 sans `current`, génération précédente reconnue par son identité publiée, fenêtre M1–M2 et restaurations vérifiées, fenêtres résiduelles du verrou, codes B2b et `current-exists` historique dans ADR 0007 ; §4.5 et §8.3 ajustés ; matrice du §9.3 nommée test par test ; pas de GO de développement B2c |

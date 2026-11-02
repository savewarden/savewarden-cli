# ADR 0006 — Protocole de publication et de récupération

Date : 29 octobre 2026. Statut : accepté pour B2, contenu normatif du plan B2
v0.2 §5, validé par le propriétaire le 29 octobre 2026 après revue intégrale ;
amendé le même jour avant les premières écritures de B2b, puis pour le
cadrage et les revues de B2c ; amendements de B2c postérieurs au cadrage
ratifiés par le propriétaire le 30 octobre 2026 (voir Amendements). Le §10,
snapshots de B3, est accepté par le propriétaire le 31 octobre 2026, avec le
plan B3 v0.2, et implémenté par B3, fusionné en `08ec0c2`. Le §11, restauration
isolée de B4, et ses renvois depuis les §1, §2, §3 et §8 sont un **projet,
non accepté**, rédigé le 1er novembre 2026 avec le plan B4 v0.2.
**Référence de sécurité** de toute publication et de toute récupération de
Savewarden : une opération qui ne peut pas s'y conformer exige un amendement
revu de cet ADR avant son code.

## Contexte

B2 publie pour la première fois des fichiers dans un coffre : un premier
`current` en B2b, sa rotation vers `retained\` en B2c. Windows ne fournit pas
d'échange atomique de deux répertoires : `MoveFileEx` déplace un nom, refuse
de remplacer un dossier existant et ne promet ni atomicité globale, ni
durabilité d'un renommage de dossier après une coupure de courant. Le
remplacement de `current` doit donc être une **transaction par protocole** :
un journal écrit avant chaque mutation, des identités de fichier observées
avant d'agir, et une récupération qui refuse tout état qu'elle ne sait pas
expliquer. Le journal vivant dans le coffre, la création du coffre lui-même
suit un protocole propre.

Les sections numérotées ci-dessous reprennent le §5 du plan B2 v0.2 ; leurs
renvois `§N` désignent les sections de cet ADR, leurs renvois `plan B2 §N`
celles du plan.

## Décision

### 1. Portée

ADR 0006 s'applique à **toute opération qui rend visible un ensemble de
fichiers sous un nom stable** dans un coffre Savewarden, ou dans une zone de
restauration (§11, projet), et à **toute
récupération** : `current` en B2, snapshots en B3, restauration vers un dossier
isolé en B4 pour son staging et sa publication, dans une zone de restauration
hors du coffre (§11, projet). Une future opération qui ne
peut pas s'y conformer exige un amendement de l'ADR, revu, avant son code.

### 2. Invariants

| # | Invariant |
| --- | --- |
| I1 | Rien n'est écrasé, tronqué ni supprimé : créations exclusives, renommages sans remplacement et sans copie |
| I2 | Publier, c'est renommer un dossier entièrement vérifié, sur le même volume ; jamais copier vers le nom publié |
| I3 | Write-ahead : chaque création de staging et chaque renommage d'un dossier d'opération, `staging\<op>`, `current` ou `retained\`, rollback compris, est précédé d'un enregistrement d'intention validé qui décrit l'état d'avant et d'après par identité de fichier |
| I4 | Le journal n'est jamais cru seul : une récupération n'agit que si le dernier enregistrement valide et l'observation concordent |
| I5 | Fail-closed : toute ambiguïté arrête tout, sans déplacement ni écriture, code 3, diagnostic |
| I6 | Rollback par défaut avant publication achevée ; finalisation seulement après revérification intégrale du nouvel état contre le plan |
| I7 | Une génération publiée ou conservée, un receipt, un enregistrement ne sont jamais rouverts en écriture |
| I8 | Une seule opération par jeu à la fois ; une opération non terminale bloque toute nouvelle `--apply` ; précisé par domaine pour les snapshots au §10 (B3), et par zone pour les restaurations, qui n'écrivent pas le coffre, au §11 (B4, projet) |
| I9 | La publication est **transactionnelle par protocole, pas atomique** : la fenêtre où `current` est absent est reconnue et récupérable |
| I10 | Seules les garanties documentées de Windows sont revendiquées ; le reste est une hypothèse couverte par une récupération fondée sur l'observation |
| I11 | L'initialisation du coffre, qui précède tout journal, n'est faite que de créations exclusives et d'un marqueur validé par renommage ; chaque état partiel est reconnu par le tableau du §4 ou refusé sans mutation, jamais adopté implicitement |
| I12 | La récupération est garantie **dans une enveloppe bornée et déclarée** (§9) ; au-delà, fail-closed |

### 3. Journal vérifiable

Emplacement : `games\<game-id>\journal\<op-id>\`, un dossier par opération.

**Format d'un enregistrement** `NNN-<étape>.rec`. L'automate étant acyclique,
une opération compte au plus 11 enregistrements validés (enveloppe E2,
§9) : `NNN` va de `001` à `011`, et un numéro supérieur est une ambiguïté.

```text
{"at":"2026-11-05T10:15:04Z","fields":{…},"op":"20261105T101500-1a2b3c4d","prev":"<sha256 de l'enregistrement précédent>","schema":"savewarden.journal","schema_version":1,"seq":5,"step":"publish-intent"}
sha256 <SHA-256 hexadécimal des octets de la première ligne, LF compris>
```

- Première ligne : JSON canonique, clés triées, UTF-8 sans BOM, une seule ligne.
- `prev` : SHA-256 du fichier de l'enregistrement précédent ; `null` pour `001`.
  La chaîne fixe l'ordre et interdit toute substitution.
- Champs propres à l'étape, **sous la clé `fields`**, objet JSON présent même
  vide ; les propriétés communes ci-dessus restent à la racine. Ce sont les
  identités volume + index de fichier attendues avant et après, chemins
  relatifs, empreinte du plan ; `001-planned` contient le plan complet et la
  version de l'outil. Le contrat de chaque étape fixe ses champs ; ils sont
  validés strictement à la lecture : champs obligatoires, types, valeurs,
  identités et cohérence avec l'observation. Un champ inconnu ou manquant est
  une ambiguïté, jamais ignoré.

**Contrat des champs**, tous obligatoires, aucun autre accepté. Une identité
s'écrit `vvvvvvvv-iiiiiiiiiiiiiiii`, numéro de série du volume puis index de
fichier en hexadécimal minuscule ; une empreinte, 64 chiffres hexadécimaux
minuscules ; un texte, une ligne de 1 à 1 024 octets.

| Étape | Champs | Cohérence vérifiée à la lecture |
| --- | --- | --- |
| `planned` | `previous` : `null` ou identifiant d'opération ; `plan` : objet strict, version de l'outil, jeu, `started_at`, `changes` et `sources` au format du receipt v1 ; `plan_sha256` ; pour une rotation seulement, `current_identity` : l'ancien `current`, relevé sous verrou (§5) | `plan_sha256` est le SHA-256 de `plan` ; `previous` égal à celui du plan ; `current_identity` **obligatoire si `previous` n'est pas `null`, interdit sinon** |
| `staging-intent` | `staging` : `staging/<op>` | Nom fixé par l'opération |
| `staged` | `staging_identity` ; `copied_sha256` | `copied_sha256` est l'empreinte du contenu du plan |
| `verified` | `verified_sha256` | Idem |
| `publish-intent`, `promote-intent` | `staging_identity` | Égale à celle de `staged` |
| `unpublish-intent`, `unpromote-intent` | `current_identity` | Égale à celle de `staged`, puisque c'est le staging qui a été publié ou promu |
| `retire-intent` | `current_identity` : l'ancien `current` | Égale à `current_identity` de `planned`, jamais remplacée par une identité nouvellement observée |
| `restore-intent` | `retained_identity` | Égale à `current_identity` de `planned`, donc à celle de `retire-intent` |
| `rollback-failed` | `error` | — |
| `published` | `receipt_sha256` : SHA-256 du receipt validé | Comparée au receipt observé |
| `failed`, `abandoned` | `reason` | — |
| `rolled-back` | aucun | — |

L'empreinte du contenu est le SHA-256 de la forme JSON canonique des
`sources` du plan : identifiants, chemins, tailles, dates et SHA-256 de
chaque fichier, dossiers. Le receipt reprend `plan_sha256` et les `sources`
du plan.

**Compatibilité.** Un `planned` de première publication, `previous: null`,
n'a pas de `current_identity` : les journaux écrits par B2b restent lisibles
sans changement. Un `planned` de rotation sans `current_identity`, ou une
première publication qui en porte un, est une ambiguïté ; B2b n'en a jamais
écrit.

**Validation d'un enregistrement, dans cet ordre :**

1. créer `NNN-<étape>.rec.tmp-<k>` en `CREATE_NEW`, avec le plus petit `k`
   de 1 à 9 qu'aucun temporaire du numéro `NNN` ne porte, **quelle que soit
   son étape** (enveloppe E1, §9), pour qu'un temporaire laissé par un crash
   n'empêche pas une nouvelle tentative ; un changement d'étape n'ouvre aucun
   budget supplémentaire ; si les neuf `k` sont pris, aucun essai : code 3,
   `recovery envelope exhausted`, aucune mutation ;
2. écrire les deux lignes en entier ;
3. `FlushFileBuffers`, puis fermer ;
4. renommer vers `NNN-<étape>.rec` par `MoveFileEx` sans drapeau, avec les
   retries du §7 ;
5. l'enregistrement n'est **validé** que si le nom final existe, que sa somme
   de contrôle et sa chaîne sont correctes.

Aucune action protégée par un enregistrement ne commence avant sa validation.
Un temporaire n'est jamais supprimé, jamais réutilisé, jamais lu comme
enregistrement. Receipts et marqueur suivent le même procédé, chacun avec
ses neuf `k` pour sa destination logique : chaque `receipts\<op>.json` d'une
part, `savewarden-vault.json` de l'autre. Un temporaire qui subsiste à côté
de son fichier validé est un essai abandonné, ignoré.

**Ce que prouve un enregistrement.** Trois niveaux, jamais confondus :

| Niveau | Ce qui existe | Ce que cela indique | Ce que cela ne prouve pas |
| --- | --- | --- | --- |
| Préparé | Temporaire `*.rec.tmp-<k>` seulement | Une intention a été rédigée ; **en exécution normale**, son action n'a pas commencé. C'est un invariant de protocole, pas une certitude après une coupure de courant (§6) | Rien sur le coffre : l'observation doit montrer l'état d'avant ; toute observation contraire est une contradiction, code 3, sans récupération conjecturale |
| Validé | Enregistrement final correct | L'action est **autorisée** et a pu être effectuée, en tout ou partie, ou pas du tout | Que l'action a eu lieu ; seule l'observation des identités le dit |
| Réalisé | Observation conforme à l'état d'après de l'intention validée, ou enregistrement terminal **et** observation conforme | L'action a eu lieu | — |

Un journal incomplet n'autorise jamais à conclure. L'absence d'un
enregistrement ne prouve pas qu'une action n'a pas eu lieu, et sa présence ne
prouve pas qu'elle a eu lieu. Après une coupure, le journal peut être en
retard comme en avance sur le coffre : un journal absent ou incomplet n'est
pas la preuve qu'aucune mutation n'a eu lieu. Seule une observation prévue par
les tableaux des §4 et §5 permet de décider ; toute autre est une
ambiguïté.

**Lecture du journal d'une opération** : enregistrements finaux de numéros
consécutifs depuis `001`, sommes et chaîne correctes, `op` égal au dossier,
étape connue du schéma, transition permise par l'automate de §4 ou §5.
Un ou plusieurs temporaires du numéro **suivant le dernier validé** signalent
une intention préparée : l'observation doit montrer l'état d'avant cette
intention, sinon ambiguïté.

**Temporaires abandonnés.** Un essai interrompu laisse son temporaire ; un
essai suivant peut ensuite valider le même numéro, sous la même étape ou
sous une autre : `005-publish-intent.rec.tmp-1` reste ainsi à côté de
`005-abandoned.rec` après une reprise A1, `006-published.rec.tmp-1` à côté de
`006-published.rec` après une reprise A6. Comme rien n'est supprimé, cet état
est produit par le protocole et doit rester lisible. Pour un journal de `c`
enregistrements validés, un temporaire `NNN-<étape>.rec.tmp-<k>` est accepté
si et seulement si :

1. son nom suit la grammaire : `NNN` sur trois chiffres, étape en minuscules,
   `k` de 1 à 9 ; tout autre nom est une entrée inattendue ;
2. aucun autre temporaire du même numéro ne porte le même `k`, quelle que
   soit son étape ;
3. et, selon son numéro :
   - `NNN` ≤ `c`, **temporaire abandonné** : son étape était admise par
     l'automate après les enregistrements `001` à `NNN-1`, dans le mode de
     l'opération, première publication ou rotation, et le numéro `NNN` compte
     au plus huit temporaires, puisque son enregistrement provient du
     renommage de l'un des neuf noms. Il est accepté même après un état
     terminal ;
   - `NNN` = `c+1`, **intention préparée** : l'opération n'est pas terminale
     et son étape est une suite admise du dernier enregistrement ;
   - tout autre numéro, dont `000` : ambiguïté.

Le contenu d'un temporaire n'est jamais lu, qu'il soit vide, tronqué ou
complet. Un temporaire abandonné ne contraint pas l'observation et ne compte
pas dans l'enveloppe : son numéro ne sera plus jamais écrit. Seuls les
temporaires du numéro suivant comptent pour E1. Les `k` ne sont pas exigés
contigus : une absence ne prouve rien, et aucune décision ne repose sur elle.
Le lecteur n'exige pas non plus que l'étape abandonnée soit l'une de celles
que la reprise aurait pu remplacer à ce numéro : cette combinaison ne change
aucune décision, qui ne repose que sur les enregistrements validés,
l'observation et les temporaires du numéro suivant.

**Ambiguïtés, toutes fail-closed, code 3, rien déplacé, rien écrit :**

| Cas | Pourquoi |
| --- | --- |
| Enregistrement final illisible, somme fausse ou JSON tronqué | Impossible par le protocole sauf corruption : jamais ignoré |
| Numéro manquant ou dupliqué, chaîne rompue, numéro supérieur à `011` | Ordre ou contenu non prouvé, enveloppe E2 dépassée |
| Étape inconnue, schéma inconnu, transition interdite | Automate non reconnu |
| Temporaire d'un numéro supérieur au suivant ou `000` ; temporaire d'une étape que l'automate n'admettait pas à son numéro ; deux temporaires d'un même numéro portant le même `k` ; neuf temporaires à côté d'un enregistrement validé ; temporaire du numéro suivant après un état terminal | Historique incohérent : le protocole ne produit pas cet état |
| Neuf temporaires du numéro suivant quand la reprise doit l'écrire | Enveloppe E1 épuisée |
| Second `rollback-failed` pour la même intention | Enveloppe E3 dépassée |
| Enregistrement d'une autre opération dans le dossier | Mélange d'opérations |
| Deux opérations non terminales pour un jeu ; par domaine selon le §10 (B3) ; par zone selon le §11 (B4, projet) | Exclusivité violée |
| Enregistrement après un état terminal | Opération close modifiée |
| Observation contraire au dernier enregistrement, dans un sens ou dans l'autre : `published` sans `current` de l'identité attendue, ou `current` publié alors que `publish-intent` manque | Durabilité ou intervention extérieure non prouvées |
| Identité de fichier indisponible, dossier du journal illisible | Observation impossible |
| `staging\<op>`, `current` ou `retained\<op>` qu'aucun journal n'explique, par nom ou par identité | Objet d'origine inconnue |
| Temporaire du numéro suivant alors que l'observation montre l'état d'après | Action réalisée sans intention validée : protocole violé ou ordre perdu par une coupure |

Le diagnostic nomme chaque fichier en cause et renvoie au guide de
récupération. Hors des cas prévus par les tableaux de récupération des §4
et §5, Savewarden ne tente aucune réparation.

### 4. Initialisation du coffre et première publication (B2b)

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
| I8 | `CreateDirectory(journal)` s'il manque, puis `CreateDirectory(journal\<op>)`, exclusif | Dossier d'opération vide, ou ne contenant que des temporaires `001-…` | « Opération non commencée » : bloque `--apply` (I8) ; `--recover --apply` valide `001-abandoned` aux seules conditions de la règle M3 ci-dessous |
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

**Génération précédente reconnue.** `current` est reconnu comme la
génération publiée par l'opération `<prev-op>` seulement si toute la chaîne
de preuve tient, observation comprise :

1. le receipt `<prev-op>` est le receipt de tête du jeu, valide et du même
   jeu ;
2. le journal `<prev-op>` est terminal en `published`, et son
   `receipt_sha256` est le SHA-256 du fichier receipt ;
3. l'identité observée de `current`, lue sans suivi de lien, est l'identité
   que ce journal a publiée : `staging_identity` de `staged`, reprise par
   `publish-intent` pour une première publication ou par `promote-intent`
   pour une rotation ;
4. le contenu de `current` est intégralement conforme au receipt, relu comme
   au §4.5 du plan B2.

Un dossier présent dont l'une de ces preuves manque est `unidentified` ou
`other`, jamais `old`. La règle vaut pour une génération publiée par B2b
comme par une rotation de B2c, et pour un receipt reconstruit par la
récupération, dont le journal porte les mêmes identités.

**Coffre copié ou déplacé.** Une copie ou un déplacement manuel du coffre
peut changer les identités NTFS sans altérer les données. B2 ne prend pas en
charge ce cas : la génération n'est plus reconnue, code 3, rien écrit. Le
diagnostic constate un écart d'identité, sans prétendre à une corruption ni
à une compromission ; aucune identité n'est adoptée ni réécrite
automatiquement. La portabilité d'un coffre relève d'une procédure future,
explicite, de requalification ou d'import contrôlé, qui ne pourra pas
affaiblir la récupération fail-closed.

**Opération non commencée (règle M3).** Un dossier d'opération sans aucun
enregistrement validé, vide ou ne contenant que des temporaires `001-…`, n'a
fixé ni plan ni identité. Une **génération publiée est attendue** dès que le
jeu a un receipt, valide ou non, ou un journal terminal en `published`.
`staging\<op>` doit être absent, et tout `retained\<…>` présent expliqué
par son receipt, comme le §3 l'exige déjà. Alors :

| Génération publiée attendue | `current` | Action |
| --- | --- | --- |
| Non | Absent | `001-abandoned` |
| Oui | Présent et génération précédente reconnue | `001-abandoned` |
| Oui | Absent | Contradiction : code 3, rien écrit |
| Non | Présent | Code 3, rien écrit |
| Oui | Présent mais non reconnu : identité, contenu, receipt ou journal en défaut | Code 3, rien écrit |

Aucune réparation n'est tentée. Ce cas, antérieur à `001-planned`, est
distinct de celui d'une rotation dont `planned` est validé, où
`current_identity` est la référence (§5).

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
`old` celle de l'ancien `current`, relevée sous verrou et fixée par
`current_identity` de `planned` (§5) ; `—` un nom absent. Toute autre
identité sous l'un de ces noms, notée `other`, est une ambiguïté.

Récupération `--recover --apply` d'une première publication :

| Dernier validé | Observation (`C`, `S`) | Action |
| --- | --- | --- |
| Aucun : dossier d'opération vide ou temporaires `001` seuls | Règle M3 : aucune génération publiée attendue et `current` absent, ou génération précédente reconnue ; `staging\<op>` absent | `001-abandoned` ; sinon code 3, rien écrit |
| `planned` | (—, —) | `abandoned` |
| `staging-intent` | (—, —), ou (—, dossier ordinaire sans point d'analyse) | `abandoned` ; staging laissé en place, ni lu ni déplacé |
| `staged`, `verified` | (—, `new`) | `abandoned` ; staging laissé en place |
| `publish-intent` | (—, `new`) | Renommage non effectué : `rolled-back` |
| `publish-intent` | (`new`, —) | Renommage effectué : revérification intégrale de `current` contre le plan ; égal : receipt (plan B2 §4.7) puis `published` ; différent : `unpublish-intent`, puis `current` → `staging\<op>` |
| `unpublish-intent` | (`new`, —) | Déplacement non effectué : l'exécuter, puis `rolled-back` ; s'il échoue : `rollback-failed`, code 3 |
| `unpublish-intent` ou `rollback-failed` | (—, `new`) | Déplacement effectué : `rolled-back` |
| `rollback-failed` | (`new`, —) | Nouvel essai ; succès : `rolled-back` ; échec : code 3, sans nouvel enregistrement (E3) |
| `published` | (`new`, —), receipt conforme | Rien |
| Tout autre cas, dont un `staging\<op>` présent avant `staging-intent`, un `S` absent après `staged`, ou `C` et `S` tous deux présents | — | Fail-closed, code 3 |

En B2b, un `current` existant et différent du plan est refusé avant tout
staging (`current-exists`, code 2). En B2c, seule la génération précédente
reconnue est remplacée, par la rotation du §5 ; tout autre `current` reste
refusé avant tout staging (§5, point 1).

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
   tout code de bascule (plan B2 §9.2).
3. **Pré-contrôle** : ouvrir le dossier source sans suivi
   (`FILE_FLAG_OPEN_REPARSE_POINT`, `FILE_FLAG_BACKUP_SEMANTICS`), avec partage
   lecture, écriture et suppression ; identité égale à celle de l'intention ;
   aucun point d'analyse ; nom cible absent ; refermer le handle aussitôt.
4. `MoveFileEx(source, cible, 0)`, avec les retries du §7.
5. **Post-contrôle** : ouvrir la cible sans suivi ; identité égale à celle de
   l'intention ; nom source absent.
6. **Course** : `MoveFileEx` opère sur des noms ; un remplacement du dossier
   source entre 3 et 4 n'est pas empêché. Le post-contrôle le détecte :
   identité inattendue, aucun autre déplacement, l'objet inconnu n'est ni lu
   ni déplacé, code 3. Cette fenêtre relève du processus hostile, hors modèle
   (plan B2 §6.1). Le renommage par handle, `FileRenameInfo`, la supprimerait, mais il
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
| A3 | Pendant ou juste après le renommage, retour reçu ou non | `publish-intent` validé | (`new`, —) ; aucun receipt | Revérification intégrale de `current` contre le plan ; égal : receipt reconstruit (plan B2 §4.7) puis `published` ; différent : `unpublish-intent`, `current` → `staging\<op>`, `rolled-back` |
| A4 | Après le post-contrôle, pendant l'écriture du receipt | Idem | Idem, plus `receipts\<op>.json.tmp-<k>` | Temporaires ignorés, ni lus comme receipt, ni réutilisés, ni supprimés ; nouveau temporaire au `k` libre suivant ; puis comme A3 |
| A5 | Après validation du receipt, avant `published` | Idem | Receipt final présent | Receipt relu : schéma, opération et entrées égaux au plan, `current` revérifié : `published` ; sinon fail-closed |
| A6 | Pendant l'écriture de `published` | Idem, plus temporaire `006-…tmp-<k>` | Idem | Comme A5, avec un nouveau temporaire |
| A7 | Après `published` | Terminal | Conforme | Rien |
| A8 | État non prévu, par exemple `current` d'une autre identité, `C` et `S` tous deux présents, `published` sans `current` conforme, `current` publié sans `publish-intent` | — | — | Fail-closed, code 3, rien déplacé ni écrit |

Pendant A3 à A6, le mode plan annonce « operation `<op>` pending recovery:
current published, receipt missing » : il ne confond jamais cet état avec un
`current` modifié hors de Savewarden (plan B2 §4.5).

### 5. Rotation de `current` (B2c)

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
| 1–4 | `planned`, `staging-intent`, `staged`, `verified` | Comme §4 |
| 5 | `retire-intent` | M1, bascule selon l'ordre du §4 |
| 6 | `promote-intent` | M2 ; post-contrôle ; receipt |
| 7 | `published` | Terminal |

Rollback : M4 puis M3 si le nouveau `current` doit être retiré, M3 seul si la
promotion n'a pas eu lieu. `rollback-failed` : au plus un par intention (E3),
non terminal.

**Identité persistante de l'ancien `current` (A1, A2).** L'identité de
l'ancien `current` est relevée une fois, sous verrou, et ne change plus :

1. à l'étape I7, sous verrou et avant `001-planned`, `current` doit être
   la génération précédente reconnue (§4) : son identité est celle que le
   journal `<prev-op>` a publiée et son contenu est relu intégralement contre
   le receipt ; un écart de contenu sous cette identité donne
   `current-modified`, code 2, sans journal ; une autre identité, une
   identité indisponible ou un point d'analyse, code 3. Le mode plan
   applique la même reconnaissance : un `current` dont l'identité n'est pas
   celle que le journal `<prev-op>` a publiée est signalé
   `unexpected-entry`, code 3, quel que soit son contenu, jamais comparé
   comme s'il était la génération précédente ; un receipt absent ou
   invalide garde `current-modified`, code 2, comme en B2b ;
2. `planned` fixe cette identité dans `current_identity` : c'est la
   référence `old` des observations, de `retire-intent` et de
   `restore-intent` pour toute la suite de l'opération et de sa
   récupération ;
3. après `verified` et immédiatement avant `retire-intent`, `current` est
   relu intégralement contre son receipt et son identité comparée à
   `current_identity` : contenu différent sous la même identité, `failed`
   avec `current-modified`, code 2, rien déplacé ; identité différente ou
   indisponible, objet qu'aucun enregistrement n'explique, code 3, rien
   enregistré ni déplacé ;
4. le pré-contrôle de M1 suit l'ordre du §4 : identité égale à
   `current_identity`, `retained\<prev-op>` absent, portées de lecture
   fermées ;
5. après le post-contrôle de M1 et avant `promote-intent`, l'observation
   doit être exactement (—, `old`, `new`) : `current` absent, staging
   d'identité `new`, et `retained\<prev-op>` relu intégralement contre le
   receipt `<prev-op>`, identité égale à `current_identity`. Tout écart,
   dont un `current` réapparu ou un `retained` modifié, est une
   contradiction : code 3, **rien n'est enregistré ni déplacé**, ni
   promotion, ni restauration d'une génération altérée. Un objet apparu
   sous `current` n'a pas de provenance démontrée : il n'est ni lu comme une
   génération, ni déplacé, ni écrasé, et l'ancienne génération reste dans
   `retained\<prev-op>`, momentanément indisponible sous son nom courant ;
   le diagnostic nomme l'ambiguïté. Le journal reste à `retire-intent`. Sa
   récupération ne déduit rien de cet écart : elle classe l'observation
   qu'elle trouve, selon le tableau ci-dessous :
   - objet inattendu sous `current`, (`other`, `old`, `new`), ou
     `retained\<prev-op>` d'une autre identité, (—, `other`, `new`) : aucune
     ligne ne s'applique ; fail-closed, code 3, rien écrit, enregistré ni
     déplacé tant que cet objet reste présent ; `retained\<prev-op>` et
     `staging\<op>` restent intacts ;
   - `retained\<prev-op>` de même identité au contenu altéré, (—, `old`,
     `new`) : la ligne prévue exige une restauration vérifiée, qui échoue :
     code 3, rien écrit ;
   - une récupération n'est possible que lorsqu'une observation ultérieure
     correspond entièrement à une ligne du tableau, toutes ses vérifications
     comprises.

**Fenêtres résiduelles.** `sync.lock` n'exclut qu'une autre opération
Savewarden : il n'empêche pas un autre processus d'écrire dans `current` ou
`retained\<prev-op>`. Les relectures des points 3 et 5 bornent ces fenêtres
sans les fermer : une écriture entre la relecture du point 3 et M1, entre
celle du point 5 et M2, ou entre la relecture d'une restauration vérifiée et
M3, n'est pas détectée ; de même dans `staging\<op>` entre sa relecture
(`verified`) et M2, une fenêtre qu'une rotation allonge des relectures de
`current` et de `retained`. Le plan suivant relit la génération publiée
contre son receipt et signale tout écart. Chaque relecture relit l'identité
après le contenu : un dossier remplacé pendant la lecture est une ambiguïté,
code 3, jamais pris pour un changement de contenu. Jeu, launcher et tout outil qui
touche le coffre doivent être fermés, comme pour la source.

**Restaurations vérifiées.** Toute restauration M3, en cours d'exécution
comme en récupération, est précédée de la relecture intégrale de
`retained\<prev-op>` contre son receipt, identité égale à
`current_identity` : conforme, M3 a lieu ; non conforme, code 3, rien
déplacé ni enregistré. Une génération altérée n'est jamais remise en place
comme si elle était intacte.

En cours d'exécution :

- M1 échoue : l'observation confirme (`old`, —, `new`) : `rolled-back`, code 1,
  `vault: rolled back <op>; current unchanged` ;
- relecture du point 5 en écart : code 3, `vault: intervention required for
  <op>: …`, diagnostic nommant le nom en écart ; aucun enregistrement, aucun
  déplacement ;
- M2 échoue : nouvelle observation avant toute restauration ; (—, `old`,
  `new`) confirmé : restauration vérifiée, puis `restore-intent`, M3,
  post-contrôle, `rolled-back`, code 1 ; si M3 échoue : `rollback-failed`,
  code 3, diagnostic nommant les deux emplacements, tous deux intacts ;
  `retained` non conforme, `current` réapparu ou toute autre observation :
  code 3, rien d'autre enregistré ni déplacé ;
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
| Aucun : dossier d'opération vide ou temporaires `001` seuls | Règle M3 du §4 | `001-abandoned` ; sinon code 3, rien écrit |
| `planned` à `verified` | (`old`, —, `S` selon les règles du §4), `old` étant `current_identity` de `planned` | `abandoned` |
| `retire-intent` | (`old`, —, `new`) | M1 non effectué : `rolled-back` |
| `retire-intent` | (—, `old`, `new`) | M1 effectué : restauration vérifiée ; `retained` conforme : `restore-intent`, puis M3, puis `rolled-back` ; non conforme : code 3, rien écrit |
| `promote-intent` | (—, `old`, `new`) | M2 non effectué : restauration vérifiée ; conforme : `restore-intent`, puis M3, puis `rolled-back` ; non conforme : code 3, rien écrit |
| `promote-intent` | (`new`, `old`, —) | M2 effectué : revérification intégrale ; égal : receipt, `published` ; différent : `unpromote-intent`, puis M4, sauf si le receipt de `<op>` est déjà validé : code 3, rien écrit, comme en A5 et A6 |
| `unpromote-intent` | (`new`, `old`, —) | M4 non effectué : M4, puis restauration vérifiée ; conforme : `restore-intent`, M3, `rolled-back` ; non conforme : code 3, rien d'autre écrit |
| `unpromote-intent` | (—, `old`, `new`) | M4 effectué : restauration vérifiée ; conforme : `restore-intent`, puis M3, puis `rolled-back` ; non conforme : code 3, rien écrit |
| `restore-intent` | (—, `old`, `new`) | M3 non effectué : restauration vérifiée ; conforme : M3, puis `rolled-back` ; non conforme : code 3, rien écrit |
| `restore-intent` | (`old`, —, `new`) | M3 effectué : `rolled-back` |
| `rollback-failed` | L'une des observations prévues pour l'intention qui le précède | Comme la ligne de cette intention ; nouvel échec du déplacement : code 3, sans nouvel enregistrement (E3) |
| `retire-intent`, `promote-intent`, `unpromote-intent`, `restore-intent` ou `rollback-failed` | (`other`, `old`, `new`) : objet inattendu sous `current` | Aucune ligne ne s'applique : fail-closed, code 3, rien écrit, enregistré ni déplacé ; objet, `retained\<prev-op>` et `staging\<op>` laissés en place, à chaque exécution, tant que l'objet est présent |
| `published` | (`new`, `old`, —), receipt conforme | Rien |
| Tout autre cas | — | Fail-closed, code 3 |

Chaque ligne est atteinte par un test : crash avant et après chacun des
déplacements M1 à M4, et pendant la validation de chaque enregistrement, puis
récupération répétée (plan B2 §9.3).

### 6. Durabilité : ce qui est garanti, ce qui ne l'est pas

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

### 7. Retries bornés

- Uniquement sur les renommages de dossiers et de validation, et
  **exclusivement** sur `ERROR_SHARING_VIOLATION` (32) et
  `ERROR_LOCK_VIOLATION` (33) (V4).
- `ERROR_ACCESS_DENIED` (5) reste un **échec immédiat**, même si un antivirus
  ou l'Explorateur en est la cause probable : le diagnostic invite à fermer
  les programmes qui utilisent le coffre. Toute autre erreur : échec immédiat.
- La liste 32 et 33 est fermée pour B2 ; l'étendre exigerait une décision du
  propriétaire et un amendement d'ADR 0006.
- Codes observés sur Windows 11 et NTFS par les tests réels de
  `platform/vault` : un dossier tenu ouvert sans `FILE_SHARE_DELETE`, comme
  par une portée de lecture, refuse son renommage en 32, retenté ; un fichier
  ouvert à l'intérieur du dossier le refuse en 5, jamais retenté : la
  bascule échoue aussitôt et le diagnostic invite à fermer les programmes
  qui utilisent le coffre. Une cible existante, fichier ou dossier, est
  refusée en 183 ; `CREATE_NEW` sur un nom de dossier existant, en 5.
- Backoff exponentiel par horloge injectée, nombre de tentatives borné,
  **5 secondes au plus** au total par renommage ; jamais de boucle infinie.
- Pendant la section critique, de `publish-intent` ou `retire-intent` jusqu'à
  l'état terminal, l'annulation est différée jusqu'à la fin de la bascule ou
  du rollback.

### 8. Ce que publication et récupération ne font jamais

Supprimer ; remplacer ; copier vers un nom publié ; déplacer entre volumes ;
suivre un lien ; déplacer ou lire un objet dont l'identité n'est pas celle
attendue ; agir sur un journal ambigu ; écrire dans un dossier non marqué,
hormis l'initialisation I2 à I4 d'un dossier absent, vide ou ne contenant que
des temporaires de marqueur, et celle, Z2 à Z4, d'une zone de restauration
(§11, projet) ; supprimer un temporaire pour rouvrir
l'enveloppe ; toucher une génération `retained\` autre que celle retirée par
la même opération, lors de son propre rollback.

### 9. Propriétés de la récupération

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
| E1 | Au plus 9 temporaires par destination logique validée par renommage : par numéro d'enregistrement, toutes étapes confondues ; par receipt ; pour le marqueur | Une relance n'en consomme un que si elle est interrompue pendant l'écriture de ce fichier ; aucun temporaire n'est supprimé ni réutilisé ; changer d'étape à un numéro n'ouvre pas de nouveau budget |
| E2 | Au plus 11 enregistrements validés par opération | L'automate est acyclique ; son plus long chemin est la rotation dépromue puis restaurée, avec un `rollback-failed` par intention : `001` à `011` |
| E3 | Au plus un `rollback-failed` par intention de rollback | Un nouvel échec du même déplacement n'ajoute rien au journal et sort en code 3 |
| E4 | Retries de renommage sur 32 et 33 seulement, 5 secondes au plus | §7 |

Au-delà de l'enveloppe, Savewarden ne promet aucune convergence. L'état reste
tel quel, décrit par le diagnostic ; le guide du coffre explique comment
l'examiner, sans commande de suppression. Savewarden ne supprime jamais un
temporaire pour rouvrir l'enveloppe : toute intervention sur le coffre
relève de son propriétaire.

### 10. Snapshots (B3)

**Statut : accepté** par le propriétaire le 31 octobre 2026, avec le plan B3
v0.2, rédigé le 30 octobre 2026. Normatif pour tout code de snapshot ;
implémenté par B3, fusionné en `08ec0c2` (PR #11) et qualifié par le pilote
du 1er novembre 2026. Il ne change aucune règle des §1 à §9 pour le domaine
de `current`.

Un snapshot est la copie vérifiée d'une génération `current` publiée, rangée
sous `games\<game-id>\snapshots\`, approuvée à sa publication, puis jamais
rouverte en écriture, déplacée, remplacée ni supprimée. Les invariants I1 à
I12 s'y appliquent avec les précisions de ce paragraphe, qui prévalent pour le
domaine des snapshots. Le contrat de la commande et le receipt de snapshot
sont fixés par ADR 0008.

#### Domaines et disposition

```text
games\<game-id>\
  sync.lock                                             verrou commun aux deux domaines
  current\, retained\, staging\, receipts\, journal\    domaine de current (§4, §5)
  snapshots\                                            domaine des snapshots
    <snap-id>\<source-id>\...              P : snapshot publié
    <snap-id>.incomplete\<source-id>\...   I : copie en cours, échouée ou abandonnée
    receipts\<snap-id>.json                receipt de snapshot (ADR 0008)
    journal\<snap-id>\NNN-<étape>.rec      journal de l'opération de snapshot
```

- `<snap-id>` est l'identifiant de l'opération, de la forme du §3. Il est
  distinct de tout identifiant déjà employé dans l'un ou l'autre domaine ; le
  contrôle a lieu avant tout journal.
- **Grammaire** du domaine, toute autre entrée étant une ambiguïté du domaine
  des snapshots, code 3 :
  - `snapshots\` : `receipts`, `journal`, et des dossiers ordinaires
    `<snap-id>` ou `<snap-id>.incomplete` ;
  - `snapshots\receipts\` : des fichiers `<snap-id>.json` et leurs temporaires
    `<snap-id>.json.tmp-<k>`, `k` de 1 à 9 (§3, E1) ;
  - `snapshots\journal\` : des dossiers `<snap-id>`, dont le contenu suit le
    §3 ;
  - un lien, un fichier là où un dossier est attendu, ou un point d'analyse,
    partout.
- **Explication**, par nom ou par identité (§3) : `P`, `I` et chaque receipt
  de snapshot portent le nom d'une opération de snapshot journalisée. Leur
  identité est exigée dès que `staged` l'a fixée. Un `I` dont le journal
  s'arrête avant `staged` est accepté s'il est un dossier ordinaire sans point
  d'analyse ; il n'est jamais lu ni déplacé. Un `I` d'une opération close
  n'est ni relu ni exigé : son absence n'est ni signalée ni bloquante.
- **Structure.** `snapshots`, `snapshots\receipts` et `snapshots\journal` sont
  créés sous verrou, après le second contrôle d'espace et avant tout journal,
  par création exclusive, dans cet ordre et seulement s'ils manquent ; suit
  `snapshots\journal\<snap-id>`. Comme pour I11, une structure faite de
  dossiers de structure vides est reconnue et complétée par la création
  suivante ; un dossier d'opération vide, ou ne contenant que des temporaires
  `001-…`, est une opération non commencée ; tout autre état partiel est une
  ambiguïté.
- **Verrou.** `sync.lock` sert aux deux domaines. Une opération de snapshot le
  prend par la même primitive que `sync`, qui le crée vide s'il manque ; elle
  ne l'écrit, ne le tronque ni ne le supprime jamais. Ainsi, l'absence du
  verrou ne rend jamais la récupération d'un snapshot dépendante du domaine
  de `current`.

#### I8 par domaine

Précise I8. Le domaine de `current` compte les opérations de `journal\` ;
celui des snapshots, celles de `snapshots\journal\`. Le verrou garantit
qu'une seule opération s'exécute à la fois par jeu, quel que soit son
domaine.

- Deux opérations non terminales d'un même domaine : ambiguïté, code 3.
- Une opération non terminale du domaine de `current` bloque toute nouvelle
  création dans les deux domaines.
- Une opération de snapshot non terminale ne bloque que la création d'un
  snapshot.
- Une opération non terminale dans chaque domaine est un état admis ; chaque
  domaine se récupère seul.

Aucune observation faite **sous** `snapshots\` n'entre dans les décisions du
domaine de `current` : explication de `staging\`, règle M3, receipt de tête,
opérations en attente. `sync` ne lit rien sous `snapshots\`. L'entrée
`snapshots` elle-même reste soumise à la lecture de `games\<game-id>` par
`sync` : un fichier ou un lien sous ce nom y est une entrée imprévue.

#### Automate et contrat des champs

Même format d'enregistrement qu'au §3 et mêmes noms d'étapes que la première
publication. Le lecteur applique cet automate au seul dossier
`snapshots\journal\<snap-id>`. Tous les champs sont obligatoires, aucun autre
n'est accepté.

| # | Enregistrement validé | Champs | Cohérence vérifiée à la lecture | Action qui suit |
| --- | --- | --- | --- | --- |
| 1 | `planned` | `source_operation` ; `source_receipt_sha256` ; `source_identity`, identité de `current` relevée sous verrou ; `plan` : objet strict, version de l'outil, jeu, `started_at`, `sources` au format du receipt v1 ; `plan_sha256` | `plan_sha256` est le SHA-256 de `plan` ; `source_operation` au format du §3 et différent de l'opération ; jeu du plan égal au `<game-id>` du dossier | — |
| 2 | `staging-intent` | `staging` : `snapshots/<snap-id>.incomplete` | Nom fixé par l'opération | Créer `I`, copier, flusher |
| 3 | `staged` | `staging_identity` ; `copied_sha256` | `copied_sha256` est l'empreinte de contenu de `plan.sources` | Relecture de `I` |
| 4 | `verified` | `verified_sha256` | Idem | Porte de provenance |
| 5 | `publish-intent` | `staging_identity` | Égale à celle de `staged` ; `P` absent attendu | Bascule `I` → `P`, porte de provenance, receipt de snapshot |
| 6 | `published` | `receipt_sha256` | — | Terminal |

La comparaison de `receipt_sha256` au receipt de snapshot présent est une
**observation**, pas une cohérence du journal : un écart relève des
snapshots approuvés, plus bas, et non des ambiguïtés du §3.

Rollback : `unpublish-intent`, de champ `staging_identity` égal à celui de
`staged`, précède le seul déplacement `P` → `I` ; puis `rolled-back`, sans
champ. `rollback-failed` (`error`) : au plus un par intention, non terminal.
`failed` et `abandoned` (`reason`), `published` et `rolled-back` sont
terminaux. Les étapes de rotation et les champs `previous` et
`current_identity` sont interdits dans ce mode : `source_identity` porte un
autre nom pour que l'identité de `current` relevée par un snapshot ne soit
jamais lue comme celle de l'ancien `current` d'une rotation.

#### Création

Sous verrou, préconditions de création d'ADR 0008 remplies : `current`
reconnu par toute sa chaîne de preuve (§4, §5), égal à son receipt, identité
relue après le contenu ; aucune opération non terminale dans l'un ou l'autre
domaine ; espace recontrôlé.

1. Identifiant contrôlé ; structure complétée ; dossier d'opération ;
   `planned`.
2. `current` relu en entier contre son receipt, identité relue après ;
   `staging-intent` ; création exclusive de `I` ; copie hachée, chaque fichier
   flushé avant sa fermeture ; identité de `current` relue après la copie.
   Identité changée : code 3, rien enregistré de plus. Contenu copié différent
   du plan : `failed`, code 2. `I` est conservé dans les deux cas.
3. `staged` ; relecture de `I` depuis le disque contre `plan.sources`, puis
   identité de `I` relue. Contenu différent : `failed`,
   `verification-mismatch`, code 2, `I` conservé. Identité changée : code 3,
   rien enregistré de plus. Conforme : `verified`.
4. Porte de provenance. Fermée : `failed`, code 3, cause `source-unverified`,
   `I` conservé ; rien n'est publié. Franchie : `publish-intent`.
5. Bascule `I` → `P` dans l'ordre du §4 : fermer toute portée ouverte sur
   `I`, `P` et `current` ; ne garder épinglés que les ancêtres de
   `snapshots\journal\<snap-id>` et ce dossier ; pré-contrôle de `I`, nom `P`
   absent ; `MoveFileEx(I, P, 0)` avec les retries du §7 ; post-contrôle :
   `P` de l'identité `staging_identity`, `I` absent. Un renommage dont l'échec
   est confirmé par la source et la cible donne `rolled-back`, code 1 ; toute
   autre observation, code 3.
6. Porte de provenance de nouveau. Preuve contradictoire :
   `unpublish-intent`, `P` → `I`, `rolled-back`, code 3, cause
   `source-unverified`. Preuve indisponible : code 3, rien écrit,
   l'opération reste en `publish-intent`. Franchie : receipt de snapshot
   validé par temporaire et renommage (§3, E1), puis `published`.

#### Porte de provenance

Une opération de snapshot n'approuve rien sans recouper trois preuves :
`receipts\<source-op>.json`, du domaine de `current`, que le protocole ne
réécrit ni ne supprime ; son `planned` ; le receipt de snapshot qu'elle va
écrire. La porte est franchie avant `publish-intent` et avant l'écriture du
receipt de snapshot, en exécution comme en récupération :

1. `receipts\` et `receipts\<source_operation>.json` ouverts sans suivi de
   lien, dossier et fichier ordinaires sans point d'analyse ;
2. receipt valide selon le receipt v1, d'opération `source_operation`, de jeu
   égal au `<game-id>` du dossier et au jeu du plan ;
3. SHA-256 de ses octets égal à `source_receipt_sha256` ;
4. empreinte de contenu de ses `sources` égale à celle de `plan.sources`.

Il n'a pas à être le receipt de tête : après une rotation de `current`, il ne
l'est plus, et la porte reste franchissable. Ni `current` ni `retained\` ne
sont lus.

- **Preuve indisponible** : erreur de lecture autre que l'absence, verrou ou
  partage refusé. Rien n'est écrit ni déplacé ; une nouvelle exécution
  réessaie.
- **Preuve contradictoire** : receipt absent, lien ou point d'analyse,
  invalide, d'une autre opération ou d'un autre jeu, octets ou empreinte
  différents.
- Selon la position : avant `publish-intent`, toute porte fermée clôt
  l'opération en `failed`, rien n'étant publié ; après la bascule et avant
  tout receipt de snapshot validé, une preuve contradictoire mène au
  rollback, `unpublish-intent` puis `P` → `I` puis `rolled-back`, qui
  n'approuvent rien et ne franchissent pas la porte ; une preuve
  indisponible laisse l'opération en `publish-intent`, sans rien écrire.
  Dans tous les cas, cause `source-unverified`, code 3.
- Une fois le receipt de snapshot validé (A5, A6), la porte n'est plus
  rejouée : ce receipt n'est écrit qu'après elle, et ses `source.*` égalent
  ceux de `planned`. Il tient lieu de preuve de provenance, comme pour un
  snapshot publié.

**Limite.** La porte vérifie la **cohérence** de preuves conservées dans le
même coffre, liées par des SHA-256 sans clé. Elle refuse une altération
accidentelle ou partielle ; elle n'authentifie pas contre une réécriture
délibérée et coordonnée du journal et du receipt source, hors modèle, comme
pour le recoupement du receipt de `<prev-op>` en B2c.

#### Points d'arrêt

Ceux de la première publication, A1 à A8 (§4), avec `P` à la place de
`current` et `I` à la place de `staging\<op>`. En A3 et A4, `P` est revérifié
en entier contre le plan, sans lire `current`, et la porte de provenance est
franchie avant tout receipt. En A5 et A6, le receipt de snapshot validé
interdit tout retour en arrière : relu strictement, égal au plan, `P`
conforme, il donne `published` ; toute autre observation, code 3.

#### Récupération

`--recover` ne s'appuie que sur le domaine des snapshots :

- **exigé** : le marqueur du coffre ; `<vault>`, `games`, `games\<game-id>`,
  `snapshots` et `snapshots\journal`, dossiers ordinaires sans point
  d'analyse ; les journaux et receipts du domaine des snapshots, relus
  strictement ; avec `--apply`, le verrou, pris comme ci-dessus ;
- **ignoré** : la présence, la reconnaissance et l'intégrité de `current` ;
  les opérations, journaux, receipts et anomalies du domaine de `current` ;
  les autres entrées de la racine du coffre, de `games` et de
  `games\<game-id>`, qui ne sont pas lues. Seul le receipt source est lu, à la
  porte de provenance. `current` n'est jamais lu ;
- `snapshots\` ou `snapshots\journal` absent : rien à récupérer, code 0.

Un receipt de snapshot validé pour une opération n'admet que deux lignes :
`publish-intent` avec (`new`, —), qui est A5 ou A6, et `published`. Toute
autre combinaison, et tout receipt de snapshot sans journal ou d'une
opération close autrement qu'en `published`, est une ambiguïté, code 3.

| Dernier validé | Observation (`P`, `I`) | Action |
| --- | --- | --- |
| Aucun : dossier d'opération vide ou temporaires `001` seuls | (—, —) | `001-abandoned` ; sinon code 3, rien écrit |
| `planned` | (—, —) | `abandoned` |
| `staging-intent` | (—, —), ou (—, dossier ordinaire sans point d'analyse) | `abandoned` ; `I` laissé en place, ni lu ni déplacé |
| `staged`, `verified` | (—, `new`) | `abandoned` ; `I` conservé |
| `publish-intent`, aucun receipt de snapshot | (—, `new`) | Renommage non effectué : `rolled-back` |
| `publish-intent`, aucun receipt de snapshot | (`new`, —) | Renommage effectué : revérification intégrale de `P` contre le plan. Égal, porte franchie : receipt, puis `published`. Égal, preuve contradictoire : `unpublish-intent`, `P` → `I`, `rolled-back`, code 3, `source-unverified`. Égal, preuve indisponible : code 3, rien écrit. Différent : `unpublish-intent`, puis `P` → `I` |
| `publish-intent`, receipt de snapshot validé | (`new`, —) | Receipt relu strictement, égal au plan, `P` revérifié en entier : `published`, sans rejouer la porte ; sinon code 3, rien écrit ni déplacé |
| `unpublish-intent`, aucun receipt de snapshot | (`new`, —) | Déplacement non effectué : l'exécuter, puis `rolled-back` ; échec : `rollback-failed`, code 3 |
| `unpublish-intent` ou `rollback-failed`, aucun receipt de snapshot | (—, `new`) | Déplacement effectué : `rolled-back` |
| `rollback-failed`, aucun receipt de snapshot | (`new`, —) | Nouvel essai ; succès : `rolled-back` ; échec : code 3, sans nouvel enregistrement (E3) |
| `published` | (`new`, —), receipt de snapshot conforme | Rien |
| `published` | Toute autre observation de `P`, de `I` ou du receipt de snapshot | Snapshot dégradé, plus bas : rien écrit ni déplacé |
| Tout autre cas, dont `P` et `I` présents ensemble hors d'une opération close, une identité `other`, un receipt de snapshot validé sans bascule observée | — | Fail-closed, code 3, rien écrit |

`new` est l'identité de `I` relevée dans `staged` ; `other`, toute autre
identité. Chaque revérification relit l'identité après le contenu. Les
propriétés du §9 s'appliquent : explicite, idempotente, interruptible,
convergente et bornée dans l'enveloppe, monotone, non aggravante.

#### Snapshots approuvés : exception au §3

Une opération de snapshot dont le journal, valide, se termine par `published`
n'est jamais une ambiguïté, quoi qu'on observe de son snapshot : aucune
opération ne dépend d'un snapshot déjà approuvé, et rien ne pourrait lever un
code 3 sans suppression ni réécriture. Elle est signalée comme **dégradée**
selon ADR 0008, rien n'est écrit ni déplacé pour elle, et elle ne compte plus
pour l'idempotence :

| Observation | Diagnostic |
| --- | --- |
| `P` absent, `I` absent ou d'une autre identité | `snapshot-missing` |
| `P` absent, `I` présent de l'identité `new` : renommage perdu, signature d'une coupure (§6) | `snapshot-rename-lost` ; `I` n'est jamais renommé |
| `P` d'une autre identité, ou `P` conforme à côté d'un `I` présent ; receipt de snapshot absent, invalide, contraire au plan ou d'un SHA-256 différent de `receipt_sha256` ; contenu relu différent | `snapshot-modified` |

Hors de cette exception, toute ambiguïté du domaine des snapshots reste
fail-closed selon le §3 : opération non terminale, journal illisible ou
incohérent, entrée inexpliquée. Elle bloque la création de snapshots, jamais
`sync` ; elle exige l'intervention du propriétaire du coffre, et un nouveau
coffre reste possible, comme en B2.

#### Enveloppe

E1, E3 et E4 inchangées ; E1 vaut aussi pour chaque
`snapshots\receipts\<snap-id>.json`. E2 : au plus 11 enregistrements ; le plus
long chemin d'un snapshot en compte 8, `planned` à `publish-intent`, puis
`unpublish-intent`, `rollback-failed` et `rolled-back`.

#### Ce qu'une opération de snapshot ne fait jamais

Écrire, renommer ou supprimer hors de `snapshots\`, hormis la création du
fichier de verrou vide par `Lock`, comme `sync` ; écrire dans `sync.lock` ;
déplacer `P` après `published` ; renommer un `I` vers `P` hors de la bascule
de sa propre opération ; remplacer une cible ; copier vers un nom publié ;
lire un objet dont l'identité n'est pas celle attendue ; lire `current`
pendant une récupération ; approuver sans porte de provenance ; supprimer un
temporaire ou un `I` ; recréer ou réparer un snapshot approuvé.
Immutabilité protocolaire, non matérielle : rien n'empêche un autre programme
de modifier un snapshot ; Savewarden ne le fait jamais et le signale dans les
limites d'ADR 0008.

### 11. Restauration isolée (B4)

**Statut : projet, non accepté**, rédigé le 1er novembre 2026 avec le plan B4
v0.2. Aucun code ne l'implémente. Il ne change aucune règle des §1 à §10 :
une restauration n'écrit pas le coffre. Il ajoute un invariant, plus bas, qui
contraint toute opération future du coffre.

Une restauration copie un snapshot approuvé (§10) dans une **zone de
restauration**, dossier hors du coffre que `restore` crée et marque, la
vérifie et l'y publie par renommage. Les invariants I1 à I12 s'appliquent à
la zone avec les précisions et les écarts de ce paragraphe, qui prévalent
pour elle. Le contrat de la commande, le receipt de restauration et le
marqueur de zone sont fixés par ADR 0009.

#### Zone et disposition

```text
<dest>\
  savewarden-restore.json            marqueur de zone (ADR 0009), et ses temporaires .tmp-<k>
  restore.lock                       verrou de la zone
  receipts\<op>.json                 receipt de restauration, et ses temporaires .tmp-<k>
  journal\<op>\NNN-<étape>.rec       journal de l'opération de restauration
  <op>.incomplete\<source-id>\...    I : copie en cours, échouée ou abandonnée
  <op>\<source-id>\...               P : restauration publiée, remise à l'utilisateur
```

- `<op>` est l'identifiant de l'opération, de la forme du §3, distinct de
  tout identifiant employé dans la zone et de celui du snapshot restauré ;
  le contrôle a lieu avant tout journal.
- La zone est marquée par `savewarden-restore.json`, validé par temporaire et
  renommage comme le marqueur du coffre ; elle sert à un seul jeu.
- **Une session d'écriture** de `platform/vault` s'ouvre sur la zone comme
  sur un coffre, avec les mêmes primitives et le même garde (ADR 0005) ; elle
  ne s'ouvre jamais sur le coffre pour une restauration.
- **Grammaire**, chaque entrée observée sans suivi, en deux classes :
  - **structure**, jamais remise : le marqueur, ses temporaires et
    `restore.lock`, fichiers ordinaires ; `receipts` et `journal`, dossiers
    ordinaires ; dans `receipts\`, des fichiers `<op>.json` et leurs
    temporaires `<op>.json.tmp-<k>`, `k` de 1 à 9 ; dans `journal\`, des
    dossiers `<op>` au contenu du §3. Toute autre entrée, un lien, un point
    d'analyse, un fichier à la place d'un dossier ou l'inverse : ambiguïté,
    code 3 ;
  - **objets d'opération**, `<op>` et `<op>.incomplete` à la racine, noms
    comparés sans égard à la casse comme Windows les résout : un nom
    qu'aucune opération journalisée de la zone n'explique est une ambiguïté,
    code 3, quel que soit son type. Pour une opération non terminale, `P` et
    `I` sont des dossiers ordinaires sans point d'analyse, l'identité de `I`
    est exigée dès que `staged` l'a fixée, et un `I` antérieur à `staged`
    est accepté sans être lu ni déplacé ; tout autre type ou identité est une
    ambiguïté. Pour une opération close autrement qu'en `published`, un
    `<op>` présent, de tout type, est une ambiguïté ; un `I` présent doit
    être un dossier ordinaire sans point d'analyse, de l'identité `new` si
    `staged` l'a fixée, sinon ambiguïté ; son absence ne l'est pas. Pour une
    restauration **remise**, voir plus bas : aucun objet sous ses noms n'est
    une ambiguïté, et une entrée de la racine qu'on ne peut décrire n'est
    classée `not observable` que sous ses noms ; partout ailleurs, elle
    arrête la lecture, code 1.
- **Structure.** `receipts`, puis `journal`, sont créés sous verrou, après le
  second contrôle d'espace et avant tout journal, par création exclusive et
  seulement s'ils manquent ; suit `journal\<op>`. Une structure faite de
  dossiers vides est complétée par la création suivante ; un dossier
  d'opération vide, ou ne contenant que des temporaires `001-…`, est une
  opération non commencée ; des opérations journalisées sans `receipts` sont
  une ambiguïté.

#### Destination et distinctions

- `--apply` n'écrit que dans une destination absente dont le parent est un
  dossier ordinaire sans point d'analyse, un dossier ordinaire vide, ou une
  zone marquée du même jeu sans opération non terminale ni ambiguïté. Tout
  autre dossier, dont un dossier non vide sans marqueur, n'est **jamais
  adopté**.
- Aucun ancêtre de la zone n'est un lien ou un point d'analyse, ni ne
  contient `savewarden-vault.json` ou `savewarden-restore.json` ; volume NTFS
  sur un lecteur local fixe ou amovible.
- **Chemins de référence** : le coffre, à la création ; l'installation et
  chaque source du manifeste ; et les chemins des `sources` du plan, recopiés
  du receipt de snapshot, qui sont ceux réellement sauvegardés et se lisent
  dans le journal de la zone sans le coffre. La zone n'est ni égale, ni
  intérieure, ni ancêtre d'aucun d'eux. Ces contrôles sont faits à la
  création et **refaits à chaque récupération** qui peut écrire, puisqu'une
  zone déplacée sur son volume garde ses identités et ses noms relatifs ; le
  type et le système de fichiers du volume de la zone sont recontrôlés de
  même.
- **Observabilité.** Une identité impossible à observer ne prouve jamais la
  séparation. Les chaînes s'entendent **depuis la racine du volume, racine
  comprise**, pour la zone comme pour `X`, et les identités se comparent dans
  les deux sens. Pour chaque chemin de référence `X`, la première ligne qui
  s'applique décide :

  | # | Cas | Contrôle | Issue |
  | --- | --- | --- | --- |
  | 1 | Chaîne existante de `X` non observable, quelle que soit la lettre : accès refusé, erreur de lecture, identité illisible, lien ou point d'analyse | Aucune preuve | **Refus**, code 1 |
  | 2 | Lettre de `X` sans volume : racine rapportée absente, lettre non attribuée, alias retiré ou lecteur sans média | Aucune preuve : la lettre a pu désigner un alias ou un volume monté aujourd'hui sous une autre lettre | **Refus**, code 1, « separation from `X` cannot be proven » |
  | 3 | `X` présent | Identité de `X` cherchée dans la chaîne de la zone ; identité de la zone cherchée dans la chaîne de `X` ; si la zone est absente, seule la première recherche s'applique, sur la chaîne de son parent | Décisif |
  | 4 | `X` absent, son volume lisible | Chaîne existante de `X` jusqu'à son ancêtre présent le plus profond : `X` ne peut contenir la zone ; il y serait si l'identité de la zone figure dans cette chaîne. En outre, chemins comparés sous la racine de leur lettre, segments sans égard à la casse, contre une permutation de lettres de lecteurs amovibles : égaux ou imbriqués, refus | Décisif pour `X` tel qu'il se résout aujourd'hui |

  Sur une même lettre, la comparaison des segments sans égard à la casse
  précède ces lignes et refuse tout recouvrement. **Limite** : les
  distinctions portent sur les chemins de référence tels qu'ils se résolvent
  aujourd'hui ; des données déplacées sans que ces chemins le reflètent ne
  sont protégées que si un cas décisif les atteint. Une restauration dont un
  chemin de référence est sur une lettre sans volume est refusée tant que
  cette lettre ne désigne pas de nouveau un volume.

#### Initialisation de la zone

Comme I11, faite uniquement de créations exclusives ; Z2 à Z4 ne concernent
qu'une zone nouvelle, une zone marquée commençant en Z5 :

| # | Étape | État laissé par un arrêt juste après | Traitement au lancement suivant |
| --- | --- | --- | --- |
| Z1 | Préflight : destination acceptée, distinctions, espace | Rien | — |
| Z2 | Session ouverte sur `<dest>`, son parent épinglé ; `CreateDirectory(<dest>)` s'il manque ; s'il apparaît entre-temps, retour au préflight | `<dest>` vide | Destination vide, acceptée |
| Z3 | `<dest>` épinglé et **relu** : vide exigé ; temporaire du marqueur en `CREATE_NEW`, écrit, flushé, fermé | `<dest>` ne contient que des temporaires du marqueur | Initialisation interrompue : refusée par `--apply`, code 3 ; seul `--recover --apply`, distinctions refaites, **sans verrou**, `<dest>` épinglé et relu, valide un nouveau marqueur au `k` libre suivant ; aucun verrou n'est jamais créé dans un dossier non marqué |
| Z4 | `MoveFileEx(temporaire, savewarden-restore.json, 0)` | Zone marquée | Normal |
| Z5 | `restore.lock` par `Lock`, créé vide s'il manque, jamais écrit | Verrou vide | Normal |
| Z6 | Sous verrou : zone relue et comparée au plan, hors des observations des restaurations remises, qui ne la changent pas ; espace recontrôlé | Métadonnées seules | Zone changée depuis le plan, ou espace insuffisant : code 1 ou 2, rien écrit de plus |
| Z7 | `receipts`, puis `journal`, s'ils manquent | Structure | Ci-dessus |
| Z8 | `journal\<op>`, exclusif | Opération non commencée | Bloque `--apply` ; `--recover --apply` valide `001-abandoned` |
| Z9 | Validation de `001-planned` | Opération commencée | Automate ci-dessous |

Refus sans mutation : temporaires du marqueur et toute autre entrée ;
marqueur illisible, de schéma inconnu ou d'un autre jeu ; neuf temporaires du
marqueur, enveloppe E1 épuisée, code 3. La relecture de Z3 réduit sans la
fermer la fenêtre où un autre programme remplirait `<dest>` juste avant le
marqueur ; un ajout dans cette fenêtre est vu par Z6, code 1, rien écrit de
plus, puis, aux exécutions suivantes, comme une entrée imprévue de la zone,
code 3.

#### Verrous, I8 et invariant des snapshots publiés

- **I8 par zone.** `restore.lock` garantit qu'une seule opération s'exécute
  à la fois dans une zone. Deux opérations non terminales dans une zone sont
  une ambiguïté, code 3 ; une opération non terminale bloque toute nouvelle
  restauration dans cette zone. Avant de déclarer une opération en attente,
  le plan sonde `restore.lock` en lecture seule ; tenu, c'est une opération
  en cours, code 1.
- **Aucun verrou du coffre.** Une restauration n'ouvre, ne sonde ni ne crée
  `sync.lock`, n'exclut ni `sync` ni `snapshot`, et n'en est pas exclue.
- **Invariant, prémisse de cette absence** : aucune opération de Savewarden ne
  renomme un snapshot publié ni l'un de ses ancêtres, ni ne les ouvre sans
  partage en lecture. Une opération qui le lèverait exige un amendement de
  ce paragraphe avant son code.

#### Authentification du snapshot

Avant tout journal, puis de nouveau sous le verrou de la zone, sans relire
le contenu la seconde fois, par une lecture **ciblée** du coffre : son
marqueur, la chaîne `<vault>` à `snapshots\journal\<snap-id>`, le journal du
snapshot, `snapshots\receipts` et son receipt, `S` = `snapshots\<snap-id>` et
l'absence de `snapshots\<snap-id>.incomplete`. Le journal est terminal en
`published` ; le receipt de snapshot est relu strictement (ADR 0008) et ses
octets ont pour SHA-256 le `receipt_sha256` de `published` ; `S` a l'identité
`staging_identity` ; son contenu est relu en entier contre les `sources` du
receipt, identité relue après. Rien d'autre n'est lu dans le coffre : ni
`games\<game-id>` n'est listé, ni `sync.lock` n'est observé, même par
`Lstat`. Le classement des refus est celui d'ADR 0009 ; un objet d'une
identité inattendue n'est jamais lu.

#### Automate et contrat des champs

Même format d'enregistrement qu'au §3, mêmes noms d'étapes que la première
publication, mode **restauration** propre : le lecteur l'applique au seul
dossier `journal\<op>` d'une zone, et un journal de snapshot ou de `current`
n'y est jamais admis, ni l'inverse. Tous les champs sont obligatoires, aucun
autre n'est accepté.

| # | Enregistrement validé | Champs | Cohérence vérifiée à la lecture | Action qui suit |
| --- | --- | --- | --- | --- |
| 1 | `planned` | `snapshot` ; `snapshot_receipt_sha256` ; `snapshot_identity`, identité de `S` ; `plan` : objet strict, version de l'outil, jeu, `started_at`, `source` recopiée du receipt de snapshot (`operation`, `receipt_sha256`), `sources` au format du receipt v1 ; `plan_sha256` | `plan_sha256` est le SHA-256 de `plan` ; `snapshot` et `plan.source.operation` au format du §3, distincts entre eux et de l'opération ; jeu du plan égal à celui du marqueur | — |
| 2 | `staging-intent` | `staging` : `<op>.incomplete` | Nom fixé par l'opération | Créer `I`, copier, flusher |
| 3 | `staged` | `staging_identity` ; `copied_sha256` | `copied_sha256` est l'empreinte de contenu de `plan.sources` | Relecture de `I` |
| 4 | `verified` | `verified_sha256` | Idem | — |
| 5 | `publish-intent` | `staging_identity` | Égale à celle de `staged` ; `P` absent attendu | Bascule `I` → `P`, receipt de restauration |
| 6 | `published` | `receipt_sha256` | — | Terminal |

Rollback : `unpublish-intent`, de champ `staging_identity` égal à celui de
`staged`, précède le seul déplacement `P` → `I` ; puis `rolled-back`.
`rollback-failed` (`error`) : au plus un par intention. `failed` et
`abandoned` (`reason`), `published` et `rolled-back` sont terminaux. Les
étapes de rotation, et les champs `previous`, `current_identity`,
`source_operation`, `source_receipt_sha256` et `source_identity`, sont
interdits dans ce mode.

#### Création

Sous le verrou de la zone, préconditions d'ADR 0009 remplies :

1. Identifiant contrôlé ; Z7, Z8 ; `planned`.
2. `staging-intent` ; création exclusive de `I` ; copie hachée de chaque
   fichier de `S` contre le plan, date conservée, chaque fichier flushé avant
   sa fermeture ; identité de `S` relue après la copie ; portée de lecture du
   coffre fermée. Contenu différent, ou `S` remplacé : `failed`, code 2, `I`
   conservé (écart X2). Fichier de `S` absent : racine et marqueur du coffre
   contrôlés de nouveau ; coffre disparu ou illisible : `failed`, code 1 ;
   coffre présent : `failed`, code 2. Toute autre erreur de lecture :
   `failed`, code 1.
3. `staged` ; relecture de `I` depuis le disque contre `plan.sources`, puis
   identité de `I` relue. Contenu différent : `failed`, code 2, `I` conservé.
   Identité changée : code 3, rien enregistré de plus. Conforme : `verified`.
4. `publish-intent` ; bascule `I` → `P` dans l'ordre du §4 : fermer toute
   portée sur `I` et `P` ; ne garder épinglés que les ancêtres de `<dest>`,
   `<dest>` et `journal\<op>` ; pré-contrôle de `I`, nom `P` absent ;
   `MoveFileEx(I, P, 0)` avec les retries du §7 ; post-contrôle : `P` de
   l'identité `staging_identity`, `I` absent. Échec confirmé par la source et
   la cible : `rolled-back`, code 1 ; toute autre observation, récupération.
5. Receipt de restauration validé par temporaire et renommage (§3, E1), puis
   `published`.

Aucune porte de provenance n'est rejouée sur le coffre après `planned` : la
copie et sa relecture sont comparées au plan, issu du snapshot authentifié.

#### Points d'arrêt

Ceux de la première publication, A1 à A8 (§4), avec `P` à la place de
`current` et `I` à la place de `staging\<op>`. En A3 et A4, `P` est revérifié
en entier contre le plan, sans lire le coffre. En A5 et A6, le receipt de
restauration validé interdit tout retour en arrière : relu strictement, égal
au plan, `P` conforme, il donne `published` ; toute autre observation,
code 3.

#### Récupération

`--recover` ne s'appuie que sur la zone et le manifeste : il est
**indépendant du coffre**, jamais ouvert, lu ni nommé, et **dépendant du
manifeste** et des contrôles de sûreté de la destination. Dans cet ordre :

1. `<dest>` est classé par le tableau ci-dessous ; un état sans rien à
   récupérer conclut aussitôt, **sans ouvrir de session ni rien écrire**, pas
   même un `restore.lock` absent ;
2. pour tout état où la récupération peut écrire : manifeste strict, marqueur
   du même jeu, distinctions refaites selon la règle d'observabilité, volume
   recontrôlé ; un échec donne code 1 avant toute écriture ;
3. grammaire de la zone, journaux et receipts relus strictement ; avec
   `--apply`, le verrou de la zone, pris seulement dans une zone marquée ;
   puis la ligne du tableau de récupération.

Le contenu de l'installation et des sources n'est jamais lu ; seules leurs
identités servent.

**Destination sans opération journalisée** : seule une zone sans aucun objet
d'opération n'a rien à récupérer.

| Observation | Issue |
| --- | --- |
| `<dest>` absent, son parent présent ou non, ou dossier ordinaire vide | Rien à récupérer, code 0 |
| `<dest>` fichier, lien, jonction ou autre point d'analyse | Pas une zone : code 1 |
| Temporaires du marqueur seulement | Reprise de Z3 par `--recover --apply`, sans verrou |
| Dossier non vide sans marqueur, hors du cas précédent | Pas une zone : code 1, jamais adopté |
| Zone marquée dont les seules entrées sont le marqueur, ses temporaires, `restore.lock`, et `receipts` et `journal` vides ou absents | Rien à récupérer, code 0 |
| `<op>`, `<op>.incomplete`, receipt ou temporaire de receipt qu'aucun `journal\<op>` n'explique | Ambiguïté, code 3, rien écrit |
| Zone marquée portant toute autre entrée hors de la grammaire | `unexpected-entry`, code 3, rien écrit |

Pendant A3 à A6, le plan annonce « operation `<op>` pending recovery: copy
published, receipt missing; do not use `<dest>\<op>` yet ».

Un receipt de restauration validé n'admet que deux lignes, A5 ou A6 et
`published` ; toute autre combinaison, et tout receipt sans journal ou d'une
opération close autrement qu'en `published`, est une ambiguïté, code 3.

| Dernier validé | Observation (`P`, `I`) | Action |
| --- | --- | --- |
| Aucun : dossier d'opération vide ou temporaires `001` seuls | (—, —) | `001-abandoned` ; sinon code 3, rien écrit |
| `planned` | (—, —) | `abandoned` |
| `staging-intent` | (—, —), ou (—, dossier ordinaire sans point d'analyse) | `abandoned` ; `I` laissé en place, ni lu ni déplacé |
| `staged`, `verified` | (—, `new`) | `abandoned` ; `I` conservé |
| `publish-intent`, aucun receipt | (—, `new`) | Renommage non effectué : `rolled-back` |
| `publish-intent`, aucun receipt | (`new`, —) | Renommage effectué : revérification intégrale de `P` contre le plan ; égal : receipt, puis `published` ; différent : `unpublish-intent`, puis `P` → `I`, `rolled-back`, code 2, `verification-mismatch` |
| `publish-intent`, receipt validé | (`new`, —) | Receipt relu strictement, égal au plan, `P` revérifié en entier : `published` ; sinon code 3, rien écrit ni déplacé |
| `unpublish-intent`, aucun receipt | (`new`, —) | Déplacement non effectué : l'exécuter, puis `rolled-back` ; échec : `rollback-failed`, code 3 |
| `unpublish-intent` ou `rollback-failed`, aucun receipt | (—, `new`) | Déplacement effectué : `rolled-back` |
| `rollback-failed`, aucun receipt | (`new`, —) | Nouvel essai ; succès : `rolled-back` ; échec : code 3, sans nouvel enregistrement (E3) |
| `published` | Tout objet, de tout type, sous `P` et sous `I` | Restauration remise, plus bas : rien écrit ni déplacé ; receipt absent, invalide ou d'un SHA-256 différent de `receipt_sha256` : ambiguïté, code 3 |
| `failed`, `abandoned`, `rolled-back` | `P` absent ; `I` absent, ou dossier ordinaire sans point d'analyse, de l'identité `new` si `staged` l'a fixée | Rien |
| `failed`, `abandoned`, `rolled-back` | `P` présent, de tout type ; ou `I` d'un autre type ou d'une autre identité | Ambiguïté, code 3, rien écrit |
| Tout autre cas, dont `P` et `I` présents ensemble avant `published`, une identité `other`, un receipt validé sans bascule observée | — | Fail-closed, code 3, rien écrit |

`new` est l'identité de `I` relevée dans `staged` ; `other`, toute autre
identité. Chaque revérification relit l'identité après le contenu. Les
propriétés du §9 s'appliquent : explicite, idempotente, interruptible,
convergente et bornée dans l'enveloppe, monotone, non aggravante.

#### Restaurations remises : exception au §3

Une opération de restauration dont le journal, valide, se termine par
`published` est **remise** : sa copie appartient à l'utilisateur, qui peut la
modifier, la déplacer ou la supprimer. Aucune observation de ses noms `P` et
`I` n'est une ambiguïté, quel que soit le type de l'objet : dossier, fichier,
lien, jonction ou autre point d'analyse. Ils ne sont observés que par `Lstat`
et, pour un dossier ordinaire sans point d'analyse, par leur identité, lue
par un handle sans suivi aussitôt refermé ; ils ne sont jamais ouverts pour
leur contenu, listés, épinglés, lus, suivis, déplacés, réutilisés ni
réparés, et Savewarden n'y écrit jamais. Le classement, informatif et jamais
bloquant, est celui d'ADR 0009. Son receipt et son journal ne sont pas
remis : ils restent soumis au §3. Un receipt absent après `published`, même
perdu par une coupure qui aurait réordonné les métadonnées (§6), est donc une
ambiguïté, code 3, qui ferme la zone à de nouvelles restaurations ; la copie
remise reste en place et une nouvelle zone reste possible.

#### Écarts nommés

| # | Écart | Raison |
| --- | --- | --- |
| X1 | Ni verrou du coffre ni I8 du coffre pour une restauration, qui ne sonde pas `sync.lock` | Invariant des snapshots publiés ci-dessus ; sonder `sync.lock` l'ouvrirait un instant et ferait échouer le `Lock` d'un `sync` concurrent |
| X2 | `S` remplacé pendant la copie : `failed`, code 2, et non une ambiguïté | `S` est dans le coffre, qu'une restauration n'écrit pas ; la zone reste cohérente, rien n'est publié ; le diagnostic nomme le remplacement sans le prendre pour un changement de contenu |
| X3 | Lecture ciblée : les anomalies du coffre hors du snapshot désigné sont ignorées | L'authenticité de `S` repose sur sa propre chaîne de preuve, relue en entier |
| X4 | Opération du snapshot non terminale ou journal ambigu côté coffre : code 1, et non 3 | Une restauration ne peut ni distinguer une opération vivante sans sonder `sync.lock`, ni intervenir sur le coffre ; le code 3 reste réservé à la zone |

#### Enveloppe

E1, E3 et E4 inchangées ; E1 vaut pour chaque numéro d'enregistrement, chaque
`receipts\<op>.json` et le marqueur de zone. E2 : au plus 11 enregistrements ;
le plus long chemin d'une restauration en compte 8, `planned` à
`publish-intent`, puis `unpublish-intent`, `rollback-failed` et
`rolled-back`.

#### Ce qu'une restauration ne fait jamais

Écrire, renommer, verrouiller, sonder ou supprimer quoi que ce soit dans le
coffre ; y lire autre chose que le snapshot désigné ; lire le coffre pendant
une récupération ; écrire hors de la zone, hormis la création de `<dest>`
absent ; écrire dans un dossier non marqué, hormis Z2 à Z4 ; adopter un
dossier ; remplacer une cible ; copier vers un nom publié ; déplacer `P` après
`published` ; ouvrir, lire, suivre ou déplacer un objet remis ; supprimer un
temporaire ou un `I` ; restaurer un snapshot non publié, dégradé ou non
vérifiable ; tenir une identité inobservable pour une séparation ; restaurer
dans l'installation, une source, un coffre ou une autre zone.

## Alternatives

- **Échange atomique de répertoires** : Windows n'en fournit pas ; un
  renommage par-dessus un dossier existant échoue.
- **NTFS transactionnel** (`MoveFileTransacted`, TxF) : Microsoft en
  déconseille l'usage ; rejeté.
- **`MOVEFILE_WRITE_THROUGH` comme garantie** : sa documentation ne vise que
  les déplacements réalisés par copie puis suppression, que Savewarden refuse.
- **`os.Rename`, `Root.Rename` ou `MoveFile`** : les deux premiers remplacent
  une cible existante, le troisième autorise la copie entre volumes pour un
  fichier, donc une suppression implicite ; tous interdits.
- **Renommage par handle** (`FileRenameInfo`) : il supprimerait la fenêtre
  entre pré-contrôle et renommage, mais exige `SetFileInformationByHandle`,
  dont la même API supprime par `FileDispositionInfo` ; écarté en B2.
- **Récupération automatique au lancement suivant** : elle agirait sans
  consentement sur un état partiel ; la récupération reste explicite (D6).
- **Supprimer les temporaires pour rouvrir l'enveloppe** : contraire à la
  non-destruction (D7) ; l'enveloppe est bornée et déclarée.
- **Journal unique réécrit** : un fichier remplacé à chaque étape serait
  perdu en cas d'écriture interrompue ; le journal est fait d'enregistrements
  écrits une seule fois, chaînés.

## Conséquences et validation

- La publication n'est pas atomique : la fenêtre d'absence de `current` est
  documentée et récupérable, jamais présentée comme un échange atomique.
- Une ambiguïté bloque l'utilisateur en code 3 : c'est voulu. Le guide du
  coffre explique l'état, sans commande de suppression.
- Le classement de récupération ne concerne que les opérations non
  terminales. Pour une opération terminale, la cohérence de `current` relève
  du contrôle contre receipt du plan B2 §4.5.
- B2a implémente, sans aucune écriture, la validation pure du journal, le
  classement pur des observations de l'initialisation, de la première
  publication et de la rotation, et leurs tests par tableaux
  (`internal/entities/backup`). B2b implémente l'initialisation, la première
  publication et sa récupération ; B2c, la rotation. Chacun prouve ses
  transitions par énumération des points de crash.
- Les hypothèses Windows de §4 et §6, dont le non-blocage du renommage d'un
  enfant par un parent épinglé, sont confrontées à des tests réels avant le
  code qui en dépend ; un écart amende cet ADR d'abord.
- B3 et B4 publieront snapshots et restaurations selon ce protocole, ou
  l'amenderont : B3 par le §10, B4 par le §11, en projet.

## Amendements

- **29 octobre 2026, avant les premières écritures de B2b**, sur décision du
  propriétaire : enveloppe E1 au sens strict, neuf temporaires par numéro
  d'enregistrement toutes étapes confondues, et une enveloppe propre à chaque
  receipt et au marqueur ; champs propres à l'étape sous la clé `fields`,
  validés strictement par étape ; conditions exactes d'acceptation des
  temporaires abandonnés à côté d'un enregistrement validé (§3), que le
  lecteur de B2a refusait à tort et qui auraient rendu irrécupérable un état
  produit par le protocole. Tests : `TestAbandonedTemporariesAreAccepted`,
  `TestIncoherentTemporariesAreAmbiguous`,
  `TestAbandonedTemporariesDoNotConsumeTheEnvelope`.
- **29 octobre 2026, champs d'étape** : contrat fermé et typé de chaque
  étape, cohérence entre enregistrements vérifiée à la lecture (§3) ;
  `TestStepFieldsAreStrict`, `TestRecordsAreCoherent`.
- Ces amendements prévalent sur le §5 du plan B2 v0.2, qui reste inchangé
  comme document validé : notamment sur « Temporaire d'un autre numéro que le
  suivant », désormais précisé au §3, et sur l'enveloppe E1, désormais par
  destination logique.
- **29 octobre 2026, adaptateur `platform/vault`** : l'hypothèse du §4,
  qu'un parent épinglé n'empêche pas de renommer un enfant, est confirmée par
  `TestParentPinsDoNotBlockChildRename`, identité conservée par le
  renommage ; codes d'erreur observés consignés au §7
  (`TestOpenHandlesBlockTheRename`, `TestMoveNeverReplaces`). Aucun écart au
  protocole.
- **29 octobre 2026, cadrage de B2c**, sur décision du propriétaire, avant
  tout code de rotation : identité persistante de l'ancien `current`,
  `current_identity` de `planned`, obligatoire pour une rotation et interdit
  pour une première publication, reprise sans substitution par
  `retire-intent` et `restore-intent` (A1, §3) ; vérification intégrale de
  `current` contre son receipt sous verrou avant `planned` et de nouveau juste
  avant le retrait (A2, §5) ; règle M3 de l'opération non commencée, qui
  n'abandonne en présence d'un `current` qu'une génération précédente dont
  receipt, journal publié et contenu sont vérifiés (§4) ; receipt déjà validé
  protégé de la même façon en rotation qu'en première publication (§5). Sans
  cette identité, toute rotation interrompue avant `retire-intent` aurait été
  ambiguë. Les journaux B2b restent lisibles ; les enveloppes E1 à E4 sont
  inchangées. Ces amendements prévalent sur le §5 du plan B2 v0.2 ; leurs
  tests sont exigés par son §9.3 amendé.
- **29 octobre 2026, revue du cadrage de B2c** par le propriétaire :
  définition unique de la génération précédente reconnue, dont l'identité
  observée doit être celle que son journal a publiée (P0-2, §4) ; règle M3
  qui tient l'absence de `current` pour une contradiction dès qu'une
  génération publiée est attendue (P0-1, §4) ; relecture de
  `retained\<prev-op>` entre M1 et M2, nouvelle observation avant toute
  restauration après un échec de M2, et restaurations toujours vérifiées,
  sans promotion ni restauration d'une génération altérée (P1, §5) ;
  fenêtres résiduelles du verrou explicitées (P1, §5) ; mode plan aligné,
  `unexpected-entry` pour un `current` d'une autre identité que celle
  publiée ; `current-exists` conservé comme règle de B2b (§4).
- **29 octobre 2026, arbitrage des effets de la revue** par le
  propriétaire : coffre copié ou déplacé refusé en code 3 sans écriture ni
  adoption d'identité, diagnostic sans présomption de corruption,
  portabilité renvoyée à une procédure future de requalification ou
  d'import contrôlé (§4) ; règle M3 plus stricte que le code B2b validée ;
  `current` réapparu entre M1 et M2 : code 3, ni promotion ni restauration,
  objets conservés en place (§5).
- **29 octobre 2026, cohérence de la récupération après M1**, sur revue du
  propriétaire : un objet inattendu sous `current`, noté `other`, ou un
  `retained\<prev-op>` d'une autre identité ne relève d'aucune ligne du
  tableau : code 3, rien écrit, enregistré ni déplacé tant qu'il reste
  présent ; une récupération ne reprend que sur une observation entièrement
  conforme à une ligne, toutes ses vérifications comprises (§4, §5).
- **29 octobre 2026, revue indépendante du remplacement** (B2c, commit 4) :
  la troisième fenêtre résiduelle, entre la relecture d'une restauration
  vérifiée et M3, est nommée au §5 ; aucune règle ne change. Le code applique
  désormais strictement le §3 au pré-contrôle d'une bascule : une identité
  illisible, un lien ou une cible qui ne peut être observée sont une
  ambiguïté, code 3, sans `rolled-back` ni `rollback-failed`.
- **29 octobre 2026, revue de sécurité indépendante** (B2c) : un receipt de
  `<op>` validé alors que la bascule de `<op>` n'est pas observée est une
  contradiction du §3 : code 3, rien écrit, quelle que soit la ligne du
  tableau, première publication comprise ; la récupération d'une rotation
  vérifie, par recoupement, le receipt de `<prev-op>` contre le journal
  publié de cette génération et l'identité fixée par `planned` avant toute
  restauration vérifiée : une altération accidentelle ou partielle est
  refusée, pas une réécriture délibérée du receipt et du journal, hors
  modèle ; la fenêtre du staging entre `verified` et M2 est nommée au §5, et
  chaque relecture relit l'identité après le contenu, récupération et règle
  M3 comprises.
- **30 octobre 2026, ratification** par le propriétaire, après la fusion de
  B2c et son pilote, des deux entrées précédentes, postérieures au cadrage
  certifié ; aucune règle ne change. Le propriétaire accepte le compromis
  fail-closed de la ligne `unpromote-intent` (§5) : M4 précède la
  restauration vérifiée ; si `retained\<prev-op>` est altéré, rien n'est
  restauré et `current` reste absent jusqu'à une intervention, plutôt qu'une
  restauration non vérifiée.
- **30 octobre 2026, cadrage de B3**, accepté par le propriétaire le
  31 octobre 2026 (plan B3 v0.2) :
  §10, snapshots ; I8 précisé par domaine ; porte de provenance avant toute
  approbation d'un snapshot, rollback sur preuve contradictoire, aucune porte
  rejouée après un receipt de snapshot validé ; récupération d'un snapshot
  indépendante du domaine de `current`, verrou pris comme `sync` ; exception
  au §3 pour une opération de snapshot terminale en `published`, dont le
  snapshot dégradé est signalé sans blocage. Aucune règle des §1 à §9 ne
  change pour le domaine de `current`.
- **1er novembre 2026, cadrage de B4, projet non accepté** (plan B4 v0.2) :
  §11, restauration isolée dans une zone marquée hors du coffre, que la
  restauration n'écrit jamais ; journal, receipt et verrou dans la zone ;
  récupération sans le coffre, distinctions refaites selon une règle
  d'observabilité ; restaurations remises classées sans suivi ni blocage,
  exception au §3 ; écarts X1 à X4 nommés ; invariant des snapshots publiés.
  Renvois ajoutés aux §1, §2 (I8), §3 et §8 et dans les Conséquences. Aucune
  règle des §1 à §10 ne change pour le coffre.
- **2 novembre 2026, correction documentaire**, séparée des changements
  normatifs : l'en-tête et le statut du §10 disaient qu'aucun code ne
  l'implémentait ; B3 l'implémente depuis `08ec0c2` (PR #11), qualifié par le
  pilote du 1er novembre 2026. Aucune règle ne change.

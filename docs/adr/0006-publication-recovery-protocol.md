# ADR 0006 — Protocole de publication et de récupération

Date : 29 octobre 2026. Statut : accepté pour B2, contenu normatif du plan B2
v0.2 §5, validé par le propriétaire le 29 octobre 2026 après revue intégrale ;
amendé le même jour avant les premières écritures de B2b (voir Amendements).
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
fichiers sous un nom stable** dans un coffre Savewarden et à **toute
récupération** : `current` en B2, snapshots en B3, restauration vers un dossier
isolé en B4 pour son staging et sa publication. Une future opération qui ne
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
| I8 | Une seule opération par jeu à la fois ; une opération non terminale bloque toute nouvelle `--apply` |
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
| Deux opérations non terminales pour un jeu | Exclusivité violée |
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
| `publish-intent` | (`new`, —) | Renommage effectué : revérification intégrale de `current` contre le plan ; égal : receipt (plan B2 §4.7) puis `published` ; différent : `unpublish-intent`, puis `current` → `staging\<op>` |
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
| `planned` à `verified` | (`old`, —, `S` selon les règles du §4) | `abandoned` |
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
des temporaires de marqueur ; supprimer un temporaire pour rouvrir
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
  l'amenderont.

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

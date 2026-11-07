# Modèle de sécurité

## Garantie actuelle (B4)

`inspect` lit un manifeste désigné par l'utilisateur et les fichiers des
sources qu'il déclare. `sync` et `snapshot`, en mode plan, lisent en plus le
coffre désigné par `--vault`, s'il existe ; `snapshot` ne lit pas le contenu
des sources. `--apply` et `--recover --apply` écrivent, mais **seulement dans
ce coffre**, et seulement par créations exclusives et renommages qui ne
remplacent rien ; `snapshot` n'écrit que sous `snapshots\`, hormis le verrou
vide que `sync` créerait aussi. `restore` lit dans le coffre le seul snapshot
désigné et **n'écrit jamais le coffre** : il n'écrit que dans la zone de
restauration désignée par `--to`, distincte du coffre, du jeu et des sources,
avec les mêmes primitives. Aucune découverte d'installation, création de lien
ou mutation de configuration ; aucune écriture dans une source.

**Défaut ouvert, chantier [S-02](#chantiers-de-sécurité-ouverts).** Les
distinctions du coffre et de la zone ne tiennent pas quand une lettre de
lecteur désigne un dossier, par un alias `subst` par exemple : un coffre ou
une zone peut alors être créé dans l'installation, une source ou le coffre.
N'employez aucun alias de lettre, ni pour `--vault` ou `--to`, ni dans le
manifeste.

| Garantie | Mécanisme | Preuve |
| --- | --- | --- |
| Écritures confinées, aucune exécution | Seul `platform/vault` écrit, avec les API nommées par [ADR 0005](adr/0005-write-allowed-package.md) ; garde statique contre toute autre API d'écriture, d'exécution, de réseau et de chargement dynamique, dans tous les packages | `readonly_test.go` et ses listes attendues indépendantes ; `inspect` et le plan lancés deux fois avec l'écriture refusée par ACL ou `chmod`, empreinte identique ; `--apply` avec l'écriture refusée sur l'installation, source identique |
| Rien n'est remplacé ni supprimé | `CREATE_NEW`, `CreateDirectory`, `MoveFileEx` avec le littéral `0` ; aucun fichier existant rouvert en écriture ; temporaires jamais supprimés | Tests Windows réels : création et renommage refusés sur cible existante, erreur 183 ; garde AST des drapeaux |
| Publication vérifiée | Copie hachée, source inventoriée deux fois, staging relu depuis le disque, identité du staging contrôlée avant et après le renommage | Fautes injectées : source changée, écriture refusée, octet altéré, course sur le renommage |
| Rotation sans perte | `current` reconnu par receipt, journal publié et identité, relu en entier sous verrou et juste avant le retrait ; identité fixée par `planned` ; génération retirée relue avant la promotion ; restauration seulement si elle est intacte ; toute anomalie après le retrait : code 3, rien déplacé | Fautes injectées à chaque fenêtre : contenu ou identité changés, `current` réapparu, génération retenue altérée ; crash avant et après chaque appel, puis récupération ; test Windows réel de bout en bout |
| Snapshot fidèle, jamais réécrit | `current` reconnu et relu en entier sous verrou ; copie relue depuis le disque et comparée au receipt de `current` ; porte de provenance avant toute approbation ; aucune écriture sous un snapshot publié ; domaines séparés, une anomalie des snapshots ne bloque jamais `sync` | Fautes injectées : `current` changé pendant la copie, receipt source altéré ou disparu, copie altérée ; crash avant et après chaque appel, puis récupération ; test croisé des deux domaines ; test Windows réel de bout en bout |
| Restauration isolée, coffre jamais écrit | Snapshot authentifié par une lecture ciblée, contenu relu en entier ; seule session d'écriture ouverte sur la zone ; zone créée vide ou marquée du même jeu, jamais adoptée ; copie hachée, relue depuis la zone, publiée par renommage ; récupération sans le coffre | Trace des lectures et des sessions sur faux ; crash avant et après chaque appel, puis récupération ; binaire depuis un coffre sous ACL de refus d'écriture, empreinte du coffre identique ; tests Windows réels de bout en bout, fichier ouvert, processus tué |
| Zone distincte, sauf alias d'un dossier ([S-02](#chantiers-de-sécurité-ouverts)) | Coffre, installation, sources et chemins sauvegardés comparés par segments puis par identité depuis la racine de chaque lettre, et non du volume comme l'exige ADR 0006 §11 ; aucun ancêtre coffre ou zone sous cette racine ; identité inobservable ou lettre sans volume : refus. Une lettre qui désigne un sous-dossier d'une référence n'est pas détectée | Faux à deux lettres pour les mêmes nœuds ; alias `subst` réel sur Windows, égal à une source puis au coffre, à la création et à la récupération. Alias réel d'un sous-dossier : restauration publiée dans la source ou le coffre, S-02 |
| Restauration remise, jamais suivie | Après `published`, les noms d'une restauration sont observés par `Lstat` et par l'identité d'un dossier ordinaire seulement, jamais ouverts, listés, suivis ni déplacés ; classement informatif, jamais bloquant | Objets de tout type sous ces noms sur faux ; jonction réelle vers un dossier témoin, témoin identique |
| Interruption récupérable, jamais devinée | Journal validé par renommage avant chaque action, contrat strict des champs, récupération explicite selon les tableaux d'ADR 0006 ; ambiguïté ou enveloppe épuisée : code 3 sans écriture | Crash avant et après chaque appel au coffre, puis récupération ; crash pendant la récupération ; processus tué pendant le staging |
| Aucun lien suivi, parents compris | Contrôle de chaque composant depuis la racine de la lettre, qui n'est celle du volume que sans alias : au-dessus de la cible d'un alias, rien n'est contrôlé ([S-02](#chantiers-de-sécurité-ouverts)) ; ouverture Windows avec `FILE_FLAG_OPEN_REPARSE_POINT` et contrôle des attributs sur le handle ; refus des placeholders | Faux système de fichiers sur trois OS ; vraies jonctions Windows dans la portée, en ancre, et substituées après `Lstat` ; lien symbolique relatif qu'`os.Root` seul aurait suivi |
| Pas de substitution pendant l'inspection | Dossiers traversés gardés ouverts sans partage de suppression jusqu'à la fin | Renommage refusé tant que la portée est ouverte, accepté après fermeture |
| Fichier inventorié = fichier lu | Identité, taille et date contrôlées avant, à l'ouverture et après la lecture | Remplacement, croissance, modification et disparition injectés à des instants précis |
| Manifeste non ambigu | Décodage strict, grammaire Windows canonique | [ADR 0002](adr/0002-manifest-v1-strict-yaml.md) et ses tests |
| Plan sans écriture | Le plan ne reçoit aucun port d'écriture ; le coffre n'est pas créé | Binaire `sync` lancé deux fois, écriture refusée par ACL ou `chmod` sur tout l'arbre, empreinte identique ; coffre absent après le plan |
| Coffre distinct sur une même lettre, jamais adopté | Comparaison par segments sans casse, puis par identité, avec l'installation, seulement si les deux portent la même lettre : sinon, aucune comparaison ([S-02](#chantiers-de-sécurité-ouverts)) ; dossier non vide sans marqueur refusé | Tests de la feature, nom court 8.3 compris ; test Windows de bout en bout. Alias réel, côté coffre ou côté manifeste : coffre publié dans l'installation, S-02 |
| État du coffre jamais deviné | Journal, receipts et marqueur relus strictement ; toute entrée inexpliquée, opération en attente ou ambiguïté arrête le plan en code 3 | Tests de classement exhaustifs d'[ADR 0006](adr/0006-publication-recovery-protocol.md) dans `entities/backup` |

Limites assumées :

- L'inventaire est vérifié au mieux, **pas atomique** : une modification qui
  préserverait taille, date et identité d'un fichier passerait inaperçue.
- Lire peut mettre à jour la date de dernier accès selon les réglages NTFS.
- Un snapshot est relu avant sa publication, pas entre sa relecture et son
  receipt : une écriture d'un autre programme dans cette fenêtre n'est
  détectée que si un plan suivant relit son contenu, tant qu'il est le
  candidat `up to date` ; il est alors dégradé. Après une rotation, il n'est
  plus relu en B3.
- Une restauration remise n'est plus vérifiée : une modification à
  l'intérieur n'est pas signalée. Un snapshot réécrit délibérément avec son
  receipt et son journal serait restauré fidèlement : la vérification refuse
  une altération accidentelle ou partielle, pas une falsification cohérente.
- Les distinctions d'une zone portent sur les chemins tels qu'ils se
  résolvent aujourd'hui ; une restauration est refusée tant qu'une lettre de
  référence n'a pas de volume. B4 n'est pas une reprise après sinistre
  complète.
- **Défaut ouvert, chantier [S-02](#chantiers-de-sécurité-ouverts).** Une
  lettre de lecteur qui désigne un dossier contourne les distinctions de
  `sync`, `snapshot` et `restore`. Rien n'est remplacé ni supprimé, mais un
  coffre ou une zone peut être créé là où Savewarden ne doit jamais écrire.
  Interdiction opérationnelle tant que S-02 est ouvert.
- **Risque résiduel, chantier [S-01](#chantiers-de-sécurité-ouverts).** Les
  sessions de `platform/vault` créent et renomment **par chemin complet**.
  Leurs épingles interdisent de renommer les dossiers déjà ouverts, mais ne
  neutralisent pas la réattribution d'une lettre de lecteur. `restore` refait
  les distinctions une fois sa session ouverte, avant toute écriture : une
  lettre réattribuée entre le plan et l'ouverture est refusée, ce que prouve
  un test Windows réel avec `subst`. Une lettre réattribuée **pendant**
  qu'une session est ouverte, après ce contrôle et avant une écriture, n'est
  pas exclue, pour `restore` comme pour `sync` et `snapshot` : aucune
  garantie technique ne l'empêche. Acceptation provisoire, ratifiée par le
  propriétaire le 4 novembre 2026, limitée à la qualification interne de B4
  sur volumes stables, sans lettre ni alias modifié pendant une opération ;
  durcissement obligatoire avant toute diffusion publique (ADR 0006 §11).
- Le garde statique ne prouve pas l'absence de tout effet indirect ; les
  tests dynamiques couvrent le comportement livré, sur leurs fixtures.
- Sous Unix, l'adaptateur sert aux tests du contrat ; un échange suivi d'une
  restauration d'un dossier parent entre deux contrôles reste possible.
  `inspect` refuse de toute façon les chemins Windows sur ces systèmes.

Les tests restent dans des dossiers temporaires aux données synthétiques.
L'agent n'accède à `D:\Ga`, `D:\Savewarden` ou `E:\GMS` qu'avec une
autorisation explicite et nominative du propriétaire ; le pilote réel est
exécuté par lui.

## Invariants exigés avant les mutations

1. Source de vérité sélectionnée explicitement par l'utilisateur. Aucune
   heuristique fondée sur une ancienne copie, un nom ou une date.
2. Canonicaliser les chemins et rejeter `..`, évasion et recouvrement entre
   source, destination et snapshots. Détecter symlinks/jonctions/reparse points
   et ne pas les suivre par défaut, y compris dans les parents résolus.
3. Ne jamais exécuter les fichiers observés ou du code issu d'un manifeste.
4. Préparer une destination nouvelle/staging ; copier et vérifier les mêmes
   ensembles de paths relatifs, tailles et SHA-256 avant publication.
5. Une modification de source pendant copie est une erreur ; l'opération
   n'est pas publiée. Un échec partiel reste identifiable comme incomplet.
6. Pas de suppression implicite, `/MIR`, mutation de `Ga`, écrasement de
   destination non vide, relocation, `SavePath` modifié ou restauration en place.
7. Snapshot validé = immuable. Ne jamais réécrire ou supprimer un snapshot
   approuvé, notamment `20261009T183208-cacd3cae` du pilote.

B1 et B2a implémentent les invariants 1 à 3 pour la lecture : source
déclarée par l'utilisateur, chemins canoniques sans `..` ni chevauchement,
coffre distinct de l'installation, liens jamais suivis, rien d'exécuté. B2b
et B2c implémentent les invariants 4 à 6 pour la première publication et
la rotation, selon [ADR 0006](adr/0006-publication-recovery-protocol.md).
B3 implémente le 7 avec la commande [`snapshot`](commands/snapshot.md),
selon ADR 0006 §10 : un snapshot publié n'est jamais rouvert en écriture,
déplacé, remplacé ni supprimé par Savewarden, et aucun snapshot créé hors de
Savewarden n'est adopté. Cette garantie est **protocolaire, pas matérielle** :
ce n'est pas un stockage WORM, et un autre programme peut toujours modifier
un snapshot. Savewarden le signale alors comme dégradé, sans jamais le
réparer. B4 applique les invariants 2, 4, 6 et 7 à la commande
[`restore`](commands/restore.md), selon ADR 0006 §11 : une restauration va
dans un dossier séparé, jamais en place, ne restaure qu'un snapshot approuvé
et vérifié en entier, et ne touche jamais au coffre.

## Windows et remplacement de current

Windows n'offre pas d'échange atomique de deux répertoires, et `os.Rename`
remplace une cible existante. [ADR 0006](adr/0006-publication-recovery-protocol.md),
référence de sécurité de toute publication et de toute récupération, fixe le
protocole : créations exclusives, `MoveFileEx` sans drapeau, journal
write-ahead vérifiable, fenêtre d'absence de `current` reconnue, rollback par
défaut, récupération explicite dans une enveloppe bornée, fail-closed sur
toute ambiguïté. Ses hypothèses Windows ont été confrontées à des tests réels
avant le code qui en dépend. Le verrou `sync.lock` n'exclut qu'une autre
opération Savewarden : jeu, launcher et tout outil qui touche le coffre
doivent être fermés pendant une synchronisation. Un coffre copié ou déplacé
change les identités NTFS sans altérer les données : il est refusé en code 3
sans écriture, et aucune identité n'est adoptée ; sa portabilité relève d'une
procédure future.

## Récupération prévue

Avant tout write, conserver la source originale et l'état actuel, fournir un
plan/dry-run et attendre l'autorisation du pilote. Si vérification ou
publication échoue, ne pas supprimer la source ni l'ancien `current` ;
conserver l'incomplet identifié et diagnostiquer depuis le receipt. Une
restauration crée un dossier distinct et vérifié ; elle ne change jamais la
sauvegarde active du jeu. Reprendre des fichiers dans le jeu reste un geste
manuel, décrit dans la page du [receipt de restauration](restore-receipt-v1.md#reprendre-les-fichiers-dans-le-jeu).
Le rollback du code B4 est neutre pour les coffres, qu'aucune restauration
n'écrit ; une zone reste un dossier ordinaire, lisible par `inspect`.

En B2b comme en B2c, le rollback du code consiste à reverter la PR du lot
sur la release branch. Un binaire B2b refuse un coffre qui a connu une
rotation : son contrat strict ne connaît pas `current_identity`, le journal
est `journal-ambiguous`, code 3, et rien n'est écrit, plan comme `--apply` ;
vérifié sur un coffre synthétique avec un binaire construit depuis
`54b1c02`. Les données y restent lisibles par `inspect`. Les coffres déjà
écrits ne sont jamais réécrits : une génération publiée ou conservée, un
staging conservé ou un journal clos restent en place ; un nouveau coffre est
toujours possible.

## Chantiers de sécurité ouverts

### S-01 — Écritures indépendantes des lettres de lecteur

| Champ | Valeur |
| --- | --- |
| Statut | **Ouvert**. Ni défaut éditorial, ni résolu |
| Ouvert le | 4 novembre 2026 : revue de sécurité de B4, P1-02, et revues ciblées de `5184d51` et `0b7bfcf` |
| Risque | Une lettre de lecteur réattribuée pendant qu'une session d'écriture est ouverte peut faire porter une création ou un renommage ailleurs que là où le dernier contrôle l'a vue, une source par exemple. Les épingles ne l'empêchent pas |
| Portée | Toutes les sessions de `platform/vault` : `sync`, `snapshot` et `restore` |
| Déjà fermé | Pour `restore`, la réattribution entre le plan et l'ouverture de la session : `TestRestoreRefusesALetterRemappedBeforeTheSession`, sur Windows réel |
| Acceptation provisoire | Qualification interne de B4 seulement : volumes NTFS locaux stables, aucune lettre ni alias `subst` créé, retiré ou réattribué pendant une opération ; pilote interne contrôlé après ses autorisations propres. Ratifiée par le propriétaire le 4 novembre 2026, avec l'amendement d'ADR 0006 §11. Elle ne prouve pas l'innocuité du risque : elle borne les conditions dans lesquelles B4 continue |
| Interdit tant que S-01 est ouvert | Présenter Savewarden comme protégeant contre une réattribution concurrente ; diffuser une version publique |
| Sortie attendue | Amendement d'ADR 0005 ; primitives d'écriture qui ne résolvent plus de lettre une fois la session ouverte ; requalification de B2, B3 et B4 ; test Windows réel qui réattribue une lettre pendant une session ouverte et montre qu'aucune écriture ne sort de la cible contrôlée ; nouveau gate de sécurité |
| Pistes, à évaluer | Créations et renommages relatifs aux handles des dossiers épinglés plutôt qu'à un chemin complet ; aucune n'est vérifiée |
| Gate | **Bloquant pour toute diffusion publique.** Non bloquant pour la qualification interne de B4, dans les conditions ci-dessus |

### S-02 — Lettre de lecteur qui désigne un dossier

| Champ | Valeur |
| --- | --- |
| Statut | **Ouvert**. Défaut du code livré par B2, B3 et B4 ; ni éditorial, ni résolu |
| Ouvert le | 6 novembre 2026 : revue indépendante du plan B5 v0.1, constat P0, prouvé le même jour sur Windows réel |
| Risque | Une lettre de lecteur qui désigne un dossier, et non la racine d'un volume, alias `subst` ou `DefineDosDevice` par exemple, contourne les distinctions : un coffre ou une zone de restauration peut être créé dans l'installation, dans une source ou dans le coffre. Rien n'est remplacé ni supprimé, les créations restent exclusives ; mais Savewarden écrit là où il promet de ne jamais écrire |
| Cause | `restore` construit ses chaînes d'identités depuis la racine de la lettre, et non du volume comme l'exige ADR 0006 §11, et ne cherche les marqueurs de coffre ou de zone que sous cette racine : une zone désignée par l'alias d'un sous-dossier d'une référence échappe aux deux contrôles. `sync` et `snapshot` ne comparent le coffre et l'installation que s'ils portent la même lettre : avec deux lettres, aucune comparaison n'a lieu, quel que soit le côté de l'alias |
| Prouvé sur Windows réel | Binaire de `a59d787`, vraies lettres `subst` vers des dossiers temporaires synthétiques, retirées ensuite. `restore --apply` publie dans une source, la zone étant désignée par l'alias d'un sous-dossier de la source, et dans le coffre, par l'alias de son dossier `games`. `sync --apply` publie un coffre dans l'installation, désigné par l'alias d'un sous-dossier de l'installation, ou sur la vraie lettre quand le manifeste nomme l'installation par l'alias de son parent. Les témoins, même lettre ou alias égal à la référence, sont refusés |
| Constaté sur faux seulement | Zone publiée dans le dossier du jeu, dans le coffre. Coffre créé dans une source : la copie s'arrête en `changed-during-sync`, coffre et copie inachevée restent dans la source |
| Non sondé | `snapshot` et les récupérations, qui partagent ces contrôles. D'après le code : une zone créée sous `games\` du coffre a la forme du dossier d'un autre jeu, et `sync` ne la signale pas ; le contrôle des liens part aussi de la racine de la lettre, et ne voit pas les composants au-dessus de la cible d'un alias |
| Pilotes réels | Pilote B4 : préflight et contrôles finaux sans aucun alias `subst` ([rapport B4](validation/b4.md)). Le rapport du pilote B3 ne consigne pas les lettres de lecteur |
| Interdiction opérationnelle | Tant que S-02 est ouvert : aucun coffre ni zone désigné par un alias de lettre ; aucun alias de lettre non plus dans les chemins du manifeste, installation et sources. Chaque chemin commence par une lettre qui désigne la racine d'un volume ; `subst`, sans argument, ne liste aucune des lettres employées |
| Interdit tant que S-02 est ouvert | Présenter les distinctions comme indépendantes des lettres de lecteur ; diffuser une version publique |
| Sortie attendue | Correction dans B5a, avec S-01, par décision du propriétaire du 6 novembre 2026 : distinctions depuis la racine du volume réel, ou refus de toute lettre qui n'en est pas une ; plus de raccourci « même lettre » dans `sync`, `snapshot` et les récupérations ; tests Windows réels avec un alias d'un sous-dossier, côté coffre ou zone et côté manifeste, pour `sync`, `snapshot`, `restore` et leurs récupérations ; requalification de B2, B3 et B4 |
| Gate | **Bloquant pour toute diffusion publique.** Pour la qualification interne, seulement dans le respect de l'interdiction ci-dessus |

## Preuves et confidentialité

CI : données exclusivement synthétiques. Tests sur les vraies sauvegardes :
pilote séparé opt-in, lecture/dry-run avant toute écriture explicitement
autorisée. Le nombre de fichiers et le snapshot Odyssey sont des constats
rapportés par le plan, sans revalidation du disque dans B0.

Ne pas committer saves, identifiants de comptes, secrets, `CPY.ini` ou receipts
réels. `.gitignore` exclut les répertoires importés accidentellement, mais un
chemin absolu hors dépôt n'est pas couvert par Git : ne jamais le copier dans
le workspace. Inspecter les fichiers suivis avant un commit/public push.

Une copie sur le **même disque physique** n'est pas une sauvegarde hors-disque.
Les copies externes/chiffrées et leur politique de rétention seront des
décisions distinctes ; elles ne sont pas promises dans le bootstrap.

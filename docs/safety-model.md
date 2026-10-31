# Modèle de sécurité

## Garantie actuelle (B3)

`inspect` lit un manifeste désigné par l'utilisateur et les fichiers des
sources qu'il déclare. `sync` et `snapshot`, en mode plan, lisent en plus le
coffre désigné par `--vault`, s'il existe ; `snapshot` ne lit pas le contenu
des sources. `--apply` et `--recover --apply` écrivent, mais **seulement dans
ce coffre**, et seulement par créations exclusives et renommages qui ne
remplacent rien ; `snapshot` n'écrit que sous `snapshots\`, hormis le verrou
vide que `sync` créerait aussi. Aucune découverte d'installation, création
de lien ou mutation de configuration ; aucune écriture dans une source.

| Garantie | Mécanisme | Preuve |
| --- | --- | --- |
| Écritures confinées, aucune exécution | Seul `platform/vault` écrit, avec les API nommées par [ADR 0005](adr/0005-write-allowed-package.md) ; garde statique contre toute autre API d'écriture, d'exécution, de réseau et de chargement dynamique, dans tous les packages | `readonly_test.go` et ses listes attendues indépendantes ; `inspect` et le plan lancés deux fois avec l'écriture refusée par ACL ou `chmod`, empreinte identique ; `--apply` avec l'écriture refusée sur l'installation, source identique |
| Rien n'est remplacé ni supprimé | `CREATE_NEW`, `CreateDirectory`, `MoveFileEx` avec le littéral `0` ; aucun fichier existant rouvert en écriture ; temporaires jamais supprimés | Tests Windows réels : création et renommage refusés sur cible existante, erreur 183 ; garde AST des drapeaux |
| Publication vérifiée | Copie hachée, source inventoriée deux fois, staging relu depuis le disque, identité du staging contrôlée avant et après le renommage | Fautes injectées : source changée, écriture refusée, octet altéré, course sur le renommage |
| Rotation sans perte | `current` reconnu par receipt, journal publié et identité, relu en entier sous verrou et juste avant le retrait ; identité fixée par `planned` ; génération retirée relue avant la promotion ; restauration seulement si elle est intacte ; toute anomalie après le retrait : code 3, rien déplacé | Fautes injectées à chaque fenêtre : contenu ou identité changés, `current` réapparu, génération retenue altérée ; crash avant et après chaque appel, puis récupération ; test Windows réel de bout en bout |
| Snapshot fidèle, jamais réécrit | `current` reconnu et relu en entier sous verrou ; copie relue depuis le disque et comparée au receipt de `current` ; porte de provenance avant toute approbation ; aucune écriture sous un snapshot publié ; domaines séparés, une anomalie des snapshots ne bloque jamais `sync` | Fautes injectées : `current` changé pendant la copie, receipt source altéré ou disparu, copie altérée ; crash avant et après chaque appel, puis récupération ; test croisé des deux domaines ; test Windows réel de bout en bout |
| Interruption récupérable, jamais devinée | Journal validé par renommage avant chaque action, contrat strict des champs, récupération explicite selon les tableaux d'ADR 0006 ; ambiguïté ou enveloppe épuisée : code 3 sans écriture | Crash avant et après chaque appel au coffre, puis récupération ; crash pendant la récupération ; processus tué pendant le staging |
| Aucun lien suivi, parents compris | Contrôle de chaque composant depuis la racine du volume ; ouverture Windows avec `FILE_FLAG_OPEN_REPARSE_POINT` et contrôle des attributs sur le handle ; refus des placeholders | Faux système de fichiers sur trois OS ; vraies jonctions Windows dans la portée, en ancre, et substituées après `Lstat` ; lien symbolique relatif qu'`os.Root` seul aurait suivi |
| Pas de substitution pendant l'inspection | Dossiers traversés gardés ouverts sans partage de suppression jusqu'à la fin | Renommage refusé tant que la portée est ouverte, accepté après fermeture |
| Fichier inventorié = fichier lu | Identité, taille et date contrôlées avant, à l'ouverture et après la lecture | Remplacement, croissance, modification et disparition injectés à des instants précis |
| Manifeste non ambigu | Décodage strict, grammaire Windows canonique | [ADR 0002](adr/0002-manifest-v1-strict-yaml.md) et ses tests |
| Plan sans écriture | Le plan ne reçoit aucun port d'écriture ; le coffre n'est pas créé | Binaire `sync` lancé deux fois, écriture refusée par ACL ou `chmod` sur tout l'arbre, empreinte identique ; coffre absent après le plan |
| Coffre distinct, jamais adopté | Comparaison par segments sans casse, puis par identité, avec l'installation ; dossier non vide sans marqueur refusé | Tests de la feature, nom court 8.3 compris ; test Windows de bout en bout |
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
réparer.

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
restauration B4 crée un dossier distinct et vérifié ; elle ne change jamais
la sauvegarde active du jeu.

En B2b comme en B2c, le rollback du code consiste à reverter la PR du lot
sur la release branch. Un binaire B2b refuse un coffre qui a connu une
rotation : son contrat strict ne connaît pas `current_identity`, le journal
est `journal-ambiguous`, code 3, et rien n'est écrit, plan comme `--apply` ;
vérifié sur un coffre synthétique avec un binaire construit depuis
`54b1c02`. Les données y restent lisibles par `inspect`. Les coffres déjà
écrits ne sont jamais réécrits : une génération publiée ou conservée, un
staging conservé ou un journal clos restent en place ; un nouveau coffre est
toujours possible.

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

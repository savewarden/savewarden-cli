# Modèle de sécurité

## Garantie actuelle (B1)

B1 ajoute une seule capacité : **lire**. `inspect` lit un manifeste désigné
par l'utilisateur et les fichiers des sources qu'il déclare. Aucune
découverte d'installation, création de lien ou mutation de configuration.

| Garantie | Mécanisme | Preuve |
| --- | --- | --- |
| Aucune écriture applicative ni exécution | Aucun appel d'écriture dans le code ; garde statique contre les API d'écriture, d'exécution, de réseau et de chargement dynamique | `readonly_test.go` ; binaire lancé deux fois avec environnement isolé, écriture refusée par ACL ou `chmod`, empreinte complète de l'arborescence identique |
| Aucun lien suivi, parents compris | Contrôle de chaque composant depuis la racine du volume ; ouverture Windows avec `FILE_FLAG_OPEN_REPARSE_POINT` et contrôle des attributs sur le handle ; refus des placeholders | Faux système de fichiers sur trois OS ; vraies jonctions Windows dans la portée, en ancre, et substituées après `Lstat` ; lien symbolique relatif qu'`os.Root` seul aurait suivi |
| Pas de substitution pendant l'inspection | Dossiers traversés gardés ouverts sans partage de suppression jusqu'à la fin | Renommage refusé tant que la portée est ouverte, accepté après fermeture |
| Fichier inventorié = fichier lu | Identité, taille et date contrôlées avant, à l'ouverture et après la lecture | Remplacement, croissance, modification et disparition injectés à des instants précis |
| Manifeste non ambigu | Décodage strict, grammaire Windows canonique | [ADR 0002](adr/0002-manifest-v1-strict-yaml.md) et ses tests |

Limites assumées :

- L'inventaire est vérifié au mieux, **pas atomique** : une modification qui
  préserverait taille, date et identité d'un fichier passerait inaperçue.
- Lire peut mettre à jour la date de dernier accès selon les réglages NTFS.
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

B1 implémente les invariants 1 à 3 pour la lecture : source déclarée par
l'utilisateur, chemins canoniques sans `..` ni chevauchement, liens jamais
suivis, rien d'exécuté. Les invariants 4 à 7 concernent les écritures et
restent bloquants pour B2 à B5.

## Windows et remplacement de current

Ne pas supposer que `os.Rename` remplace atomiquement un répertoire non vide
sous Windows. B2 nécessite un ADR distinct pour conflits, conservation de
l'ancien `current`, rollback, interruptions et récupération après crash.
Les fichiers verrouillés, permissions, collisions de casse, junctions et
changements concomitants auront des tests Windows avant writes réels.

## Récupération prévue

Avant tout write, conserver la source originale et l'état actuel, fournir un
plan/dry-run et attendre l'autorisation du pilote. Si vérification ou
publication échoue, ne pas supprimer la source ni l'ancien `current` ;
conserver l'incomplet identifié et diagnostiquer depuis le receipt. Une
restauration B4 crée un dossier distinct et vérifié ; elle ne change jamais
la sauvegarde active du jeu.

En B1, le rollback consiste à reverter la PR du lot sur la release branch :
`inspect` n'écrit rien, il n'existe aucun état de sauvegarde à récupérer.

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

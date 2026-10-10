# Modèle de sécurité

## Garantie actuelle

B0 fournit uniquement aide/version, sans lecture ni écriture de sauvegardes,
sans manifeste, découverte d'installation, création de lien ou mutation de
configuration. Tous les tests restent dans le dépôt ou des dossiers temporaires
contenant des données synthétiques. Aucun accès à `D:\Ga`, `D:\Savewarden` ou
`E:\GMS` pendant ce bootstrap.

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

Ces règles sont bloquantes pour B1–B5 mais ne constituent pas encore des
fonctionnalités implémentées dans B0.

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

Dans B0, le rollback consiste à retirer le binaire généré ou réviser les
fichiers du bootstrap : aucun état de sauvegarde à récupérer.

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

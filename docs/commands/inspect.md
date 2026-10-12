# `savewarden inspect`

Inventorie, en lecture seule, les dossiers de sauvegarde déclarés par un
[manifeste v1](../manifest-v1.md).

```powershell
savewarden inspect --manifest 'D:\Savewarden\games\mon-jeu\manifest.yaml'
```

`--manifest` est obligatoire. Aucun autre argument n'est accepté. Fermez le
jeu et son launcher avant l'inspection.

## Garanties

- **Aucune écriture applicative.** Rien n'est créé, modifié, déplacé,
  supprimé ni exécuté : ni fichier de configuration, ni cache, ni journal,
  ni fichier temporaire. Le rapport ne va que sur la sortie standard.
- **Aucun lien suivi.** Liens symboliques, jonctions, autres points
  d'analyse (*reparse points*) et placeholders hors ligne ou cloud sont
  signalés, jamais traversés ni lus. La règle s'applique à chaque dossier
  depuis la racine du volume jusqu'à la source, puis à chaque entrée.
- **Dossiers épinglés.** Sous Windows, chaque dossier traversé reste ouvert
  sans droit de suppression jusqu'à la fin : il ne peut pas être renommé ni
  remplacé par une jonction pendant l'inspection.
- **Fichiers vérifiés autour de la lecture.** Un fichier n'est inventorié que
  si son identité, sa taille et sa date de modification sont restées
  identiques entre la description initiale, l'ouverture et la fin de lecture.
- **Sortie stable.** Deux exécutions sur un état inchangé produisent la même
  sortie, octet pour octet : aucune date d'exécution n'y figure.

## Sortie

Texte anglais UTF-8, sans couleur, fins de ligne LF.

```text
manifest: D:\Savewarden\games\synthetic-game\manifest.yaml
game: synthetic-game (Synthetic Game)
installation: D:\Games\Synthetic Game

source saves (save, required): D:\Games\Synthetic Game\saves
  status: inventoried
  files: 2  bytes: 6
  7692c3ad3540bb803c020b3aee66cd8887123234ea0c6e7143c0add73ff431ed  3  2026-10-09T16:32:08.1234567Z  1.save
  3fc4ccfe745870e2c0d99f71f30ff0656c8dedd41cc1d7d3d376b0dbe685e2f3  3  2026-10-09T16:32:08.1234567Z  Profil été/2.save

findings: none
```

- Une ligne par fichier : SHA-256, taille en octets, date de modification UTC
  et chemin relatif à la source, avec `/` comme séparateur, triés octet par
  octet.
- `status` vaut `inventoried`, `missing` ou `refused`. Une source refusée ou
  absente n'a pas de liste de fichiers.
- Un nom qui pourrait masquer ou imiter une ligne est cité entre guillemets
  avec échappements, par exemple un retour à la ligne ou une espace finale.

Quand il y a des anomalies :

```text
findings: 1 blocking, 1 info
  blocking  link-not-followed  saves  jn  junction or reparse point; not followed
  info  source-missing  config  D:\Games\Synthetic Game\config  does not exist
```

| Code | Sévérité | Signification |
| --- | --- | --- |
| `source-missing` | bloquante si `required: true`, sinon information | La source ou un dossier parent n'existe pas, ou le volume est absent |
| `source-not-directory` | bloquante | Un élément du chemin de la source n'est pas un dossier |
| `link-not-followed` | bloquante | Lien, jonction, point d'analyse ou placeholder non suivi |
| `special-file` | bloquante | Périphérique, tube nommé ou autre fichier spécial non lu |
| `unreadable` | bloquante | Accès refusé ou fichier verrouillé par un autre processus |
| `changed-during-read` | bloquante | Fichier modifié ou disparu pendant l'inspection |
| `replaced-before-read` | bloquante | Fichier remplacé par un autre entre sa description et son ouverture |
| `case-collision` | bloquante | Deux noms qui ne diffèrent que par la casse |

## Codes de sortie

Décidés par [ADR 0003](../adr/0003-inspect-exit-codes.md).

| Code | Signification |
| --- | --- |
| `0` | Inventaire complet, aucune anomalie bloquante ; des informations sont possibles |
| `1` | Invocation invalide, manifeste invalide, volume inutilisable, sources qui se chevauchent, interruption ou sortie impossible à écrire ; un seul diagnostic sur stderr, rien sur stdout |
| `2` | Rapport complet sur stdout avec au moins une anomalie bloquante ; une ligne de synthèse sur stderr |

## Limites

- **Windows seulement.** Sous Linux et macOS, un manifeste valide est refusé
  avec le code 1 : les chemins `D:\...` n'y existent pas.
- **Pas un instantané atomique.** Un changement qui conserverait taille, date
  et identité du fichier pourrait passer inaperçu. B1 donne un inventaire
  vérifié au mieux, pas une preuve de cohérence globale.
- **Date de dernier accès.** Lire un fichier peut mettre à jour cette date
  selon les réglages NTFS ; le contenu n'est jamais modifié.
- **Fichiers OneDrive.** Les fichiers gérés par OneDrive ou d'autres
  fournisseurs cloud sont des points d'analyse : ils sont refusés en B1,
  même s'ils sont présents localement.
- **Fichiers dédupliqués** de Windows Server : Go les traite comme réguliers,
  mais ils portent un point d'analyse ; ils sont refusés.
- **Lecteur `subst` ou réseau.** Un lecteur virtuel `subst` n'est pas
  détecté comme redirection. Les chemins UNC sont refusés par le manifeste.
- **SHA-256 identique** prouve l'identité des octets, pas la capacité du jeu
  à charger la partie.
- **Confidentialité.** La sortie contient des chemins locaux et parfois un
  nom de profil. Anonymisez-la avant tout partage public.

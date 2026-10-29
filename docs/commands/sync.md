# `savewarden sync`

Planifie, en lecture seule, la copie des sources d'un
[manifeste v1](../manifest-v1.md) vers un [coffre](../vault.md).

```powershell
savewarden sync --manifest 'D:\Manifests\mon-jeu.yaml' --vault 'F:\Coffre'
```

`--manifest` et `--vault` sont obligatoires. `--vault` est un chemin Windows
absolu avec lettre de lecteur, selon la grammaire du manifeste : chemins UNC,
relatifs ou avec variables refusés, rien n'est nettoyé ni complété. Fermez le
jeu et son launcher avant de planifier.

**Cette version planifie seulement.** `--apply`, qui copiera et publiera, et
`--recover`, qui reprendra une opération interrompue, arrivent avec les lots
B2b et B2c ; aujourd'hui ce sont des flags inconnus, refusés en code 1. Le
contrat complet de la commande est fixé par
[ADR 0007](../adr/0007-sync-contract-receipt-v1.md).

## Garanties

- **Aucune écriture.** Le plan ne crée, ne modifie, ne déplace, ne supprime
  et n'exécute rien : ni dans les sources, ni dans le coffre, qui n'est même
  pas créé s'il n'existe pas. Le vrai binaire est testé avec l'écriture
  refusée par ACL sur tout l'arbre, empreinte complète inchangée.
- **Aucun lien suivi.** Les sources sont inventoriées par le parcours
  d'[`inspect`](inspect.md) ; le coffre est lu de la même façon. Liens,
  jonctions, points d'analyse et placeholders sont signalés, jamais traversés.
- **Coffre distinct.** Le coffre ne peut ni être l'installation, ni la
  contenir, ni s'y trouver, même sous un autre nom, par exemple un nom court
  8.3.
- **Aucune adoption.** Un dossier non vide qui n'est pas un coffre Savewarden
  est refusé, jamais adopté.
- **Sortie stable.** Deux plans sur un état inchangé sont identiques, octet
  pour octet.

## Sortie

Texte anglais UTF-8, sans couleur, fins de ligne LF. Exemple, chemins fictifs,
coffre sur le volume de la source :

```text
manifest: D:\Manifests\synthetic-game.yaml
game: synthetic-game (Synthetic Game)
vault: D:\Vault (will be created)
destination: D:\Vault\games\synthetic-game\current
volume: not checked by this build
current: none
pending: none
mode: plan only, nothing written
plan: changes

source saves (save, required): D:\Games\Synthetic Game\saves
  status: planned
  files: 2  bytes: 6  added: 2  modified: 0  removed: 0  unchanged: 0
  + 7692c3ad3540bb803c020b3aee66cd8887123234ea0c6e7143c0add73ff431ed  3  2026-10-09T16:32:08.1234567Z  1.save
  + 3fc4ccfe745870e2c0d99f71f30ff0656c8dedd41cc1d7d3d376b0dbe685e2f3  3  2026-10-09T16:32:08.1234567Z  Slot 1/2.save

findings: 0 blocking, 1 info
  info  same-volume  saves  D:\  vault and source share a volume; not an off-disk backup
```

| Ligne | Sens |
| --- | --- |
| `vault` | Chemin et état : `will be created`, `empty, will be initialized`, `initialized` ou `initialization interrupted` |
| `destination` | Emplacement de la génération courante du jeu dans le coffre |
| `volume` | Le type de volume, NTFS local exigé, sera contrôlé à partir de B2b |
| `current` | Opération qui a produit la génération courante, ou `none` |
| `pending` | Opérations interrompues, avec leur dernière étape validée |
| `plan` | `changes`, `up to date`, `blocked` ou `intervention required` |

Chaque fichier est préfixé par `+` ajouté, `~` modifié, `-` retiré ou `=`
inchangé par rapport à la génération courante, puis suivi de son SHA-256, de
sa taille, de sa date de modification UTC et de son chemin relatif à la source.
Un nom qui pourrait imiter une ligne est cité entre guillemets, comme pour
`inspect`.

Quand le coffre demande une intervention, aucune source n'est planifiée et un
bloc `interventions` précède les anomalies :

```text
interventions: 1
  pending-operation  20261105T101500-1a2b3c4d  last committed step publish-intent; recovery needed, --recover is available from B2b
```

## Anomalies

| Code | Sévérité | Signification |
| --- | --- | --- |
| `source-missing` | bloquante si `required: true`, sinon information | Source absente |
| `source-vanished` | bloquante | Source facultative absente alors que la génération courante la contient : une absence n'est jamais une suppression |
| `source-removed-from-manifest` | bloquante | Source de la génération courante qui n'est plus dans le manifeste |
| `files-removed` | bloquante | Fichier ou dossier de la génération courante absent de la source |
| `unportable-name` | bloquante | Nom que Win32 altérerait : point ou espace final, nom réservé, caractère interdit |
| `encrypted-source` | bloquante | Fichier chiffré par EFS, jamais copié en clair |
| `current-modified` | bloquante | La génération courante ne correspond plus exactement à son receipt, n'en a pas, ou le receipt n'a plus de génération |
| `long-path` | information | Chemin au-delà de 259 caractères dans le coffre |
| `same-volume` | information | Coffre et source sur le même volume : ce n'est pas une sauvegarde hors disque |

Les anomalies du parcours d'`inspect` s'appliquent aussi : `link-not-followed`,
`unreadable`, `changed-during-read`, `case-collision`, etc. Une anomalie sans
source porte `-` à sa place.

| Intervention | Signification |
| --- | --- |
| `pending-operation` | Une opération s'est interrompue ; la récupération explicite arrive avec B2b |
| `initialization-interrupted` | La création du coffre s'est interrompue ; seule la récupération explicite pourra la reprendre |
| `unexpected-entry` | Une entrée n'a pas sa place dans le coffre, ou un staging, une génération conservée n'est expliqué par aucun journal ni receipt |
| `journal-ambiguous` | Un journal est incomplet, corrompu ou incohérent, ou deux opérations sont en attente |

Voir le [guide du coffre](../vault.md) pour chacun de ces cas.

## Codes de sortie

| Code | Signification | stderr |
| --- | --- | --- |
| `0` | Plan sans anomalie bloquante, ou déjà à jour | Vide |
| `1` | Invocation, manifeste ou coffre invalide, coffre qui chevauche l'installation, volume indisponible, erreur de lecture, interruption ; ou plan impossible à écrire en entier, auquel cas stdout peut contenir un **préfixe tronqué** | Un diagnostic |
| `2` | Plan complet avec au moins une anomalie bloquante | `savewarden: plan blocked: N blocking finding(s)` |
| `3` | Le coffre demande une intervention avant tout plan | `savewarden: intervention required: <cause> <sujet>` |

Fiez-vous au code de sortie, jamais à la seule présence d'une sortie.

## Limites

- **Windows seulement**, comme `inspect`.
- **Plan seulement.** Rien n'est copié avant B2b.
- **Pas un instantané atomique** : le plan repose sur le même parcours au
  mieux qu'`inspect`.
- **Volume non contrôlé** dans cette version : NTFS local sera exigé pour
  écrire, à partir de B2b.
- **Même disque physique** : deux volumes d'un même disque ne sont pas
  détectés ; un lecteur `subst` n'est pas reconnu comme redirection.
- **Confidentialité.** La sortie contient des chemins locaux ; anonymisez-la
  avant tout partage.

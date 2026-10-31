# `savewarden snapshot`

```text
savewarden snapshot --manifest <chemin> --vault <dossier>                     plan seul, rien n'est écrit
savewarden snapshot --manifest <chemin> --vault <dossier> --apply             copie current, vérifie, publie
savewarden snapshot --manifest <chemin> --vault <dossier> --recover           plan de récupération, rien n'est écrit
savewarden snapshot --manifest <chemin> --vault <dossier> --recover --apply   exécute la récupération
```

Un **snapshot** est une copie vérifiée de la génération `current` d'un jeu,
rangée sous `games\<game-id>\snapshots\` du [coffre](../vault.md). Il fige
l'état **sauvegardé dans le coffre**, pas nécessairement l'état actuel du jeu :
pour figer celui-ci, lancez d'abord [`sync --apply`](sync.md). Le protocole est
fixé par [ADR 0006 §10](../adr/0006-publication-recovery-protocol.md), le
contrat de la commande par [ADR 0008](../adr/0008-snapshot-contract-receipt-v1.md).

`snapshot` ne fonctionne que sous Windows, comme `sync`.

## Garanties

- **Lecture seule sans `--apply`.** Le plan et `--recover` sans `--apply`
  n'écrivent rien, nulle part.
- **Rien n'est lu de la source du jeu ni de son installation** : seules leurs
  identités servent à vérifier que le coffre en reste distinct. Le jeu peut
  rester ouvert.
- **Copie exacte d'une génération reconnue.** `current` doit être reconnu par
  toute sa chaîne de preuve, receipt, journal publié et identité NTFS, et
  égal à son receipt, relu en entier sous verrou. La copie est relue depuis le
  disque et comparée au plan avant toute publication.
- **Provenance recoupée.** Avant d'approuver un snapshot, Savewarden relit le
  receipt de la génération copiée et le compare au plan journalisé, par son
  SHA-256 et son contenu. Ce contrôle vérifie la cohérence de preuves
  conservées dans le même coffre ; il ne protège pas d'une réécriture
  délibérée et coordonnée de ces preuves.
- **Immuable après publication.** Un snapshot publié n'est jamais rouvert en
  écriture, déplacé, remplacé ni supprimé par Savewarden ; les lectures de
  contrôle restent permises. Cette garantie est **protocolaire, pas
  matérielle** : ce n'est pas un stockage WORM, et rien n'empêche un autre
  programme de le modifier.
- **N'écrit que sous `snapshots\`**, à l'exception du fichier de verrou vide
  `sync.lock`, créé comme `sync` le crée s'il manque.
- **Aucune suppression**, pas même d'une copie `.incomplete` abandonnée.

## Disposition

```text
<coffre>\games\<game-id>\snapshots\
  <snap-id>\<source-id>\...              snapshot publié, immuable
  <snap-id>.incomplete\<source-id>\...   copie en cours, échouée ou abandonnée
  receipts\<snap-id>.json                receipt de snapshot
  journal\<snap-id>\NNN-<étape>.rec      journal de l'opération
```

`<snap-id>` est l'identifiant de l'opération de snapshot,
`AAAAMMJJTHHMMSS-xxxxxxxx`, date UTC et huit chiffres hexadécimaux. Le
[receipt de snapshot](../snapshot-receipt-v1.md) décrit le snapshot et la
génération qu'il copie.

## Sortie du plan

```text
manifest: D:\Manifests\synthetic-game.yaml
game: synthetic-game (Synthetic Game)
vault: F:\Vault (initialized)
volume: NTFS, fixed drive, 67108870 bytes needed
current: 20261105T101500-1a2b3c4d, recognized
pending: none
mode: plan only, nothing written
plan: new snapshot

snapshots: 2 published, 1 degraded, 0 incomplete
  20261101T090000-0a1b2c3d  from 20261030T120000-11223344  2 files  6 bytes
  20261102T090000-4e5f6a7b  from 20261101T120000-55667788  2 files  6 bytes  missing
snapshot: new, from 20261105T101500-1a2b3c4d
files: 2  bytes: 6

findings: 0 blocking, 1 info
  info  snapshot-missing  -  snapshots/20261102T090000-4e5f6a7b  the approved snapshot is absent; it is never recreated
```

Avec un snapshot dégradé, stderr porte aussi `savewarden: warning: 1 degraded
snapshot(s)`, à chaque exécution, récupération comprise, avant la ligne d'état.

| Ligne | Sens |
| --- | --- |
| `current` | Opération de la génération courante ; `recognized` une fois prouvée, contenu et identité ; `none`, ou `not checked, intervention required` |
| `pending` | Opération de snapshot non terminale |
| `plan` | `new snapshot`, `up to date`, `blocked` ou `intervention required` |
| `snapshots` | Snapshots approuvés, dégradés, et copies `.incomplete` conservées. Celles-ci sont comptées par leur nom, jamais parcourues : `size not measured` le rappelle dès qu'il en existe |
| `snapshot` | `new, from <op>` : ce que `--apply` copierait ; `up to date (<snap-id>)` : un snapshot intact de cette génération existe déjà, `--apply` n'écrit rien |

Une seule génération donne un seul snapshot conforme. Après une rotation de
`current`, un nouveau snapshot devient possible ; les anciens restent intacts.

## Snapshots dégradés

À chaque exécution, Savewarden contrôle le receipt et l'identité de chaque
snapshot approuvé, et le plan relit en entier celui qui le rendrait
`up to date` ; une récupération ne relit aucun contenu. Il ne vérifie pas le
contenu de tous les snapshots : ce sera le rôle de `doctor`. Sous le nom d'un
snapshot approuvé, de sa copie ou de son receipt, un fichier ou un lien le
dégrade, sans jamais bloquer.

| Code | Ce qui s'est passé |
| --- | --- |
| `snapshot-missing` | Le snapshot approuvé est absent |
| `snapshot-rename-lost` | Le snapshot est absent et sa copie est restée sous `.incomplete`, avec l'identité publiée : renommage perdu, probablement par une coupure de courant |
| `snapshot-modified` | Le snapshot a une autre identité, ou une copie `.incomplete` à côté de lui ; son receipt est absent, invalide ou différent ; ou son contenu relu diffère |

Un snapshot dégradé ne bloque ni `snapshot` ni `sync`. Il n'est jamais recréé,
déplacé, réparé ni adopté, et ne compte plus pour l'idempotence : un nouveau
snapshot de la même génération devient possible. Il n'est jamais silencieux
non plus : il est compté, nommé, et stderr en avertit. Aucune sortie ne
qualifie l'ensemble des snapshots de sain ou de vérifié.

## Publier : `--apply`

1. Verrou du jeu, pris comme le prend `sync` ; le coffre est relu sous
   verrou, `current` relu en entier contre son receipt, l'espace recontrôlé.
2. Structure `snapshots`, `receipts`, `journal` et dossier d'opération, par
   créations exclusives ; enregistrement `planned`.
3. `current` relu de nouveau en entier, identité relue après ; copie dans
   `<snap-id>.incomplete`, puis identité de `current` relue. Une identité
   changée l'emporte sur un contenu différent : code 3, rien enregistré.
4. Relecture de la copie depuis le disque, comparée au plan, puis son
   identité, jugée d'abord.
5. Contrôle de provenance, puis `publish-intent` et renommage en `<snap-id>`.
6. Contrôle de provenance de nouveau, receipt de snapshot, `published`.

| Ligne d'état | Sens |
| --- | --- |
| `not modified` | Rien écrit, par exemple `up to date` |
| `metadata only; nothing staged or published` | Structure, verrou ou dossier d'opération créés, aucune copie, y compris une opération close avant sa copie |
| `snapshot <snap-id> kept incomplete` | Opération close sans publication, copie conservée |
| `published snapshot <snap-id>` | Snapshot publié et approuvé |
| `rolled back snapshot <snap-id>` | Renommage refusé puis annulé, code 1, ou annulé faute de provenance, code 3 |
| `intervention required for <snap-id>: <cause>` | Code 3 |

Espace exigé : la taille de la génération plus max(64 Mio, 5 %), disponible
pour l'utilisateur, au plan puis sous verrou, avant toute écriture.

## Récupérer : `--recover`

`snapshot --recover` ne s'appuie que sur le domaine des snapshots : il ne lit
jamais `current`, et ni l'état de `current`, ni une opération de `sync` en
attente, ni une anomalie de ce domaine ne le bloquent. Il n'en lit que le
receipt de la génération copiée, au contrôle de provenance. Réciproquement,
`sync` ne lit rien sous `snapshots\`.

| Situation | Action de `--recover --apply` |
| --- | --- |
| Opération arrêtée avant le renommage | `abandoned` ; la copie reste, jamais lue ni déplacée |
| Renommage voulu, non effectué | `rolled-back` |
| Renommage effectué, receipt absent | Snapshot revérifié en entier ; égal et provenance recoupée : receipt reconstruit, `published` ; différent : remis en `.incomplete` ; provenance contradictoire : remis en `.incomplete`, code 3 ; illisible : rien écrit, code 3, à relancer |
| Renommage effectué, receipt validé | Receipt relu contre le plan, snapshot revérifié : `published` ; sinon code 3, rien déplacé |
| Remise en `.incomplete` interrompue | La termine ; un échec est enregistré une fois, code 3 |

Toute autre combinaison est une ambiguïté : code 3, rien écrit ni déplacé, à
chaque exécution, comme une structure partielle, `snapshots\receipts` absent
à côté d'opérations. Elle bloque la création de snapshots, jamais `sync`. Une
opération dont un autre processus tient le verrou est en cours, pas
interrompue : code 1, `another operation is running on this game`.

## Codes de sortie

| Code | Résultat |
| --- | --- |
| `0` | Plan sans anomalie bloquante, `up to date`, snapshot publié, récupération achevée ou sans objet |
| `1` | Invocation, manifeste ou coffre invalide, coffre absent ou vide, chevauchement, verrou occupé ou opération en cours, coffre changé depuis le plan, identifiant déjà employé, erreur, renommage refusé puis annulé |
| `2` | Anomalie bloquante : `no-current`, `current-modified`, `insufficient-space`, `verification-mismatch` |
| `3` | Intervention requise, dans l'un ou l'autre domaine à la création, dans le domaine des snapshots à la récupération ; provenance non recoupée, `source-unverified` |

## Limites

- Un binaire antérieur à B3 refuse en code 3, sans rien écrire, un coffre qui
  contient `snapshots\`.
- Les snapshots et les copies `.incomplete` s'accumulent : aucun n'est
  supprimé.
- Le contenu de tous les snapshots n'est pas revérifié à chaque exécution.
- Le verrou n'exclut qu'une autre opération Savewarden : une écriture d'un
  autre programme dans la copie, entre sa relecture et le receipt du
  snapshot, n'est pas détectée à la publication. Le snapshot apparaît
  dégradé, `snapshot-modified`, quand un plan suivant relit son contenu, tant
  qu'il est le candidat `up to date`.
- Le receipt et le journal de snapshot, comme la sortie de la commande,
  contiennent des chemins locaux et le nom du jeu ; journaux et diagnostics
  contiennent aussi des identités de fichiers NTFS, dont le numéro de série
  du volume. Anonymisez-les avant tout partage.

# `savewarden restore`

```text
savewarden restore --manifest <chemin> --vault <coffre> --snapshot <snap-id> --to <dossier>                     plan seul, rien n'est écrit
savewarden restore --manifest <chemin> --vault <coffre> --snapshot <snap-id> --to <dossier> --apply             copie, vérifie, publie
savewarden restore --manifest <chemin> --to <dossier> --recover                                                 plan de récupération, rien n'est écrit
savewarden restore --manifest <chemin> --to <dossier> --recover --apply                                         exécute la récupération
```

Une **restauration** copie un [snapshot](snapshot.md) approuvé du
[coffre](../vault.md) dans une **zone de restauration** : un dossier isolé,
hors du jeu et hors du coffre, que `restore` crée et marque. Vous reprenez
ensuite vous-même les fichiers dont vous avez besoin. Le protocole est fixé
par [ADR 0006 §11](../adr/0006-publication-recovery-protocol.md), le contrat
de la commande par [ADR 0009](../adr/0009-restore-contract-receipt-v1.md).

`restore` ne fonctionne que sous Windows, comme `sync` et `snapshot`.

## Garanties

- **Lecture seule sans `--apply`.** Le plan et `--recover` sans `--apply`
  n'écrivent rien, nulle part.
- **Le coffre n'est jamais écrit**, pas même son verrou `sync.lock`, qui
  n'est ni ouvert ni observé : une restauration peut tourner pendant un
  `sync` ou un `snapshot` du même jeu, et depuis un coffre protégé en
  écriture.
- **Seul le snapshot désigné est lu dans le coffre**, en entier : son
  journal publié, son receipt et son identité, puis son contenu, relu contre
  son receipt. Rien d'autre du coffre n'est lu ; une anomalie ailleurs dans le
  coffre ne bloque pas la restauration d'un snapshot intact.
- **Jamais dans le jeu, le coffre ou une source.** La zone est comparée à
  l'installation, aux sources du manifeste, aux chemins que le snapshot a
  sauvegardés et au coffre, par nom et par identité NTFS depuis la racine de
  chaque volume, quelle que soit la lettre de lecteur : un alias `subst` ne
  contourne pas le contrôle. Une séparation qui ne peut pas être prouvée est
  refusée.
- **Rien n'est remplacé ni supprimé.** Chaque `--apply` publie une nouvelle
  restauration sous un nouvel identifiant ; un dossier existant non vide
  n'est jamais adopté.
- **Remise à l'utilisateur.** Une restauration publiée vous appartient :
  Savewarden ne la rouvre jamais en écriture, ne la déplace ni ne la
  supprime, et ne la vérifie plus.

## Zone de restauration

```text
<dossier>\
  savewarden-restore.json            marqueur de zone, et ses temporaires .tmp-<k>
  restore.lock                       verrou de la zone, vide, jamais écrit
  receipts\<op>.json                 receipt de chaque restauration publiée
  journal\<op>\NNN-<étape>.rec       journal de chaque opération
  <op>.incomplete\<source-id>\...    copie en cours, échouée ou abandonnée
  <op>\<source-id>\...               restauration publiée, à vous
```

`--to` accepte un dossier **absent** dont le parent existe, un dossier
**vide**, ou une zone déjà marquée **pour le même jeu**. Tout autre dossier est
refusé en code 1, sans rien écrire : un dossier non vide sans marqueur, un
fichier, un lien ou une jonction, un coffre, une zone d'un autre jeu, ou un
dossier situé dans un coffre ou dans une autre zone. Le volume doit être NTFS,
sur un lecteur local fixe ou amovible.

`<op>` est l'identifiant de l'opération de restauration,
`AAAAMMJJTHHMMSS-xxxxxxxx`. Le [receipt de restauration et le marqueur de
zone](../restore-receipt-v1.md) sont décrits à part.

## Sortie du plan

```text
manifest: D:\Manifests\synthetic-game.yaml
game: synthetic-game (Synthetic Game)
vault: F:\Vault
snapshot: 20261102T090000-4e5f6a7b, from 20261101T120000-55667788, verified
destination: G:\Restores\synthetic (restore area), 2 published, 1 incomplete
  20261103T100000-9a8b7c6d  of 20261102T090000-4e5f6a7b  present
  20261103T110000-0a1b2c3d  of 20261101T090000-0a1b2c3d  rename lost
volume: NTFS, removable drive
mode: plan only, nothing written
plan: new restore
restore: new, to G:\Restores\synthetic\<restore-id>
files: 2  bytes: 6  space needed: 67108870 bytes

findings: 0 blocking, 2 info
  info  snapshot-already-restored  -  20261103T100000-9a8b7c6d  this snapshot was already restored in this area; a new restore is still possible
  info  restore-rename-lost  -  20261103T110000-0a1b2c3d  the handed-over restore is still under .incomplete with its published identity, as after a power cut; it is never renamed
```

| Ligne | Sens |
| --- | --- |
| `snapshot` | Snapshot désigné, génération qu'il copie, et verdict : `verified`, ou `missing`, `rename-lost`, `modified` suivi de `not restorable` |
| `destination` | `new restore area`, `empty directory, new restore area` ou `restore area` ; pour une zone, ses restaurations publiées et ses copies `.incomplete`, comptées par leur nom, jamais parcourues |
| Restaurations remises | Une ligne par restauration publiée : identifiant, snapshot restauré, et `present`, `absent`, `rename lost`, `replaced` ou `not observable` |
| `plan` | `new restore`, `blocked` ou `intervention required` |
| `restore` | Ce que `--apply` publierait ; l'identifiant n'est tiré qu'à l'exécution |

## Restaurer : `--apply`

1. Préflight : snapshot authentifié en entier, destination acceptée,
   distinctions, volume, espace.
2. Session ouverte sur la zone, puis distinctions refaites avant toute
   écriture : une lettre de lecteur réattribuée depuis le plan est refusée.
   Zone nouvelle ou vide : dossier créé s'il manque, relu, il doit être
   vide ; marqueur écrit dans un temporaire puis validé par un renommage qui
   ne remplace jamais.
3. Verrou `restore.lock` de la zone ; zone et snapshot relus sous verrou, sans
   relire le contenu ; espace recontrôlé ; identifiant contrôlé.
4. Structure, journal, `planned` ; copie du snapshot dans `<op>.incomplete`,
   chaque fichier haché contre le plan, date conservée ; identité du snapshot
   relue après la copie.
5. Relecture de la copie depuis le disque, contre le plan ; `publish-intent`,
   renommage en `<op>`, receipt, `published`.

| Ligne d'état | Sens |
| --- | --- |
| `not modified` | Rien écrit |
| `metadata only; nothing staged or published` | Zone, marqueur, verrou, structure ou dossier d'opération créés, aucune copie |
| `restore <op> kept incomplete` | Opération close sans publication ; copie conservée sous `<op>.incomplete` |
| `published restore <op>` | Restauration publiée et vérifiée : elle peut être utilisée |
| `rolled back restore <op>` | Renommage refusé puis annulé, code 1 ; ou copie remise en `.incomplete` par la récupération, code 2 |
| `intervention required for <op>: <cause>` | Code 3 ; `the restore area` quand aucune opération n'est en cause |

**N'utilisez pas `<dossier>\<op>` avant la ligne `published restore <op>`.**
Jusque-là, une récupération peut encore le remettre sous `.incomplete`.

Espace exigé : la taille du snapshot plus max(64 Mio, 5 %), disponible pour
l'utilisateur sur le volume de la zone, au plan puis sous verrou.

## Récupérer : `--recover`

`restore --recover` ne lit **jamais le coffre** : il ne le nomme pas, et
refuse `--vault` et `--snapshot`. Il s'appuie sur la zone et sur le manifeste,
qui donne l'installation et les sources dont la zone doit rester distincte.

Une destination absente ou vide n'a rien à récupérer : code 0, sans rien
ouvrir ni écrire, avant même la lecture du manifeste. Une zone marquée avec
seulement sa structure n'a rien à récupérer non plus, code 0, sans session ni
écriture, mais seulement après la lecture du manifeste, exigé, une fois son
marqueur relu strictement et reconnu du jeu du manifeste : un manifeste
invalide, ou un marqueur illisible, de schéma inconnu ou d'un autre jeu,
donne code 1. Sinon, la destination est contrôlée comme à la création, puis :

| Situation | Action de `--recover --apply` |
| --- | --- |
| Initialisation interrompue : seuls des temporaires du marqueur | Nouveau marqueur validé, sans verrou, temporaires conservés |
| Opération arrêtée avant le renommage | `abandoned` ; la copie reste, jamais lue ni déplacée |
| Renommage voulu, non effectué | `rolled-back` |
| Renommage effectué, receipt absent | Restauration revérifiée en entier contre le plan journalisé dans la zone ; égale : receipt reconstruit, `published` ; différente : remise en `.incomplete`, code 2, `verification-mismatch` |
| Renommage effectué, receipt validé | Receipt relu contre le plan, restauration revérifiée : `published` ; sinon code 3, rien déplacé |
| Remise en `.incomplete` interrompue | La termine ; un échec est enregistré une fois, code 3 |

Toute autre combinaison est une ambiguïté : code 3, rien écrit ni déplacé. Une
opération dont un autre processus tient le verrou est en cours, pas
interrompue : code 1, `another restore is running in this area`. Le guide
pas à pas est dans la page du [receipt de restauration](../restore-receipt-v1.md#guide-de-récupération-dune-zone).

## Restaurations successives et remise

Chaque `--apply` publie une nouvelle restauration ; le plan signale celles du
même snapshot, `snapshot-already-restored`, en information. À chaque
exécution, Savewarden observe le nom de chaque restauration remise et sa copie
`.incomplete`, sans jamais les ouvrir, les lister ni les suivre :

| Code | Ce qui s'est passé |
| --- | --- |
| `restore-missing` | La restauration n'est plus sous son nom |
| `restore-rename-lost` | Elle est restée sous `.incomplete` avec son identité publiée, probablement après une coupure : elle n'est jamais renommée |
| `restore-replaced` | Un autre objet porte son nom, fichier, lien ou jonction compris, ou une copie `.incomplete` est à côté d'elle |

Ces informations ne bloquent jamais rien. Une modification **à l'intérieur**
d'une restauration remise n'est pas signalée : son contenu n'est plus lu.

**Copiez hors de la zone** ce dont vous avez besoin ; ne renommez rien et
n'ajoutez rien dans la zone. Un nom d'opération qu'aucun journal n'explique,
par exemple une restauration renommée, rend la zone ambiguë, code 3, pour
toute nouvelle restauration ; une nouvelle zone reste toujours possible. Un
receipt de restauration supprimé a le même effet.

## Codes de sortie

| Code | Résultat |
| --- | --- |
| `0` | Plan sans anomalie bloquante, restauration publiée, récupération achevée ou sans objet |
| `1` | Invocation, manifeste, coffre ou destination invalide ; snapshot introuvable, non publié, non vérifiable ou pas restaurable maintenant ; recouvrement ou séparation non prouvée ; zone d'un autre jeu ; destination non vide sans marqueur ; restauration en cours dans la zone ; zone changée depuis le plan ; erreur ; renommage refusé puis annulé |
| `2` | Anomalie bloquante, rien publié : snapshot dégradé, `insufficient-space` ; avec `--apply`, `changed-during-restore`, `verification-mismatch` |
| `3` | Intervention requise **dans la zone** : opération en attente, initialisation interrompue, entrée imprévue, journal ou receipt ambigus |

Un snapshot encore en cours ou au journal ambigu côté coffre donne code 1,
`is not restorable`, avec un renvoi à `snapshot --recover` : `restore` n'agit
jamais sur le coffre.

## Limites

- **Disque d'origine indisponible.** Tant qu'une lettre de lecteur nommée par
  le manifeste, ou par les chemins que le snapshot a sauvegardés, ne désigne
  aucun volume, la restauration est refusée : la séparation ne peut pas être
  prouvée. Rattacher un volume à cette lettre rend la décision possible, sur
  le chemin tel qu'il se résout aujourd'hui. B4 n'est pas une reprise après
  sinistre complète.
- **Tenez le manifeste à jour** avant de restaurer : les distinctions portent
  sur les chemins tels qu'ils se résolvent aujourd'hui.
- **Lettres de lecteur.** Les distinctions sont faites au plan, refaites une
  fois la session ouverte, avant toute écriture, puis sous verrou. La session
  écrit par chemin : une lettre réattribuée, par `subst` par exemple, pendant
  qu'une restauration s'exécute n'est pas exclue, et ses épingles ne
  l'empêchent pas. C'est un risque résiduel, qui vaut aussi pour `sync` et
  `snapshot`. Ne modifiez pas les lettres de lecteur pendant une
  restauration.
- **Coffre copié vers un autre volume** : les identités NTFS changent, chaque
  snapshot est `snapshot-modified`, la restauration est refusée, sans
  présumer une corruption. Un disque externe déplacé vers un autre ordinateur
  garde ses identités ; c'est une hypothèse sur NTFS, non testée par la CI.
- **Authenticité sans clé** : un snapshot réécrit délibérément avec son
  receipt et son journal serait restauré fidèlement ; la vérification refuse
  une altération accidentelle ou partielle, pas une falsification cohérente.
- **Fichier ouvert** dans `<op>.incomplete`, par un antivirus ou l'Explorateur
  : renommage refusé, `rolled back`, code 1 ; relancez une fois le fichier
  fermé.
- Aucun attribut ni ACL n'est reproduit ; les fichiers restaurés héritent des
  droits de la zone. Les copies `.incomplete` s'accumulent : aucune n'est
  supprimée.
- Le receipt et le journal de restauration, comme la sortie de la commande,
  contiennent des chemins locaux et le nom du jeu ; anonymisez-les avant tout
  partage.

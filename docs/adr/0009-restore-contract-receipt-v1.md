# ADR 0009 — Contrat de `restore`, receipt de restauration v1 et marqueur de zone v1

Date : 1er novembre 2026. Statut : **projet, non accepté**, rédigé avec le
plan B4 v0.2. Aucun code ne l'implémente.

## Contexte

B4 restaure un snapshot approuvé vers un dossier isolé, hors du jeu et hors
du coffre, sans rien remplacer ni supprimer. Le protocole de publication et de
récupération est le §11 d'ADR 0006 : une **zone de restauration** marquée,
créée par `restore`, porte son verrou, son journal, ses receipts et les
copies ; le coffre n'est jamais écrit. Cet ADR fixe ce qu'un utilisateur ou un
script observe : la commande, ses préconditions, sa sortie, ses codes, le
receipt de restauration et le marqueur de zone. Les contrats de `sync` et de
`snapshot` restent ceux d'ADR 0007 et 0008, inchangés.

## Décision

### Commande et modes

```text
savewarden restore --manifest <m> --vault <v> --snapshot <s> --to <d> [--apply]
savewarden restore --manifest <m> --to <d> --recover [--apply]
```

| Mode | Effet |
| --- | --- |
| Plan, par défaut | Affiche l'authentification du snapshot, l'état de la destination et ce que `--apply` ferait ; n'écrit rien |
| `--apply` | Authentifie le snapshot, prépare la zone, copie, vérifie, publie une restauration |
| `--recover` | Affiche l'action de récupération des opérations de la zone ; n'écrit rien |
| `--recover --apply` | Exécute cette récupération |

- `--manifest` et `--to` toujours obligatoires ; `--vault` et `--snapshot`
  obligatoires hors de `--recover` et refusés avec lui, code 1. Aucun défaut,
  aucune variable d'environnement, aucun fichier de configuration.
- `--to` et `--vault` : chemins absolus et propres sous une lettre de
  lecteur, noms Windows contrôlés ; `--snapshot` au format
  `AAAAMMJJTHHMMSS-xxxxxxxx`.
- Le plan et `--recover` sans `--apply` ne reçoivent aucun port d'écriture.
  Aucun mode n'ouvre de session d'écriture sur le coffre.
- Ni restauration partielle, ni restauration en place, ni drapeau de
  politique. Windows seulement.

### Préconditions

**Authentification du snapshot**, plan et `--apply`, par une lecture ciblée
du coffre, sans verrou du coffre (ADR 0006 §11) :

| # | Contrôle | Refus |
| --- | --- | --- |
| 1 | Coffre marqué ; `<vault>`, `games`, `games\<game-id>`, `snapshots`, `snapshots\journal` et `snapshots\journal\<snap-id>` dossiers ordinaires sans point d'analyse | Coffre invalide ou lien : code 1 ; journal du snapshot absent : `snapshot <snap-id> not found`, code 1 |
| 2 | Journal relu strictement, mode snapshot ; ni `games\<game-id>` listé, ni `sync.lock` observé | Non terminal ou ambigu : `snapshot <snap-id> is not restorable: <état>`, code 1, le détail disant si l'opération est en attente ou le journal ambigu et renvoyant à `snapshot` et à `snapshot --recover` ; terminal autre que `published` : `snapshot <snap-id> was not published`, code 1 |
| 3 | `snapshots\receipts` dossier ordinaire ; receipt de snapshot relu strictement (ADR 0008), SHA-256 égal au `receipt_sha256` de `published` ; un receipt reconstruit, `published_at` nul, est valide | Dossier ou receipt absent, invalide ou différent : `snapshot-modified`, code 2 ; lien ou point d'analyse : code 1, comme partout dans la chaîne |
| 4 | `snapshots\<snap-id>` de l'identité publiée ; `snapshots\<snap-id>.incomplete` absent | Classement d'ADR 0008 : `snapshot-missing`, `snapshot-rename-lost` ou `snapshot-modified`, code 2 ; un objet d'une autre identité n'est jamais lu |
| 5 | Contenu relu en entier contre les `sources` du receipt ; identité relue après | `snapshot-modified`, code 2 |

Partage refusé, verrou ou erreur de lecture autre que l'absence :
`snapshot <snap-id> not verifiable: <détail>`, code 1, sans code de
dégradation. Exception écrite : un volume non prêt que l'adaptateur de
lecture rapporte absent donne encore `snapshot-missing`. Un coffre copié vers
un autre volume change les identités : chaque snapshot est
`snapshot-modified`, le diagnostic disant que l'identité diffère de celle
publiée, sans présumer une corruption.

**Destination**, plan et `--apply` : `<dest>` absent avec un parent ordinaire
sans point d'analyse, dossier ordinaire vide, ou zone marquée du même jeu
sans opération non terminale ni ambiguïté ; distinctions d'ADR 0006 §11 avec
ses chemins de référence, coffre, installation, sources du manifeste et
chemins sauvegardés du receipt de snapshot, selon sa règle d'observabilité :
un chemin de référence non observable, ou sur une lettre sans volume, donne
code 1, « separation from `X` cannot be proven ». **Limite** : B4 ne garantit
pas la restauration lorsque le disque d'origine est indisponible ; rattacher
un volume de remplacement à la lettre rend les identités observables sans
prouver l'identité historique de l'ancien disque ;
aucun ancêtre lien, point d'analyse, coffre ou zone ; volume NTFS sur un
lecteur local fixe ou amovible ; espace : taille du snapshot plus max(64 Mio,
5 %), disponible pour l'utilisateur, au plan puis sous verrou.

**Récupération**, `--recover` avec ou sans `--apply`, dans l'ordre d'ADR 0006
§11 : `<dest>` absent, dossier vide ou zone marquée sans objet d'opération :
rien à récupérer, code 0, sans session ni écriture ; `<dest>` non dossier, ou
non vide sans marqueur ni seuls temporaires du marqueur : code 1 ; sinon
manifeste strict, marqueur du même jeu, distinctions et volume refaits sans
le coffre, puis grammaire, journaux et receipts de la zone relus
strictement. Le coffre n'est jamais ouvert. La récupération est indépendante
du coffre et dépendante du manifeste.

**Verrou.** Avec `--apply`, `restore` prend `restore.lock` de la zone par la
même primitive que `sync.lock`, qui le crée vide s'il manque ; il ne l'écrit
ni ne le supprime jamais, et ne le prend que dans une zone marquée : la
reprise d'une initialisation interrompue valide le marqueur sans verrou.
Deux validations concurrentes du marqueur ne peuvent aboutir qu'une fois ; le
perdant sort en code 1, `destination-changed`, sans rien écrire de plus
(ADR 0006 §11). Il
n'ouvre jamais `sync.lock`. Le plan sonde
`restore.lock` en lecture seule avant de déclarer une opération de la zone en
attente.

### Sortie du plan

Texte anglais UTF-8, sans couleur, fins de ligne LF, noms échappés comme
`inspect`, sans date d'exécution : deux plans sur un état inchangé sont
identiques octet pour octet. Ordre des éléments, détaillé dans
`docs/commands/restore.md` :

1. en-tête : `manifest`, `game`, `vault`, `snapshot` et son verdict ;
2. **destination** : nouvelle zone, ou zone existante avec le nombre de ses
   restaurations publiées et de ses `.incomplete`, comptés sur les noms sans
   parcourir les dossiers, puis une ligne par restauration remise :
   identifiant, snapshot restauré, et `present`, `absent`, `rename lost`,
   `replaced` ou `not observable` ;
3. décision : `restore: new, to <dest>\<restore-id>`, avec fichiers, octets
   et espace requis ;
4. les interventions requises, s'il y en a ;
5. les anomalies, comme `inspect`.

En récupération, l'en-tête annonce l'opération en attente de la zone et
l'action exacte que `--apply` exécuterait. Pendant A3 à A6, le plan annonce
« operation `<op>` pending recovery: copy published, receipt missing; do not
use `<dest>\<op>` yet ».

### Codes de sortie

Même sens qu'ADR 0007 : le code décrit **le résultat de la commande**,
jamais à lui seul l'état publié.

| Code | Résultat |
| --- | --- |
| `0` | Plan sans anomalie bloquante ; restauration publiée et vérifiée ; récupération achevée ou sans objet |
| `1` | Invocation, manifeste, coffre ou destination invalides ; snapshot introuvable, non publié, non vérifiable ou pas restaurable maintenant ; recouvrement ou séparation non prouvée ; zone d'un autre jeu ; destination non vide sans marqueur ; verrou de la zone occupé ; zone changée depuis le plan ; identifiant déjà employé ; erreur de lecture ; interruption ; rapport impossible à écrire en entier ; bascule refusée puis annulée |
| `2` | Au moins une anomalie bloquante ; aucune restauration publiée |
| `3` | Intervention requise **dans la zone** : opération en attente, initialisation interrompue, entrée imprévue, journal ou receipt ambigus |

### Ligne d'état de la destination

Avec `--apply` ou `--recover`, dernière ligne de stderr
`savewarden: destination: <état>`. Le coffre n'étant jamais écrit, aucune
ligne d'état du coffre n'est émise.

| État | Sens |
| --- | --- |
| `not modified` | Rien écrit |
| `metadata only; nothing staged or published` | Zone, marqueur, verrou, structure ou dossier d'opération créés, aucune copie |
| `restore <op> kept incomplete` | Opération close sans publication, `failed` ou `abandoned` ; `I` conservé |
| `published restore <op>` | Restauration publiée et vérifiée ; la copie peut être utilisée |
| `rolled back restore <op>` | Bascule refusée puis annulée, code 1 ; ou copie revérifiée différente du plan en récupération, code 2, `verification-mismatch` ; `I` conservé |
| `intervention required for <op>: <cause>` | Code 3 |

Une publication réussie suivie d'un rapport impossible à écrire sort en
code 1 avec `published restore <op>` : le receipt de restauration fait foi.
Tant que cette ligne n'a pas été écrite, `<dest>\<op>` ne doit pas être
utilisé : la récupération peut encore le remettre en `I`.

### Codes stables et causes

| Code | Sévérité | Sens |
| --- | --- | --- |
| `snapshot-missing`, `snapshot-rename-lost`, `snapshot-modified` | bloquante pour `restore` | Classement d'ADR 0008 appliqué au snapshot désigné ; information pour `snapshot`, inchangé |
| `insufficient-space` | bloquante, au plan comme sous verrou | Volume de la zone |
| `changed-during-restore` | bloquante, `--apply` | Snapshot différent du plan, ou remplacé, pendant la copie ; `I` conservé |
| `verification-mismatch` | bloquante, `--apply` | Relecture de `I` différente du plan ; `I` conservé |
| `long-path` | information | Chemin le plus long écrit, celui de `I`, au-delà de 259 unités UTF-16 |
| `snapshot-already-restored` | information | Restauration publiée du même snapshot dans la zone ; une nouvelle restauration reste possible |
| `restore-missing` | information | Restauration remise introuvable sous son nom |
| `restore-rename-lost` | information | Restauration remise restée sous `.incomplete` avec l'identité publiée ; jamais renommée par Savewarden |
| `restore-replaced` | information | Nom d'une restauration remise porté par un autre objet, ou flanqué d'un `.incomplete` |

Causes d'intervention : `pending-operation`, `initialization-interrupted`,
`unexpected-entry`, `journal-ambiguous`. Diagnostics de code 1 sans code
stable : `destination-changed` ; « another restore is running in this
area » ; `snapshot <snap-id> not found`, `was not published`, `not
verifiable`, `is not restorable` ; « separation from `X` cannot be proven ».

### Restaurations remises

Après `published`, la copie appartient à l'utilisateur. Chaque exécution
observe, pour chaque restauration remise, ses noms `P` et `I` par `Lstat` sans
suivi, et l'identité d'un dossier ordinaire seulement ; jamais leur contenu.

| `P` | `I` | Classement |
| --- | --- | --- |
| Dossier ordinaire de l'identité publiée | Absent | `present` |
| Absent | Absent, ou objet observé autre qu'un dossier de l'identité publiée | `restore-missing` |
| Absent | Type ou identité illisible | `not observable`, sans code |
| Absent | Dossier ordinaire de l'identité publiée | `restore-rename-lost` |
| Dossier d'une autre identité, fichier, lien, jonction ou point d'analyse | Quelconque | `restore-replaced` |
| Dossier ordinaire de l'identité publiée | Tout objet présent | `restore-replaced` |
| Type ou identité illisible | Quelconque | `not observable`, sans code |

Ces classements ne bloquent rien et ne déclenchent aucune écriture ; un objet
remis n'est jamais ouvert pour son contenu, listé, épinglé, lu, suivi,
déplacé ni réutilisé, et son identité est lue par un handle sans suivi
aussitôt refermé. Ils sont exclus de la comparaison de Z6 : une copie remise
que l'utilisateur supprime ou renomme entre le plan et l'exécution ne bloque
pas une nouvelle restauration. Un nom d'opération qu'aucun journal n'explique,
y compris une copie renommée, reste une entrée imprévue, code 3. Le receipt et
le journal d'une restauration remise ne sont pas remis : un receipt absent,
invalide ou d'empreinte différente est une ambiguïté, code 3, même s'il a été
perdu par une coupure ; la zone se ferme alors aux nouvelles restaurations,
la copie restant en place.

### Receipt de restauration v1

Fichier `<dest>\receipts\<op>.json`, écrit en `CREATE_NEW` puis validé par
renommage (ADR 0006 §3), jamais réécrit. Encodage `encoding/json`, UTF-8,
objet unique. Forme indicative ; l'exemple exact sera fixé par les tests
d'`internal/entities/backup` :

```json
{
  "schema": "savewarden.restore",
  "schema_version": 1,
  "restore": "20261107T090000-9a8b7c6d",
  "tool_version": "dev",
  "recovered_by": null,
  "game": {"id": "synthetic-game", "name": "Synthetic Game"},
  "started_at": "2026-11-07T09:00:00Z",
  "publish_intent_at": "2026-11-07T09:00:02Z",
  "published_at": "2026-11-07T09:00:03Z",
  "recovered_at": null,
  "snapshot": {
    "id": "20261106T080000-5e6f7a8b",
    "receipt_sha256": "<SHA-256 des octets du receipt de snapshot>"
  },
  "source": {
    "operation": "20261105T101500-1a2b3c4d",
    "receipt_sha256": "<recopié du receipt de snapshot>"
  },
  "plan_sha256": "<SHA-256 du plan journalisé>",
  "sources": ["… identiques à celles du receipt de snapshot …"]
}
```

| Champ | Contenu |
| --- | --- |
| `schema`, `schema_version` | `savewarden.restore` et `1`, exactement |
| `restore` | Identifiant de l'opération de restauration |
| `tool_version`, `recovered_by` | Comme le receipt v1 |
| `game` | Celui du plan |
| `started_at`, `publish_intent_at`, `published_at`, `recovered_at` | Comme le receipt v1, mêmes règles d'ordre et de reconstruction |
| `snapshot.id`, `snapshot.receipt_sha256` | Snapshot restauré, SHA-256 des octets de son receipt |
| `source.operation`, `source.receipt_sha256` | Recopiés du receipt de snapshot : génération d'origine |
| `plan_sha256` | Empreinte du plan journalisé |
| `sources` | Celles du plan, au format du receipt v1 |

**Relecture stricte**, dans la zone seulement ; le coffre n'est jamais relu
pour la juger :

- règles du receipt v1 pour les champs inconnus, manquants ou dupliqués, les
  dates, la reconstruction et les `sources` ;
- `restore`, `snapshot.id` et `source.operation` au format
  `AAAAMMJJTHHMMSS-xxxxxxxx`, `restore` distinct des deux autres ;
  empreintes de 64 chiffres hexadécimaux minuscules ;
- `restore` égal au nom du fichier et à l'opération du journal ;
  `snapshot.*`, `source.*` et `plan_sha256` égaux à ceux de `planned` ;
  empreinte de contenu des `sources` égale à celle du plan ; SHA-256 des
  octets égal au `receipt_sha256` de `published` ;
- receipt sans journal qui l'explique, ou présent alors que l'opération n'a
  pas validé `publish-intent` ou qu'elle est close autrement qu'en
  `published` : ambiguïté, code 3.

Le receipt contient des chemins locaux et le nom du jeu : à anonymiser avant
tout partage.

### Marqueur de zone v1

Fichier `<dest>\savewarden-restore.json`, validé par temporaire et renommage
comme le marqueur du coffre (ADR 0006 §4, I3 et I4), jamais réécrit :

```json
{"created_at":"2026-11-07T09:00:00Z","game":"synthetic-game","schema":"savewarden.restore-area","schema_version":1,"tool_version":"dev"}
```

Relecture stricte : champs inconnus, manquants ou dupliqués refusés ;
`schema` et `schema_version` exacts ; `game` au format des identifiants de
jeu du manifeste v1. Le marqueur du coffre n'est pas un marqueur de zone, ni
l'inverse. Illisible ou de schéma inconnu : code 1, jamais réécrit ; d'un
autre jeu : code 1.

## Alternatives

- **Journal et receipt dans le coffre, publication directe dans `<dest>`** :
  la restauration écrirait le coffre, verrou compris, le ferait refuser par
  les binaires antérieurs, exigerait un coffre accessible en écriture et
  présent pour récupérer, et écrirait un staging dans un dossier non marqué.
- **Journal à côté de la destination, dans le dossier parent** : des noms
  choisis par Savewarden dans un dossier de l'utilisateur, non marqué, dont
  un dossier de métadonnées permanent.
- **Idempotence par snapshot** : empêcherait d'obtenir une copie neuve dans
  la même zone ; une restauration est une remise, pas un snapshot de plus.
- **Restaurations remises soumises au §3** : un utilisateur qui supprime ou
  modifie sa copie bloquerait la zone sans issue, Savewarden ne supprimant
  rien.
- **Code 3 pour un snapshot en cours côté coffre** : `restore` n'en est pas
  l'acteur de la récupération et ne peut le distinguer d'une opération
  vivante sans sonder `sync.lock`.

## Conséquences

Les scripts testent le code pour le résultat et lisent le receipt de
restauration ou le plan pour l'état. Les tests de `internal/cli`,
`internal/app` et du binaire fixent chaque code ; ceux
d'`internal/entities/backup` fixent chaque règle de relecture ;
`TestStableCodesAreDocumented` couvre cet ADR. Un changement incompatible du
receipt de restauration ou du marqueur de zone exige un nouveau
`schema_version` et un amendement de cet ADR. Le coffre n'étant jamais
écrit, les binaires antérieurs à B4 lisent un coffre après restauration comme
avant ; une zone n'est lue par aucune autre commande, et `inspect` peut lire
une restauration publiée avec un manifeste qui la désigne.

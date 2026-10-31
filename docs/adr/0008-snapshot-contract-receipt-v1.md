# ADR 0008 — Contrat de `snapshot` et receipt de snapshot v1

Date : 30 octobre 2026. Statut : **projet**, rédigé avec le plan B3 v0.2 et
soumis au propriétaire ; **non accepté**. Aucun code ne s'en réclame avant
son acceptation.

## Contexte

B3 ajoute les snapshots : des copies vérifiées de la génération `current`
d'un jeu, approuvées à leur publication et jamais rouvertes en écriture,
déplacées, remplacées ni supprimées ensuite. Le protocole de publication et
de récupération est le §10 d'ADR 0006, projet lui aussi. Cet ADR fixe ce
qu'un utilisateur ou un script observe : la commande, ses préconditions, sa
sortie, ses codes, et le receipt de snapshot, artefact durable du coffre que
B4 et `doctor` (B5) reliront. Le contrat de `sync` reste celui d'ADR 0007.

## Décision

### Commande et modes

```text
savewarden snapshot --manifest <m> --vault <v> [--apply] [--recover]
```

| Mode | Effet |
| --- | --- |
| Plan, par défaut | Affiche l'état et ce que `--apply` ferait ; n'écrit rien |
| `--apply` | Copie `current`, vérifie, recoupe la provenance, publie un snapshot |
| `--recover` | Affiche l'action de récupération des opérations de snapshot ; n'écrit rien |
| `--recover --apply` | Exécute cette récupération |

- `--manifest` et `--vault` obligatoires, mêmes grammaires que `sync` ; aucun
  défaut, aucune variable d'environnement, aucun fichier de configuration.
- Le plan et `--recover` sans `--apply` ne reçoivent aucun port d'écriture.
- `--recover` ne copie rien et ne récupère que des opérations de snapshot ;
  pour une opération de `current`, il renvoie à `sync --recover`.
- Ni libellé, ni sous-commande, ni drapeau de politique. Windows seulement.

### Préconditions

**Création**, plan et `--apply` : manifeste strict ; coffre marqué, sans
lien ; distinction du coffre, de l'installation et des sources, par segments
et par identité, sans lire leur contenu ; dossier du jeu présent ; domaine de
`current` sans aucune intervention, telle que `sync` la relèverait, dont une
opération non terminale, un `staging\` inexpliqué ou un coffre copié ;
`current` reconnu par toute sa chaîne de preuve (ADR 0006 §4, §5) et égal à
son receipt ; aucune opération de snapshot non terminale ; domaine des
snapshots conforme à la grammaire d'ADR 0006 §10 ; espace disponible. Toute
intervention du domaine de `current` donne le code 3, avec la cause que
`sync` lui donnerait.

**Récupération**, `--recover` avec ou sans `--apply` : manifeste strict ;
coffre marqué ; ancêtres de `snapshots\journal` dossiers ordinaires sans
point d'analyse ; distinction ; domaine des snapshots lisible. La présence,
la reconnaissance et l'intégrité de `current`, toute opération ou anomalie du
domaine de `current`, et les autres entrées du coffre sont ignorées : une
opération B2 en attente ne bloque pas la récupération d'un snapshot, ni
l'inverse. Cette indépendance porte sur l'**état vivant** de `current` et de
ses opérations : la seule lecture du domaine de `current` est celle du
receipt source historique, à la porte de provenance. Sans
`snapshots\journal`, il n'y a rien à récupérer : code 0.

**Verrou.** Avec `--apply`, `snapshot` prend `sync.lock` par la même
primitive que `sync`, qui le crée vide s'il manque ; il ne l'écrit ni ne le
supprime jamais.

### Sortie du plan

Texte anglais UTF-8, sans couleur, fins de ligne LF, noms échappés comme
`inspect`, sans date d'exécution : deux plans sur un état inchangé sont
identiques octet pour octet. Ordre des éléments, détaillé dans
`docs/commands/snapshot.md` :

1. en-tête : `manifest`, `game`, `vault`, `current` et sa reconnaissance,
   `pending` ;
2. **état des snapshots** : `snapshots: N published, K degraded,
   M incomplete`, puis une ligne par snapshot publié, du plus ancien au plus
   récent : identifiant, opération source, fichiers et octets, ou son
   diagnostic de dégradation. Le nombre des `.incomplete` conservés est
   compté sur les seuls noms des entrées de `snapshots\`, sans ouvrir ni
   parcourir ces dossiers ; leur taille n'est pas mesurée, et la ligne
   l'écrit, `size not measured`, dès qu'il en existe ;
3. décision : `snapshot: new, from <op>` ou `snapshot: up to date
   (<snap-id>)`, avec fichiers, octets et espace requis ;
4. les interventions requises, s'il y en a ;
5. les anomalies, comme `inspect`.

En récupération, l'en-tête annonce l'opération de snapshot en attente et
l'action exacte que `--apply` exécuterait.

### Codes de sortie

Même sens qu'ADR 0007 : le code décrit **le résultat de la commande**,
jamais à lui seul l'état publié.

| Code | Résultat |
| --- | --- |
| `0` | Plan sans anomalie bloquante, `up to date`, ou snapshot publié et vérifié ; récupération achevée ou sans objet |
| `1` | Invocation, manifeste ou coffre invalide, chevauchement, erreur de lecture, verrou occupé, coffre changé depuis le plan, identifiant déjà employé, interruption, rapport impossible à écrire en entier, bascule refusée puis annulée |
| `2` | Au moins une anomalie bloquante ; aucun snapshot publié |
| `3` | Intervention requise : dans le domaine des snapshots ; dans le domaine de `current` lors d'une création ; provenance non recoupée, `source-unverified` |

### Ligne d'état du coffre

Avec `--apply` ou `--recover`, dernière ligne de stderr
`savewarden: vault: <état>` :

| État | Sens |
| --- | --- |
| `not modified` | Rien écrit |
| `metadata only; nothing staged or published` | Structure ou dossier d'opération créés, aucune copie |
| `snapshot <snap-id> kept incomplete` | Opération close sans publication, `failed` ou `abandoned` ; `I` conservé |
| `published snapshot <snap-id>` | Snapshot publié et approuvé |
| `rolled back snapshot <snap-id>` | Bascule refusée puis annulée, code 1, ou annulée faute de provenance, code 3 ; `I` conservé |
| `intervention required for <snap-id>: <cause>` | Code 3 |

Une publication réussie suivie d'un rapport impossible à écrire sort en
code 1 avec `published snapshot <snap-id>` : le receipt de snapshot fait foi.

### Codes stables et causes

| Code | Sévérité | Sens |
| --- | --- | --- |
| `no-current` | bloquante | Aucune génération publiée : rien à copier |
| `current-modified` | bloquante | Comme ADR 0007 : `current` différent de son receipt, sans receipt valide, ou receipt sans `current` |
| `insufficient-space` | bloquante, au plan comme sous verrou | Génération source plus max(64 Mio, 5 %) |
| `verification-mismatch` | bloquante, `--apply` | Relecture de `I` différente du plan ; `I` conservé |
| `long-path` | information | Chemin le plus long écrit, celui de `I`, au-delà de 259 unités UTF-16 |
| `snapshot-missing` | information, avertissement | Snapshot approuvé absent |
| `snapshot-rename-lost` | information, avertissement | Journal en `published`, snapshot absent, copie restée sous `.incomplete` avec l'identité publiée : renommage perdu, probablement par une coupure ; jamais renommé par Savewarden |
| `snapshot-modified` | information, avertissement | Snapshot approuvé d'une autre identité, ou flanqué d'un `.incomplete` ; receipt de snapshot absent, invalide, contraire au plan ou d'empreinte différente ; contenu relu différent |

Causes d'intervention : `pending-operation`, `unexpected-entry`,
`journal-ambiguous`, et `source-unverified` : receipt source indisponible ou
contradictoire à la porte de provenance d'ADR 0006 §10. Avant toute
publication, l'opération est close en `failed` ; après la bascule, une preuve
contradictoire mène au rollback et une preuve indisponible laisse
l'opération en attente, sans rien écrire.
Diagnostics de code 1 sans code stable : `vault-changed` ; « another
operation is running on this game ».

### Dégradation des snapshots

- Chaque exécution contrôle, pour chaque snapshot approuvé, son receipt et
  la présence de `P` sous l'identité publiée. Le contenu n'est relu que pour
  le snapshot qui rendrait le plan `up to date`. La vérification du contenu
  de tous les snapshots relève de `doctor` (B5) ; aucune sortie de B3 ne
  qualifie l'ensemble des snapshots de sain ou de vérifié.
- Un snapshot dégradé ne bloque ni `snapshot` ni `sync`, ne compte plus pour
  l'idempotence, et n'est jamais recréé, déplacé, réparé ni adopté.
- Il n'est jamais silencieux : compté et nommé dans l'état des snapshots, et,
  tant qu'il en existe, chaque exécution écrit sur stderr
  `savewarden: warning: K degraded snapshot(s)`, avant la ligne d'état.
- Ces codes restent de sévérité `Info` : `entities/backup` n'en connaît que
  deux, partagées avec `inspect`, et B3 n'en ajoute pas.
- Toute autre ambiguïté du domaine des snapshots, opération non terminale,
  journal illisible ou incohérent, entrée inexpliquée, reste fail-closed :
  code 3 pour `snapshot`, sans effet sur `sync`.

### Immutabilité

Après sa publication, un snapshot n'est jamais rouvert en écriture, déplacé,
remplacé ni supprimé par Savewarden ; les lectures de contrôle restent
permises. La garantie est **protocolaire, pas matérielle** : ce n'est pas une
immutabilité de type WORM, et rien n'empêche un autre programme,
l'utilisateur ou un administrateur de modifier un snapshot. Savewarden le
signale dans les limites ci-dessus.

### Receipt de snapshot v1

Fichier `games\<game-id>\snapshots\receipts\<snap-id>.json`, écrit en
`CREATE_NEW` puis validé par renommage (ADR 0006 §3), jamais réécrit.
Encodage `encoding/json`, UTF-8, objet unique. Forme indicative ; l'exemple
exact sera fixé par les tests d'`internal/entities/backup` :

```json
{
  "schema": "savewarden.snapshot",
  "schema_version": 1,
  "snapshot": "20261106T080000-5e6f7a8b",
  "tool_version": "dev",
  "recovered_by": null,
  "game": {"id": "synthetic-game", "name": "Synthetic Game"},
  "started_at": "2026-11-06T08:00:00Z",
  "publish_intent_at": "2026-11-06T08:00:02Z",
  "published_at": "2026-11-06T08:00:03Z",
  "recovered_at": null,
  "source": {
    "operation": "20261105T101500-1a2b3c4d",
    "receipt_sha256": "<SHA-256 des octets de receipts/20261105T101500-1a2b3c4d.json>"
  },
  "plan_sha256": "<SHA-256 du plan journalisé>",
  "sources": ["… identiques à celles du receipt source …"]
}
```

| Champ | Contenu |
| --- | --- |
| `schema`, `schema_version` | `savewarden.snapshot` et `1`, exactement |
| `snapshot` | Identifiant du snapshot, celui de son opération |
| `tool_version`, `recovered_by` | Version qui a planifié et copié ; `null`, ou version qui a reconstruit le receipt en récupération |
| `game` | Celui du plan, donc du receipt source |
| `started_at`, `publish_intent_at`, `published_at`, `recovered_at` | Comme le receipt v1 |
| `source.operation` | Opération de la génération copiée |
| `source.receipt_sha256` | SHA-256 des octets de son receipt |
| `plan_sha256` | Empreinte du plan journalisé |
| `sources` | Celles du plan, au format du receipt v1 |

**Relecture stricte.** Sa portée dépend du journal de l'opération (ADR 0006
§10) :

- journal valide et terminal en `published` : un receipt absent, ou une
  violation des règles ci-dessous, rend le snapshot **dégradé**,
  `snapshot-modified`, sans blocage ;
- receipt de snapshot sans journal qui l'explique, ou incompatible avec
  l'état de son opération, c'est-à-dire présent alors que l'opération n'a pas
  validé `publish-intent` ou qu'elle est close autrement qu'en `published` :
  **ambiguïté**, code 3, pour `snapshot` seulement ;
- opération en `publish-intent` avec receipt validé, A5 ou A6 : la
  récupération le relit selon ces règles, et toute violation donne le
  code 3, rien écrit ni déplacé.

Règles :

- règles du receipt v1 pour les champs inconnus, manquants ou dupliqués, les
  dates, la reconstruction et les `sources` ;
- `snapshot` et `source.operation` au format `AAAAMMJJTHHMMSS-xxxxxxxx`,
  distincts ; empreintes de 64 chiffres hexadécimaux minuscules ;
- `snapshot` égal au nom du fichier et à l'opération du journal ;
  `source.operation`, `source.receipt_sha256` et `plan_sha256` égaux à ceux
  de `planned` ; empreinte de contenu des `sources` égale à celle du plan ;
  SHA-256 des octets égal au `receipt_sha256` de `published`.

Pas de `previous` ni de `changes` : un snapshot ne remplace rien. Le receipt
contient des chemins locaux et le nom du jeu : à anonymiser avant tout
partage.

## Alternatives

- **Étendre le receipt v1** : sa relecture stricte refuserait les nouveaux
  champs, et un receipt de snapshot dans `receipts\` deviendrait une seconde
  tête pour `sync`.
- **`snapshot list` et libellés** : une interface de plus à stabiliser ; le
  plan liste déjà les snapshots, et un libellé libre pourrait porter des
  données personnelles.
- **Anomalie de snapshot en code 3** : sans issue, puisque B3 ne supprime ni
  ne réécrit rien.
- **Troisième sévérité « avertissement »** : elle changerait un type partagé
  avec `inspect` ; la ligne d'état des snapshots et l'avertissement suffisent.

## Conséquences

Les scripts testent le code pour le résultat et lisent le receipt de
snapshot ou le plan pour l'état. Les tests de `internal/cli`, `internal/app`
et du binaire fixent chaque code ; ceux d'`internal/entities/backup` fixent
chaque règle de relecture. Un changement incompatible du receipt de snapshot
exige un nouveau `schema_version` et un amendement de cet ADR. Un binaire
antérieur à B3 refuse en code 3 un coffre qui contient `snapshots\`, sans
rien écrire (ADR 0007, amendement B3).

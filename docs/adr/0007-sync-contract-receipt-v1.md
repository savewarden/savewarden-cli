# ADR 0007 — Contrat de `sync` et receipt v1

Date : 29 octobre 2026. Statut : accepté pour B2, plan B2 v0.2 §4, validé par
le propriétaire le 29 octobre 2026. B2a livre le mode plan ; B2b et B2c
ajoutent leurs modes sans changer ce qui est fixé ici, sauf amendement.

## Contexte

`sync` est la première commande dont le résultat peut différer de l'état
réellement publié : une publication peut réussir puis l'écriture du rapport
échouer. Un script doit distinguer « plan sûr », « plan bloqué »,
« intervention requise » et « échec », sans analyser le texte, et retrouver
l'état publié sans faire confiance à la sortie. Le receipt, lui, est un
artefact durable du coffre, relu à chaque `sync` et par `doctor` en B5.

## Décision

### Commande et disponibilité

```text
savewarden sync --manifest <m> --vault <v> [--apply] [--recover] [--allow-removals] [--accept-missing-source <id>]...
```

| Mode ou flag | B2a | B2b | B2c |
| --- | --- | --- | --- |
| Plan, par défaut | Oui | Oui | Oui |
| `--apply` | Absent : flag inconnu, code 1 | Première publication | Première publication et rotation |
| `--recover` | Absent | Première publication et initialisation interrompues | Toute opération interrompue |
| `--allow-removals`, `--accept-missing-source` | Absents | Absents | Selon le plan B2 §4.4 et V1 |

- `--manifest` et `--vault` obligatoires ; aucun défaut, aucune variable
  d'environnement, aucun fichier de configuration.
- `--vault` suit la grammaire `ParseWindowsAbsolute` du manifeste v1 ; rien
  n'est nettoyé ni complété.
- Le mode plan n'écrit rien, nulle part. Il ne reçoit aucun port d'écriture.

### Sortie du plan

Texte anglais UTF-8, sans couleur, fins de ligne LF, noms échappés comme
`inspect`. Aucune date d'exécution : deux plans sur un état inchangé sont
identiques octet pour octet. Ordre des éléments, détaillé dans
[`docs/commands/sync.md`](../commands/sync.md) :

1. en-tête : `manifest`, `game`, `vault` et son état, `destination`,
   `volume`, `current`, `pending`, `mode`, `plan` ;
2. un bloc par source du manifeste, avec son statut, ses compteurs et une
   ligne par fichier, préfixée par `+` ajouté, `~` modifié, `-` retiré ou
   `=` inchangé ;
3. les interventions requises, s'il y en a ;
4. les anomalies, comme `inspect`.

Quand une intervention est requise, les sources ne sont pas planifiées : un
plan calculé sur un coffre ambigu serait trompeur.

### Codes de sortie

Le code décrit **le résultat de la commande**, jamais à lui seul l'état
publié.

| Code | Résultat | stdout | stderr en mode plan |
| --- | --- | --- | --- |
| `0` | Plan sans anomalie bloquante, ou déjà à jour ; en B2b et B2c, publication vérifiée | Rapport | Vide |
| `1` | Invocation, manifeste ou coffre invalide, volume indisponible, chevauchement, erreur de lecture, interruption, rapport impossible à écrire en entier | Vide, ou préfixe tronqué | Un diagnostic |
| `2` | Rapport complet avec au moins une anomalie bloquante ; rien n'est publié | Rapport | `savewarden: plan blocked: N blocking finding(s)` |
| `3` | Intervention requise : opération en attente, initialisation interrompue, entrée imprévue dans le coffre, journal ou receipts ambigus | Rapport | `savewarden: intervention required: <cause>` |

Avec `--apply` ou `--recover` (B2b), stderr se termine par la **ligne d'état
du coffre** `savewarden: vault: <état>`, avec les états et combinaisons du
plan B2 §4.8 : `not modified`, `metadata only; nothing staged or published`,
`current unchanged; operation <op> kept in staging`, `published <op>`,
`rolled back <op>; current unchanged`, `intervention required for <op>:
<cause>`. Une publication réussie suivie d'un rapport impossible à écrire
sort en code 1 avec `vault: published <op>` : le receipt fait foi.

### Codes stables

Anomalies propres à `sync`, en plus de celles d'`inspect` héritées du
parcours :

| Code | Sévérité | Sens |
| --- | --- | --- |
| `source-missing` | bloquante si requise, sinon information | Source absente, sans fichiers au receipt précédent pour une facultative |
| `source-vanished` | bloquante | Source facultative absente alors que le receipt précédent la contient |
| `source-removed-from-manifest` | bloquante | Source du receipt précédent absente du manifeste ; aucun flag en B2 |
| `files-removed` | bloquante | Fichiers ou dossiers du receipt précédent absents d'une source présente |
| `unportable-name` | bloquante | Nom que Win32 altérerait : point ou espace final, nom réservé, caractère interdit |
| `encrypted-source` | bloquante | Fichier chiffré par EFS : jamais copié en clair |
| `current-modified` | bloquante | `current` différent de son receipt, sans receipt valide, ou receipt sans `current` |
| `long-path` | information | Chemin final au-delà de 259 unités UTF-16 |
| `same-volume` | information | Coffre et source sur le même volume : pas une sauvegarde hors disque |

Causes d'intervention : `pending-operation`, `initialization-interrupted`,
`unexpected-entry`, `journal-ambiguous`.

### Sources absentes, retraits et `current`

Les règles du plan B2 §4.4 et §4.5 sont normatives : une absence n'est jamais
une suppression volontaire ; `--allow-removals` ne couvre jamais une source
absente ou retirée du manifeste ; `--accept-missing-source <id>` ne couvre
qu'une source facultative, physiquement absente et présente dans le receipt
précédent ; un `current` qui ne correspond plus exactement à son receipt
bloque toute publication, sans contournement en B2.

Le receipt de `current` est le seul receipt valide qu'aucun autre ne désigne
comme `previous`. Aucun, ou plusieurs : `current-modified`.

### Receipt v1

Fichier `games\<game-id>\receipts\<op-id>.json`, écrit en `CREATE_NEW` puis
validé par renommage (ADR 0006), jamais réécrit. Encodage `encoding/json`,
UTF-8, objet unique :

```json
{
  "schema": "savewarden.receipt",
  "schema_version": 1,
  "operation": "20261105T101500-1a2b3c4d",
  "tool_version": "dev",
  "recovered_by": null,
  "game": {"id": "synthetic-game", "name": "Synthetic Game"},
  "started_at": "2026-11-05T10:15:00Z",
  "publish_intent_at": "2026-11-05T10:15:03Z",
  "published_at": "2026-11-05T10:15:04Z",
  "recovered_at": null,
  "previous": null,
  "plan_sha256": "<64 chiffres hexadécimaux>",
  "changes": {"added": 1, "modified": 0, "removed": 0, "unchanged": 0},
  "sources": [
    {
      "id": "saves", "kind": "save", "required": true,
      "status": "copied", "accepted": false,
      "path": "D:\\Games\\Synthetic Game\\saves",
      "files": 1, "bytes": 3,
      "directories": ["Slot 1"],
      "entries": [
        {"path": "Slot 1/1.save", "size": 3, "mod_time": "2026-10-09T16:32:08.1234567Z", "sha256": "<64 chiffres hexadécimaux>"}
      ]
    }
  ]
}
```

Relecture stricte, toute violation étant un receipt invalide :

- champs inconnus, manquants ou dupliqués refusés ; aucune donnée après
  l'objet ;
- `schema` et `schema_version` exacts ; `operation` et `previous` au format
  `AAAAMMJJTHHMMSS-xxxxxxxx`, `previous` différent de `operation` ;
- dates RFC 3339 en UTC, suffixe `Z` ; `publish_intent_at` postérieur ou égal à
  `started_at` ;
- receipt normal : `published_at` renseigné et postérieur ou égal à
  `publish_intent_at`, `recovered_at` et `recovered_by` nuls ;
- receipt reconstruit : `published_at` nul, `recovered_at` postérieur ou égal
  à `publish_intent_at`, `recovered_by` renseigné ; provenance de chaque champ
  selon le plan B2 §4.7 ;
- sources d'identifiants uniques ; `status` vaut `copied` ou `missing` ;
  `accepted` seulement pour une source `missing` ; une source `missing` n'a
  ni fichiers, ni octets, ni entrées, ni dossiers ;
- entrées et dossiers en chemins relatifs à barres obliques, sans segment
  vide, `.` ni `..`, triés octet par octet et sans doublon ; `files` égal au
  nombre d'entrées, `bytes` à la somme des tailles ; tailles positives ou
  nulles ; SHA-256 de 64 chiffres hexadécimaux minuscules.

Le receipt contient des chemins locaux et le nom du jeu : à anonymiser avant
tout partage.

### Marqueur du coffre v1

`savewarden-vault.json`, validé par renommage à l'initialisation (ADR 0006,
étapes I3 et I4), jamais réécrit :

```json
{"schema": "savewarden.vault", "schema_version": 1, "created_at": "2026-11-05T10:15:00Z", "tool_version": "dev"}
```

Relecture stricte comme le receipt. Un marqueur invalide ou de schéma
inconnu est refusé en code 1, jamais réparé.

### Espace libre (B2b)

Réserve exigée : taille complète de la nouvelle génération plus
max(64 Mio, 5 % de cette taille), comparée aux octets disponibles pour
l'utilisateur appelant, au préflight puis sous verrou (V2). Estimation, pas
garantie.

## Alternatives

- **Code 1 pour une intervention requise** : un script confondrait un coffre
  à réparer avec une faute de frappe.
- **État publié déduit du code** : faux dès qu'une écriture du rapport échoue
  après publication.
- **Sortie JSON de la CLI dès B2** : un second format à stabiliser ; le
  receipt suffit comme artefact machine (D10).
- **Receipt dans `current`** : il ferait partie des données copiées et
  fausserait leur comparaison ; il vit dans `receipts\`.

## Conséquences

Les scripts testent le code pour le résultat et lisent receipt ou mode plan
pour l'état. Les tests de `internal/cli`, `internal/app` et du binaire fixent
chaque code ; ceux d'`internal/entities/backup` fixent chaque règle de
relecture du receipt et du marqueur. Un changement incompatible du receipt
exige un nouveau `schema_version` et un amendement de cet ADR.

# Receipt de snapshot v1

Un receipt de snapshot décrit exactement un snapshot publié dans un
[coffre](vault.md) : la génération qu'il copie, et chaque fichier, avec sa
taille, sa date de modification et son SHA-256. Il est écrit une seule fois,
à la publication, dans `games\<game-id>\snapshots\receipts\<snap-id>.json`, et
jamais réécrit.

**Statut :** schéma fixé par
[ADR 0008](adr/0008-snapshot-contract-receipt-v1.md), distinct du
[receipt v1](receipt-v1.md) d'une génération.

## Exemple

Exemple synthétique, vérifié par les tests de `internal/entities/backup` :

```json
{
  "schema": "savewarden.snapshot",
  "schema_version": 1,
  "snapshot": "20261106T080000-5e6f7a8b",
  "tool_version": "dev",
  "recovered_by": null,
  "game": {
    "id": "synthetic-game",
    "name": "Synthetic Game"
  },
  "started_at": "2026-11-06T08:00:00Z",
  "publish_intent_at": "2026-11-06T08:00:04Z",
  "published_at": "2026-11-06T08:00:05Z",
  "recovered_at": null,
  "source": {
    "operation": "20261105T101500-1a2b3c4d",
    "receipt_sha256": "6ac52be28458527a4a3c042496efca203b2981b45908c29c09559c5bc2eb8a11"
  },
  "plan_sha256": "ae833c72f4d3dd1ad9b602f6aa38e5304c9a0bdf4f7f3800f872a4500e89c757",
  "sources": [
    {
      "id": "saves",
      "kind": "save",
      "required": true,
      "status": "copied",
      "accepted": false,
      "path": "D:\\Games\\Synthetic Game\\saves",
      "files": 1,
      "bytes": 3,
      "directories": [],
      "entries": [
        {
          "path": "1.save",
          "size": 3,
          "mod_time": "2026-11-05T10:15:00Z",
          "sha256": "7692c3ad3540bb803c020b3aee66cd8887123234ea0c6e7143c0add73ff431ed"
        }
      ]
    }
  ]
}
```

## Champs

| Champ | Contenu |
| --- | --- |
| `schema`, `schema_version` | `savewarden.snapshot` et `1`, exactement |
| `snapshot` | Identifiant du snapshot, celui de son opération |
| `tool_version` | Version de Savewarden qui a planifié et copié |
| `recovered_by` | `null`, ou version qui a reconstruit le receipt lors d'une récupération |
| `game` | Identifiant et nom du jeu, ceux du receipt de la génération copiée |
| `started_at`, `publish_intent_at`, `published_at`, `recovered_at` | Comme le [receipt v1](receipt-v1.md) : `published_at` est nul dans un receipt reconstruit |
| `source.operation` | Opération de la génération copiée |
| `source.receipt_sha256` | SHA-256 des octets du receipt de cette génération : la preuve de provenance |
| `plan_sha256` | Empreinte du plan journalisé de l'opération de snapshot |
| `sources` | Les sources de la génération copiée, au format du receipt v1 |

Pas de `previous` ni de `changes` : un snapshot ne remplace rien.

## Relecture stricte

Les règles du receipt v1 s'appliquent : champs inconnus, manquants ou en
double, dates, reconstruction, chemins. S'y ajoutent `snapshot` et
`source.operation` distincts, et l'accord avec le journal de l'opération :
`source`, `plan_sha256` et contenu égaux au plan, SHA-256 des octets égal à
celui que l'enregistrement `published` a retenu.

Pour une opération close en `published`, un receipt absent ou contraire rend
le snapshot **dégradé**, `snapshot-modified`, sans rien bloquer. Un receipt
qu'aucun journal n'explique, ou que son opération ne peut pas avoir validé,
est une ambiguïté : code 3 pour `snapshot`, sans effet sur `sync`.

Le receipt contient des chemins locaux et le nom du jeu : à anonymiser avant
tout partage.

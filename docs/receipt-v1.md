# Receipt v1

Un receipt décrit exactement une génération publiée dans un
[coffre](vault.md) : quelles sources, quels dossiers, quels fichiers, avec
leur taille, leur date de modification et leur SHA-256. Il est écrit une seule
fois, à la publication, dans `games\<game-id>\receipts\<op-id>.json`, et
jamais réécrit. `sync` le relit avant chaque plan pour vérifier que `current`
n'a pas changé.

**Statut :** schéma fixé par
[ADR 0007](adr/0007-sync-contract-receipt-v1.md). Cette version planifie
seulement : aucun receipt n'est encore écrit, ils le seront à partir de B2b.
Le modèle et sa relecture stricte existent déjà, testés.

## Exemple

Exemple synthétique, vérifié par les tests de `internal/entities/backup` :

```json
{
  "schema": "savewarden.receipt",
  "schema_version": 1,
  "operation": "20261105T101500-1a2b3c4d",
  "tool_version": "dev",
  "recovered_by": null,
  "game": {
    "id": "synthetic-game",
    "name": "Synthetic Game"
  },
  "started_at": "2026-11-05T10:15:00Z",
  "publish_intent_at": "2026-11-05T10:15:03Z",
  "published_at": "2026-11-05T10:15:04Z",
  "recovered_at": null,
  "previous": null,
  "plan_sha256": "c75bccf4839e3bcb90317713e0b7f3062a0e099013ca5f784457bd2b9d8c3354",
  "changes": {
    "added": 2,
    "modified": 0,
    "removed": 0,
    "unchanged": 0
  },
  "sources": [
    {
      "id": "saves",
      "kind": "save",
      "required": true,
      "status": "copied",
      "accepted": false,
      "path": "D:\\Games\\Synthetic Game\\saves",
      "files": 2,
      "bytes": 6,
      "directories": [
        "Slot 1"
      ],
      "entries": [
        {
          "path": "1.save",
          "size": 3,
          "mod_time": "2026-10-09T16:32:08.1234567Z",
          "sha256": "7692c3ad3540bb803c020b3aee66cd8887123234ea0c6e7143c0add73ff431ed"
        },
        {
          "path": "Slot 1/2.save",
          "size": 3,
          "mod_time": "2026-10-09T16:32:08.1234567Z",
          "sha256": "3fc4ccfe745870e2c0d99f71f30ff0656c8dedd41cc1d7d3d376b0dbe685e2f3"
        }
      ]
    },
    {
      "id": "config",
      "kind": "config",
      "required": false,
      "status": "missing",
      "accepted": false,
      "path": "D:\\Games\\Synthetic Game\\config",
      "files": 0,
      "bytes": 0,
      "directories": [],
      "entries": []
    }
  ]
}
```

## Champs

| Champ | Contenu |
| --- | --- |
| `schema`, `schema_version` | `savewarden.receipt` et `1`, exactement |
| `operation` | Identifiant de l'opération qui a produit la génération |
| `tool_version` | Version de Savewarden qui a planifié et copié |
| `recovered_by` | `null`, ou version qui a reconstruit le receipt lors d'une récupération |
| `game` | Identifiant et nom du jeu, tels que le manifeste les déclarait |
| `started_at` | Début de l'opération, UTC |
| `publish_intent_at` | Validation de l'intention de publier : la génération a été publiée **après** cet instant |
| `published_at` | Instant de la publication ; `null` dans un receipt reconstruit, car cet instant est alors inconnu |
| `recovered_at` | `null`, ou instant de la reconstruction |
| `previous` | Opération de la génération remplacée, `null` pour la première |
| `plan_sha256` | Empreinte du plan journalisé |
| `changes` | Fichiers ajoutés, modifiés, retirés et inchangés par rapport à la génération précédente |
| `sources[].status` | `copied`, ou `missing` pour une source absente |
| `sources[].accepted` | `true` seulement pour une source facultative absente, acceptée explicitement |
| `sources[].directories`, `entries` | Dossiers et fichiers, en chemins relatifs à la source, triés octet par octet |

Un consommateur qui a besoin d'une date de publication lit `published_at`,
ou à défaut la borne `publish_intent_at`, jamais `recovered_at`.

## Relecture stricte

Toute violation rend le receipt invalide, et `sync` bloque alors toute
publication sur ce jeu (`current-modified`) :

- champs inconnus, manquants ou en double, `null` pour un champ obligatoire,
  données après l'objet ;
- dates hors UTC ou sans suffixe `Z` ; intention de publier antérieure au
  début ; publication antérieure à l'intention ;
- receipt reconstruit qui prétendrait connaître `published_at`, ou dont
  `recovered_at` et `recovered_by` ne sont pas renseignés ensemble ;
- chemins non relatifs, avec `.` ou `..`, non triés ou en double ;
- `files` ou `bytes` qui ne correspondent pas aux entrées ; SHA-256 qui ne
  serait pas en 64 chiffres hexadécimaux minuscules.

## Confidentialité

Un receipt contient des chemins locaux et le nom du jeu. Anonymisez-le avant
tout partage.

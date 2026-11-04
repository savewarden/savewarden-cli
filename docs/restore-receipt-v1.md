# Receipt de restauration v1 et marqueur de zone v1

Une [zone de restauration](commands/restore.md#zone-de-restauration) porte un
marqueur, `savewarden-restore.json`, et un receipt par restauration publiée,
`receipts\<op>.json`. Tous deux sont écrits une seule fois, par un temporaire
renommé sans remplacement, et jamais réécrits. Ils se lisent **sans le
coffre** : le receipt chaîne la restauration au snapshot restauré et à la
génération que ce snapshot copiait.

**Statut :** schémas fixés par
[ADR 0009](adr/0009-restore-contract-receipt-v1.md), distincts du
[receipt v1](receipt-v1.md) d'une génération et du
[receipt de snapshot v1](snapshot-receipt-v1.md).

## Receipt de restauration

Exemple synthétique, vérifié par les tests de `internal/entities/backup` :

```json
{
  "schema": "savewarden.restore",
  "schema_version": 1,
  "restore": "20261107T090000-9a8b7c6d",
  "tool_version": "dev",
  "recovered_by": null,
  "game": {
    "id": "synthetic-game",
    "name": "Synthetic Game"
  },
  "started_at": "2026-11-07T09:00:00Z",
  "publish_intent_at": "2026-11-07T09:00:04Z",
  "published_at": "2026-11-07T09:00:05Z",
  "recovered_at": null,
  "snapshot": {
    "id": "20261106T080000-5e6f7a8b",
    "receipt_sha256": "9cd29f81d6df80c7e3236ae5c33e1101c13c4ab7859be7469c704a5c0ee3c23d"
  },
  "source": {
    "operation": "20261105T101500-1a2b3c4d",
    "receipt_sha256": "6ac52be28458527a4a3c042496efca203b2981b45908c29c09559c5bc2eb8a11"
  },
  "plan_sha256": "7c339e93a2cae2c3916c8aa0582c619ef0475fcce314838f783a7e29819dde64",
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

| Champ | Contenu |
| --- | --- |
| `schema`, `schema_version` | `savewarden.restore` et `1`, exactement |
| `restore` | Identifiant de l'opération de restauration, nom du dossier publié |
| `tool_version`, `recovered_by` | Comme le [receipt v1](receipt-v1.md) : `recovered_by` n'est renseigné que pour un receipt reconstruit |
| `game` | Jeu du manifeste au moment de la restauration |
| `started_at`, `publish_intent_at`, `published_at`, `recovered_at` | Comme le receipt v1 : `published_at` est nul dans un receipt reconstruit par `--recover` |
| `snapshot.id`, `snapshot.receipt_sha256` | Snapshot restauré, et SHA-256 des octets de son receipt |
| `source.operation`, `source.receipt_sha256` | Recopiés du receipt de snapshot : la génération d'origine |
| `plan_sha256` | Empreinte du plan journalisé de l'opération |
| `sources` | Celles du receipt de snapshot, au format du receipt v1 : les chemins réellement sauvegardés, puis chaque fichier restauré |

**Relecture stricte**, dans la zone seulement : règles du receipt v1 pour les
champs inconnus, manquants ou en double, les dates et la reconstruction ;
`restore`, `snapshot.id` et `source.operation` distincts ; `restore` égal au
nom du fichier et à l'opération du journal ; snapshot, source, plan et contenu
égaux au plan journalisé ; SHA-256 des octets égal à celui que
l'enregistrement `published` a retenu. Un receipt absent, invalide ou
différent pour une restauration publiée, ou un receipt qu'aucune opération de
la zone n'a pu valider, est une ambiguïté de la zone, code 3.

## Marqueur de zone

```json
{"created_at":"2026-11-07T09:00:00Z","game":"synthetic-game","schema":"savewarden.restore-area","schema_version":1,"tool_version":"dev"}
```

Relecture stricte : champs inconnus, manquants ou en double refusés ; `schema`
et `schema_version` exacts ; `game` au format des identifiants de jeu du
manifeste v1. Une zone ne sert qu'à un jeu. Le marqueur du coffre,
`savewarden-vault.json`, n'est pas un marqueur de zone, ni l'inverse. Un
marqueur illisible, de schéma inconnu ou d'un autre jeu : code 1, jamais
réécrit.

## Guide de récupération d'une zone

`restore` ne lit jamais le coffre pour récupérer une zone : gardez le
manifeste, qui sert à vérifier que la zone reste distincte du jeu.

1. **Lisez ce que la zone contient**, sans rien écrire :

   ```powershell
   .\bin\savewarden.exe restore --manifest 'D:\Manifests\mon-jeu.yaml' --to 'G:\Restaurations\mon-jeu' --recover
   ```

   La sortie donne l'opération interrompue, son dernier enregistrement, ce
   qui est observé sous `<op>` et `<op>.incomplete`, et l'action prévue.
2. **Pendant les étapes A3 à A6**, la sortie avertit : `operation <op> pending
   recovery: copy published, receipt missing; do not use <dossier>\<op> yet`.
   N'ouvrez pas ce dossier avant la fin de la récupération.
3. **Exécutez la récupération**, une fois fermés les programmes qui utilisent
   la zone :

   ```powershell
   .\bin\savewarden.exe restore --manifest 'D:\Manifests\mon-jeu.yaml' --to 'G:\Restaurations\mon-jeu' --recover --apply
   ```

   La dernière ligne de stderr dit ce que contient la zone. Relancez-la après
   une interruption : elle est idempotente et converge.
4. **Code 1, `another restore is running in this area`** : une restauration
   est en cours ; attendez sa fin.
5. **Code 3** : la zone contient quelque chose qu'aucun journal n'explique,
   ou un journal ou un receipt ambigu. Savewarden n'y touche plus. Ne
   supprimez ni ne renommez rien dans la zone ; copiez ailleurs les
   restaurations publiées dont vous avez besoin, et restaurez dans une
   nouvelle zone.

Une copie `<op>.incomplete` n'est jamais une restauration utilisable : seule
`<op>`, annoncée par `published restore <op>`, l'est.

## Reprendre les fichiers dans le jeu

Savewarden ne restaure jamais dans le jeu. Pour reprendre une sauvegarde,
**à la main et à vos risques** :

1. Fermez le jeu et son launcher.
2. Lancez `sync --apply` pour conserver l'état actuel du jeu dans le coffre,
   puis, si vous le souhaitez, `snapshot --apply` pour le figer.
3. Copiez, depuis `<dossier>\<op>\<source-id>\`, les seuls fichiers dont vous
   avez besoin vers la source du jeu, avec l'Explorateur ou `robocopy` sans
   `/MIR`, sans rien supprimer.
4. Vérifiez avec `inspect` que la source contient ce que vous attendez, puis
   lancez le jeu.

Le receipt de restauration donne le SHA-256 de chaque fichier restauré. Les
fichiers de la zone peuvent être copiés autant de fois que nécessaire : la
restauration publiée n'est jamais modifiée par Savewarden.

Le receipt et le marqueur contiennent des chemins locaux et le nom du jeu : à
anonymiser avant tout partage.

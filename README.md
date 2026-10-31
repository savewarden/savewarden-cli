# Savewarden

**Your progress. Always protected.**

Savewarden vise à inspecter, centraliser, vérifier et restaurer les sauvegardes
de jeux à partir de sources explicitement choisies. Le moteur sera indépendant
de la CLI, local-first et Windows-first.

**État actuel : lot B3, version `dev`.** La commande `inspect` lit un
[manifeste v1](docs/manifest-v1.md) et inventorie en lecture seule les dossiers
de sauvegarde qu'il déclare. La commande `sync` planifie, en lecture seule,
leur copie vers un [coffre](docs/vault.md) ; avec `--apply`, elle y publie une
nouvelle génération vérifiée du jeu, l'ancienne étant conservée dans
`retained`, et `--recover` reprend toute opération interrompue. La commande
`snapshot` fige une copie vérifiée et immuable de cette génération. Les
sources ne sont que lues ; rien n'est jamais supprimé ni écrasé. Les commandes
`restore` et `doctor` ne sont pas encore disponibles. Aucune release ni
installation via package manager.

## Compiler et utiliser

Prérequis : Git et Go **1.27.3**. Depuis la racine du dépôt :

```sh
go mod download
go build -trimpath -o bin/savewarden ./cmd/savewarden
go run ./cmd/savewarden --help
go run ./cmd/savewarden version
```

Le dernier exemple affiche exactement :

```text
savewarden dev
```

Pour produire un exécutable Windows nommé explicitement :

```powershell
go build -trimpath -o bin/savewarden.exe ./cmd/savewarden
.\bin\savewarden.exe --help
.\bin\savewarden.exe version
```

Sans argument, `savewarden` affiche l'aide. `--help`, `-h`, `help` et
`<commande> --help` sont disponibles ; `--version` affiche la même version que
`version`. Succès : code **0**, sortie sur stdout. Erreur d'invocation ou
d'exécution : code **1**, diagnostic unique sur stderr, sans sortie de données
ni affichage automatique de l'aide ; seul un échec d'écriture sur stdout peut
y laisser une sortie tronquée. Pas de sortie JSON à ce stade.

Les sorties sont en anglais, UTF-8, sans couleur et terminées par LF. La version
de développement est explicite ; aucune date ou version calculée depuis Git.

## Inspecter un jeu

Écrivez un manifeste pour votre jeu, en partant de
[l'exemple de référence](docs/manifest-v1.md), puis, jeu et launcher fermés :

```powershell
.\bin\savewarden.exe inspect --manifest 'D:\Manifests\mon-jeu.yaml'
```

La sortie liste chaque fichier avec son SHA-256, sa taille et sa date de
modification, puis les anomalies. Code **0** : inventaire complet. Code **2** :
rapport complet mais au moins une anomalie bloquante, par exemple une jonction
refusée ou un fichier verrouillé. Code **1** : manifeste invalide ou erreur,
sans rapport complet ; si l'écriture du rapport échoue, stdout peut en garder
un début tronqué. Détails, garanties et limites :
[`savewarden inspect`](docs/commands/inspect.md).

`inspect` ne fonctionne que sous Windows. Sa sortie contient des chemins
locaux : anonymisez-la avant de la partager.

## Planifier une synchronisation

Choisissez un coffre : un dossier absent ou vide, distinct de l'installation,
de préférence sur un autre disque physique. Puis, jeu et launcher fermés :

```powershell
.\bin\savewarden.exe sync --manifest 'D:\Manifests\mon-jeu.yaml' --vault 'F:\Coffre'
```

Le plan liste chaque fichier comme ajouté, modifié, retiré ou inchangé par
rapport à la génération courante du coffre, puis les anomalies. Rien n'est
écrit, pas même le coffre. Code **0** : plan sans anomalie bloquante.
Code **2** : plan bloqué, par exemple une source requise absente ou un fichier
retiré. Code **3** : le coffre demande une intervention. Code **1** : manifeste
ou coffre invalide, ou erreur.

Pour publier, toujours jeu et launcher fermés :

```powershell
.\bin\savewarden.exe sync --manifest 'D:\Manifests\mon-jeu.yaml' --vault 'F:\Coffre' --apply
```

La copie passe par un staging, vérifié en SHA-256 avant de devenir `current`,
avec un receipt. La dernière ligne de stderr dit ce que contient le coffre,
par exemple `savewarden: vault: published <opération>`. Après une
interruption, `sync --recover` montre la récupération prévue et
`sync --recover --apply` l'exécute. Détails : [`savewarden sync`](docs/commands/sync.md).

## Figer un snapshot

Un snapshot copie la génération que le coffre détient déjà, vérifiée contre
son receipt : l'état sauvegardé par le dernier `sync --apply`. Il ne lit pas
la source du jeu, qui peut rester ouvert :

```powershell
.\bin\savewarden.exe snapshot --manifest 'D:\Manifests\mon-jeu.yaml' --vault 'F:\Coffre'
.\bin\savewarden.exe snapshot --manifest 'D:\Manifests\mon-jeu.yaml' --vault 'F:\Coffre' --apply
```

Le premier plan liste les snapshots déjà publiés et ce que `--apply`
copierait ; le second copie, relit, recoupe la provenance et publie le
snapshot sous `snapshots\<opération>`, avec son receipt. Une fois publié, un
snapshot n'est jamais rouvert en écriture, déplacé ni supprimé par
Savewarden. Cette garantie est protocolaire, pas un stockage WORM. Un même
état n'est figé qu'une fois : `--apply` répond alors `up to date`. Détails :
[`savewarden snapshot`](docs/commands/snapshot.md).

## Sécurité et support

- `inspect` ne lit que les dossiers déclarés. Il ne suit aucun lien,
  jonction ni point d'analyse, et ne lit pas les placeholders cloud.
- `sync` refuse tout dossier non vide qui n'est pas un coffre Savewarden et
  tout coffre qui chevauche l'installation. Sans `--apply`, il n'écrit rien.
- `--apply` n'écrit que dans le coffre, par créations exclusives et
  renommages qui ne remplacent rien ; chaque publication passe par un staging
  vérifié en SHA-256 et un journal, et toute ambiguïté arrête tout en code 3.
- Le code de production est gardé par un test qui refuse les API d'écriture,
  d'exécution et de réseau, sauf dans le seul package `platform/vault`, en
  création seule. Les tests utilisent des données synthétiques.
- Les sauvegardes sources et les snapshots approuvés ne sont jamais supprimés.
  Un snapshot absent ou altéré est signalé, jamais réparé.
- Une copie sur le même disque physique ne protège pas contre la panne du disque.

Windows est le pilote fonctionnel. La CI teste sur Windows, Linux et macOS ;
sous Linux et macOS, `inspect` refuse explicitement les chemins Windows et
seule la logique générique est vérifiée. Aucun binaire distribué pour
l'instant. Voir le [modèle de sécurité](docs/safety-model.md).

## Développement et feuille de route

La [contribution](CONTRIBUTING.md) décrit les contrôles reproductibles.
L'[architecture](docs/architecture.md) fixe les frontières des packages et
le [workflow GitOps](docs/gitops.md) gouverne les branches et revues.
Le [pilote Odyssey](docs/pilots/odyssey.md) conserve les constats historiques
sans inclure de sauvegardes privées.

Ordre prévu, avec revue et Human GO entre lots : B1 manifeste strict et
`inspect` read-only ; B2 `sync` vérifié sans suppression, en trois PR : B2a
plan en lecture seule, B2b première publication, B2c rotation et récupération
complète ; B3 snapshots immuables ; B4 restauration isolée ; B5 `doctor` et
validation pilote.
La [roadmap originale](.plans/Savewarden_Plan_Bootstrap_Architecture_GitOps_v0.1.md),
le [plan B1](.plans/Savewarden_Plan_B1_Manifeste_Inspect_v0.2.md), le
[plan B2](.plans/Savewarden_Plan_B2_Synchronisation_v0.2.md) et le
[plan B3](.plans/Savewarden_Plan_B3_Snapshots_v0.2.md) restent les
sources du périmètre, et le [changelog](CHANGELOG.md) décrit l'existant.

Licence [MIT](LICENSE), choisie explicitement pour ce bootstrap.

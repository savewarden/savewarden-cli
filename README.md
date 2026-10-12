# Savewarden

**Your progress. Always protected.**

Savewarden vise à inspecter, centraliser, vérifier et restaurer les sauvegardes
de jeux à partir de sources explicitement choisies. Le moteur sera indépendant
de la CLI, local-first et Windows-first.

**État actuel : lot B1, version `dev`.** La commande `inspect` lit un
[manifeste v1](docs/manifest-v1.md) et inventorie en lecture seule les dossiers
de sauvegarde qu'il déclare. Rien n'est copié, déplacé, écrit ni exécuté.
Les commandes `sync`, `snapshot`, `restore` et `doctor` ne sont pas encore
disponibles. Aucune release ni installation via package manager.

## Compiler et utiliser

Prérequis : Git et Go **1.27.2**. Depuis la racine du dépôt :

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
ni affichage automatique de l'aide. Pas de sortie JSON à ce stade.

Les sorties sont en anglais, UTF-8, sans couleur et terminées par LF. La version
de développement est explicite ; aucune date ou version calculée depuis Git.

## Inspecter un jeu

Écrivez un manifeste pour votre jeu, en partant de
[l'exemple de référence](docs/manifest-v1.md), puis, jeu et launcher fermés :

```powershell
.\bin\savewarden.exe inspect --manifest 'D:\Savewarden\games\mon-jeu\manifest.yaml'
```

La sortie liste chaque fichier avec son SHA-256, sa taille et sa date de
modification, puis les anomalies. Code **0** : inventaire complet. Code **2** :
rapport complet mais au moins une anomalie bloquante, par exemple une jonction
refusée ou un fichier verrouillé. Code **1** : manifeste invalide ou erreur,
sans rapport. Détails, garanties et limites :
[`savewarden inspect`](docs/commands/inspect.md).

`inspect` ne fonctionne que sous Windows. Sa sortie contient des chemins
locaux : anonymisez-la avant de la partager.

## Sécurité et support

- `inspect` ne lit que les dossiers déclarés. Il ne suit aucun lien,
  jonction ni point d'analyse, et ne lit pas les placeholders cloud.
- Le code de production est gardé par un test qui refuse les API d'écriture,
  d'exécution et de réseau. Les tests utilisent des données synthétiques.
- Les prochaines mutations exigeront une destination explicite, un plan,
  une copie en staging et une vérification SHA-256 avant publication.
- Les sauvegardes sources et les snapshots approuvés ne seront jamais supprimés
  silencieusement.
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
`inspect` read-only ; B2 `sync` vérifié sans suppression ; B3 snapshots
immuables ; B4 restauration isolée ; B5 `doctor` et validation pilote.
La [roadmap originale](.plans/Savewarden_Plan_Bootstrap_Architecture_GitOps_v0.1.md)
et le [plan B1](.plans/Savewarden_Plan_B1_Manifeste_Inspect_v0.1.md) restent
les sources du périmètre, et le [changelog](CHANGELOG.md) décrit l'existant.

Licence [MIT](LICENSE), choisie explicitement pour ce bootstrap.

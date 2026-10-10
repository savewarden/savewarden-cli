# Savewarden

**Your progress. Always protected.**

Savewarden vise à inspecter, centraliser, vérifier et restaurer les sauvegardes
de jeux à partir de sources explicitement choisies. Le moteur sera indépendant
de la CLI, local-first et Windows-first.

**État actuel : bootstrap B0, version `dev`.** Seules l'aide et la version sont
implémentées. Aucun manifeste ni répertoire de sauvegarde n'est lu ou modifié.
Les commandes `inspect`, `sync`, `snapshot`, `restore` et `doctor` ne sont pas
encore disponibles. Aucune release ni installation via package manager.

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
`version --help` sont disponibles ; `--version` affiche la même version que
`version`. Succès : code **0**, sortie sur stdout. Erreur d'invocation ou
d'exécution : code **1**, diagnostic unique sur stderr, sans sortie de données
ni affichage automatique de l'aide. Pas de sortie JSON à ce stade.

Les sorties sont en anglais, UTF-8, sans couleur et terminées par LF. La version
de développement est explicite ; aucune date ou version calculée depuis Git.

## Sécurité et support

- Aucun accès aux sauvegardes dans B0. Les tests utilisent des données
  synthétiques et des dossiers temporaires.
- Les prochaines mutations exigeront une destination explicite, un plan,
  une copie en staging et une vérification SHA-256 avant publication.
- Les sauvegardes sources et les snapshots approuvés ne seront jamais supprimés
  silencieusement. Les liens/jonctions ne seront pas suivis par défaut.
- Une copie sur le même disque physique ne protège pas contre la panne du disque.

Windows est le pilote fonctionnel prévu. La CI compile et teste le bootstrap
sur Windows, Linux et macOS ; cela ne valide pas de futures opérations sur les
sauvegardes de jeux sur ces trois plateformes. Aucun binaire distribué pour
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
reste la source du périmètre, et le [changelog](CHANGELOG.md) décrit l'existant.

Licence [MIT](LICENSE), choisie explicitement pour ce bootstrap.

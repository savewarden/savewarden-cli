# Contribuer à Savewarden

Prérequis : Go **1.27.3**, Git. Outils de contrôle : golangci-lint **2.14.0**
et govulncheck **1.6.0**, exécutés depuis leurs modules Go versionnés. Les
outils ne font pas partie du binaire Savewarden.

## Workflow

Lire [AGENTS.md](AGENTS.md), [l'architecture](docs/architecture.md),
[la sécurité](docs/safety-model.md) et [GitOps](docs/gitops.md) avant de changer
les frontières ou de travailler sur des sauvegardes.

Une fois le commit initial B0 revu et intégré, créer `release/v0.1.0-a1`
depuis son SHA de `main`, puis les branches `feat/*`, `fix/*`, `docs/*` ou
`test/*` depuis cette release branch. Les PR de version ciblent cette branche.
Après fusion, supprimer la branche de travail sur le remote et en local.
Pas de push direct sur `main`, de merge automatique ou d'auto-approbation.
La PR finale vers `main` utilise un merge commit. Les tags et releases
nécessitent une validation humaine distincte.

Une PR livre ensemble code, tests, documentation, changelog et ADR si une
décision durable change. Décrire le comportement utilisateur et joindre les
résultats exacts. Ne pas intégrer les lots B1–B5 au bootstrap B0.

## Contrôles locaux

Exécuter depuis la racine avec la version Go fixée :

```sh
gofmt -w cmd internal .github/scripts
go mod tidy
git diff --exit-code -- go.mod go.sum
go mod verify
go vet ./...
go vet ./.github/scripts
go test ./...
go test ./.github/scripts
go run ./.github/scripts
go build -trimpath -o bin/savewarden ./cmd/savewarden
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 run ./... ./.github/scripts
go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...
```

Sur Linux avec CGO et un compilateur C :

```sh
go test -race ./...
go test -race ./.github/scripts
```

Le race detector Windows nécessite lui aussi une toolchain C compatible ;
s'il manque, la preuve vient du job Linux. Sur un poste Windows où le contrôle
intelligent des applications ou WDAC bloque les exécutables non signés, les
binaires de test fraîchement compilés peuvent être refusés : relancer le
package concerné, ou s'appuyer sur la CI, sans désactiver cette protection. Une cross-compilation depuis
Windows ne remplace jamais l'exécution des tests Linux/macOS.
La CI exécute les mêmes gates sur `ubuntu-24.04`, `windows-2025` et `macos-15`.

Au premier commit, `git diff --exit-code` ne peut pas constater des fichiers
non suivis. Comparer aussi les empreintes de `go.mod`/`go.sum` avant et après un
second `go mod tidy`. Les dépendances directes et indirectes sont versionnées
avec leurs empreintes dans `go.sum`.

## Tests et données

L'aide, la version, les erreurs, les codes de sortie et les commandes des
exemples README sont vérifiés par des tests CLI, dont un smoke du vrai binaire
dans un dossier temporaire comportant espaces et Unicode. Les règles d'import
et le garde de lecture seule vérifient les sources Go de production, y compris
celles sous build tags.

`inspect` est testé à trois niveaux : faux système de fichiers en mémoire,
qui injecte liens, verrous et changements concurrents sur tous les OS ;
vrais dossiers temporaires avec jonctions et verrous sous Windows ; binaire
réel lancé avec un environnement isolé et l'écriture refusée, suivi d'une
comparaison d'empreinte complète de l'arborescence.

`sync` est testé sur un faux coffre qui reproduit la sémantique de
`platform/vault` sur les trois OS, avec crash avant et après chaque appel au
coffre et pendant la récupération ; `platform/vault` est testé sur de vrais
dossiers Windows, jonctions et liens compris ; le binaire réel publie avec
l'écriture refusée sur l'installation, et est tué pendant un staging puis
récupéré, ce qui prend une quinzaine de secondes. Toute API d'écriture ajoutée
au garde exige un amendement d'ADR 0005.

Pour les futures fonctions : fixtures synthétiques, tempfiles, failure
injection, chemins absents/identiques/chevauchants, destination non vide,
collisions, Unicode, permissions, fichiers vides/verrouillés, grandes listes,
liens/jonctions et changements pendant copie. Ajouter les tests au moment
où le comportement existe ; ne pas créer de packages vides.

Ne jamais lire ou écrire les sauvegardes réelles dans un test. Aucun secret,
compte de joueur, `CPY.ini`, receipt réel ou save ne doit entrer dans Git.
La revue du staging reste obligatoire : `.gitignore` est une aide, pas une
barrière de confidentialité contre `git add -f`.

## Supply chain

Toutes les Actions utilisent un SHA complet vérifié. Les outils `go run`
utilisent une version exacte et la checksum database Go ; aucun script
téléchargé et exécuté à la volée. Le checker de pins inspecte les workflows
et possède des fixtures négatives. Tout ajout d'un outil binaire téléchargé
nécessitera une empreinte versionnée et un contrôle correspondant.

Le checker accepte les clés `uses` explicites en YAML block-style et refuse
les autres formes. C'est un contrôle mécanique de la surface B0, pas un
analyseur universel de shell/YAML : un nouvel installateur ou style de workflow
doit être revu et étendre le contrôle dans la même PR.

La CI dispose de `contents: read`, sans token de publication. Pas de
`pull_request_target`. Le pipeline release/GoReleaser sera ajouté seulement
quand les artefacts, SBOM et attestations seront réellement produits et testés.

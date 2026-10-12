# Changelog

Ce fichier est la source unique des futures notes GitHub Release.
Aucun tag ni release n'est publié par le bootstrap.

## Unreleased

### Added

- Commande `inspect --manifest <chemin>` : inventaire en lecture seule des
  sources d'un manifeste, avec SHA-256, taille et date de chaque fichier,
  sortie texte stable et codes de sortie 0, 1 et 2 (ADR 0003). Windows
  seulement ; refus explicite des chemins Windows sous Linux et macOS.
- Manifeste v1 strict (ADR 0002) : schéma fermé, grammaire canonique des
  chemins Windows, refus des alias, tags, clés dupliquées ou inconnues et
  booléens YAML 1.1, erreurs localisées par ligne, colonne et champ.
- Portée de lecture sans suivi de liens : jonctions, liens symboliques,
  points d'analyse et placeholders signalés et jamais traversés ; dossiers
  épinglés pendant l'inspection sous Windows ; fichier inventorié seulement
  si identité, taille et date sont stables autour de la lecture.
- Garde statique de lecture seule du code de production, liste blanche des
  dépendances par couche, `errorlint` et `gosec`.
- Dépendance `go.yaml.in/yaml/v3` v3.0.5, limitée à `platform/manifests`.
- Bootstrap B0 Go/Cobra : aide par défaut, `--help`, `help`, `version` et
  `--version`, version de développement `dev` et codes de sortie 0/1.
- Composition explicite `cmd/savewarden → internal/app → internal/cli`.
- Tests CLI, smoke du binaire, exemples README et frontières d'import.
- CI Windows/Linux/macOS, tidy, verify, format, vet, race Linux, lint,
  vulnérabilités et contrôle des pins Actions/outils.
- Architecture FSD-inspired Go, ADR initial, modèle de sécurité, GitOps,
  instructions agents et dossier historique du pilote Odyssey.
- Licence MIT choisie par le propriétaire du projet.

### Limits

- sync, snapshot, restore et doctor restent à implémenter ; pas de sortie JSON.
- L'inventaire n'est pas un instantané atomique ; les fichiers OneDrive et
  autres placeholders cloud sont refusés ; `subst` n'est pas détecté.
- Schéma de manifeste v1 provisoire jusqu'à `v0.1.0`.
- Aucun pipeline release ou package manager.

# Changelog

Ce fichier est la source unique des futures notes GitHub Release.
Aucun tag ni release n'est publié par le bootstrap.

## Unreleased

### Added

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

- Manifeste, inspect, sync, snapshot, restore et doctor restent à implémenter.
- Aucun accès aux sauvegardes réelles ; aucun pipeline release ou package manager.

# Changelog

Ce fichier est la source unique des futures notes GitHub Release.
Aucun tag ni release n'est publié par le bootstrap.

## Unreleased

### Added

- Commande `sync --manifest <chemin> --vault <dossier>`, **plan seulement** :
  ce que copierait une synchronisation vers un coffre, fichier par fichier,
  sans rien écrire, pas même le coffre. Codes de sortie 0, 1, 2 et 3, ce
  dernier quand le coffre demande une intervention (ADR 0007). Coffre
  distinct de l'installation, par nom et par identité ; dossier non vide sans
  marqueur refusé, jamais adopté ; `current` comparé exactement à son receipt ;
  source absente jamais interprétée comme une suppression. Windows seulement.
- Protocole de publication et de récupération (ADR 0006), référence de
  sécurité des écritures à venir, et frontières de `sync` (ADR 0004).
- Package `entities/backup` : types d'inventaire partagés, receipt v1 et
  marqueur du coffre relus strictement, journal d'opération chaîné et son
  automate, classement des observations de récupération, tous sans I/O.
  L'enveloppe E1 compte neuf temporaires par numéro d'enregistrement, toutes
  étapes confondues ; un temporaire abandonné à côté d'un enregistrement
  validé est accepté aux conditions exactes d'ADR 0006, au lieu de rendre le
  journal ambigu.
- `inspect` expose l'inventaire sans suivi d'un dossier quelconque ; sa
  sortie ne change pas. Les fichiers chiffrés par EFS sont signalés à `sync`.
- Documentation du [coffre](docs/vault.md), du [receipt v1](docs/receipt-v1.md)
  et de la [commande `sync`](docs/commands/sync.md) ; plan B2 v0.2 validé.
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

### Security

- Toolchain Go épinglée en 1.27.3 dans `go.mod`, donc en CI : corrige
  GO-2026-6809 dans `crypto/x509` de la bibliothèque standard, signalé par
  govulncheck au niveau module seulement ; le code ne l'appelle pas.

### Limits

- `sync` ne fait que planifier : `--apply` et `--recover` arrivent avec B2b
  et B2c ; le type de volume du coffre n'est pas encore contrôlé.
- snapshot, restore et doctor restent à implémenter ; pas de sortie JSON de
  la CLI.
- L'inventaire n'est pas un instantané atomique ; les fichiers OneDrive et
  autres placeholders cloud sont refusés ; `subst` n'est pas détecté.
- Schéma de manifeste v1 provisoire jusqu'à `v0.1.0`.
- Aucun pipeline release ou package manager.

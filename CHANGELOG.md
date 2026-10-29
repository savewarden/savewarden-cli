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
  Le plan qualifie le volume du coffre, NTFS sur lecteur local fixe ou
  amovible, sinon code 1, et exige l'espace de la génération complète plus
  max(64 Mio, 5 %) disponible pour l'utilisateur, sinon `insufficient-space`.
- Commande `sync --apply`, `--recover` et `--recover --apply`, avec la ligne
  d'état du coffre en dernière ligne de stderr (ADR 0007).
- Première publication par la feature `sync` : initialisation du coffre I2 à I9 en créations exclusives, verrou,
  état et espace relus sous verrou, journal validé par renommage avec
  l'enveloppe E1 et les retries bornés sur les erreurs 32 et 33, copie hachée
  avec détection de changement de la source, second inventaire, relecture du
  staging, bascule contrôlée par identité avant et après le renommage,
  receipt puis `published`. Énumération des crashs avant et après chaque
  appel au coffre.
- Rotation de `current` (B2c, ADR 0006 §5) : `current` reconnu par toute sa
  chaîne de preuve, receipt de tête, journal publié et son identité NTFS,
  puis relu en entier sous verrou avant `planned` et juste avant le retrait ;
  `planned` fixe l'identité de l'ancien `current` ; retrait vers
  `retained\<op>`, observation exacte et relecture de la génération retirée
  avant la promotion ; restauration vérifiée après une promotion refusée ;
  `rollback-failed` une seule fois. Aucune anomalie après le retrait ne
  déclenche de rollback conjectural : code 3, rien déplacé.
- Récupération d'une rotation interrompue, tableau fermé d'ADR 0006 §5,
  restaurations vérifiées comprises, et règle M3 d'une opération sans
  enregistrement : abandon seulement sans génération publiée attendue et
  sans `current`, ou à côté de la génération précédente reconnue.
- `--allow-removals` pour les fichiers retirés d'une source présente, et
  `--accept-missing-source <id>`, répétable, pour une source facultative
  absente que `current` contient ; anciens fichiers conservés dans
  `retained`, receipt `missing` et `accepted: true` ; tout autre usage, code 1.
- Un `current` d'une autre identité que celle publiée, par exemple dans un
  coffre copié ou déplacé, est refusé en code 3 sans écriture,
  `unexpected-entry`, sans conclure à une corruption.
- Récupération d'une première publication interrompue par la feature
  `sync` : plan de récupération sans
  écriture, observation des identités de `current` et du staging,
  classement d'ADR 0006, puis exécution sous verrou après relecture ;
  marqueur repris, `abandoned`, `rolled-back`, `current` revérifié puis
  receipt reconstruit et `published`, ou remis en staging ; `rollback-failed`
  enregistré une seule fois. Toute ambiguïté et tout dépassement de
  l'enveloppe arrêtent en code 3 sans écriture. La portée de lecture
  fournit l'identité d'un fichier sans le suivre.
- Protocole de publication et de récupération (ADR 0006), référence de
  sécurité des écritures à venir, et frontières de `sync` (ADR 0004).
- Écritures autorisées dans le seul package `internal/platform/vault`
  (ADR 0005) : garde statique assoupli pour ce répertoire exact, API de
  création exclusive nommées une à une, `MoveFileEx` seulement avec le
  littéral `0`, accès en écriture d'un `CreateFile` seulement avec
  `CREATE_NEW` ; interdits renforcés partout, vault compris, dont tout
  `MOVEFILE_*`, `ReplaceFile`, `Unlink`, `Rmdir`, `SetFileInformationByHandle`,
  `NtCreateFile`, `FILE_FLAG_DELETE_ON_CLOSE`, le chargement dynamique et les
  imports `unsafe` et `reflect`. `golang.org/x/sys/windows` n'est permis que
  dans ce package.
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
- Adaptateur `platform/vault`, Windows seulement, pas encore câblé à une
  commande : session de coffre en création seule, dossiers et fichiers
  `CREATE_NEW`, flush, date à 100 ns, renommage `MoveFileEx` sans drapeau,
  identité volume + index, verrou exclusif, type de volume et espace
  disponible pour l'appelant ; chaque dossier traversé ou créé est épinglé
  sans suivi de lien. Tests Windows réels, dont le renommage d'un enfant sous
  un parent épinglé et les codes d'erreur consignés dans ADR 0006.
- Dépendance `golang.org/x/sys` v0.49.0, package `windows` seul, limitée à
  `platform/vault`.
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

- Les générations remplacées s'accumulent dans `retained` : aucune
  suppression ni rétention automatique en B2. Pendant une rotation, `current`
  est absent un court instant. Le verrou n'exclut qu'une autre opération
  Savewarden : une écriture d'un autre programme entre une relecture et le
  renommage suivant n'est pas détectée. Copier ou déplacer un coffre n'est pas
  pris en charge : refus en code 3. `current-exists` n'est plus émis.
- ReFS n'est pas encore qualifié ; la durabilité d'un renommage après une
  coupure de courant n'est pas garantie par Windows, la récupération repose
  sur l'observation.
- snapshot, restore et doctor restent à implémenter ; pas de sortie JSON de
  la CLI.
- L'inventaire n'est pas un instantané atomique ; les fichiers OneDrive et
  autres placeholders cloud sont refusés ; `subst` n'est pas détecté.
- Schéma de manifeste v1 provisoire jusqu'à `v0.1.0`.
- Aucun pipeline release ou package manager.

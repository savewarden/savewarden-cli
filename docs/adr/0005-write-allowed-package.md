# ADR 0005 — Écritures autorisées : `internal/platform/vault` seul

Date : 29 octobre 2026. Statut : accepté pour B2, plan B2 v0.2 §6.2, validé
par le propriétaire le 29 octobre 2026 ; introduit par le premier commit de
B2b, après le GO B2b.

## Contexte

De B0 à B2a, tout le code de production est en lecture seule. Le garde
statique `internal/app/readonly_test.go` le vérifie par les noms : il refuse
les API connues d'écriture, d'exécution, de réseau et de chargement
dynamique, ainsi que les modes d'ouverture en écriture. B2b publie pour la
première fois des fichiers dans un coffre, selon le protocole d'ADR 0006 :
création exclusive, journal validé par renommage, aucun remplacement, aucune
suppression. Il faut un endroit, et un seul, où ces écritures sont permises,
avec les API les plus étroites possibles.

## Décision

**Un seul package écrit : `internal/platform/vault`.** L'exception porte sur
ce répertoire exact : ni ses sous-packages, ni un nom voisin comme
`internal/platform/vaultx`, ni aucun autre package. Tous les autres gardent
le garde de B1, auquel s'ajoutent les interdits renforcés ci-dessous.

| Règle | Portée |
| --- | --- |
| Imports interdits `os/exec`, `net`, `net/http`, `plugin` | Partout, `platform/vault` compris |
| Interdits de B1 et interdits renforcés | Partout, `platform/vault` compris |
| API permises | `platform/vault` seulement |
| `MoveFileEx` | `platform/vault` seulement, en appel direct avec le littéral `0` |
| `golang.org/x/sys/windows` | `platform/vault` seulement, ce package exact |

**API permises dans `platform/vault`**, chacune pour un besoin du protocole :

| API | Besoin |
| --- | --- |
| `GENERIC_WRITE` | Ouvrir en écriture un fichier que l'on crée, toujours avec `CREATE_NEW` |
| `FILE_WRITE_ATTRIBUTES` | Dater un fichier ou un dossier que le coffre vient de créer |
| `CREATE_NEW` | Seule disposition qui crée : échec si le nom existe |
| `CreateDirectory` | Créer un dossier, un par un : échec s'il existe |
| `FlushFileBuffers` | Rendre durable un temporaire avant son renommage, et chaque fichier copié |
| `SetFileTime` | Reporter la date de modification de la source sur le fichier créé, par son handle |
| `MoveFileEx` | Seul renommage : drapeaux `0`, donc ni remplacement d'une cible existante, ni copie entre volumes |

**Interdits renforcés, partout, `platform/vault` compris :** `Remove`,
`RemoveAll`, `DeleteFile`, `RemoveDirectory`, `Rename`, `MoveFile`,
`MoveFileWithProgress`, `MoveFileTransacted`, `ReplaceFile`, tout
`MOVEFILE_*`, dont `MOVEFILE_REPLACE_EXISTING`, `MOVEFILE_COPY_ALLOWED`,
`MOVEFILE_DELAY_UNTIL_REBOOT`, `MOVEFILE_CREATE_HARDLINK` et
`MOVEFILE_WRITE_THROUGH`, `Truncate`, `Ftruncate`, `SetEndOfFile`,
`SetFileValidData`, `O_TRUNC`, `O_APPEND`, `CREATE_ALWAYS`, `OPEN_ALWAYS`,
`TRUNCATE_EXISTING`, `FILE_FLAG_DELETE_ON_CLOSE`, `DELETE`,
`SetFileInformationByHandle`, `NtCreateFile`, `NtSetInformationFile`,
`NtSetEaFile`, `Link`, `Symlink`, `CreateSymbolicLink`, `CreateHardLink`,
`DeviceIoControl`, `SetFileAttributes`, `Chmod`, `Chown` et leurs variantes,
`Utimes`, `SetSecurityInfo`, `SetNamedSecurityInfo`, `WRITE_DAC`,
`WRITE_OWNER`, les points de montage, l'exécution de processus, le
chargement dynamique, dont `NewLazySystemDLL` et `LoadLibraryEx`, et les
appels système bruts. La liste exacte est celle du garde. Restent aussi
interdits dans `platform/vault` les API de B1 qui créent ou écrivent sans
`CREATE_NEW` : `os.Create`, `os.OpenFile`, `os.WriteFile`, `os.Mkdir`,
`O_CREATE`, `O_EXCL`, `O_WRONLY`, `O_RDWR`, `Chtimes`. Le coffre écrit le
contenu d'un fichier par le handle qu'il vient de créer, jamais par une API
qui ouvrirait un fichier existant.

**Règle d'appel de `MoveFileEx`.** Un sélecteur `MoveFileEx` n'est accepté
que comme fonction d'un appel direct à trois arguments dont le troisième est
le littéral entier `0`. Sont refusés : un autre littéral, `0x0` ou `(0)`
compris, une variable, une constante nommée, une expression, des arguments
étalés, et toute valeur de fonction qui pourrait être appelée autrement. Le
garde le vérifie sur l'arbre syntaxique.

**`golang.org/x/sys/windows`.** Le test d'imports ne l'autorise que dans
`internal/platform/vault`, ce répertoire exact, et seulement ce package :
pas `x/sys/windows/registry`, `svc` ou `x/sys/unix`. Il apporte `MoveFileEx`
et les requêtes en lecture seule `GetVolumeInformation`, `GetDriveType` et
`GetDiskFreeSpaceEx`, absentes de `syscall`. Sa version est exacte, sans
plage, épinglée dans `go.mod` avec ses empreintes dans `go.sum` : v0.49.0,
publiée le 6 octobre 2026. Elle entre dans `go.mod` au commit qui l'importe
pour la première fois, l'adaptateur, puisque `go mod tidy` retire une
dépendance que rien n'importe.

**Fichiers par OS.** `platform/vault` est réservé à Windows. POSIX
`rename(2)` remplace sa cible : un fichier pour les autres systèmes renvoie
`errors.ErrUnsupported`. La logique de `features/sync` est testée sur un faux
coffre sur les trois OS.

**Changer cette liste** exige un amendement revu de cet ADR, avec ses tests,
avant le code. B2c n'ajoute aucune API : `readonly_test.go` ne change pas
après B2b.

## Alternatives

- **Assouplir le garde pour `features/sync` ou pour toute la couche
  `platform`** : la surface d'écriture s'étendrait à du code qui n'en a pas
  besoin ; un seul adaptateur suffit et se revoit entièrement.
- **Permettre `os.OpenFile` avec `O_CREATE|O_EXCL`** : correct à l'exécution,
  mais le garde, fondé sur les noms, ne distingue pas ces drapeaux de
  `O_TRUNC` dans un même appel ; `CREATE_NEW` est explicite et seul permis.
- **`os.Root` pour créer et renommer** : `Root.Rename` remplace une cible
  existante et `os.Root` suit les liens internes ; il ne peut être l'unique
  garantie de non-suivi.
- **`os.Rename`, `MoveFile`, `MoveFileEx` avec `MOVEFILE_REPLACE_EXISTING`
  ou `MOVEFILE_COPY_ALLOWED`** : remplacement ou copie suivie d'une
  suppression ; refusés partout (ADR 0006).
- **`MOVEFILE_WRITE_THROUGH` pour la durabilité** : sa documentation ne vise
  que les déplacements par copie ; la durabilité passe par `FlushFileBuffers`
  avant le renommage, et ses limites sont déclarées par ADR 0006 §6.
- **Renommage par handle, `SetFileInformationByHandle` avec
  `FileRenameInfo`** : la même API supprime par `FileDispositionInfo` ;
  refusée partout.
- **Analyse des drapeaux par type plutôt que par littéral** : exigerait le
  chargement des types et accepterait des constantes nommées dont la valeur
  peut changer ; le littéral `0` se vérifie sans ambiguïté.

## Conséquences et validation

- `platform/vault` devient le package le plus sensible du produit. Sa revue
  indépendante est obligatoire avant le Human GO de B2b et consignée dans
  `docs/validation/b2b.md` : écritures Windows, accès concurrents, journal,
  récupération.
- Tests du garde : chaque API permise est refusée hors de `platform/vault`,
  dans `internal/platform/vaultx`, dans un sous-package et dans les autres
  couches ; chaque interdit, y compris chaque `MOVEFILE_*` et chaque import
  interdit, est refusé dans `platform/vault` ; `MoveFileEx` est refusé avec
  tout autre troisième argument que le littéral `0`, et comme valeur de
  fonction. Le test d'imports refuse `golang.org/x/sys/windows` hors de
  `platform/vault`, et ses packages voisins partout.
- Le garde reste un filet statique fondé sur les noms : il ne prouve pas
  l'absence d'effet indirect. Il est complété par les tests dynamiques de
  création exclusive, cible existante refusée sans changement d'état, de
  non-mutation de la source, binaire lancé avec la source en écriture
  refusée, et d'absence d'écriture du mode plan.

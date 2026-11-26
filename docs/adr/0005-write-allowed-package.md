# ADR 0005 — Écritures autorisées : `internal/platform/vault` seul

Date : 29 octobre 2026. Statut : accepté pour B2, plan B2 v0.2 §6.2, validé
par le propriétaire le 29 octobre 2026 ; introduit par le premier commit de
B2b, après le GO B2b. Amendement de B5a (S-01 et S-02), rédigé le
26 novembre 2026 avec les résultats des hypothèses HW1 à HW13 :
**accepté** par le propriétaire le 26 novembre 2026, dans sa révision r3,
avant tout code de production de B5a (plan B5 v0.5 §3.5 et §8, commit 2).

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
| Imports interdits `os/exec`, `net` et tout `net/…`, `crypto/tls`, `plugin`, `unsafe`, `reflect` | Partout, `platform/vault` compris |
| Directive `//go:linkname` ; sources assembleur, cgo ou objets (`.s`, `.syso`, `.c`…) | Partout |
| Interdits de B1 et interdits renforcés | Partout, `platform/vault` compris |
| API permises | `platform/vault` seulement |
| `MoveFileEx` | `platform/vault` seulement, en appel direct avec le littéral `0` |
| Accès d'écriture d'un `CreateFile` | `platform/vault` seulement, et seulement avec `CREATE_NEW` |
| `golang.org/x/sys/windows` | `platform/vault` seulement, ce package exact |

**API permises dans `platform/vault`**, chacune pour un besoin du protocole :

| API | Besoin |
| --- | --- |
| `GENERIC_WRITE` | Ouvrir en écriture un fichier que l'on crée, toujours avec `CREATE_NEW` |
| `FILE_WRITE_ATTRIBUTES` | Dater un fichier que le coffre crée, avec `CREATE_NEW` |
| `CREATE_NEW` | Seule disposition qui crée : échec si le nom existe |
| `CreateDirectory` | Créer un dossier, un par un : échec s'il existe |
| `FlushFileBuffers` | Rendre durable un temporaire avant son renommage, et chaque fichier copié |
| `SetFileTime` | Reporter la date de modification de la source sur le fichier créé, par son handle |
| `MoveFileEx` | Seul renommage : drapeaux `0`, donc ni remplacement d'une cible existante, ni copie entre volumes |

**Interdits renforcés, partout, `platform/vault` compris :** `Remove`,
`RemoveAll`, `DeleteFile`, `RemoveDirectory`, `Unlink`, `Rmdir` et leurs
variantes `…at`, `Rename`, `Renameat`, `MoveFile`, `MoveFileWithProgress`,
`MoveFileTransacted`, `ReplaceFile`, `Exchangedata`, tout `MOVEFILE_*`, dont
`MOVEFILE_REPLACE_EXISTING`, `MOVEFILE_COPY_ALLOWED`,
`MOVEFILE_DELAY_UNTIL_REBOOT`, `MOVEFILE_CREATE_HARDLINK` et
`MOVEFILE_WRITE_THROUGH`, `Truncate`, `Ftruncate`, `SetEndOfFile`,
`SetFileValidData`, `Mknod`, `Mkfifo`, `O_TRUNC`, `O_APPEND`,
`CREATE_ALWAYS`, `OPEN_ALWAYS`, `TRUNCATE_EXISTING`,
`FILE_FLAG_DELETE_ON_CLOSE`, les droits `DELETE`, `WRITE_DAC`, `WRITE_OWNER`,
`FILE_GENERIC_WRITE`, `MAXIMUM_ALLOWED`, `STANDARD_RIGHTS_REQUIRED`,
`STANDARD_RIGHTS_ALL`, `SPECIFIC_RIGHTS_ALL` et `ACCESS_SYSTEM_SECURITY`,
`SetFileInformationByHandle`, `NtCreateFile`, `NtSetInformationFile`,
`NtSetEaFile`, `Link`, `Symlink`, `CreateSymbolicLink`, `CreateHardLink`,
`DeviceIoControl`, `SetFileAttributes`, `Chmod`, `Chown` et leurs variantes,
`Utimes` et ses variantes, les attributs étendus, `SetSecurityInfo`,
`SetNamedSecurityInfo`, `SetKernelObjectSecurity`, les points de montage,
`DefineDosDevice`, `SetVolumeLabel`, l'exécution de processus et de
services, `WriteProcessMemory`, les sockets, le chargement dynamique, dont
`LazyDLL`, `NewProc`, `FindProc` et ses variantes par ordinal,
`NewLazySystemDLL` et `LoadLibraryEx`, l'appel par nom `MethodByName`, et
tous les appels système bruts, `Syscall15`, `Syscall18` et `RawSyscall`
compris. La liste exacte est celle du garde. Restent aussi interdits dans
`platform/vault` les API de B1 qui créent ou écrivent sans `CREATE_NEW` :
`os.Create`, `os.OpenFile`, `os.WriteFile`, `os.Mkdir`, `O_CREATE`, `O_EXCL`,
`O_WRONLY`, `O_RDWR`, `Chtimes`. Le coffre écrit le contenu d'un fichier par
le handle qu'il vient de créer, jamais par une API qui ouvrirait un fichier
existant.

**Règle d'appel de `MoveFileEx`.** Un sélecteur `MoveFileEx` n'est accepté
que comme fonction d'un appel direct à trois arguments dont le troisième est
le littéral entier `0`. Sont refusés : un autre littéral, `0x0` ou `(0)`
compris, une variable, une constante nommée, une expression, des arguments
étalés, et toute valeur de fonction qui pourrait être appelée autrement. Le
garde le vérifie sur l'arbre syntaxique.

**Règle d'ouverture.** Un `CreateFile` n'est accepté qu'en appel direct à
sept arguments. Son accès est une combinaison par `|` de droits nommés :
`GENERIC_READ`, `FILE_READ_ATTRIBUTES`, `FILE_READ_DATA`,
`FILE_LIST_DIRECTORY`, `READ_CONTROL`, `SYNCHRONIZE`, et dans
`platform/vault` seulement `GENERIC_WRITE` et `FILE_WRITE_ATTRIBUTES`. Sa
disposition est `OPEN_EXISTING`, ou `CREATE_NEW` dans `platform/vault`
seulement ; un droit d'écriture exige `CREATE_NEW`. Aucun littéral, aucune
variable, aucune constante locale n'est accepté à ces deux places. Les
drapeaux d'un `Open` à trois arguments ou d'un `Openat` sont de même des
drapeaux de lecture nommés : `O_RDONLY`, `O_NOFOLLOW`, `O_CLOEXEC`,
`O_NONBLOCK`, `O_DIRECTORY`, `O_NOCTTY`, `O_LARGEFILE`. Ainsi rien ne peut
rouvrir en écriture un fichier existant, enregistrement validé, receipt ou
fichier publié, ni en changer la date.

**`golang.org/x/sys/windows`.** Le test d'imports ne l'autorise que dans
`internal/platform/vault`, ce répertoire exact, et seulement ce package :
pas `x/sys/windows/registry`, `svc` ou `x/sys/unix`. Il apporte `MoveFileEx`
et les requêtes en lecture seule `GetVolumeInformation`, `GetDriveType` et
`GetDiskFreeSpaceEx`, absentes de `syscall`. Sa version est exacte, sans
plage, épinglée dans `go.mod` avec ses empreintes dans `go.sum` : v0.49.0,
publiée le 6 octobre 2026. Elle entre dans `go.mod` au commit qui l'importe
pour la première fois, l'adaptateur, puisque `go mod tidy` retire une
dépendance que rien n'importe.

**Requêtes en lecture seule ajoutées par B5a** (amendement accepté, S-01 et
S-02 ; ADR 0006 §11, « Lettres de lecteur, alias et volumes »). Elles
s'ajoutent à `GetVolumeInformation`, `GetDriveType` et `GetDiskFreeSpaceEx`.
Aucune n'écrit. Chacune répond à un besoin de la session, et ses preuves sont
consignées dans `docs/validation/b5a.md` :

| Requête | Besoin | Preuves |
| --- | --- | --- |
| `GetFinalPathNameByHandle` | Chemin de volume `\\?\Volume{GUID}\…` d'un handle : base de la session, contrôle de chaîne, chemin final de chaque nouveau handle, détection S-02 | HW1, HW3, HW8, HW9, HW12 |
| `GetVolumeNameForVolumeMountPoint` | Contre-épreuve S-02 : la racine de la lettre est le point de montage d'un volume, de même GUID que son chemin final | HW12, HW13 |
| `GetVolumeInformationByHandle` | Système de fichiers du volume tenu, lu sur l'épingle racine | HW10 |

- **Drapeaux.** `GetFinalPathNameByHandle` ne reçoit qu'une constante locale
  `0x1`, `VOLUME_NAME_GUID` avec `FILE_NAME_NORMALIZED`, que x/sys ne nomme
  pas. Aucune autre valeur n'est employée pour construire une base.
- **Tampon.** Sa taille vient d'une première requête. Un résultat n'est
  accepté que si la taille rendue est strictement inférieure à la capacité
  du tampon ; égale ou supérieure, c'est une troncature, refusée. Elle ne se
  signale par aucune erreur (HW1, 142 caractères demandés pour un tampon de
  8).
- **Formes d'arguments.** `GetDriveType` et `GetDiskFreeSpaceEx`, déjà
  permises, reçoivent aussi `\\?\Volume{GUID}\` et rendent les mêmes valeurs
  que par la lettre (HW10). Ce n'est pas une API nouvelle.
- **Aucune API d'écriture n'est ajoutée.** La table des sept API permises,
  les interdits renforcés, les règles d'ouverture et d'appel de `MoveFileEx`
  ne changent pas. Ouvrir `X:\` ou `\\?\Volume{GUID}\` se fait avec les
  droits déjà nommés, `GENERIC_READ` ou `FILE_READ_ATTRIBUTES`, en
  `OPEN_EXISTING`, comme l'ont fait les tests HW. Les sélecteurs du garde
  `readonly_test.go` restent inchangés.
- **Aucun chemin à lettre après `Open`.** La session ne résout une lettre
  qu'à son ouverture. Ensuite, chaque écriture et chaque lecture de décision
  passent par la base, chemin de volume tiré de ses épingles. Une
  instrumentation interne au package le vérifie en test.
- **`DefineDosDevice` et `subst`** restent interdits au code de production.
  Seuls les tests d'hypothèses, fichiers `_test.go` hors de la portée du
  garde, définissent des lettres et des noms DOS, sous la règle des lettres
  d'`AGENTS.md`.
- **Modèle de menace.** Savewarden protège ses écritures contre une lettre ou
  un alias réattribué par accident ou par un outil bénin. Il ne protège pas
  contre un processus hostile du même compte, qui peut de toute façon écrire
  dans le coffre. Le chemin de volume est résolu par `\??\`, qui consulte
  d'abord les noms DOS de la session de connexion : un nom local
  `Volume{GUID}` détourne les appels (HW8). ADR 0006 §11 prévoit des
  contrôles de la session qui doivent le détecter après coup, sans le
  confiner. Ce sont des garanties à implémenter et à prouver en B5a, pas des
  protections déjà disponibles.

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
  fonction ; un `CreateFile` en écriture sans `CREATE_NEW`, ou dont l'accès
  ou la disposition est numérique, est refusé. Les listes attendues, les sept
  API permises et les interdits renforcés, sont écrites dans les tests à part
  du garde : modifier le garde seul fait échouer un test. Le test d'imports
  refuse `golang.org/x/sys/windows` hors de `platform/vault`, et ses packages
  voisins partout.
- La revue indépendante de cet assouplissement, point de contrôle du plan
  B2 §8.2, a conduit aux règles d'ouverture, aux imports `unsafe` et
  `reflect`, à `go:linkname` et aux interdits ajoutés ci-dessus, avant tout
  code d'écriture.
- Le garde reste un filet statique fondé sur les noms : il ne prouve pas
  l'absence d'effet indirect. Restent hors de sa portée, par exemple, un
  drapeau numérique dans les attributs d'un `CreateFile`, ou une valeur
  calculée hors de l'appel. Il est complété par les tests dynamiques de
  création exclusive, cible existante refusée sans changement d'état, de
  non-mutation de la source, binaire lancé avec la source en écriture
  refusée, et d'absence d'écriture du mode plan.
- Les requêtes ajoutées par B5a ne sont pas des écritures : le garde ne les
  restreint pas, et rien ne l'y oblige. Leur usage est tenu par les tests de
  la session : instrumentation sans chemin à lettre après `Open`, troncature
  refusée, et tests réels de S-01 et S-02 du plan B5 §9.

## Amendements

- **26 novembre 2026, B5a, accepté** : requêtes en lecture seule
  `GetFinalPathNameByHandle`, `GetVolumeNameForVolumeMountPoint` et
  `GetVolumeInformationByHandle`, constante locale `0x1`, règle « aucun
  chemin à lettre après `Open` », phrase de modèle de menace ; rédigé avec
  les résultats de HW1 à HW13 (run 45297320109 sur `d2b79f4`, protocole HW7
  du propriétaire du 26 novembre 2026). Aucune API d'écriture ajoutée.
  Révisé le même jour sur la revue CTO : troncature refusée dès que la
  taille rendue atteint la capacité du tampon ; contrôles de HW8 écrits
  comme garanties à prouver. **Ratifié par le propriétaire le 26 novembre
  2026**, dans la révision r3 (blob `8653f03` avant l'enregistrement de ce
  statut), après sa revue finale : « Je ratifie les amendements d'ADR 0005
  et d'ADR 0006 §11 ainsi que l'erratum E2 au plan B5 v0.5, dans leur
  révision r3. » Ce n'est pas un GO de commit, de push ni de code.

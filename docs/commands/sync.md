# `savewarden sync`

Planifie, publie ou récupère la copie des sources d'un
[manifeste v1](../manifest-v1.md) vers un [coffre](../vault.md).

```powershell
savewarden sync --manifest 'D:\Manifests\mon-jeu.yaml' --vault 'F:\Coffre'
savewarden sync --manifest 'D:\Manifests\mon-jeu.yaml' --vault 'F:\Coffre' --apply
savewarden sync --manifest 'D:\Manifests\mon-jeu.yaml' --vault 'F:\Coffre' --recover
savewarden sync --manifest 'D:\Manifests\mon-jeu.yaml' --vault 'F:\Coffre' --recover --apply
savewarden sync --manifest 'D:\Manifests\mon-jeu.yaml' --vault 'F:\Coffre' --apply --allow-removals
savewarden sync --manifest 'D:\Manifests\mon-jeu.yaml' --vault 'F:\Coffre' --apply --accept-missing-source profil
```

`--manifest` et `--vault` sont obligatoires. `--vault` est un chemin Windows
absolu avec lettre de lecteur, selon la grammaire du manifeste : chemins UNC,
relatifs ou avec variables refusés, rien n'est nettoyé ni complété. Fermez le
jeu et son launcher avant de planifier.

| Mode | Effet |
| --- | --- |
| Sans flag | Plan seulement : rien n'est écrit, pas même le coffre |
| `--apply` | Copie, vérifie et publie une nouvelle génération du jeu ; un `current` existant, reconnu, est renommé dans `retained\<op>` et conservé |
| `--recover` | Montre comment une opération interrompue serait récupérée, sans rien écrire |
| `--recover --apply` | Exécute cette récupération |
| `--allow-removals` | Accepte les fichiers retirés d'une source **présente** ; ils restent dans la génération conservée |
| `--accept-missing-source <id>` | Accepte qu'une source **facultative**, que `current` contient, soit absente ; une source par occurrence, répétable |

Aucun des deux drapeaux ne couvre une source requise absente ni une source
retirée du manifeste, et aucun n'accorde la permission de l'autre. Nommer avec
`--accept-missing-source` une source requise, présente, inconnue, retirée du
manifeste, absente de la génération courante ou deux fois la même est une
invocation invalide, code 1, rien n'est écrit ; `--recover` refuse les deux
drapeaux. Le contrat complet de la commande est fixé par
[ADR 0007](../adr/0007-sync-contract-receipt-v1.md), le protocole d'écriture
par [ADR 0006](../adr/0006-publication-recovery-protocol.md).

## Garanties

- **Le plan n'écrit rien.** Il ne crée, ne modifie, ne déplace, ne supprime
  et n'exécute rien : ni dans les sources, ni dans le coffre, qui n'est même
  pas créé s'il n'existe pas. Le vrai binaire est testé avec l'écriture
  refusée par ACL sur tout l'arbre, empreinte complète inchangée.
- **`--apply` n'écrit que dans le coffre**, en créations exclusives : aucun
  fichier existant n'est ouvert en écriture, remplacé, tronqué ni supprimé,
  et un renommage n'écrase jamais rien. Les sources ne sont que lues ; le
  vrai binaire publie avec l'écriture refusée par ACL sur l'installation.
- **Vérifié avant publication.** Chaque fichier est haché pendant la copie,
  la source est inventoriée une seconde fois, et le staging est relu depuis
  le disque avant de devenir `current`.
- **Rien n'est perdu à la rotation.** L'ancienne génération n'est remplacée
  que si elle est reconnue : son receipt, le journal qui l'a publiée et son
  identité NTFS concordent, et son contenu est relu en entier. Elle est
  renommée dans `retained\<op>`, relue encore avant la promotion, et
  n'est jamais supprimée.
- **Aucun lien suivi.** Les sources sont inventoriées par le parcours
  d'[`inspect`](inspect.md) ; le coffre est lu de la même façon. Liens,
  jonctions, points d'analyse et placeholders sont signalés, jamais traversés.
- **Coffre distinct.** Le coffre ne peut ni être l'installation, ni la
  contenir, ni s'y trouver, même sous un autre nom, par exemple un nom court
  8.3.
- **Aucune adoption.** Un dossier non vide qui n'est pas un coffre Savewarden
  est refusé, jamais adopté.
- **Sortie stable.** Deux plans sur un état inchangé sont identiques, octet
  pour octet.

## Sortie

Texte anglais UTF-8, sans couleur, fins de ligne LF. Exemple, chemins fictifs,
coffre sur le volume de la source :

```text
manifest: D:\Manifests\synthetic-game.yaml
game: synthetic-game (Synthetic Game)
vault: D:\Vault (will be created)
destination: D:\Vault\games\synthetic-game\current
volume: NTFS, fixed drive, 67108870 bytes needed
current: none
pending: none
mode: plan only, nothing written
plan: changes

source saves (save, required): D:\Games\Synthetic Game\saves
  status: planned
  files: 2  bytes: 6  added: 2  modified: 0  removed: 0  unchanged: 0
  + 7692c3ad3540bb803c020b3aee66cd8887123234ea0c6e7143c0add73ff431ed  3  2026-10-09T16:32:08.1234567Z  1.save
  + 3fc4ccfe745870e2c0d99f71f30ff0656c8dedd41cc1d7d3d376b0dbe685e2f3  3  2026-10-09T16:32:08.1234567Z  Slot 1/2.save

findings: 0 blocking, 1 info
  info  same-volume  saves  D:\  vault and source share a volume; not an off-disk backup
```

| Ligne | Sens |
| --- | --- |
| `vault` | Chemin et état : `will be created`, `empty, will be initialized`, `initialized` ou `initialization interrupted` |
| `destination` | Emplacement de la génération courante du jeu dans le coffre |
| `volume` | Système de fichiers et type de lecteur du coffre, contrôlés avant tout : NTFS sur un lecteur local fixe ou amovible, sinon code 1 ; puis l'espace nécessaire, la taille complète de la nouvelle génération plus max(64 Mio, 5 %). L'espace disponible n'est pas affiché : il varierait d'un plan à l'autre |
| `current` | Opération qui a produit la génération courante, reconnue, `none`, ou `not checked, intervention required` quand une intervention arrête le plan |
| `pending` | Opérations interrompues, avec leur dernière étape validée, ou `unknown, ambiguous journal` |
| `plan` | `changes`, `up to date`, `blocked` ou `intervention required` |

Chaque fichier est préfixé par `+` ajouté, `~` modifié, `-` retiré ou `=`
inchangé par rapport à la génération courante, puis suivi de son SHA-256, de
sa taille, de sa date de modification UTC et de son chemin relatif à la source.
Un nom qui pourrait imiter une ligne est cité entre guillemets, comme pour
`inspect`.

Quand le coffre demande une intervention, aucune source n'est planifiée et un
bloc `interventions` précède les anomalies :

```text
interventions: 1
  pending-operation  20261105T101500-1a2b3c4d  last committed step verified; run sync --recover to see how it would be recovered
```

## Anomalies

| Code | Sévérité | Signification |
| --- | --- | --- |
| `source-missing` | bloquante si `required: true`, sinon information | Source absente |
| `source-vanished` | bloquante ; information si `--accept-missing-source` la nomme | Source facultative absente alors que la génération courante la contient : une absence n'est jamais une suppression |
| `source-removed-from-manifest` | bloquante, toujours | Source de la génération courante qui n'est plus dans le manifeste |
| `files-removed` | bloquante ; information avec `--allow-removals` | Fichier ou dossier de la génération courante absent de la source |
| `unportable-name` | bloquante | Nom que Win32 altérerait : point ou espace final, nom réservé, caractère interdit |
| `encrypted-source` | bloquante | Fichier chiffré par EFS, jamais copié en clair |
| `current-modified` | bloquante | La génération courante ne correspond plus exactement à son receipt, n'en a pas, ou le receipt n'a plus de génération |
| `long-path` | information | Chemin au-delà de 259 caractères dans le coffre |
| `same-volume` | information | Coffre et source sur le même volume : ce n'est pas une sauvegarde hors disque |
| `insufficient-space` | bloquante | Espace disponible pour l'utilisateur, quotas compris, inférieur à l'espace nécessaire ; ou taille qui déborderait 64 bits |

Les anomalies du parcours d'`inspect` s'appliquent aussi : `link-not-followed`,
`unreadable`, `changed-during-read`, `case-collision`, etc. Une anomalie sans
source porte `-` à sa place.

| Intervention | Signification |
| --- | --- |
| `pending-operation` | Une opération s'est interrompue ; `--recover` montre sa récupération, `--recover --apply` l'exécute |
| `initialization-interrupted` | La création du coffre s'est interrompue ; seul `--recover --apply` la reprend |
| `unexpected-entry` | Une entrée n'a pas sa place dans le coffre, ou un staging, une génération conservée n'est expliqué par aucun journal ni receipt ; ou `current` n'est pas le dossier que son journal a publié, par exemple dans un coffre copié ou déplacé |
| `journal-ambiguous` | Un journal est incomplet, corrompu ou incohérent, ou deux opérations sont en attente |

Voir le [guide du coffre](../vault.md) pour chacun de ces cas.

## Publier : `--apply`

`--apply` refait le plan, puis, s'il n'a aucune anomalie bloquante :

1. crée le coffre et son marqueur s'il le faut, puis `games\<id>`, et prend
   le verrou `sync.lock` : une seconde synchronisation du même jeu échoue,
   `another sync is running` ;
2. relit le coffre sous verrou, qui doit être tel que planifié, sinon
   `vault-changed`, code 1 ; revérifie l'espace, sinon `insufficient-space`,
   code 2 ;
3. ouvre un journal d'opération, `journal\<op>`, dont chaque étape est
   validée avant l'action qu'elle protège ;
4. copie chaque fichier vers `staging\<op>`, en le hachant, et refuse tout
   fichier source changé depuis le plan, `changed-during-sync`, code 2 ;
5. inventorie la source une seconde fois, puis relit le staging depuis le
   disque, `verification-mismatch` en cas d'écart, code 2 ;
6. renomme `staging\<op>` en `current`, avec contrôle d'identité avant et
   après, écrit le receipt, puis clôt le journal.

Quand une génération existe déjà, `--apply` la remplace par **rotation** :

- sous verrou, avant tout journal, `current` doit encore être la génération
  reconnue par le plan, même identité, et son contenu est relu en entier :
  identité changée, code 3 ; contenu changé, `current-modified`, code 2 ;
- le journal fixe l'identité de l'ancien `current` ; juste avant le retrait,
  il est relu une dernière fois ;
- l'ancien `current` est renommé dans `retained\<op précédente>` ; avant la
  promotion, `current` doit être absent, le staging intact, et la génération
  retirée relue en entier contre son receipt ; tout écart arrête en code 3
  sans rien enregistrer ni déplacer ;
- le staging devient `current`, puis le receipt, qui nomme l'opération
  précédente, est écrit.

Si le retrait est refusé, rien n'a bougé : `rolled back`, code 1. Si la
promotion est refusée, l'ancienne génération est relue, puis remise en
place : `rolled back`, code 1 ; si cette remise est refusée aussi, les deux
générations restent intactes à leur place, code 3. Un objet apparu sous
`current` n'est jamais écrasé, et l'ancienne génération n'est pas remise par
dessus.

Un plan déjà à jour n'écrit rien et sort en code 0. Après le plan, stdout
ajoute, par exemple :

```text
operation: 20261105T101500-1a2b3c4d
verification: 2 files, 6 bytes, SHA-256 verified
published: F:\Coffre\games\synthetic-game\current
receipt: F:\Coffre\games\synthetic-game\receipts\20261105T101500-1a2b3c4d.json
```

Après une rotation, une dernière ligne nomme la génération conservée :
`retained: F:\Coffre\games\synthetic-game\retained\<op précédente>`.

La dernière ligne de stderr dit ce que contient le coffre, quel que soit le
code de sortie :

| Ligne | Sens |
| --- | --- |
| `savewarden: vault: not modified` | Rien n'a été écrit |
| `savewarden: vault: metadata only; nothing staged or published` | Seuls le coffre, son marqueur, `games`, le dossier du jeu ou le verrou ont été créés |
| `savewarden: vault: current unchanged; operation <op> kept in staging` | L'opération a échoué et est close ; son staging reste en place |
| `savewarden: vault: published <op>` | La génération est publiée, même si le code est 1 parce que stdout n'a pas pu être écrit |
| `savewarden: vault: rolled back <op>; current unchanged` | Le renommage a échoué et rien n'a bougé, ou, pour une rotation, l'ancienne génération a été remise en place |
| `savewarden: vault: intervention required for <op>: <cause>` | L'opération attend une récupération, code 3 |

Le renommage n'est retenté que si le dossier est momentanément utilisé,
erreurs Windows 32 et 33, pendant cinq secondes au plus. Un fichier ouvert
dans le staging, par exemple par l'Explorateur ou un antivirus, fait échouer
le renommage aussitôt, erreur 5 : fermez les programmes qui utilisent le
coffre, puis relancez.

## Récupérer : `--recover`

Une opération interrompue, par une coupure, un arrêt du processus ou une
erreur, bloque le plan en code 3 avec `pending-operation`. `--recover`
montre ce qui sera fait, sans rien écrire. Pour une rotation, il nomme aussi
la génération remplacée, `replaces:`, et ce qu'il trouve sous `retained`.
Exemple d'une première publication :

```text
manifest: D:\Manifests\synthetic-game.yaml
game: synthetic-game (Synthetic Game)
vault: F:\Coffre (initialized)
volume: NTFS, fixed drive
operation: 20261105T101500-1a2b3c4d
last committed step: publish-intent
observed: current new, staging absent
action: verify current against the plan, then commit its receipt and published; if it differs, move it back to staging
mode: recovery plan only, nothing written
```

`--recover --apply` l'exécute, sous verrou, après avoir relu le coffre :
même opération, même observation, même action, sinon rien. Les actions sont
décrites dans le [guide du coffre](../vault.md#récupérer-une-opération-interrompue).
Toute situation que le protocole n'explique pas sort en code 3 sans rien
écrire, à chaque exécution, avec le même diagnostic.

## Codes de sortie

| Code | Signification | stderr |
| --- | --- | --- |
| `0` | Plan sans anomalie bloquante ; publication vérifiée ; déjà à jour ; récupération menée à son terme ; rien à récupérer | Vide pour le plan ; sinon la ligne d'état |
| `1` | Invocation, manifeste ou coffre invalide, drapeau de politique hors de ses conditions, coffre qui chevauche l'installation, sources qui se chevauchent sous un autre nom, volume indisponible ou non qualifié : ReFS, FAT32, exFAT, réseau, optique, RAM, erreur de lecture ou d'écriture, verrou pris, `vault-changed`, renommage refusé puis annulé, interruption ; ou rapport impossible à écrire en entier, auquel cas stdout peut contenir un **préfixe tronqué** | Un diagnostic, puis la ligne d'état avec `--apply` ou `--recover` |
| `2` | Rapport complet avec au moins une anomalie bloquante, ou `insufficient-space`, `changed-during-sync`, `verification-mismatch`, `current-modified` : cette exécution n'a rien publié. `current-exists` garde son sens de B2b mais n'est plus émis | `savewarden: plan blocked: N blocking finding(s)` ou la cause, puis la ligne d'état avec `--apply` |
| `3` | Le coffre demande une intervention : opération en attente, initialisation interrompue, journal ambigu, `current` non reconnu, observation contradictoire, anomalie après le retrait d'une rotation, rollback impossible, enveloppe de récupération épuisée | Le diagnostic, puis la ligne d'état avec `--apply` ou `--recover` |

Fiez-vous au code de sortie pour le résultat de la commande, et à la ligne
d'état, au receipt ou au plan pour l'état du coffre.

## Limites

- **Windows seulement**, comme `inspect`.
- **Générations conservées sans limite.** Chaque rotation garde l'ancienne
  génération entière dans `retained` ; rien n'est supprimé ni nettoyé
  automatiquement.
- **Fenêtre d'absence.** Pendant une rotation, entre le retrait et la
  promotion, `current` n'existe pas ; une interruption à cet instant est
  reprise par `--recover --apply`.
- **Verrou.** `sync.lock` n'exclut qu'une autre opération Savewarden : une
  écriture d'un autre programme entre une relecture et le renommage suivant
  n'est pas détectée. Fermez le jeu, le launcher et tout outil qui touche le
  coffre.
- **Coffre copié ou déplacé** : non pris en charge. Une copie change les
  identités NTFS sans altérer les données ; Savewarden le refuse en code 3,
  sans rien écrire ni adopter.
- **Pas un instantané atomique** : chaque fichier publié est exactement celui
  qui a été haché, et aucun changement n'a été vu pendant la fenêtre ; un
  changement suivi d'un retour à l'identique entre deux lectures reste
  possible. Fermez le jeu et son launcher.
- **Durabilité** : chaque fichier est vidé sur disque avant validation, mais
  Windows ne garantit pas qu'un renommage de dossier survive à une coupure de
  courant ; la récupération repose sur l'observation, jamais sur une
  supposition (ADR 0006 §6).
- **NTFS local seulement.** ReFS, y compris un Dev Drive, n'est pas encore
  qualifié ; FAT32, exFAT et les lecteurs réseau sont refusés.
- **Même disque physique** : deux volumes d'un même disque ne sont pas
  détectés ; un lecteur `subst` n'est pas reconnu comme redirection.
- **Confidentialité.** La sortie contient des chemins locaux ; anonymisez-la
  avant tout partage.

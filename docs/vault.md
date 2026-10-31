# Coffre Savewarden

Un coffre est un dossier que Savewarden crée et dont il est le seul à écrire
le contenu. `sync --apply` y publie une copie vérifiée des sources d'un
manifeste ; le plan, sans `--apply`, le lit sans rien écrire. Une nouvelle
génération remplace la précédente par rotation : l'ancienne est renommée dans
`retained`, jamais supprimée. `snapshot --apply` y range en outre des
snapshots : des copies vérifiées et immuables de `current`. Les règles de
publication et de récupération sont fixées par
[ADR 0006](adr/0006-publication-recovery-protocol.md), le contrat des
commandes par [ADR 0007](adr/0007-sync-contract-receipt-v1.md) pour `sync` et
[ADR 0008](adr/0008-snapshot-contract-receipt-v1.md) pour `snapshot`.

## Disposition

```text
<coffre>\
  savewarden-vault.json               marqueur : schéma, version de l'outil, date de création
  games\<game-id>\
    sync.lock                         verrou pendant une opération, jamais supprimé
    current\<source-id>\...           génération publiée et vérifiée
    retained\<op-id>\<source-id>\...  générations remplacées, jamais modifiées ni supprimées
    staging\<op-id>\<source-id>\...   opération en cours, échouée ou abandonnée
    receipts\<op-id>.json             receipt de chaque génération publiée
    journal\<op-id>\NNN-<étape>.rec   enregistrements validés, jamais réécrits
    snapshots\                        domaine des snapshots
      <snap-id>\<source-id>\...         snapshot publié, immuable
      <snap-id>.incomplete\...          copie en cours, échouée ou abandonnée
      receipts\<snap-id>.json           receipt de chaque snapshot
      journal\<snap-id>\NNN-<étape>.rec journal de chaque opération de snapshot
```

- `<op-id>` et `<snap-id>` ont la forme `AAAAMMJJTHHMMSS-xxxxxxxx` : date et
  heure UTC, puis huit chiffres hexadécimaux.
- Le [receipt v1](receipt-v1.md) décrit exactement une génération : chemins,
  tailles, dates, SHA-256. Le [receipt de snapshot](snapshot-receipt-v1.md)
  décrit un snapshot et la génération qu'il copie.
- `sync` accepte le dossier `snapshots` et ne lit rien dedans : aucune
  anomalie des snapshots ne bloque la protection de `current`, récupération
  comprise. Un binaire antérieur à B3 refuse un coffre qui contient
  `snapshots`, en code 3, sans rien écrire.

## Choisir un coffre

- Un dossier **absent**, dont le parent existe, ou un dossier **vide**. Un
  dossier non vide qui n'est pas un coffre Savewarden est refusé : il n'est
  jamais adopté, même s'il ressemble à un coffre.
- **Distinct** de l'installation du jeu : ni elle, ni dedans, ni autour.
- De préférence sur un **autre disque physique** que la source : une copie sur
  le même disque ne protège pas d'une panne de ce disque.
- Un volume **NTFS** sur un lecteur local, fixe ou amovible. ReFS, y compris
  un Dev Drive, n'est pas encore qualifié ; FAT32, exFAT, les lecteurs réseau,
  optiques et RAM sont refusés.

Ne modifiez pas le contenu d'un coffre à la main. Savewarden compare la
génération courante à son receipt avant toute publication : un écart bloque
tout, et il ne déplace jamais un `current` qu'il ne reconnaît plus.

Ne copiez ni ne déplacez un coffre. Savewarden reconnaît `current` aussi par
son identité NTFS, que le journal a enregistrée à la publication ; une copie
la change sans altérer les données. Un coffre copié ou déplacé est refusé en
code 3, sans rien écrire : ce n'est pas le signe d'une corruption, et
Savewarden n'adopte ni ne réécrit aucune identité. Utilisez le coffre
d'origine, ou un nouveau coffre ; une procédure de requalification ou
d'import contrôlé est prévue plus tard.

Des fichiers `*.tmp-<k>` peuvent subsister à la racine, dans `receipts` ou
dans un journal, à côté du fichier validé : ce sont des essais interrompus,
conservés exprès et jamais lus. Ne les supprimez pas.

## Que faire quand `sync` demande une intervention (code 3)

Savewarden refuse de deviner un état qu'il ne sait pas expliquer. Dans tous
les cas ci-dessous, il n'a rien déplacé ni écrit. Aucune étape de ce guide ne
supprime quoi que ce soit.

| Cause | Ce qui s'est passé | Que faire |
| --- | --- | --- |
| `initialization-interrupted` | La création du coffre s'est arrêtée avant que son marqueur soit validé : le dossier ne contient que des fichiers `savewarden-vault.json.tmp-<k>` | `sync --recover --apply` valide un nouveau marqueur ; les temporaires restent en place. Un autre dossier vide reste toujours possible |
| `pending-operation` | Une opération s'est interrompue : son journal n'atteint pas d'étape terminale | Lancer `sync --recover` pour voir l'action prévue, puis `sync --recover --apply` |
| `unexpected-entry` | Une entrée n'a pas sa place : fichier ajouté à la main, staging sans journal, génération conservée sans receipt ; ou `current` n'est pas le dossier que son journal a publié, coffre copié ou déplacé compris | Examiner l'entrée nommée ; si elle a été ajoutée à la main, la déplacer hors du coffre vous-même. Pour `current` d'une autre identité, reprendre le coffre d'origine ou un nouveau coffre |
| `journal-ambiguous` | Un journal est incomplet, corrompu, incohérent, ou deux opérations sont en attente pour un même jeu | Ne rien modifier. Conserver le coffre tel quel et demander une analyse ; Savewarden ne réparera jamais un journal ambigu |

Et pour une anomalie bloquante (code 2) liée au coffre :

| Code | Ce qui s'est passé | Que faire |
| --- | --- | --- |
| `current-modified` | `current` ne correspond plus exactement à son receipt, n'en a pas, ou un receipt n'a plus de `current` | Comparer avec `inspect`, par un manifeste qui pointe sur `current` ; la résolution d'un tel écart relève d'une décision ultérieure. Un nouveau coffre reste toujours possible |

## Récupérer une opération interrompue

`sync --recover` lit le journal de l'opération et observe, par leur identité
de fichier, ce que contiennent `current`, `staging\<op>` et, pour une
rotation, `retained\<op précédente>`. Il n'agit que dans les cas prévus par
[ADR 0006](adr/0006-publication-recovery-protocol.md) :

| Situation | Action de `--recover --apply` | Ligne d'état |
| --- | --- | --- |
| Initialisation interrompue | Valide un nouveau marqueur | `metadata only; …` |
| Opération arrêtée avant la publication, staging éventuel en place | Enregistre `abandoned` ; le staging reste, jamais lu ni déplacé | `current unchanged; operation <op> kept in staging` |
| Publication voulue, renommage non effectué | Enregistre `rolled-back` | `rolled back <op>; current unchanged` |
| Staging déjà devenu `current`, receipt absent ou non clos | Vérifie `current` entier contre le plan ; égal : écrit le receipt, reconstruit avec `recovered_at` et `recovered_by`, puis clôt ; différent : remet `current` en staging | `published <op>` ou `rolled back <op>; …` |
| Remise en staging interrompue | La termine ; un échec est enregistré une fois, `rollback-failed`, code 3 | `rolled back <op>; …` ou `intervention required …` |
| Opération sans aucun enregistrement | Enregistre `abandoned` seulement si aucune génération n'a été publiée et que `current` est absent, ou si `current` est la génération précédente reconnue ; une génération publiée sans `current` est une contradiction, code 3 | `current unchanged; …` ou `intervention required …` |
| Rotation, retrait non effectué | Enregistre `rolled-back` | `rolled back <op>; current unchanged` |
| Rotation, ancienne génération déjà dans `retained`, nouvelle pas promue | **Restauration vérifiée** : relit la génération retenue contre son receipt ; intacte, la remet en `current`, puis `rolled-back` ; altérée ou remplacée, code 3, rien n'est déplacé | `rolled back <op>; …` ou `intervention required …` |
| Rotation, nouvelle génération promue | Vérifie `current` entier contre le plan ; égal : receipt et `published` ; différent : la remet en staging, puis restauration vérifiée ; receipt déjà validé : code 3 | `published <op>`, `rolled back <op>; …` ou `intervention required …` |
| Receipt de l'opération validé alors que sa bascule n'est pas observée | Contradiction, quelle que soit l'étape : code 3, rien écrit, à chaque exécution | `intervention required …` |
| Rotation arrêtée avant le retrait, `current` disparu ou remplacé entre-temps | Aucune ligne ne s'applique : code 3, rien écrit, à chaque exécution. L'ancienne génération n'a pas été déplacée par Savewarden ; remettez-la à sa place si vous l'avez déplacée, sinon conservez le coffre tel quel et utilisez un nouveau coffre | `intervention required …` |
| Rotation, objet inattendu sous `current` après le retrait | Aucune ligne ne s'applique : code 3, rien écrit ni déplacé, à chaque exécution, tant que l'objet est là ; une fois l'objet déplacé hors du coffre par vous, la ligne de restauration vérifiée s'applique | `intervention required …` |

Une interruption pendant la récupération elle-même laisse un état que la
récupération suivante reconnaît. Tout le reste est une ambiguïté : code 3,
rien n'est écrit ni déplacé, à chaque exécution.

**Enveloppe épuisée.** Chaque fichier validé par renommage dispose de neuf
essais : un enregistrement du journal, toutes étapes confondues, un receipt,
le marqueur. Si neuf essais ont été interrompus, `recovery envelope
exhausted` : Savewarden ne fait plus rien sur ce coffre. Ne supprimez aucun
temporaire ; conservez le coffre tel quel pour analyse et utilisez, au
besoin, un nouveau coffre.

## Snapshots : interventions et récupération

`snapshot` applique les mêmes causes d'intervention, mais au seul domaine des
snapshots, et sa récupération ne dépend jamais de `current` :

| Situation | Que faire |
| --- | --- |
| `pending-operation` d'un snapshot | `snapshot --recover` montre l'action prévue, `snapshot --recover --apply` l'exécute ; `sync` n'en est pas bloqué. Si un autre processus Savewarden tient le verrou, l'opération est en cours : code 1, attendre qu'elle se termine |
| `pending-operation` de `current` au moment d'un snapshot | Récupérer d'abord par `sync --recover --apply` ; un snapshot exige un `current` reconnu |
| `source-unverified` | Le receipt de la génération copiée est absent, modifié ou illisible. Avant la publication, l'opération est close sans rien publier ; après le renommage, une preuve contraire remet la copie en `.incomplete`, une preuve illisible laisse l'opération en attente : relancer `snapshot --recover --apply` une fois le fichier lisible |
| `snapshot-missing`, `snapshot-rename-lost`, `snapshot-modified` | Snapshot dégradé, signalé sans blocage : Savewarden ne le répare jamais. Un nouveau snapshot de la même génération est possible. Après `snapshot-rename-lost`, la copie complète reste sous `.incomplete` |

Une ambiguïté du domaine des snapshots, journal incohérent, entrée
inexpliquée ou `snapshots\receipts` absent à côté d'opérations, bloque la
création de snapshots et leur récupération jusqu'à votre intervention,
jamais `sync`. Ne supprimez aucun fichier ; un nouveau coffre reste possible.
Les copies `.incomplete` des opérations closes ne sont ni relues ni exigées.

## Espace disque

Chaque publication copie la génération complète en staging. Savewarden exige,
au plan puis sous verrou, la taille de la génération plus max(64 Mio, 5 %)
d'espace disponible pour l'utilisateur, quotas compris ; une rotation ne fait
que renommer l'ancienne génération, sans la copier. Les générations
remplacées s'accumulent dans `retained`, une de plus à chaque publication, et
ni elles ni les stagings échoués ne sont supprimés automatiquement : l'espace
qu'ils occupent relève du propriétaire du coffre. Il en va de même des
snapshots et de leurs copies `.incomplete` : un snapshot copie la génération
complète, avec la même réserve d'espace, et un seul snapshot conforme est
créé par génération.

## Confidentialité

Receipts et journaux contiennent des chemins locaux et le nom du jeu ;
journaux et diagnostics contiennent aussi des identités de fichiers NTFS, dont
le numéro de série du volume, identifiant stable du disque. Anonymisez-les
avant tout partage.

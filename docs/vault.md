# Coffre Savewarden

Un coffre est un dossier que Savewarden crée et dont il est le seul à écrire
le contenu. `sync` y publiera, à partir de B2b, une copie vérifiée des sources
d'un manifeste. Dans cette version, `sync` **planifie seulement** : il lit le
coffre s'il existe et n'écrit rien. Les règles de publication et de
récupération sont fixées par
[ADR 0006](adr/0006-publication-recovery-protocol.md), le contrat de la
commande par [ADR 0007](adr/0007-sync-contract-receipt-v1.md).

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
```

- `<op-id>` a la forme `AAAAMMJJTHHMMSS-xxxxxxxx` : date et heure UTC, puis
  huit chiffres hexadécimaux.
- Le [receipt v1](receipt-v1.md) décrit exactement une génération : chemins,
  tailles, dates, SHA-256.
- `snapshots\` est réservé au lot B3.

## Choisir un coffre

- Un dossier **absent**, dont le parent existe, ou un dossier **vide**. Un
  dossier non vide qui n'est pas un coffre Savewarden est refusé : il n'est
  jamais adopté, même s'il ressemble à un coffre.
- **Distinct** de l'installation du jeu : ni elle, ni dedans, ni autour.
- De préférence sur un **autre disque physique** que la source : une copie sur
  le même disque ne protège pas d'une panne de ce disque.
- À partir de B2b : un volume **NTFS local**. ReFS, FAT32, exFAT et les
  lecteurs réseau seront refusés.

Ne modifiez pas le contenu d'un coffre à la main. Savewarden compare la
génération courante à son receipt avant toute publication : un écart bloque
tout, et il ne déplace jamais un `current` qu'il ne reconnaît plus.

## Que faire quand `sync` demande une intervention (code 3)

Savewarden refuse de deviner un état qu'il ne sait pas expliquer. Dans tous
les cas ci-dessous, il n'a rien déplacé ni écrit. Aucune étape de ce guide ne
supprime quoi que ce soit.

| Cause | Ce qui s'est passé | Que faire |
| --- | --- | --- |
| `initialization-interrupted` | La création du coffre s'est arrêtée avant que son marqueur soit validé : le dossier ne contient que des fichiers `savewarden-vault.json.tmp-<k>` | Jusqu'à B2b, choisir un autre dossier pour le coffre. À partir de B2b, `sync --recover --apply` reprendra l'initialisation, explicitement |
| `pending-operation` | Une opération s'est interrompue : son journal n'atteint pas d'étape terminale | Jusqu'à B2b, aucune opération ne peut exister. À partir de B2b, lancer `sync --recover` pour voir l'action prévue, puis `sync --recover --apply` |
| `unexpected-entry` | Une entrée n'a pas sa place : fichier ajouté à la main, staging sans journal, génération conservée sans receipt | Examiner l'entrée nommée ; si elle a été ajoutée à la main, la déplacer hors du coffre vous-même |
| `journal-ambiguous` | Un journal est incomplet, corrompu, incohérent, ou deux opérations sont en attente pour un même jeu | Ne rien modifier. Conserver le coffre tel quel et demander une analyse ; Savewarden ne réparera jamais un journal ambigu |

Et pour une anomalie bloquante (code 2) liée au coffre :

| Code | Ce qui s'est passé | Que faire |
| --- | --- | --- |
| `current-modified` | `current` ne correspond plus exactement à son receipt, n'en a pas, ou un receipt n'a plus de `current` | Comparer avec `inspect`, par un manifeste qui pointe sur `current` ; la résolution d'un tel écart relève d'une décision ultérieure. Un nouveau coffre reste toujours possible |

## Espace disque

Chaque publication copie la génération complète en staging. À partir de B2b,
Savewarden exigera, avant d'écrire, la taille de la génération plus
max(64 Mio, 5 %) d'espace disponible pour l'utilisateur. Les générations
remplacées et les stagings échoués ne sont jamais supprimés automatiquement :
l'espace qu'ils occupent relève du propriétaire du coffre.

## Confidentialité

Receipts et journaux contiennent des chemins locaux et le nom du jeu.
Anonymisez-les avant tout partage.

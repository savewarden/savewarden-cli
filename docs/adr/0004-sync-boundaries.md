# ADR 0004 — Frontières de `sync`

Date : 29 octobre 2026. Statut : accepté pour B2, plan B2 v0.2 §3, validé par
le propriétaire le 29 octobre 2026.

## Contexte

B2 ajoute `sync`, qui copie les sources d'un manifeste vers un coffre. Le
parcours sans suivi de liens d'`inspect` est le code le plus sensible de B1 :
`sync` en a besoin pour inventorier la source avant et après copie, vérifier
le staging et contrôler `current` contre son receipt. ADR 0001 interdit
qu'une feature en importe une autre. Les types d'inventaire vivent dans
`features/inspect`, que `sync` ne peut pas importer. B2 introduit aussi le
premier code qui écrit ; il doit rester confiné à un adaptateur.

## Décision

```text
cmd/savewarden ──► app ─┬─► cli ─┬─► features/inspect ──► entities/game, entities/backup
                        │        └─► features/sync ─────► entities/game, entities/backup
                        ├─► platform/manifests ──► entities/game, YAML
                        ├─► platform/filesystem ─► stdlib, syscall          (lecture seule)
                        └─► platform/vault ──────► stdlib, syscall, x/sys   (B2b, seul package qui écrit)
```

- **`internal/entities/backup`** porte les types communs : `Entry`,
  `Directory`, `Finding`, `Severity`, `Code`, la différence pure entre deux
  inventaires, le receipt v1, le marqueur du coffre, l'enregistrement de
  journal, sa validation et le classement des observations d'ADR 0006. Il ne
  dépend que de la bibliothèque standard et ne fait aucune I/O : l'appelant
  lit les octets, le package les décode et les juge.
- **`internal/features/inspect`** expose `Service.Directory`, l'inventaire
  sans suivi de liens d'un dossier quelconque, sur le même parcours que son
  `Inspect`. Sa sortie ne change pas.
- **`internal/features/sync`** possède l'orchestration et définit ses propres
  ports, étroits :
  - `ManifestReader`, comme `inspect` ;
  - `Inventories`, dont la méthode `Directory` a la signature exacte de
    `inspect.Service.Directory` : `app` lui passe le service `inspect`, sans
    adaptateur et sans qu'aucune feature importe l'autre ;
  - `Files`, portée de lecture sans suivi, pour lire le coffre et comparer
    des identités ; `app` le satisfait avec `platform/filesystem`.
  
  B2b ajoute, par l'amendement ci-dessous :
  - `Volumes`, requête en lecture seule du système de fichiers, du type de
    lecteur et de l'espace disponible pour l'appelant ; le mode plan la reçoit ;
  - `Vaults` et `Vault`, session de coffre en création seule, que seuls
    `--apply` et `--recover` reçoivent ;
  - `Clock`, date des opérations et attente des retries bornés.
- **`internal/platform/vault`**, créé en B2b, est le seul adaptateur
  autorisé à écrire (ADR 0005). `golang.org/x/sys/windows`, en version
  exacte, n'y sera permis que là, par le test d'imports.
- **`internal/cli`** n'importe toujours que des features. Elle lit les
  champs de types d'entités exposés par les features sans nommer le package
  d'entités : `Severity` et les statuts se rendent par leur méthode `String`
  ou par des méthodes des features.
- Les tests d'imports couvrent `features/sync` et `entities/backup` par des
  cas positifs et négatifs explicites.

## Alternatives

- **`sync` importe `inspect`** : interdit par ADR 0001, et lierait deux
  features qui doivent pouvoir évoluer séparément.
- **Dupliquer le parcours dans `sync`** : deux copies du code le plus
  sensible du produit, qui divergeraient.
- **Déplacer le parcours dans `platform/filesystem`** : la politique de
  sécurité, ne pas suivre, classer, détecter un changement, quitterait la
  feature, où un faux système de fichiers la teste sur les trois OS.
- **Déplacer le parcours dans `entities`** : une entité orchestrerait des
  I/O, même derrière une interface ; contraire au rôle des entités.

## Conséquences

`inspect` devient le fournisseur unique du parcours sans suivi, consommé par
`sync` à travers un port qu'il ne connaît pas. Un changement de signature de
`Directory` casse la compilation de `app`, qui fait le pont : la dépendance
est visible et vérifiée. Les types d'inventaire migrent vers
`entities/backup`, prouvé par une sortie `inspect` identique octet pour
octet. Une future feature, `snapshot` ou `restore`, réutilisera
`entities/backup` et le même principe de port.

## Amendement B2b

29 octobre 2026. Les ports d'écriture s'écartent de la forme indicative du
plan B2 §3.2, sans changer les frontières :

- `Vaults.Open(path)` ouvre une session sur le chemin du coffre, sans
  exécuter l'initialisation : le protocole I1 à I9 d'ADR 0006 est de
  l'orchestration et vit dans `features/sync`, testé sur faux coffre sur les
  trois OS. L'adaptateur n'offre que des primitives.
- `Vault` expose `CreateDir`, `Hold`, `Release`, `NewFile`, `Move`,
  `Identity`, `Lock` et `Close`. Il n'y a pas de `Commit` : un enregistrement
  est validé par `NewFile` sur son temporaire puis `Move`, et c'est
  `features/sync` qui choisit le `k`, applique l'enveloppe E1 et les retries
  du §7 d'ADR 0006. `Hold` et `Release` épinglent un dossier ou le libèrent
  avant une bascule.
- La création de fichier s'appelle `NewFile` : le garde d'ADR 0005 contrôle
  les arguments de tout appel `CreateFile`, et un port de ce nom serait
  refusé dans `features/sync`.
- `Volumes.Describe(path)` reçoit un dossier existant, le coffre ou son
  parent, et rapporte les octets disponibles pour l'utilisateur.
- Correction consignée en B2c : la portée de lecture `Scope` a reçu en B2b
  `Identity(name)`, l'identité volume et index d'un fichier ou dossier lue
  sans suivi de lien, et `platform/filesystem` décrit un fichier tenu sans
  partage sans l'ouvrir. B2c n'ajoute ni frontière ni port : la rotation et
  sa récupération n'emploient que `Vault`, `Files`, `Inventories`, `Volumes`
  et `Clock`.


# ADR 0004 — Frontières de `sync`

Date : 29 octobre 2026. Statut : accepté pour B2, plan B2 v0.2 §3, validé par
le propriétaire le 29 octobre 2026. Amendement B3 accepté le 31 octobre 2026.
Amendement B4 accepté le 3 novembre 2026.

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
`entities/backup` et le même principe de port. *Phrase historique, conservée
pour la traçabilité : remplacée pour `snapshot` par l'amendement B3, et pour
`restore` par l'amendement B4.*

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

## Amendement B3

30 octobre 2026, plan B3 v0.2 ; accepté par le propriétaire le
31 octobre 2026, à livrer par la PR B3. Les snapshots ne forment pas une feature
séparée : `features/sync` possède **toutes les publications dans le coffre
d'un jeu**, `current` et snapshots, et leurs récupérations. La phrase des
Conséquences qui annonçait une feature `snapshot` réutilisant
`entities/backup` est remplacée par celle-ci ; `restore` (B4) reste une
feature à cadrer. *Cette dernière phrase est historique : remplacée par
l'amendement B4.*

- Le moteur de publication, non exporté, est paramétré par la disposition et
  les codes de l'opération, dans le même package, sans changement de
  comportement pour `current`.
- La commande CLI `snapshot` appelle des méthodes de service propres au
  snapshot ; `internal/cli` n'importe toujours que des features.
- Aucun port, aucune frontière, aucune règle d'import ne change ;
  `platform/vault` et ADR 0005 restent inchangés.
- `entities/backup` reçoit le receipt de snapshot v1, l'automate et le
  contrat des champs du mode snapshot, et le classement de ses observations,
  sans I/O.

Alternatives écartées pour B3 : une feature `snapshot` dupliquant le moteur
d'écriture et ses faux de test ; l'extraction du moteur dans un package de
protocole, frontière durable nouvelle, réexaminée au cadrage de B4.

## Amendement B4

1er novembre 2026, plan B4 v0.2 ; accepté par le propriétaire le
3 novembre 2026, à livrer par la PR B4. La restauration
isolée n'est pas une feature séparée : `features/sync` possède **toutes les
publications selon ADR 0006**, dans le coffre d'un jeu et dans une zone de
restauration (ADR 0006 §11), et leurs récupérations. Elle remplace la phrase
des Conséquences qui annonçait une future feature `restore` réutilisant
`entities/backup`, et celle de l'amendement B3 qui laissait `restore`
« feature à cadrer ». **Priorité** : un amendement prévaut sur le texte qui le
précède ; les phrases qu'il remplace restent en place, marquées comme
historiques, pour la traçabilité, et ne décrivent plus les frontières
actuelles.

- **Couture interne.** Le moteur de publication distingue, par des types non
  exportés du même package, la **source de lecture** et la **cible
  d'écriture** : chemin, volume, segments, nom du verrou et libellé des
  diagnostics. Ils remplacent le préfixe du dossier de jeu, le nom
  `sync.lock` et les champs du plan de `sync` dont le moteur tirait sa
  racine ; une opération ne porte plus le plan de `sync`. Ce commit est isolé,
  sans changement de comportement pour `current` ni pour les snapshots, et
  soumis à une revue indépendante de non-régression avant tout code de
  `restore`.
- La commande CLI `restore` appelle des méthodes de service propres à la
  restauration ; `internal/cli` n'importe toujours que des features.
- Aucun port, aucune frontière, aucune règle d'import ne change.
  `platform/vault` et ADR 0005 restent inchangés : une session d'écriture
  s'ouvre sur une zone comme sur un coffre ; seuls des commentaires le disent.
- `entities/backup` reçoit le receipt de restauration v1, le marqueur de zone
  v1, l'automate et le contrat des champs du mode restauration, et le
  classement de ses observations, sans I/O.

**Extraction du moteur, réexaminée et non retenue pour B4.** `restore` a
besoin du moteur d'écriture et du lecteur du domaine des snapshots, tous deux
dans `features/sync` ; les extraire déplacerait la plus grande partie du code
qualifié, pour un gain de nommage. La couture est ce qu'une extraction future
détacherait. **Critères de réexamen** : une quatrième opération d'écriture,
restauration en place ou requalification d'un coffre copié notamment ; une
opération qui aurait besoin du moteur sans le lecteur du coffre ; une
opération qui renommerait un snapshot publié ou ses ancêtres, ou les
tiendrait sans partage, levant l'invariant d'ADR 0006 §11 ; une revue qui
juge le package illisible.

Alternatives écartées pour B4 : une feature `restore` dupliquant le moteur,
le lecteur des snapshots et les faux de test ; l'extraction immédiate du
moteur dans un package de protocole, avec authentification servie par un
port que `sync.Service` satisferait.

**Erratum proposé, 3 novembre 2026, en attente de ratification par le
propriétaire.** Relevé par la revue indépendante de non-régression du
commit 2 de B4 et précisé par la vérification de ses corrections.

- « Une opération ne porte plus le plan de `sync` » se lit ainsi : le moteur
  ne tire plus aucune racine des champs du plan. La cible d'écriture, le
  type `target`, et la source de lecture, le type `readRoot`, sont
  construites une fois, à l'ouverture de l'opération, depuis le chemin du
  coffre et l'identifiant du jeu. Seule l'opération de `sync` porte son
  plan, pour ses propres phases : inventaires des sources, état du coffre
  planifié, espace requis, identité du jeu du plan journalisé et résultat
  rendu. Le moteur, les opérations de snapshot et de restauration et toutes
  les récupérations ne le lisent ni ne le renseignent :
  `TestOnlySyncReadsItsPlan` n'en autorise la lecture qu'à ces phases,
  nommées une à une, et refuse toute construction d'opération qui le
  renseigne.
- « Ce commit est isolé » se lit : la couture et les commits qui corrigent
  ses revues précèdent tout code de `restore`.

Aucune autre règle ne change.

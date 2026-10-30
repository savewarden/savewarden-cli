# Pilote Assassin's Creed Odyssey

Source : plan Savewarden v0.1 daté du **9 octobre 2026**. Les éléments ci-dessous
sont des constats historiques rapportés, **pas une nouvelle inspection**.
B0 n'a lu ni l'installation, ni `CPY.ini`, ni les fichiers de sauvegarde.

| Élément | Constat rapporté par le plan |
| --- | --- |
| Source active déclarée dans CPY.ini | `E:\GMS\Assassins Creed - Odyssey\saves` |
| État actif copié et vérifié par SHA-256 | 14 fichiers |
| Copie actuelle centralisée | `D:\Savewarden\games\assassins-creed-odyssey\current\saves` |
| Snapshot vérifié existant | `20261009T183208-cacd3cae` |
| Anciennes copies | `D:\Ga`, laissées intactes |

Ne jamais réécrire ni supprimer ce snapshot. Les copies anciennes ne décident
pas de la source de vérité. Les fichiers privés, hash individuels, identifiants
de joueur et contenu de `CPY.ini` ne sont pas importés dans ce dépôt.

## Prochains lots, avec Human GO distinct

B1 fournit le [manifeste v1](../manifest-v1.md) et
[`inspect`](../commands/inspect.md). Le manifeste déclare le chemin de
l'installation et la source `saves` relative à celle-ci ; `CPY.ini` n'est pas
lu automatiquement. L'utilisateur confirme la source active ; aucun
rapprochement automatique avec `Ga`.

### Protocole manuel B1, lecture seule

Exécuté par le propriétaire, ou par l'agent sur autorisation explicite et
nominative seulement.

1. Fermer le jeu et le launcher.
2. Écrire le manifeste **hors du dépôt**, avec le chemin réel de
   l'installation et `path: saves`.
3. Lancer `savewarden inspect --manifest <chemin>` deux fois ; les deux
   sorties doivent être identiques.
4. Attendu : code 0, au moins les 14 fichiers `.save` constatés le
   9 octobre, davantage si le jeu a sauvegardé depuis, aucune anomalie.
5. Optionnel : sauvegarder en jeu, fermer, relancer `inspect`, comparer.
6. Consigner dans [le rapport B1](../validation/b1.md) le nombre de fichiers,
   le code de sortie et les anomalies, sans hash individuel, nom de profil ni
   chemin personnel.

### Résultat B1 — 12 octobre 2026

Exécuté par le propriétaire sous Windows 11 avec le binaire du commit de
fusion de B1 (`3fe7d92`) : deux inspections, codes de sortie `0` et `0`,
rapports identiques, **14 fichiers** inventoriés, `findings: none`,
`PilotPassed = True`. Qualification fonctionnelle validée par le propriétaire
le 24 octobre 2026. Détail dans [le rapport B1](../validation/b1.md). L'étape
facultative d'écriture en jeu n'est pas consignée ; aucune empreinte
individuelle n'est versionnée.

### Protocole B2c

Exécuté par le propriétaire après la fusion de B2c, dans le coffre du pilote
B2b ; l'agent n'y accède pas :

1. jouer et sauvegarder une fois, puis fermer le jeu et son launcher ;
2. `sync`, plan seul : code `0`, `plan: changes`, `current` égal à
   l'opération du pilote B2b ;
3. `sync --apply` : code `0`, `published <op>`, ligne `retained:` qui nomme
   l'ancienne génération ;
4. `inspect` d'un manifeste qui pointe sur `retained\<op du pilote B2b>` :
   SHA-256 égaux à ceux de son receipt, sans les versionner ;
5. plan suivant `up to date`, puis second `sync --apply` : `not modified`,
   empreinte du coffre inchangée.

Consigner codes, nombre de fichiers et résultat d'idempotence dans le rapport
B2c, sans hash individuel, nom de profil, chemin personnel ni receipt réel.

### Résultat B2b — 29 octobre 2026

Exécuté par le propriétaire sous Windows 11, après la fusion de B2b, avec un
binaire compilé depuis un clone dont le script du pilote vérifie que `HEAD`
est le commit de fusion `8f9f5e1`. Première publication vers un **coffre
neuf**, distinct de l'ancien coffre du pilote :

- `sync --apply` : code `0` ; **14 fichiers**, 9 520 705 octets, 14 SHA-256 sur
  14 identiques à la source ; source inchangée ; un receipt ;
- plan suivant : code `0`, `up to date`, aucune anomalie ;
- second `sync --apply`, lancé seulement après ce plan : code `0`,
  `up to date`, `savewarden: vault: not modified`, empreinte du coffre
  inchangée ; `IdempotencePassed = True`.

Qualification fonctionnelle validée par le propriétaire le 29 octobre 2026.
Le pilote qualifie le chemin nominal ; les crashs et la récupération restent
prouvés par les tests synthétiques et la CI. Détail dans
[le rapport B2b](../validation/b2b.md#pilote-b2b). Aucune empreinte
individuelle, aucun receipt réel ni aucun chemin personnel n'est versionné.

B2 copie seulement après plan, choix explicite de destination et
autorisation ; pas de suppression de la source. L'ancien coffre du pilote,
non vide et sans marqueur, est refusé par la règle générique du coffre,
jamais adopté. B3 ajoutera
un snapshot neuf
et immuable. B4 restaurera un snapshot dans un dossier **isolé**. B5 contrôlera
manifestes/receipts/hash et validera le pilote complet avant une RC.

Les autorisations d'un lot ne valent pas consentement pour relocation,
symlink/junction, changement de `SavePath` ou restauration en place.
La campagne testera ensuite Cyberpunk 2077 (source sous `C:`) et Metal Gear
Solid V (profil différent) avant de généraliser les garanties.

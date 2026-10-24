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

B2 copiera seulement après plan/dry-run, choix explicite de destination et
autorisation ; pas de suppression de la source. B3 ajoutera un snapshot neuf
et immuable. B4 restaurera un snapshot dans un dossier **isolé**. B5 contrôlera
manifestes/receipts/hash et validera le pilote complet avant une RC.

Les autorisations d'un lot ne valent pas consentement pour relocation,
symlink/junction, changement de `SavePath` ou restauration en place.
La campagne testera ensuite Cyberpunk 2077 (source sous `C:`) et Metal Gear
Solid V (profil différent) avant de généraliser les garanties.

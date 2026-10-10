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

B1 définira un manifeste v1 avec source relative à l'installation, base
`game-install` et `SavePath` vérifié en entrée. Inventaire read-only après
parsing strict et rejet des liens/jonctions. L'utilisateur devra confirmer
la source active ; aucun rapprochement automatique avec `Ga`.

B2 copiera seulement après plan/dry-run, choix explicite de destination et
autorisation ; pas de suppression de la source. B3 ajoutera un snapshot neuf
et immuable. B4 restaurera un snapshot dans un dossier **isolé**. B5 contrôlera
manifestes/receipts/hash et validera le pilote complet avant une RC.

Les autorisations d'un lot ne valent pas consentement pour relocation,
symlink/junction, changement de `SavePath` ou restauration en place.
La campagne testera ensuite Cyberpunk 2077 (source sous `C:`) et Metal Gear
Solid V (profil différent) avant de généraliser les garanties.

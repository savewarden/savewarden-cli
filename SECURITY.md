# Sécurité

Savewarden est en bootstrap B0 (`dev`) et aucune release supportée n'est
disponible. Seules l'aide et la version existent. Le modèle de sécurité est
décrit dans [docs/safety-model.md](docs/safety-model.md).

## Signaler un problème

Utiliser **Security → Report a vulnerability** sur le dépôt GitHub lorsque
le signalement privé est activé. À défaut, contacter le mainteneur via les
coordonnées publiques de son profil GitHub ; ne pas ouvrir une issue publique
avec des secrets ou des sauvegardes personnelles.

Décrire la version/commit, l'OS, le comportement attendu et observé, puis les
étapes de reproduction sur un jeu de données synthétique. Aucun délai de
réponse garanti à ce stade ; le mainteneur précise le suivi au cas par cas.

Ne jamais joindre des sauvegardes réelles, identifiants de compte, tokens,
clés, chemins personnels non nécessaires ou fichiers de configuration privés.

## Limites

Le bootstrap ne chiffre, ne copie, ne vérifie ni ne restaure de fichiers.
Les invariants futurs ne sont pas déclarés implémentés ou validés avant leurs
tests. Une copie locale sur le même disque n'est pas une sauvegarde hors-disque.
Toute opération sur des données réelles demande un pilote séparé, lecture
ou dry-run d'abord, puis autorisation explicite d'écriture.

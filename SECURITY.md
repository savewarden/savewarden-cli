# Sécurité

Savewarden est en développement, version `dev`, lot B2a : aucune release
supportée n'est disponible. `inspect` inventorie des sources en lecture seule
et `sync` planifie leur copie vers un coffre, en lecture seule aussi. Le
modèle de sécurité est décrit dans [docs/safety-model.md](docs/safety-model.md),
le protocole de publication et de récupération qui gouvernera les écritures
dans [ADR 0006](docs/adr/0006-publication-recovery-protocol.md).

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
Les sorties d'`inspect` et de `sync`, les receipts et les journaux d'un
coffre contiennent des chemins locaux : les anonymiser avant de les joindre.

## Limites

Cette version ne copie, ne chiffre ni ne restaure aucun fichier ; elle
n'écrit rien. Les invariants des écritures futures ne sont pas déclarés
implémentés ou validés avant leurs tests. Une copie locale sur le même disque
n'est pas une sauvegarde hors-disque. Toute opération sur des données réelles
demande un pilote séparé, lecture ou plan d'abord, puis autorisation explicite
d'écriture.

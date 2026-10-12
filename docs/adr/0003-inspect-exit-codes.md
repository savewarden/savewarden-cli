# ADR 0003 — Contrat de sortie d'`inspect`

Date : 12 octobre 2026. Statut : accepté pour B1, décision D1 du plan B1 v0.2.

## Contexte

Le contrat B0 ne connaît que deux codes : 0 en succès, 1 en erreur avec un
diagnostic unique sur stderr et rien sur stdout. `inspect` produit un rapport
complet même quand il rencontre des anomalies : jonction refusée, fichier
verrouillé, source requise absente. Ce rapport doit rester lisible, et un
script doit pouvoir distinguer « inventaire fiable » de « inventaire avec
anomalies » sans analyser le texte.

## Décision

| Code | Cas | stdout | stderr |
| --- | --- | --- | --- |
| `0` | Inventaire complet sans anomalie bloquante | Rapport | Vide |
| `1` | Invocation invalide, manifeste invalide, volume inutilisable, sources qui se chevauchent, interruption, sortie impossible à écrire | Vide | Un diagnostic |
| `2` | Rapport complet avec au moins une anomalie bloquante | Rapport | `savewarden: inspection incomplete: N blocking finding(s)` |

- Une source facultative absente est une **information** : elle apparaît
  dans le rapport mais laisse le code à 0.
- Une interruption ne produit jamais de rapport partiel : code 1.
- Si le rapport ne peut pas être entièrement écrit, le code est 1, même
  quand il aurait valu 2 : un rapport tronqué n'est jamais présenté comme
  complet.
- `cli` porte le code par une `ExitError` ; `app` est le seul endroit qui
  traduit une erreur en code de processus.
- Les commandes existantes gardent le contrat B0.

## Alternatives

- Code 1 avec le rapport sur stdout : rompt la règle B0 « rien sur stdout en
  erreur » et confond un manifeste invalide avec un inventaire incomplet.
- Code 0 avec anomalies signalées dans le texte : un script ou une tâche
  planifiée considérerait un inventaire incomplet comme réussi.

## Conséquences

Les scripts testent `$LASTEXITCODE` : 0 fiable, 2 à examiner, 1 à corriger.
Les codes sont vérifiés par les tests de `internal/cli`, de `internal/app` et
du binaire. Une future sortie JSON (B2) gardera ces codes.

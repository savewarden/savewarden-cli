# ADR 0001 — Frontières FSD-inspired avec packages Go idiomatiques

Date : 10 octobre 2026. Statut : décision de bootstrap, à examiner en revue B0.

## Contexte

Savewarden doit isoler un moteur de sauvegarde de sa CLI Windows-first.
Dirloom démontre des responsabilités séparées et un workflow release branch.
Mosaera et le blueprint Sonora décrivent propriété, couches, slices et
composition ; cette conception n'est pas une norme Savewarden stabilisée.

## Décision

Adopter `cmd → app → cli` dans B0. Ajouter progressivement `features/<verbe>`,
`entities/<concept>` et `platform/<adaptateur>`. Features indépendantes de
Cobra, des adaptateurs concrets et des autres features ; entités dépendantes
de la bibliothèque standard. Ports définis par les consommateurs et injection
explicite dans `app`. Contrôler les imports par tests AST de toutes les sources.

Ne créer que les packages qui contiennent un besoin effectif. Conserver les
API exportées Go plutôt que copier les barrels Dart `index.dart`. Aucun
framework DI, service locator, couche `core/shared` systématique ou conteneur
générique de managers. La conformité Mosaera n'est pas revendiquée.

## Alternatives

Un unique package CLI serait rapide mais couplerait le futur moteur à Cobra.
Un découpage technique global controllers/services/repositories mélangerait
la propriété des opérations. Reproduire toutes les couches Flutter de Sonora
créerait des abstractions sans besoin pour une CLI Go.

## Conséquences et validation

Le squelette est petit ; les lots suivants ajoutent leur domaine et leurs ports
sans importer la CLI. Les scénarios entre features demandent une orchestration
supérieure explicite. Les contrôles d'import refusent les violations même dans
des fichiers sous build tags ; leurs cas négatifs sont testés.

Les tests de compilation trouvent les cycles actifs. Le test AST complète les
contraintes de couches, sans prétendre prouver la qualité sémantique du moteur.
Une future exception durable demande un ADR, des tests et une revue humaine.

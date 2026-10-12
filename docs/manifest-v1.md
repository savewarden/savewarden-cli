# Manifeste v1

Un manifeste décrit, pour **un jeu sur une machine**, l'installation et les
dossiers de sauvegarde que Savewarden doit inspecter. Il est écrit par
l'utilisateur et lu par `savewarden inspect --manifest <chemin>`.

**Statut : provisoire jusqu'à `v0.1.0`.** Le schéma `schema_version: 1` peut
encore recevoir des champs en B2 (`strategy`, `target`, `exclude`). Les
décisions de lecture stricte sont fixées par
[ADR 0002](adr/0002-manifest-v1-strict-yaml.md).

## Exemple de référence

```yaml
schema_version: 1

game:
  id: assassins-creed-odyssey
  name: "Assassin's Creed Odyssey"

installation:
  path: 'D:\Games\Assassins Creed - Odyssey'

sources:
  - id: saves
    kind: save
    base: game-install
    path: saves
    required: true
```

Cet exemple est vérifié par les tests. Son chemin d'installation est fictif :
remplacez-le par celui de votre machine. Écrivez les chemins Windows entre
apostrophes simples, car les guillemets doubles YAML interprètent `\`.

## Champs

Tous les champs sont obligatoires. Aucune valeur par défaut n'est appliquée.

| Champ | Règle |
| --- | --- |
| `schema_version` | Entier décimal `1` exact |
| `game.id` | Minuscules, chiffres et tirets simples, 64 caractères au plus, par exemple `cyberpunk-2077` |
| `game.name` | Texte non vide, 200 caractères au plus, sans caractère de contrôle |
| `installation.path` | Chemin Windows absolu avec lettre de lecteur, par exemple `D:\Games\Jeu` |
| `sources` | Liste d'au moins une source |
| `sources[].id` | Même règle que `game.id`, unique dans le manifeste |
| `sources[].kind` | `save`, `profile`, `config`, `controls` ou `other` |
| `sources[].base` | `game-install` : la source est relative à `installation.path` |
| `sources[].path` | Chemin Windows relatif, par exemple `saves` ou `CPY_SAVES\Profil` |
| `sources[].required` | `true` ou `false` |

Une source `required: true` absente rend l'inspection incomplète, code 2.
Une source `required: false` absente est signalée comme information, code 0.

## Chemins Windows

La grammaire est celle de Windows, appliquée de la même façon sur tous les
systèmes. Seules les formes canoniques sont acceptées : rien n'est nettoyé,
complété ni développé.

Refusés dans `installation.path` et `sources[].path` :

- chemins UNC `\\serveur\partage` et préfixes `\\?\`, `\\.\`, `\??\` ;
- chemin relatif au lecteur `D:Jeux` ou à la racine `\Jeux` pour l'installation ;
- lettre de lecteur ou `\` initial dans un chemin de source ;
- segments `.` et `..`, segments vides, `\` final ou doublé ;
- `/` comme séparateur ;
- caractères `< > : " | ? *` et caractères de contrôle ; `:` couvre les flux
  alternatifs NTFS ;
- segment terminé par un point ou une espace, que Windows supprimerait ;
- noms de périphériques réservés, quelle que soit la casse ou l'extension :
  `CON`, `PRN`, `AUX`, `NUL`, `COM0` à `COM9`, `LPT0` à `LPT9`, `COM¹`,
  `CONIN$`, `CONOUT$`, etc. ;
- variables d'environnement `%NOM%`, non développées en v1 ;
- segment de plus de 255 unités UTF-16.

Deux sources ne peuvent ni coïncider ni se contenir, sans tenir compte de la
casse. À l'exécution, `inspect` compare aussi l'identité réelle des dossiers,
ce qui couvre les noms courts 8.3.

## Lecture stricte du fichier

- 64 Kio au plus, UTF-8 valide, sans octet NUL. Un BOM UTF-8 initial et des
  fins de ligne CRLF sont acceptés, pour qu'un fichier du Bloc-notes passe.
  UTF-16 est refusé.
- Un seul document YAML.
- Refus des ancres `&`, alias `*`, clés de fusion `<<` et tags explicites
  comme `!!binary`.
- Refus des champs inconnus et des clés dupliquées, à tous les niveaux.
- Types exacts : `1943` est un entier, à écrire `"1943"` pour un nom ;
  `yes`, `on`, `True` ou `"true"` ne sont pas des booléens.
- Chaque erreur indique le fichier, la ligne, la colonne et le champ :

```text
savewarden: manifest.yaml:15:11: sources[0].path: "." and ".." segments are not allowed
```

## Hors de la v1

- Dossiers connus Windows et variables comme `%USERPROFILE%` ou
  `Saved Games` : prévus avec le pilote Cyberpunk 2077.
- Découverte automatique, par exemple la lecture de `CPY.ini`.
- `strategy`, `target`, `exclude`, `platform`, variantes et profils.

Ne publiez jamais un manifeste contenant des chemins personnels sans les
anonymiser.

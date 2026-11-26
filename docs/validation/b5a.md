# B5a — Rapport de validation

Durcissement S-01 et S-02, selon le
[plan B5 v0.5](../../.plans/Savewarden_Plan_B5_Durcissement_Doctor_RC_v0.5.md),
ADR 0005 et ADR 0006 §11, référence de sécurité. Ce rapport est tenu au fil
des commits de B5a. À cette rédaction, il ne consigne que le commit 1, les
hypothèses Windows HW1 à HW13, et les amendements d'ADR du commit 2, acceptés.
Aucun code de production n'est écrit. **S-01 et S-02 restent ouverts**
jusqu'au gate de sécurité de B5a (plan §8, commit 7). Aucune PR, aucun tag ni
aucune release.

## Cadre

| Élément | Valeur |
| --- | --- |
| GO humains | « Oui, GO B5a », 8 novembre 2026, GO de développement ; « Oui, push et CI » et « Oui, avec l'étape », étape verbeuse de la CI ; GO du protocole HW13 sans élévation, 22 novembre 2026, conditionnel ; « Oui, commit et CI », 24 novembre 2026 ; validation de HW13 sans élévation, 24 novembre 2026 ; validation du gate des hypothèses et GO de préparation locale du commit 2, 26 novembre 2026 ; « Je ratifie les amendements d'ADR 0005 et d'ADR 0006 §11 ainsi que l'erratum E2 au plan B5 v0.5, dans leur révision r3 », 26 novembre 2026, qui n'est pas un GO de commit |
| Branche | `feat/b5a-hardening`, depuis `9f4fea6`, tête de `release/v0.1.0-a1` ; Go 1.27.3 |
| Commit 1 | `05d95fa`, tests HW1 à HW13 ; puis `6ece02c`, VHD par chemin long ; `dc9d350`, étape verbeuse de la CI ; `1780555`, `2c15e9c` et `d2b79f4`, banc HW13 sans élévation. Tous en `_test.go`, hors du garde `readonly_test.go` |
| Code de production | Inchangé : `git diff --stat 9f4fea6..d2b79f4` ne touche que des tests, `.github/workflows/ci.yml` (étape verbeuse), `docs/gitops.md` et `docs/validation/b5a-protocoles.md` |
| Commit 2 | Amendements d'ADR 0005 et d'ADR 0006 §11 et erratum E2 du plan, révisés le 26 novembre 2026 sur trois revues CTO (r1 : P1-01, P1-02, F2, F3, F8 ; r2 : codes de sortie, lectures de décision, fermeture du writer) ; **ratifiés par le propriétaire le 26 novembre 2026, dans la révision r3**, résidu de la classe 2b accepté explicitement |

## Hypothèses Windows (commit 1)

**Run de qualification** : [45297320109](https://github.com/savewarden/savewarden-cli/actions/runs/45297320109),
sur `d2b79f4`, 7 checks sur 7. Les tests HW tournent dans la suite complète
puis dans l'étape verbeuse `go test -count=1 -v -run '^TestHW'` du job
Windows, sur `windows-2025`. Une assertion arrête le test si elle échoue ;
une observation est consignée par le journal sans critère de réussite.

| # | Résultat | Nature | Preuve |
| --- | --- | --- | --- |
| HW1 | Le chemin final d'une épingle est `\\?\Volume{GUID}\…` ; rouvert, même identité ; même chemin par une lettre `subst`. Tampon de 8 : taille 142 rendue, **sans erreur**, troncature détectée par la taille | Assertions | Run 45297320109 |
| HW2 | Par le chemin de volume : 183 pour `CreateDirectory` et `MoveFileEx` sur cible existante, 80 pour `CreateFile` en `CREATE_NEW` ; 17 entre volumes, hors de `Session` ; état inchangé | Assertions | Run 45297320109 |
| HW3 | Chaque préfixe rouvert a l'identité de son épingle ; chaque chemin final prolonge le précédent d'un segment. **Constat** : le segment `RUNNER~1` a pour nom final `runneradmin`. Lettre réattribuée entre deux épingles : chemin qui ne prolonge plus, détecté | Assertions ; constat consigné | Run 45297320109 |
| HW4 | Lettre `subst` retirée puis donnée à un témoin pendant la session : création, fichier et renommage dans la cible tenue, témoin intact | Assertions | Run 45297320109 |
| HW5 | Épingle par chemin de volume : renommage refusé en 32 ; mêmes conflits de partage, 32, par lettre et par chemin de volume | Assertions | Run 45297320109 |
| HW6 | Dossiers et fichier Unicode créés au-delà de `MAX_PATH`, 324 caractères UTF-16 | Assertions | Run 45297320109 |
| HW7 | VHD fixe détaché en CI, puis vraie clé amovible, voir [HW7](#hw7-sur-un-vrai-volume-amovible) : codes 3, 3, 21 ; GUID rouvert, 3 ; après rattachement, ancien handle 21 et **même GUID** ; rien écrit | Assertions ; codes consignés | Run 45297320109 ; protocole du propriétaire |
| HW8 | Masquage effectif : `CreateDirectory` et `CreateFile` atteignent le leurre ; chemin final du fichier détecté ; `MoveFileEx` renomme l'objet du leurre, source tenue intacte, postcondition qui détecte l'absence de la destination | Observation, CI seulement, runner élevé | Run 45297320109 |
| HW9 | Racine absente créée en exclusif par le parent tenu, seconde création refusée en 183, épinglée, chemin final égal à la base, identité par handle | Assertions | Run 45297320109 |
| HW10 | `GetVolumeInformationByHandle` sur l'épingle racine : NTFS et numéro de série, comme par la lettre ; type de lecteur et capacité par `\\?\Volume{GUID}\` égaux | Assertions | Run 45297320109 |
| HW11 | Point de montage d'un VHD parmi les ancêtres : refusé par `vault.Open` par la lettre, vu comme point d'analyse par le chemin de volume | Assertions | Run 45297320109 |
| HW12 | Vraie lettre : trois conditions remplies. Alias `subst` et `DefineDosDevice` d'un sous-dossier : chemin final avec reste, `GetVolumeNameForVolumeMountPoint` en 87, identité différente | Assertions | Run 45297320109 |
| HW13 | Sous un compte local standard éphémère, jeton non élevé, intégrité moyenne : `subst` et `DefineDosDevice` donnent chacun une lettre strictement équivalente à la racine d'un volume, puis la perdent une fois réattribués vers un sous-dossier. Aucune violation, 26 lettres rétablies, compte, profil et processus supprimés. Sous élévation, les deux voies vers le volume du test et vers un autre volume | Assertions ; table de décision | Run 45297320109 |

### HW7 sur un vrai volume amovible

Protocole de [b5a-protocoles.md](b5a-protocoles.md), exécuté par le
propriétaire le 26 novembre 2026, sur une clé USB NTFS dédiée, retirée sans
éjection :

| Élément | Valeur |
| --- | --- |
| Binaire | Compilé par le propriétaire à `d2b79f4`, SHA-256 `b6846c78b7ba38b74371cb2ca9ecfa4d188acf37fa309d9586ec53b86bd71128`. Une recompilation de contrôle par l'agent, même commit et même Go 1.27.3, donne un binaire identique octet pour octet |
| Résultat | `--- PASS: TestHW7OwnerRemovableVolume (30.88s)`, code de sortie 0 |
| HW7a | Clé retirée : `CreateDirectory` 3, `CreateFile` 3, handle de la session 21 |
| HW7c | Chemin GUID rouvert pendant le retrait : 3 |
| HW7b | Clé rebranchée : ancien handle 21 ; même GUID ; `after` et `f` absents, `before` présent |
| Journal | Conservé par le propriétaire, hors du dépôt ; SHA-256 `0b5d2ba6f448a430a86b9227a8a4d86b2ee4f4d0e6ead6f130887a5a26481bdd` |

Les codes égalent ceux du VHD de la CI. Ce résultat n'est jamais présenté
comme obtenu en CI.

### Runs antérieurs, historique

| Run | SHA | Issue | Rôle |
| --- | --- | --- | --- |
| 42531025850 | `05d95fa` | 6/7 | HW7 du VHD arrêté par un défaut du banc, chemin court 8.3, corrigé en `6ece02c` |
| 42533377873 | `dc9d350` | 7/7 | Premiers relevés verbeux ; HW13 sous élévation seulement |
| 44961893884 | `1780555` | Échec | Banc HW13 : `TEMP` hérité, nettoyage interrompu ; corrigé en `2c15e9c` |
| 44962970440 | `2c15e9c` | 7/7 | Exploratoire, avant les deux revues suivantes du banc |

Ils restent l'historique des essais. La preuve officielle est le run
45297320109.

### Gate des hypothèses

- 24 novembre 2026, le propriétaire : « HW13 VALIDÉ SANS ÉLÉVATION », sur le
  run 45297320109, après trois revues du banc.
- 26 novembre 2026, le propriétaire : « Je valide le gate des hypothèses
  Windows du commit 1 de B5a. » HW1 à HW13 sont qualifiées dans leurs
  périmètres respectifs ; HW8 reste une observation de classe 2, détection
  après coup, sans confinement revendiqué.

### Constats reportés dans les amendements d'ADR

| Constat | Preuve | Décision ratifiée |
| --- | --- | --- |
| Le segment final diffère du segment donné : nom court 8.3, casse | HW3 | Contrôle de chaîne segment par segment, préfixe rouvert comparé à l'épingle précédente, jamais par le segment littéral ni par préfixe textuel (ADR 0006 §11 ; erratum E2, F2) |
| Troncature rendue sans erreur | HW1 | Taille par première requête ; refus dès que la taille rendue atteint la capacité du tampon (ADR 0005) |
| Le code 3 est aussi celui d'un chemin introuvable | HW7a, HW7c | Sonde de vivacité par le handle tenu sur toute erreur autre que 15 et 21, sans récursion (ADR 0006 §11 ; erratum E2, F3) |
| Le GUID survit à la réinsertion | HW7b | Sonde avant et après chaque mutation par nom et chaque tentative de `Move` ; invalidation définitive, writers et journal compris ; scénario A en classe 1, scénario B en classe 2b (ADR 0006 §11 ; erratum E2, F1) |
| Masquage local effectif, signaux de détection observés | HW8 | Résidu de classe 2a écrit d'après ces constats seuls ; invalidation à prouver au commit 5 (ADR 0006 §11) |
| Alias de sous-dossier : erreur 87 | HW12 | Toute erreur de la contre-épreuve refuse la lettre (ADR 0006 §11) |
| Équivalent strict d'une racine, sans élévation | HW13 | Admis aux trois conditions, décision 24 (ADR 0006 §11) |

### Limites

- HW8 a tourné sur un runner élevé : la définition sans élévation d'un nom
  `Volume{GUID}` n'est pas exercée ; elle est tenue pour possible.
- HW13, et tout test qui emploie sa voie, tourne en CI seulement.
- HW7 sur volume amovible : une clé, un retrait ; NTFS seulement, ReFS non
  qualifié.
- Les hypothèses ne ferment ni S-01 ni S-02 : leurs essais de bout en bout
  et la revue de sécurité indépendante restent à faire (plan §8, commits 3
  à 7).

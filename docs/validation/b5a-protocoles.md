# B5a — Protocoles d'hypothèses Windows pour le propriétaire

Le [plan B5](../../.plans/Savewarden_Plan_B5_Durcissement_Doctor_RC_v0.5.md)
prouve ses hypothèses Windows avant le code qui en dépend (§5.1, T3). La
plupart tournent en CI. HW7 exige en plus une preuve sur un **vrai volume
amovible**, que seul le propriétaire peut faire (décision 13). Ce document la
décrit. HW11 n'a besoin d'un protocole que si la CI ne peut pas monter de
VHD ; il serait ajouté ici le cas échéant.

## HW7 — volume amovible retiré pendant une session

**Ce qui est prouvé.** Une session tient un dossier de la clé par son chemin
de volume. Clé retirée, chaque appel par ce chemin échoue, et rien n'est
redirigé (HW7a). Le chemin GUID rouvert pendant le retrait échoue aussi
(HW7c). Clé rebranchée, le handle de la session reste mort, et rien n'a été
écrit sur le nouveau montage (HW7b). Les codes d'erreur observés sont
consignés : ils complètent le classement « volume indisponible » du §4.1.

**Ce qui n'est pas touché.** Aucun coffre ni zone de pilote, aucune donnée
réelle. Le test refuse toute lettre qui n'est pas un volume amovible portant
le fichier témoin ci-dessous. Il crée un seul dossier neuf sur la clé et ne
supprime rien.

### Préparation

1. Prenez une clé USB **dédiée au test**, sans aucune donnée à conserver,
   formatée en NTFS.
2. À sa racine, créez un fichier vide nommé exactement
   `SAVEWARDEN-HW7-CLE-DE-TEST`.
3. Fermez toute application qui pourrait ouvrir la clé (Explorateur sur la
   clé, antivirus en analyse, indexation).
4. Notez sa lettre, par exemple `F:`.

### Exécution

Depuis la racine du dépôt, sur le commit à qualifier, dans PowerShell :

```powershell
go test -c -o "$env:TEMP\savewarden-hw7.exe" ./internal/platform/vault
$env:SAVEWARDEN_HW7_VOLUME = 'F:'
& "$env:TEMP\savewarden-hw7.exe" -test.run TestHW7OwnerRemovableVolume -test.v -test.timeout 20m
```

Le binaire de test affiche les consignes au fil de l'eau :

1. « Retirez maintenant la clé » : débranchez-la physiquement, **sans
   l'éjecter** : le test mesure un retrait brutal.
2. « Rebranchez maintenant la clé » : rebranchez-la.

Chaque attente dure 5 minutes au plus.

### Résultat

- `PASS` : HW7 est prouvé sur ce volume. Transmettez la sortie complète, et
  notamment les lignes `HW7a`, `HW7b` et `HW7c`, avec le SHA du commit.
- `FAIL` : HW7 tombe sur ce volume. Arrêt de B5a et arbitrage, selon T3.
- Le dossier `savewarden-hw7-…` reste sur la clé : supprimez-le vous-même
  une fois la sortie transmise.

Ce résultat ne remplace pas celui de la CI, qui prouve le cas d'un volume
fixe détaché, et n'est jamais présenté comme obtenu en CI.

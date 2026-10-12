# Workflow GitOps

Le workflow reprend le modèle release branch de
[Dirloom](https://github.com/dirloom/dirloom/blob/main/docs/release-workflow.md),
adapté au bootstrap et aux artefacts réellement disponibles de Savewarden.

## Initialisation vierge

Le dossier `savewarden-cli` ne contenait initialement que le plan, sans `.git`
ni remote. Destination choisie : `savewarden/savewarden-cli`, **public** ;
licence MIT. La création d'une organisation/d'un remote n'est pas assimilée
à une release produit.

Le seul commit autorisé directement sur `main` est le bootstrap initial B0.
Il peut être créé localement pour rendre le résultat identifiable ; son push
attend présentation du diff et revue humaine. Ne pas fabriquer un faux commit
de base pour ouvrir une PR sans historique. Tant que la revue n'a pas eu lieu,
le commit reste reviewable localement, sans push.

Après ce commit, activer les protections, créer `release/v0.1.0-a1` depuis
ce SHA, puis seulement commencer B1 après Human GO et un plan de lot accepté.
Si un historique existe à la destination, préserver celui-ci et adapter la
procédure : jamais `force`, reset destructeur ou remplacement de `main`.

## Lots versionnés

```text
main → release/v0.1.0-a1 → feat/* → PR vers release/v0.1.0-a1
     → freeze → CI du SHA exact → PR finale vers main (merge commit)
     → tag annoté sur le vrai merge SHA → release draft → Human GO
```

Une release branch par incrément effectivement publiable ; les alphas et RC
ne sont pas des tags obligatoires à créer. Les branches de travail partent de
leur release branch et ciblent celle-ci. PR finale vers `main` sans squash.
Freeze : corrections ciblées seulement, code/tests/docs/changelog cohérents.
Ne jamais réécrire un tag publié ; nouvelle correction = nouvelle version.

## CI B0 et checks requis

Workflow `.github/workflows/ci.yml`, sur PR et push `main`/`release/**`, plus
déclenchement manuel pour vérifier un commit revu. Permissions `contents: read`.
Les noms à configurer comme checks requis sont :

- `Verify (ubuntu-24.04)`
- `Verify (windows-2025)`
- `Verify (macos-15)`
- `Race detector`
- `Lint`
- `Vulnerability scan`
- `Pins`

Verify : modules vérifiés, tidy sans diff, gofmt, vet, tests, smoke binaire
et build CLI. Race sur Linux. Lint 2.14.0 et govulncheck 1.6.0 via modules Go
épinglés. Go 1.27.2 fixé dans go.mod ; OS explicitement versionnés, sans `latest`.
Les Actions sont épinglées par SHA complet ; les pins sont contrôlés.
Ces labels OS sont vérifiés dans le catalogue `actions/runner-images`, mais
les images hébergées continuent à recevoir leurs mises à jour.

Une CI n'est verte qu'après exécution terminée sur son SHA. Le dossier
[validation B0](validation/b0.md) conserve les preuves et les limites
locales ; le [compte rendu post-push](validation/b0-post-push.md) consigne
la CI distante, les protections appliquées et le Human GO B0.
L'absence de remote, de push autorisé ou de run ne doit jamais être présentée
comme une validation GitHub Actions. Pas de jobs de diagrammes/13 artefacts.

## Protections à activer après bootstrap

Rulesets `main` et `release/*` : PR obligatoire, checks ci-dessus requis,
conversations résolues, branches à jour, force-push et suppression interdits,
pas de bypass d'agent. Autoriser les merge commits pour la PR finale ; ne pas
imposer un historique linéaire incompatible avec cette cérémonie.
Protections de tags à fixer avant premier tag.

Un seul mainteneur est identifié à ce stade : Samuelbenny. Ne pas exiger deux
reviews indépendantes fictives ni approuver sa propre PR. Avec un seul
mainteneur, configurer la PR obligatoire et zéro approbation automatique
requise, documenter l'absence de seconde revue et garder le Human GO dans le
rapport. Ajouter un reviewer indépendant lorsque l'équipe le permet.

Les règles cibles ne sont **pas** réputées actives avant vérification de leur
réponse API et de leur enforcement. Le commit initial doit exister avant que
les protections n'empêchent son unique push initial.

## Release et distribution ultérieures

B0 ne fournit ni GoReleaser, release workflow, tag, SBOM ou installation.
Avant premier tag : décider les OS fonctionnellement supportés et archives
distribuées, activer GoReleaser check/snapshot et smoke des archives sur le SHA
exact, checksums, SBOM par archive et attestations réellement vérifiées.

Le pipeline créera une GitHub Release **draft**, jamais publiée automatiquement.
`CHANGELOG.md` est sa seule source de notes. Human GO après audit du tag exact.
Six archives + six SBOM + checksums ne seront annoncés qu'une fois ces treize
artefacts réellement produits et validés. Les canaux Scoop/Homebrew/Winget
resteront séparés et ne suivront que les releases stables publiées et les
recettes testées. Secrets en environnement protégé, jamais en workflow PR.

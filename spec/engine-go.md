# Prompt d’implémentation — moteur Go LibBusinessID

## Mission

Tu dois implémenter intégralement le dépôt Go du moteur LibBusinessID. Avant toute
modification, lis entièrement `engine.md` situé à côté de ce document. Avant de
coder, exige et lis aussi `rules.proto`, `conformance.proto`, `ir.md`, `features.md`,
le manifeste, `SHA256SUMS`, l’attestation et les deux BINPB de la même release. Si un
élément manque ou se contredit, arrête-toi et fais corriger le dépôt `spec` : ne
déduis jamais un opcode depuis l’interpréteur Go de référence. `engine.md`
est la spécification normative commune ; ce document ajoute les décisions propres
à Go. En cas de conflit, `engine.md` prévaut sur les préférences de style, sauf si une
contrainte Go rend l’API proposée impossible : dans ce cas, documente l’écart et
préserve la sémantique sérialisée commune.

Le moteur ne lit pas le bundle à l'exécution : un générateur le lit à la construction
et émet du code, comme l'exige `engine.md` section 1.1. Il n'existe donc aucune
fabrique publique acceptant un bundle en octets, et un jeu de règles personnalisé
passe par le générateur.

Le résultat doit être une bibliothèque Go de production : sûre, idiomatique,
documentée, fortement testée et installable comme module public. Ne réalise pas un
port ligne à ligne d’un autre moteur.

## Résultat fonctionnel attendu

La bibliothèque doit :

- embarquer `businessid-rules.binpb` ;
- décoder et valider défensivement l’IR Protobuf ;
- exécuter canonicalisation, format et checksum ;
- passer intégralement `businessid-conformance.binpb` ;
- exposer les versions moteur/règles/format et capabilities ;
- permettre de construire un moteur depuis des bytes personnalisés ;
- définir l’interface de registre, sans provider concret ni dépendance HTTP ;
- être sûre en concurrence ;
- ne jamais panic sur une entrée utilisateur ou un bundle non fiable.

## Module et package

- Module attendu : `github.com/libbusinessid/businessid-go`, sauf module déjà fixé
  par le dépôt.
- Le package public principal s’appelle `businessid`.
- Le package doit fonctionner avec la version stable de Go déclarée dans `go.mod`.
- Choisir et documenter une version minimale réaliste ; la CI teste cette version et
  la dernière stable.
- Éviter les dépendances sauf nécessité claire. Protobuf officiel Go est autorisé.
- Aucun package interne ne doit être importable par les consommateurs.

Structure recommandée :

```text
.
├── businessid.go
├── input.go
├── result.go
├── reason.go
├── registry.go
├── errors.go
├── engine.go
├── options.go
├── internal/
│   ├── irpb/              # code Protobuf généré
│   ├── rules/
│   ├── runtime/
│   ├── canonicalize/
│   ├── checksum/
│   └── limits/
├── assets/
│   └── businessid-rules.binpb
├── conformance/
│   └── businessid-conformance.binpb
├── cmd/businessid-example/
└── integration/
```

Adapte la structure si une alternative est plus idiomatique, mais conserve une
séparation nette entre API domaine, décodage Protobuf, validation du bundle et
interpréteur.

## API Go attendue

Proposer une API proche de :

```go
type Engine struct { /* immutable after construction */ }

func Default() *Engine
func New(rules []byte, opts ...EngineOption) (*Engine, error)

func (e *Engine) Canonicalize(
    input IdentifierInput,
    opts ...ValidationOption,
) CanonicalizationResult
func (e *Engine) Validate(input IdentifierInput, opts ...ValidationOption) ValidationReport
func (e *Engine) ValidateFormat(input IdentifierInput, opts ...ValidationOption) ValidationReport
func (e *Engine) ValidateChecksum(input IdentifierInput, opts ...ValidationOption) ValidationReport
func (e *Engine) RulesInfo() RulesInfo
func (e *Engine) Capabilities() Capabilities
```

Si une erreur interne théoriquement possible doit remonter pendant la validation,
une signature `(ValidationReport, error)` est acceptable, mais les entrées métier
invalides ne doivent jamais être des `error`. Décide une convention unique et
documente-la.

Types publics :

- structs immuables par convention, champs exportés seulement si utiles ;
- enums sous forme de types chaîne avec constantes ;
- `IdentifierKind` est un `type string` extensible avec constantes connues, jamais
  un enum fermé qui empêcherait `unsupported_kind` ;
- `String()` si pertinent ;
- JSON tags conformes à `engine.md` si sérialisation fournie ;
- aucune fuite de types Protobuf ;
- pas d’interface prématurée pour le moteur lui-même ;
- options fonctionnelles seulement lorsque plusieurs configurations réelles existent.

`Default()` doit être initialisé une seule fois avec `sync.Once` ou équivalent sûr.
Une erreur de bundle embarqué est un défaut de build ; elle doit être exposée de
façon testable sans rendre les entrées utilisateur panics. Une fonction
`DefaultEngine() (*Engine, error)` peut être préférée si elle rend le contrat plus sûr.

## Interface registre

Définir une interface idiomatique avec `context.Context` :

```go
type RegistryProvider interface {
    Supports(kind IdentifierKind, countryCode string) bool
    Lookup(ctx context.Context, input RegistryInput) (RegistryResult, error)
}
```

- aucun provider concret ;
- aucun client HTTP dans le module cœur ;
- contexte en premier paramètre ;
- distinguer résultat métier et erreur de transport future ;
- ne pas stocker de `context.Context` dans une struct ;
- `Validate` ne reçoit pas de contexte car il est purement local, sauf justification
  API documentée.

## Protobuf et artefacts

- Utiliser `google.golang.org/protobuf`.
- Générer les types dans `internal/irpb` et ne pas les exposer.
- Verrouiller les versions de `protoc`, `protoc-gen-go` ou utiliser Buf selon la CI.
- Commettre le code généré si cela rend le build consommateur indépendant des outils.
- Ajouter une commande `make generate` et `make check-generated`.
- Embarquer le bundle officiel avec `//go:embed`.
- Vérifier les hashes lors du workflow de mise à jour ; ne pas resérialiser Protobuf
  pour vérifier l’intégrité.
- Le runtime ne parse jamais HCL ou JSONL.

## Style et sûreté Go

- `gofmt` et `goimports` obligatoires ;
- erreurs enveloppées avec `%w` ;
- erreurs sentinelles ou types structurés compatibles avec `errors.Is/As` ;
- pas de panic hors invariant véritablement impossible ;
- aucun `init()` cachant de l’I/O ou une erreur ;
- pas de global mutable ;
- slices et maps internes copiées ou non exposées ;
- vérifier les conversions d’entiers et indices avant slicing ;
- utiliser des calculs modulo chiffre par chiffre ;
- commentaires GoDoc sur tous les symboles exportés ;
- exemples exécutables pour le chemin heureux et `unsupported` ;
- préférer code explicite et table-driven aux abstractions réflexives ;
- éviter `reflect` dans le cœur d’exécution ;
- aucune dépendance à la locale.

Le package doit passer sans avertissement :

```text
gofmt -w / check diff
go vet ./...
golangci-lint run
govulncheck ./...
```

Configure notamment `staticcheck`, `errcheck`, `govet`, `ineffassign`, `revive`,
`gosec`, `bodyclose` si pertinent, et les linters de complexité avec seuils
raisonnables. Toute suppression de lint doit être locale et commentée.

## Méthode de développement

TDD obligatoire :

1. écrire un test échouant ;
2. implémenter le minimum ;
3. refactorer ;
4. exécuter tests unitaires et conformité ;
5. ajouter fuzz/property tests pour toute frontière de confiance.

Ne commence pas par implémenter toute l’IR. Construis verticalement : chargement
d’un bundle minimal, une opération, un rapport, un cas de conformité, puis étends.

## Tests Go obligatoires

- tests table-driven pour chaque opération ;
- sous-tests nommés et `t.Parallel()` lorsque réellement sûrs ;
- tests du bundle embarqué et personnalisé ;
- tests de toutes les limites et erreurs de graphe ;
- tests JSON si API de sérialisation ;
- tests de concurrence et `go test -race ./...` ;
- tests d’exemples `Example...` ;
- fuzz targets natifs `testing.F` pour Protobuf, chaînes, indices, arithmetic et
  canonicalisation ;
- benchmarks `testing.B` pour cold load, validation, invalidité précoce, checksum
  complexe et parallélisme ;
- conformité commune sans filtre ;
- test d’intégration d’un module consommateur minimal.

Quality gates :

- lignes ≥ 95 % ;
- branches couvertes indirectement par mutation/property tests ;
- 100 % dispatch IR et refus version/feature ;
- race detector vert ;
- fuzz smoke dans chaque PR, fuzz long planifié ;
- mutation testing périodique avec un outil maintenu, score cœur recommandé ≥ 80 %.

Les fichiers `.pb.go` générés peuvent être exclus du calcul de couverture, pas le
validateur de bundle ni l’interpréteur.

## CI GitHub Actions

Créer des workflows pour :

- versions Go minimale et actuelle ;
- Linux, macOS et Windows si le package est annoncé multiplateforme ;
- format/lint/vet ;
- unit + conformance ;
- race sous plateforme supportée ;
- couverture avec seuil ;
- fuzz smoke ;
- vulnérabilités ;
- `check-generated` ;
- build d’exemple consommateur ;
- publication de tag/module après validation.

Une CI planifiée réalise fuzz long, benchmarks et mutation tests. Conserver les
résultats de couverture et crashers comme artefacts.

## Documentation et packaging

Le README doit montrer :

- installation `go get` ;
- validation complète ;
- format seul ;
- interprétation de `unsupported` ;
- versions des règles ;
- construction depuis bytes ;
- interface registre sans implémentation ;
- limites : format/checksum ne prouvent pas l’existence.

Ajouter `LICENSE`, `SECURITY.md`, `CONTRIBUTING.md`, changelog et exemples. Préparer
les tags SemVer. Vérifier `go list`, `go test` et documentation pkg.go.dev.

## Interdictions spécifiques

- ne pas réutiliser directement l’interpréteur de référence du dépôt `spec` ;
- ne pas ajouter les dépendances HCL au module runtime ;
- ne pas exposer `proto.Message` ;
- ne pas utiliser des `regexp` pour recréer les règles ;
- ne pas effectuer d’I/O au chargement du package ;
- ne pas avaler une erreur Protobuf ;
- ne pas utiliser `unsafe` sans ADR, benchmark et revue de sécurité ;
- ne pas réduire la conformité ou les seuils pour faire passer la CI.

## Livrables

- module Go complet ;
- API et runtime ;
- code Protobuf généré et reproductible ;
- bundle et conformité verrouillés ;
- interface registre ;
- tests, fuzzers, benchmarks ;
- CI et release ;
- documentation et exemples ;
- rapport final indiquant versions, couverture, commandes exécutées, limitations et
  correspondance exacte avec `engine.md`.

## Définition de terminé

Tous les critères de `engine.md` sont satisfaits, `go test -race ./...`, lint,
conformité, couverture, fuzz smoke et test consommateur sont verts. Aucun TODO de
fonctionnalité V1, règle nationale hardcodée, provider réseau ou divergence connue
ne subsiste.

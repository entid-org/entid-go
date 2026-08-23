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
et émet du code, comme l'exige `engine.md` section 1.2. Il n'existe donc aucune
fabrique publique acceptant un bundle en octets, et un jeu de règles personnalisé
passe par le générateur.

Le résultat doit être une bibliothèque Go de production : sûre, idiomatique,
documentée, fortement testée et installable comme module public. Ne réalise pas un
port ligne à ligne d’un autre moteur.

## Résultat fonctionnel attendu

La bibliothèque doit :

- générer, à la construction, le code des canonicalisations, formats et checksums à
  partir de `businessid-rules.binpb` ;
- décoder et valider défensivement l’IR Protobuf **dans le générateur**, en appliquant
  les contrôles de chargement de `ir.md` section 10 et en refusant d’émettre du code
  si l’un échoue ;
- exécuter canonicalisation, format et checksum depuis le code généré, sans décodeur
  ni machine d’exécution de l’IR dans la bibliothèque publiée ;
- passer intégralement `businessid-conformance.binpb` ; le testee répond aux cas
  `load_ruleset` en appelant le générateur, comme le dit le champ 7 de
  `testee.proto` ;
- exposer les versions moteur/règles/format et capabilities ;
- être sûre en concurrence ;
- ne jamais panic sur une entrée utilisateur ou un bundle non fiable.

## Module et package

- Module attendu : `github.com/libbusinessid/businessid-go`, sauf module déjà fixé
  par le dépôt.
- Le package public principal s’appelle `businessid`.
- Le package doit fonctionner avec la version stable de Go déclarée dans `go.mod`.
- Choisir et documenter une version minimale réaliste ; la CI teste cette version et
  la dernière stable.
- La bibliothèque publiée n'a aucune dépendance : elle ne contient que du code émis
  et ses primitives. Le générateur, lui, utilise le Protobuf officiel Go.
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
├── rules_gen.go           # code généré depuis le bundle, jamais édité
├── internal/
│   ├── runtime/           # primitives que le code généré appelle
│   ├── canonicalize/
│   ├── checksum/
│   └── limits/
├── cmd/businessid-gen/    # le générateur : lit le bundle, valide, émet rules_gen.go
├── cmd/businessid-example/
└── integration/
```

Le générateur et la bibliothèque sont deux programmes. Le décodage Protobuf, les
contrôles de chargement et la connaissance des opcodes vivent dans le
générateur ; la bibliothèque publiée ne contient que le code émis, les primitives
qu'il appelle et l'API. Aucun `.binpb` n'est compilé dans le paquet.

Adapte la structure si une alternative est plus idiomatique, mais conserve cette
séparation : ce qui lit le bundle ne doit pas être ce qui est livré.

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

`Default()` ne peut pas échouer et ne retourne donc pas d'erreur : il n'y a rien à
charger ni à valider à l'exécution, le jeu de règles est du code émis. Un défaut du
jeu de règles se voit à la génération, où le build s'arrête, jamais chez l'appelant.

### Registre : rien à livrer en V1

Ce moteur n'expose aucun type de registre. `engine.md` section 10 diffère la
consultation d'un registre distant à une version ultérieure, et un moteur qui n'en
porte pas est pleinement conforme.

Trois choses à ne pas faire, parce qu'elles fermeraient la porte :

- n'exposez aucun type de registre, même marqué expérimental : un type public est un
  engagement que SemVer fige ;
- ne rendez aucune méthode de validation asynchrone « au cas où » — la validation
  locale reste synchrone définitivement ;
- ne mettez aucune dépendance HTTP dans le paquet du cœur.

Une consultation de registre porte un jeton d'API : elle ne devra jamais être possible
depuis un navigateur, et vivra donc dans un module séparé, chargé côté serveur
uniquement.


## Protobuf et artefacts

- Utiliser `google.golang.org/protobuf` **dans le générateur**. Le module publié
  ne l'importe pas, ce qu'un test doit vérifier plutôt qu'affirmer.
- Générer les types dans `internal/irpb` et ne pas les exposer.
- Verrouiller les versions de `protoc`, `protoc-gen-go` ou utiliser Buf selon la CI.
- Commettre le code généré si cela rend le build consommateur indépendant des outils.
- Ajouter une commande `make generate` et `make check-generated`.
- Ne pas embarquer le bundle : ce qui est compilé dans la bibliothèque est le code
  généré à partir de lui. Le `.binpb` est une entrée du générateur, pas une donnée du
  paquet publié.
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
- tests du générateur : bundle valide, bundles hostiles, jeu de règles personnalisé ;
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
validateur de bundle du générateur, ni les primitives que le code émis appelle.

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

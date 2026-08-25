# EntID — Spécification du dépôt `spec`

## 1. Statut et objectif du document

Ce document est la spécification d’implémentation complète du dépôt
`github.com/entid-org/spec`. Il doit permettre à un agent de développement de
créer le dépôt depuis zéro sans inventer d’architecture, de contrat ou de politique
de compatibilité non décrit ici.

Les mots **DOIT**, **NE DOIT PAS**, **DEVRAIT**, **NE DEVRAIT PAS** et **PEUT** sont
normatifs.

Le dépôt `spec` est la source de vérité de EntID. Il contient :

- le langage HCL utilisé pour écrire les définitions d’identifiants ;
- les schémas Protobuf de l’IR et des cas de conformité ;
- le compilateur/linker `entidc`, écrit en Go ;
- les règles officielles de format et de checksum ;
- les cas de conformité communs aux moteurs Go, Swift, Kotlin et TypeScript ;
- un interpréteur de référence utilisé uniquement pour vérifier la spécification ;
- les outils de formatage, lint, inspection, comparaison et publication ;
- la documentation générée et les artefacts de release.

Le dépôt ne contient aucune implémentation de consultation de registre distant.

## 2. Principes non négociables

### 2.1 Fiabilité avant couverture

Un faux négatif — refuser un identifiant valide — est considéré comme le défaut le
plus grave du projet. Une valeur ne DOIT être déclarée `invalid` que lorsqu’une
règle documentée et applicable prouve son invalidité.

En cas de règle inconnue, d’algorithme non publié, de variante ambiguë ou de
couverture incomplète, le résultat DOIT être `unsupported`, jamais `invalid`.

Le projet ne prétend pas qu’une entreprise existe lorsqu’il vérifie uniquement le
format ou le checksum. Les termes doivent rester précis :

- `format valid` : la forme est compatible avec une variante documentée ;
- `checksum valid` : le contrôle interne documenté est satisfait ;
- aucune conclusion d’existence ou d’activité n’est fournie sans registre.

### 2.2 Une source, plusieurs moteurs idiomatiques

Les règles et les cas de conformité sont communs. Les moteurs sont indépendants et
idiomatiques à leur langage. Aucun moteur ne doit être une traduction mécanique
ligne par ligne d’un autre moteur.

Deux rôles distincts sont à distinguer dans tout ce document.

Le **générateur** est l’outil qui lit le bundle IR, le valide et produit du code
source dans un langage cible. Il s’exécute chez l’auteur du moteur, jamais chez le
consommateur de la bibliothèque. Chaque langage fournit son propre générateur, écrit
dans le langage de son choix ; ce dépôt n’en impose aucun et n’en héberge aucun.

Le **moteur** est la bibliothèque publiée : le code généré, les primitives de
support qu’il appelle, et une API publique écrite à la main. Le moteur n’interprète
pas le bundle et n’en embarque pas nécessairement une copie.

Cette séparation est ce qui permet à la spécification de rester agnostique : elle
définit une sémantique et un artefact, jamais une stratégie d’exécution.

### 2.3 Génération fermée en cas d’incompatibilité

Un générateur qui ne comprend pas une version, un champ, un opcode ou une
fonctionnalité requise par un bundle DOIT refuser de produire du code. Il NE DOIT
PAS ignorer une opération inconnue et générer partiellement.

Le refus est une propriété du temps de génération, jamais du temps d’exécution : un
moteur généré ne rencontre par construction aucune construction qu’il ne comprend
pas, puisque tout ce qu’il contient a été produit à partir d’un bundle accepté.

Un moteur NE DOIT PAS interpréter le bundle à l’exécution. Le bundle est lu par le
générateur, au moment de la construction, et ce qui est livré est le code produit.
Cette phrase remplace une tolérance antérieure qui autorisait l’interprétation à
condition de produire les mêmes résultats : elle contredisait `PROVENANCE.md`, laissait
`engine.md` muet sur le sujet, et un moteur l’a suivie de bonne foi jusqu’à livrer un
interpréteur complet.

La raison n’est pas l’uniformité pour elle-même. Un interpréteur porte à l’exécution,
chez chaque appelant, le validateur complet des vingt-cinq contrôles et la machine
d’exécution de soixante-trois opcodes — soit une surface d’attaque et un coût que le
générateur paie une fois, à la construction, et que le code livré ne porte plus. Le
refus reste une propriété du temps de génération.

### 2.4 Déterminisme observable

À bundle, entrée et contexte identiques, tous les moteurs DOIVENT produire le même
résultat sémantique : valeur canonique, étapes exécutées, statuts et codes de raison.
Les textes humains peuvent différer et ne font pas partie du contrat normatif.

Les « étapes exécutées » désignent les niveaux de validation observables — format et
checksum, chacun avec son statut et son code de raison — et non une trace des nœuds
IR parcourus. Aucune trace d’exécution n’est normative.

Le contrat porte donc sur les sorties, jamais sur la méthode. Un moteur PEUT
interpréter le bundle, compiler ses règles en code natif, ou employer toute autre
stratégie, dès lors que les sorties observables sont identiques. Cette liberté est
délibérée : elle autorise les optimisations qu’un code généré rend possibles, comme
la suppression des branches mortes ou l’absence totale d’allocation.

### 2.5 Aucune dépendance réseau

La compilation des règles et l’exécution des tests de conformité doivent pouvoir
fonctionner hors ligne après installation initiale des dépendances et outils.

### 2.6 Support des variantes historiques

Le critère d’inclusion d’une variante est l’usage, jamais la date d’émission.

Une variante DOIT être supportée tant qu’une valeur conforme à cette variante peut
légitimement apparaître dans une donnée traitée aujourd’hui. Cela couvre les
identifiants encore portés par une entité existante et les identifiants figurant
dans des données en circulation, même lorsque l’entité a disparu : un système qui
traite une facture ancienne doit continuer d’accepter l’identifiant qu’elle porte.

Une variante qui n’est plus émise mais qui satisfait ce critère est une variante
historique. Elle DOIT rester acceptée par le profil `compatible` et PEUT être
refusée par `strict_current`, qui n’accepte que les variantes d’émission actuelle.
Le profil est le seul mécanisme normatif distinguant une variante historique d’une
variante actuelle.

Une variante NE DOIT être retirée que lorsqu’une source documente qu’elle a cessé
de circuler. Un format dont plus aucune valeur ne circule NE DOIT PAS être
supporté : l’élargir sans preuve d’usage augmente le risque de faux positif sans
supprimer aucun faux négatif.

La charge de la preuve est asymétrique. Inclure une variante exige une source
attestant son existence ; l’exclure exige une source attestant sa disparition. En
l’absence de preuve, la variante est supportée, conformément à 2.1.

## 3. Périmètre de la première version

La V1 couvre uniquement :

- canonicalisation ;
- validation de format ;
- validation de checksum ;
- métadonnées de provenance ;
- profils de compatibilité ;
- cas de conformité.

La V1 ne couvre pas :

- requêtes vers VIES, INSEE, GLEIF, Companies House ou tout autre registre ;
- cache réseau ;
- preuve d’existence, de statut actif ou de propriété ;
- détection automatique probabiliste du type d’identifiant ;
- code arbitraire dans les règles ;
- regex dépendantes d’un runtime ;
- téléchargement dynamique de règles par les moteurs par défaut.

La première release stable DEVRAIT migrer la couverture hors ligne du dépôt Go
historique `hyperscale-stack/entid`, après vérification de chaque source et de
chaque vecteur. Une règle historique ne doit pas être copiée sans provenance.

## 4. Architecture générale

```text
rules/*.hcl + conformance/*.jsonl
                 │
                 ▼
       parser / linker / typechecker
                 │
                 ▼
 IR typée : dispatchers + définitions + programmes
                 │
        ┌────────┴─────────┐
        ▼                  ▼
entid-rules.binpb  conformance.binpb
        │                  │
        │                  └──────────────┐
        ▼                                 ▼
générateur du langage cible        runner de conformité
  (hors de ce dépôt)                (dans ce dépôt)
        │                                 │
        ▼                                 │
  code source généré                      │
        │                                 │
        ▼                                 │
moteur publié = code généré ◀──── testé ──┘
  + primitives + API
```

HCL est uniquement un langage d’auteur. Aucun moteur de production n’analyse HCL.
Toutes les références symboliques sont résolues par `entidc`.

Le bundle est destiné à être consommé par un générateur au moment où le moteur est
construit, et non par le moteur au moment où il valide un identifiant. Ce dépôt
produit et atteste l’artefact ; il ne produit aucun générateur et ne connaît aucun
langage cible.

La conformité est vérifiée par un protocole d’exécution commun décrit en 8.5, et non
par une suite de tests réécrite dans chaque moteur. Toute implémentation, y compris
tierce et dans un langage que ce projet ne publie pas, se déclare conforme en
satisfaisant ce protocole.

## 5. Organisation du dépôt

La structure cible est :

```text
.
├── README.md
├── LICENSE
├── SECURITY.md
├── DATA_POLICY.md
├── CONTRIBUTING.md
├── CODE_OF_CONDUCT.md
├── Makefile
├── go.mod
├── go.sum
├── buf.yaml
├── buf.gen.yaml
├── .golangci.yml
├── .editorconfig
├── .github/
│   ├── workflows/
│   │   ├── ci.yml
│   │   ├── release.yml
│   └── dependabot.yml
├── cmd/entidc/
├── cmd/conformance-runner/
├── internal/
│   ├── ast/
│   ├── hcllang/
│   ├── linker/
│   ├── typecheck/
│   ├── lower/
│   ├── optimize/
│   ├── artifact/
│   ├── conformance/
│   ├── reference/
│   └── diagnostics/
├── proto/entid/
│   ├── ir/v1/rules.proto
│   ├── conformance/v1/conformance.proto
│   └── testee/v1/testee.proto
├── gen/go/                       # code Protobuf généré
├── rules/
│   ├── common/
│   ├── global/
│   ├── vat/
│   ├── euid/
│   └── national/
├── conformance/
│   ├── global/
│   ├── vat/
│   ├── euid/
│   └── national/
├── docs/
│   ├── language.md
│   ├── ir.md
│   ├── features.md
│   ├── conformance.md
│   └── generated/
├── testdata/
├── tools/
└── dist/                         # ignoré par Git sauf fixtures dédiées
```

Le code généré DOIT être reproductible avec les versions d’outils verrouillées. Les
fichiers HCL et JSONL sont édités manuellement ; les `.binpb`, manifestes, tables de
couverture et documents sous `docs/generated/` sont générés.

## 6. Le langage HCL EntID

### 6.1 Choix de conception

Le langage utilise HCL v2 natif. HCL fournit la syntaxe, les expressions et les
diagnostics de position ; EntID définit toute la sémantique.

Le compilateur utilise une approche en deux passes :

1. découverte des fichiers, symboles, labels et dépendances ;
2. résolution typée, détection des cycles et construction de l’IR.

Il n’existe **aucun mécanisme d’import en V1**. `entidc` découvre récursivement
les fichiers `*.hcl` sous `rules/`, les trie par chemin relatif POSIX puis les analyse
comme une seule unité de compilation et une seule table de symboles globale. Il
ignore uniquement les répertoires cachés ainsi que `dist/` et `vendor/`. Les liens
symboliques sont refusés. Une règle ne peut donc pas dépendre de l’ordre du système
de fichiers, du répertoire courant ou d’un fichier extérieur à `rules/`.

Les interpolations textuelles du type
`"FR.${rule.fr.siren.format}"` NE DOIVENT PAS servir à composer des règles. Les
références sont structurelles et typées.

### 6.2 Espaces de noms

Les catégories de symboles V1 sont :

- `canonicalizer.<namespace>.<name>` ;
- `format.<namespace>.<name>` ;
- `checksum.<namespace>.<name>` ;
- `dispatcher.<kind>` ;
- `identifier.<kind>.<country_or_global>`.

Les déclarations réutilisables utilisent deux labels HCL pour le namespace et le
nom :

```hcl
format "fr" "siren" {
  # ...
}
```

Une référence s’écrit directement :

```hcl
rule = format.fr.siren
```

Les symboles sont immuables. Les doublons et les cycles sont des erreurs de
compilation.

`identifier` utilise les labels kind/pays ; `dispatcher` utilise un seul label kind,
comme montré en 6.11.

### 6.3 Types du langage

Le typechecker connaît au minimum :

- `StringExpr` : produit une chaîne ou une vue de chaîne ;
- `IntExpr` : produit un entier borné ;
- `BoolExpr` : produit un booléen ;
- `CanonicalizationStep` ;
- `Predicate` ;
- `Assertion` ;
- `FormatRule` ;
- `ChecksumRule` : produit `valid`, `invalid` ou `unsupported` ;
- `SourceRef` ;
- listes et objets fermés de ces types.

Il n’existe aucun type dynamique dans l’IR finale. Toute expression doit être
entièrement typée avant émission.

### 6.4 Contexte d’exécution normatif

Une règle peut lire uniquement :

- `input.value` : valeur brute fournie par l’appelant ;
- `input.country_code` : code pays optionnel ;
- `input.kind` : type demandé ;
- `options.profile` : `compatible` ou `strict_current` ;
- la valeur courante pendant la canonicalisation ;
- les captures produites par le parseur de format.

Aucune heure système, variable d’environnement, locale, réseau ou source aléatoire
n’est accessible.

### 6.5 Unicode et ASCII

Les identifiants sont évalués par points de code Unicode, mais les classes
`digits`, `letters` et `alphanumeric` de la V1 désignent exclusivement ASCII :

- chiffres : `U+0030..U+0039` ;
- lettres majuscules : `U+0041..U+005A` ;
- lettres minuscules : `U+0061..U+007A`.

`uppercase_ascii()` ne transforme que `a..z`. Les moteurs NE DOIVENT PAS utiliser
la locale courante.

La classe `whitespace_v1` est figée et contient :

```text
U+0009..U+000D, U+0020, U+0085, U+00A0, U+1680,
U+2000..U+200A, U+2028, U+2029, U+202F, U+205F,
U+3000, U+FEFF
```

Les runtimes NE DOIVENT PAS déléguer cette définition à leurs propres fonctions
Unicode, dont les versions peuvent différer.

Les positions et longueurs des formats métier sont comptées en points de code après
canonicalisation. Comme les alphabets valides V1 sont ASCII, une fois le format
établi les indices d’octets et de points de code coïncident.

### 6.6 Déclarations de canonicalisation

Exemple :

```hcl
canonicalizer "vat" "common" {
  steps = [
    trim_whitespace(),
    uppercase_ascii(),
    remove_whitespace(),
    remove_chars([".", "-"]),
    replace_prefix("GR", "EL"),
    replace_prefix("UK", "GB"),
    prepend_country_if_missing(),
  ]
}
```

Opérations V1 obligatoires :

| Fonction | Sémantique |
|---|---|
| `trim_whitespace()` | retire `whitespace_v1` aux extrémités |
| `remove_whitespace()` | retire tous les caractères `whitespace_v1` |
| `uppercase_ascii()` | transforme uniquement `a..z` en `A..Z` |
| `remove_chars(list)` | retire une liste explicite de caractères |
| `replace_prefix(from, to)` | remplace un préfixe exact s’il est présent |
| `prepend(value)` | ajoute une chaîne |
| `append(value)` | ajoute une chaîne en suffixe |
| `insert(index, value)` | insère à une position vérifiée |
| `left_pad(length, char)` | complète jusqu’à une longueur, sans tronquer |
| `prepend_country_if_missing()` | ajoute le pays normalisé si aucun préfixe pays ASCII n’est présent |
| `when(predicate, step...)` | applique les étapes si le prédicat est vrai |

Une opération hors limites ou impossible est une erreur de canonicalisation interne,
pas un résultat `invalid`. Le compilateur DOIT rejeter les opérations manifestement
impossibles statiquement.

La canonicalisation doit être idempotente. `entidc lint` DOIT vérifier cette
propriété sur tous les cas de conformité et sur des valeurs générées.

L'idempotence est énoncée sur les entrées valides, c'est-à-dire sur de l'UTF-8
bien formé. Elle ne vaut pas sur une suite d'octets arbitraire, et ne peut pas
valoir : retirer un espace peut recoller deux fragments malformés en un code
point valide que la passe suivante retirerait à son tour. C'est pourquoi une
entrée qui n'est pas de l'UTF-8 valide est refusée avant toute canonicalisation,
avec `invalid_encoding` — un identifiant est une suite de points de code, et des
octets qui n'en forment pas n'en ont aucun à évaluer. Un moteur n'a donc jamais à
canonicaliser une valeur malformée, et la propriété tient sur tout le domaine où
elle est énoncée.

### 6.7 Expressions de chaîne et captures

Constructeurs V1 :

| Fonction | Résultat |
|---|---|
| `value()` | valeur canonique courante |
| `subject()` | entrée courante du programme ou de la règle réutilisée |
| `country_code()` | code pays canonique du contexte |
| `slice(expr, start, end)` | sous-chaîne `[start,end)` |
| `slice_from(expr, start)` | sous-chaîne depuis `start` |
| `slice_to(expr, end)` | sous-chaîne avant `end` |
| `before_first(expr, delimiter)` | partie avant le premier séparateur |
| `after_first(expr, delimiter)` | partie après le premier séparateur |
| `strip_prefix(expr, prefix)` | retire un préfixe exact, sinon valeur absente |
| `concat(expr...)` | concatène des chaînes |
| `capture(name)` | référence une capture nommée validée |

Les opérations pouvant échouer produisent une valeur absente typée. Un prédicat qui
lit une valeur absente retourne `false`, sauf opération explicite `is_absent`.

Le sujet par défaut d’un format ou checksum est `value()`. Une déclaration peut le
remplacer avec `subject = <StringExpr>`. Le compilateur résout cet attribut avant les
autres expressions du bloc ; `subject()` désigne ensuite cette valeur. Lorsqu’une
règle est réutilisée avec `use_format` ou `apply_checksum`, son `subject()` est la vue
fournie par l’appelant.

### 6.8 Prédicats V1

Les prédicats obligatoires sont :

```text
is_empty(expr)
is_absent(expr)
equals(left, right)
length_eq(expr, n)
length_in(expr, [n...])
length_between(expr, min, max)
ascii_digits(expr)
ascii_upper_letters(expr)
ascii_alphanumeric(expr)
ascii_charset(expr, chars)
starts_with(expr, prefix)
ends_with(expr, suffix)
prefix_in(expr, prefixes)
char_at_in(expr, index, chars)
contains(expr, literal)
all(predicate...)
any(predicate...)
not(predicate)
profile_is(name)
```

Il n’y a pas de regex générique en V1. Une nouvelle primitive peut être ajoutée
uniquement si sa sémantique est identique et testable sur les quatre moteurs.

### 6.9 Règles de format

Une règle de format est une suite ordonnée d’assertions. La première assertion qui
échoue détermine le `reason_code` et le `message_key`.

```hcl
format "fr" "siren" {
  checks = [
    require(not(is_empty(subject())), "empty", "fr.siren.empty"),
    require(length_eq(subject(), 9), "invalid_length", "fr.siren.length"),
    require(ascii_digits(subject()), "invalid_characters", "fr.siren.characters"),
  ]
}
```

`require(predicate, reason_code, message_key)` est la seule construction qui produit
une invalidité de format. Les `reason_code` sont pris dans le registre normatif de
la section 8.4, maintenu identiquement dans `engine.md`.

Une règle peut déclarer des captures :

```hcl
format "euid" "generic" {
  capture "registration" {
    value = after_first(value(), ".")
  }

  checks = [
    # assertions sur la structure EUID
  ]
}
```

Une règle peut réutiliser une autre règle sur une vue :

```hcl
use_format {
  rule  = format.fr.siren
  input = capture.registration
}
```

Les checks du parent s’exécutent avant `use_format`. Le compilateur DOIT détecter
les cycles et préserver les codes de raison de la règle appelée.

### 6.10 Règles de checksum

Une règle de checksum retourne l’un de trois états :

- `valid` ;
- `invalid` avec `invalid_checksum` ;
- `unsupported` avec une raison stable.

Exemple :

```hcl
checksum "fr" "siren" {
  rule = luhn(subject())
}
```

Pour une variante sans algorithme publié :

```hcl
checksum "is" "vat" {
  subject = slice_from(value(), 2)

  rule = choose(
    when_checksum(
      length_eq(subject(), 10),
      apply_checksum(checksum.is.kennitala, subject()),
    ),
    unsupported_checksum("checksum_not_published"),
  )
}
```

Ainsi, un VAT islandais de longueur documentée mais sans checksum public n’est
jamais marqué invalide par absence d’algorithme.

Primitives checksum V1 obligatoires :

- `luhn(expr)` ;
- `iso7064_mod97_10(expr)` ;
- `digits_to_integer(expr)` pour des tranches bornées ;
- `mod_digits(expr, modulus)` sans conversion en grand entier ;
- `weighted_sum(expr, weights, alignment, mapping)` ;
- `modulo(int_expr, modulus)` ;
- `complement(int_expr, modulus)` ;
- `remainder_map(int_expr, values)` ;
- `compare_digit(int_expr, string_expr, index)` ;
- `compare_slice(int_expr, string_expr, start, end)` ;
- `choose(branches...)` ;
- `when_checksum(predicate, checksum_rule)` ;
- `all_checks(checksum_rule...)` ;
- `any_check(checksum_rule...)` ;
- `unsupported_checksum(reason_code)`.
- `apply_checksum(checksum_reference, string_expr)`.

`weighted_sum` doit préciser :

- la séquence des poids ;
- `left`, `right` ou `cycle` pour l’alignement ;
- la transformation éventuelle des caractères ;
- la tranche d’entrée ;
- le traitement explicite des restes spéciaux.

Les algorithmes complexes peuvent être exprimés par combinaison. Une primitive
spécialisée n’est acceptée que si elle possède une spécification autonome, des
vecteurs positifs et négatifs, et une justification démontrant qu’une composition
des primitives existantes serait moins sûre.

### 6.11 Déclarations d’identifiants

La sélection d’une définition est volontairement séparée de sa canonicalisation.
Un dispatcher par `kind` effectue uniquement la normalisation sûre nécessaire au
routage ; la canonicalisation propre au format ne s’exécute qu’après sélection de
la définition. Cela évite toute dépendance circulaire entre pays et canonicalizer.

Exemple logique :

```hcl
dispatcher "vat" {
  aliases           = ["vat_id"]
  pre_canonicalizer = canonicalizer.dispatch.identifier

  country_aliases = {
    "UK" = "GB"
  }

  target {
    country_code                     = "GR"
    accepted_prefixes                = ["EL", "GR"]
    canonical_prefix                 = "EL"
    identifier                       = identifier.vat.GR
    allow_unprefixed_without_country = false
  }
}
```

Les étapes autorisées dans un `pre_canonicalizer` V1 sont limitées à
`trim_whitespace`, `remove_whitespace`, `uppercase_ascii` et `remove_chars` avec une
liste constante. Elles ne peuvent ni ajouter, ni remplacer, ni interpréter un
préfixe. Les transformations spécifiques (`replace_prefix`, ajout du préfixe
canonique, padding, etc.) appartiennent au canonicalizer de la définition.

Chaque dispatcher déclare :

- un `kind` canonique ASCII en minuscules et ses alias explicites ;
- un pré-canonicalizer de routage ;
- zéro ou plusieurs alias de code pays vers un code pays canonique ISO 3166-1
  alpha-2 en majuscules ;
- ses cibles, chacune liée à exactement une définition ;
- pour chaque cible, les préfixes acceptés et le préfixe canonique éventuel ;
- si la cible peut être choisie sans pays ni préfixe.

Les alias de kind, de pays et les préfixes sont trois espaces distincts. Le champ
générique `aliases` d’une définition n’existe pas. Les doublons après normalisation,
les alias contradictoires, deux cibles pour le même pays et deux correspondances de
la **même valeur de préfixe** vers des cibles différentes sont des erreurs de
compilation. Des préfixes différents de même longueur sont évidemment autorisés.

`trim ASCII` dans ce contexte retire uniquement `U+0009..U+000D` et `U+0020` aux
extrémités. Un kind canonique ou alias doit respecter
`[a-z][a-z0-9_-]{0,63}` après lowercase ASCII. Un pays doit respecter exactement
`[A-Z]{2}` après trim et uppercase ASCII. Un préfixe est une constante ASCII
alphanumérique non vide de 1 à 8 caractères ; sa comparaison est sensible à la casse
sur la valeur déjà uppercasée par le pré-canonicalizer lorsque le dispatcher le
prévoit.

L’algorithme de dispatch normatif est :

1. normaliser le kind par trim ASCII, lowercase ASCII et table d’alias ;
2. si le kind est inconnu, retourner `unsupported_kind` sans exécuter de programme ;
3. exécuter une fois le pré-canonicalizer sur la valeur brute ;
4. normaliser le pays explicite par uppercase ASCII et table d’alias ; un pays
   syntaxiquement invalide retourne `unsupported_country` ; pour un dispatcher
   country-specific, un pays sans cible retourne aussi `unsupported_country` ;
5. chercher le plus long `accepted_prefix` exact ; une même valeur ne peut appartenir
   à deux cibles et les préfixes qui se recouvrent sont départagés par la longueur ;
6. si pays explicite et préfixe désignent deux cibles différentes, retourner
   `country_mismatch` ;
7. sélectionner, par priorité, la cible confirmée par pays, puis celle du préfixe,
   puis la cible `GLOBAL`, puis l’unique cible
   `allow_unprefixed_without_country = true` ;
8. si aucune cible n’est sélectionnable, retourner `missing_country_code` ;
9. exécuter le canonicalizer spécifique de la définition sélectionnée sur la valeur
   pré-canonique, exactement une fois.

L’ordre des étapes 3 et 4 est observable : la pré-canonicalisation précède la décision
de pays, donc un pays inutilisable est rapporté avec la valeur déjà pré-canonique. Ce
document et `engine.md` les donnaient dans l’ordre inverse de `ir.md` section 5, qui
fait foi ; aucun cas du corpus ne distinguait les deux.

Un dispatcher `GLOBAL` possède exactement cette unique cible et ne peut pas mélanger
cible globale et cibles pays. Sa cible n’a aucun préfixe ; un contexte pays bien
formé est ignoré pour le routage mais conservé, normalisé, dans le résultat. Pour une
cible pays, la valeur canonique du pays est le code ISO déclaré par la cible, même
lorsque son préfixe métier diffère (par exemple pays `GR`, préfixe VAT canonique
`EL`).

```hcl
identifier "vat" "BE" {
  canonicalizer = canonicalizer.vat.common
  format        = format.vat.be
  checksum      = checksum.vat.be

  default_profile = "compatible"

  source {
    id          = "be-tax-authority-vat"
    url         = "https://..."
    authority   = "Belgian tax authority"
    accessed_at = "2026-08-18"
  }
}
```

Champs obligatoires :

- `kind` via le premier label ;
- pays ou `GLOBAL` via le second label ;
- canonicalizer ;
- format ;
- checksum ou déclaration explicite d’absence ;
- au moins une source pour toute règle qui rejette une entrée ;
- profil par défaut.

Toute définition DOIT être référencée par exactement une cible de dispatcher. Toute
cible DOIT référencer une définition du même kind et du même pays. Le linker rejette
les définitions orphelines et les incohérences.

Le profil `compatible` est le défaut normatif. Il accepte les variantes actuelles et
les variantes historiques documentées qui peuvent encore légitimement apparaître au
sens de 2.6.
`strict_current` est opt-in et ne doit pas modifier la canonicalisation commune.

Les dates `valid_from` et `valid_until` sont des métadonnées en V1. Elles ne causent
pas de rejet automatique sauf si l’appelant demande explicitement une validation
temporelle dans une future version de format.

### 6.12 Provenance

Toute règle de rejet DOIT être reliée à une source. Une source contient :

```text
id, url, authority, title, accessed_at, jurisdiction,
language, notes, license_or_terms, archive_url optionnel
```

Les sources primaires officielles sont préférées. Une implémentation tierce comme
`python-stdnum` peut servir de contre-vérification, jamais d’unique autorité lorsque
la documentation officielle existe.

Les changements de règle doivent expliquer :

- l’ancienne sémantique ;
- la nouvelle sémantique ;
- le risque de faux positif et de faux négatif ;
- les nouveaux cas de conformité ;
- la date et la source du changement.

## 7. IR Protobuf

### 7.1 Règles générales

Utiliser `proto3` pour la V1. Ne pas utiliser `google.protobuf.Any`, les extensions,
les champs `required`, ni des `map` lorsque l’ordre est significatif. Les listes
doivent être triées par le compilateur lorsque l’ordre métier ne l’est pas.

Chaque enum réserve la valeur zéro à `UNSPECIFIED`, que les générateurs rejettent
lors de la validation du bundle.

Tout champ Protobuf inconnu est rejeté en V1, à n’importe quelle profondeur. Un
générateur configure son décodeur pour conserver/rapporter les unknown fields ou
effectue un pré-scan wire borné contre les descripteurs avant le décodage métier. Les
ignorer en se reposant uniquement sur `required_feature_ids` n’est pas sûr face à un
bundle forgé qui omettrait volontairement la capability correspondante.

Les champs supprimés doivent avoir leur numéro et leur nom placés dans `reserved`.
Un numéro de champ ne doit jamais être réutilisé.

Ordre de sérialisation V1 : feature IDs numériques croissants ; dispatchers par
octets UTF-8 de kind ; aliases et préfixes par octets UTF-8 ; cibles avec `GLOBAL`
d’abord puis code pays ; définitions par kind puis `GLOBAL`/code pays ; programmes
par ID ; sources par `id`. Les assertions, branches, étapes de canonicalisation,
poids et nœuds topologiques gardent leur ordre sémantique et ne sont jamais triés.
Deux clés de tri égales constituent un doublon rejeté, pas un départage dépendant de
l’ordre d’entrée.

Les IDs sont attribués à partir de 1 après tri des noms symboliques pleinement
qualifiés en UTF-8 ; les sous-programmes synthétiques utilisent un nom stable composé
du symbole parent, du champ AST et de l’index source. Les nœuds sont émis par parcours
post-order déterministe des opérandes dans l’ordre syntaxique, avec déduplication
optionnelle fondée sur une clé structurelle documentée. Aucune itération de map Go ne
peut influencer un ID ou un ordre binaire.

### 7.2 En-tête du bundle

Le message racine doit au minimum contenir :

```proto
message RuleBundle {
  uint32 format_version = 1;
  string rules_version = 2;
  repeated uint32 required_feature_ids = 3;
  bytes source_digest = 4;
  reserved 5;
  reserved "generated_at";
  repeated IdentifierDefinition identifiers = 6;
  repeated Program programs = 7;
  repeated IdentifierDispatcher dispatchers = 8;
}
```

Le bundle ne contient aucun timestamp. `generatedAt` appartient uniquement au
manifeste externe ; il est dérivé de `SOURCE_DATE_EPOCH` en build reproductible.

`source_digest` est le SHA-256 d’un flux canonique. Pour chaque fichier source, le
compilateur refuse les symlinks, calcule son chemin relatif à la racine, remplace les
séparateurs par `/`, exige UTF-8 sans BOM, normalise CRLF et CR en LF sans retirer
commentaires ni espaces, puis trie les entrées par ordre lexicographique des octets
UTF-8 du chemin. Le flux hashé commence par les octets ASCII
`ENTID-SOURCE-V1\n`, puis, pour chaque fichier : longueur du chemin sur
8 octets non signés big-endian, chemin UTF-8, longueur du contenu normalisé sur
8 octets non signés big-endian, contenu. Aucun séparateur implicite n’est ajouté.
Le digest des règles couvre tous les `rules/**/*.hcl`. Celui de conformité utilise
le domaine `ENTID-CONFORMANCE-SOURCE-V1\n` et couvre tous les JSONL ainsi
que les fixtures incorporées, avec le même encodage.

Les chemins du flux sont virtuels et indépendants du checkout : une source de règle
est `rules/` + son chemin relatif à l’argument `--rules`, un JSONL est
`conformance/` + son chemin relatif à `--cases`, et une fixture est `fixtures/` + son
chemin relatif à `testdata/`. Les segments `.`/`..`, chemins absolus, collisions
après normalisation et noms non UTF-8 sont refusés.

Les champs digest font exactement 32 octets. Toute autre longueur est invalide ; le
manifeste encode ces valeurs en hexadécimal minuscule de 64 caractères.

Le hash SHA-256 exact du `.binpb` est stocké dans le manifeste externe et dans
`SHA256SUMS`. Protobuf n’étant pas une sérialisation canonique, un moteur ne doit pas
resérialiser le message pour vérifier ce hash.

Le compilateur Go utilise `proto.MarshalOptions{Deterministic: true}` avec version de
runtime verrouillée, émet les champs répétés dans l’ordre normatif documenté et
compare toujours les octets originaux. Cette option garantit la reproductibilité du
compilateur verrouillé, pas une canonicalisation universelle entre runtimes
Protobuf ; c’est pourquoi les générateurs vérifient le hash du fichier avant
décodage et ne le recalculent jamais depuis l’objet décodé.

### 7.3 Programmes et nœuds

L’IR doit être un graphe acyclique typé, sérialisé dans un ordre topologique. Chaque
nœud ne peut référencer que des nœuds d’indice inférieur. Les racines de programmes
référencent leurs nœuds finaux.

Le schéma doit séparer au minimum :

- opérations de chaîne ;
- opérations entières ;
- prédicats ;
- assertions ;
- opérations de canonicalisation ;
- résultats checksum tri-état ;
- définitions d’identifiants.

Un `oneof operation` porte l’opération d’un nœud. Après décodage, l’absence
d’opération est une erreur `invalid_ruleset`.

Le schéma V1 doit suivre ce squelette logique. Les sous-messages d’opérations ont des
champs explicites ; ils ne transportent jamais une expression HCL ou une map libre.

```proto
message IdentifierDefinition {
  uint32 id = 1;
  string kind = 2;
  optional string country_code = 3;
  uint32 canonicalization_program = 4;
  uint32 format_program = 5;
  optional uint32 checksum_program = 6;
  string default_profile = 7;
  repeated Source sources = 8;
}

message IdentifierDispatcher {
  string kind = 1;
  repeated string kind_aliases = 2;
  uint32 pre_canonicalization_program = 3;
  repeated CountryAlias country_aliases = 4;
  repeated DispatchTarget targets = 5;
}

message CountryAlias {
  string alias = 1;
  string country_code = 2;
}

message DispatchTarget {
  optional string country_code = 1;
  repeated string accepted_prefixes = 2;
  optional string canonical_prefix = 3;
  uint32 identifier_definition_id = 4;
  bool allow_unprefixed_without_country = 5;
}

message Program {
  uint32 id = 1;
  ProgramKind kind = 2;
  repeated Node nodes = 3;
  uint32 root_node = 4;
}

message Node {
  ValueType output_type = 1;
  repeated uint32 input_nodes = 2;

  oneof operation {
    StringOperation string_operation = 10;
    IntegerOperation integer_operation = 11;
    PredicateOperation predicate_operation = 12;
    CanonicalizationOperation canonicalization_operation = 13;
    AssertionOperation assertion_operation = 14;
    ChecksumOperation checksum_operation = 15;
    CallOperation call_operation = 16;
  }
}
```

`IdentifierDefinition.id` et `Program.id` sont uniques dans leur espace. Les
références de programmes sont résolues par ID numérique. `Node.input_nodes` contient
des indices locaux strictement
inférieurs à l’indice du nœud. `root_node` doit être valide. Un programme checksum
retourne un `ChecksumOutcome`, pas un booléen.

L’absence de `country_code` dans une définition et sa cible représente `GLOBAL` ;
une chaîne vide ou la chaîne littérale `"GLOBAL"` dans l’IR est invalide. Dans HCL,
le label `GLOBAL` est abaissé vers cette absence. Une cible globale n’a ni
`accepted_prefixes`, ni `canonical_prefix`, ni alias pays et doit être l’unique cible
de son dispatcher.

Le graphe d’appels entre programmes doit lui aussi être acyclique. Le compilateur et
chaque moteur construisent le graphe des `CallOperation`, rejettent toute composante
cyclique (y compris l’auto-appel), vérifient les kinds entrée/sortie à chaque arête et
calculent statiquement une profondeur maximale ≤ 32. L’ordre topologique des nœuds
locaux ne suffit pas à satisfaire cette exigence.

Le compilateur PEUT dédupliquer des sous-graphes uniquement si cela ne modifie ni
l’ordre des assertions ni les `message_key`. L’optimisation doit être désactivable
pour faciliter le debug et couverte par un test d’équivalence.

### 7.4 Version et fonctionnalités

`format_version` versionne la structure et les invariants de l’IR.
`rules_version` versionne les données métier et suit `YYYY.MM.PATCH`. `YYYY.MM` est
l'année et le mois où les données ont été coupées ; `PATCH` est un compteur dans ce
mois, **sans borne supérieure**. `2026.08.31` est donc suivi de `2026.08.32`, pas de
`2026.09.0` : le troisième champ n'est pas un jour, et quatre versions ont annoncé
septembre en plein mois d'août pour l'avoir pris pour un.

`tools/next_rules_version.sh` la calcule depuis toutes les valeurs que le fichier a
portées, donc il ne peut ni basculer de mois par erreur ni réutiliser une version
qu'un moteur a déjà épinglée. Un test refuse par ailleurs une version nommant un mois
qui n'a pas commencé, jugé sur la date du commit et jamais sur l'horloge.

`required_feature_ids` énumère toutes les primitives nécessaires. Chaque moteur
publie la liste des fonctionnalités supportées. Le chargement échoue si un seul ID
est inconnu.

Le registre des capabilities est publié par `docs/features.md`, généré depuis le
registre Go unique. Il n’est pas recopié ici : une table tenue à la main dérive, et
celle qui occupait cette place s’était arrêtée à quatorze IDs alors que le bundle en
portait dix-huit.

Chaque ID désigne l’ensemble exact et figé d’opérations, champs, bornes et sémantiques
documenté dans `docs/features.md`. Cet ensemble ne peut jamais être élargi ou
réinterprété. Les IDs ne sont ni renumérotés ni réutilisés. Une nouvelle opération,
une nouvelle variante d’opération ou une modification observable reçoit toujours un
nouvel ID, même si elle est conceptuellement apparentée à une capability existante.
Les opérations concrètes et leur ID sont générés depuis un registre Go unique vers
`docs/ir.md` et `docs/features.md` ; la CI refuse toute divergence.

Une évolution additive de Protobuf n’est pas automatiquement une évolution sûre du
langage. Toute nouvelle opération nécessite :

1. réservation d’un feature ID documenté ;
2. implémentation dans les quatre moteurs ;
3. cas de conformité ;
4. publication des moteurs compatibles ;
5. seulement ensuite, utilisation par une règle officielle.

### 7.5 Limites structurelles du format V1

Les compilateurs et moteurs appliquent au minimum :

```text
bundle binaire maximum          16 MiB
identifiants maximum            10 000
nœuds totaux maximum            500 000
nœuds par programme maximum     4 096
profondeur d’appel maximum       32
chaîne constante maximum        4 096 octets UTF-8
entrée utilisateur maximum      1 024 octets UTF-8
étapes par validation maximum   100 000
captures par format maximum     128
```

Ces limites sont normatives et incluses dans les tests de conformité de sécurité.

Elles ne s’appliquent pas toutes au même moment. Les limites portant sur la forme du
bundle — taille binaire, nombre d’identifiants, nœuds, profondeur d’appel, chaînes
constantes — sont vérifiées par le générateur et n’ont plus d’objet une fois le code
produit. La limite d’entrée utilisateur reste une obligation du moteur : elle est
observable, et une entrée plus longue DOIT être refusée sans être traitée.

Le budget d’étapes borne une interprétation. Un moteur généré n’en a pas besoin, car
l’acyclicité du graphe et la borne de profondeur d’appel sont établies à la
génération : le code produit termine par construction. Un moteur qui interprète le
bundle applique le budget tel quel.

La borne d’entrée de 1 024 octets est délibérément petite. Elle rend possible une
implémentation sans allocation, sur un tampon de taille fixe, ce que les moteurs
DEVRAIENT viser.

Limites arithmétiques V1 supplémentaires :

```text
constante entière                   int64 signé
modulus / complément                2..1 000 000 000
valeur absolue d’un poids           0..1 000 000
poids par opération                 1..256
éléments d’une remainder_map        1..1 000 000
index / borne de tranche            0..4 096
opérandes de concat                 1..256
```

Toutes les additions, multiplications, négations et conversions sont vérifiées. Le
compilateur rejette tout overflow prouvable et calcule des bornes conservatrices
pour chaque expression entière. Le générateur répète ces validations avant de
produire du code ; si la sûreté d’un calcul ne peut pas être prouvée dans les bornes
V1, le bundle est `invalid_ruleset` et rien n’est généré. Aucun wrap, saturation ou
promotion silencieuse propre au langage n’est autorisé.

La borne `int64` est un maximum théorique que tout langage cible ne représente pas
exactement. Un langage dont le type entier natif est un flottant double précision —
JavaScript en particulier — n’est exact que jusqu’à 2^53. Un générateur ciblant un
tel langage DOIT soit émettre un type entier de précision arbitraire, soit refuser de
générer lorsqu’une expression peut dépasser 2^53. Il NE DOIT PAS émettre un calcul
silencieusement inexact. Aucune règle officielle n’excède 2^53 à ce jour ; cette
exigence porte sur les règles futures.

`digits_to_integer` n’est autorisé que si la longueur maximale et la borne de la vue
prouvent un résultat ≤ `INT64_MAX` ; une longueur non bornée ou une tranche pouvant
dépasser cette valeur est rejetée. Pour les identifiants plus longs, seule la famille
`mod_digits` chiffre par chiffre est conforme.

## 8. Cas de conformité

### 8.1 Deux formats, une seule source de vérité

Les cas sont écrits et révisés en JSONL. Le compilateur les valide puis produit
`conformance.binpb`.

- JSONL est la source humaine canonique ;
- BINPB est l’artefact typé consommé par les moteurs ;
- le BINPB ne doit jamais être modifié manuellement ;
- le JSONL et le BINPB doivent porter la même `rules_version`.

### 8.2 Interdiction des oracles circulaires

Le compilateur NE DOIT PAS calculer automatiquement le résultat attendu à partir de
la règle testée. Les attentes sont écrites ou approuvées explicitement à partir de
sources indépendantes.

Le compilateur peut générer des cas supplémentaires de robustesse ou mutation, mais
ils sont marqués `generated` et ne remplacent jamais les cas-oracles revus.

### 8.3 Confidentialité, licences et redistribution

`DATA_POLICY.md` est normatif pour tous les cas et fixtures. Il interdit les données
issues de production, les identifiants transmis par des utilisateurs et toute donnée
personnelle collectée dans un registre, un incident ou une télémétrie. Un identifiant
d’entreprise peut aussi identifier une personne physique ; il est donc traité comme
potentiellement personnel par défaut.

Seuls sont acceptés :

- les exemples explicitement publiés par une autorité, avec droit de
  redistribution vérifié ;
- les valeurs entièrement synthétiques, dérivées d’un générateur documenté et ne
  provenant d’aucun jeu réel ;
- les mutations synthétiques de ces deux catégories.

Pour un format susceptible d’identifier une personne physique, un cas positif ne
peut être synthétique que dans une plage officiellement réservée aux tests ou avec
une preuve documentée de non-attribution ; sinon seuls les exemples de test
volontairement publiés par l’autorité sont acceptés. Les cas négatifs synthétiques
doivent être manifestement non attribuables lorsque possible. Le projet ne consulte
pas un registre de personnes pour « vérifier » une valeur aléatoire, car cette
vérification créerait elle-même un traitement de données injustifié.

Chaque cas déclare `dataClassification` parmi `official_public_example` et
`synthetic`, ainsi qu’un `redistributionBasis` non vide. Un exemple officiel conserve
son URL, sa date d’accès et les termes applicables. Une PR de données déclenche une
revue confidentialité/licence et un contrôle automatique contre les secrets et
motifs interdits. `DATA_POLICY.md` décrit aussi la procédure de signalement et de
retrait urgent d’un cas publié par erreur. Une suppression pour confidentialité peut
être immédiate et déroge à la conservation habituelle des golden files.

### 8.4 Schéma logique JSONL

Chaque ligne est un objet autonome :

```json
{
  "id": "vat-be-valid-official-001",
  "description": "Official Belgian VAT example",
  "kind": "vat",
  "countryCode": "BE",
  "input": "BE 0123.456.749",
  "profile": "compatible",
  "operation": "validate",
  "expected": {
    "canonicalValue": "BE0123456749",
    "format": {"status": "valid", "reasonCode": "ok"},
    "checksum": {"status": "valid", "reasonCode": "ok"}
  },
  "tags": ["official", "valid", "normalization"],
  "sourceIds": ["be-tax-authority-vat"],
  "dataClassification": "official_public_example",
  "redistributionBasis": "Example intentionally published by the authority"
}
```

Champs toujours obligatoires : `id`, `operation`, `tags`, `dataClassification`,
`redistributionBasis`. Les quatre opérations métier exigent en plus `kind`, `input`,
`profile` et `expected`. `load_ruleset` interdit ces quatre champs et exige `fixture`
et `expectedEngineError`. Un cas positif officiel doit référencer au moins une
source. L’expression « cas de production » est interdite : aucune donnée de
production ne peut entrer dans le corpus.

Opérations supportées :

- `canonicalize` ;
- `validate_format` ;
- `validate_checksum` ;
- `validate` ;
- `load_ruleset` pour les cas de sécurité du décodeur.

Pour rendre ce document autonome, le contrat de résultat minimal est répété ici.
Les statuts sont `valid`, `invalid`, `unsupported`, `not_run`. Les niveaux sont
`format`, `checksum`, `registry`. Le registre V1 des raisons est :

```text
ok
empty
invalid_length
invalid_characters
invalid_format
invalid_checksum
missing_country_code
country_mismatch
unsupported_kind
unsupported_country
unsupported_format
unsupported_checksum
checksum_not_published
not_requested
not_run_format_invalid
not_run_format_unsupported
registry_not_configured
incompatible_ruleset
invalid_ruleset
input_too_long
invalid_encoding
```

`valid` exige `ok`. `not_run` exige `not_requested`,
`not_run_format_invalid` ou `not_run_format_unsupported`. `invalid` exige une preuve
parmi `empty`, `invalid_length`, `invalid_characters`, `invalid_format`,
`invalid_checksum`, `country_mismatch`; les autres raisons métier sont
`unsupported`. Les échecs antérieurs à une assertion ont une `message_key` absente.

Toutes les opérations commencent par la limite d’entrée et le dispatch de 6.11.
`canonicalize` retourne kind, entrée brute, valeur canonique, pays, profil, versions,
statut, raison et message key, sans format ni checksum. `validate_format` retourne un
rapport format+checksum ; après un format valide, checksum vaut obligatoirement
`not_run`/`not_requested`. `validate_checksum` et `validate` retournent le même
rapport : format sert de garde, un format invalide donne checksum
`not_run`/`not_run_format_invalid`, un format unsupported donne
`not_run`/`not_run_format_unsupported`, sinon le checksum est exécuté ou vaut
`unsupported` si aucun algorithme applicable n’est publié.

Dans tous les résultats, l’entrée brute est inchangée. Le kind est canonique après
résolution d’un dispatcher, sinon le token demandé après trim/lowercase ASCII. Avant
sélection d’une définition, la valeur canonique est la valeur pré-canonique et le
pays est le contexte normalisé ; avant même résolution du dispatcher ou sur entrée
trop longue, la valeur canonique est l’entrée brute et le pays est le contexte brut.
Après sélection, valeur et pays viennent de la définition/cible ; une cible globale
conserve toutefois le contexte pays normalisé sans l’utiliser. Les quatre moteurs et
l’interpréteur de référence appliquent exactement ces règles.

Le schéma Protobuf de conformité suit au minimum :

```proto
message ConformanceBundle {
  uint32 format_version = 1;
  string rules_version = 2;
  bytes source_digest = 3;
  repeated ConformanceCase cases = 4;
}

message ConformanceCase {
  string id = 1;
  string description = 2;
  string kind = 3;
  optional string country_code = 4;
  string input = 5;
  string profile = 6;
  Operation operation = 7;
  ExpectedOutcome expected = 8;
  repeated string tags = 9;
  repeated string source_ids = 10;
  optional bytes rules_payload = 11;
  optional string expected_engine_error = 12;
  bool generated = 13;
  string data_classification = 14;
  string redistribution_basis = 15;
}

message ExpectedOutcome {
  oneof value {
    ExpectedCanonicalization canonicalization = 1;
    ExpectedValidationReport validation_report = 2;
  }
}

message ExpectedCanonicalization {
  string kind = 1;
  string input_value = 2;
  string canonical_value = 3;
  optional string country_code = 4;
  string profile = 5;
  string rules_version = 6;
  uint32 format_version = 7;
  StepStatus status = 8;
  ReasonCode reason_code = 9;
  optional string message_key = 10;
}

message ExpectedValidationReport {
  string kind = 1;
  string input_value = 2;
  string canonical_value = 3;
  optional string country_code = 4;
  string profile = 5;
  string rules_version = 6;
  uint32 format_version = 7;
  ExpectedStep format = 8;
  ExpectedStep checksum = 9;
}

message ExpectedStep {
  ValidationLevel level = 1;
  StepStatus status = 2;
  ReasonCode reason_code = 3;
  optional string message_key = 4;
}
```

`rules_payload` est utilisé uniquement par `load_ruleset`. Dans le JSONL source, il
provient d’un chemin `fixture` sous `testdata/`; le compilateur lit et incorpore les
octets. Les chemins absolus et sorties du dépôt sont interdits.
`ExpectedCanonicalization` et `ExpectedValidationReport` reproduisent exactement les
contrats ci-dessus, hors `engineVersion`, et doivent rester synchronisés avec
`engine.md`. `canonicalize` exige la première variante ; `validate_format`,
`validate_checksum` et `validate` exigent la seconde. `load_ruleset` n’a pas de
`ExpectedOutcome` et exige `expected_engine_error`. Toute autre combinaison est une
erreur de compilation.

Le JSONL reste concis : le compilateur copie mécaniquement dans l’attente Protobuf
`kind`, `input`, `profile`, le pays attendu explicitement déclaré dans `expected`,
ainsi que `rules_version`/`format_version` du bundle. Il ne peut déduire que ces
métadonnées identiques à l’entrée. `canonicalValue`, pays résultant, statuts, raisons
et message keys sont toujours écrits explicitement par l’auteur ; ils ne sont jamais
calculés par l’interpréteur. Pour un kind alias, `expected.kind` est donc obligatoire.

Limites de la conformité V1 : bundle ≤ 64 MiB, un million de cas maximum, ID unique,
description ≤ 4 096 octets, tags triés et uniques. Les moteurs peuvent streamer ou
indexer les cas, mais doivent tous les exécuter.

### 8.5 Matrice minimale par règle

Chaque variante doit posséder :

- au moins deux exemples valides indépendants si disponibles ;
- longueur trop courte et trop longue ;
- caractères invalides à chaque classe de position ;
- checksum valide ;
- mutations de chaque chiffre de contrôle ;
- séparateurs et casse acceptés par canonicalisation ;
- valeur vide ;
- pays manquant et pays contradictoire lorsque pertinent ;
- profil compatible et strict lorsque différents ;
- branche `unsupported` lorsqu’un checksum n’est pas publié ;
- limites exactes de toutes les plages ;
- chaque branche d’un algorithme multi-variante.

Tout bug de faux négatif ou faux positif signalé doit produire un cas de régression
avant correction.

### 8.6 Interpréteur de référence

Le dépôt contient un interpréteur de référence en Go, interne au compilateur. Son
objectif est de vérifier l’IR et les cas, pas de servir de bibliothèque publique.

Il doit privilégier lisibilité, validations défensives et exactitude. Le moteur Go de
production reste une implémentation indépendante et idiomatique. Cette indépendance
réduit les risques de reproduire le même défaut.

L’interpréteur de référence vérifie les attentes écrites ; il ne les génère pas.

Aucun moteur NE DOIT être dérivé, porté ou transcrit depuis cet interpréteur. Un
moteur qui en descendrait passerait la conformité par construction et ne prouverait
ni que l’IR est implémentable, ni que la documentation des opcodes suffit à
l’implémenter. C’est précisément ce que chaque nouveau moteur doit établir.

### 8.7 Protocole de conformité

La conformité NE DOIT PAS être vérifiée par une suite de tests réécrite dans chaque
moteur. Un moteur qui interprète lui-même les résultats attendus peut se déclarer
conforme à tort, en comparant trop faiblement ou en ignorant un champ absent.

Ce dépôt publie donc un **runner de conformité** : le seul programme qui lit les
résultats attendus et décide de la conformité. Un moteur ne voit jamais un résultat
attendu.

Le moteur fournit en regard un exécutable minimal, le **testee**, qui lit des
requêtes sur son entrée standard, appelle son API publique et écrit des réponses sur
sa sortie standard. Le protocole est un cadrage binaire simple : pour chaque message,
un entier non signé de 32 bits en petit-boutiste donnant la longueur, suivi du
message Protobuf sérialisé. Les schémas de requête et de réponse sont normatifs et
publiés avec les autres schémas du dépôt.

Le testee NE DOIT PAS lire le corpus, ni interpréter un résultat attendu, ni adapter
son comportement au cas reçu. Il traduit une requête en appel d’API et un résultat en
réponse, rien de plus.

Trois propriétés découlent de ce découpage. La logique de comparaison n’existe qu’une
fois, donc une divergence d’interprétation des attentes est impossible. Le protocole
est agnostique au langage, donc une implémentation tierce, dans un langage que ce
projet ne publie pas, se déclare conforme par les mêmes moyens que les moteurs
officiels. Et le testee reste assez petit pour être relu intégralement, ce qui rend
vérifiable l’absence de triche.

Un moteur est conforme lorsque le runner rapporte zéro écart sur la totalité du
corpus. Une exécution partielle, une catégorie ignorée ou un cas déclaré non
applicable NE constituent PAS une conformité.

#### Obtenir le runner

Le runner s'exécute depuis ce dépôt, épinglé au commit que `rules.lock` enregistre
sous `source_commit` — le même commit que le corpus, ce qui rend impossible de
juger un corpus avec le comparateur d'un autre :

```bash
go run github.com/entid-org/spec/cmd/conformance-runner@<source_commit> \
  -corpus spec/entid-conformance.binpb -- ./mon-testee
```

Aucune release n'est nécessaire et rien n'est à télécharger à la main. Le seul
prérequis est une toolchain Go dans la CI du moteur : c'est un outil de
construction, il n'entre ni dans le paquet publié ni dans ses dépendances.

`actions/setup-go` pose `GOTOOLCHAIN: local`, ce qui interdit de récupérer une
toolchain. L'étape du runner doit donc poser `GOTOOLCHAIN: auto` ; le testee, lui,
reste construit avec la toolchain épinglée du moteur.

La condition exacte est la ligne `go` du module `spec`, pas sa ligne `toolchain` :
c'est la première qui lie. Un moteur épinglant un Go antérieur échoue sur la
résolution de la toolchain, pas sur un écart de conformité — c'est arrivé au moteur
Go, épinglé plus bas. Un moteur qui résout `stable` ne rencontre jamais le cas, et le
moteur TypeScript l'a mesuré ainsi. Posez `GOTOOLCHAIN: auto` quand même : c'est une
assurance qui ne coûte rien, et la ligne `go` de `spec` montera un jour.

Un moteur NE DOIT PAS écrire son propre runner. Deux moteurs l'ont fait faute de
savoir que celui-ci était accessible, et cela vide de son sens la propriété
énoncée plus haut : leur verdict de conformité était rendu par leur propre
comparateur. Ce qu'un moteur écrit, et qu'il doit garder, ce sont les tests qui
prouvent que son testee ne triche pas — qu'il ne lit pas le corpus, n'interprète
aucun attendu et n'adapte pas son comportement au cas reçu.

### 8.8 Cas de chargement et cas d’exécution

Les cas dont l’opération est `load_ruleset` éprouvent le refus d’un bundle hostile ou
malformé. Ils s’adressent au **générateur**, qui doit refuser de produire du code,
et non au moteur généré, lequel ne charge aucun bundle.

Les autres cas s’adressent au moteur, à travers le protocole de 8.7.

Un moteur qui interprète le bundle à l’exécution répond aux deux catégories. Un
moteur généré répond aux cas d’exécution, et son générateur répond aux cas de
chargement. Dans les deux cas, la totalité du corpus est couverte : aucun cas n’est
sans destinataire.

## 9. CLI `entidc`

La CLI doit proposer :

```text
entidc fmt [paths...]
entidc lint [paths...]
entidc compile --rules rules --cases conformance --out dist
entidc verify --rules rules --cases conformance
entidc inspect dist/entid-rules.binpb
entidc diff old.binpb new.binpb
entidc check-generated
entidc version
```

### 9.1 Exigences CLI

- diagnostics avec fichier, ligne, colonne, code stable et suggestion ;
- plusieurs erreurs collectées par exécution lorsque sûr ;
- sortie humaine sur stderr, sortie machine JSON optionnelle ;
- codes de sortie documentés ;
- chemins et ordre de fichiers normalisés ;
- aucune donnée temporelle dans les BINPB ; en mode release et `check-generated`,
  `SOURCE_DATE_EPOCH` est obligatoire et `generatedAt` du manifeste en est la
  représentation UTC RFC 3339 ; une compilation locale non reproductible peut
  utiliser l’heure UTC uniquement si le manifeste est marqué `reproducible: false` ;
- `compile` écrit de manière atomique ;
- aucun artefact partiel après échec ;
- `diff` classe les changements en élargissement, restriction, métadonnée,
  fonctionnalité IR et incompatibilité potentielle.

`check-generated` reconstruit dans un répertoire temporaire et compare les octets
publiés, manifestes et documents générés.

## 10. Artefacts de release

Une release des règles publie :

```text
entid-rules-YYYY.MM.PATCH.binpb
entid-conformance-YYYY.MM.PATCH.binpb
entid-conformance-YYYY.MM.PATCH.jsonl.gz
entid-manifest-YYYY.MM.PATCH.json
rules.proto
conformance.proto
ir.md
features.md
SHA256SUMS
SBOM.spdx.json
provenance.intoto.jsonl
```

Le `.jsonl.gz` est généré, jamais concaténé naïvement : cas triés par `id`, clés JSON
dans l’ordre du schéma, encodage UTF-8, une ligne LF par cas, aucune espace non
significative. Gzip utilise un niveau verrouillé, aucun nom/commentaire, `mtime =
SOURCE_DATE_EPOCH` et un champ OS fixé à 255. Ses octets sont inclus dans
`SHA256SUMS` et l’attestation.

Le manifeste contient au minimum :

```text
rulesVersion, formatVersion, requiredFeatures, sourceDigest,
artifactSha256, conformanceSha256, compilerVersion,
sourceCommit, generatedAt, identifierCount, countryCount,
coverageByKind, minimumEngineCapabilities, rulesProtoSha256,
conformanceProtoSha256, irDocSha256, featuresDocSha256, reproducible
```

Une release publique exige `reproducible: true`. Le workflow fixe
`SOURCE_DATE_EPOCH` au timestamp du commit source et reconstruit deux fois dans des
répertoires temporaires distincts avant publication.

La release utilise une identité OIDC de workflow GitHub Actions et produit une
attestation Sigstore/GitHub Artifact Attestation liée au dépôt
`entid-org/spec`, au commit, au tag protégé et au workflow de release attendu.
Le job de build n’a que `contents: read` ; un job distinct, protégé par environnement,
reçoit `id-token: write`, `attestations: write` et la permission minimale de publier.
Les tags de release sont immuables et protégés ; aucun secret longue durée ne signe
les artefacts.

Les dépôts moteurs vérifient **à la fois** SHA-256 et attestation (propriétaire,
dépôt, workflow, commit et tag), avant d’accepter une mise à jour. `SECURITY.md`
définit la révocation : advisory, retrait de release si nécessaire, ajout du digest
dans une liste de révocation signée et PR downstream de rollback. Si les moteurs
téléchargent un jour des règles dynamiquement, une politique de confiance runtime et
une signature applicative dédiées devront être spécifiées avant activation ; elles
sont hors périmètre V1.

## 11. Synchronisation des dépôts moteurs

Après publication d’une release, **le moteur va la chercher ; la release ne
pousse rien**. Chaque dépôt moteur porte un workflow `rules-sync` déclenché par
une horloge, et non par un événement émis depuis `spec` : un `repository_dispatch`
rendrait d’une main le jeton inter-dépôts que cette inversion retire de l’autre.
`spec` n’a donc aucun droit d’écriture sur les quatre moteurs, et le rayon
d’impact d’une compromission de `spec` s’arrête à `spec`. Le prix est une journée
de latence au pire ; le déclenchement manuel existe pour le jour où elle coûte
trop cher.

1. le workflow `rules-sync` de `entid-go`, `entid-swift`, `entid-kotlin` et
   `entid-typescript` découvre la nouvelle release et ouvre une PR chez lui ;
2. la PR met à jour `rules.lock` et le code généré ;
3. le générateur du moteur récupère les artefacts de la release désignée, vérifie
   tous les SHA-256 et l’attestation, puis régénère le code — la régénération
   demande la chaîne d’outils du moteur, que `spec` n’a pas et n’aura pas ;
4. le code généré est committé ; les artefacts binaires ne le sont pas ;
5. chaque CI exécute toute la conformité et ses tests propres ;
6. aucune publication de package n’est automatique avant succès de ses quality gates.

La PR ne transporte pas le bundle. Un dépôt moteur qui committerait un `.binpb`
présenterait à ses relecteurs un objet opaque à la place du changement réel ; le code
généré, lui, se lit en diff et montre exactement quelle règle a changé. Le bundle
reste l’artefact attesté et fait toujours autorité — il est vérifié à la génération,
pas archivé.

Le code généré DOIT être committé. Cette obligation garantit qu’une construction du
moteur, chez un consommateur comme en intégration continue, n’exige ni réseau ni
accès au dépôt `spec`, conformément à 2.5. La régénération est une opération
volontaire de mainteneur, jamais une étape de build.

Un moteur N’EMBARQUE PAS le bundle et ne l’interprète pas : `engine.md` section 1.2
l’interdit, et cette phrase disait le contraire. Elle a survécu à quatre audits parce
que les gardes mécaniques lisaient `engine.md` et les contrats par langage, jamais ce
document-ci. Le bundle est une entrée du générateur, vérifiée par digest et par
attestation à la construction, et rien de lui ne voyage dans le paquet publié.

Format de `rules.lock` :

```text
rules_version = "2026.08.0"
format_version = 1
rules_sha256 = "..."
conformance_sha256 = "..."
conformance_jsonl_sha256 = "..."
rules_proto_sha256 = "..."
conformance_proto_sha256 = "..."
testee_proto_sha256 = "..."
ir_doc_sha256 = "..."
features_doc_sha256 = "..."
source_commit = "..."
attestation_identity = "entid-org/spec/.github/workflows/release.yml@refs/tags/..."
```

Le lock est généré depuis le manifeste attesté ; l’automatisation ne peut pas
auto-approuver ni fusionner sa propre PR. Lorsqu’une nouvelle feature IR est
nécessaire, les moteurs doivent la publier avant que les règles officielles ne
l’utilisent.

`rules.lock` est ainsi le seul point de couplage entre ce dépôt et un moteur. Il
désigne une release, en atteste le contenu, et ne dit rien du langage ni de la
stratégie d’implémentation.

## 12. Qualité, tests et sécurité du dépôt `spec`

### 12.1 Développement

- TDD obligatoire pour parser, linker, typechecker, lowering et opérations IR ;
- tests unitaires table-driven ;
- tests d’intégration CLI ;
- golden tests lisibles pour diagnostics et artefacts ;
- fuzzing des parseurs HCL, JSONL et Protobuf ;
- property tests pour idempotence et invariants ;
- tests de limites et entrées malveillantes ;
- `go test -race ./...` ;
- couverture de lignes minimale 95 % ;
- couverture de branches minimale 90 % ;
- 100 % des opérations IR et chemins de refus de bundle couverts ;
- mutation testing périodique sur les algorithmes critiques.

La couverture seule n’est pas une preuve. Les assertions doivent vérifier le
comportement, pas uniquement exécuter les lignes.

### 12.2 Fuzzing minimum

Les fuzz targets incluent :

- HCL arbitraire ;
- graphes de références cycliques et profonds ;
- binpb tronqué ou surdimensionné ;
- `oneof` absent ou inconnu ;
- indices de nœuds invalides ;
- UTF-8 invalide ;
- entiers et poids aux bornes ;
- JSONL partiel, dupliqué ou contradictoire.

Aucune entrée ne doit causer panic, boucle infinie ou allocation non bornée.

### 12.3 Outils qualité

Le dépôt utilise :

- `gofmt` et `goimports` ;
- `go vet` ;
- `golangci-lint` avec configuration versionnée ;
- `govulncheck` ;
- `buf lint` et `buf breaking` pour Protobuf ;
- analyse de dépendances et SBOM ;
- Dependabot ou Renovate ;
- CodeQL si disponible.

Les versions des outils de génération sont verrouillées. Les mises à jour de
Protobuf doivent reconstruire les artefacts et expliquer les différences d’octets.

### 12.4 Menaces considérées

- bundle forgé ou corrompu ;
- bombe de taille ou de profondeur ;
- nœud inconnu ignoré par Protobuf ;
- dépassement d’entier ;
- différence Unicode entre runtimes ;
- ReDoS — évité par absence de regex générique ;
- cycle de règles ;
- checksum exécuté sur une entrée non conforme ;
- source métier erronée ou devenue obsolète ;
- attaque de chaîne d’approvisionnement.

Tous les calculs d’entiers doivent vérifier les débordements. Les checksums de grands
nombres utilisent des calculs modulo chiffre par chiffre.

## 13. CI

La CI de pull request exécute au minimum :

1. format ;
2. lint Go et HCL ;
3. `buf lint` et compatibilité Protobuf ;
4. tests unitaires ;
5. tests race ;
6. tests de conformité de référence ;
7. fuzz smoke tests ;
8. couverture et seuils ;
9. compilation reproductible ;
10. `check-generated` ;
11. audit de vulnérabilités ;
12. vérification de provenance pour toute règle modifiée.
13. vérification de classification, licence et absence de données interdites dans
    chaque cas de conformité.

Une CI planifiée exécute les fuzzers longs, mutation tests, vérification des liens de
sources et compatibilité avec les dernières releases des quatre moteurs.

## 14. Documentation générée

Le compilateur génère :

- matrice pays × type × format × checksum ;
- liste des variantes et profils ;
- sources et dates de vérification ;
- algorithmes utilisés ;
- règles sans checksum publié ;
- changements entre deux versions ;
- statistiques de conformité ;
- feature IDs nécessaires.

Il génère également les deux références normatives des moteurs :

- `docs/ir.md`, sémantique exhaustive de chaque message, enum, opcode, valeur
  absente, erreur et limite d’exécution ;
- `docs/features.md`, contenu exact et immuable de chaque capability ID.

Ces documents, `rules.proto`, `conformance.proto`, un bundle de référence minimal et
sa conformité sont publiés avant le début d’implémentation des moteurs. Un prompt
moteur qui ne dispose que de `engine.md` est incomplet et ne doit pas commencer à
coder en devinant la sémantique manquante.

La documentation ne doit pas être maintenue manuellement en parallèle des règles.

## 15. Politique de contribution des règles

Une PR de règle est recevable uniquement si elle inclut :

- source officielle ou justification documentée ;
- cas valides et invalides ;
- analyse du risque de faux négatif ;
- résultat de `entidc diff` ;
- mise à jour de provenance ;
- conformité complète verte ;
- absence de restriction involontaire des formats existants.

Une restriction de règle est considérée comme un changement à haut risque et exige
une revue renforcée. Un élargissement documenté corrigeant un faux négatif peut être
publié en patch de règles.

## 16. Plan d’implémentation obligatoire

L’agent chargé du dépôt doit suivre cet ordre :

1. initialiser gouvernance, Go, CI et outils verrouillés ;
2. écrire les `.proto` et leurs tests de compatibilité ;
3. implémenter l’AST typée, les dispatchers et le registre de feature IDs immuables ;
4. implémenter le parser HCL et les diagnostics ;
5. implémenter linker, cycles et typechecker ;
6. implémenter lowering vers graphe topologique ;
7. implémenter validation défensive et sérialisation binpb ;
8. implémenter le lecteur/compilateur JSONL → conformance binpb ;
9. implémenter l’interpréteur de référence en TDD ;
10. implémenter la CLI complète ;
11. créer un pilote représentatif : SIREN, VAT BE, VAT FR, VAT DE et EUID FR ;
12. couvrir format simple, checksum Luhn, modulo pondéré, composition et unsupported ;
13. mettre en place fuzzing, race, couverture et mutation ;
14. générer `ir.md`, `features.md`, documentation et manifestes ;
15. mettre en place release et synchronisation downstream ;
16. migrer progressivement le reste de la couverture historique après validation des sources.

Chaque étape doit laisser le dépôt compilable et les tests verts.

## 17. Définition de terminé

Le dépôt V1 est terminé lorsque :

- toutes les commandes de la CLI sont fonctionnelles et documentées ;
- les HCL invalides produisent des diagnostics précis ;
- les cycles et erreurs de type sont détectés ;
- les cycles inter-programmes et ambiguïtés de dispatch sont détectés ;
- les deux schémas Protobuf sont lintés et protégés contre les breaking changes ;
- les artefacts binpb et manifestes sont reproductibles ;
- deux compilations depuis des chemins absolus et systèmes compatibles différents
  produisent les mêmes octets BINPB et les mêmes digests ;
- l’interpréteur de référence passe toute la conformité ;
- les cas JSONL et binpb sont strictement synchronisés ;
- aucune attente n’est générée circulairement ;
- les limites de sécurité sont testées ;
- les seuils de couverture sont atteints ;
- les fuzzers ne trouvent aucun crash ;
- la documentation générée correspond aux règles ;
- `ir.md`, `features.md`, les deux `.proto` et les fixtures de référence permettent
  à un moteur d’être implémenté sans consulter le code Go du compilateur ;
- le runner de conformité exécute un testee externe de bout en bout et rapporte un
  écart lorsqu’une réponse diffère, y compris sur un seul champ ;
- le protocole de testee est documenté et son schéma est publié, de sorte qu’une
  implémentation tierce puisse se déclarer conforme sans modifier ce dépôt ;
- une release de test est découverte par les quatre moteurs, qui ouvrent chacun
  leur PR de synchronisation ;
- le pilote couvre toutes les familles d’opérations nécessaires ;
- README, CONTRIBUTING, SECURITY, DATA_POLICY et guide de langage permettent une
  contribution autonome.

## 18. Références techniques

- HCL native syntax : <https://github.com/hashicorp/hcl/blob/main/hclsyntax/spec.md>
- HCL language design : <https://github.com/hashicorp/hcl/blob/main/guide/language_design.rst>
- Protobuf proto3 : <https://protobuf.dev/programming-guides/proto3/>
- Protobuf best practices : <https://protobuf.dev/best-practices/dos-donts/>
- Protobuf non-canonical serialization : <https://protobuf.dev/programming-guides/serialization-not-canonical/>
- Biscuit serialization/versioning : <https://doc.biscuitsec.org/reference/specifications>
- SwiftProtobuf : <https://github.com/apple/swift-protobuf>
- Protobuf Kotlin : <https://protobuf.dev/reference/kotlin/>
- Protobuf-ES : <https://github.com/bufbuild/protobuf-es>

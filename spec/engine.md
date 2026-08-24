# LibBusinessID — Contrat commun des moteurs

## 1. Objet

Ce document définit le comportement normatif de tous les moteurs LibBusinessID :

- `businessid-go` ;
- `businessid-swift` ;
- `businessid-kotlin` ;
- `businessid-typescript`.

Il est indépendant d’un langage. Les documents `engine-<langage>.md` complètent ce
contrat avec les conventions, outils et APIs propres à chaque écosystème.

Les mots **DOIT**, **NE DOIT PAS**, **DEVRAIT**, **NE DEVRAIT PAS** et **PEUT** sont
normatifs.

Un moteur est responsable de :

1. charger et valider défensivement un `businessid-rules.binpb` ;
2. exposer une API idiomatique ;
3. dispatcher puis canonicaliser une entrée selon les deux phases normatives ;
4. exécuter format puis checksum selon les règles communes ;
5. produire le contrat de résultat commun ;
6. exécuter la suite `businessid-conformance.binpb` ;
7. prévoir une interface de registre sans implémentation réseau en V1.

Un moteur n’analyse jamais HCL et ne contient aucune règle nationale codée en dur.

### 1.1 Entrées normatives obligatoires

L’implémentation d’un moteur NE DOIT PAS commencer avec ce seul document. Le dépôt
`spec` doit fournir, pour la même release :

- `engine.md` (le présent contrat) ;
- `rules.proto` et `conformance.proto` ;
- `ir.md`, qui spécifie exhaustivement chaque opcode et invariant ;
- `features.md`, qui fige le contenu exact de chaque capability ID ;
- `businessid-rules.binpb`, `businessid-conformance.binpb`, leur manifeste,
  `SHA256SUMS` et leur attestation ;
- les fixtures minimales valides et invalides du décodeur.

Ces éléments sont conjointement normatifs. En cas de contradiction, l’agent doit
arrêter l’implémentation, ouvrir un défaut dans `spec` et ne pas choisir arbitrairement
une interprétation. Le code Go de l’interpréteur de référence n’est jamais une source
normative et ne doit pas être copié.

## 1.2 Générateur, pas interpréteur

Un moteur NE DOIT PAS interpréter le bundle à l’exécution. Le travail se sépare en
deux :

- un **générateur**, que vous écrivez, s’exécute à la construction du moteur. Il lit
  `businessid-rules.binpb`, applique les vingt-cinq contrôles de chargement de
  `ir.md` section 10, et émet du code source dans votre langage. Il refuse de produire
  quoi que ce soit s’il ne comprend pas une version, un champ, un opcode ou une
  capacité.
- le **moteur**, qui est ce qui est livré : le code généré, un jeu réduit de
  primitives qu’il appelle, et une API publique écrite à la main.

Le générateur n’est pas dans le dépôt `spec` et n’y sera jamais : la spécification
reste agnostique et n’héberge aucun langage cible. Écrivez-le dans le langage qui vous
convient, du moment qu’il sait lire le bundle.

Ce point était auparavant absent de ce document, énoncé dans `PROVENANCE.md`, et
contredit par une tolérance de `spec.md` section 2.3. Un moteur a suivi cette
tolérance de bonne foi et livré un interpréteur complet. La règle est désormais ici,
dans le contrat commun, où elle aurait dû être.

Conséquence sur l’API : aucun moteur n’expose de fabrique acceptant un bundle en
octets à l’exécution. Un jeu de règles personnalisé passe par le générateur, à la
construction. Les vingt-cinq contrôles de chargement restent intégralement exigés,
et le protocole `testee` les exerce : un testee qui génère du code en amont répond aux
cas `load_ruleset` en appelant son générateur, comme le dit le commentaire du champ 7
de `testee.proto`.

## 2. Priorités de conception

Dans l’ordre :

1. ne pas refuser une entrée valide ;
2. produire le même comportement sémantique dans les quatre moteurs ;
3. échouer de façon sûre face à un bundle incompatible ou corrompu ;
4. fournir une API naturelle dans le langage cible ;
5. rester hors ligne, déterministe et thread-safe ;
6. être performant sans compromettre lisibilité ou vérifiabilité.

Un moteur NE DOIT PAS transformer une absence de connaissance en invalidité. Les
statuts `unsupported` et `not_run` sont des résultats normaux.

## 3. Terminologie

- **bundle** : artefact `businessid-rules.binpb` ;
- **règles version** : version métier `YYYY.MM.PATCH` ;
- **format version** : version entière de l’IR ;
- **moteur** : runtime d’un langage ;
- **définition** : ensemble canonicalizer + format + checksum pour kind/pays ;
- **dispatcher** : table kind/pays/préfixe et pré-canonicalizer servant à sélectionner
  une définition avant toute transformation spécifique ;
- **entrée brute** : chaîne fournie par l’utilisateur ;
- **valeur canonique** : chaîne après canonicalisation ;
- **étape** : format, checksum ou registre ;
- **conformité** : suite partagée issue du dépôt `spec` ;
- **erreur moteur** : problème technique empêchant d’obtenir un résultat métier.

## 4. Périmètre V1

Le moteur V1 implémente :

- chargement de règles embarquées ;
- canonicalisation ;
- format ;
- checksum ;
- résultats et diagnostics ;
- interface de registre sans provider concret ;
- inspection des versions et capabilities.

Il n’implémente pas :

- téléchargement de bundle ;
- appel HTTP ;
- cache de registre ;
- auto-détection probabiliste ;
- exécution HCL ;
- mise à jour silencieuse des règles.

## 5. Modèle de données commun

Les noms publics peuvent être adaptés au langage, mais leur sens et leur forme
sérialisée sont normatifs.

### 5.1 IdentifierInput

```text
IdentifierInput
  kind: IdentifierKind
  value: String
  countryCode: String?        # contexte optionnel
```

`value` conserve exactement l’entrée brute dans le résultat. Le moteur ne doit pas
la modifier en place.

`IdentifierKind` est un value type chaîne extensible avec constantes connues, pas un
enum fermé : le moteur doit pouvoir rapporter proprement `unsupported_kind` pour un
token futur ou inconnu.

`countryCode` est optionnel. Lorsqu’il est fourni, le moteur applique la
canonicalisation de code pays définie par le bundle. Un conflit prouvé entre le
préfixe et le contexte produit `country_mismatch`.

### 5.2 ValidationOptions

```text
ValidationOptions
  profile: ValidationProfile | absent
```

Le profil est optionnel et son absence est signifiante : elle laisse la définition
sélectionnée appliquer son `default_profile`. Une API qui remplirait `compatible`
par défaut rendrait `default_profile` inatteignable, puisque le moteur ne pourrait
plus distinguer un appelant silencieux d’un appelant demandant `compatible`. Voir
`ir.md` section 5.2, et le commentaire du champ 4 de `testee.proto`.

Profils V1 :

- `compatible` : défaut, accepte les variantes actuelles et legacy encore
  documentées comme légitimes ;
- `strict_current` : opt-in, applique uniquement les variantes d’émission actuelle
  explicitement déclarées.

Aucune option implicite dépendante de la locale ou de la plateforme n’est permise.

### 5.3 StepStatus

```text
valid
invalid
unsupported
not_run
```

Sémantique :

- `valid` : toutes les règles applicables de l’étape ont réussi ;
- `invalid` : une règle applicable prouve l’échec ;
- `unsupported` : aucune conclusion n’est possible avec ce bundle/moteur ;
- `not_run` : une étape précédente interdit ou rend inutile l’exécution.

### 5.4 ValidationLevel

```text
format
checksum
registry
```

### 5.5 ReasonCode

Registre V1 minimal :

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

Les moteurs peuvent ajouter un type d’erreur technique, mais ne doivent pas inventer
de `ReasonCode` métier sans évolution du dépôt `spec`.

### 5.6 StepResult

```text
StepResult
  level: ValidationLevel
  status: StepStatus
  reasonCode: ReasonCode
  messageKey: String?
```

`messageKey` est stable lorsqu’il vient de la règle. Un message humain localisé est
optionnel et non normatif. Les tests communs comparent le code et la clé, pas le
texte.

Les résultats produits par le moteur avant une assertion de règle (`input_too_long`,
dispatch, `not_requested` et raisons `not_run_*`) ont toujours `messageKey = null`.
Une assertion ou un checksum déclaré par le bundle conserve exactement sa
`message_key`, y compris si elle est absente.

Les couples sont contraints : `valid` utilise toujours `ok`; `not_run` utilise
uniquement `not_requested`, `not_run_format_invalid` ou
`not_run_format_unsupported`; `invalid` ne peut utiliser qu’une raison prouvant une
invalidité (`empty`, `invalid_length`, `invalid_characters`, `invalid_format`,
`invalid_checksum`, `country_mismatch` ou une future raison déclarée par une nouvelle
capability) ; les autres raisons métier sont `unsupported`. Le loader rejette une
règle pouvant produire un couple interdit.

### 5.7 ValidationReport

```text
ValidationReport
  kind: IdentifierKind
  inputValue: String
  canonicalValue: String
  countryCode: String?
  profile: ValidationProfile
  rulesVersion: String
  formatVersion: Integer
  engineVersion: String
  format: StepResult
  checksum: StepResult
```

Les champs d’identité suivent les mêmes règles pour tous les rapports : `inputValue`
est toujours la chaîne brute ; `kind` est le kind canonique lorsqu’un dispatcher a
été résolu, sinon le token demandé après trim/lowercase ASCII ; `countryCode` et
`canonicalValue` suivent les états intermédiaires définis en 5.8. Ainsi, une sortie
reste entièrement déterminée même quand le dispatch échoue avant la définition.

Il n’existe pas de booléen `isValid` normatif sur le rapport complet, car
`format=valid` avec `checksum=unsupported` n’est ni entièrement validé ni invalide.
Chaque API peut fournir des propriétés calculées clairement nommées :

- `isFormatValid` ;
- `isChecksumValid` ;
- `isFullyValidated` = format et checksum valides ;
- `isInvalid` = au moins une étape exécutée invalide.

Une propriété ambiguë `valid` ou `isValid` sur le rapport complet est déconseillée.

### 5.8 CanonicalizationResult

```text
CanonicalizationResult
  kind: IdentifierKind
  inputValue: String
  canonicalValue: String
  countryCode: String?
  profile: ValidationProfile
  rulesVersion: String
  formatVersion: Integer
  engineVersion: String
  status: StepStatus
  reasonCode: ReasonCode
  messageKey: String?
```

`canonicalize` retourne exactement ce type sémantique. Son statut est `valid`/`ok`
si une définition a été sélectionnée et les deux phases ont réussi,
`invalid`/`country_mismatch` lorsqu’un pays explicite contredit un préfixe reconnu,
ou `unsupported` avec `unsupported_kind`, `unsupported_country`,
`missing_country_code` ou `input_too_long`. `not_run` n’est jamais un statut final de
canonicalisation.

Lorsque le kind est inconnu ou l’entrée trop longue, `canonicalValue` est exactement
`inputValue` et `countryCode` conserve le contexte fourni. Après résolution du
dispatcher, mais avant sélection d’une définition, `canonicalValue` est la valeur
pré-canonique et `countryCode` est le pays normalisé s’il existe. Après sélection,
il s’agit de la valeur canonicalisée par la définition et du pays de la cible ; pour
une cible globale, le contexte pays normalisé est conservé sans influencer la règle.

## 6. Erreurs techniques et résultats métier

Les entrées utilisateur ordinaires ne doivent pas lever d’exception ni provoquer de
panic. Elles produisent un rapport.

Sont des erreurs techniques :

- bundle tronqué ou Protobuf invalide ;
- version ou feature incompatible lors de la construction du moteur ;
- graphe invalide ;
- dépassement de limite structurelle ;
- invariant interne violé ;
- provider registre ayant échoué techniquement — réservé, pas à livrer : la
  section 10 diffère le registre et aucun moteur V1 n'en porte.

Une API peut représenter ces erreurs par exception, `Result`, `error`, type scellé ou
Promise rejetée selon le langage. Les APIs spécifiques sont définies dans les
documents langage.

Une définition absente n’est pas une erreur technique : elle produit
`unsupported_kind` ou `unsupported_country`.

## 7. Chargement du bundle

### 7.1 Moteur par défaut

Chaque package embarque un bundle officiel et expose un moteur par défaut prêt à
l’emploi. Le bundle est décodé au plus une fois par processus ou instance globale,
selon les conventions du langage.

Le chargement doit être lazy si cela améliore le coût d’initialisation sans créer de
race. Après chargement, toutes les structures sont immuables.

### 7.2 Bundle personnalisé

Un jeu de règles personnalisé passe par le générateur, à la construction. Le moteur
n'expose aucune fabrique acceptant un bundle en octets : la section 1.2 l'interdit,
parce qu'une telle API impose le validateur complet et la machine d'exécution de l'IR
à chaque appelant.

Le générateur, lui, traite le bundle comme une entrée non fiable et applique les
vingt-cinq contrôles avant d'émettre la moindre ligne.

Le moteur ne télécharge jamais le bundle.

### 7.3 Validation obligatoire avant exécution

Dans cet ordre :

1. taille binaire ≤ 16 MiB ;
2. décodage Protobuf complet ;
3. `format_version` supportée ;
4. toutes les `required_feature_ids` connues ;
5. absence de champ inconnu à toute profondeur ; si le runtime ne les expose pas,
   effectuer un pré-scan wire borné contre les descripteurs ;
6. champs obligatoires sémantiques présents ;
7. valeurs enum différentes de `UNSPECIFIED` ;
8. nombres d’objets dans les limites ;
9. unicité dispatchers, kind/pays et IDs ;
10. références de nœuds dans les bornes et vers un index inférieur ;
11. types d’entrées/sorties cohérents ;
12. racines présentes ;
13. chaînes UTF-8, tailles valides et digests SHA-256 de exactement 32 octets ;
14. absence de feature utilisée mais non déclarée ;
15. dispatchers sans alias/préfixe ambigu, cible orpheline ou incohérence kind/pays ;
16. graphe des `CallOperation` acyclique, typé et de profondeur statique ≤ 32 ;
17. toutes les constantes, poids, moduli, tranches et bornes arithmétiques conformes
    à `ir.md` et à la section 9.2, sans overflow possible ;
18. version métier non vide.

L’absence d’un `oneof operation` après décodage doit être refusée. Un moteur ne doit
jamais exécuter un graphe partiellement validé.

L’ordre des trois premiers contrôles sémantiques porte un sens. Un bundle construit
contre une version ultérieure porte des champs que ce runtime n’a jamais vus ;
les signaler comme champs inconnus revient à traiter un écart de version comme une
contrefaçon. Demander d’abord si le bundle annonce quelque chose de non supporté
donne la réponse exacte, et dit à un opérateur de mettre à jour plutôt que de
suspecter le fichier.

Cette distinction ne tient que si le décodage reste au niveau du fil. Un moteur qui
résout un opcode, une valeur d’énumération ou un identifiant de capacité pendant le
décodage signale un bundle plus récent au contrôle 2, avant que le contrôle des
capacités puisse l’excuser, et l’écart de version redevient un bundle malformé.
`ir.md` section 10 fait foi sur l’ordre complet des contrôles et sur leur nombre ; la liste
ci-dessus en est la vue par famille.

### 7.4 Protobuf n’est pas l’API publique

Les types Protobuf générés restent internes. Les consommateurs utilisent les types
du domaine du moteur. Cela permet de faire évoluer le runtime Protobuf sans casser
l’API publique.

## 8. Pipeline normatif de validation

### 8.0 Dispatch commun à toutes les opérations

Le dispatch précède toute canonicalisation spécifique :

`trim ASCII` retire seulement `U+0009..U+000D` et `U+0020` aux extrémités. Après
normalisation, un kind respecte `[a-z][a-z0-9_-]{0,63}`, un pays `[A-Z]{2}` et un
préfixe déclaré est ASCII alphanumérique de 1 à 8 caractères. Les autres tokens pays
sont `unsupported_country` ; un token kind mal formé ou inconnu est
`unsupported_kind`.

1. normaliser le kind par trim ASCII, lowercase ASCII et table `kind_aliases` ;
2. si aucun dispatcher ne correspond, retourner `unsupported_kind` sans programme ;
3. exécuter le `pre_canonicalization_program`, limité aux transformations sûres
   déclarées dans `ir.md` ;
4. normaliser un pays explicite par uppercase ASCII et `country_aliases` ; s’il est
   syntaxiquement invalide, retourner `unsupported_country`; s’il n’a pas de cible
   dans un dispatcher country-specific, retourner aussi `unsupported_country` ;
5. choisir la correspondance exacte au plus long `accepted_prefix` ;
6. si pays et préfixe pointent vers deux cibles différentes, retourner
   `country_mismatch` ;
7. choisir la cible du pays, sinon celle du préfixe, sinon la cible `GLOBAL`, sinon
   l’unique cible `allow_unprefixed_without_country` ;
8. si aucune cible n’est choisie, retourner `missing_country_code` ;
9. vérifier la cohérence kind/pays de la définition puis exécuter son programme de
   canonicalisation sur la valeur pré-canonique.

L’ordre des étapes 3 et 4 est observable et `ir.md` section 5 fait foi. La
pré-canonicalisation précède la décision de pays, donc un pays inutilisable est
rapporté avec la valeur déjà pré-canonique, pas avec la valeur brute. Ce document les
donnait dans l’ordre inverse, ce qui changeait la `canonical_value` rapportée sur
cette branche ; aucun cas du corpus ne le distinguait.

Les alias de kind, alias de pays et préfixes sont des espaces séparés. Le pays du
résultat est le code ISO de la cible, pas nécessairement son préfixe métier (par
exemple `GR` et `EL`). Un dispatcher global a exactement une cible globale sans
préfixe ; un pays explicite bien formé ne participe pas au routage et reste présent,
normalisé, dans le résultat. Le loader a déjà rejeté l’association d’une même valeur de
préfixe à plusieurs cibles, les alias contradictoires et les cibles implicites
multiples ; aucune résolution ne dépend donc de l’ordre de sérialisation.

### 8.1 `validate`

Algorithme :

1. vérifier la longueur brute maximale de 1 024 octets UTF-8 ;
   si elle est dépassée, produire format `unsupported`/`input_too_long` et checksum
   `not_run`/`not_run_format_unsupported` sans exécuter le bundle ;
2. exécuter l’algorithme de dispatch 8.0, y compris ses deux phases de
   canonicalisation au plus une fois chacune ;
3. si le dispatch retourne `country_mismatch`, produire format
   `invalid`/`country_mismatch` et checksum `not_run`/`not_run_format_invalid` ;
4. si le dispatch retourne une autre non-réussite, produire format `unsupported`
   avec cette raison et checksum `not_run`/`not_run_format_unsupported` ;
5. exécuter le format sur la valeur canonique ;
6. si format invalide : checksum `not_run` avec `not_run_format_invalid` ;
7. si format unsupported : checksum `not_run` avec
   `not_run_format_unsupported` ;
8. si format valide et checksum absent : checksum `unsupported` avec
    `unsupported_checksum` ou la raison plus précise portée par la définition ;
9. si format valide et checksum présent : exécuter le checksum ;
10. retourner un `ValidationReport` immuable.

Le checksum ne doit jamais recevoir une entrée dont le format n’a pas été validé.

La pré-canonicalisation et la canonicalisation spécifique forment deux phases
distinctes. Chacune s’exécute au plus une fois par opération publique ; l’expression
« canonicaliser une fois » dans les APIs signifie ne jamais répéter l’une de ces
phases.

### 8.2 `validateFormat`

Cette opération suit les étapes 1 à 7 applicables de `validate`, sans exécuter de
checksum, et retourne toujours un `ValidationReport` complet. Si le dispatch et le
format réussissent, `format` vaut `valid`/`ok` et `checksum` vaut
`not_run`/`not_requested`. Si le dispatch ou le format échoue, les deux étapes ont
exactement les mêmes statuts et raisons que `validate`. Aucun rapport partiel ni
`StepResult` isolé n’est conforme.

### 8.3 `validateChecksum`

Cette opération applique obligatoirement le format comme garde. Si le format est
invalide, le checksum n’est pas exécuté et son statut est `not_run` avec
`not_run_format_invalid`. Si le format est unsupported, le checksum est `not_run`
avec `not_run_format_unsupported`. L’appelant doit pouvoir inspecter le résultat de
format. Elle retourne le même `ValidationReport` que `validate` pour la même entrée,
les mêmes options et le même bundle. L’opération séparée existe pour la lisibilité de
l’API, pas pour contourner le format.

### 8.4 Canonicalisation

L’opération publique `canonicalize` exécute uniquement la limite d’entrée, le
dispatch, la pré-canonicalisation et la canonicalisation spécifique. Elle n’exécute
ni format ni checksum et retourne exactement `CanonicalizationResult`.

Chaque phase de canonicalisation est :

- exécutée au plus une fois par opération publique ;
- idempotente ;
- indépendante de la locale ;
- conforme à `whitespace_v1` et à l’uppercase ASCII ;
- ne masque jamais une erreur métier non autorisée par la règle ;
- conserve l’entrée brute séparément ;
- ne tronque jamais silencieusement.

Une impossibilité interne dans un programme de canonicalisation après validation du
bundle est une erreur moteur, jamais un statut `invalid`. Les résultats exacts en cas
d’absence de dispatcher, pays, préfixe ou contradiction sont ceux de la section 5.8.

### 8.5 Ordre des assertions

Les assertions de format sont exécutées dans l’ordre de l’IR. La première qui échoue
détermine raison et message. Tous les moteurs doivent respecter cet ordre.

### 8.6 Checksums multi-variantes

Les branches sont testées dans l’ordre publié. Une branche non applicable ne vaut
pas `invalid`. Si aucune branche documentée n’est applicable mais que le format est
valide, le résultat est `unsupported_checksum` ou `checksum_not_published`.

## 9. Sémantique d’exécution de l’IR

### 9.1 Types et absence

Le moteur possède des valeurs internes typées : chaîne, entier borné, booléen,
chaîne absente et résultat checksum. Les conversions implicites sont interdites.

Une vue hors limites produit une valeur absente, jamais une exception. `ir.md`
section 1.1 le dit sans réserve : « Absence is never an error and never an
exception. »

Ce paragraphe ajoutait ensuite qu'un accès hors limites dans un checksum après un
format valide devait produire une erreur moteur. Il se contredisait donc en deux
phrases, et l'écart était observable sur une entrée réelle : deux moteurs pouvaient
répondre l'un une absence, l'autre une erreur. Le moteur Kotlin l'a relevé et a suivi
`ir.md`, ce qui est la bonne lecture. L'intuition derrière la phrase supprimée reste
juste — une règle de format est censée établir les bornes avant le checksum — mais
c'est une propriété du jeu de règles, pas un comportement d'exécution, et rien ne
permet de la prouver au chargement.

### 9.2 Entiers

Les opérations utilisent des entiers signés d’au moins 64 bits avec vérification de
débordement, ou un type sûr équivalent. Les numéros arbitrairement longs ne sont pas
convertis en entier : le modulo est calculé chiffre par chiffre.

La V1 borne : constantes à `int64`, modulus/compléments à `2..1 000 000 000`, valeur
absolue des poids à `0..1 000 000`, nombre de poids à `1..256`, tables de reste à
`1..1 000 000`, indices/tranches à `0..4 096` et concaténations à 256 opérandes.
Addition, multiplication, négation et conversion sont checked. Tout calcul dont la
sûreté ne peut être prouvée au chargement rend le bundle `invalid_ruleset` ; aucun
wrap ou saturation silencieuse n’est permis.
`digits_to_integer` exige une borne statique ≤ `INT64_MAX`; sinon le bundle est
refusé et l’algorithme doit utiliser le modulo chiffre par chiffre.

### 9.3 Chaînes

Les classes métier sont ASCII explicites. Les positions sont calculées selon les
points de code définis dans `ir.md`, pas selon les unités UTF-16 propres à certains
runtimes. Après validation ASCII, les optimisations par octets sont permises.

### 9.4 Limites d’exécution

Le moteur applique toutes les limites de `ir.md`, notamment :

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

Une limite dépassée pendant le chargement rejette le bundle. Une entrée trop longue
produit format `unsupported`/`input_too_long` et checksum `not_run` : la limite de
sécurité ne constitue pas à elle seule une preuve métier d’invalidité. Les moteurs
peuvent choisir une limite interne plus haute, jamais plus basse.

## 10. Registre : différé, mais sa place est réservée

La consultation d'un registre distant n'est **pas** dans cette version. Un moteur V1
valide le format et le checksum, localement, et rien d'autre. Aucun moteur ne doit
livrer `RegistryProvider`, et un moteur qui ne le livre pas est pleinement conforme.

Ce qui est décidé maintenant, ce n'est pas l'interface : c'est la place qu'elle
occupera, pour qu'elle puisse arriver sans casser une API publique déjà figée par
SemVer.

### 10.1 Ce qui ne bougera pas

Trois propriétés sont arrêtées dès maintenant, parce que les changer plus tard serait
une rupture :

- **La validation locale reste synchrone, définitivement.** `canonicalize`, `validate`,
  `validateFormat` et `validateChecksum` ne deviendront jamais asynchrones. Une
  consultation de registre est une opération distincte, asynchrone, jamais un mode de
  celles-ci. C'est la propriété la plus structurante : la rompre transformerait chaque
  appelant.
- **`validate` n'appellera jamais un registre.** Le niveau registre n'entre pas dans
  `ValidationReport` par un champ que `validate` remplirait ; il arrivera comme un
  rapport distinct, produit par une opération distincte.
- **`registry_not_configured` existe déjà** dans le registre des `ReasonCode` et y
  reste. Il n'a pas d'usage en V1 ; il est réservé.

### 10.2 La contrainte qui décide de la forme

Une consultation de registre porte un jeton d'API. **Un moteur ne doit jamais rendre
cette consultation possible depuis un navigateur** : le jeton y serait exposé à
quiconque ouvre les outils de développement, et aucune précaution d'implémentation ne
répare cela.

La conséquence est une exigence de packaging, pas de style :

- le cœur, qui valide format et checksum, reste utilisable partout, navigateur
  compris ;
- tout ce qui consulte un registre vit dans un **module ou paquet séparé**, qui ne
  s'installe et ne se charge que dans un contexte serveur ;
- pour un langage qui cible les deux, comme TypeScript, cela signifie un point
  d'entrée d'export distinct, absent du bundle navigateur, et non un drapeau
  d'exécution.

### 10.3 Disponibilité variable, conformité identique

Le registre pourra exister dans un langage et pas dans un autre, et cela ne crée pas
deux niveaux de conformité. La conformité se mesure sur le corpus, qui ne contient que
des opérations locales : un moteur sans registre passe les mêmes cas qu'un moteur qui
en a un.

Un langage qui ne cible que le serveur pourra donc le proposer avant les autres, et un
langage qui cible le navigateur pourra ne l'offrir que dans son point d'entrée serveur.

### 10.4 Ce qu'un moteur V1 doit faire

Rien, sinon ne pas se fermer la porte :

- n'exposez aucun type de registre, même expérimental — un type public est un
  engagement ;
- ne rendez aucune méthode de validation asynchrone « au cas où » ;
- ne mettez aucune dépendance HTTP dans le paquet du cœur.

L'interface elle-même — `RegistryProvider`, les statuts d'un résultat, la distinction
entre indisponibilité et absence — sera spécifiée quand elle sera construite. Une
indisponibilité ne devra jamais devenir `invalid`, mais cette règle se formulera avec
le reste.

## 11. Conformité commune

### 11.1 Source exécutée

Chaque moteur consomme `businessid-conformance.binpb` correspondant exactement au
`rules.lock`. Le hash doit être vérifié lors de la mise à jour du dépôt.

Tous les cas communs sont obligatoires. Un moteur ne peut pas exclure un cas pour
faire passer sa CI. Une incompatibilité doit être corrigée ou documentée comme
blocage de release.

Le moteur fournit un **testee** et rien d'autre : un exécutable qui lit des requêtes
sur son entrée standard, appelle son API publique et écrit des réponses. Il ne lit
pas le corpus, n'interprète aucun résultat attendu et n'adapte pas son comportement
au cas reçu.

Le **runner**, lui, vient de `spec` et de nulle part ailleurs. Il s'exécute épinglé au
commit que `rules.lock` enregistre sous `source_commit`, donc au même commit que le
corpus :

```bash
go run github.com/libbusinessid/spec/cmd/conformance-runner@<source_commit> \
  -corpus spec/businessid-conformance.binpb -- ./mon-testee
```

Aucune release n'est nécessaire, rien n'est à télécharger à la main, et le seul
prérequis est une toolchain Go dans la CI : c'est un outil de construction, il
n'entre ni dans le paquet publié ni dans ses dépendances.

`actions/setup-go` pose `GOTOOLCHAIN: local`, ce qui interdit de récupérer une
toolchain. L'étape du runner doit donc poser `GOTOOLCHAIN: auto` ; le testee, lui,
reste construit avec la toolchain épinglée du moteur.

La condition exacte est la ligne `go` du module `spec`, pas sa ligne `toolchain` :
c'est la première qui lie. Un moteur épinglant un Go antérieur échoue sur la
résolution de la toolchain, pas sur un écart de conformité — c'est arrivé au moteur
Go, épinglé plus bas. Un moteur qui résout `stable` ne rencontre jamais le cas, et le
moteur TypeScript l'a mesuré ainsi. Posez `GOTOOLCHAIN: auto` quand même : c'est une
assurance qui ne coûte rien, et la ligne `go` de `spec` montera un jour.

Un moteur NE DOIT PAS écrire son propre runner. C'est la seule chose qui fasse que
« conforme » veuille dire quelque chose : un comparateur écrit par le moteur qu'il
juge peut comparer trop faiblement — oublier un champ, traiter un champ absent
comme un champ vide — et son moteur affichera la conformité en étant faux. Deux
moteurs en ont écrit un, faute de savoir que celui-ci était accessible.

### 11.2 Comparaison normative

Ce qui suit décrit ce que le runner compare. Un moteur n'a pas à l'implémenter ;
c'est ici pour qu'il sache sur quoi il est jugé.

Pour `canonicalize`, les champs comparés sont tous ceux de
`CanonicalizationResult`, sauf `engineVersion`. Pour les trois opérations de
validation, ils sont tous ceux de `ValidationReport`, sauf `engineVersion`, donc :

- kind ;
- entrée brute ;
- valeur canonique ;
- pays ;
- profil ;
- rules version ;
- format version ;
- statut et reason code de chaque étape (`checksum=not_run/not_requested` est
  obligatoire pour `validate_format` après un format valide) ;
- message key lorsqu’elle est spécifiée.

`engineVersion` et le texte humain ne sont pas comparés entre moteurs.

### 11.3 Tests propres au moteur

La conformité partagée ne remplace pas :

- tests unitaires de chaque opération IR ;
- tests du décodeur et du validateur de bundle, dans le générateur ;
- tests de l’API publique ;
- tests de concurrence ;
- tests de packaging : que le paquet publié ne porte ni bundle ni décodeur ;
- tests de limites ;
- property tests et fuzzing ;
- benchmarks ;
- tests de régression propres au runtime ;
- tests prouvant que le testee ne triche pas.

Ce dernier point mérite d'être précisé, parce qu'une intention ne se teste pas.
Le moteur TypeScript l'a formulé en propriétés observables, et c'est la forme
exigée :

| Ce qu'on affirme | Ce que ça exclut |
| --- | --- |
| le testee ne nomme ni le corpus ni rien qui en lise un | la lecture directe des attendus |
| il n'atteint aucun système de fichiers | le corpus est un fichier ; qui n'ouvre rien ne le lit pas |
| il répond identiquement quel que soit l'identifiant de cas — plausible, absurde, vide | la reconnaissance d'un cas |
| il répond identiquement quel que soit l'ordre des requêtes | un comportement dépendant de l'historique |
| il répond identiquement à une requête répétée | le non-déterminisme |

Les requêtes de ces tests sont inventées sur place : le test d'honnêteté n'ouvre
pas le corpus non plus, sinon il démontrerait le contraire de ce qu'il affirme.

## 12. Exigences qualité

### 12.1 TDD

Le développement doit suivre une boucle rouge/vert/refactor. Chaque nouvelle
opération IR commence par :

1. cas nominal ;
2. limites ;
3. erreur ;
4. cas de sécurité ;
5. conformité inter-langages.

### 12.2 Couverture

Quality gates minimum :

- couverture lignes/instructions ≥ 95 % ;
- couverture branches ≥ 90 % lorsque l’outil la fournit ;
- 100 % du dispatch des opérations IR ;
- 100 % des chemins de rejet d’une version/feature inconnue ;
- 100 % des ReasonCode atteignables couverts ;
- aucun fichier critique exclu artificiellement.

Le code Protobuf généré et les simples façades mécaniques peuvent être exclus du
calcul, avec exclusion documentée.

**Le code émis depuis le bundle l'est aussi, et pour une raison différente.** Ces
seuils portent sur le code écrit à la main : le moteur, ses primitives, son API,
son générateur. Le code émis, lui, est couvert par la conformité, et sa couverture
ne mesure pas la qualité du moteur mais celle du corpus — une branche de règle
jamais exécutée dit qu'aucun cas ne l'atteint, ce que le rapport des opérations
inutilisées dit déjà mieux. **Mesurez-la et publiez-la, ne l'érigez pas en seuil** :
un moteur irréprochable échouerait sur une lacune du corpus, et la seule façon de
repasser au vert serait d'abaisser le seuil.

Deux moteurs ont mesuré la même chose : retirer six cent soixante-huit cas de
conformité d'une suite unitaire n'a pas bougé la couverture du code écrit à la main.
La conformité est un accord entre implémentations, pas un parcours de code ; ce sont
deux outils, et confondre leurs chiffres fait prendre une bonne nouvelle pour une
mauvaise.

### 12.2.1 Les identifiants d'un README

Deux moteurs se sont arrêtés sur la même question, ce qui veut dire que le document
ne répondait pas. `DATA_POLICY.md` section 3 la tranche déjà, et sa raison mérite
d'être répétée ici : une valeur synthétique prouve un **algorithme**, une valeur
réelle prouve que la **règle décrit ce qu'un registre émet**. Ce sont deux
démonstrations différentes.

Un exemple de README démontre une API, pas le format d'un registre. Une valeur
synthétique y est donc correcte — et une valeur réelle y serait un choix discutable,
puisqu'elle désigne une entreprise sans que personne ait besoin qu'elle en désigne
une. La seule exigence est que l'exemple **dise ce qu'il est** : synthétique, produit
par le générateur documenté, et de préférence le cas de conformité dont il est tiré.

Une valeur inventée de mémoire n'est aucune des deux, et reste interdite partout.

### 12.3 Property tests

Au minimum :

- canonicalisation idempotente ;
- absence de panic/exception pour toute chaîne utilisateur ;
- ajout/retrait de séparateurs autorisés conserve la canonical value ;
- mutation du chiffre de contrôle invalide les exemples où l’algorithme le garantit ;
- un checksum unsupported ne devient jamais invalid ;
- mêmes bytes de bundle chargés simultanément donnent des moteurs équivalents ;
- le moteur est thread-safe et déterministe.

### 12.4 Fuzzing

Fuzzer :

- bytes Protobuf arbitraires ;
- bundles valides mutés ;
- graphes et indices ;
- UTF-8 et Unicode inhabituel ;
- valeurs très longues ;
- toutes les opérations arithmétiques ;
- canonicalisation conditionnelle.

Aucun crash, blocage, allocation non bornée ou lecture hors limites n’est accepté.

### 12.5 Mutation testing

Lorsque l’écosystème le permet, appliquer le mutation testing aux checksums,
comparaisons de positions, bornes et dispatch. Objectif recommandé : mutation score
≥ 80 % sur le cœur, avec analyse des mutants survivants.

## 13. Concurrence et état

- moteur immuable après construction ;
- partage sûr entre threads/tasks ;
- aucune locale globale ;
- aucun cache mutable non synchronisé ;
- aucun état de validation conservé entre deux appels ;
- initialisation lazy thread-safe.

Les types publics doivent signaler leur sûreté de concurrence selon les conventions
du langage (`Sendable`, immutable data classes, etc.).

## 14. Performance

Objectifs non normatifs mais testés en régression :

- décoder le bundle une seule fois ;
- validation sans I/O ;
- allocations limitées ;
- aucune compilation de regex ;
- lookup de définition O(1) ou O(log n) ;
- exécution proportionnelle à la taille du programme et de l’entrée ;
- pas de travail dépendant du nombre total de pays à chaque validation.

Les benchmarks doivent mesurer : chargement cold, validation simple, checksum
complexe, entrée invalide précoce et exécution parallèle.

## 15. API et compatibilité

### 15.1 API minimale

Chaque moteur expose l’équivalent de :

```text
defaultEngine()
canonicalize(input, options)
validate(input, options)
validateFormat(input, options)
validateChecksum(input, options)
rulesInfo()
capabilities()
```

Les noms et styles sont adaptés au langage. Le chemin heureux doit être simple.

Cette liste portait `engineFromRules(bytes)` et `registryLookup(...)` jusqu'à ce
qu'un moteur relève qu'elle contredisait, dans le même document, la section 1.2 qui
interdit la première et la section 10 qui diffère la seconde. Les deux sections sont
les réécritures argumentées ; cette liste est celle à laquelle elles n'avaient pas été
appliquées. Toutes ces opérations sont synchrones, définitivement.

### 15.2 Stabilité

- SemVer pour le package moteur ;
- `rulesVersion` indépendante ;
- `formatVersion` indépendante ;
- types publics documentés ;
- aucun type Protobuf public ;
- changement de comportement commun piloté par `spec` et conformité ;
- dépréciation avant suppression selon les conventions de l’écosystème.

### 15.3 Sérialisation facultative du rapport

Si le moteur offre JSON/Codable/Serializable, les noms et valeurs de champs suivent
le modèle commun. L’ordre JSON n’est pas normatif. Les valeurs enum utilisent les
chaînes en minuscules définies dans ce document.

## 16. Mise à jour des règles

Le dépôt contient :

```text
Sources/Resources/.../businessid-rules.binpb
Tests/Resources/.../businessid-conformance.binpb
rules.lock
```

Le chemin exact varie selon le langage.

La PR automatique de mise à jour doit :

1. vérifier les SHA-256 ;
2. vérifier l’attestation OIDC/Sigstore contre l’organisation, le dépôt, le workflow,
   le commit et le tag protégés attendus ;
3. vérifier `format_version` et features ;
4. vérifier que `rules.proto`, `conformance.proto`, `ir.md` et `features.md`
   correspondent à la même release ;
5. exécuter toute la conformité ;
6. produire un diff de couverture ;
7. exécuter tests, lint, fuzz smoke et couverture ;
8. mettre à jour la version exposée ;
9. ne jamais publier si une conformité échoue.

## 17. CI commune attendue

Chaque repo moteur possède au minimum :

- build sur environnements supportés ;
- format check ;
- lint strict ;
- tests unitaires ;
- conformité complète ;
- couverture avec seuils ;
- tests de concurrence ;
- fuzz smoke ;
- audit de dépendances ;
- build du package distribuable ;
- test d’installation dans un projet consommateur minimal ;
- vérification que les fichiers générés sont à jour.

Une CI planifiée exécute fuzzing long, mutation testing, benchmarks et tests avec les
versions minimales/maximales supportées du toolchain.

## 18. Revue et règles de contribution

Une PR modifiant le cœur doit répondre à :

- quelle sémantique commune est affectée ?
- les quatre moteurs peuvent-ils l’implémenter sans divergence ?
- quel risque de faux négatif est introduit ?
- quelles limites et entrées hostiles sont testées ?
- la conformité doit-elle évoluer ?
- la feature IR est-elle déjà réservée et supportée ?
- l’API publique reste-t-elle compatible ?

Une optimisation doit démontrer par tests qu’elle ne change aucun résultat.

## 19. Interdictions

Un moteur NE DOIT PAS :

- coder une règle pays en dur hors bundle ;
- parser HCL ;
- utiliser la locale système pour uppercase ou classes de caractères ;
- utiliser une regex générique pour interpréter l’IR V1 ;
- ignorer un opcode, feature ou version inconnu ;
- transformer une exception interne en `invalid_checksum` ;
- exécuter checksum après format invalide ;
- télécharger des règles implicitement ;
- effectuer un appel réseau pendant `validate` ;
- exposer les classes Protobuf comme API métier ;
- réduire les cas de conformité pour satisfaire la CI ;
- copier naïvement l’architecture interne d’un autre langage.

## 20. Définition de terminé d’un moteur V1

Un moteur est prêt à publier lorsque :

- son générateur refuse tous les bundles invalides communs, et la bibliothèque
  publiée ne porte ni bundle ni décodeur ;
- il supporte toutes les feature IDs requises ;
- l’API publique est idiomatique et documentée ;
- dispatch, canonicalisation, format et checksum suivent exactement le pipeline
  normatif et les quatre opérations retournent les formes imposées ;
- toute la conformité partagée passe ;
- les tests propres au moteur couvrent l'IR, l'API, la concurrence et le
  packaging, et ceux du générateur couvrent le décodeur ;
- les seuils de couverture sont atteints ;
- les fuzzers ne trouvent aucun crash ;
- lint et analyse statique sont sans avertissement ;
- un projet consommateur minimal peut installer et utiliser le package ;
- le bundle et `rules.lock` correspondent ;
- les schémas, `ir.md`, `features.md`, hashes et attestation correspondent à la même
  release de règles ;
- README explique précisément ce qui est et n’est pas validé ;
- SECURITY et CONTRIBUTING sont présents ;
- CI de release produit un artefact reproductible et signé selon l’écosystème.

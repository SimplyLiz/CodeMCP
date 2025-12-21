# SCIP Indexer Research

Research on CLI commands for each language's SCIP indexer, verified against official documentation.

---

## Summary Table

| Language | Indexer | Command | Prereqs | Complexity |
|----------|---------|---------|---------|------------|
| Kotlin | scip-java | Build-tool integrated | Gradle plugin + SemanticDB | Build-integrated |
| C/C++ | scip-clang | `scip-clang --compdb-path=<path>` | compile_commands.json | Medium |
| Dart | scip_dart | `dart pub global run scip_dart ./` | `dart pub get` | Low |
| Ruby | scip-ruby | `bundle exec scip-ruby [.]` | Gemfile, bundle install | Low |
| C# | scip-dotnet | `scip-dotnet index` | .NET 8+, .csproj/.sln | Low |
| PHP | scip-php | `vendor/bin/scip-php` | PHP 8.2+, composer install (lock recommended) | Low |

**Verified** = command appears in the indexer's official README/docs.

---

## Detailed Findings

### Kotlin (scip-java)

**Status: Build-tool integrated - CLI exists, but Kotlin requires Gradle integration to emit SemanticDB**

Kotlin is indexed via scip-java. The CLI exists (`scip-java index`), but Kotlin projects require Gradle plugin integration to produce SemanticDB files first.

**Support matrix:**
- Gradle Kotlin: Supported
- Maven Kotlin: Auto-config NOT supported
- Gradle Android: NOT supported yet

**Recommended path:** Integrate scip-java with Gradle plugin.

**Manual/advanced path:**
1. Compile with SemanticDB plugin to generate `*.semanticdb` files
2. Use `scip-java index-semanticdb <targetdir>` to convert to SCIP

**Source:** [scip-java docs](https://sourcegraph.github.io/scip-java/)

**Recommendation for ckb index:** Skip Kotlin auto-detection. Document manual process in wiki.

---

### C/C++ (scip-clang)

**Status: Verified**

**Command:**
```bash
scip-clang --compdb-path=path/to/compile_commands.json
```

**Requirements:**
- Must have `compile_commands.json` (compilation database)
- **Must run from project root directory** (even if compdb is elsewhere)
- Generate compile_commands.json via CMake, Bear, Meson, etc.

**Common compile_commands.json locations:**
- `compile_commands.json` (root)
- `build/compile_commands.json`
- `out/compile_commands.json`
- `cmake-build-debug/compile_commands.json`
- `cmake-build-release/compile_commands.json`
- `build/*/compile_commands.json`

**Install:**
- Download from [scip-clang releases](https://github.com/sourcegraph/scip-clang/releases)

**Output:** `index.scip` in current directory

**Gotcha:** Even if `--compdb-path` points into `build/`, execute scip-clang with cwd = project root.

**Source:** [scip-clang README](https://github.com/sourcegraph/scip-clang)

---

### Dart (scip_dart)

**Status: Verified**

**Command:**
```bash
dart pub global run scip_dart ./
```

**Install:**
```bash
dart pub global activate scip_dart
```

**Requirements:**
- Run `dart pub get` first
- Execute from project root

**Output:** `index.scip`

**Note:** Package name is `scip_dart` (underscore), not `scip-dart`.

**Source:** [pub.dev/packages/scip_dart](https://pub.dev/packages/scip_dart)

---

### Ruby (scip-ruby)

**Status: Verified**

**Command selection based on project setup:**

| Condition | Command |
|-----------|---------|
| Gemfile + sorbet/config | `bundle exec scip-ruby` |
| Gemfile, no sorbet/config | `bundle exec scip-ruby .` |
| No Gemfile | `scip-ruby .` |

**Note:** `.` is required when there is no `sorbet/config`.

**Install:**
- Download binary from [scip-ruby releases](https://github.com/sourcegraph/scip-ruby/releases)
- Or add to Gemfile and use Bundler

**Output:** `index.scip`

**Source:** [scip-ruby README](https://github.com/sourcegraph/scip-ruby)

---

### C# (scip-dotnet)

**Status: Verified**

**Command:**
```bash
scip-dotnet index
```

**Install:**
```bash
dotnet tool install --global scip-dotnet
```

**Requirements:**
- .NET 8+ runtime
- Run from project root (where .csproj or .sln lives)
- If installed but not found, ensure `$HOME/.dotnet/tools` is on PATH

**Alternative (Docker):**
```bash
docker run -v $(pwd):/app sourcegraph/scip-dotnet:latest scip-dotnet index
```

**Output:** `index.scip`

**Source:** [scip-dotnet README](https://github.com/sourcegraph/scip-dotnet)

---

### PHP (scip-php)

**Status: Verified**

**Command:**
```bash
vendor/bin/scip-php
```

**Install:**
```bash
composer require --dev davidrjenni/scip-php
composer install
```

**Requirements:**
- PHP 8.2+
- Run from project root
- Requires `composer.json`
- `composer.lock` recommended (warning if missing)
- Dependencies must be installed in `vendor/`

**Output:** `index.scip`

**Source:** [scip-php README](https://github.com/davidrjenni/scip-php)

**Note:** Wiki had wrong URL (`AhmedAbdulrahman/scip-php`). Correct repo is `davidrjenni/scip-php`.

---

## Implementation Notes for ckb index

### Languages to add (5):
1. **C/C++** - Medium complexity (compdb detection)
2. **Dart** - Low complexity
3. **Ruby** - Low complexity (sorbet/config detection)
4. **C#** - Low complexity (glob markers)
5. **PHP** - Low complexity (vendor binary check)

### Kotlin decision:
Skip auto-detection. Document scip-java Gradle integration in wiki.

### Key implementation details:
- **Don't mutate global config** - Build command args locally
- **Bounded depth scan** - For glob markers like `*.csproj`
- **Error on multi-language** - If multiple languages detected, error unless `--lang` is provided
- **C++: run from project root** - Pass compdb path as arg, keep cwd at project root
- **Ruby: check sorbet/config** - Affects whether `.` argument is needed
- **PHP: validate vendor/bin** - composer.lock missing is warning only

---

## Wiki Corrections Needed

1. **PHP URL:** `AhmedAbdulrahman/scip-php` -> `davidrjenni/scip-php`
2. **PHP prereqs:** Add "Requires PHP 8.2+"
3. **C# prereqs:** Add "Requires .NET 8+"
4. **Kotlin:** Reframe as "scip-java (Gradle plugin). Maven Kotlin auto-config not supported."
5. **C/C++:** Note compile_commands.json commonly in `build/` directory
6. **Dart:** Clarify "Package name is scip_dart (underscore)"

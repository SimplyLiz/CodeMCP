package tier

import "testing"

func TestLanguageString(t *testing.T) {
	tests := []struct {
		lang Language
		want string
	}{
		{LangGo, "go"},
		{LangTypeScript, "typescript"},
		{LangPython, "python"},
		{LangRust, "rust"},
		{Language("custom"), "custom"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.lang.String()
			if got != tt.want {
				t.Errorf("Language.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLanguageDisplayName(t *testing.T) {
	tests := []struct {
		lang Language
		want string
	}{
		{LangGo, "Go"},
		{LangTypeScript, "TypeScript"},
		{LangJavaScript, "JavaScript"},
		{LangPython, "Python"},
		{LangRust, "Rust"},
		{LangJava, "Java"},
		{LangKotlin, "Kotlin"},
		{LangCpp, "C/C++"},
		{LangCSharp, "C#"},
		{LangRuby, "Ruby"},
		{LangDart, "Dart"},
		{LangPHP, "PHP"},
		{Language("unknown"), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.lang.DisplayName()
			if got != tt.want {
				t.Errorf("%s.DisplayName() = %q, want %q", tt.lang, got, tt.want)
			}
		})
	}
}

func TestAllLanguages(t *testing.T) {
	langs := AllLanguages()

	// Should have all supported languages
	if len(langs) != 12 {
		t.Errorf("AllLanguages() returned %d languages, want 12", len(langs))
	}

	// Check they are in alphabetical order
	for i := 1; i < len(langs); i++ {
		if langs[i-1].String() > langs[i].String() {
			t.Errorf("AllLanguages() not sorted: %s > %s", langs[i-1], langs[i])
		}
	}

	// Verify some expected languages are present
	expected := []Language{LangGo, LangTypeScript, LangPython, LangRust}
	for _, exp := range expected {
		found := false
		for _, l := range langs {
			if l == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("AllLanguages() missing %s", exp)
		}
	}
}

func TestParseLanguage(t *testing.T) {
	tests := []struct {
		input string
		want  Language
		ok    bool
	}{
		// Go
		{"go", LangGo, true},
		{"golang", LangGo, true},
		{"GO", LangGo, true},
		{"  go  ", LangGo, true},

		// TypeScript
		{"typescript", LangTypeScript, true},
		{"ts", LangTypeScript, true},

		// JavaScript
		{"javascript", LangJavaScript, true},
		{"js", LangJavaScript, true},

		// Python
		{"python", LangPython, true},
		{"py", LangPython, true},

		// Rust
		{"rust", LangRust, true},
		{"rs", LangRust, true},

		// Java
		{"java", LangJava, true},

		// Kotlin
		{"kotlin", LangKotlin, true},
		{"kt", LangKotlin, true},

		// C++
		{"cpp", LangCpp, true},
		{"c++", LangCpp, true},
		{"cxx", LangCpp, true},
		{"c", LangCpp, true},

		// C#
		{"csharp", LangCSharp, true},
		{"c#", LangCSharp, true},
		{"cs", LangCSharp, true},

		// Ruby
		{"ruby", LangRuby, true},
		{"rb", LangRuby, true},

		// Dart
		{"dart", LangDart, true},

		// PHP
		{"php", LangPHP, true},

		// Unknown
		{"unknown", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := ParseLanguage(tt.input)
			if ok != tt.ok {
				t.Errorf("ParseLanguage(%q) ok = %v, want %v", tt.input, ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Errorf("ParseLanguage(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

package site

import (
	"encoding/json"
	"maps"
	"regexp"
	"slices"
	"strings"
)

type Language struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type LanguageSettings struct {
	Default string `json:"default"`
	// Fixed is true once the site has languages, when the default can't change.
	Fixed     bool       `json:"fixed"`
	Languages []Language `json:"languages"`
}

// LanguageError is a refused language change, with a code the UI translates.
type LanguageError struct {
	Code    string
	Message string
	Details map[string]string
}

func (e *LanguageError) Error() string { return e.Message }

var languageCode = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*$`)

const maxPagesNamed = 5

func (s *Site) LanguageSettings() LanguageSettings {
	table := s.languagesTable()
	out := LanguageSettings{Default: s.defaultLanguage(), Fixed: len(table) > 0}
	for _, code := range s.contentLanguages() {
		entry, _ := table[code].(map[string]any)
		name, _ := entry["languageName"].(string)
		if name == "" {
			name = code
		}
		out.Languages = append(out.Languages, Language{Code: code, Name: name})
	}
	return out
}

// SaveLanguages writes defaultContentLanguage and one languages table entry
// per language, with the list's order as weight. They go to the languages
// file when the site has one, otherwise to the main config.
func (s *Site) SaveLanguages(defaultLanguage string, languages []Language) error {
	current := s.LanguageSettings()
	codes := make([]string, len(languages))
	for index, language := range languages {
		if !languageCode.MatchString(language.Code) || slices.Contains(codes, language.Code) {
			return &LanguageError{Code: "language-code", Message: "That isn't a valid language code.", Details: map[string]string{"code": language.Code}}
		}
		codes[index] = language.Code
	}
	if !slices.Contains(codes, defaultLanguage) || (current.Fixed && defaultLanguage != current.Default) {
		return &LanguageError{Code: "language-default", Message: "The default language can't be changed or removed."}
	}
	for _, language := range current.Languages {
		if !slices.Contains(codes, language.Code) {
			if err := s.checkLanguageUnused(language.Code); err != nil {
				return err
			}
		}
	}

	table := s.languagesTable()
	weights := languageWeights(codes, table)
	entries := make(map[string]any, len(languages))
	for index, language := range languages {
		entry := map[string]any{}
		if existing, ok := table[language.Code].(map[string]any); ok {
			maps.Copy(entry, existing)
		}
		entry["languageName"] = language.Name
		entry["weight"] = weights[index]
		entries[language.Code] = entry
	}

	if !current.Fixed {
		if err := s.Config.doc.Set("defaultContentLanguage", defaultLanguage, s.Config.Format()); err != nil {
			return err
		}
	}
	if s.Languages != nil {
		for code := range table {
			if _, kept := entries[code]; !kept {
				s.Languages.doc.Delete(code)
			}
		}
		for _, code := range codes {
			if err := s.Languages.doc.Set(code, entries[code], s.Languages.Format()); err != nil {
				return err
			}
		}
		if err := s.writeConfig(s.Languages); err != nil {
			return err
		}
	} else {
		if err := s.Config.doc.Set("languages", entries, s.Config.Format()); err != nil {
			return err
		}
	}
	return s.saveConfig()
}

// languageWeights keeps the site's own weights when they already sort the
// languages in the new order, so adding a language leaves the others' lines
// alone. Otherwise the weights are 1, 2, 3 and so on.
func languageWeights(codes []string, table map[string]any) []int {
	weights := make([]int, len(codes))
	previous := 0
	for index, code := range codes {
		existing, known := table[code].(map[string]any)
		weight := toInt(existing["weight"])
		switch {
		case !known:
			weight = previous + 1
		case weight <= previous:
			for position := range weights {
				weights[position] = position + 1
			}
			return weights
		}
		weights[index] = weight
		previous = weight
	}
	return weights
}

func (s *Site) checkLanguageUnused(code string) error {
	tree, err := s.Tree()
	if err != nil {
		return err
	}
	var titles []string
	var visit func(node *Node)
	visit = func(node *Node) {
		for _, translation := range node.Translations {
			if translation.Language == code {
				titles = append(titles, node.Title)
			}
		}
		for _, child := range node.Children {
			visit(child)
		}
	}
	visit(tree)
	if len(titles) == 0 {
		return nil
	}
	shown := titles[:min(len(titles), maxPagesNamed)]
	quoted, err := json.Marshal(shown)
	if err != nil {
		return err
	}
	named := strings.Join(shown, ", ")
	if len(titles) > maxPagesNamed {
		named += ", …"
	}
	return &LanguageError{
		Code:    "language-in-use",
		Message: "Pages still use " + code + ": " + named,
		Details: map[string]string{"language": code, "pages": named, "titles": string(quoted)},
	}
}

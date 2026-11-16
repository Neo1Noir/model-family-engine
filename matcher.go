package modelfamily

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const EngineVersion = "deterministic-v2"
const maxRuleVariants = 250000
const maxIndexEntries = 1500000

type Selection struct {
	Dimension int      `json:"dimension"`
	Codes     []string `json:"codes"`
}

type Match struct {
	Model      string      `json:"canonical_model"`
	Method     string      `json:"method"`
	Selections []Selection `json:"selections,omitempty"`
	RuleIDs    []string    `json:"rule_ids,omitempty"`
	Ambiguous  bool        `json:"ambiguous,omitempty"`
}

type RuleReport struct {
	Model       string `json:"canonical_model"`
	Schema      string `json:"schema_key"`
	Fingerprint string `json:"source_fingerprint"`
	Status      string `json:"status"`
	Reason      string `json:"reason,omitempty"`
	Variants    int    `json:"variants"`
}

type Index struct {
	EngineVersion    string            `json:"engine_version"`
	Version          string            `json:"mapping_version"`
	SourceHash       string            `json:"source_hash"`
	Entries          map[string]Match  `json:"entries,omitempty"`
	RuleFingerprints map[string]string `json:"rule_fingerprints"`
	Reports          []RuleReport      `json:"reports"`
}

type matchDimensionOptions struct {
	Separator    string         `json:"separator"`
	Permutations bool           `json:"permutations"`
	Order        []string       `json:"order,omitempty"`
	MaxCount     int            `json:"max_count"`
	RepeatLimits map[string]int `json:"repeat_limits,omitempty"`
}

type matchConstraint struct {
	When    map[string][]string `json:"when,omitempty"`
	Require map[string][]string `json:"require,omitempty"`
	Forbid  map[string][]string `json:"forbid,omitempty"`
}

// This approved interpretation is separate from the unchanged manual text.
// The fingerprint binds it to the exact source rule, including its evidence.
type deterministicRule struct {
	Version           int                              `json:"version"`
	SourceFingerprint string                           `json:"source_fingerprint"`
	ReviewBasis       string                           `json:"review_basis"`
	Patterns          []string                         `json:"patterns"`
	Dimensions        map[string]matchDimensionOptions `json:"dimensions,omitempty"`
	Constraints       []matchConstraint                `json:"constraints,omitempty"`
	Suffixes          map[string][]string              `json:"suffixes_by_model,omitempty"`
}

type matchingRuleFacts struct {
	Schema string `json:"schema_key"`
	Dims   []struct {
		Type     string                     `json:"type"`
		Required *bool                      `json:"required"`
		Codes    map[string]json.RawMessage `json:"codes"`
	} `json:"dims"`
	Identity *struct {
		Dimension int               `json:"dimension"`
		Products  map[string]string `json:"products"`
	} `json:"identity_by_code,omitempty"`
	Matching *deterministicRule `json:"deterministic,omitempty"`
}

func modelMatchKey(value string) string {
	return legacyModelMatchKey(value)
}

func legacyModelMatchKey(value string) string {
	value = strings.NewReplacer("（", "(", "）", ")", "－", "-").Replace(value)
	return strings.ToUpper(strings.Join(strings.Fields(value), ""))
}

func selectionSourceFingerprint(raw json.RawMessage) (string, error) {
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	delete(value, "deterministic")
	return fingerprint(value), nil
}

func addModelMatch(entries map[string]Match, key string, entry Match) {
	if old, ok := entries[key]; ok {
		if old.Method == "conflicting_rules" || old.Model != entry.Model {
			entries[key] = Match{Ambiguous: true, Method: "conflicting_rules"}
			return
		}
		if old.Ambiguous {
			return
		}
		if old.Method == "registered_identity" && entry.Method != "registered_identity" {
			entries[key] = entry
			return
		}
		if entry.Method == "registered_identity" {
			return
		}
		if fingerprint(old.Selections) != fingerprint(entry.Selections) {
			entries[key] = Match{Model: old.Model, Ambiguous: true, Method: "conflicting_configurations"}
			return
		}
		for _, id := range entry.RuleIDs {
			if !containsModel(old.RuleIDs, id) {
				old.RuleIDs = append(old.RuleIDs, id)
			}
		}
		sort.Strings(old.RuleIDs)
		entries[key] = old
		return
	}
	entries[key] = entry
}

func matchingConditions(conditions map[string][]string, selections [][]string) bool {
	for key, codes := range conditions {
		n, _ := strconv.Atoi(key)
		if n < 1 || n > len(selections) {
			return false
		}
		found := false
		for _, code := range codes {
			found = found || containsModel(selections[n-1], code)
		}
		if !found {
			return false
		}
	}
	return true
}

func validMatchingConditions(r matchingRuleFacts) error {
	for key := range r.Matching.Dimensions {
		n, err := strconv.Atoi(key)
		if err != nil || strconv.Itoa(n) != key || n < 1 || n > len(r.Dims) || r.Dims[n-1].Type != "multi" {
			return fmt.Errorf("multi choice options refer to an invalid dimension")
		}
	}
	for _, constraint := range r.Matching.Constraints {
		for _, conditions := range []map[string][]string{constraint.When, constraint.Require, constraint.Forbid} {
			for key, codes := range conditions {
				n, err := strconv.Atoi(key)
				if err != nil || n < 1 || n > len(r.Dims) || len(codes) == 0 {
					return fmt.Errorf("constraint refers to invalid dimension")
				}
				for _, code := range codes {
					if _, ok := r.Dims[n-1].Codes[code]; !ok {
						return fmt.Errorf("constraint refers to unknown code")
					}
				}
			}
		}
	}
	return nil
}

type dimensionChoice struct {
	Text  string
	Codes []string
}

func dimensionChoices(r matchingRuleFacts, number int) ([]dimensionChoice, error) {
	dim := r.Dims[number]
	if dim.Required == nil || (dim.Type != "single" && dim.Type != "multi") || len(dim.Codes) == 0 {
		return nil, fmt.Errorf("dimension type/required/codes need review")
	}
	codes := make([]string, 0, len(dim.Codes))
	for code := range dim.Codes {
		if code == "" || len(code) > 128 {
			return nil, fmt.Errorf("invalid option code")
		}
		codes = append(codes, code)
	}
	sort.Strings(codes)
	out := []dimensionChoice{}
	if !*dim.Required {
		out = append(out, dimensionChoice{Codes: []string{}})
	}
	if dim.Type == "single" {
		for _, code := range codes {
			out = append(out, dimensionChoice{Text: code, Codes: []string{code}})
		}
		return out, nil
	}
	options, ok := r.Matching.Dimensions[strconv.Itoa(number+1)]
	if !ok || options.MaxCount < 1 || options.MaxCount > 8 {
		return nil, fmt.Errorf("multi choice order/separator/count needs review")
	}
	if !options.Permutations && len(options.Order) == 0 {
		return nil, fmt.Errorf("fixed multi choice order needs an explicit code order")
	}
	if len(options.Order) > 0 {
		seen := map[string]bool{}
		for _, code := range options.Order {
			if _, ok := dim.Codes[code]; !ok || seen[code] {
				return nil, fmt.Errorf("invalid multi choice code order")
			}
			seen[code] = true
		}
		if len(seen) != len(codes) {
			return nil, fmt.Errorf("incomplete multi choice code order")
		}
		codes = append([]string{}, options.Order...)
	}
	for code, limit := range options.RepeatLimits {
		if _, ok := dim.Codes[code]; !ok || limit < 1 || limit > options.MaxCount {
			return nil, fmt.Errorf("invalid repeated option limit")
		}
	}
	var walk func([]string, int) error
	walk = func(selected []string, from int) error {
		if len(selected) > 0 {
			normal := append([]string{}, selected...)
			sort.Strings(normal)
			out = append(out, dimensionChoice{Text: strings.Join(selected, options.Separator), Codes: normal})
			if len(out) > maxRuleVariants {
				return fmt.Errorf("dimension enumeration budget exceeded")
			}
		}
		if len(selected) == options.MaxCount {
			return nil
		}
		for i := from; i < len(codes); i++ {
			code := codes[i]
			limit := options.RepeatLimits[code]
			if limit == 0 {
				limit = 1
			}
			count := 0
			for _, value := range selected {
				if value == code {
					count++
				}
			}
			if count >= limit {
				continue
			}
			next := i
			if options.Permutations {
				next = 0
			}
			if err := walk(append(append([]string{}, selected...), code), next); err != nil {
				return err
			}
		}
		return nil
	}
	err := walk(nil, 0)
	return out, err
}

// Circled placeholders reference complete option strings. Only the separator
// immediately preceding an omitted dimension is omitted; no suffix is guessed.
func assembleModel(pattern string, choices []dimensionChoice) (string, error) {
	result, literal := "", ""
	seen := map[int]bool{}
	for _, ch := range pattern {
		if ch < '①' || ch > '⑨' {
			literal += string(ch)
			continue
		}
		n := int(ch - '①')
		if n >= len(choices) || seen[n] {
			return "", fmt.Errorf("invalid/repeated placeholder")
		}
		seen[n] = true
		if choices[n].Text == "" {
			literal = strings.TrimSuffix(strings.TrimRight(literal, " "), "-")
		}
		result += literal + choices[n].Text
		literal = ""
	}
	if len(seen) != len(choices) {
		return "", fmt.Errorf("unrepresented dimension")
	}
	return result + literal, nil
}

func compileMatchingRule(rule SelectionRule, models map[string]bool) (map[string]Match, RuleReport) {
	var facts matchingRuleFacts
	report := RuleReport{Model: rule.Model, Status: "needs_review"}
	entries := map[string]Match{}
	fail := func(reason string) (map[string]Match, RuleReport) {
		report.Reason = reason
		return map[string]Match{}, report
	}
	if err := json.Unmarshal(rule.Rule, &facts); err != nil {
		return fail("invalid rule JSON")
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(rule.Rule, &fields)
	if raw := fields["deterministic"]; len(raw) != 0 {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&facts.Matching); err != nil {
			return fail("unsupported interpretation field")
		}
	}
	report.Schema = facts.Schema
	report.Fingerprint, _ = selectionSourceFingerprint(rule.Rule)
	if rule.Status != "verified" {
		return fail("source rule is not verified")
	}
	if facts.Matching == nil || facts.Matching.Version != 1 || facts.Matching.ReviewBasis == "" || facts.Matching.SourceFingerprint != report.Fingerprint {
		return fail("approved interpretation absent or stale")
	}
	if len(facts.Matching.Patterns) == 0 || len(facts.Matching.Patterns) > 16 {
		return fail("invalid pattern count")
	}
	for model := range facts.Matching.Suffixes {
		if !models[model] {
			return fail("suffix interpretation references an unknown identity")
		}
	}
	if err := validMatchingConditions(facts); err != nil {
		return fail(err.Error())
	}
	options := make([][]dimensionChoice, len(facts.Dims))
	count := 1
	for i := range facts.Dims {
		values, err := dimensionChoices(facts, i)
		if err != nil {
			return fail(err.Error())
		}
		options[i] = values
		if len(values) == 0 || count > maxRuleVariants/len(values) {
			return fail("rule enumeration budget exceeded; nothing published")
		}
		count *= len(values)
	}
	if facts.Identity != nil {
		if facts.Identity.Dimension < 1 || facts.Identity.Dimension > len(facts.Dims) {
			return fail("invalid identity dimension")
		}
		identityDim := facts.Dims[facts.Identity.Dimension-1]
		if identityDim.Type != "single" || identityDim.Required == nil || !*identityDim.Required {
			return fail("identity must use one required code")
		}
		if len(facts.Identity.Products) != len(identityDim.Codes) {
			return fail("identity mapping must cover every declared code")
		}
		for code, model := range facts.Identity.Products {
			if _, ok := facts.Dims[facts.Identity.Dimension-1].Codes[code]; !ok || !models[model] {
				return fail("identity references unknown code/model")
			}
		}
	}
	ruleID := report.Fingerprint[:24]
	var walk func([]dimensionChoice) error
	walk = func(chosen []dimensionChoice) error {
		if len(chosen) < len(options) {
			for _, choice := range options[len(chosen)] {
				if err := walk(append(append([]dimensionChoice{}, chosen...), choice)); err != nil {
					return err
				}
			}
			return nil
		}
		selected := make([][]string, len(chosen))
		selection := make([]Selection, len(chosen))
		for i, c := range chosen {
			selected[i] = c.Codes
			selection[i] = Selection{i + 1, c.Codes}
		}
		for _, c := range facts.Matching.Constraints {
			if !matchingConditions(c.When, selected) {
				continue
			}
			if !matchingConditions(c.Require, selected) {
				return nil
			}
			for dim, codes := range c.Forbid {
				if matchingConditions(map[string][]string{dim: codes}, selected) {
					return nil
				}
			}
		}
		model := rule.Model
		if facts.Identity != nil {
			values := selected[facts.Identity.Dimension-1]
			if len(values) != 1 {
				return nil
			}
			model = facts.Identity.Products[values[0]]
			if model == "" {
				return nil
			}
		}
		if !models[model] {
			return fmt.Errorf("unknown output identity")
		}
		entry := Match{Model: model, Method: "compiled_rules", Selections: selection, RuleIDs: []string{ruleID}}
		for _, pattern := range facts.Matching.Patterns {
			value, err := assembleModel(pattern, chosen)
			if err != nil {
				return err
			}
			for _, suffix := range append([]string{""}, facts.Matching.Suffixes[model]...) {
				key := modelMatchKey(value + suffix)
				if key == "" || len(key) > 256 {
					return fmt.Errorf("invalid generated model length")
				}
				addModelMatch(entries, key, entry)
				if len(entries) > maxRuleVariants {
					return fmt.Errorf("rule enumeration budget exceeded; nothing published")
				}
			}
		}
		return nil
	}
	if err := walk(nil); err != nil {
		return fail(err.Error())
	}
	if len(entries) == 0 {
		return fail("no legal configuration")
	}
	report.Status = "compiled"
	report.Variants = len(entries)
	return entries, report
}

func Compile(rules *Rules, version string) (*Index, error) {
	if rules == nil || version == "" {
		return nil, fmt.Errorf("rules and version required")
	}
	if rules.Review.Decision != "approved" || rules.Review.Reviewer == "" || rules.Review.Time == "" || rules.Review.Basis == "" {
		return nil, fmt.Errorf("an explicit review decision is required")
	}
	index := &Index{EngineVersion: EngineVersion, Version: version, SourceHash: fingerprint([]any{rules.Models, rules.Selection}), Entries: map[string]Match{}, RuleFingerprints: map[string]string{}, Reports: []RuleReport{}}
	allowed := map[string]bool{}
	names := make([]string, 0, len(rules.Models))
	for name := range rules.Models {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		var facts struct {
			Aliases []string `json:"aliases"`
			Blocked bool     `json:"blocked"`
		}
		if err := json.Unmarshal(rules.Models[name], &facts); err != nil {
			return nil, err
		}
		if facts.Blocked {
			continue
		}
		allowed[name] = true
		for _, value := range append([]string{name}, facts.Aliases...) {
			addModelMatch(index.Entries, modelMatchKey(value), Match{Model: name, Method: "registered_identity"})
		}
	}
	for _, rule := range rules.Selection {
		if rule.Scope != "standard" || rule.Order != "" {
			return nil, fmt.Errorf("only explicitly reviewed standard-scope rules are supported")
		}
		entries, report := compileMatchingRule(rule, allowed)
		index.Reports = append(index.Reports, report)
		if report.Status != "compiled" {
			return nil, fmt.Errorf("%s: %s", report.Model, report.Reason)
		}
		if report.Status == "compiled" {
			var raw any
			_ = json.Unmarshal(rule.Rule, &raw)
			id, hash := report.Fingerprint[:24], fingerprint(raw)
			if prior := index.RuleFingerprints[id]; prior != "" && prior != hash {
				return nil, fmt.Errorf("different interpretations for the same source rule")
			}
			index.RuleFingerprints[id] = hash
		}
		for key, value := range entries {
			addModelMatch(index.Entries, key, value)
		}
		if len(index.Entries) > maxIndexEntries {
			return nil, fmt.Errorf("index budget exceeded; no partial index")
		}
	}
	if len(index.Entries) > maxIndexEntries {
		return nil, fmt.Errorf("index budget exceeded; no partial index")
	}
	return index, nil
}

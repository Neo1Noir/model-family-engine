package modelfamily

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Rules is an explicitly reviewed set of product identities and option tables.
type Rules struct {
	Models    map[string]json.RawMessage `json:"models"`
	Selection []SelectionRule            `json:"selection_rules"`
	Review    Review                     `json:"review"`
}

type Review struct {
	Decision string `json:"decision"`
	Reviewer string `json:"reviewer"`
	Time     string `json:"reviewed_at"`
	Basis    string `json:"basis"`
}

type SelectionRule struct {
	Model  string          `json:"canonical_model"`
	Scope  string          `json:"document_scope"`
	Order  string          `json:"custom_order,omitempty"`
	Status string          `json:"verification_status"`
	Rule   json.RawMessage `json:"rule_json"`
}

func fingerprint(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func containsModel(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

// Approve records an explicit review decision and binds each interpretation to
// the exact source rule. It does not verify the identity of the reviewer.
func Approve(candidate *Rules, reviewer, basis string) (*Rules, error) {
	if candidate == nil || strings.TrimSpace(reviewer) == "" || strings.TrimSpace(basis) == "" {
		return nil, fmt.Errorf("candidate, reviewer, and review basis are required")
	}
	data, err := json.Marshal(candidate)
	if err != nil {
		return nil, err
	}
	var reviewed Rules
	if err = json.Unmarshal(data, &reviewed); err != nil {
		return nil, err
	}
	if len(reviewed.Models) == 0 || len(reviewed.Selection) == 0 {
		return nil, fmt.Errorf("candidate needs registered models and selection rules")
	}
	for name, raw := range reviewed.Models {
		var facts struct {
			Family string `json:"family"`
		}
		if strings.TrimSpace(name) == "" || json.Unmarshal(raw, &facts) != nil || facts.Family == "" {
			return nil, fmt.Errorf("model identity and family require explicit review")
		}
	}
	for i := range reviewed.Selection {
		rule := &reviewed.Selection[i]
		var value map[string]any
		if err = json.Unmarshal(rule.Rule, &value); err != nil {
			return nil, err
		}
		sha, _ := value["source_sha256"].(string)
		decoded, e := hex.DecodeString(sha)
		if e != nil || len(decoded) != 32 {
			return nil, fmt.Errorf("source SHA-256 is required")
		}
		pages, ok := value["physical_pages"].([]any)
		if !ok || len(pages) == 0 {
			return nil, fmt.Errorf("source physical pages are required")
		}
		for _, page := range pages {
			n, ok := page.(float64)
			if !ok || n < 1 || n != float64(int(n)) {
				return nil, fmt.Errorf("invalid physical page")
			}
		}
		quotes, ok := value["source_excerpts"].([]any)
		if !ok || len(quotes) == 0 {
			return nil, fmt.Errorf("source excerpts are required")
		}
		for _, quote := range quotes {
			if s, ok := quote.(string); !ok || strings.TrimSpace(s) == "" {
				return nil, fmt.Errorf("empty source excerpt")
			}
		}
		interpretation, ok := value["deterministic"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("deterministic interpretation is required")
		}
		if rule.Scope != "standard" || rule.Order != "" {
			return nil, fmt.Errorf("only standard-scope rules can be approved here")
		}
		sourceHash, err := selectionSourceFingerprint(rule.Rule)
		if err != nil {
			return nil, err
		}
		interpretation["source_fingerprint"] = sourceHash
		interpretation["review_basis"] = basis
		rule.Rule, err = json.Marshal(value)
		if err != nil {
			return nil, err
		}
		rule.Status = "verified"
	}
	reviewed.Review = Review{"approved", reviewer, time.Now().UTC().Format(time.RFC3339), basis}
	if _, err = Compile(&reviewed, "review-validation"); err != nil {
		return nil, err
	}
	return &reviewed, nil
}

type Resolution struct {
	Status  string `json:"status"`
	Input   string `json:"input"`
	Version string `json:"mapping_version"`
	Match   *Match `json:"match,omitempty"`
}

// Resolve performs a lookup only. It never calls a model or guesses a prefix.
func (index *Index) Resolve(input string) Resolution {
	result := Resolution{Status: "not_found", Input: input, Version: index.Version}
	if strings.TrimSpace(input) == "" || len(input) > 256 {
		result.Status = "invalid_request"
		return result
	}
	if entry, ok := index.Entries[modelMatchKey(input)]; ok {
		result.Match = &entry
		result.Status = "resolved"
		if entry.Ambiguous {
			result.Status = "needs_clarification"
		}
	}
	return result
}

package sessions

import (
	"context"
	"fmt"
	"regexp"
	"sort"
)

// WorkRefRule names a pattern that identifies one kind of work reference.
type WorkRefRule struct {
	// Name is the rule's name and the Kind of every reference it matches, such as rmi.
	Name string `json:"name"`
	// Pattern is a regular expression that matches the identifier as written.
	Pattern string `json:"pattern"`
}

// DefaultWorkRefRules matches initiative IDs (INIT-<SLUG>-NNN) and
// roadmap-item IDs (RMI-<REPOSLUG>-NNN). Slugs are upper-case letters and
// digits, and the number has at least three digits.
func DefaultWorkRefRules() []WorkRefRule {
	return []WorkRefRule{
		{Name: "initiative", Pattern: `\bINIT-[A-Z0-9]+-\d{3,}\b`},
		{Name: "rmi", Pattern: `\bRMI-[A-Z0-9]+-\d{3,}\b`},
	}
}

type compiledRule struct {
	name string
	re   *regexp.Regexp
}

// WorkRefExtractor finds work references in text using named rules.
type WorkRefExtractor struct {
	rules []compiledRule
}

// NewWorkRefExtractor compiles the rules. It returns an error for an empty
// or duplicate name or a pattern that does not compile, so a bad
// configuration fails when loaded rather than silently matching nothing.
func NewWorkRefExtractor(rules []WorkRefRule) (*WorkRefExtractor, error) {
	seen := map[string]bool{}
	compiled := make([]compiledRule, 0, len(rules))
	for i, r := range rules {
		if r.Name == "" {
			return nil, fmt.Errorf("work reference rule %d has no name", i)
		}
		if seen[r.Name] {
			return nil, fmt.Errorf("duplicate work reference rule name %q", r.Name)
		}
		seen[r.Name] = true
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			return nil, fmt.Errorf("work reference rule %q: %w", r.Name, err)
		}
		compiled = append(compiled, compiledRule{name: r.Name, re: re})
	}
	return &WorkRefExtractor{rules: compiled}, nil
}

// NewSet returns an empty accumulator that merges references found in
// different places.
func (e *WorkRefExtractor) NewSet() *WorkRefSet {
	return &WorkRefSet{ext: e, byID: map[string]*workRefEntry{}}
}

type workRefEntry struct {
	kind    string
	sources map[WorkRefSource]bool
}

// WorkRefSet accumulates work references. The zero value is not usable; use
// WorkRefExtractor.NewSet.
type WorkRefSet struct {
	ext  *WorkRefExtractor
	byID map[string]*workRefEntry
}

// Add scans text and records every reference it finds, noting where it was
// found. When rules overlap, the first rule that matches an identifier names
// its kind. Repeated sightings merge: an identifier keeps one entry and
// gains each source it appears in.
func (s *WorkRefSet) Add(source WorkRefSource, text string) {
	for _, rule := range s.ext.rules {
		for _, id := range rule.re.FindAllString(text, -1) {
			e, ok := s.byID[id]
			if !ok {
				e = &workRefEntry{kind: rule.name, sources: map[WorkRefSource]bool{}}
				s.byID[id] = e
			}
			e.sources[source] = true
		}
	}
}

// sourceOrder is the order sources are listed in, so output is stable.
var sourceOrder = []WorkRefSource{WorkRefPrompt, WorkRefBranch, WorkRefCommit, WorkRefPath}

// Refs returns the accumulated references sorted by identifier, each with
// its sources in a fixed order.
func (s *WorkRefSet) Refs() []WorkRef {
	ids := make([]string, 0, len(s.byID))
	for id := range s.byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]WorkRef, 0, len(ids))
	for _, id := range ids {
		e := s.byID[id]
		ref := WorkRef{ID: id, Kind: e.kind}
		for _, src := range sourceOrder {
			if e.sources[src] {
				ref.Sources = append(ref.Sources, src)
			}
		}
		out = append(out, ref)
	}
	return out
}

// WorkRefResolver supplies titles for work references, for example from a
// work-tracking system. Nothing implements it by default.
type WorkRefResolver interface {
	// Titles returns the title of each identifier it knows. Unknown
	// identifiers are left out of the result rather than treated as errors.
	Titles(ctx context.Context, ids []string) (map[string]string, error)
}

// ResolveTitles fills in Title on each reference the resolver knows. On a
// resolver error it returns the references unchanged along with the error,
// so a lookup failure never loses the references themselves.
func ResolveTitles(ctx context.Context, refs []WorkRef, r WorkRefResolver) ([]WorkRef, error) {
	if r == nil || len(refs) == 0 {
		return refs, nil
	}
	ids := make([]string, len(refs))
	for i, ref := range refs {
		ids[i] = ref.ID
	}
	titles, err := r.Titles(ctx, ids)
	if err != nil {
		return refs, fmt.Errorf("resolve work reference titles: %w", err)
	}
	out := make([]WorkRef, len(refs))
	copy(out, refs)
	for i := range out {
		if t, ok := titles[out[i].ID]; ok {
			out[i].Title = t
		}
	}
	return out, nil
}

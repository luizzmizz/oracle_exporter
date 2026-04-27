package collector

import (
	"fmt"
	"regexp"
)

// Filter applies include/exclude regex lists to a metric name.
// If include patterns are set, a name must match at least one to pass.
// If exclude patterns are set, a name matching any of them is dropped.
// Exclude is evaluated after include.
type Filter struct {
	include []*regexp.Regexp
	exclude []*regexp.Regexp
}

func NewFilter(include, exclude []string) (*Filter, error) {
	f := &Filter{}
	for _, p := range include {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("invalid include pattern %q: %w", p, err)
		}
		f.include = append(f.include, re)
	}
	for _, p := range exclude {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("invalid exclude pattern %q: %w", p, err)
		}
		f.exclude = append(f.exclude, re)
	}
	return f, nil
}

func (f *Filter) Allow(name string) bool {
	if len(f.include) > 0 {
		matched := false
		for _, re := range f.include {
			if re.MatchString(name) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	for _, re := range f.exclude {
		if re.MatchString(name) {
			return false
		}
	}
	return true
}

// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command changelog classifies stdin subjects using the release configuration.
package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

type rule struct {
	title      string
	expression *regexp.Regexp
}

// Read the single-quoted expressions verbatim: YAML single quotes preserve backslashes.
func readRules(b []byte) ([]rule, []*regexp.Regexp, error) {
	var groups []rule
	var excludes []*regexp.Regexp
	title := ""
	filtering := false
	for line := range strings.SplitSeq(string(b), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- title: ") {
			title = strings.TrimPrefix(line, "- title: ")
		}
		if line == "exclude:" {
			filtering = true
		}
		var pattern string
		if strings.HasPrefix(line, "regexp: '") {
			pattern = strings.TrimSuffix(strings.TrimPrefix(line, "regexp: '"), "'")
		} else if filtering && strings.HasPrefix(line, "- ") {
			pattern = strings.Trim(strings.TrimPrefix(line, "- "), "'")
		} else {
			continue
		}
		rx, err := regexp.Compile(pattern)
		if err != nil {
			return nil, nil, err
		}
		if filtering {
			excludes = append(excludes, rx)
		} else {
			groups = append(groups, rule{title, rx})
		}
	}
	return groups, excludes, nil
}

func classify(subject string, groups []rule, excludes []*regexp.Regexp) string {
	for _, rx := range excludes {
		if rx.MatchString(subject) {
			return "Excluded"
		}
	}
	for _, group := range groups {
		if group.expression.MatchString(subject) {
			return group.title
		}
	}
	return "Other"
}

func main() {
	configuration, err := os.ReadFile(".goreleaser.yml")
	if err != nil {
		panic(err)
	}
	groups, excludes, err := readRules(configuration)
	if err != nil {
		panic(err)
	}
	allowed := regexp.MustCompile(`^(docs|ci|chore|test)(\([^)]*\))?!?:`)
	merges := regexp.MustCompile(`^Merge (branch|tag|pull request|remote-tracking branch) `)
	scanner := bufio.NewScanner(os.Stdin)
	failed := false
	for scanner.Scan() {
		subject := scanner.Text()
		group := classify(subject, groups, excludes)
		fmt.Printf("%s | %s\n", group, subject)
		if (group == "Other" || group == "Excluded") && !allowed.MatchString(subject) && !(group == "Excluded" && merges.MatchString(subject)) {
			failed = true
		}
	}
	if scanner.Err() != nil || failed {
		os.Exit(1)
	}
}

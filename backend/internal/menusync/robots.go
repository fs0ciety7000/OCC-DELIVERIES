package menusync

import (
	"bufio"
	"bytes"
	"regexp"
	"strings"
)

// Robots is a parsed robots.txt, reduced to the group that applies to us.
// Matching follows RFC 9309: longest matching rule wins, Allow wins ties,
// "*" matches any sequence and a trailing "$" anchors the end.
type Robots struct {
	rules []robotsRule
}

type robotsRule struct {
	allow   bool
	pattern string
	re      *regexp.Regexp
}

// ParseRobots parses robots.txt content for the given product token (e.g.
// "OCC-Deliveries-menusync"). The most specific group wins: a group naming
// our token, otherwise the "*" group. A body that is not a robots.txt (an
// HTML page served by a SPA fallback) yields no rules, i.e. everything allowed.
func ParseRobots(body []byte, token string) *Robots {
	token = strings.ToLower(token)
	type group struct {
		agents []string
		rules  []robotsRule
	}
	var groups []*group
	var cur *group
	lastWasAgent := false

	sc := bufio.NewScanner(bytes.NewReader(body))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		key, val, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		val = strings.TrimSpace(val)
		switch key {
		case "user-agent":
			if cur == nil || !lastWasAgent {
				cur = &group{}
				groups = append(groups, cur)
			}
			cur.agents = append(cur.agents, strings.ToLower(val))
			lastWasAgent = true
		case "allow", "disallow":
			lastWasAgent = false
			if cur == nil || val == "" {
				continue // empty Disallow = allow everything
			}
			cur.rules = append(cur.rules, robotsRule{allow: key == "allow", pattern: val, re: robotsPattern(val)})
		default:
			lastWasAgent = false
		}
	}

	var star, mine *group
	for _, g := range groups {
		for _, a := range g.agents {
			if a == "*" && star == nil {
				star = g
			}
			if a != "*" && a != "" && token != "" && strings.Contains(token, a) && mine == nil {
				mine = g
			}
		}
	}
	r := &Robots{}
	switch {
	case mine != nil:
		r.rules = mine.rules
	case star != nil:
		r.rules = star.rules
	}
	return r
}

func robotsPattern(p string) *regexp.Regexp {
	anchored := strings.HasSuffix(p, "$")
	p = strings.TrimSuffix(p, "$")
	if !strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "*") {
		// rules like "apply/confirm?" are relative: match anywhere
		p = "*" + p
	}
	var b strings.Builder
	b.WriteString("^")
	for i, part := range strings.Split(p, "*") {
		if i > 0 {
			b.WriteString(".*")
		}
		b.WriteString(regexp.QuoteMeta(part))
	}
	if anchored {
		b.WriteString("$")
	}
	return regexp.MustCompile(b.String())
}

// Allowed reports whether a path (with its query string) may be fetched.
func (r *Robots) Allowed(pathAndQuery string) bool {
	if r == nil {
		return true
	}
	if pathAndQuery == "" {
		pathAndQuery = "/"
	}
	best, allowed := -1, true
	for _, rule := range r.rules {
		if !rule.re.MatchString(pathAndQuery) {
			continue
		}
		l := len(rule.pattern)
		if l > best || (l == best && rule.allow) {
			best, allowed = l, rule.allow
		}
	}
	return allowed
}

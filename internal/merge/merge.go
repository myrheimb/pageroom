// Package merge implements a line-based three-way merge (diff3), so two
// people editing different parts of a page are merged automatically and only
// real overlaps reach the user.
package merge

import (
	"strings"
)

// Hunk is one region of a merge result: either clean (agreed text) or a
// conflict that a person has to decide.
type Hunk struct {
	Conflict bool     `json:"conflict"`
	Lines    []string `json:"lines,omitempty"`  // clean hunk
	Base     []string `json:"base,omitempty"`   // conflict: common ancestor
	Ours     []string `json:"ours,omitempty"`   // conflict: this computer
	Theirs   []string `json:"theirs,omitempty"` // conflict: the other person
}

type Result struct {
	Hunks []Hunk `json:"hunks"`
}

func (r Result) HasConflicts() bool {
	for _, h := range r.Hunks {
		if h.Conflict {
			return true
		}
	}
	return false
}

// Resolution says which side to keep for a conflicting hunk.
type Resolution string

const (
	KeepOurs   Resolution = "ours"
	KeepTheirs Resolution = "theirs"
	KeepBoth   Resolution = "both"
)

// Text renders the merged text. Conflicts are resolved with choices[i] for
// the i-th conflict (defaulting to ours).
func (r Result) Text(choices []Resolution) string {
	var out []string
	ci := 0
	for _, h := range r.Hunks {
		if !h.Conflict {
			out = append(out, h.Lines...)
			continue
		}
		choice := KeepOurs
		if ci < len(choices) && choices[ci] != "" {
			choice = choices[ci]
		}
		ci++
		switch choice {
		case KeepTheirs:
			out = append(out, h.Theirs...)
		case KeepBoth:
			out = append(out, h.Ours...)
			if len(h.Ours) > 0 && len(h.Theirs) > 0 && h.Ours[len(h.Ours)-1] != "" {
				out = append(out, "") // keep the two versions as separate paragraphs
			}
			out = append(out, h.Theirs...)
		default:
			out = append(out, h.Ours...)
		}
	}
	return strings.Join(out, "\n")
}

func Merge(base, ours, theirs string) Result {
	b, o, t := split(base), split(ours), split(theirs)
	mo := matchMap(b, o)
	mt := matchMap(b, t)

	var res Result
	appendClean := func(lines []string) {
		if len(lines) == 0 {
			return
		}
		if n := len(res.Hunks); n > 0 && !res.Hunks[n-1].Conflict {
			res.Hunks[n-1].Lines = append(res.Hunks[n-1].Lines, lines...)
			return
		}
		res.Hunks = append(res.Hunks, Hunk{Lines: append([]string(nil), lines...)})
	}

	bi, oi, ti := 0, 0, 0
	for bi < len(b) || oi < len(o) || ti < len(t) {
		// Advance through lines that are stable in all three.
		if bi < len(b) && mo[bi] == oi && mt[bi] == ti {
			appendClean([]string{b[bi]})
			bi, oi, ti = bi+1, oi+1, ti+1
			continue
		}
		// Find the next base line that is matched in both and comes after
		// the current positions: that is where the unstable region ends.
		nb := bi
		for nb < len(b) && !(mo[nb] >= oi && mt[nb] >= ti) {
			nb++
		}
		no, nt := len(o), len(t)
		if nb < len(b) {
			no, nt = mo[nb], mt[nb]
		}
		baseChunk, oursChunk, theirsChunk := b[bi:nb], o[oi:no], t[ti:nt]
		oursChanged := !equal(baseChunk, oursChunk)
		theirsChanged := !equal(baseChunk, theirsChunk)
		switch {
		case !oursChanged:
			appendClean(theirsChunk)
		case !theirsChanged:
			appendClean(oursChunk)
		case equal(oursChunk, theirsChunk):
			appendClean(oursChunk) // both made the same change
		default:
			res.Hunks = append(res.Hunks, Hunk{
				Conflict: true,
				Base:     append([]string(nil), baseChunk...),
				Ours:     append([]string(nil), oursChunk...),
				Theirs:   append([]string(nil), theirsChunk...),
			})
		}
		bi, oi, ti = nb, no, nt
	}
	return res
}

func split(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// matchMap returns, for each line of a, the index of the matching line in b
// according to their longest common subsequence, or -1 if unmatched.
func matchMap(a, b []string) []int {
	m := make([]int, len(a))
	for i := range m {
		m[i] = -1
	}
	// Trim common prefix/suffix first: cheap and covers most edits.
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		m[pre] = pre
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		m[len(a)-1-suf] = len(b) - 1 - suf
		suf++
	}
	ma, mb := a[pre:len(a)-suf], b[pre:len(b)-suf]
	for i, j := range lcs(ma, mb) {
		if j >= 0 {
			m[pre+i] = pre + j
		}
	}
	return m
}

// lcs is a plain O(n*m) LCS, fine for small content files. Very large inputs
// match nothing, which gives one big conflict instead of a slow merge.
func lcs(a, b []string) []int {
	res := make([]int, len(a))
	for i := range res {
		res[i] = -1
	}
	if len(a) == 0 || len(b) == 0 || len(a)*len(b) > 25_000_000 {
		return res
	}
	n, m := len(a), len(b)
	dp := make([][]int32, n+1)
	for i := range dp {
		dp[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	for i, j := 0, 0; i < n && j < m; {
		switch {
		case a[i] == b[j]:
			res[i] = j
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			i++
		default:
			j++
		}
	}
	return res
}

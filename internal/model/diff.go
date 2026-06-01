// internal/model/diff.go
package model

import "strings"

// DiffOp is one line in a structured diff. Kind is "context", "del", or "add".
// Deleted lines precede their added counterparts (unified-diff convention).
type DiffOp struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// DiffLines computes a line-level diff between oldStr and newStr using the
// classic longest-common-subsequence algorithm. Both inputs are split on "\n".
// The result is a flat list of context/del/add ops in display order.
func DiffLines(oldStr, newStr string) []DiffOp {
	a := strings.Split(oldStr, "\n")
	b := strings.Split(newStr, "\n")

	// LCS length table: lcs[i][j] = LCS of a[i:] and b[j:].
	n, m := len(a), len(b)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	var ops []DiffOp
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, DiffOp{Kind: "context", Text: a[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			ops = append(ops, DiffOp{Kind: "del", Text: a[i]})
			i++
		default:
			ops = append(ops, DiffOp{Kind: "add", Text: b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, DiffOp{Kind: "del", Text: a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, DiffOp{Kind: "add", Text: b[j]})
	}
	return ops
}

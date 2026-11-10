package shadow

import "sort"

// Opcode is one step turning a into b, as in Python's difflib: Tag is "equal", "replace",
// "delete" or "insert", and a[I1:I2] corresponds to b[J1:J2].
type Opcode struct {
	Tag            string
	I1, I2, J1, J2 int
}

type match struct{ i, j, size int }

// Opcodes ports Python's difflib.SequenceMatcher(None, a, b, autojunk=False).get_opcodes(), so
// scores match the earlier Python implementation exactly.
func Opcodes[T comparable](a, b []T) []Opcode {
	b2j := map[T][]int{}
	for j, x := range b {
		b2j[x] = append(b2j[x], j)
	}

	findLongestMatch := func(alo, ahi, blo, bhi int) match {
		besti, bestj, bestsize := alo, blo, 0
		j2len := map[int]int{}
		for i := alo; i < ahi; i++ {
			newj2len := map[int]int{}
			for _, j := range b2j[a[i]] {
				if j < blo {
					continue
				}
				if j >= bhi {
					break
				}
				k := j2len[j-1] + 1
				newj2len[j] = k
				if k > bestsize {
					besti, bestj, bestsize = i-k+1, j-k+1, k
				}
			}
			j2len = newj2len
		}
		for besti > alo && bestj > blo && a[besti-1] == b[bestj-1] {
			besti, bestj, bestsize = besti-1, bestj-1, bestsize+1
		}
		for besti+bestsize < ahi && bestj+bestsize < bhi && a[besti+bestsize] == b[bestj+bestsize] {
			bestsize++
		}
		return match{besti, bestj, bestsize}
	}

	// get_matching_blocks
	type span struct{ alo, ahi, blo, bhi int }
	queue := []span{{0, len(a), 0, len(b)}}
	var blocks []match
	for len(queue) > 0 {
		q := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		m := findLongestMatch(q.alo, q.ahi, q.blo, q.bhi)
		if m.size == 0 {
			continue
		}
		blocks = append(blocks, m)
		if q.alo < m.i && q.blo < m.j {
			queue = append(queue, span{q.alo, m.i, q.blo, m.j})
		}
		if m.i+m.size < q.ahi && m.j+m.size < q.bhi {
			queue = append(queue, span{m.i + m.size, q.ahi, m.j + m.size, q.bhi})
		}
	}
	sort.Slice(blocks, func(x, y int) bool {
		if blocks[x].i != blocks[y].i {
			return blocks[x].i < blocks[y].i
		}
		if blocks[x].j != blocks[y].j {
			return blocks[x].j < blocks[y].j
		}
		return blocks[x].size < blocks[y].size
	})
	var collapsed []match
	cur := match{}
	for _, m := range blocks {
		if cur.i+cur.size == m.i && cur.j+cur.size == m.j {
			cur.size += m.size
			continue
		}
		if cur.size > 0 {
			collapsed = append(collapsed, cur)
		}
		cur = m
	}
	if cur.size > 0 {
		collapsed = append(collapsed, cur)
	}
	collapsed = append(collapsed, match{len(a), len(b), 0})

	// get_opcodes
	var ops []Opcode
	i, j := 0, 0
	for _, m := range collapsed {
		tag := ""
		switch {
		case i < m.i && j < m.j:
			tag = "replace"
		case i < m.i:
			tag = "delete"
		case j < m.j:
			tag = "insert"
		}
		if tag != "" {
			ops = append(ops, Opcode{tag, i, m.i, j, m.j})
		}
		i, j = m.i+m.size, m.j+m.size
		if m.size > 0 {
			ops = append(ops, Opcode{"equal", m.i, i, m.j, j})
		}
	}
	return ops
}

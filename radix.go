package radix

import (
	"fmt"
	"sort"
	"strings"
)

// WalkFn is used when walking the tree. Takes a
// key and value, returning if iteration should
// be terminated.
type WalkFn func(s string, v interface{}) bool

// leafNode is used to represent a value
type leafNode struct {
	val interface{}
}

// Value returns value stored in node
func (l *leafNode) Value() interface{} {
	return l.val
}

// Edge is used to represent an Edge node
type Edge struct {
	label byte
	node  *Node
}

// Label return label for Edge
func (e *Edge) Label() byte {
	return e.label
}

// Node return node which this Edge connects to
func (e *Edge) Node() *Node {
	return e.node
}

type Node struct {
	// leaf is used to store possible leaf
	leaf *leafNode

	// prefix is the common prefix we ignore
	prefix string

	// Edges should be stored in-order for iteration.
	// We avoid a fully materialized slice to save memory,
	// since in most cases we expect to be sparse
	edges Edges
}

func (n *Node) Prefix() string {
	return n.prefix
}

func (n *Node) Edges() Edges {
	return n.edges
}

func (n *Node) HasValue() bool {
	return n.leaf != nil
}

func (n *Node) Value() interface{} {
	if !n.HasValue() {
		return nil
	}

	return n.leaf.Value()
}

func (n *Node) addEdge(e Edge) {
	n.edges = append(n.edges, e)
	n.edges.Sort()
}

func (n *Node) updateEdge(label byte, node *Node) {
	num := len(n.edges)
	idx := sort.Search(num, func(i int) bool {
		return n.edges[i].label >= label
	})
	if idx < num && n.edges[idx].label == label {
		n.edges[idx].node = node
		return
	}
	panic("replacing missing Edge")
}

func (n *Node) getEdge(label byte) *Node {
	left := 0
	right := len(n.edges) - 1

	for left <= right {
		i := (left + right) / 2
		edgeLabel := n.edges[i].label

		if edgeLabel < label {
			left = i + 1
		} else if edgeLabel > label {
			right = i - 1
		} else {
			return n.edges[i].node
		}
	}

	return nil
}

func (n *Node) delEdge(label byte) {
	num := len(n.edges)
	idx := sort.Search(num, func(i int) bool {
		return n.edges[i].label >= label
	})
	if idx < num && n.edges[idx].label == label {
		copy(n.edges[idx:], n.edges[idx+1:])
		n.edges[len(n.edges)-1] = Edge{}
		n.edges = n.edges[:len(n.edges)-1]
	}
}

type Edges []Edge

func (e Edges) Len() int {
	return len(e)
}

func (e Edges) Less(i, j int) bool {
	return e[i].label < e[j].label
}

func (e Edges) Swap(i, j int) {
	e[i], e[j] = e[j], e[i]
}

func (e Edges) Sort() {
	sort.Sort(e)
}

// Tree implements a radix tree. This can be treated as a
// Dictionary abstract data type. The main advantage over
// a standard hash map is prefix-based lookups and
// ordered iteration,
type Tree struct {
	root *Node
	size int
}

// New returns an empty Tree
func New() *Tree {
	return NewFromMap(nil)
}

// NewFromMap returns a new tree containing the keys
// from an existing map
func NewFromMap(m map[string]interface{}) *Tree {
	t := &Tree{root: &Node{}}
	for k, v := range m {
		t.Insert(k, v)
	}
	return t
}

// Len is used to return the number of elements in the tree
func (t *Tree) Len() int {
	return t.size
}

// longestPrefix finds the length of the shared prefix
// of two strings
func longestPrefix(k1, k2 string) int {
	max := len(k1)
	if l := len(k2); l < max {
		max = l
	}
	var i int
	for i = 0; i < max; i++ {
		if k1[i] != k2[i] {
			break
		}
	}
	return i
}

// Insert is used to add a newentry or update
// an existing entry. Returns if updated.
func (t *Tree) Insert(s string, v interface{}) (interface{}, bool) {
	var parent *Node
	n := t.root
	search := s
	for {
		// Handle key exhaution
		if len(search) == 0 {
			if n.HasValue() {
				old := n.leaf.val
				n.leaf.val = v
				return old, true
			}

			n.leaf = &leafNode{
				val: v,
			}
			t.size++
			return nil, false
		}

		// Look for the Edge
		parent = n
		n = n.getEdge(search[0])

		// No Edge, create one
		if n == nil {
			e := Edge{
				label: search[0],
				node: &Node{
					leaf: &leafNode{
						val: v,
					},
					prefix: search,
				},
			}
			parent.addEdge(e)
			t.size++
			return nil, false
		}

		// Determine longest prefix of the search key on match
		commonPrefix := longestPrefix(search, n.prefix)
		if commonPrefix == len(n.prefix) {
			search = search[commonPrefix:]
			continue
		}

		// Split the node
		t.size++
		child := &Node{
			prefix: search[:commonPrefix],
		}
		parent.updateEdge(search[0], child)

		// Restore the existing node
		child.addEdge(Edge{
			label: n.prefix[commonPrefix],
			node:  n,
		})
		n.prefix = n.prefix[commonPrefix:]

		// Create a new leaf node
		leaf := &leafNode{
			val: v,
		}

		// If the new key is a subset, add to to this node
		search = search[commonPrefix:]
		if len(search) == 0 {
			child.leaf = leaf
			return nil, false
		}

		// Create a new Edge for the node
		child.addEdge(Edge{
			label: search[0],
			node: &Node{
				leaf:   leaf,
				prefix: search,
			},
		})
		return nil, false
	}
}

// Delete is used to delete a key, returning the previous
// value and if it was deleted
func (t *Tree) Delete(s string) (interface{}, bool) {
	var parent *Node
	var label byte
	n := t.root
	search := s
	for {
		// Check for key exhaution
		if len(search) == 0 {
			if !n.HasValue() {
				break
			}
			goto DELETE
		}

		// Look for an Edge
		parent = n
		label = search[0]
		n = n.getEdge(label)
		if n == nil {
			break
		}

		// Consume the search prefix
		if strings.HasPrefix(search, n.prefix) {
			search = search[len(n.prefix):]
		} else {
			break
		}
	}
	return 0, false

DELETE:
	// Delete the leaf
	leaf := n.leaf
	n.leaf = nil
	t.size--

	// Check if we should delete this node from the parent
	if parent != nil && len(n.edges) == 0 {
		parent.delEdge(label)
	}

	// Check if we should merge this node
	if n != t.root && len(n.edges) == 1 {
		n.mergeChild()
	}

	// Check if we should merge the parent's other child
	if parent != nil && parent != t.root && len(parent.edges) == 1 && !parent.HasValue() {
		parent.mergeChild()
	}

	return leaf.val, true
}

// DeletePrefix is used to delete the subtree under a prefix
// Returns how many nodes were deleted
// Use this to delete large subtrees efficiently
func (t *Tree) DeletePrefix(s string) int {
	return t.deletePrefix(nil, t.root, s)
}

// delete does a recursive deletion
func (t *Tree) deletePrefix(parent, n *Node, prefix string) int {
	// Check for key exhaustion
	if len(prefix) == 0 {
		// Remove the leaf node
		subTreeSize := 0
		//recursively walk from all Edges of the node to be deleted
		recursiveWalk(prefix, n, func(s string, v interface{}) bool {
			subTreeSize++
			return false
		})
		if n.HasValue() {
			n.leaf = nil
		}
		n.edges = nil // deletes the entire subtree

		// Check if we should merge the parent's other child
		if parent != nil && parent != t.root && len(parent.edges) == 1 && !parent.HasValue() {
			parent.mergeChild()
		}
		t.size -= subTreeSize
		return subTreeSize
	}

	// Look for an Edge
	label := prefix[0]
	child := n.getEdge(label)
	if child == nil || (!strings.HasPrefix(child.prefix, prefix) && !strings.HasPrefix(prefix, child.prefix)) {
		return 0
	}

	// Consume the search prefix
	if len(child.prefix) > len(prefix) {
		prefix = prefix[len(prefix):]
	} else {
		prefix = prefix[len(child.prefix):]
	}
	return t.deletePrefix(n, child, prefix)
}

func (n *Node) mergeChild() {
	e := n.edges[0]
	child := e.node
	n.prefix = n.prefix + child.prefix
	n.leaf = child.leaf
	n.edges = child.edges
}

// Root returns root node of tree
func (t *Tree) Root() *Node {
	return t.root
}

// Find find key in tree
func (t *Tree) Find(parent *Node, s string) (isFound bool, prefixLen int, lastLeafNode *Node, lastNode *Node) {
	n := parent
	search := s
	for {
		// Check for key exhaution
		if len(search) == 0 {
			break
		}

		if n.HasValue() {
			lastLeafNode = n
		}

		// Look for an Edge
		child := n.getEdge(search[0])
		if child == nil {
			break
		}

		n = child

		// Consume the search prefix
		if strings.HasPrefix(search, n.prefix) {
			search = search[len(n.prefix):]
		} else {
			break
		}
	}

	return len(search) == 0, len(s) - len(search), lastLeafNode, n
}

// Get is used to lookup a specific key, returning
// the value and if it was found
func (t *Tree) Get(s string) (interface{}, bool) {
	isFound, _, _, lastNode := t.Find(t.Root(), s)
	if !isFound {
		return 0, false
	}

	return lastNode.Value(), true
}

// LongestPrefix is like Get, but instead of an
// exact match, it will return the longest prefix match.
func (t *Tree) LongestPrefix(s string) (string, interface{}, bool) {
	_, prefixLen, _, lastLeafNode := t.Find(t.Root(), s)

	return s[:prefixLen], lastLeafNode.Value(), true
}

// Minimum is used to return the minimum value in the tree
func (t *Tree) Minimum() (string, interface{}, bool) {
	n := t.root
	prefix := ""
	for {
		if n.HasValue() {
			return prefix, n.leaf.val, true
		}
		if len(n.edges) > 0 {
			n = n.edges[0].node
			prefix += n.prefix
		} else {
			break
		}
	}
	return "", nil, false
}

// Maximum is used to return the maximum value in the tree
func (t *Tree) Maximum() (string, interface{}, bool) {
	n := t.root
	prefix := ""
	for {
		if num := len(n.edges); num > 0 {
			n = n.edges[num-1].node
			prefix += n.prefix
			continue
		}
		if n.HasValue() {
			return prefix, n.leaf.val, true
		}
		break
	}
	return "", nil, false
}

// Walk is used to walk the tree
func (t *Tree) Walk(parent *Node, prefix string, fn WalkFn) {
	recursiveWalk(prefix, parent, fn)
}

// WalkPrefix is used to walk the tree under a prefix
func (t *Tree) WalkPrefix(prefix string, fn WalkFn) {
	n := t.root
	search := prefix
	lcp := ""
	for {
		// Check for key exhaution
		if len(search) == 0 {
			recursiveWalk(lcp, n, fn)
			return
		}

		lcp += n.prefix

		// Look for an Edge
		n = n.getEdge(search[0])
		if n == nil {
			break
		}

		// Consume the search prefix
		if strings.HasPrefix(search, n.prefix) {
			search = search[len(n.prefix):]
		} else if strings.HasPrefix(n.prefix, search) {
			// Child may be under our search prefix
			recursiveWalk(lcp, n, fn)
			return
		} else {
			break
		}
	}

}

// WalkPath is used to walk the tree, but only visiting nodes
// from the root down to a given leaf. Where WalkPrefix walks
// all the entries *under* the given prefix, this walks the
// entries *above* the given prefix.
func (t *Tree) WalkPath(path string, fn WalkFn) {
	n := t.root
	search := path
	prefix := ""
	for {
		// Visit the leaf values if any
		if n.leaf != nil && fn(prefix, n.leaf.val) {
			return
		}

		// Check for key exhaution
		if len(search) == 0 {
			return
		}

		// Look for an Edge
		n = n.getEdge(search[0])
		if n == nil {
			return
		}

		prefix += n.prefix

		// Consume the search prefix
		if strings.HasPrefix(search, n.prefix) {
			search = search[len(n.prefix):]
		} else {
			break
		}
	}
}

// recursiveWalk is used to do a pre-order walk of a node
// recursively. Returns true if the walk should be aborted
func recursiveWalk(prefix string, n *Node, fn WalkFn) bool {
	// Visit the leaf values if any
	newPrefix := prefix + n.prefix
	if n.leaf != nil && fn(newPrefix, n.leaf.val) {
		return true
	}

	// Recurse on the children
	for _, e := range n.edges {
		if recursiveWalk(newPrefix, e.node, fn) {
			return true
		}
	}
	return false
}

// ToMap is used to walk the tree and convert it into a map
func (t *Tree) ToMap() map[string]interface{} {
	out := make(map[string]interface{}, t.size)
	t.Walk(t.Root(), "", func(k string, v interface{}) bool {
		out[k] = v
		return false
	})
	return out
}

// VisitNodes visits all nodes one by one
func (t *Tree) VisitNodes(n *Node, fn func(*Node) error) error {
	err := fn(n)
	if err != nil {
		return fmt.Errorf("can't process node: %s", err)
	}

	for i := range n.edges {
		err := t.VisitNodes(n.edges[i].node, fn)
		if err != nil {
			return fmt.Errorf("can't traverse inner nodes: %s", err)
		}
	}

	return nil
}

// VisitValues visits all nodes with values
func (t *Tree) VisitValues(parent *Node, prefix string, fn func(key string, n *Node) error) error {
	n := parent
	key := prefix + n.prefix

	if parent.HasValue() {
		err := fn(key, n)
		if err != nil {
			return fmt.Errorf("can't process node: %s", err)
		}
	}

	for i := range n.edges {
		err := t.VisitValues(n.edges[i].node, key, fn)
		if err != nil {
			return fmt.Errorf("can't traverse inner nodes: %s", err)
		}
	}

	return nil
}

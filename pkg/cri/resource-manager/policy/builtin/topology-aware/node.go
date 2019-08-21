// Copyright 2019 Intel Corporation. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package topologyaware

import (
	"fmt"
	system "github.com/intel/cri-resource-manager/pkg/sysfs"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
)

//
// Nodes (currently) correspond to some tangible entity in the hardware topology
// hierarchy: full machine (virtual root in multi-socket systems), an individual
// sockets a NUMA node. These nodes are linked into a tree resembling the topology
// tree, with the full machine at the top, and CPU cores at the bottom. In a single
// socket system, the virtual root is replaced with the single socket. In a single
// NUMA node case, the single node is omitted. Also, CPU cores are not modelled as
// nodes, instead they are properties of the nodes (as capacity and free CPU).
//

// NodeKind represents a unique node type.
type NodeKind string

const (
	// NilNode is the type of a nil node.
	NilNode NodeKind = ""
	// UnknownNode is the type of unknown node type.
	UnknownNode NodeKind = "unknown"
	// SocketNode represents a physical CPU package/socket in the system.
	SocketNode NodeKind = "socket"
	// NumaNode represents a NUMA node in the system.
	NumaNode NodeKind = "numa node"
	// VirtualNode represents a virtual node, currently the root multi-socket setups.
	VirtualNode NodeKind = "virtual node"
)

// Node is the abstract interface our partition tree nodes implement.
type Node interface {
	// IsNil tests if this node is nil.
	IsNil() bool
	// Name returns the name of this node.
	Name() string
	// Kind returns the type of this node.
	Kind() NodeKind
	// NodeId returns the (enumerated) node id of this node.
	NodeId() int
	// Parent returns the parent node of this node.
	Parent() Node
	// Children returns the child nodes of this node.
	Children() []Node
	// LinkParent sets the given node as the parent node, and appends this node as a its child.
	LinkParent(Node)
	// AddChildren appends the nodes to the children, *WITHOUT* updating their parents.
	AddChildren([]Node)
	// IsSameNode returns true if the given node is the same as this one.
	IsSameNode(Node) bool
	// IsRootNode returns true if this node has no parent.
	IsRootNode() bool
	// IsLeafNode returns true if this node has no children.
	IsLeafNode() bool
	// Get the distance of this node from the root node.
	RootDistance() int
	// Get the height of this node (inverse of depth: tree depth - node depth).
	NodeHeight() int
	// System returns the policy sysfs instance.
	System() *system.System
	// Policy returns the policy back pointer.
	Policy() *policy
	// DiscoverCpu
	DiscoverCpu() CpuSupply
	// GetCpu
	GetCpu() CpuSupply
	// FreeCpu returns the availble CPU supply of the node.
	FreeCpu() CpuSupply
	// GrantedCpu returns the amount of granted shared CPU capacity of this node.
	GrantedCpu() int
	// GetMemset
	GetMemset() system.IdSet
	// DiscoverMemset
	DiscoverMemset() system.IdSet
	// Score calculates the score of the node wrt. the given request.
	Score(CpuRequest) float64
	// DepthFirst traverse the tree@node calling the function at each node.
	DepthFirst(func(Node) error) error
	// BreadthFirst traverse the tree@node calling the function at each node.
	BreadthFirst(func(Node) error) error
	// Dump state of the node.
	Dump(string, ...int)
	// Dump type-specific state of the node.
	dump(string, ...int)
}

// node represents data common to all node types.
type node struct {
	policy   *policy      // policy back pointer
	self     nodeself     // upcasted/type-specific interface
	name     string       // node name
	id       int          // node id
	kind     NodeKind     // node type
	depth    int          // node depth in the tree
	parent   Node         // parent node
	children []Node       // child nodes
	nodecpu  CpuSupply    // CPU available at this node
	freecpu  CpuSupply    // CPU allocatable at this node
	mem      system.IdSet // memory attached to this node
}

// nodeself is used to 'upcast' a generic Node interface to a type-specific one.
type nodeself struct {
	node Node
}

// socketnode represents a physical CPU package/socket in the system.
type socketnode struct {
	node                   // common node data
	id     system.Id       // NUMA node socket id
	syspkg *system.Package // corresponding system.Package
}

// numanode represents a NUMA node in the system.
type numanode struct {
	node                 // common node data
	id      system.Id    // NUMA node system id
	sysnode *system.Node // corresponding system.Node
}

// virtualnode represents a virtual node (ATM only the root in a multi-socket system).
type virtualnode struct {
	node // common node data
}

// special node instance to represent a nonexistent node
var nilnode Node = &node{
	name:     "<nil node>",
	id:       -1,
	kind:     NilNode,
	depth:    -1,
	children: nil,
}

// Init initializes the resource with common node data.
func (n *node) init(p *policy, name string, kind NodeKind, parent Node) {
	n.policy = p
	n.name = name
	n.kind = kind
	n.parent = parent

	n.LinkParent(parent)
}

// IsNil tests if a node
func (n *node) IsNil() bool {
	return n.kind == NilNode
}

// Name returns the name of this node.
func (n *node) Name() string {
	if n.IsNil() {
		return "<nil node>"
	}
	return n.name
}

// Kind returns the kind of this node.
func (n *node) Kind() NodeKind {
	return n.kind
}

// NodeId returns the node id of this node.
func (n *node) NodeId() int {
	if n.IsNil() {
		return -1
	}
	return n.id
}

// IsSameNode checks if the given node is that same as this one.
func (n *node) IsSameNode(other Node) bool {
	return n.NodeId() == other.NodeId()
}

// IsRootNode returns true if this node has no parent.
func (n *node) IsRootNode() bool {
	return n.parent.IsNil()
}

// IsLeafNode returns true if this node has no children.
func (n *node) IsLeafNode() bool {
	return len(n.children) == 0
}

// RootDistance returns the distance of this node from the root node.
func (n *node) RootDistance() int {
	if n.IsNil() {
		return -1
	}
	return n.depth
}

// NodeHeight returns the hight of this node (tree depth - node depth).
func (n *node) NodeHeight() int {
	if n.IsNil() {
		return -1
	}
	return n.policy.depth - n.depth
}

// Parent returns the parent of this node.
func (n *node) Parent() Node {
	if n.IsNil() {
		return nil
	}

	return n.parent
}

// Children returns the children of this node.
func (n *node) Children() []Node {
	if n.IsNil() {
		return nil
	}

	return n.children
}

// LinkParent sets the given node as the node parent and appends this node to the parents children.
func (n *node) LinkParent(parent Node) {
	n.parent = parent
	if !parent.IsNil() {
		parent.AddChildren([]Node{n})
	}

	n.depth = parent.RootDistance() + 1
}

// AddChildren appends the nodes to the childres, *WITHOUT* setting their parent.
func (n *node) AddChildren(nodes []Node) {
	n.children = append(n.children, nodes...)
}

// Dump information/state of the node.
func (n *node) Dump(prefix string, level ...int) {
	if !log.DebugEnabled() {
		return
	}

	lvl := 0
	if len(level) > 0 {
		lvl = level[0]
	}
	idt := indent(prefix, lvl)

	n.self.node.dump(prefix, lvl)
	log.Debug("%s  - node CPU: %v", idt, n.nodecpu)
	log.Debug("%s  - free CPU: %v", idt, n.freecpu)
	log.Debug("%s  - memory: %v", idt, n.mem)
	for _, grant := range n.policy.allocations.Cpu {
		if grant.GetNode().NodeId() == n.id {
			log.Debug("%s    + %s", idt, grant.String())
		}
	}
	if !n.Parent().IsNil() {
		log.Debug("%s  - parent: <%s>", idt, n.Parent().Name())
	}
	log.Debug("%s  - children:", idt)
	for _, c := range n.children {
		c.Dump(prefix, lvl+1)
	}
}

// Dump type-specific information about the node.
func (n *node) dump(prefix string, level ...int) {
	n.self.node.dump(prefix, level...)
}

// Do a depth-first traversal starting at node calling the given function at each node.
func (n *node) DepthFirst(fn func(Node) error) error {
	for _, c := range n.children {
		if err := c.DepthFirst(fn); err != nil {
			return err
		}
	}

	return fn(n)
}

// Do a breadth-first traversal starting at node calling the given function at each node.
func (n *node) BreadthFirst(fn func(Node) error) error {
	if err := fn(n); err != nil {
		return err
	}

	for _, c := range n.children {
		if err := c.BreadthFirst(fn); err != nil {
			return err
		}
	}

	return nil
}

// System returns the policy System instance.
func (n *node) System() *system.System {
	return n.policy.sys
}

// Policy returns the policy back pointer.
func (n *node) Policy() *policy {
	return n.policy
}

// Get CPU available at this node.
func (n *node) GetCpu() CpuSupply {
	return n.self.node.GetCpu()
}

// Discover CPU available at this node.
func (n *node) DiscoverCpu() CpuSupply {
	return n.self.node.DiscoverCpu()
}

// FreeCpu returns the available CPU supply in this node.
func (n *node) FreeCpu() CpuSupply {
	return n.freecpu
}

// Get score for a cpu request.
func (n *node) Score(req CpuRequest) float64 {
	f := n.FreeCpu()
	return f.Score(req)
}

// Get the set of memory attached to this node.
func (n *node) GetMemset() system.IdSet {
	return n.self.node.GetMemset()
}

// Discover the set of memory attached to this node.
func (n *node) DiscoverMemset() system.IdSet {
	return n.self.node.DiscoverMemset()
}

// Granted returns the amount of granted shared CPU capacity of this node.
func (n *node) GrantedCpu() int {
	granted := n.freecpu.Granted()
	for _, c := range n.children {
		granted += c.GrantedCpu()
	}
	return granted
}

// NewNumaNode create a node for a CPU socket.
func (p *policy) NewNumaNode(id system.Id, parent Node) Node {
	n := &numanode{}
	n.self.node = n
	n.node.init(p, fmt.Sprintf("numa node #%v", id), NumaNode, parent)
	n.id = id
	n.sysnode = p.sys.Node(id)

	return n
}

// Dump (the NUMA-specific parts of) this node.
func (n *numanode) dump(prefix string, level ...int) {
	log.Debug("%s<NUMA node #%v>", indent(prefix, level...), n.id)
}

// Get CPU supply available at this node.
func (n *numanode) GetCpu() CpuSupply {
	return n.nodecpu.Clone()
}

// DiscoverCpu discovers the CPU supply available at this node.
func (n *numanode) DiscoverCpu() CpuSupply {
	log.Debug("discovering CPU available at node %s...", n.Name())

	nodecpus := n.sysnode.CPUSet()
	isolated := nodecpus.Intersection(n.policy.isolated)
	sharable := nodecpus.Difference(isolated)
	n.nodecpu = newCpuSupply(n, isolated, sharable, 0)

	n.freecpu = n.nodecpu.Clone()
	return n.nodecpu.Clone()
}

// GetMemset() returns the set of memory attached to this node.
func (n *numanode) GetMemset() system.IdSet {
	return n.mem.Clone()
}

// DiscoverMemset discovers the set of memory attached to this node.
func (n *numanode) DiscoverMemset() system.IdSet {
	n.mem = system.NewIdSet(n.sysnode.Id())
	return n.mem.Clone()
}

// NewSocketNode create a node for a CPU socket.
func (p *policy) NewSocketNode(id system.Id, parent Node) Node {
	n := &socketnode{}
	n.self.node = n
	n.node.init(p, fmt.Sprintf("socket #%v", id), SocketNode, parent)
	n.id = id
	n.syspkg = p.sys.Package(id)

	return n
}

// Dump (the socket-specific parts of) this node.
func (n *socketnode) dump(prefix string, level ...int) {
	log.Debug("%s<socket #%v>", indent(prefix, level...), n.id)
}

// Get CPU supply available at this node.
func (n *socketnode) GetCpu() CpuSupply {
	return n.nodecpu.Clone()
}

// DiscoverCpu discovers the CPU supply available at this socket.
func (n *socketnode) DiscoverCpu() CpuSupply {
	log.Debug("discovering CPU available at node %s...", n.Name())

	if n.IsLeafNode() {
		sockcpus := n.syspkg.CPUSet()
		isolated := sockcpus.Intersection(n.policy.isolated)
		sharable := sockcpus.Difference(isolated)
		n.nodecpu = newCpuSupply(n, isolated, sharable, 0)
	} else {
		n.nodecpu = newCpuSupply(n, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 0)
		for _, c := range n.children {
			n.nodecpu.Cumulate(c.DiscoverCpu())
		}
	}

	n.freecpu = n.nodecpu.Clone()
	return n.nodecpu.Clone()
}

// GetMemset() returns the set of memory attached to this socket.
func (n *socketnode) GetMemset() system.IdSet {
	return n.mem.Clone()
}

// DiscoverMemset discovers the set of memory attached to this socket.
func (n *socketnode) DiscoverMemset() system.IdSet {
	n.mem = system.NewIdSet()
	for _, c := range n.children {
		n.mem.Add(c.GetMemset().Members()...)
	}

	return n.mem.Clone()
}

// NewVirtualNode creates a new virtual node.
func (p *policy) NewVirtualNode(name string, parent Node) Node {
	n := &virtualnode{}
	n.self.node = n
	n.node.init(p, fmt.Sprintf("%s", name), VirtualNode, parent)

	return n
}

// Dump (the virtual-node specific parts of) this node.
func (n *virtualnode) dump(prefix string, level ...int) {
	log.Debug("%s<virtual %s>", indent(prefix, level...), n.name)
}

// Get CPU supply available at this node.
func (n *virtualnode) GetCpu() CpuSupply {
	return n.nodecpu.Clone()
}

// DiscoverCpu discovers the CPU supply available at this node.
func (n *virtualnode) DiscoverCpu() CpuSupply {
	log.Debug("discovering CPU available at node %s...", n.Name())

	n.nodecpu = newCpuSupply(n, cpuset.NewCPUSet(), cpuset.NewCPUSet(), 0)
	for _, c := range n.children {
		n.nodecpu.Cumulate(c.DiscoverCpu())
	}

	n.freecpu = n.nodecpu.Clone()
	return n.nodecpu.Clone()
}

// GetMemset() returns the set of memory attached to this socket.
func (n *virtualnode) GetMemset() system.IdSet {
	return n.mem.Clone()
}

// DiscoverMemset discovers the set of memory attached to this socket.
func (n *virtualnode) DiscoverMemset() system.IdSet {
	n.mem = system.NewIdSet()
	for _, c := range n.children {
		n.mem.Add(c.GetMemset().Members()...)
	}
	return n.mem.Clone()
}

// Finalize the setup of nilnode.
func init() {
	nilnode.(*node).self.node = nilnode
	nilnode.(*node).parent = nilnode.(*node).self.node
}

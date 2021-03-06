package nodeManager

import (
	"errors"
	"fmt"
	"net"
	"reflect"
	"sync"
	"time"

	"github.com/skycoin/skycoin/src/cipher"
)

// Debugging; of set to true, all debugging messages will be printed out.
var Debugging = false

const defaultMaxNodes = 1000
const defaultMaxConnections = 1000
const defaultMaxMessageLength = 1024 * 256
const defaultIP = "127.0.0.1"
const defaultPort uint16 = 0 // 0 tells the kernel to assign a free port
const defaultHandshakeTimeout = time.Duration(time.Second * 5)
const defaultReadDeadline = time.Duration(time.Minute * 5)

// ManagerConfig is the configuration struct of a node manager
type ManagerConfig struct {
	IP   string
	ip   net.IP
	Port uint16

	// max number of nodes
	MaxNodes int
	// max connections per node (x2) per upstream and downstream
	MaxConnections int
	// Messages greater than length are rejected and the sender disconnected
	MaxMessageLength int
}

// Manager is a node manager.
type Manager struct {
	mu *sync.RWMutex

	config *ManagerConfig

	nodes map[cipher.PubKey]*Node

	downstreamCallbacks map[string]reflect.Type
	upstreamCallbacks   map[string]reflect.Type
}

// NewManagerConfig returns an instance of ManagerConfig loaded with default values
func NewManagerConfig() ManagerConfig {
	// set defaults
	newConfig := ManagerConfig{
		IP:               defaultIP,
		Port:             defaultPort,
		MaxNodes:         defaultMaxNodes,
		MaxConnections:   defaultMaxConnections,
		MaxMessageLength: defaultMaxMessageLength,
	}
	return newConfig
}

func (mc *ManagerConfig) validate() error {
	// parse and validate IP address
	mc.ip = net.ParseIP(mc.IP)
	if mc.ip == nil {
		return fmt.Errorf("cannot parse IP: %v", mc.IP)
	}
	return nil
}

// NewManager returns a new manager
func NewManager(config ManagerConfig) (*Manager, error) {
	// validate config
	err := config.validate()
	if err != nil {
		return &Manager{}, err
	}

	newManager := Manager{
		nodes:               make(map[cipher.PubKey]*Node),
		config:              &config,
		downstreamCallbacks: make(map[string]reflect.Type),
		upstreamCallbacks:   make(map[string]reflect.Type),
		mu:                  &sync.RWMutex{},
	}

	// REGISTER HANDSHAKE MESSAGES
	// register messages that this node can receive from downstream
	newManager.registerDownstreamMessage(hsm1{})
	newManager.registerDownstreamMessage(hsm3{})

	// register messages that this node can receive from upstream
	newManager.registerUpstreamMessage(hsm2{})
	newManager.registerUpstreamMessage(hsm4{})

	return &newManager, nil
}

func (nm *Manager) registerDownstreamMessage(msg interface{}) {
	registerMessage("DownstreamMessage", nm.mu, nm.downstreamCallbacks, msg)
}
func (nm *Manager) registerUpstreamMessage(msg interface{}) {
	registerMessage("UpstreamMessage", nm.mu, nm.upstreamCallbacks, msg)
}

// Nodes returns all the nodes registered in the NodeManager
func (nm *Manager) Nodes() []*Node {
	nodeList := []*Node{}
	for _, node := range nm.nodes {
		nodeList = append(nodeList, node)
	}
	return nodeList
}

// NodeByID returns a Node by its pubKey
func (nm *Manager) NodeByID(pubKey cipher.PubKey) (*Node, error) {
	node, ok := nm.nodes[pubKey]
	if !ok {
		return &Node{}, errors.New("Node not found")
	}
	return node, nil
}

func (nm *Manager) Shutdown() error {
	// TODO: Shutdown gracefully every component (starting from the level most at the bottom)
	return nil
}

func (nm *Manager) TerminateNodeByID(pubKey *cipher.PubKey, reason error) error {
	if pubKey == nil {
		return ErrPubKeyIsNil
	}

	elem, err := nm.NodeByID(*pubKey)
	if err != nil {
		return err
	}

	return nm.TerminateNode(elem, reason)
}

func (nm *Manager) TerminateNode(node *Node, reason error) error {
	fmt.Println("terminating node; reason:", reason)

	// close and cleanup all resources used by the node
	err := node.close()
	if err != nil {
		return err
	}

	// remove node from pool
	err = nm.removeNodeByID(node.pubKey)
	if err != nil {
		return err
	}
	return nil
}

func (nm *Manager) removeNodeByID(pubKey *cipher.PubKey) error {
	if pubKey == nil {
		return ErrPubKeyIsNil
	}
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if _, exists := nm.nodes[*pubKey]; !exists {
		return fmt.Errorf("node does not exist: %v", pubKey.Hex())
	}

	delete(nm.nodes, *pubKey)
	return nil
}

package autoraft

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/grandcat/zeroconf"
	"github.com/hashicorp/raft"
	"github.com/rs/zerolog/log"
)

const (
	serviceType = "_autoraft."
	domain      = "local."
)

// Node represents a discoverable raft node
type Node struct {
	id          string
	serviceName string
	server      *zeroconf.Server
	peers       map[string]*zeroconf.ServiceEntry
	raftNode    *raft.Raft
}

// New registers itself with any other listeners
func New(raftNode *raft.Raft, id, service string, port int, ttl uint32) (*Node, error) {
	// zeroconf register
	serviceName := service + serviceType
	server, err := zeroconf.Register(id, serviceName, domain, port, nil, nil)
	if err != nil {
		log.Error().Err(err).Msg("Unable to create a new node")
		return nil, err
	}
	n := &Node{
		id:          id,
		serviceName: serviceName,
		peers:       make(map[string]*zeroconf.ServiceEntry),
		server:      server,
		raftNode:    raftNode,
	}
	if ttl > 0 {
		n.server.TTL(ttl)
	}
	log.Info().
		Str("Name", id).
		Int("Port", port).
		Str("Service", service).
		Str("Domain", domain).
		Msg("Advertizing node")
	return n, nil
}

// Shutdown stops the node from advertizing itself on the network
func (n *Node) Shutdown() {
	n.server.Shutdown()
}

// Listen searches for peers
func (n *Node) Listen() (chan *zeroconf.ServiceEntry, error) {
	// start resolver
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to initialize listener")
		return nil, err
	}

	// keep looking for entries
	entries := make(chan *zeroconf.ServiceEntry)
	ctx := context.Background()
	err = resolver.Browse(ctx, n.serviceName, domain, entries)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to listen for peers")
	}

	// raftPeers := n.processLookup(entries)
	newPeers := n.processEntry(entries, n.filterNewEntries)
	voters := n.processEntry(newPeers, n.processRaftVoter)
	return voters, nil
}

func (n *Node) processEntry(in chan *zeroconf.ServiceEntry, fn func(entry *zeroconf.ServiceEntry) bool) (out chan *zeroconf.ServiceEntry) {
	out = make(chan *zeroconf.ServiceEntry)
	go func() {
		for entry := range in {
			if fn(entry) {
				out <- entry
			}
		}
	}()
	return
}

func (n *Node) filterNewEntries(entry *zeroconf.ServiceEntry) (forwardEntry bool) {
	if entry.Instance == n.id {
		// ignore self
		forwardEntry = false
		return
	}
	if _, ok := n.peers[entry.Instance]; ok {
		// already known
		forwardEntry = false
		return
	}
	// add to known peers
	n.peers[entry.Instance] = entry
	log.Info().
		Str("Name", entry.ServiceName()).
		Str("HostName", entry.HostName).
		Int("Port", entry.Port).
		Str("IPv4", fmt.Sprintf("%+v", entry.AddrIPv4)).
		Str("IPv6", fmt.Sprintf("%+v", entry.AddrIPv6)).
		Msg("Discovered new service peer")
	forwardEntry = true
	return
}

func (n *Node) processRaftVoter(entry *zeroconf.ServiceEntry) (forwardEntry bool) {
	ip := entry.AddrIPv4[0]
	addr := net.TCPAddr{
		IP:   ip,
		Port: entry.Port,
	}
	log.Info().Str("addr", addr.String()).Msg("Adding new raft voter")
	f := n.raftNode.AddVoter(raft.ServerID(entry.Instance), raft.ServerAddress(addr.String()), 0, time.Second*5)
	err := f.Error()
	if err != nil {
		log.Error().Err(err).Msg("Error adding voter")
	}
	forwardEntry = true
	return
}

package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/hashicorp/raft"
	store "github.com/hashicorp/raft-boltdb"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/urjitbhatia/autoraft"
)

var (
	id          = flag.String("name", "foo1", "The unique id for the service instance.")
	serviceName = flag.String("serviceName", "exampleSrv", "The name for the service.")
	port        = flag.Int("port", 42424, "Set the port the service is listening to.")
	waitTime    = flag.Int("wait", 10, "Duration in [s] to publish service for.")
	bootstrap   = flag.Bool("bootstrap", false, "Bootstrap this node.")
)

func init() {
	log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.Kitchen}).
		With().
		Timestamp().
		Logger()
}

type myFSM struct{}

func (m *myFSM) Apply(*raft.Log) interface{}         { return nil }
func (m *myFSM) Snapshot() (raft.FSMSnapshot, error) { return nil, nil }
func (m *myFSM) Restore(io.ReadCloser) error         { return nil }

func main() {
	flag.Parse()

	rn := getRaft()

	node, err := autoraft.New(rn, *id, *serviceName, *port, 1000)
	if err != nil {
		log.Fatal().Err(err).Msg("Exiting")
	}
	peers, err := node.Listen()
	if err != nil {
		log.Fatal().Msg("Exiting")
	}

	go func() {
		for range peers {
			// drain
		}
	}()

	for range time.NewTicker(time.Second * 15).C {
		log.Info().Msgf("got rcluster, %+v", rn.GetConfiguration())
	}
}

func getRaft() *raft.Raft {
	logStore, err := store.New(store.Options{Path: "./foo" + *id, BoltOptions: nil, NoSync: false})
	if err != nil {
		log.Fatal().Err(err).Msg("Error starting boltdb store")
	}
	var fsm raft.FSM = &myFSM{}
	var stableStore = logStore
	var snapStore raft.SnapshotStore = raft.NewDiscardSnapshotStore()

	// create address
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatal().Err(err).Msg("Error resolving tcp addr")
	}

	// connect tcp transport
	transport, err := raft.NewTCPTransport(addr.String(), addr, 1, time.Second, os.Stdout)
	if err != nil {
		log.Fatal().Str("addr", addr.String()).Err(err).Msg("Error starting raft transport")
	}
	log.Info().Str("transport", string(transport.LocalAddr())).Send()
	log.Info().Str("transportbind", addr.String()).Send()

	var rcluster *raft.Raft
	config := raft.DefaultConfig()
	config.LocalID = raft.ServerID(*id)
	rcluster, err = raft.NewRaft(config, fsm, logStore, stableStore, snapStore, transport)

	if err != nil {
		log.Fatal().Err(err).Msg("Error creating cluster")
	}

	if *bootstrap {
		f := rcluster.BootstrapCluster(raft.Configuration{
			Servers: []raft.Server{{
				ID:       raft.ServerID(*id),
				Address:  transport.LocalAddr(),
				Suffrage: raft.Voter}},
		})
		err = f.Error()
		if err != nil {
			log.Error().Err(err).Msg("Error bootstrapping cluster")
		}
		<-rcluster.LeaderCh()
	}

	return rcluster
}

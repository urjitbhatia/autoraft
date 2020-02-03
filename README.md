# Autoraft

Autoraft is wrapper around [Raft](https://github.com/hashicorp/raft) and [Zeroconf](https://github.com/grandcat/zeroconf).
It autobootstraps a raft cluster using zeroconf (via mDNS)

## Example

See `example/main.go` for how to use autoraft

## Usage

```go
rn := getRaftNode()
// getRaftNode returns a bootstrapped raft instance
ttl := 1000 // ms - zeroconf mDNS entry ttl
port := 9999
node, err := autoraft.New(rn, "myServiceID", "myService", port, ttl)
if err != nil {
    log.Fatal("Exiting")
}

peers, err := node.Listen()
if err != nil {
    log.Fatal("Exiting")
}

for raftPeer := range peers {
    log.Printf("Connected to new peer: %+v", raftPeer)
}
```

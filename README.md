#Freeway

Freeway is a partial implementation of the distributed hash table that powers the BitTorrent network.

## About

Most queries and replies defined in the KRPC protocol are implemented.

This project was an experiment and is unlikely to be maintained. The code provided should not be used in any serious project.

## Example

````go
package main

import (
    "log"

    "github.com/mohamedattahri/freeway"
)

const port int = 6881

func main() {
    dht, err := freeway.NewDHT(freeway.NewRandomID(), port)
    if err != nil {
        log.Fatal(err)
    }

    if err := dht.Start(); err != nil {
        log.Fatal(err)
    }

    //Bootstrap
    bittorrent, _ := freeway.NewPeerFromIP(dht, "router.bittorrent.com", 6881, freeway.NewRandomID())
    transmission, _ := freeway.NewPeerFromIP(dht, "dht.transmissionbt.com", 6881, freeway.NewRandomID())
    magnets, _ := freeway.NewPeerFromIP(dht, "1.a.magnets.im", 6881, freeway.NewRandomID())
    utorrent, _ := freeway.NewPeerFromIP(dht, "router.utorrent.com", 6881, freeway.NewRandomID())
    dht.Bootstrap([]*freeway.Peer{bittorrent, utorrent, transmission, magnets})

    log.Println("This machine is now known as", dht.LocalPeer())
    <-make(chan int)
}
````

package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

const (
	protocolICMP = 1
	maxHops      = 64
	timeout      = 2 * time.Second
)

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: go run main.go <destination>")
		os.Exit(1)
	}
	destAddr := os.Args[1]

	// Resolve destination IP address
	destIPAddr, err := net.ResolveIPAddr("ip", destAddr)
	if err != nil {
		fmt.Println("Error resolving destination IP:", err)
		os.Exit(1)
	}

	// Create ICMP echo request socket
	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		fmt.Println("Error creating ICMP socket:", err)
		os.Exit(1)
	}
	defer conn.Close()

	// Set TTL and deadline
	conn.IPv4PacketConn().SetTTL(maxHops)
	conn.SetDeadline(time.Now().Add(timeout))

	// Catch interrupt signal for graceful termination
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	// Perform traceroute
	for ttl := 1; ttl <= maxHops; ttl++ {
		// Set TTL
		conn.IPv4PacketConn().SetTTL(ttl)

		// Create ICMP echo request message
		msg := icmp.Message{
			Type: ipv4.ICMPTypeEcho,
			Code: 0,
			Body: &icmp.Echo{
				ID:   os.Getpid() & 0xffff,
				Seq:  ttl,
				Data: []byte(""),
			},
		}
		msgBytes, err := msg.Marshal(nil)
		if err != nil {
			fmt.Println("Error marshaling ICMP message:", err)
			os.Exit(1)
		}

		// Send ICMP echo request
		start := time.Now()
		if _, err := conn.WriteTo(msgBytes, destIPAddr); err != nil {
			fmt.Println("Error sending ICMP message:", err)
			continue
		}

		// Receive ICMP echo reply
		reply := make([]byte, 1500)
		n, _, err := conn.ReadFrom(reply)
		if err != nil {
			fmt.Println("Error receiving ICMP reply:", err)
			continue
		}
		duration := time.Since(start)

		// Parse ICMP echo reply
		pkt, err := icmp.ParseMessage(protocolICMP, reply[:n])
		if err != nil {
			fmt.Println("Error parsing ICMP reply:", err)
			continue
		}
		switch pkt.Type {
		case ipv4.ICMPTypeTimeExceeded:
			// Print hop details
			hopAddr := net.IP(pkt.Body.(*icmp.TimeExceeded).Data)
			fmt.Printf("%d: %s (%s) %v\n", ttl, hopAddr, hopAddr, duration)
		case ipv4.ICMPTypeEchoReply:
			// Print final destination details
			destName, err := net.LookupAddr(destIPAddr.String())
			if err != nil {
				destName = []string{destIPAddr.String()}
			}
			fmt.Printf("%d: %s %v\n", ttl, destName[0], duration)
			return
		default:
			fmt.Printf("%d: Unexpected ICMP message type: %v\n", ttl, pkt.Type)
		}

		// Check if interrupted
		select {
		case <-interrupt:
			fmt.Println("Traceroute interrupted.")
			return
		default:
		}
	}

	fmt.Println("Max hops reached.")
}

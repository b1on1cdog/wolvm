//go:build linux
// +build linux

package main

import (
	"bytes"
	"encoding/hex"
	"flag"
	"log"
	"net"
	"os/exec"
	"strings"

	"golang.org/x/sys/unix"
)

func htons(i uint16) uint16 {
	return (i<<8)&0xff00 | i>>8
}

func main() {
	ifaceName := flag.String("iface", "br0", "Interface to listen on")
	vmName := flag.String("vm", "", "VM name")
	macStr := flag.String("mac", "", "MAC to wake (aa:bb:cc:dd:ee:ff)")
	flag.Parse()

	if *vmName == "" || *macStr == "" {
		log.Fatal("vm and mac are required")
	}

	// Build WoL magic packet pattern
	macClean := strings.ReplaceAll(strings.ToLower(*macStr), ":", "")
	macBytes, err := hex.DecodeString(macClean)
	if err != nil {
		log.Fatal(err)
	}

	pattern := bytes.Repeat([]byte{0xff}, 6)
	for i := 0; i < 16; i++ {
		pattern = append(pattern, macBytes...)
	}

	iface, err := net.InterfaceByName(*ifaceName)
	if err != nil {
		log.Fatal(err)
	}

	// Open raw packet socket
	fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_RAW, int(htons(unix.ETH_P_ALL)))
	if err != nil {
		log.Fatal(err)
	}
	defer unix.Close(fd)

	// Bind socket to interface
	sll := &unix.SockaddrLinklayer{
		Protocol: htons(unix.ETH_P_ALL),
		Ifindex:  iface.Index,
	}

	if err := unix.Bind(fd, sll); err != nil {
		log.Fatal(err)
	}

	log.Printf("Listening for WoL on %s for VM %s (%s)", *ifaceName, *vmName, *macStr)

	buf := make([]byte, 2048)

	for {
		n, _, err := unix.Recvfrom(fd, buf, 0)
		if err != nil {
			continue
		}

		if bytes.Contains(buf[:n], pattern) {
			out, _ := exec.Command("virsh", "domstate", *vmName).Output()
			if !bytes.Contains(out, []byte("running")) {
				log.Printf("WoL received — starting VM %s", *vmName)
				exec.Command("virsh", "start", *vmName).Run()
			}
		}
	}
}

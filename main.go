//go:build linux
// +build linux

package main

import (
	"bytes"
	"encoding/hex"
	"flag"
	"log"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

func htons(i uint16) uint16 {
	return (i<<8)&0xff00 | i>>8
}

func is_vm_running(vmName string) bool {
	out, _ := exec.Command("virsh", "domstate", vmName).Output()
	if bytes.Contains(out, []byte("running")) {
		return true
	}
	return false
}

func is_vm_paused(vmName string) bool {
	out, _ := exec.Command("virsh", "domstate", vmName).Output()
	if bytes.Contains(out, []byte("paused")) {
		return true
	}
	return false
}

func vmStateStalker(vmName string, woffUrl string) {
	// Wait for VM to shutdown to call a webhook
	if woffUrl == "" {
		return
	}
	for {
		time.Sleep(8 * time.Second)
		if !is_vm_paused(vmName) && !is_vm_running(vmName) {
			http.Get(woffUrl)
			return
		}
	}
}

func main() {
	ifaceName := flag.String("iface", "br0", "Interface to listen on")
	vmName := flag.String("vm", "", "VM name")
	macStr := flag.String("mac", "", "MAC to wake (aa:bb:cc:dd:ee:ff)")

	wonUrl := flag.String("won", "", "WebHook URL for VM start")
	woffUrl := flag.String("woff", "", "WebHook URL for VM shutdown")

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
			if !is_vm_running(*vmName) {
				log.Printf("WoL received — starting VM %s", *vmName)
				vmCmd := "start"
				if is_vm_paused(*vmName) {
					vmCmd = "resume"
				}
				go func() {
					http.Get(*wonUrl)
				}()
				exec.Command("virsh", vmCmd, *vmName).Run()
				go func() {
					vmStateStalker(*vmName, *woffUrl)
				}()
			}
		}
	}
}

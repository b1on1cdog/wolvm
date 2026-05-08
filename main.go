//go:build linux
// +build linux

package main

import (
	"bytes"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/exec"
	"regexp"
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

func parseVirshList(virshOutput string) [][]string {
	list := strings.Split(virshOutput, "\n")[2:]
	out_list := [][]string{}
	for _, line := range list {
		if len(line) > 12 {
			line = strings.TrimSpace(line)
			whitespaces := regexp.MustCompile(`\s\s+`)
			line = whitespaces.ReplaceAllString(line, "|")
			out_list = append(out_list, strings.Split(line, "|"))
		}
	}
	return out_list
}

func readVMInterface(vmName string, keyName string) string {
	output, _ := exec.Command("virsh", "domiflist", vmName).Output()
	virshOutput := string(output[:])
	virshList := parseVirshList(virshOutput)
	validKeys := map[string]int{
		"Interface": 0,
		"Type":      1,
		"Source":    2,
		"Model":     3,
		"MAC":       4,
	}

	for _, virshEl := range virshList {
		return virshEl[validKeys[keyName]]
	}
	return ""
}

func listVMs() []string {
	output, _ := exec.Command("virsh", "list", "--all").Output()
	virshOutput := string(output[:])
	virshList := parseVirshList(virshOutput)
	vms := []string{}
	for _, virshEl := range virshList {
		vms = append(vms, virshEl[1])
		//fmt.Printf("vm: %v\n", virshEl[1])
	}
	return vms
}

func startWolForVM(vmName string, macStr string, ifaceName string, wonUrl string, woffUrl string) {
	// Build WoL magic packet pattern
	macClean := strings.ReplaceAll(strings.ToLower(macStr), ":", "")
	macBytes, err := hex.DecodeString(macClean)
	if err != nil {
		log.Fatal(err)
	}

	pattern := bytes.Repeat([]byte{0xff}, 6)
	for i := 0; i < 16; i++ {
		pattern = append(pattern, macBytes...)
	}

	iface, err := net.InterfaceByName(ifaceName)
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

	log.Printf("Listening for WoL on %s for VM %s (%s)", ifaceName, vmName, macStr)

	buf := make([]byte, 2048)

	for {
		n, _, err := unix.Recvfrom(fd, buf, 0)
		if err != nil {
			continue
		}

		if bytes.Contains(buf[:n], pattern) {
			if !is_vm_running(vmName) {
				log.Printf("WoL received — starting VM %s", vmName)
				vmCmd := "start"
				if is_vm_paused(vmName) {
					vmCmd = "resume"
				}
				go func() {
					http.Get(wonUrl)
				}()
				exec.Command("virsh", vmCmd, vmName).Run()
				go func() {
					vmStateStalker(vmName, woffUrl)
				}()
			}
		}
	}
}

func main() {
	vmName := flag.String("vm", "", "VM name, if unspecified all VMs that match the filter will be used")
	ifaceName := flag.String("iface", "", "Interface to listen on, work as filter when -vm is unspecified")
	macStr := flag.String("mac", "", "MAC to wake (aa:bb:cc:dd:ee:ff), work as filter when -vm is unspecified")

	wonUrl := flag.String("won", "", "WebHook URL for VM start")
	woffUrl := flag.String("woff", "", "WebHook URL for VM shutdown")

	selectFirst := flag.Bool("first", false, "Select first VM only (when VM is unspecified)")
	flag.Parse()

	if *vmName == "" {
		if *macStr == "" && *ifaceName == "" {
			fmt.Printf("vm name and network fields not specified, starting wolvm for ALL virsh domains....\n")
		} else {
			fmt.Printf("vm name not specified, searching for vm with matching network fields....\n")
		}
		vms := listVMs()
		for _, domain := range vms {
			networkSource := readVMInterface(domain, "Source")
			if *ifaceName != "" && *ifaceName != networkSource {
				fmt.Printf("Skipping %v because interface mismatch\n", domain)
				continue
			}
			macAddr := readVMInterface(domain, "MAC")
			if *macStr != "" && *macStr != macAddr {
				fmt.Printf("Skipping %v because mac mismatch\n", domain)
				continue
			}
			if len(vms) == 1 || *selectFirst {
				*vmName = domain
				*macStr = macAddr
				*ifaceName = networkSource
				break
			}
			go func() {
				startWolForVM(domain, macAddr, networkSource, *wonUrl, *woffUrl)
			}()
		}
		if *vmName == "" {
			select {}
		}
	}

	if *macStr == "" {
		*macStr = readVMInterface(*vmName, "MAC")
	}

	if *ifaceName == "" {
		*ifaceName = readVMInterface(*vmName, "Source")
	}

	startWolForVM(*vmName, *macStr, *ifaceName, *wonUrl, *woffUrl)

}

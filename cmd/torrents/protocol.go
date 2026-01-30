package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"
	"torrent/cmd/pkg/bencode"
)

func udpTrackerRequest(announceURL string, infoHash []byte, length int) {
	u, err := url.Parse(announceURL)
	if err != nil {
		fmt.Printf("failed to parse announce URL: %v\n", err)
		return
	}

	addr, err := net.ResolveUDPAddr("udp", u.Host)
	if err != nil {
		fmt.Printf("failed to resolve UDP address: %v\n", err)
		return
	}
	fmt.Printf("resolved UDP address: %s%s\n", u.Host, addr)

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		fmt.Printf("failed to dial UDP: %v\n", err)
		return
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(10 * time.Second))

	// 1. Connect Phase
	transactionID := rand.Uint32()
	connectReq := new(bytes.Buffer)
	binary.Write(connectReq, binary.BigEndian, uint64(0x41727101980)) // protocol_id
	binary.Write(connectReq, binary.BigEndian, uint32(0))             // action: connect
	binary.Write(connectReq, binary.BigEndian, transactionID)

	if _, err := conn.Write(connectReq.Bytes()); err != nil {
		fmt.Printf("failed to write connect request: %v\n", err)
		return
	}

	resp := make([]byte, 1024)
	n, err := conn.Read(resp)
	fmt.Printf("read %d bytes from connect response\n", n)
	if err != nil {
		fmt.Printf("failed to read connect response: %v\n", err)
		return
	}

	if n < 16 {
		// invalid response
		// refer UDP tracker Protocol
		return
	}

	receivedTransactionID := binary.BigEndian.Uint32(resp[4:8])
	connectionID := binary.BigEndian.Uint64(resp[8:16])

	if receivedTransactionID != transactionID {
		return
	}

	// 2. Announce Phase
	announceReq := new(bytes.Buffer)
	binary.Write(announceReq, binary.BigEndian, connectionID)
	binary.Write(announceReq, binary.BigEndian, uint32(1)) // action: announce
	binary.Write(announceReq, binary.BigEndian, transactionID)
	announceReq.Write(infoHash)
	announceReq.Write([]byte(PeerID))
	binary.Write(announceReq, binary.BigEndian, uint64(0))      // downloaded
	binary.Write(announceReq, binary.BigEndian, uint64(length)) // left
	binary.Write(announceReq, binary.BigEndian, uint64(0))      // uploaded
	binary.Write(announceReq, binary.BigEndian, uint32(0))      // event: none
	binary.Write(announceReq, binary.BigEndian, uint32(0))      // IP address: default
	binary.Write(announceReq, binary.BigEndian, rand.Uint32())  // key
	binary.Write(announceReq, binary.BigEndian, int32(-1))      // num_want: default
	binary.Write(announceReq, binary.BigEndian, uint16(6881))   // port

	if _, err := conn.Write(announceReq.Bytes()); err != nil {
		fmt.Printf("failed to write announce request: %v\n", err)
		return
	}

	n, err = conn.Read(resp)
	if err != nil {
		fmt.Printf("failed to read announce response: %v\n", err)
		return
	}
	fmt.Printf("read %d bytes from announce response\n", n)

	if n < 20 {
		return
	}

	action := binary.BigEndian.Uint32(resp[0:4])
	if action != 1 {
		return
	}
	fmt.Printf("action: %d\n", action)

	peersRaw := resp[20:n]
	for i := 0; i+6 <= len(peersRaw); i += 6 {
		ip := net.IP(peersRaw[i : i+4])
		port := binary.BigEndian.Uint16(peersRaw[i+4 : i+6])
		fmt.Printf("%s:%d\n", ip.String(), port)
	}
}

func httpTrackerRequest(announceURL string, infoHash []byte, length int) {
	u, err := url.Parse(announceURL)
	if err != nil {
		fmt.Printf("failed to parse announce URL: %v\n", err)
		return
	}

	params := url.Values{}
	params.Set("info_hash", string(infoHash))
	params.Set("peer_id", PeerID)
	params.Set("port", "6881")
	params.Set("uploaded", "0")
	params.Set("downloaded", "0")
	params.Set("left", strconv.Itoa(length))
	params.Set("compact", "1")

	u.RawQuery = params.Encode()

	resp, err := http.Get(u.String())
	if err != nil {
		fmt.Printf("tracker request error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("tracker returned status %d\n", resp.StatusCode)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("failed to read tracker response: %v\n", err)
		return
	}

	val, _, err := bencode.Decode(body)
	if err != nil {
		fmt.Printf("failed to decode tracker response: %v\n", err)
		return
	}

	dict, ok := val.(map[string]interface{})
	if !ok {
		fmt.Println("tracker response is not a dictionary")
		return
	}

	if failure, ok := dict["failure reason"].([]byte); ok {
		fmt.Printf("tracker failure: %s\n", string(failure))
		return
	}

	peersRaw, ok := dict["peers"].([]byte)
	if !ok {
		fmt.Println("peers field missing or not a string")
		return
	}

	for i := 0; i+6 <= len(peersRaw); i += 6 {
		ip := net.IP(peersRaw[i : i+4])
		port := binary.BigEndian.Uint16(peersRaw[i+4 : i+6])
		fmt.Printf("%s:%d\n", ip.String(), port)
	}
}

func handshake(peer string, infoHash []byte) {
	conn, err := net.DialTimeout("tcp", peer, 3*time.Second)
	if err != nil {
		fmt.Printf("failed to connect to peer: %v\n", err)
		return
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(5 * time.Second))

	// Assemble handshake message:
	// 1 byte: length of protocol string (19)
	// 19 bytes: protocol string ("BitTorrent protocol")
	// 8 bytes: reserved (zeroes)
	// 20 bytes: info hash
	// 20 bytes: peer id
	req := make([]byte, 68)
	req[0] = 19
	copy(req[1:20], "BitTorrent protocol")
	// bytes 20-28 are reserved (zeroes)
	copy(req[28:48], infoHash)
	copy(req[48:68], []byte(PeerID))

	if _, err := conn.Write(req); err != nil {
		fmt.Printf("failed to write handshake: %v\n", err)
		return
	}

	resp := make([]byte, 68)
	if _, err := io.ReadFull(conn, resp); err != nil {
		fmt.Printf("failed to read handshake response: %v\n", err)
		return
	}

	remotePeerID := resp[48:68]
	fmt.Printf("Peer ID: %x\n", remotePeerID)
}

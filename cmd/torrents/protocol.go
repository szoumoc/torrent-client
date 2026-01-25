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

	if n < 20 {
		return
	}

	action := binary.BigEndian.Uint32(resp[0:4])
	if action != 1 {
		return
	}

	peersRaw := resp[20:n]
	for i := 0; i+6 <= len(peersRaw); i += 6 {
		ip := net.IP(peersRaw[i : i+4])
		port := binary.BigEndian.Uint16(peersRaw[i+4 : i+6])
		fmt.Printf("%s:%d\n", ip.String(), port)
		tcpTrackerRequest(u.String(), infoHash, length)
	}
}

func tcpTrackerRequest(announce string, infoHash []byte, length int) {
	params := url.Values{}
	params.Set("info_hash", string(infoHash))
	params.Set("peer_id", PeerID)
	params.Set("port", strconv.Itoa(6881))
	params.Set("uploaded", "0")
	params.Set("downloaded", "0")
	params.Set("left", strconv.Itoa(length))
	params.Set("compact", "1")

	u, _ := url.Parse(announce)
	u.RawQuery = params.Encode()

	resp, err := http.Get(u.String())
	if err != nil {
		fmt.Printf("tracker request error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	val, _, err := bencode.Decode(body)
	if err != nil {
		return
	}

	dict, ok := val.(map[string]interface{})
	if !ok {
		return
	}

	peersRaw, ok := dict["peers"].([]byte)
	if !ok {
		return
	}

	for i := 0; i+6 <= len(peersRaw); i += 6 {
		ip := net.IP(peersRaw[i : i+4])
		port := int(peersRaw[i+4])<<8 | int(peersRaw[i+5])
		fmt.Printf("%s:%d\n", ip.String(), port)
	}
}

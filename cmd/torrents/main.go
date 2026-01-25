package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"torrent/cmd/pkg/bencode"
)

const PeerID = "-GT0001-123456789012"

func main() {
	torrentFile := "/Users/szoumo/Downloads/big-buck-bunny.torrent"
	data, err := os.ReadFile(torrentFile)
	if err != nil {
		fmt.Printf("failed to read torrent file: %v\n", err)
		os.Exit(1)
	}

	announces, length, infoHash, err := ParseTorrent(data)
	if err != nil {
		fmt.Printf("failed to parse torrent: %v\n", err)
		os.Exit(1)
	}

	// Re-parsing the info map for piece extraction (since ParseTorrent returns simplified values)
	val, _, _ := bencode.Decode(data)
	root, _ := val.(map[string]interface{})
	info, _ := root["info"].(map[string]interface{})

	pieceLength, hashes, err := extractPieces(info)
	if err == nil {
		fmt.Println("Trackers:", announces)
		fmt.Println("Length:", length)
		fmt.Println("Info Hash (hex):", hex.EncodeToString(infoHash))
		fmt.Println("Piece Length:", pieceLength)
		fmt.Println("Piece Hashes:", hashes[0], "...", hashes[len(hashes)-1])
	}

	for _, announce := range announces {
		if strings.HasPrefix(announce, "udp://") {
			udpTrackerRequest(announce, infoHash, length)
		} else {
			tcpTrackerRequest(announce, infoHash, length)
		}
	}
}

func ParseTorrent(data []byte) ([]string, int, []byte, error) {
	val, _, err := bencode.Decode(data)
	if err != nil {
		return nil, 0, nil, err
	}

	root, ok := val.(map[string]interface{})
	if !ok {
		return nil, 0, nil, fmt.Errorf("root is not dictionary")
	}

	var announces []string
	if announceList, ok := root["announce-list"].([]interface{}); ok {
		for _, tier := range announceList {
			if trackers, ok := tier.([]interface{}); ok {
				for _, tracker := range trackers {
					if tBytes, ok := tracker.([]byte); ok {
						announces = append(announces, string(tBytes))
					}
				}
			}
		}
	}

	if len(announces) == 0 {
		announceBytes, ok := root["announce"].([]byte)
		if !ok {
			return nil, 0, nil, fmt.Errorf("announce missing")
		}
		announces = append(announces, string(announceBytes))
	}

	info, ok := root["info"].(map[string]interface{})
	if !ok {
		return nil, 0, nil, fmt.Errorf("info missing")
	}

	// find raw bencoded "info" bytes in original torrent data
	idx := bytes.Index(data, []byte("4:info"))
	if idx < 0 {
		return nil, 0, nil, fmt.Errorf("couldn't find info offset in torrent")
	}
	infoStart := idx + len("4:info")
	_, consumed, err := bencode.Decode(data[infoStart:])
	if err != nil {
		return nil, 0, nil, fmt.Errorf("failed decoding raw info: %w", err)
	}
	infoBytes := data[infoStart : infoStart+consumed]

	h := sha1.Sum(infoBytes)
	infoHash := h[:]

	length, err := extractLength(info)
	if err != nil {
		return nil, 0, nil, err
	}

	return announces, length, infoHash, nil
}

func extractLength(info map[string]interface{}) (int, error) {
	if l, ok := info["length"].(int); ok {
		return l, nil
	}

	filesRaw, ok := info["files"].([]interface{})
	if !ok {
		return 0, fmt.Errorf("no length or files field")
	}

	total := 0
	for _, f := range filesRaw {
		fileDict, ok := f.(map[string]interface{})
		if !ok {
			return 0, fmt.Errorf("invalid file entry")
		}

		l, ok := fileDict["length"].(int)
		if !ok {
			return 0, fmt.Errorf("file length missing")
		}

		total += l
	}

	return total, nil
}

func extractPieces(info map[string]interface{}) (int, []string, error) {
	pieceLength, ok := info["piece length"].(int)
	if !ok {
		return 0, nil, fmt.Errorf("piece length missing")
	}

	piecesRaw, ok := info["pieces"].([]byte)
	if !ok {
		return 0, nil, fmt.Errorf("pieces missing")
	}

	if len(piecesRaw)%20 != 0 {
		return 0, nil, fmt.Errorf("invalid pieces length")
	}

	hashes := make([]string, 0, len(piecesRaw)/20)
	for i := 0; i < len(piecesRaw); i += 20 {
		hashes = append(hashes, hex.EncodeToString(piecesRaw[i:i+20]))
	}

	return pieceLength, hashes, nil
}

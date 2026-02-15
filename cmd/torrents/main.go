package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"torrent/cmd/pkg/bencode"
)

func main() {
	fmt.Println("DEBUG ARGS:", os.Args)
	if len(os.Args) < 3 {
		fmt.Println("Usage: ./your_program.sh <command> <torrent_file> [extra_args...]")
		os.Exit(1)
	}

	command := os.Args[1]

	var torrentFile string
	if command == "download_piece" {
		if len(os.Args) < 5 {
			// Let the case handle the error details or just safe guard here
		} else {
			torrentFile = os.Args[4]
		}
	} else if len(os.Args) >= 3 {
		torrentFile = os.Args[2]
	}

	// We'll read and parse inside cases or just here if we have a file
	var announces []string
	var length int
	var infoHash []byte
	var data []byte
	var err error

	if torrentFile != "" {
		data, err = os.ReadFile(torrentFile)
		if err != nil {
			fmt.Printf("failed to read torrent file: %v\n", err)
			os.Exit(1)
		}

		announces, length, infoHash, err = ParseTorrent(data)
		if err != nil {
			fmt.Printf("failed to parse torrent: %v\n", err)
			os.Exit(1)
		}
	}

	switch command {
	case "info":
		fmt.Println("Trackers:", announces)
		fmt.Println("Length:", length)
		fmt.Println("Info Hash (hex):", hex.EncodeToString(infoHash))
		// Re-parsing the info map for piece extraction
		val, _, _ := bencode.Decode(data)
		root, _ := val.(map[string]interface{})
		info, _ := root["info"].(map[string]interface{})
		pieceLength, hashes, _ := extractPieces(info)
		fmt.Println("Piece Length:", pieceLength)
		fmt.Println("Piece Hashes:", hashes[0], "...", hashes[len(hashes)-1])

	case "peers":
		for _, announce := range announces {
			peers, err := getPeers(announce, infoHash, length)
			if err != nil {
				// verify if we should print errors or just continue
				// for this challenge, errors on some trackers are expected
				continue
			}
			for _, peer := range peers {
				fmt.Println(peer)
			}
		}

	case "handshake":
		if len(os.Args) < 4 {
			fmt.Println("Usage: handshake <torrent_file> <peer_ip:port>")
			os.Exit(1)
		}
		peer := os.Args[3]
		handshake(peer, infoHash)

	case "download_piece":
		if len(os.Args) < 6 || os.Args[2] != "-o" {
			fmt.Println("Usage: download_piece -o <output_path> <torrent_file> <piece_index>")
			os.Exit(1)
		}
		outputPath := os.Args[3]
		torrentFile := os.Args[4]
		pieceIndexStr := os.Args[5]
		pieceIndex, err := strconv.Atoi(pieceIndexStr)
		if err != nil {
			fmt.Println("Invalid piece index")
			os.Exit(1)
		}

		// Re-parse to ensure we have the correct file info for validation if needed
		data, _ := os.ReadFile(torrentFile)
		val, _, _ := bencode.Decode(data)
		root, _ := val.(map[string]interface{})
		info, _ := root["info"].(map[string]interface{})
		pieceLength, _, _ := extractPieces(info)

		// We need to find a peer first
		// detailed logic will be inside downloadPiece which handles connection and download
		// For this stage, we might need to reuse peer discovery or just pick one.
		// The prompt implies we need to do discovery -> handshake -> download.
		// Let's assume we pick the first available peer for now.
		peer := ""
		for _, announce := range announces {
			peers, err := getPeers(announce, infoHash, length)
			if err != nil {
				fmt.Printf("Error getting peers from %s: %v\n", announce, err)
				continue
			}
			if len(peers) > 0 {
				peer = peers[0]
				break
			}
		}

		if peer == "" {
			fmt.Println("No peers found")
			os.Exit(1)
		}

		err = downloadPiece(peer, infoHash, pieceIndex, pieceLength, length, outputPath)
		if err != nil {
			fmt.Printf("Failed to download piece: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Piece %d downloaded to %s\n", pieceIndex, outputPath)

	default:
		fmt.Printf("unknown command: %s\n", command)
		os.Exit(1)
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

package main

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"torrent/cmd/pkg/bencode"
)

func main() {
	data, err := os.ReadFile("/Users/szoumo/Downloads/big-buck-bunny.torrent")
	if err != nil {
		panic(err)
	}

	announce, length, infoHash, info, err := ParseTorrent(data)
	if err != nil {
		panic(err)
	}

	pieceLength, hashes, err := extractPieces(info)
	if err != nil {
		panic(err)
	}

	fmt.Println("Tracker URL:", announce)
	fmt.Println("Length:", length)
	fmt.Println("Info Hash:", infoHash)
	fmt.Println("Piece Length:", pieceLength)
	fmt.Println("Piece Hashes:")
	for _, h := range hashes {
		fmt.Println(h)
	}
}

func ParseTorrent(data []byte) (string, int, string, map[string]interface{}, error) {
	val, _, err := bencode.Decode(data)
	if err != nil {
		return "", 0, "", nil, err
	}

	root, ok := val.(map[string]interface{})
	if !ok {
		return "", 0, "", nil, fmt.Errorf("root is not dictionary")
	}

	announceBytes, ok := root["announce"].([]byte)
	if !ok {
		return "", 0, "", nil, fmt.Errorf("announce missing")
	}

	info, ok := root["info"].(map[string]interface{})
	if !ok {
		return "", 0, "", nil, fmt.Errorf("info missing")
	}

	infoBytes, err := bencode.Encode(info)
	if err != nil {
		return "", 0, "", nil, err
	}
	h := sha1.Sum(infoBytes)
	infoHash := hex.EncodeToString(h[:])

	length, err := extractLength(info)
	if err != nil {
		return "", 0, "", nil, err
	}

	return string(announceBytes), length, infoHash, info, nil
}

func extractLength(info map[string]interface{}) (int, error) {
	// single-file torrent
	if l, ok := info["length"].(int); ok {
		return l, nil
	}

	// multi-file torrent
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

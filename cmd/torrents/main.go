package main

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"torrent/cmd/pkg/bencode"
)

func main() {
	data, _ := os.ReadFile("/Users/szoumo/Downloads/big-buck-bunny.torrent")
	// Process the data here
	announce, length, info, err := ParseTorrent(data)
	if err != nil {
		fmt.Println("Error parsing torrent:", err)
		return
	}

	fmt.Println("Announce URL:", announce)
	fmt.Println("File Length:", length)
	fmt.Println("Info Dictionary:", info)

}

func ParseTorrent(data []byte) (string, int, string, error) {
	val, _, err := bencode.Decode(data)
	if err != nil {
		return "", 0, "", err
	}

	root, ok := val.(map[string]interface{})
	if !ok {
		return "", 0, "", fmt.Errorf("root is not dictionary")
	}

	announceBytes, ok := root["announce"].([]byte)
	if !ok {
		return "", 0, "", fmt.Errorf("announce missing")
	}

	info, ok := root["info"].(map[string]interface{})
	if !ok {
		return "", 0, "", fmt.Errorf("info missing")
	}
	infoBytes, _ := bencode.Encode(info)
	h := sha1.Sum(infoBytes)
	infoHash := hex.EncodeToString(h[:])

	length, err := extractLength(info)
	if err != nil {
		return "", 0, "", fmt.Errorf("length missing")
	}

	return string(announceBytes), length, infoHash, nil
}

func extractLength(info map[string]interface{}) (int, error) {
	// single-file torrent
	if l, ok := info["length"].(int); ok {
		return l, nil
	}

	// multi-file torrent
	filesRaw, ok := info["files"].([]interface{})
	if !ok {
		return 0, fmt.Errorf("no length or files field in info")
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

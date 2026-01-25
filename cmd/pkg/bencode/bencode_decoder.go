// ...existing code...
package bencode

import (
	"fmt"
	"io"
	"log"
	"strconv"
)

var logger = log.New(io.Discard, "bencode: ", log.LstdFlags)

func Decode(s []byte) (interface{}, int, error) {
	if len(s) < 1 {
		logger.Println("Decode: input empty")
		return nil, 0, fmt.Errorf("input byte array is empty")
	}

	logger.Printf("Decode: len=%d first=%q", len(s), s[0])

	switch firstChar := s[0]; {
	case firstChar == 'i':
		return decodeInteger(s)
	case firstChar >= '0' && firstChar <= '9':
		return decodeByteString(s)
	case firstChar == 'd':
		return decodeDict(s)
	case firstChar == 'l':
		return decodeList(s)
	}

	logger.Printf("Decode: invalid starting character: %q", s[0])
	return nil, 0, fmt.Errorf("couldn't decode due to invalid starting character (must be i, d, l or a number): %s", s)
}

func findFirstByte(s []byte, b byte) (int, bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i, true
		}
	}
	return -1, false
}

func decodeInteger(s []byte) (int, int, error) {
	if s[0] != 'i' {
		logger.Printf("decodeInteger: bad start: %q", s[0])
		return 0, 0, fmt.Errorf("expected string to start with 'i': %s", s)
	}

	end := 0
	for i := 1; i < len(s); i++ {
		if s[i] == 'e' {
			end = i
			break
		}
	}

	if end == 0 {
		logger.Printf("decodeInteger: unterminated integer: %s", s)
		return 0, 0, fmt.Errorf("no end character found in s: %v", s)
	}

	logger.Printf("decodeInteger: raw=%q", s[:end+1])

	val, err := strconv.Atoi(string(s[1:end]))
	if err != nil {
		logger.Printf("decodeInteger: parse error: %v", err)
		return 0, 0, err
	}

	logger.Printf("decodeInteger: parsed=%d consumed=%d", val, end+1)
	return val, end + 1, nil
}

func decodeByteString(s []byte) ([]byte, int, error) {
	delimPos, ok := findFirstByte(s, ':')
	if !ok {
		logger.Printf("decodeByteString: missing ':' in %q", s)
		return nil, 0, fmt.Errorf("can't find ':' delimeter in: %s", s)
	}

	l, err := strconv.Atoi(string(s[:delimPos]))
	if err != nil {
		logger.Printf("decodeByteString: invalid length %q: %v", s[:delimPos], err)
		return nil, 0, fmt.Errorf("couldn't decode byte string : %s, err: %w", s, err)
	}

	logger.Printf("decodeByteString: declared length=%d", l)

	start := delimPos + 1
	end := start + l
	if end > len(s) {
		logger.Printf("decodeByteString: unexpected EOF reading string declared=%d available=%d", l, len(s)-start)
		return nil, 0, fmt.Errorf("can't find ':' delimeter in: %s", s)
	}

	byteString := make([]byte, l)
	n := copy(byteString, s[start:end])
	if n != l {
		logger.Printf("decodeByteString: copy mismatch copied=%d expected=%d", n, l)
		return nil, 0, fmt.Errorf("failed to copy all bytes to bytestring for: %s", s)
	}

	logger.Printf("decodeByteString: consumed=%d", end)
	return byteString, end, nil
}

func decodeDict(in []byte) (map[string]interface{}, int, error) {
	s := in
	if len(s) == 0 || s[0] != 'd' {
		logger.Printf("decodeDict: bad start or empty: %q", s)
		return nil, 0, fmt.Errorf("expected strint to start with 'd': %s", s)
	}

	logger.Println("decodeDict: start")
	totalConsumed := 1
	s = s[1:]
	rv := make(map[string]interface{})

	for {
		if len(s) == 0 {
			logger.Println("decodeDict: unexpected EOF while reading key")
			return nil, 0, fmt.Errorf("unterminated dict")
		}

		// check for end
		if s[0] == 'e' {
			totalConsumed += 1
			logger.Println("decodeDict: end")
			break
		}

		key, consumed, err := decodeByteString(s)
		if err != nil {
			logger.Printf("decodeDict: error decoding key: %v", err)
			return nil, 0, fmt.Errorf("error decoding key: %s error: %w", s, err)
		}

		totalConsumed += consumed
		s = s[consumed:]

		logger.Printf("decodeDict: decoded key=%q", string(key))

		val, consumed, err := Decode(s)
		if err != nil {
			logger.Printf("decodeDict: error decoding value for key=%q: %v", string(key), err)
			return nil, 0, err
		}

		totalConsumed += consumed
		s = s[consumed:]

		rv[string(key)] = val
	}

	return rv, totalConsumed, nil
}

func decodeList(in []byte) ([]interface{}, int, error) {
	s := in
	if len(s) == 0 || s[0] != 'l' {
		logger.Printf("decodeList: bad start or empty: %q", s)
		return nil, 0, fmt.Errorf("expecte string to start with 'l': %s", s)
	}

	logger.Println("decodeList: start")
	totalConsumed := 1
	s = s[1:]
	var rv []interface{}
	for {
		if len(s) == 0 {
			logger.Println("decodeList: unexpected EOF while reading element")
			return nil, 0, fmt.Errorf("unterminated list")
		}

		if s[0] == 'e' {
			totalConsumed += 1
			logger.Println("decodeList: end")
			break
		}

		val, consumed, err := Decode(s)
		if err != nil {
			logger.Printf("decodeList: error decoding element: %v", err)
			return nil, 0, err
		}

		rv = append(rv, val)
		totalConsumed += consumed
		s = s[consumed:]
	}

	logger.Printf("decodeList: elements=%d consumed=%d", len(rv), totalConsumed)
	return rv, totalConsumed, nil
}

// ...existing code...

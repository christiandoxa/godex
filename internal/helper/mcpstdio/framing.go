package mcpstdio

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	MessageMaxBytes         = 64 * 1024 * 1024
	FirstHeaderLineMaxBytes = 4 * 1024
	HeaderLineMaxBytes      = 16 * 1024
	FramingMetadataMaxBytes = 64 * 1024
)

type Framing uint8

const (
	JSONLine Framing = iota
	ContentLength
)

func ReadMessage(reader *bufio.Reader) ([]byte, Framing, error) {
	framingBytes := 0
	first, found, err := readFirstLine(reader, &framingBytes)
	if err != nil || !found {
		return nil, JSONLine, err
	}
	if headerIsContentLength(first) {
		length, err := parseContentLength(first)
		if err != nil {
			return nil, ContentLength, err
		}
		if length > MessageMaxBytes {
			return nil, ContentLength, fmt.Errorf("MCP message exceeds safe size limit (%d bytes)", MessageMaxBytes)
		}
		for {
			header, found, err := readLimitedLine(reader, HeaderLineMaxBytes)
			if err != nil {
				return nil, ContentLength, err
			}
			if !found {
				break
			}
			if err := chargeFraming(&framingBytes, len(header)); err != nil {
				return nil, ContentLength, err
			}
			if strings.TrimSpace(header) == "" {
				break
			}
			if headerIsContentLength(header) {
				return nil, ContentLength, errors.New("duplicate MCP Content-Length header")
			}
		}
		body := make([]byte, length)
		if _, err := io.ReadFull(reader, body); err != nil {
			return nil, ContentLength, err
		}
		canonical, err := canonicalJSON(body)
		if err != nil {
			return nil, ContentLength, fmt.Errorf("failed to parse MCP JSON body: %w", err)
		}
		return canonical, ContentLength, nil
	}
	canonical, err := canonicalJSON([]byte(strings.TrimSpace(first)))
	if err != nil {
		return nil, JSONLine, fmt.Errorf("failed to parse MCP JSON line: %w", err)
	}
	return canonical, JSONLine, nil
}

func WriteMessage(writer io.Writer, message []byte, framing Framing) error {
	canonical, err := canonicalJSON(message)
	if err != nil {
		return fmt.Errorf("failed to serialize MCP response: %w", err)
	}
	switch framing {
	case JSONLine:
		if _, err := writer.Write(canonical); err != nil {
			return err
		}
		if _, err := writer.Write([]byte{'\n'}); err != nil {
			return err
		}
	case ContentLength:
		if _, err := fmt.Fprintf(writer, "Content-Length: %d\r\n\r\n", len(canonical)); err != nil {
			return err
		}
		if _, err := writer.Write(canonical); err != nil {
			return err
		}
	default:
		return errors.New("invalid MCP message framing")
	}
	if flusher, ok := writer.(interface{ Flush() error }); ok {
		return flusher.Flush()
	}
	return nil
}

func canonicalJSON(content []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("multiple JSON values")
		}
		return nil, err
	}
	return json.Marshal(value)
}

func readFirstLine(reader *bufio.Reader, framingBytes *int) (string, bool, error) {
	for {
		line, found, err := readPhysicalFirstLine(reader)
		if err != nil || !found {
			return "", found, err
		}
		trimmed := strings.TrimSpace(line)
		first := firstNonWhitespace(line)
		if trimmed == "" || (first != 0 && first != '{' && first != '[') {
			if err := chargeFraming(framingBytes, len(line)); err != nil {
				return "", false, err
			}
		}
		if trimmed != "" {
			return line, true, nil
		}
	}
}

func readPhysicalFirstLine(reader *bufio.Reader) (string, bool, error) {
	content := make([]byte, 0, 256)
	first := byte(0)
	limit := FirstHeaderLineMaxBytes
	for {
		value, err := reader.ReadByte()
		if errors.Is(err, io.EOF) {
			if len(content) == 0 {
				return "", false, nil
			}
			return string(content), true, nil
		}
		if err != nil {
			return "", false, err
		}
		content = append(content, value)
		if first == 0 && !isASCIIWhitespace(value) {
			first = value
			if first == '{' || first == '[' {
				limit = MessageMaxBytes
			}
		}
		if len(content) > limit {
			return "", false, fmt.Errorf("MCP first line exceeds safe size limit (%d bytes)", limit)
		}
		if value == '\n' {
			return string(content), true, nil
		}
	}
}

func readLimitedLine(reader *bufio.Reader, limit int) (string, bool, error) {
	content := make([]byte, 0, min(limit, 256))
	for {
		value, err := reader.ReadByte()
		if errors.Is(err, io.EOF) {
			if len(content) == 0 {
				return "", false, nil
			}
			return string(content), true, nil
		}
		if err != nil {
			return "", false, err
		}
		content = append(content, value)
		if len(content) > limit {
			return "", false, fmt.Errorf("MCP line exceeds safe size limit (%d bytes)", limit)
		}
		if value == '\n' {
			return string(content), true, nil
		}
	}
}

func chargeFraming(total *int, amount int) error {
	next := *total + amount
	if next < *total || next > FramingMetadataMaxBytes {
		return fmt.Errorf("MCP framing headers exceed safe size limit (%d bytes)", FramingMetadataMaxBytes)
	}
	*total = next
	return nil
}

func firstNonWhitespace(value string) byte {
	for index := 0; index < len(value); index++ {
		if !isASCIIWhitespace(value[index]) {
			return value[index]
		}
	}
	return 0
}

func isASCIIWhitespace(value byte) bool {
	switch value {
	case ' ', '\t', '\r', '\n', '\v', '\f':
		return true
	default:
		return false
	}
}

func headerIsContentLength(line string) bool {
	name, _, ok := strings.Cut(line, ":")
	return ok && strings.EqualFold(strings.TrimSpace(name), "Content-Length")
}

func parseContentLength(line string) (int, error) {
	name, raw, ok := strings.Cut(line, ":")
	if !ok || !strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
		return 0, errors.New("invalid Content-Length header")
	}
	value := strings.TrimSpace(raw)
	if value == "" || strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-") {
		return 0, errors.New("invalid Content-Length value")
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil || parsed > uint64(^uint(0)>>1) {
		return 0, errors.New("invalid Content-Length value")
	}
	return int(parsed), nil
}

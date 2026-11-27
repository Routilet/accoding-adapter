// Package wire implements the phase-01 transport, not an OJ business API.
package wire

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/Routilet/accoding-adapter/accoding"
	"io"
	"unicode/utf8"
)

const Protocol = "routilet-oj-adapter/1"

// Development marker, replaced when an actual release is built.
var Version = "0.0.0-dev"

const MaxLine = 8 << 20 // bytes, excluding LF

type Request struct {
	ID     uint64          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}
type Failure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type Response struct {
	ID     uint64          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *Failure        `json:"error,omitempty"`
}
type Capabilities struct {
	Protocol string   `json:"protocol"`
	Adapter  string   `json:"adapter"`
	Version  string   `json:"version"`
	Methods  []string `json:"methods"`
}

func decode(line []byte, v any) error {
	if !utf8.Valid(line) {
		return errors.New("invalid UTF-8")
	}
	d := json.NewDecoder(bytes.NewReader(line))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}

// Serve reads one request at a time. Malformed framing terminates the stream.
// No startup/background networking or submission methods.
func Serve(in io.Reader, out io.Writer) error {
	return serve(in, out, accoding.New())
}

func serve(in io.Reader, out io.Writer, client accountClient) error {
	defer client.Logout()
	a := accountHandler{client: client}
	s := bufio.NewScanner(in)
	s.Buffer(make([]byte, 4096), MaxLine+2)
	s.Split(splitLine)
	var last uint64
	for s.Scan() {
		line := s.Bytes()
		var req Request
		if len(line) > MaxLine || decode(line, &req) != nil || req.ID <= last || req.Method == "" || len(req.Params) == 0 || req.Params[0] != '{' {
			return errors.New("invalid request frame")
		}
		last = req.ID
		resp := Response{ID: req.ID}
		switch req.Method {
		case "capabilities":
			var p struct {
				Protocol string `json:"protocol"`
			}
			if decode(req.Params, &p) != nil {
				resp.Error = &Failure{"invalid_params", "invalid capabilities parameters"}
			} else if p.Protocol != Protocol {
				resp.Error = &Failure{"incompatible_protocol", "unsupported protocol"}
			} else {
				resp.Result, _ = json.Marshal(Capabilities{Protocol, "accoding", Version, []string{"capabilities", "account", "login", "logout", "resolve", "import"}})
			}
		case "resolve", "import":
			resp.Result, resp.Error = a.importRequest(context.Background(), req.Method, req.Params)
		case "account", "login", "logout":
			resp.Result, resp.Error = a.handle(context.Background(), req.Method, req.Params)
		default:
			resp.Error = &Failure{"method_not_found", "method not available"}
		}
		b, err := json.Marshal(resp)
		if err != nil {
			return err
		}
		if len(b) > MaxLine {
			return errors.New("response too large")
		}
		b = append(b, '\n')
		n, err := out.Write(b)
		if err != nil {
			return err
		}
		if n != len(b) {
			return io.ErrShortWrite
		}
	}
	return s.Err()
}

// Require a complete LF-delimited frame, even at EOF.
func splitLine(data []byte, atEOF bool) (int, []byte, error) {
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF && len(data) > 0 {
		return 0, nil, errors.New("incomplete request frame")
	}
	return 0, nil, nil
}

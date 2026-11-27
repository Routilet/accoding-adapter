package wire

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestHandshakeAndErrors(t *testing.T) {
	var out bytes.Buffer
	in := `{"id":1,"method":"capabilities","params":{"protocol":"routilet-oj-adapter/1"}}
{"id":2,"method":"submit","params":{}}
{"id":3,"method":"capabilities","params":{"protocol":"v0"}}
{"id":4,"method":"capabilities","params":{"protocol":"routilet-oj-adapter/1","extra":true}}
`
	if err := Serve(strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	d := json.NewDecoder(&out)
	var r Response
	if err := d.Decode(&r); err != nil {
		t.Fatal(err)
	}
	var caps Capabilities
	if r.ID != 1 || r.Error != nil || json.Unmarshal(r.Result, &caps) != nil || caps.Adapter != "accoding" || caps.Protocol != Protocol || len(caps.Methods) != 1 {
		t.Fatalf("bad handshake: %+v", r)
	}
	for i, want := range []string{"method_not_found", "incompatible_protocol", "invalid_params"} {
		r = Response{}
		if err := d.Decode(&r); err != nil || r.ID != uint64(i+2) || r.Error == nil || r.Error.Code != want || r.Result != nil {
			t.Fatalf("bad failure: %+v %v", r, err)
		}
	}
}
func TestBadFrames(t *testing.T) {
	for _, s := range []string{`{"id":0,"method":"x","params":{}}`, `{"id":1,"method":"x","params":null}`, `{"id":1,"method":"x","params":{},"extra":true}`, `{"id":1,"method":"x","params":{}} {}`, `{`, string([]byte{0xff}), strings.Repeat("x", MaxLine+1), "{\"id\":1,\"method\":\"x\",\"params\":{}}\n{\"id\":1,\"method\":\"x\",\"params\":{}}"} {
		if err := Serve(strings.NewReader(s+"\n"), &bytes.Buffer{}); err == nil {
			t.Fatal("accepted bad frame")
		}
	}
}

func TestUnterminated(t *testing.T) {
	if err := Serve(strings.NewReader(`{"id":1,"method":"capabilities","params":{"protocol":"routilet-oj-adapter/1"}}`), &bytes.Buffer{}); err == nil {
		t.Fatal("accepted partial frame")
	}
}

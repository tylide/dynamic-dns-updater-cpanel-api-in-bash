package cpanel_test

import (
	"encoding/base64"
	"testing"

	"dnsupdater/internal/cpanel"
)

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func TestFindRecordShortAndFQDN(t *testing.T) {
	records := []cpanel.ZoneRecord{
		{DnameB64: b64("homeip.exemplo.pt."), DataB64: []string{b64("1.2.3.4")}, LineIndex: 7, RecordType: "A"},
		{DnameB64: b64("outro.exemplo.pt."), DataB64: []string{b64("9.9.9.9")}, LineIndex: 8, RecordType: "A"},
	}
	for _, sub := range []string{"homeip", "homeip.exemplo.pt", "homeip.exemplo.pt."} {
		f, err := cpanel.FindRecord(records, sub, "exemplo.pt")
		if err != nil {
			t.Fatalf("sub=%q: %v", sub, err)
		}
		if f.IP != "1.2.3.4" || f.LineIndex != 7 {
			t.Fatalf("sub=%q got %+v", sub, f)
		}
	}
}

func TestFindRecordNotFound(t *testing.T) {
	records := []cpanel.ZoneRecord{
		{DnameB64: b64("outro.exemplo.pt."), DataB64: []string{b64("9.9.9.9")}, LineIndex: 8, RecordType: "A"},
	}
	_, err := cpanel.FindRecord(records, "homeip", "exemplo.pt")
	if !cpanel.IsNotFound(err) {
		t.Fatalf("esperava not found, got %v", err)
	}
}

func TestSerial(t *testing.T) {
	records := []cpanel.ZoneRecord{
		{RecordType: "SOA", DataB64: []string{b64("ns"), b64("adm"), b64("2024010101")}},
	}
	s, err := cpanel.Serial(records)
	if err != nil || s != "2024010101" {
		t.Fatalf("serial=%q err=%v", s, err)
	}
}

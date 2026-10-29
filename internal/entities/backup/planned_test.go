package backup_test

import (
	"crypto/sha256"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/savewarden/savewarden-cli/internal/entities/backup"
)

func TestPlannedRoundTripAndDigest(t *testing.T) {
	for _, rotation := range []bool{false, true} {
		plan := testPlan(rotation)
		data, sum, err := plan.Encode()
		if err != nil {
			t.Fatal(err)
		}
		if sum != sha256.Sum256(data) || strings.Contains(string(data), "\n") {
			t.Fatalf("plan form: %s", data)
		}
		decoded, err := backup.DecodePlanned(data)
		if err != nil || !reflect.DeepEqual(&decoded, plan) {
			t.Fatalf("decoded %+v, %v", decoded, err)
		}
		again, sum2, err := decoded.Encode()
		if err != nil || string(again) != string(data) || sum2 != sum {
			t.Fatal("encoding is not stable")
		}
	}
}

func TestPlannedStrictness(t *testing.T) {
	data, _, err := testPlan(false).Encode()
	if err != nil {
		t.Fatal(err)
	}
	for name, bad := range map[string]string{
		"unknown key":        strings.Replace(string(data), `"changes"`, `"extra":1,"changes"`, 1),
		"key in capitals":    strings.Replace(string(data), `"game"`, `"Game"`, 1),
		"missing game":       strings.Replace(string(data), `"game":{"id":"synthetic-game","name":"Synthetic Game"},`, ``, 1),
		"empty tool":         strings.Replace(string(data), `"tool_version":"dev"`, `"tool_version":""`, 1),
		"previous not id":    strings.Replace(string(data), `"previous":null`, `"previous":"yesterday"`, 1),
		"wrong file count":   strings.Replace(string(data), `"files":1`, `"files":2`, 1),
		"trailing data":      string(data) + "{}",
		"negative counts":    strings.Replace(string(data), `"added":1`, `"added":-1`, 1),
		"unsorted entries":   strings.Replace(string(data), `"entries":[`, `"entries":[{"mod_time":"2026-11-05T10:15:00Z","path":"9.save","sha256":"`+strings.Repeat("0", 64)+`","size":0},`, 1),
		"unknown state":      strings.Replace(string(data), `"status":"copied"`, `"status":"lost"`, 1),
		"local time":         strings.Replace(string(data), `"started_at":"2026-11-05T10:15:00Z"`, `"started_at":"2026-11-05T11:15:00+01:00"`, 1),
		"duplicate key":      strings.Replace(string(data), `"previous":null`, `"previous":null,"previous":null`, 1),
		"null instead of []": strings.Replace(string(data), `"directories":[]`, `"directories":null`, 1),
	} {
		if bad == string(data) {
			t.Fatalf("%s: mutation did not apply", name)
		}
		if _, err := backup.DecodePlanned([]byte(bad)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

// The receipt of an operation is built from its plan, with the provenance
// of ADR 0007.
func TestReceiptFromPlan(t *testing.T) {
	plan := testPlan(false)
	_, sum, err := plan.Encode()
	if err != nil {
		t.Fatal(err)
	}
	intent := at.Add(time.Minute)
	published := intent.Add(time.Second)
	r, err := plan.Receipt(op, sum, intent, &published, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if r.PlanSHA256 != sum || !reflect.DeepEqual(r.Sources, plan.Sources) || r.Recovered() || r.Previous != "" {
		t.Fatalf("receipt %+v", r)
	}
	if _, err := backup.DecodeReceipt(must(backup.EncodeReceipt(r))); err != nil {
		t.Fatal(err)
	}
	recovered := intent.Add(time.Hour)
	rebuilt, err := plan.Receipt(op, sum, intent, nil, &recovered, "dev")
	if err != nil || !rebuilt.Recovered() || rebuilt.PublishedAt != nil {
		t.Fatalf("reconstructed %+v, %v", rebuilt, err)
	}
	if _, err := plan.Receipt(op, sum, intent, nil, nil, ""); err == nil {
		t.Fatal("receipt without publication or recovery date accepted")
	}
}

func TestContentDigestFollowsTheContent(t *testing.T) {
	a := testPlan(false).Sources
	b := testPlan(false).Sources
	da, _ := backup.ContentDigest(a)
	db, _ := backup.ContentDigest(b)
	if da != db {
		t.Fatal("digest not deterministic")
	}
	b[0].Entries[0].ModTime = b[0].Entries[0].ModTime.Add(100 * time.Nanosecond)
	if dc, _ := backup.ContentDigest(b); dc == da {
		t.Fatal("digest ignores the date")
	}
}

func must(data []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return data
}

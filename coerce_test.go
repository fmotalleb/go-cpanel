package cpanel

import (
	"encoding/json"
	"testing"
)

func TestRelaxedUnmarshalIntFields(t *testing.T) {
	type TestStruct struct {
		Uid        int64   `json:"uid"`
		MaxUsers   int64   `json:"max_users"`
		Name       string  `json:"name"`
		Score      int     `json:"score"`
		Backup     int64   `json:"backup"`
		FloatField float64 `json:"float_field"`
	}

	// WHM-style response with quoted integers
	body := []byte(`{"uid":"12345","max_users":"100","name":"example","score":"42","backup":1,"float_field":"3.14"}`)

	var v TestStruct
	// Standard json.Unmarshal will fail on "uid"
	err := json.Unmarshal(body, &v)
	if err == nil {
		t.Fatal("expected standard json.Unmarshal to fail with quoted int")
	}

	// relaxedUnmarshal should coerce
	var v2 TestStruct
	if err := relaxedUnmarshal(body, &v2); err != nil {
		t.Fatalf("relaxedUnmarshal failed: %v", err)
	}
	if v2.Uid != 12345 {
		t.Fatalf("Uid = %d, want 12345", v2.Uid)
	}
	if v2.MaxUsers != 100 {
		t.Fatalf("MaxUsers = %d, want 100", v2.MaxUsers)
	}
	if v2.Name != "example" {
		t.Fatalf("Name = %q, want example", v2.Name)
	}
	if v2.Score != 42 {
		t.Fatalf("Score = %d, want 42", v2.Score)
	}
	if v2.Backup != 1 {
		t.Fatalf("Backup = %d, want 1", v2.Backup)
	}
	if v2.FloatField != 3.14 {
		t.Fatalf("FloatField = %f, want 3.14", v2.FloatField)
	}
}

func TestRelaxedUnmarshalNestedStruct(t *testing.T) {
	type Inner struct {
		Value int64 `json:"value"`
	}
	type Outer struct {
		Item Inner `json:"item"`
	}

	body := []byte(`{"item":{"value":"42"}}`)
	var v Outer
	if err := relaxedUnmarshal(body, &v); err != nil {
		t.Fatalf("relaxedUnmarshal nested failed: %v", err)
	}
	if v.Item.Value != 42 {
		t.Fatalf("Value = %d, want 42", v.Item.Value)
	}
}

func TestRelaxedUnmarshalSlice(t *testing.T) {
	type Item struct {
		Count int64 `json:"count"`
	}
	type Container struct {
		Items []Item `json:"items"`
	}

	body := []byte(`{"items":[{"count":"10"},{"count":"20"}]}`)
	var v Container
	if err := relaxedUnmarshal(body, &v); err != nil {
		t.Fatalf("relaxedUnmarshal slice failed: %v", err)
	}
	if len(v.Items) != 2 || v.Items[0].Count != 10 || v.Items[1].Count != 20 {
		t.Fatalf("Items = %+v", v.Items)
	}
}

func TestRelaxedUnmarshalFastPath(t *testing.T) {
	type Simple struct {
		Name string `json:"name"`
	}
	body := []byte(`{"name":"hello"}`)
	var v Simple
	if err := relaxedUnmarshal(body, &v); err != nil {
		t.Fatalf("relaxedUnmarshal fast path failed: %v", err)
	}
	if v.Name != "hello" {
		t.Fatalf("Name = %q", v.Name)
	}
}

func TestRelaxedUnmarshalBool(t *testing.T) {
	type Config struct {
		Enabled bool `json:"enabled"`
	}
	body := []byte(`{"enabled":"1"}`)
	var v Config
	if err := relaxedUnmarshal(body, &v); err != nil {
		t.Fatalf("relaxedUnmarshal bool failed: %v", err)
	}
	if !v.Enabled {
		t.Fatal("enabled should be true")
	}
}

func TestRelaxedUnmarshalNumberToString(t *testing.T) {
	type Config struct {
		Limit string `json:"limit"`
	}
	body := []byte(`{"limit":500}`)
	var v Config
	if err := relaxedUnmarshal(body, &v); err != nil {
		t.Fatalf("relaxedUnmarshal number->string failed: %v", err)
	}
	if v.Limit != "500" {
		t.Fatalf("Limit = %q, want 500", v.Limit)
	}
}

func TestRelaxedUnmarshalBoolToString(t *testing.T) {
	type Config struct {
		Enabled string `json:"enabled"`
	}
	body := []byte(`{"enabled":true}`)
	var v Config
	if err := relaxedUnmarshal(body, &v); err != nil {
		t.Fatalf("relaxedUnmarshal bool->string failed: %v", err)
	}
	if v.Enabled != "1" {
		t.Fatalf("Enabled = %q, want 1", v.Enabled)
	}
}

func TestRelaxedUnmarshalPointerField(t *testing.T) {
	type Config struct {
		Limit *int64 `json:"limit"`
	}
	body := []byte(`{"limit":"500"}`)
	var v Config
	if err := relaxedUnmarshal(body, &v); err != nil {
		t.Fatalf("relaxedUnmarshal pointer field failed: %v", err)
	}
	if v.Limit == nil || *v.Limit != 500 {
		t.Fatalf("Limit = %v, want 500", v.Limit)
	}
}

func TestRelaxedUnmarshalWHMEnvelope(t *testing.T) {
	// Simulate full WHM envelope with quoted integers in data
	type AcctItem struct {
		Backup        int64  `json:"backup"`
		InodesUsed    int64  `json:"inodesused"`
		IsLocked      int64  `json:"is_locked"`
		Suspended     int64  `json:"suspended"`
		UnixStartdate int64  `json:"unix_startdate"`
		User          string `json:"user"`
		Domain        string `json:"domain"`
	}
	type AcctsData struct {
		Acct []AcctItem `json:"acct"`
	}

	body := []byte(`{"data":{"acct":[{"user":"bob","domain":"example.com","backup":"1","inodesused":"2500","is_locked":"0","suspended":"0","unix_startdate":"1700000000"}]},"metadata":{"command":"listaccts","reason":"OK","result":1,"version":1}}`)

	res, err := decodeWHMResult[AcctsData](body, "listaccts")
	if err != nil {
		t.Fatalf("decodeWHMResult with quoted ints: %v", err)
	}
	if !res.OK() {
		t.Fatal("result not OK")
	}
	if len(res.Data.Acct) != 1 || res.Data.Acct[0].User != "bob" {
		t.Fatalf("acct = %+v", res.Data.Acct)
	}
	if res.Data.Acct[0].Backup != 1 {
		t.Fatalf("Backup = %d, want 1", res.Data.Acct[0].Backup)
	}
	if res.Data.Acct[0].InodesUsed != 2500 {
		t.Fatalf("InodesUsed = %d, want 2500", res.Data.Acct[0].InodesUsed)
	}
	if res.Data.Acct[0].UnixStartdate != 1700000000 {
		t.Fatalf("UnixStartdate = %d, want 1700000000", res.Data.Acct[0].UnixStartdate)
	}
}

func TestRelaxedUnmarshalInvalidJSON(t *testing.T) {
	body := []byte(`{invalid}`)
	var v struct {
		Name string `json:"name"`
	}
	err := relaxedUnmarshal(body, &v)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

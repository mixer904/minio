// Copyright 2026 PGSTY contributors.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package cmd

import (
	"bytes"
	crand "crypto/rand"
	"crypto/rsa"
	"io"
	"testing"

	"github.com/minio/madmin-go/v3/estream"
)

func TestInspectDataUsesOnlyRequesterKey(t *testing.T) {
	privateKey, err := rsa.GenerateKey(crand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	var encoded bytes.Buffer
	writer := estream.NewWriter(&encoded)
	clusterInfo := []byte("local cluster metadata")
	if err = addInspectDataKey(writer, &privateKey.PublicKey, clusterInfo); err != nil {
		t.Fatal(err)
	}
	inspect, err := writer.AddEncryptedStream("inspect.zip", nil)
	if err != nil {
		t.Fatal(err)
	}
	inspectData := []byte("local inspect archive")
	if _, err = inspect.Write(inspectData); err != nil {
		t.Fatal(err)
	}
	if err = inspect.Close(); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}

	reader, err := estream.NewReader(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	reader.SetPrivateKey(privateKey)
	want := []struct {
		name string
		data []byte
	}{
		{name: "cluster.info", data: clusterInfo},
		{name: "inspect.zip", data: inspectData},
	}
	for _, expected := range want {
		stream, err := reader.NextStream()
		if err != nil {
			t.Fatalf("read %s: %v", expected.name, err)
		}
		if stream.Name != expected.name {
			t.Fatalf("stream name = %q, want %q", stream.Name, expected.name)
		}
		got, err := io.ReadAll(stream)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, expected.data) {
			t.Fatalf("%s = %q, want %q", expected.name, got, expected.data)
		}
	}
	if _, err = reader.NextStream(); err != io.EOF {
		t.Fatalf("final stream error = %v, want EOF", err)
	}
}

package awssts_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"nbox/internal/auth/awssts"
)

func TestInMemory_ParsesValidJSON(t *testing.T) {
	raw := []byte(`[
		{
			"arn": "arn:aws:iam::123456789012:role/entrypushd-watcher",
			"name": "prod-watcher",
			"roles": ["entrypushd"],
			"status": "active"
		}
	]`)

	s, err := awssts.NewInMemory(raw)
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}

	m, err := s.LookupByARN(context.Background(), "arn:aws:iam::123456789012:role/entrypushd-watcher")
	if err != nil {
		t.Fatalf("LookupByARN: %v", err)
	}
	if m.Name != "prod-watcher" {
		t.Errorf("Name = %q", m.Name)
	}
	if !slices.Equal(m.Roles, []string{"entrypushd"}) {
		t.Errorf("Roles = %v", m.Roles)
	}
	if m.Status != awssts.StatusActive {
		t.Errorf("Status = %q", m.Status)
	}
}

func TestInMemory_EmptyInput_NoMappings(t *testing.T) {
	s, err := awssts.NewInMemory(nil)
	if err != nil {
		t.Fatalf("NewInMemory(nil): %v", err)
	}
	if _, err := s.LookupByARN(context.Background(), "anything"); !errors.Is(err, awssts.ErrUnknownARN) {
		t.Fatalf("want ErrUnknownARN, got %v", err)
	}
}

func TestInMemory_InvalidJSON_ReturnsError(t *testing.T) {
	if _, err := awssts.NewInMemory([]byte(`not json`)); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestInMemory_MappingWithEmptyARN_ReturnsError(t *testing.T) {
	raw := []byte(`[{"arn":"","name":"x","roles":[],"status":"active"}]`)
	_, err := awssts.NewInMemory(raw)
	if err == nil {
		t.Fatal("expected error for empty ARN")
	}
	if !strings.Contains(err.Error(), "empty arn") {
		t.Errorf("error %q should mention 'empty arn'", err)
	}
}

func TestInMemory_DefaultStatusIsActive(t *testing.T) {
	raw := []byte(`[{"arn":"arn:aws:iam::1:role/x","name":"x","roles":[]}]`)
	s, err := awssts.NewInMemory(raw)
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}
	m, _ := s.LookupByARN(context.Background(), "arn:aws:iam::1:role/x")
	if m.Status != awssts.StatusActive {
		t.Errorf("Status = %q, want active (default)", m.Status)
	}
}

func TestInMemory_LookupByARN_UnknownARN_ReturnsErrUnknownARN(t *testing.T) {
	raw := []byte(`[{"arn":"arn:aws:iam::1:role/known","name":"x","roles":[],"status":"active"}]`)
	s, _ := awssts.NewInMemory(raw)
	if _, err := s.LookupByARN(context.Background(), "arn:aws:iam::1:role/unknown"); !errors.Is(err, awssts.ErrUnknownARN) {
		t.Fatalf("want ErrUnknownARN, got %v", err)
	}
}

func TestInMemory_DuplicateARN_LastWins(t *testing.T) {
	raw := []byte(`[
		{"arn":"arn:aws:iam::1:role/dup","name":"first","roles":[],"status":"active"},
		{"arn":"arn:aws:iam::1:role/dup","name":"second","roles":[],"status":"active"}
	]`)
	s, err := awssts.NewInMemory(raw)
	if err != nil {
		t.Fatalf("NewInMemory: %v", err)
	}
	m, _ := s.LookupByARN(context.Background(), "arn:aws:iam::1:role/dup")
	if m.Name != "second" {
		t.Errorf("Name = %q, want %q (last occurrence)", m.Name, "second")
	}
}

func TestInMemory_ParsesAttributes(t *testing.T) {
	raw := []byte(`[
		{
			"arn":"arn:aws:iam::1:role/x",
			"name":"x",
			"roles":["entrypushd"],
			"attributes":{"env":"production","team":"vault"},
			"status":"active"
		}
	]`)
	s, _ := awssts.NewInMemory(raw)
	m, _ := s.LookupByARN(context.Background(), "arn:aws:iam::1:role/x")
	if m.Attributes["env"] != "production" {
		t.Errorf("env = %q", m.Attributes["env"])
	}
	if m.Attributes["team"] != "vault" {
		t.Errorf("team = %q", m.Attributes["team"])
	}
}

func TestInMemory_AttributesOptional(t *testing.T) {
	raw := []byte(`[{"arn":"arn:aws:iam::1:role/x","name":"x","roles":[],"status":"active"}]`)
	s, _ := awssts.NewInMemory(raw)
	m, _ := s.LookupByARN(context.Background(), "arn:aws:iam::1:role/x")
	if m.Attributes != nil {
		t.Errorf("Attributes = %v, want nil", m.Attributes)
	}
}

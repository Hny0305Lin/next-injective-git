package main

import (
	"strings"
	"testing"
	"time"
)

func TestAuthorizeBindsTicketPayload(t *testing.T) {
	s := &service{
		secret:   []byte("01234567890123456789012345678901"),
		maxBytes: 1024,
	}
	req := replicationRequest{
		CID: "bafy-test", Owner: "inj1owner", Repo: "repo", Ref: "refs/heads/main",
		PackSHA256: strings.Repeat("a", 64), Size: 4,
		ExpiresAt: time.Now().Add(time.Minute).Unix(),
	}
	ticket, err := s.sign(claims{
		Kind: "replication", Subject: "alice", CID: req.CID, Owner: req.Owner, Repo: req.Repo,
		Ref: req.Ref, PackSHA256: req.PackSHA256, Size: req.Size,
		ExpiresAt: time.Now().Add(time.Minute).Unix(), JTI: "jti",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.authorize("Bearer "+ticket, req); err != nil {
		t.Fatal(err)
	}
	req.Ref = "refs/heads/other"
	if _, err := s.authorize("Bearer "+ticket, req); err == nil {
		t.Fatal("expected ticket payload binding failure")
	}
}

package api

import (
	"testing"

	"github.com/lejianwen/rustdesk-api/v2/model"
)

func TestGroupPeerPayloadIncludesManagedHashForAdministrator(t *testing.T) {
	p := &model.Peer{Id: "123456789", Hostname: "pc-a", Os: "windows"}
	payload := &GroupPeerPayload{}
	payload.FromPeerWithManagedHash(p, "owner", "group", true, "base64-derived-hash")
	if payload.Hash != "base64-derived-hash" {
		t.Fatalf("administrator did not receive managed hash: %+v", payload)
	}
}

func TestGroupPeerPayloadNeverIncludesManagedHashForOrdinaryUser(t *testing.T) {
	p := &model.Peer{Id: "123456789", Hostname: "pc-a", Os: "windows"}
	payload := &GroupPeerPayload{}
	payload.FromPeerWithManagedHash(p, "owner", "group", false, "base64-derived-hash")
	if payload.Hash != "" {
		t.Fatalf("ordinary user received managed hash: %+v", payload)
	}
}

package service

import (
	"strings"

	"github.com/lejianwen/rustdesk-api/v2/model"
)

func userDisplayName(user *model.User) string {
	if user == nil {
		return ""
	}
	return strings.TrimSpace(user.Nickname)
}

func (ps *PeerService) PopulateDisplayFields(peers []*model.Peer) error {
	userIDs := make([]uint, 0, len(peers))
	for _, peer := range peers {
		if peer != nil && peer.UserId > 0 {
			userIDs = append(userIDs, peer.UserId)
		}
	}

	usersByID := make(map[uint]*model.User)
	if len(userIDs) > 0 {
		var users []*model.User
		if err := DB.Select("id", "username", "nickname").Where("id IN ?", userIDs).Find(&users).Error; err != nil {
			return err
		}
		for _, user := range users {
			usersByID[user.Id] = user
		}
	}

	for _, peer := range peers {
		if peer == nil {
			continue
		}
		peer.UserDisplayName = userDisplayName(usersByID[peer.UserId])
		peer.SystemUsername = peer.Username
	}
	return nil
}

func (s *AddressBookService) PopulateDisplayFields(entries []*model.AddressBook) error {
	deviceIDs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry != nil && strings.TrimSpace(entry.Id) != "" {
			deviceIDs = append(deviceIDs, entry.Id)
		}
	}

	peersByID := make(map[string]*model.Peer)
	if len(deviceIDs) > 0 {
		var peers []*model.Peer
		if err := DB.Preload("User").Where("id IN ?", deviceIDs).Find(&peers).Error; err != nil {
			return err
		}
		for _, peer := range peers {
			peersByID[peer.Id] = peer
		}
	}

	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if peer := peersByID[entry.Id]; peer != nil {
			entry.UserDisplayName = userDisplayName(peer.User)
			entry.SystemUsername = peer.Username
			continue
		}
	}
	return nil
}

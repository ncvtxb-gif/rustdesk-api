package model

import "time"

const (
	DeviceIdentityStatusActive   = "active"
	DeviceIdentityStatusInactive = "inactive"
	DeviceIdentityStatusArchived = "archived"
)

type DeviceIdentity struct {
	IdModel
	AllocationUuid       string    `json:"allocation_uuid" gorm:"not null;uniqueIndex"`
	UserId               uint      `json:"user_id" gorm:"not null;uniqueIndex:idx_device_identity_user_machine"`
	MachineUuid          string    `json:"machine_uuid" gorm:"not null;uniqueIndex:idx_device_identity_user_machine"`
	RustdeskId           string    `json:"rustdesk_id" gorm:"not null;uniqueIndex"`
	CredentialCiphertext string    `json:"-" gorm:"not null"`
	CredentialNonce      string    `json:"-" gorm:"not null"`
	AuthenticationHash   string    `json:"-" gorm:"not null;default:''"`
	CredentialVersion    uint      `json:"credential_version" gorm:"not null;default:1"`
	KeyVersion           uint      `json:"-" gorm:"not null;default:1"`
	Status               string    `json:"status" gorm:"not null;default:active;index"`
	LastAuthAt           time.Time `json:"last_auth_at" gorm:"not null"`
	TimeModel
}

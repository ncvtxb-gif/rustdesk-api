package service

import (
	"crypto/aes"
	"crypto/cipher"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lejianwen/rustdesk-api/v2/config"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrMachineUUIDRequired       = errors.New("machine UUID is required")
	ErrDeviceIdentityArchived    = errors.New("device identity is archived")
	ErrDeviceIdentityNotFound    = errors.New("device identity was not found")
	ErrInvalidAuthenticationHash = errors.New("authentication hash must be base64 encoded SHA-256 output")
	ErrManagedDeviceUnauthorized = errors.New("managed device authorization failed")
)

type DeviceInfo struct {
	OS         string
	ClientType string
}

type DeviceIdentityService struct {
	aead cipher.AEAD
}

func NewDeviceIdentityService(cfg *config.DeviceIdentity) (*DeviceIdentityService, error) {
	if strings.TrimSpace(cfg.FeishuOidcOp) == "" {
		return nil, errors.New("Feishu OIDC provider is required")
	}
	if strings.TrimSpace(cfg.EnterpriseClientType) == "" {
		return nil, errors.New("enterprise client type is required")
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(cfg.MasterKey))
	if err != nil {
		return nil, fmt.Errorf("decode device identity master key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("device identity master key must decode to 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create device identity cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create device identity AEAD: %w", err)
	}
	return &DeviceIdentityService{aead: aead}, nil
}

func (s *DeviceIdentityService) AllocateOrGetDeviceIdentity(db *gorm.DB, userID uint, machineUUID string, _ DeviceInfo) (*model.DeviceIdentity, string, error) {
	machineUUID = strings.TrimSpace(machineUUID)
	if machineUUID == "" {
		return nil, "", ErrMachineUUIDRequired
	}
	var existing model.DeviceIdentity
	err := db.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND machine_uuid = ?", userID, machineUUID).First(&existing).Error
	if err == nil {
		if existing.Status == model.DeviceIdentityStatusArchived {
			return nil, "", ErrDeviceIdentityArchived
		}
		credential, decryptErr := s.DecryptCredential(&existing)
		if decryptErr != nil {
			return nil, "", decryptErr
		}
		now := time.Now().UTC()
		result := db.Model(&model.DeviceIdentity{}).
			Where("id = ? AND status <> ?", existing.Id, model.DeviceIdentityStatusArchived).
			Updates(map[string]interface{}{"last_auth_at": now, "status": model.DeviceIdentityStatusActive})
		if result.Error != nil {
			return nil, "", result.Error
		}
		if result.RowsAffected != 1 {
			return nil, "", ErrDeviceIdentityArchived
		}
		if updateErr := db.First(&existing, existing.Id).Error; updateErr != nil {
			return nil, "", updateErr
		}
		return &existing, credential, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, "", err
	}

	for attempt := 0; attempt < 32; attempt++ {
		rustdeskID, generateErr := randomRustdeskID()
		if generateErr != nil {
			return nil, "", generateErr
		}
		identity := &model.DeviceIdentity{
			AllocationUuid: uuid.NewString(), UserId: userID, MachineUuid: machineUUID,
			RustdeskId: rustdeskID, CredentialVersion: 1, KeyVersion: 1,
			Status: model.DeviceIdentityStatusActive, LastAuthAt: time.Now().UTC(),
		}
		credential, generateErr := randomCredential(20)
		if generateErr != nil {
			return nil, "", generateErr
		}
		if encryptErr := s.encryptCredential(identity, credential); encryptErr != nil {
			return nil, "", encryptErr
		}
		result := db.Clauses(clause.OnConflict{DoNothing: true}).Create(identity)
		if result.Error != nil {
			return nil, "", result.Error
		}
		if result.RowsAffected == 1 {
			return identity, credential, nil
		}
		if lookupErr := db.Where("user_id = ? AND machine_uuid = ?", userID, machineUUID).First(&existing).Error; lookupErr == nil {
			if existing.Status == model.DeviceIdentityStatusArchived {
				return nil, "", ErrDeviceIdentityArchived
			}
			credential, decryptErr := s.DecryptCredential(&existing)
			return &existing, credential, decryptErr
		}
	}
	return nil, "", errors.New("unable to allocate unique device identity")
}

func (s *DeviceIdentityService) LoginWithDeviceIdentity(user *model.User, loginLog *model.LoginLog) (*model.UserToken, *model.DeviceIdentity, string, error) {
	canonicalUUID, err := NormalizeMachineUUID(loginLog.Uuid)
	if err != nil {
		return nil, nil, "", err
	}
	loginLog.Uuid = canonicalUUID
	var token *model.UserToken
	var identity *model.DeviceIdentity
	var credential string
	err = DB.Transaction(func(tx *gorm.DB) error {
		var err error
		identity, credential, err = s.AllocateOrGetDeviceIdentity(tx, user.Id, loginLog.Uuid, DeviceInfo{OS: loginLog.Platform, ClientType: loginLog.Client})
		if err != nil {
			return err
		}
		if err = tx.Model(&model.DeviceIdentity{}).
			Where("machine_uuid = ? AND user_id <> ? AND status = ?", loginLog.Uuid, user.Id, model.DeviceIdentityStatusActive).
			Update("status", model.DeviceIdentityStatusInactive).Error; err != nil {
			return err
		}
		if err = tx.Where("device_uuid = ? AND user_id <> ?", loginLog.Uuid, user.Id).Delete(&model.UserToken{}).Error; err != nil {
			return err
		}
		tokenValue := (&UserService{}).GenerateToken(user)
		token = &model.UserToken{UserId: user.Id, Token: tokenValue, DeviceUuid: loginLog.Uuid, DeviceId: identity.RustdeskId, ExpiredAt: (&UserService{}).UserTokenExpireTimestamp(), Managed: true}
		if err = tx.Create(token).Error; err != nil {
			return err
		}
		loginLog.DeviceId = identity.RustdeskId
		loginLog.UserTokenId = token.Id
		return tx.Create(loginLog).Error
	})
	if err != nil {
		return nil, nil, "", err
	}
	if err := AllService.AddressBookService.EnsureCompanyDevice(user, identity.RustdeskId); err != nil {
		if Logger != nil {
			Logger.Warnf("company address book update failed after managed login: %v", err)
		}
	}
	return token, identity, credential, nil
}

func NormalizeMachineUUID(machineUUID string) (string, error) {
	machineUUID = strings.TrimSpace(machineUUID)
	if len(machineUUID) < 8 || len(machineUUID) > 512 {
		return "", ErrMachineUUIDRequired
	}
	decoded, err := base64.StdEncoding.DecodeString(machineUUID)
	if err != nil || len(decoded) == 0 || len(decoded) > 256 {
		return "", ErrMachineUUIDRequired
	}
	return machineUUID, nil
}

func (s *DeviceIdentityService) encryptCredential(identity *model.DeviceIdentity, credential string) error {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := cryptorand.Read(nonce); err != nil {
		return err
	}
	aad := []byte(identity.AllocationUuid + ":" + identity.RustdeskId)
	ciphertext := s.aead.Seal(nil, nonce, []byte(credential), aad)
	identity.CredentialNonce = base64.StdEncoding.EncodeToString(nonce)
	identity.CredentialCiphertext = base64.StdEncoding.EncodeToString(ciphertext)
	return nil
}

func (s *DeviceIdentityService) DecryptCredential(identity *model.DeviceIdentity) (string, error) {
	nonce, err := base64.StdEncoding.DecodeString(identity.CredentialNonce)
	if err != nil {
		return "", err
	}
	ciphertext, err := base64.StdEncoding.DecodeString(identity.CredentialCiphertext)
	if err != nil {
		return "", err
	}
	aad := []byte(identity.AllocationUuid + ":" + identity.RustdeskId)
	plaintext, err := s.aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func (s *DeviceIdentityService) SetAuthenticationHash(db *gorm.DB, userID uint, machineUUID, wireHash string) error {
	if len(wireHash) != base64.StdEncoding.EncodedLen(sha256.Size) {
		return ErrInvalidAuthenticationHash
	}
	decoded, err := base64.StdEncoding.DecodeString(wireHash)
	if err != nil || len(decoded) != 32 {
		return ErrInvalidAuthenticationHash
	}
	var identity model.DeviceIdentity
	if err := db.Where("user_id = ? AND machine_uuid = ?", userID, strings.TrimSpace(machineUUID)).First(&identity).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrDeviceIdentityNotFound
		}
		return err
	}
	if identity.Status == model.DeviceIdentityStatusArchived {
		return ErrDeviceIdentityArchived
	}
	if identity.Status != model.DeviceIdentityStatusActive {
		return ErrManagedDeviceUnauthorized
	}
	return db.Model(&identity).Update("authentication_hash", wireHash).Error
}

func (s *DeviceIdentityService) managedIdentityForToken(db *gorm.DB, userID uint, tokenValue, requestedUUID string) (*model.DeviceIdentity, error) {
	canonicalRequested, err := NormalizeMachineUUID(requestedUUID)
	if err != nil {
		return nil, ErrManagedDeviceUnauthorized
	}
	var token model.UserToken
	if err := db.Where("user_id = ? AND token = ?", userID, tokenValue).First(&token).Error; err != nil {
		return nil, ErrManagedDeviceUnauthorized
	}
	canonicalToken, err := NormalizeMachineUUID(token.DeviceUuid)
	if err != nil || canonicalToken != canonicalRequested || token.DeviceId == "" || (token.ExpiredAt > 0 && token.ExpiredAt <= time.Now().Unix()) {
		return nil, ErrManagedDeviceUnauthorized
	}
	var identity model.DeviceIdentity
	if err := db.Where("user_id = ? AND machine_uuid = ? AND rustdesk_id = ? AND status = ?", userID, canonicalRequested, token.DeviceId, model.DeviceIdentityStatusActive).First(&identity).Error; err != nil {
		return nil, ErrManagedDeviceUnauthorized
	}
	return &identity, nil
}

func (s *DeviceIdentityService) BootstrapForToken(db *gorm.DB, userID uint, tokenValue, machineUUID string) (*model.DeviceIdentity, string, int64, error) {
	identity, err := s.managedIdentityForToken(db, userID, tokenValue, machineUUID)
	if err != nil {
		return nil, "", 0, err
	}
	credential, err := s.DecryptCredential(identity)
	if err != nil {
		return nil, "", 0, err
	}
	var token model.UserToken
	if err := db.Select("expired_at").Where("user_id = ? AND token = ?", userID, tokenValue).First(&token).Error; err != nil {
		return nil, "", 0, ErrManagedDeviceUnauthorized
	}
	return identity, credential, token.ExpiredAt, nil
}

func (s *DeviceIdentityService) SetAuthenticationHashForToken(db *gorm.DB, userID uint, tokenValue, wireHash string) error {
	var token model.UserToken
	if err := db.Where("user_id = ? AND token = ?", userID, tokenValue).First(&token).Error; err != nil {
		return ErrManagedDeviceUnauthorized
	}
	identity, err := s.managedIdentityForToken(db, userID, tokenValue, token.DeviceUuid)
	if err != nil {
		return err
	}
	return s.SetAuthenticationHash(db, userID, identity.MachineUuid, wireHash)
}

func (s *DeviceIdentityService) AuthenticationHashByRustdeskID(db *gorm.DB, rustdeskID string) string {
	var identity model.DeviceIdentity
	if err := db.Select("authentication_hash").Where("rustdesk_id = ? AND status = ?", rustdeskID, model.DeviceIdentityStatusActive).First(&identity).Error; err != nil {
		return ""
	}
	return identity.AuthenticationHash
}

func randomRustdeskID() (string, error) {
	n, err := cryptorand.Int(cryptorand.Reader, big.NewInt(900000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%09d", n.Int64()+100000000), nil
}

func randomCredential(length int) (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
	out := make([]byte, length)
	for i := range out {
		n, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		out[i] = alphabet[n.Int64()]
	}
	return string(out), nil
}

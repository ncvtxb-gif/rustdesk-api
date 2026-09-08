package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	request "github.com/lejianwen/rustdesk-api/v2/http/request/api"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	apiResp "github.com/lejianwen/rustdesk-api/v2/http/response/api"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

type ManagedDevice struct{}

// SetAuthenticationHash stores target-derived authentication material, never plaintext credentials.
func (m *ManagedDevice) SetAuthenticationHash(c *gin.Context) {
	form := &request.ManagedDeviceAuthenticationHash{}
	if err := c.ShouldBindJSON(form); err != nil {
		response.Error(c, response.TranslateMsg(c, "ParamsError"))
		return
	}
	user := service.AllService.UserService.CurUser(c)
	tokenValue, ok := c.Get("token")
	if user == nil || !ok {
		response.Error(c, response.TranslateMsg(c, "NoAccess"))
		return
	}
	token, ok := tokenValue.(string)
	if !ok {
		response.Error(c, response.TranslateMsg(c, "NoAccess"))
		return
	}
	if service.AllService.DeviceIdentityService == nil {
		response.Error(c, response.TranslateMsg(c, "NoAccess"))
		return
	}
	if err := service.AllService.DeviceIdentityService.SetAuthenticationHashForToken(service.DB, user.Id, token, form.Hash); err != nil {
		response.Error(c, response.TranslateMsg(c, "OperationFailed"))
		return
	}
	c.Status(http.StatusNoContent)
}

func (m *ManagedDevice) Bootstrap(c *gin.Context) {
	form := &request.ManagedDeviceBootstrap{}
	if err := c.ShouldBindJSON(form); err != nil || !strings.EqualFold(strings.TrimSpace(form.Platform), "windows") {
		response.Error(c, response.TranslateMsg(c, "ParamsError"))
		return
	}
	user := service.AllService.UserService.CurUser(c)
	tokenValue, ok := c.Get("token")
	token, tokenOK := tokenValue.(string)
	if user == nil || !ok || !tokenOK || service.AllService.DeviceIdentityService == nil {
		response.Error(c, response.TranslateMsg(c, "NoAccess"))
		return
	}
	identity, credential, expiresAt, err := service.AllService.DeviceIdentityService.BootstrapForToken(service.DB, user.Id, token, form.MachineUUID)
	if err != nil {
		response.Error(c, response.TranslateMsg(c, "NoAccess"))
		return
	}
	c.JSON(http.StatusOK, &apiResp.ManagedDeviceBootstrapPayload{
		RustdeskId: identity.RustdeskId, PermanentPassword: credential, PasswordVersion: identity.CredentialVersion,
		Status: identity.Status, MachineUuid: identity.MachineUuid, SessionExpiresAt: expiresAt,
	})
}

package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	request "github.com/lejianwen/rustdesk-api/v2/http/request/api"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
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
	machineUUID := service.AllService.UserService.GetUuidByToken(user, token)
	if machineUUID == "" || service.AllService.DeviceIdentityService == nil {
		response.Error(c, response.TranslateMsg(c, "NoAccess"))
		return
	}
	if err := service.AllService.DeviceIdentityService.SetAuthenticationHash(service.DB, user.Id, machineUUID, form.Hash); err != nil {
		response.Error(c, response.TranslateMsg(c, "OperationFailed"))
		return
	}
	c.Status(http.StatusNoContent)
}

package api

type ManagedDeviceAuthenticationHash struct {
	Hash string `json:"hash" validate:"required" label:"hash"`
}
